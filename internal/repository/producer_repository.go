// internal/repository/producer_repository.go
// SQL-доступ к каталогу типов производителей и предметов студии (спека
// 2026-09-20-фабрики §3.1/§4): producer_types/items/producer_items.
// Тот же паттерн, что goods_repository.go: каждая мутация — транзакция с
// pg_advisory_xact_lock (catalogLockKey — один каталог, одна мутация за раз);
// снимок — одна транзакция чтения REPEATABLE READ. Ошибки — ErrCatalog.
package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"zorion/internal/goodsstudio/graph"
)

// ProducerTypeRow — тип производителя из БД (спека §3.1).
type ProducerTypeRow struct {
	ID         int64
	Name       string
	Kind       string // goods/items/energy
	CategoryID sql.NullInt64
	RaceFamily sql.NullString
	Output     []byte // JSONB
	Input      []byte // JSONB
	Params     []byte // JSONB
	Status     string
	CreatedAt  time.Time
}

// ItemRow — тип предмета из БД (спека §3.1; экземпляры — в инвентаре, не здесь).
type ItemRow struct {
	ID        int64
	Name      string
	SlotType  string
	Status    string
	Unlocks   []byte // JSONB
	Params    []byte // JSONB
	CreatedAt time.Time
}

// ProducerItemRow — связь «производитель предметов ↔ предмет» (спека §3.1).
type ProducerItemRow struct {
	ProducerTypeID int64
	ItemID         int64
	Requirements   []byte // JSONB
}

// nullStr — пустая строка → NULL (для nullable JSONB/TEXT).
func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// --- снимок ---

// loadProducerTypes — все типы производителей каталога.
func loadProducerTypes(q queryer) ([]ProducerTypeRow, error) {
	rows, err := q.Query(
		`SELECT id, name, kind, category_id, race_family, output, input, params, status, created_at
		 FROM producer_types ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProducerTypeRow
	for rows.Next() {
		var p ProducerTypeRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.CategoryID, &p.RaceFamily,
			&p.Output, &p.Input, &p.Params, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// loadItems — все предметы каталога.
func loadItems(q queryer) ([]ItemRow, error) {
	rows, err := q.Query(
		`SELECT id, name, slot_type, status, unlocks, params, created_at FROM items ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ItemRow
	for rows.Next() {
		var it ItemRow
		if err := rows.Scan(&it.ID, &it.Name, &it.SlotType, &it.Status, &it.Unlocks, &it.Params, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// loadProducerItems — все связи «производитель предметов ↔ предмет».
func loadProducerItems(q queryer) ([]ProducerItemRow, error) {
	rows, err := q.Query(
		`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProducerItemRow
	for rows.Next() {
		var pi ProducerItemRow
		if err := rows.Scan(&pi.ProducerTypeID, &pi.ItemID, &pi.Requirements); err != nil {
			return nil, err
		}
		out = append(out, pi)
	}
	return out, rows.Err()
}

// --- типы производителей ---

// CreateProducerType — создание типа производителя (спека §4.1): kind
// goods/items/energy; kind=goods — категория обязательна и должна
// существовать (иначе 400); дубликат нормализованного имени — 409.
func (r *GoodsRepository) CreateProducerType(name, kind string, categoryID *int64) (ProducerTypeRow, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return ProducerTypeRow{}, err
	}
	defer tx.Rollback()

	name = strings.TrimSpace(name)
	if name == "" {
		return ProducerTypeRow{}, errCatalog(400, "имя пустое")
	}
	if kind != "goods" && kind != "items" && kind != "energy" {
		return ProducerTypeRow{}, errCatalog(400, "неизвестный kind (goods/items/energy)")
	}
	if kind == "goods" && categoryID == nil {
		return ProducerTypeRow{}, errCatalog(400, "для kind=goods категория обязательна")
	}
	if categoryID != nil {
		var exists bool
		if err := tx.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1)`, *categoryID,
		).Scan(&exists); err != nil {
			return ProducerTypeRow{}, err
		}
		if !exists {
			return ProducerTypeRow{}, errCatalog(400, "категория не найдена")
		}
	}
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM producer_types WHERE name_norm = $1)`, graph.NormalizeName(name),
	).Scan(&exists); err != nil {
		return ProducerTypeRow{}, err
	}
	if exists {
		return ProducerTypeRow{}, errCatalog(409, "тип с таким именем уже есть")
	}

	var p ProducerTypeRow
	if err := tx.QueryRow(
		`INSERT INTO producer_types (name, name_norm, kind, category_id, status)
		 VALUES ($1, $2, $3, $4, 'draft') RETURNING id, name, kind, category_id, race_family, output, input, params, status, created_at`,
		name, graph.NormalizeName(name), kind, categoryID,
	).Scan(&p.ID, &p.Name, &p.Kind, &p.CategoryID, &p.RaceFamily, &p.Output, &p.Input, &p.Params, &p.Status, &p.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return ProducerTypeRow{}, errCatalog(409, "тип с таким именем уже есть")
		}
		return ProducerTypeRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProducerTypeRow{}, err
	}
	return p, nil
}

// UpdateProducerType — переименование/смена категории/семейства/JSON-полей
// (спека §4.1): категория для kind=goods должна существовать (400);
// дубликат имени — 409. JSON-поля (output/input/params) — валидный JSON.
func (r *GoodsRepository) UpdateProducerType(id int64, name *string, categoryID *int64, raceFamily *string, output, input, params *string) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var kind string
	err = tx.QueryRow(`SELECT kind FROM producer_types WHERE id = $1 FOR UPDATE`, id).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return errCatalog(404, "тип не найден")
	}
	if err != nil {
		return err
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return errCatalog(400, "имя пустое")
		}
		var exists bool
		if err := tx.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM producer_types WHERE name_norm = $1 AND id <> $2)`,
			graph.NormalizeName(trimmed), id,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return errCatalog(409, "тип с таким именем уже есть")
		}
		if _, err := tx.Exec(`UPDATE producer_types SET name = $1, name_norm = $2 WHERE id = $3`, trimmed, graph.NormalizeName(trimmed), id); err != nil {
			return err
		}
	}
	if categoryID != nil {
		if kind == "goods" {
			var exists bool
			if err := tx.QueryRow(
				`SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1)`, *categoryID,
			).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return errCatalog(400, "категория не найдена")
			}
		}
		if _, err := tx.Exec(`UPDATE producer_types SET category_id = $1 WHERE id = $2`, *categoryID, id); err != nil {
			return err
		}
	}
	if raceFamily != nil {
		if _, err := tx.Exec(`UPDATE producer_types SET race_family = $1 WHERE id = $2`, nullStr(*raceFamily), id); err != nil {
			return err
		}
	}
	for col, v := range map[string]*string{"output": output, "input": input, "params": params} {
		if v == nil {
			continue
		}
		if !json.Valid([]byte(*v)) {
			return errCatalog(400, col+" — невалидный JSON")
		}
		if _, err := tx.Exec(`UPDATE producer_types SET `+col+` = $1 WHERE id = $2`, nullStr(*v), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteProducerType — удаление типа (связи producer_items — каскадом).
func (r *GoodsRepository) DeleteProducerType(id int64) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM producer_types WHERE id = $1 FOR UPDATE)`, id).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "тип не найден")
	}
	if _, err := tx.Exec(`DELETE FROM producer_types WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetProducerTypeStatus — смена статуса типа (draft/approved/excluded/banned/unban).
func (r *GoodsRepository) SetProducerTypeStatus(id int64, status string) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM producer_types WHERE id = $1 FOR UPDATE)`, id,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "тип не найден")
	}
	switch status {
	case "banned":
		_, err = tx.Exec(`UPDATE producer_types SET status = 'banned' WHERE id = $1`, id)
	case "unban":
		_, err = tx.Exec(`UPDATE producer_types SET status = 'draft' WHERE id = $1`, id)
	case "draft", "approved", "excluded":
		_, err = tx.Exec(`UPDATE producer_types SET status = $1 WHERE id = $2`, status, id)
	default:
		return errCatalog(400, "неизвестный статус")
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// LinkProducerItem — привязать предмет к производителю (спека §4.1):
// производитель должен быть kind=items (иначе 400); предмет должен
// существовать (404); дубликат связи — 409.
func (r *GoodsRepository) LinkProducerItem(producerTypeID, itemID int64, requirements *string) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var kind string
	err = tx.QueryRow(`SELECT kind FROM producer_types WHERE id = $1 FOR UPDATE`, producerTypeID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return errCatalog(404, "тип не найден")
	}
	if err != nil {
		return err
	}
	if kind != "items" {
		return errCatalog(400, "предметы привязываются только к производителям kind=items")
	}
	var itemExists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM items WHERE id = $1)`, itemID,
	).Scan(&itemExists); err != nil {
		return err
	}
	if !itemExists {
		return errCatalog(404, "предмет не найден")
	}
	var linked bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM producer_items WHERE producer_type_id = $1 AND item_id = $2)`,
		producerTypeID, itemID,
	).Scan(&linked); err != nil {
		return err
	}
	if linked {
		return errCatalog(409, "предмет уже привязан")
	}
	var req interface{}
	if requirements != nil {
		if !json.Valid([]byte(*requirements)) {
			return errCatalog(400, "requirements — невалидный JSON")
		}
		req = *requirements
	}
	if _, err := tx.Exec(
		`INSERT INTO producer_items (producer_type_id, item_id, requirements) VALUES ($1, $2, $3)`,
		producerTypeID, itemID, req,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// UnlinkProducerItem — отвязать предмет от производителя.
func (r *GoodsRepository) UnlinkProducerItem(producerTypeID, itemID int64) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM producer_items WHERE producer_type_id = $1 AND item_id = $2)`,
		producerTypeID, itemID,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "связь не найдена")
	}
	if _, err := tx.Exec(
		`DELETE FROM producer_items WHERE producer_type_id = $1 AND item_id = $2`, producerTypeID, itemID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// --- предметы ---

// CreateItem — создание предмета (спека §4.2): slot_type обязателен;
// дубликат нормализованного имени — 409.
func (r *GoodsRepository) CreateItem(name, slotType string) (ItemRow, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return ItemRow{}, err
	}
	defer tx.Rollback()

	name = strings.TrimSpace(name)
	if name == "" {
		return ItemRow{}, errCatalog(400, "имя пустое")
	}
	slotType = strings.TrimSpace(slotType)
	if slotType == "" {
		return ItemRow{}, errCatalog(400, "slot_type пустой")
	}
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM items WHERE name_norm = $1)`, graph.NormalizeName(name),
	).Scan(&exists); err != nil {
		return ItemRow{}, err
	}
	if exists {
		return ItemRow{}, errCatalog(409, "предмет с таким именем уже есть")
	}

	var it ItemRow
	if err := tx.QueryRow(
		`INSERT INTO items (name, name_norm, slot_type, status)
		 VALUES ($1, $2, $3, 'draft') RETURNING id, name, slot_type, status, unlocks, params, created_at`,
		name, graph.NormalizeName(name), slotType,
	).Scan(&it.ID, &it.Name, &it.SlotType, &it.Status, &it.Unlocks, &it.Params, &it.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return ItemRow{}, errCatalog(409, "предмет с таким именем уже есть")
		}
		return ItemRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return ItemRow{}, err
	}
	return it, nil
}

// UpdateItem — переименование/смена slot_type/JSON-полей (unlocks/params).
func (r *GoodsRepository) UpdateItem(id int64, name *string, slotType *string, unlocks, params *string) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM items WHERE id = $1 FOR UPDATE)`, id).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "предмет не найден")
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return errCatalog(400, "имя пустое")
		}
		var dup bool
		if err := tx.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM items WHERE name_norm = $1 AND id <> $2)`,
			graph.NormalizeName(trimmed), id,
		).Scan(&dup); err != nil {
			return err
		}
		if dup {
			return errCatalog(409, "предмет с таким именем уже есть")
		}
		if _, err := tx.Exec(`UPDATE items SET name = $1, name_norm = $2 WHERE id = $3`, trimmed, graph.NormalizeName(trimmed), id); err != nil {
			return err
		}
	}
	if slotType != nil {
		st := strings.TrimSpace(*slotType)
		if st == "" {
			return errCatalog(400, "slot_type пустой")
		}
		if _, err := tx.Exec(`UPDATE items SET slot_type = $1 WHERE id = $2`, st, id); err != nil {
			return err
		}
	}
	for col, v := range map[string]*string{"unlocks": unlocks, "params": params} {
		if v == nil {
			continue
		}
		if !json.Valid([]byte(*v)) {
			return errCatalog(400, col+" — невалидный JSON")
		}
		if _, err := tx.Exec(`UPDATE items SET `+col+` = $1 WHERE id = $2`, nullStr(*v), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteItem — удаление предмета (связи producer_items — каскадом).
func (r *GoodsRepository) DeleteItem(id int64) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM items WHERE id = $1 FOR UPDATE)`, id).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "предмет не найден")
	}
	if _, err := tx.Exec(`DELETE FROM items WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetItemStatus — смена статуса предмета (draft/approved/excluded/banned/unban).
func (r *GoodsRepository) SetItemStatus(id int64, status string) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM items WHERE id = $1 FOR UPDATE)`, id,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "предмет не найден")
	}
	switch status {
	case "banned":
		_, err = tx.Exec(`UPDATE items SET status = 'banned' WHERE id = $1`, id)
	case "unban":
		_, err = tx.Exec(`UPDATE items SET status = 'draft' WHERE id = $1`, id)
	case "draft", "approved", "excluded":
		_, err = tx.Exec(`UPDATE items SET status = $1 WHERE id = $2`, status, id)
	default:
		return errCatalog(400, "неизвестный статус")
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}