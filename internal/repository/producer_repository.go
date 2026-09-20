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

// ProducerTypeRow — тип производителя из БД (спека §3.1 + дерево построек
// 2026-09-21 §1.2): parent_id — базовый тип (подтип → тип-родитель),
// race — второй уровень расовости (id расы из config/races.json).
type ProducerTypeRow struct {
	ID         int64
	Name       string
	Kind       string // goods/items/energy
	CategoryID sql.NullInt64
	RaceFamily sql.NullString
	ParentID   sql.NullInt64
	Race       sql.NullString
	Output     []byte // JSONB
	Input      []byte // JSONB
	Params     []byte // JSONB
	Status     string
	CreatedAt  time.Time
}

// ProducerSlotRow — слот родителя (спека 2026-09-21-скрытые §1.1): «родитель
// (тип kind=goods) предлагает категорию на уровне расовости»; hidden —
// «предлагает, но скрыто». Расовая привязка — существующие race_family/race.
type ProducerSlotRow struct {
	ID         int64
	ParentID   int64
	CategoryID int64
	RaceFamily sql.NullString
	Race       sql.NullString
	Hidden     bool
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

// nullStrPtr — *string → NULL (nil или пустая строка → NULL).
func nullStrPtr(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

// nullStrNS — *string → sql.NullString (типизированный NULL). Баг B2
// (2026-09-21): untyped nil от nullStr/nullStrPtr в предикате «$n IS NULL»
// DeleteProducerSlot ронял запрос «could not determine data type of parameter»
// — PostgreSQL не выводит тип параметра без типизированного контекста.
// Nullable текстовые параметры слотов передаются как sql.NullString
// (database/sql конвертирует через driver.Value → NULL/string).
func nullStrNS(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// subtypeTupleChanged — изменился ли кортеж подтипа (parent_id, category_id,
// race_family, race) относительно текущих значений записи. Пустая строка
// эквивалентна NULL (nullStrPtr); итоговые значения уже учитывают «не менять»
// (nil-указатель = текущее). Используется в UpdateProducerType: уникальность
// подтипа проверяется ТОЛЬКО при фактическом изменении кортежа — чистое
// переименование не проверяется (лаборатории легитимно делят кортеж, B1).
func subtypeTupleChanged(curParent, curCategory sql.NullInt64, curFamily, curRace sql.NullString,
	finalParent *int64, finalCategory *int64, finalFamily, finalRace *string) bool {
	if (curParent.Valid && (finalParent == nil || *finalParent != curParent.Int64)) ||
		(!curParent.Valid && finalParent != nil) {
		return true
	}
	if (curCategory.Valid && (finalCategory == nil || *finalCategory != curCategory.Int64)) ||
		(!curCategory.Valid && finalCategory != nil) {
		return true
	}
	curFam, newFam := "", ""
	if curFamily.Valid {
		curFam = curFamily.String
	}
	if finalFamily != nil {
		newFam = *finalFamily
	}
	if curFam != newFam {
		return true
	}
	curR, newR := "", ""
	if curRace.Valid {
		curR = curRace.String
	}
	if finalRace != nil {
		newR = *finalRace
	}
	return curR != newR
}

// producerSlotApplied — существует ли применяемый к уровню расовости записи
// слот родителя (спека скрытых §1.4 п.1/§2.1): слот с race_family IS NULL
// (база, покрывает все уровни), или race_family = семейства записи, или
// race = расы записи. Наследуемая база засчитывается; скрытость слота
// ортогональна (не блокирует — карточка завода прячется, завод существует).
// family/race — sql.NullString (типизированный NULL, баг B2).
func producerSlotApplied(q queryer, parentID, categoryID int64, family, race sql.NullString) (bool, error) {
	var ok bool
	err := q.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM producer_slots
		 WHERE parent_id = $1 AND category_id = $2
		   AND (race_family IS NULL OR race_family = $3 OR race = $4))`,
		parentID, categoryID, family, race,
	).Scan(&ok)
	return ok, err
}

// --- снимок ---

// loadProducerTypes — все типы производителей каталога.
func loadProducerTypes(q queryer) ([]ProducerTypeRow, error) {
	rows, err := q.Query(
		`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, status, created_at
		 FROM producer_types ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProducerTypeRow
	for rows.Next() {
		var p ProducerTypeRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.CategoryID, &p.RaceFamily,
			&p.ParentID, &p.Race, &p.Output, &p.Input, &p.Params, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// loadProducerSlots — все слоты родителя (конфигурация категорий, спека §1.1).
func loadProducerSlots(q queryer) ([]ProducerSlotRow, error) {
	rows, err := q.Query(
		`SELECT id, parent_id, category_id, race_family, race, hidden, created_at
		 FROM producer_slots ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProducerSlotRow
	for rows.Next() {
		var s ProducerSlotRow
		if err := rows.Scan(&s.ID, &s.ParentID, &s.CategoryID, &s.RaceFamily, &s.Race, &s.Hidden, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
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

// CreateProducerType — создание типа производителя (спека §4.1 + дерево
// построек 2026-09-21 §1.2 + спека скрытых §1.4): kind goods/items/energy;
// parent_id — базовый тип (подтип → тип-родитель). Инварианты §1.2: глубина 1
// (родитель обязан быть типом), kind подтипа = kind родителя, категория —
// только у подтипов kind=goods (тип kind=goods абстрактен, категория = NULL),
// уникальность подтипа (parent_id, category_id, race_family, race) — 409,
// дубликат нормализованного имени — 409. Слот-инвариант С4 (спека скрытых
// §1.4 п.1): подтип kind=goods требует применяемого слота родителя
// (parent_id, category_id, уровень расовости записи) — иначе 400. Соответствие
// race → race_family каталогу рас проверяет хендлер (races.LoreByID); здесь —
// структурная проверка «раса задана → семейство задано».
func (r *GoodsRepository) CreateProducerType(name, kind string, categoryID *int64, parentID *int64, raceFamily, race *string) (ProducerTypeRow, error) {
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
	// Родитель: существует, обязан быть типом (глубина 1), kind наследуется.
	if parentID != nil {
		var parentKind string
		var parentIsType bool
		err := tx.QueryRow(
			`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = $1`, *parentID,
		).Scan(&parentKind, &parentIsType)
		if errors.Is(err, sql.ErrNoRows) {
			return ProducerTypeRow{}, errCatalog(400, "родитель не найден")
		}
		if err != nil {
			return ProducerTypeRow{}, err
		}
		if !parentIsType {
			return ProducerTypeRow{}, errCatalog(400, "подтип подтипа запрещён (глубина 1)")
		}
		if kind != parentKind {
			return ProducerTypeRow{}, errCatalog(400, "kind подтипа = kind родителя")
		}
	}
	// Категория — только у подтипов kind=goods (спека §1.2 п.4): тип
	// (parent_id NULL) с kind=goods абстрактен, категория = NULL.
	if kind == "goods" {
		if parentID == nil && categoryID != nil {
			return ProducerTypeRow{}, errCatalog(400, "у типа kind=goods категория не задаётся — категории живут в подтипах")
		}
		if parentID != nil && categoryID == nil {
			return ProducerTypeRow{}, errCatalog(400, "для подтипа kind=goods категория обязательна")
		}
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
	// Слот-инвариант С4 (спека скрытых §1.4 п.1): подтип kind=goods требует
	// применяемого слота родителя на уровне расовости записи (§2.1).
	// Наследуемый слот базы засчитывается.
	if parentID != nil && kind == "goods" && categoryID != nil {
		hasSlot, err := producerSlotApplied(tx, *parentID, *categoryID, nullStrNS(raceFamily), nullStrNS(race))
		if err != nil {
			return ProducerTypeRow{}, err
		}
		if !hasSlot {
			return ProducerTypeRow{}, errCatalog(400, "категория не настроена у родителя на этом уровне")
		}
	}
	// Раса задана → семейство задано (соответствие каталогу — в хендлере).
	if race != nil && *race != "" && (raceFamily == nil || *raceFamily == "") {
		return ProducerTypeRow{}, errCatalog(400, "раса задана — семейство рас обязательно")
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
	// Уникальность подтипа: (parent_id, category_id, race_family, race) — 409.
	if parentID != nil {
		var dup bool
		if err := tx.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM producer_types
			 WHERE parent_id = $1 AND category_id IS NOT DISTINCT FROM $2
			   AND race_family IS NOT DISTINCT FROM $3 AND race IS NOT DISTINCT FROM $4)`,
			*parentID, categoryID, nullStrPtr(raceFamily), nullStrPtr(race),
		).Scan(&dup); err != nil {
			return ProducerTypeRow{}, err
		}
		if dup {
			return ProducerTypeRow{}, errCatalog(409, "подтип с такими (родитель, категория, семейство, раса) уже есть")
		}
	}

	var p ProducerTypeRow
	if err := tx.QueryRow(
		`INSERT INTO producer_types (name, name_norm, kind, category_id, race_family, parent_id, race, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'draft') RETURNING id, name, kind, category_id, race_family, parent_id, race, output, input, params, status, created_at`,
		name, graph.NormalizeName(name), kind, categoryID, nullStrPtr(raceFamily), parentID, nullStrPtr(race),
	).Scan(&p.ID, &p.Name, &p.Kind, &p.CategoryID, &p.RaceFamily, &p.ParentID, &p.Race, &p.Output, &p.Input, &p.Params, &p.Status, &p.CreatedAt); err != nil {
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

// UpdateProducerType — переименование/смена категории/семейства/расы/
// родителя/JSON-полей (спека §4.1 + дерево построек 2026-09-21 §1.2 + спека
// скрытых §1.4): те же инварианты, что в CreateProducerType (глубина 1, kind
// наследуется, категория только у подтипов kind=goods, уникальность подтипа —
// 409); дубликат имени — 409. parentID — **int64: nil = не менять, &id = новый
// родитель. Снятие родителя (подтип → тип) через API не поддерживается
// (UI родителя не редактирует); тип с подтипами нельзя сделать подтипом
// (глубина 2) — 409. Слот-инвариант С4 (§1.4 п.1): смена категории у подтипа
// kind=goods требует применяемого слота нового родителя. JSON-поля
// (output/input/params) — валидный JSON.
func (r *GoodsRepository) UpdateProducerType(id int64, name *string, categoryID *int64, parentID **int64, raceFamily *string, race *string, output, input, params *string) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var kind string
	var curParent sql.NullInt64
	var curCategory sql.NullInt64
	var curRaceFamily sql.NullString
	var curRace sql.NullString
	err = tx.QueryRow(
		`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = $1 FOR UPDATE`, id,
	).Scan(&kind, &curParent, &curCategory, &curRaceFamily, &curRace)
	if errors.Is(err, sql.ErrNoRows) {
		return errCatalog(404, "тип не найден")
	}
	if err != nil {
		return err
	}
	// Итоговые значения: заданы в теле → они; иначе текущие.
	var finalParent *int64
	if parentID != nil {
		finalParent = *parentID
	} else if curParent.Valid {
		finalParent = &curParent.Int64
	}
	finalCategory := categoryID
	if finalCategory == nil && curCategory.Valid {
		finalCategory = &curCategory.Int64
	}
	finalRaceFamily := raceFamily
	if finalRaceFamily == nil && curRaceFamily.Valid {
		finalRaceFamily = &curRaceFamily.String
	}
	finalRace := race
	if finalRace == nil && curRace.Valid {
		finalRace = &curRace.String
	}
	// Слот-инвариант С4 (спека скрытых §1.4 п.1): подтип kind=goods требует
	// применяемого слота родителя на ИТОГОВОМ уровне расовости записи.
	// Проверяется при изменении кортежа (категория/семейство/раса могли уйти
	// на уровень без слота); наследуемая база засчитывается (для сид-типов с
	// universal-слотами 400 не возникает).
	if kind == "goods" && finalParent != nil && finalCategory != nil &&
		(categoryID != nil || raceFamily != nil || race != nil) {
		hasSlot, err := producerSlotApplied(tx, *finalParent, *finalCategory, nullStrNS(finalRaceFamily), nullStrNS(finalRace))
		if err != nil {
			return err
		}
		if !hasSlot {
			return errCatalog(400, "категория не настроена у родителя на этом уровне")
		}
	}
	if parentID != nil {
		// Снятие родителя (подтип → тип) через API не поддерживается: UI
		// родителя не редактирует (задаётся при создании подтипа), из JSON
		// null = «не менять» — ветка «снять» недостижима.
		if *parentID == nil {
			return errCatalog(400, "родитель не задан — снятие родителя через API не поддерживается")
		}
		np := **parentID
		if np == id {
			return errCatalog(400, "тип не может быть родителем сам себе")
		}
		var parentKind string
		var parentIsType bool
		err := tx.QueryRow(
			`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = $1`, np,
		).Scan(&parentKind, &parentIsType)
		if errors.Is(err, sql.ErrNoRows) {
			return errCatalog(400, "родитель не найден")
		}
		if err != nil {
			return err
		}
		if !parentIsType {
			return errCatalog(400, "подтип подтипа запрещён (глубина 1)")
		}
		if kind != parentKind {
			return errCatalog(400, "kind подтипа = kind родителя")
		}
		// Глубина 2 запрещена (§1.2 п.2): тип с подтипами нельзя сделать
		// подтипом — его дети стали бы подтипами подтипа.
		var hasSubtypes bool
		if err := tx.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM producer_types WHERE parent_id = $1)`, id,
		).Scan(&hasSubtypes); err != nil {
			return err
		}
		if hasSubtypes {
			return errCatalog(409, "нельзя сделать тип с подтипами подтипом — сначала удалите подтипы")
		}
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
		// Категория — только у подтипов kind=goods (спека §1.2 п.4).
		if kind == "goods" && finalParent == nil {
			return errCatalog(400, "у типа kind=goods категория не задаётся — категории живут в подтипах")
		}
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
			// Применяемый слот проверен выше (С4, §1.4 п.1) по итоговому уровню.
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
	if race != nil {
		// Раса задана → семейство задано (итоговое: новое или текущее).
		if *race != "" && (finalRaceFamily == nil || *finalRaceFamily == "") {
			return errCatalog(400, "раса задана — семейство рас обязательно")
		}
		if _, err := tx.Exec(`UPDATE producer_types SET race = $1 WHERE id = $2`, nullStr(*race), id); err != nil {
			return err
		}
	}
	// Уникальность подтипа: итоговый (parent_id, category_id, race_family, race).
	// Проверяется ТОЛЬКО при фактическом изменении кортежа — чистое
	// переименование не проверяется (лаборатории легитимно делят кортеж, B1).
	if finalParent != nil && subtypeTupleChanged(curParent, curCategory, curRaceFamily, curRace,
		finalParent, finalCategory, finalRaceFamily, finalRace) {
		var dup bool
		if err := tx.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM producer_types
			 WHERE parent_id = $1 AND category_id IS NOT DISTINCT FROM $2
			   AND race_family IS NOT DISTINCT FROM $3 AND race IS NOT DISTINCT FROM $4
			   AND id <> $5)`,
			*finalParent, finalCategory, nullStrPtr(finalRaceFamily), nullStrPtr(finalRace), id,
		).Scan(&dup); err != nil {
			return err
		}
		if dup {
			return errCatalog(409, "подтип с такими (родитель, категория, семейство, раса) уже есть")
		}
	}
	if parentID != nil {
		// **int64: nil = не менять; &id = новый родитель (снятие родителя
		// не поддерживается — проверено выше).
		if _, err := tx.Exec(`UPDATE producer_types SET parent_id = $1 WHERE id = $2`, **parentID, id); err != nil {
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
// RESTRICT (спека 2026-09-21 §1.2 п.7): тип с подтипами не удаляется — 409
// «сначала удалите подтипы» (каскад запрещён: снос Фабрики не должен уносить
// фабрики категорий).
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
	var hasSubtypes bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM producer_types WHERE parent_id = $1)`, id,
	).Scan(&hasSubtypes); err != nil {
		return err
	}
	if hasSubtypes {
		return errCatalog(409, "сначала удалите подтипы")
	}
	if _, err := tx.Exec(`DELETE FROM producer_types WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- слоты родителя (спека 2026-09-21-скрытые §1.1/§1.4/§4) ---

// CreateSlot — создать слот родителя (переопределение уровня / база на
// универсальном): «родитель предлагает категорию на уровне расовости».
// Валидации (§1.4 п.3): родитель — тип kind=goods (400); категория существует
// (400); раса задана → семейство задано (структурно; каталог — хендлер).
// Дубликат кортежа (parent_id, category_id, race_family, race), NULL-safe —
// 409 (UNIQUE NULLS NOT DISTINCT — страховка на уровне БД).
func (r *GoodsRepository) CreateProducerSlot(parentID, categoryID int64, raceFamily, race *string, hidden bool) (ProducerSlotRow, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return ProducerSlotRow{}, err
	}
	defer tx.Rollback()

	var parentKind string
	var parentIsType bool
	err = tx.QueryRow(
		`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = $1`, parentID,
	).Scan(&parentKind, &parentIsType)
	if errors.Is(err, sql.ErrNoRows) {
		return ProducerSlotRow{}, errCatalog(400, "родитель не найден")
	}
	if err != nil {
		return ProducerSlotRow{}, err
	}
	if !parentIsType || parentKind != "goods" {
		return ProducerSlotRow{}, errCatalog(400, "слоты только у типов kind=goods")
	}
	var catExists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1)`, categoryID,
	).Scan(&catExists); err != nil {
		return ProducerSlotRow{}, err
	}
	if !catExists {
		return ProducerSlotRow{}, errCatalog(400, "категория не найдена")
	}
	if race != nil && *race != "" && (raceFamily == nil || *raceFamily == "") {
		return ProducerSlotRow{}, errCatalog(400, "раса задана — семейство рас обязательно")
	}
	var dup bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM producer_slots
		 WHERE parent_id = $1 AND category_id = $2
		   AND race_family IS NOT DISTINCT FROM $3 AND race IS NOT DISTINCT FROM $4)`,
		parentID, categoryID, nullStrNS(raceFamily), nullStrNS(race),
	).Scan(&dup); err != nil {
		return ProducerSlotRow{}, err
	}
	if dup {
		return ProducerSlotRow{}, errCatalog(409, "слот с такими (родитель, категория, семейство, раса) уже есть")
	}
	var s ProducerSlotRow
	if err := tx.QueryRow(
		`INSERT INTO producer_slots (parent_id, category_id, race_family, race, hidden)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, parent_id, category_id, race_family, race, hidden, created_at`,
		parentID, categoryID, nullStrNS(raceFamily), nullStrNS(race), hidden,
	).Scan(&s.ID, &s.ParentID, &s.CategoryID, &s.RaceFamily, &s.Race, &s.Hidden, &s.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return ProducerSlotRow{}, errCatalog(409, "слот с такими (родитель, категория, семейство, раса) уже есть")
		}
		return ProducerSlotRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProducerSlotRow{}, err
	}
	return s, nil
}

// UpdateSlotHidden — переключение скрытости слота (PUT /slots/{id} {hidden}).
// hidden не входит в кортеж уникальности — 409 не триггерится (как М1 первой
// волны, §1.4 п.4).
func (r *GoodsRepository) UpdateProducerSlotHidden(id int64, hidden bool) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM producer_slots WHERE id = $1 FOR UPDATE)`, id,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "слот не найден")
	}
	if _, err := tx.Exec(`UPDATE producer_slots SET hidden = $1 WHERE id = $2`, hidden, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteSlot — удалить слот (снять переопределение уровня / убрать категорию
// из набора родителя). RESTRICT (§1.4 п.2): при существующих подтипах
// kind=goods (parent_id, category_id), применяемых к слоту — 409 «сначала
// удалите заводы категории». Применяемость по §2.1 (обратное направление):
// универсальный слот (race_family IS NULL) покрывает ВСЕ записи уровня;
// семейный (race_family=Fk, race NULL) — записи с race_family=Fk (семейные и
// расовые семейства); расовый (race=R) — ТОЛЬКО записи с race=R (не семейные
// записи того же семейства). Записей нет → удаление свободно.
func (r *GoodsRepository) DeleteProducerSlot(id int64) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var parentID, categoryID int64
	var slotFamily, slotRace sql.NullString
	err = tx.QueryRow(
		`SELECT parent_id, category_id, race_family, race FROM producer_slots WHERE id = $1 FOR UPDATE`, id,
	).Scan(&parentID, &categoryID, &slotFamily, &slotRace)
	if errors.Is(err, sql.ErrNoRows) {
		return errCatalog(404, "слот не найден")
	}
	if err != nil {
		return err
	}
	// Предикат применяемости: универсальный ($3 NULL) — все; семейный
	// ($4 NULL) — race_family = $3; расовый ($4 задан) — race = $4.
	// $3/$4 — nullable; касты ::text фиксируют тип параметра В SQL
	// (драйвер-независимо): lib/pq шлёт NULL с OID 0, и PostgreSQL не выводит
	// тип из «$n IS NULL»/«$n IS NOT NULL» — «could not determine data type
	// of parameter $3» (баг B2, 2026-09-21, третий раунд).
	var hasFactories bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM producer_types pt
		 WHERE pt.parent_id = $1 AND pt.category_id = $2 AND pt.kind = 'goods'
		   AND ($3::text IS NULL OR (($4::text IS NOT NULL AND pt.race = $4::text)
		                          OR ($4::text IS NULL AND pt.race_family = $3::text))))`,
		parentID, categoryID, slotFamily, slotRace,
	).Scan(&hasFactories); err != nil {
		return err
	}
	if hasFactories {
		return errCatalog(409, "сначала удалите заводы категории")
	}
	if _, err := tx.Exec(`DELETE FROM producer_slots WHERE id = $1`, id); err != nil {
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