// internal/repository/goods_repository.go
// SQL-доступ к каталогу товаров/ресурсов студии (спека
// переноса-студии-товаров-iterA §7): categories/goods/goods_slots.
// Каждая мутация — транзакция с pg_advisory_xact_lock (сериализация
// мутаций каталога, §9.1); снимок state — одна транзакция чтения
// REPEATABLE READ (§8.1). Ошибки — ErrCatalog с HTTP-статусом
// (400/403/404/409), хендлеры маппят в ответ.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
)

// catalogLockKey — фиксированный ключ pg_advisory_xact_lock каталога
// (спека iterA §9.1): одна мутация каталога за раз; закрывает TOCTOU
// между «проверил ссылки» и «очистил» при DELETE.
const catalogLockKey = 0x474F4F4453 // "GOODS"

// ErrCatalog — ошибка каталога с HTTP-статусом (400/403/404/409).
type ErrCatalog struct {
	Status int
	Msg    string
}

func (e *ErrCatalog) Error() string { return e.Msg }

func errCatalog(status int, format string, args ...interface{}) error {
	return &ErrCatalog{Status: status, Msg: fmt.Sprintf(format, args...)}
}

// isUniqueViolation — ошибка PostgreSQL UNIQUE (код 23505): страховка от
// гонки двух вставок (спека §9.1) — маппится в 409, а не 500.
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// CategoryRow — категория каталога из БД (расширение model.Category:
// kind/is_system/code — только для БД-представления, спека iterA §7).
type CategoryRow struct {
	ID       int64
	Name     string
	Kind     string
	Code     sql.NullString
	IsSystem bool
}

// CatalogSnapshot — согласованный снимок каталога (одна транзакция
// REPEATABLE READ, §8.1): категории + товары со слотами.
type CatalogSnapshot struct {
	Categories []CategoryRow
	Goods      []model.Good
}

// ResourceView — ресурс палитры (GET /studio/api/resources, спека §7).
type ResourceView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CategoryID int64  `json:"category_id"`
	Kind       string `json:"kind"`
}

// RealResourceRow — real-ресурс витрины 94a из БД (спека iterB §5.3):
// goods kind=resource с props ? 'family' + code категории (джойн).
type RealResourceRow struct {
	ID       int64
	Name     string
	Category string // code из categories.code
	Props    []byte // props JSONB (русские ключи осей)
}

// BulkCreated/BulkSkipped/BulkError/BulkReport — отчёт подгрузки списка
// (99a.3 §9.2, спека iterA §7): created/skipped/errors, 1-based номера строк.
type BulkCreated struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BulkSkipped struct {
	Line   int    `json:"line"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type BulkError struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

type BulkReport struct {
	Created []BulkCreated `json:"created"`
	Skipped []BulkSkipped `json:"skipped"`
	Errors  []BulkError   `json:"errors"`
}

// GoodsRepository — доступ к каталогу товаров/ресурсов.
type GoodsRepository struct {
	db *sql.DB
}

func NewGoodsRepository(db *sql.DB) *GoodsRepository {
	return &GoodsRepository{db: db}
}

// queryer — общий интерфейс чтения для *sql.DB и *sql.Tx.
type queryer interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// beginMutation — транзакция мутации каталога с глобальным advisory lock
// (спека iterA §9.1): одна мутация за раз; TOCTOU закрыт.
func (r *GoodsRepository) beginMutation() (*sql.Tx, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, catalogLockKey); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// --- снимок ---

// Snapshot — полный снимок каталога в одной транзакции чтения
// REPEATABLE READ (согласованные пары goods/slots, §8.1).
func (r *GoodsRepository) Snapshot() (*CatalogSnapshot, error) {
	tx, err := r.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	cats, err := loadCategories(tx)
	if err != nil {
		return nil, err
	}
	goods, err := loadGoods(tx)
	if err != nil {
		return nil, err
	}
	slots, err := loadSlots(tx)
	if err != nil {
		return nil, err
	}
	for i := range goods {
		goods[i].Recipe = slots[goods[i].ID]
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &CatalogSnapshot{Categories: cats, Goods: goods}, nil
}

// Resources — палитра: ресурсы kind=resource, не banned (спека §7).
func (r *GoodsRepository) Resources() ([]ResourceView, error) {
	rows, err := r.db.Query(
		`SELECT id, name, category_id FROM goods WHERE kind = 'resource' AND status <> 'banned' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ResourceView
	for rows.Next() {
		var v ResourceView
		if err := rows.Scan(&v.ID, &v.Name, &v.CategoryID); err != nil {
			return nil, err
		}
		v.Kind = "resource"
		out = append(out, v)
	}
	return out, rows.Err()
}

// RealResources — real-ресурсы витрины 94a (спека iterB §5.3): источник —
// БД (С1), не real.go. Признак real-ресурса — props ? 'family'
// (JSONB-оператор наличия ключа); категория — code из categories.code.
func (r *GoodsRepository) RealResources() ([]RealResourceRow, error) {
	rows, err := r.db.Query(
		`SELECT g.id, g.name, c.code, g.props
		 FROM goods g
		 JOIN categories c ON c.id = g.category_id
		 WHERE g.kind = 'resource' AND g.props ? 'family'
		 ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RealResourceRow
	for rows.Next() {
		var v RealResourceRow
		if err := rows.Scan(&v.ID, &v.Name, &v.Category, &v.Props); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- категории ---

// CreateCategory — создание товарной категории (спека §7): дубликат
// нормализованного имени — 409.
func (r *GoodsRepository) CreateCategory(name string) (CategoryRow, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return CategoryRow{}, err
	}
	defer tx.Rollback()

	name = strings.TrimSpace(name)
	if name == "" {
		return CategoryRow{}, errCatalog(400, "имя пустое")
	}
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM categories WHERE kind = 'good' AND name_norm = $1)`,
		graph.NormalizeName(name),
	).Scan(&exists); err != nil {
		return CategoryRow{}, err
	}
	if exists {
		return CategoryRow{}, errCatalog(409, "категория с таким именем уже есть")
	}

	var c CategoryRow
	if err := tx.QueryRow(
		`INSERT INTO categories (name, name_norm, kind) VALUES ($1, $2, 'good') RETURNING id, name, kind, code, is_system`,
		name, graph.NormalizeName(name),
	).Scan(&c.ID, &c.Name, &c.Kind, &c.Code, &c.IsSystem); err != nil {
		if isUniqueViolation(err) {
			return CategoryRow{}, errCatalog(409, "категория с таким именем уже есть")
		}
		return CategoryRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return CategoryRow{}, err
	}
	return c, nil
}

// RenameCategory — переименование категории (спека §7): системная
// ресурсная — 403; дубликат нормализованного имени — 409.
func (r *GoodsRepository) RenameCategory(id int64, name string) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	name = strings.TrimSpace(name)
	if name == "" {
		return errCatalog(400, "имя пустое")
	}
	var kind string
	var isSystem bool
	err = tx.QueryRow(`SELECT kind, is_system FROM categories WHERE id = $1 FOR UPDATE`, id).
		Scan(&kind, &isSystem)
	if errors.Is(err, sql.ErrNoRows) {
		return errCatalog(404, "категория не найдена")
	}
	if err != nil {
		return err
	}
	if isSystem {
		return errCatalog(403, "системная категория — переименование запрещено")
	}
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM categories WHERE kind = $1 AND name_norm = $2 AND id <> $3)`,
		kind, graph.NormalizeName(name), id,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errCatalog(409, "категория с таким именем уже есть")
	}
	if _, err := tx.Exec(`UPDATE categories SET name = $1, name_norm = $2 WHERE id = $3`, name, graph.NormalizeName(name), id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteCategory — удаление категории (спека §7): системная ресурсная —
// 403; не пуста — 409.
func (r *GoodsRepository) DeleteCategory(id int64) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var isSystem bool
	err = tx.QueryRow(`SELECT is_system FROM categories WHERE id = $1 FOR UPDATE`, id).Scan(&isSystem)
	if errors.Is(err, sql.ErrNoRows) {
		return errCatalog(404, "категория не найдена")
	}
	if err != nil {
		return err
	}
	if isSystem {
		return errCatalog(403, "системная категория — удаление запрещено")
	}
	var hasGoods bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM goods WHERE category_id = $1)`, id,
	).Scan(&hasGoods); err != nil {
		return err
	}
	if hasGoods {
		return errCatalog(409, "категория не пуста — сначала перекатегоризуйте товары")
	}
	if _, err := tx.Exec(`DELETE FROM categories WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- товары ---

// CreateGood — создание товара/ресурса (спека §7): kind=good (дефолт) —
// черновик с одним пустым слотом (quantity=1); kind=resource — без слотов,
// категория только ресурсная (иначе 400); дубликат имени — 409.
func (r *GoodsRepository) CreateGood(name string, categoryID int64, kind model.Kind) (model.Good, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return model.Good{}, err
	}
	defer tx.Rollback()

	name = strings.TrimSpace(name)
	if name == "" {
		return model.Good{}, errCatalog(400, "имя пустое")
	}
	if kind == "" {
		kind = model.KindGood
	}
	if kind != model.KindGood && kind != model.KindResource {
		return model.Good{}, errCatalog(400, "неизвестный kind")
	}
	var catKind string
	err = tx.QueryRow(`SELECT kind FROM categories WHERE id = $1`, categoryID).Scan(&catKind)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Good{}, errCatalog(400, "категория не найдена")
	}
	if err != nil {
		return model.Good{}, err
	}
	if catKind != string(kind) {
		return model.Good{}, errCatalog(400, "категория не соответствует kind товара")
	}
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM goods WHERE name_norm = $1)`, graph.NormalizeName(name),
	).Scan(&exists); err != nil {
		return model.Good{}, err
	}
	if exists {
		return model.Good{}, errCatalog(409, "товар с таким именем уже есть")
	}

	var id int64
	var createdAt time.Time
	if err := tx.QueryRow(
		`INSERT INTO goods (name, name_norm, category_id, kind, status, source) VALUES ($1, $2, $3, $4, 'draft', 'manual') RETURNING id, created_at`,
		name, graph.NormalizeName(name), categoryID, string(kind),
	).Scan(&id, &createdAt); err != nil {
		if isUniqueViolation(err) {
			return model.Good{}, errCatalog(409, "товар с таким именем уже есть")
		}
		return model.Good{}, err
	}
	if kind == model.KindGood {
		if _, err := tx.Exec(
			`INSERT INTO goods_slots (good_id, pos, quantity) VALUES ($1, 0, 1)`, id,
		); err != nil {
			return model.Good{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Good{}, err
	}

	g := model.Good{
		ID:        strconv.FormatInt(id, 10),
		Name:      name,
		Category:  strconv.FormatInt(categoryID, 10),
		Status:    model.StatusDraft,
		Kind:      kind,
		Source:    model.SourceManual,
		CreatedAt: createdAt.UTC().Format(time.RFC3339),
	}
	if kind == model.KindGood {
		g.Recipe = []model.Slot{{Quantity: 1}}
	}
	return g, nil
}

// UpdateGood — переименование/смена категории (спека §7): категория должна
// соответствовать kind — 400; дубликат имени — 409. Для ресурсов разрешено
// (С1: без привилегий).
func (r *GoodsRepository) UpdateGood(id int64, name *string, categoryID *int64) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var kind string
	err = tx.QueryRow(`SELECT kind FROM goods WHERE id = $1 FOR UPDATE`, id).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return errCatalog(404, "товар не найден")
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
			`SELECT EXISTS(SELECT 1 FROM goods WHERE name_norm = $1 AND id <> $2)`,
			graph.NormalizeName(trimmed), id,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return errCatalog(409, "товар с таким именем уже есть")
		}
		if _, err := tx.Exec(`UPDATE goods SET name = $1, name_norm = $2 WHERE id = $3`, trimmed, graph.NormalizeName(trimmed), id); err != nil {
			return err
		}
	}
	if categoryID != nil {
		var catKind string
		err := tx.QueryRow(`SELECT kind FROM categories WHERE id = $1`, *categoryID).Scan(&catKind)
		if errors.Is(err, sql.ErrNoRows) {
			return errCatalog(400, "категория не найдена")
		}
		if err != nil {
			return err
		}
		if catKind != kind {
			return errCatalog(400, "категория не соответствует kind товара")
		}
		if _, err := tx.Exec(`UPDATE goods SET category_id = $1 WHERE id = $2`, *categoryID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteGood — удаление товара/ресурса (спека §7, решение гейта №2):
// всегда (в т.ч. ресурсы — без привилегий); слоты других товаров,
// ссылающиеся на него, очищаются (component_id → NULL, reason → '') в той
// же транзакции; свои слоты удаляются каскадом. Возвращает число
// очищенных ссылок (cleared_links, для UI-подтверждения в B).
func (r *GoodsRepository) DeleteGood(id int64) (int, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM goods WHERE id = $1 FOR UPDATE)`, id).Scan(&exists)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, errCatalog(404, "товар не найден")
	}
	res, err := tx.Exec(
		`UPDATE goods_slots SET component_id = NULL, reason = '' WHERE component_id = $1`, id,
	)
	if err != nil {
		return 0, err
	}
	cleared, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM goods WHERE id = $1`, id); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(cleared), nil
}

// SetStatus — смена статуса (спека §7): draft/approved/excluded/banned/unban;
// banned → banned_at=NOW(), unban → draft + banned_at=NULL. Для ресурсов
// разрешено (без привилегий).
func (r *GoodsRepository) SetStatus(id int64, status string) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM goods WHERE id = $1 FOR UPDATE)`, id,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "товар не найден")
	}
	switch status {
	case "banned":
		_, err = tx.Exec(`UPDATE goods SET status = 'banned', banned_at = NOW() WHERE id = $1`, id)
	case "unban":
		_, err = tx.Exec(`UPDATE goods SET status = 'draft', banned_at = NULL WHERE id = $1`, id)
	case "draft", "approved", "excluded":
		_, err = tx.Exec(`UPDATE goods SET status = $1, banned_at = NULL WHERE id = $2`, status, id)
	default:
		return errCatalog(400, "неизвестный статус")
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SetTier — тир-оверрайд (спека §7): ≥ 0, иначе 400; null — очистить.
// Для ресурсов разрешено (С3: назначенный тир — любой).
func (r *GoodsRepository) SetTier(id int64, tier *int) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM goods WHERE id = $1 FOR UPDATE)`, id,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "товар не найден")
	}
	if tier != nil && *tier < 0 {
		return errCatalog(400, "тир не может быть отрицательным")
	}
	if tier != nil {
		_, err = tx.Exec(`UPDATE goods SET tier_override = $1 WHERE id = $2`, *tier, id)
	} else {
		_, err = tx.Exec(`UPDATE goods SET tier_override = NULL WHERE id = $1`, id)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// --- слоты ---

// AddSlot — добавить пустой слот (quantity=1, спека §7); ресурсу — 403
// (решение №1: у kind=resource слотов нет).
func (r *GoodsRepository) AddSlot(goodID int64) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := checkGoodForSlots(tx, goodID); err != nil {
		return err
	}
	var maxPos sql.NullInt64
	if err := tx.QueryRow(
		`SELECT MAX(pos) FROM goods_slots WHERE good_id = $1`, goodID,
	).Scan(&maxPos); err != nil {
		return err
	}
	next := 0
	if maxPos.Valid {
		next = int(maxPos.Int64) + 1
	}
	if _, err := tx.Exec(
		`INSERT INTO goods_slots (good_id, pos, quantity) VALUES ($1, $2, 1)`, goodID, next,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// PutSlot — положить составляющую / изменить количество (спека §7):
// цикл — 409 (проверка достижимости на снимке графа из той же транзакции,
// §9.2); quantity ≥ 1 (иначе 1); good_id пустой — только количество.
func (r *GoodsRepository) PutSlot(goodID int64, pos int, componentID int64, quantity *int) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := checkGoodForSlots(tx, goodID); err != nil {
		return err
	}
	var slotExists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM goods_slots WHERE good_id = $1 AND pos = $2)`, goodID, pos,
	).Scan(&slotExists); err != nil {
		return err
	}
	if !slotExists {
		return errCatalog(404, "слот не найден")
	}
	if componentID != 0 {
		var compExists bool
		if err := tx.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM goods WHERE id = $1)`, componentID,
		).Scan(&compExists); err != nil {
			return err
		}
		if !compExists {
			return errCatalog(404, "составляющая не найдена")
		}
		// цикл-проверка внутри транзакции (§9.2): между проверкой и UPDATE
		// никто не вставит ребро (advisory lock + FOR UPDATE на родителе).
		goods, err := loadGoods(tx)
		if err != nil {
			return err
		}
		slots, err := loadSlots(tx)
		if err != nil {
			return err
		}
		for i := range goods {
			goods[i].Recipe = slots[goods[i].ID]
		}
		byID := graph.ByID(goods)
		if graph.WouldCreateCycle(strconv.FormatInt(componentID, 10), strconv.FormatInt(goodID, 10), byID) {
			return errCatalog(409, "цикл: товар не может быть составляющей самого себя (рёбра только вверх)")
		}
		if _, err := tx.Exec(
			`UPDATE goods_slots SET component_id = $1, reason = '' WHERE good_id = $2 AND pos = $3`,
			componentID, goodID, pos,
		); err != nil {
			return err
		}
	}
	if quantity != nil {
		q := *quantity
		if q < 1 {
			q = 1
		}
		if _, err := tx.Exec(
			`UPDATE goods_slots SET quantity = $1 WHERE good_id = $2 AND pos = $3`, q, goodID, pos,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteSlot — удалить слот (сдвиг pos в той же транзакции, спека §7).
func (r *GoodsRepository) DeleteSlot(goodID int64, pos int) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := checkGoodForSlots(tx, goodID); err != nil {
		return err
	}
	var slotExists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM goods_slots WHERE good_id = $1 AND pos = $2)`, goodID, pos,
	).Scan(&slotExists); err != nil {
		return err
	}
	if !slotExists {
		return errCatalog(404, "слот не найден")
	}
	if _, err := tx.Exec(
		`DELETE FROM goods_slots WHERE good_id = $1 AND pos = $2`, goodID, pos,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE goods_slots SET pos = pos - 1 WHERE good_id = $1 AND pos > $2`, goodID, pos,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearSlot — очистить слот (товар остаётся в реестре, спека §7).
func (r *GoodsRepository) ClearSlot(goodID int64, pos int) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := checkGoodForSlots(tx, goodID); err != nil {
		return err
	}
	var slotExists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM goods_slots WHERE good_id = $1 AND pos = $2)`, goodID, pos,
	).Scan(&slotExists); err != nil {
		return err
	}
	if !slotExists {
		return errCatalog(404, "слот не найден")
	}
	if _, err := tx.Exec(
		`UPDATE goods_slots SET component_id = NULL, reason = '' WHERE good_id = $1 AND pos = $2`,
		goodID, pos,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// SetSlotAllowResource — галка «заполнять ресурсом» (спека §7; влияет на
// ИИ, C).
func (r *GoodsRepository) SetSlotAllowResource(goodID int64, pos int, allow bool) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := checkGoodForSlots(tx, goodID); err != nil {
		return err
	}
	var slotExists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM goods_slots WHERE good_id = $1 AND pos = $2)`, goodID, pos,
	).Scan(&slotExists); err != nil {
		return err
	}
	if !slotExists {
		return errCatalog(404, "слот не найден")
	}
	if _, err := tx.Exec(
		`UPDATE goods_slots SET allow_resource = $1 WHERE good_id = $2 AND pos = $3`,
		allow, goodID, pos,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// --- подгрузка списка ---

// BulkCreateGoods — подгрузка списка «имя | категория» (99a.3 §9.2, спека
// iterA §7): частичный успех, отчёт created/skipped/errors; одна транзакция
// на пачку; категория — по нормализованному имени (kind=good).
func (r *GoodsRepository) BulkCreateGoods(lines []string) (BulkReport, error) {
	rep := BulkReport{Created: []BulkCreated{}, Skipped: []BulkSkipped{}, Errors: []BulkError{}}
	if len(lines) == 0 {
		return rep, errCatalog(400, "lines пуст")
	}
	tx, err := r.beginMutation()
	if err != nil {
		return rep, err
	}
	defer tx.Rollback()

	catByName := make(map[string]int64)
	rows, err := tx.Query(`SELECT id, name FROM categories WHERE kind = 'good'`)
	if err != nil {
		return rep, err
	}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return rep, err
		}
		catByName[graph.NormalizeName(name)] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return rep, err
	}

	existing := make(map[string]bool)
	rows, err = tx.Query(`SELECT name FROM goods`)
	if err != nil {
		return rep, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return rep, err
		}
		existing[graph.NormalizeName(name)] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return rep, err
	}

	for i, raw := range lines {
		line := i + 1 // физический номер строки (1-based); пустые не нумеруются
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		sep := strings.Index(trimmed, "|")
		if sep < 0 {
			rep.Errors = append(rep.Errors, BulkError{Line: line, Reason: "нет разделителя „|"})
			continue
		}
		name := strings.TrimSpace(trimmed[:sep])
		catName := strings.TrimSpace(trimmed[sep+1:])
		if name == "" {
			rep.Errors = append(rep.Errors, BulkError{Line: line, Reason: "пустое имя"})
			continue
		}
		if catName == "" {
			rep.Errors = append(rep.Errors, BulkError{Line: line, Reason: "пустая категория"})
			continue
		}
		catID, ok := catByName[graph.NormalizeName(catName)]
		if !ok {
			rep.Errors = append(rep.Errors, BulkError{Line: line, Reason: "категория не найдена: " + catName})
			continue
		}
		norm := graph.NormalizeName(name)
		if existing[norm] {
			rep.Skipped = append(rep.Skipped, BulkSkipped{Line: line, Name: name, Reason: "товар с таким именем уже есть"})
			continue
		}
		var id int64
		if err := tx.QueryRow(
			`INSERT INTO goods (name, name_norm, category_id, kind, status, source) VALUES ($1, $2, $3, 'good', 'draft', 'manual') RETURNING id`,
			name, graph.NormalizeName(name), catID,
		).Scan(&id); err != nil {
			if isUniqueViolation(err) {
				// страховка от гонки: дубликат в пачке/параллельная вставка — пропуск строки
				rep.Skipped = append(rep.Skipped, BulkSkipped{Line: line, Name: name, Reason: "товар с таким именем уже есть"})
				continue
			}
			return rep, err
		}
		if _, err := tx.Exec(
			`INSERT INTO goods_slots (good_id, pos, quantity) VALUES ($1, 0, 1)`, id,
		); err != nil {
			return rep, err
		}
		existing[norm] = true // дубликат в пачке — тоже пропуск
		rep.Created = append(rep.Created, BulkCreated{ID: strconv.FormatInt(id, 10), Name: name})
	}
	if err := tx.Commit(); err != nil {
		return rep, err
	}
	return rep, nil
}

// --- хелперы ---

// checkGoodForSlots — товар существует (404) и не ресурс (403, решение №1).
func checkGoodForSlots(tx *sql.Tx, goodID int64) error {
	var kind string
	err := tx.QueryRow(`SELECT kind FROM goods WHERE id = $1 FOR UPDATE`, goodID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return errCatalog(404, "товар не найден")
	}
	if err != nil {
		return err
	}
	if kind == "resource" {
		return errCatalog(403, "у ресурса слотов нет")
	}
	return nil
}

// loadCategories — все категории каталога.
func loadCategories(q queryer) ([]CategoryRow, error) {
	rows, err := q.Query(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CategoryRow
	for rows.Next() {
		var c CategoryRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &c.Code, &c.IsSystem); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// loadGoods — все товары каталога (id/категория — строки для graph).
func loadGoods(q queryer) ([]model.Good, error) {
	rows, err := q.Query(
		`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Good
	for rows.Next() {
		var g model.Good
		var id, catID int64
		var kind, status, source string
		var tier sql.NullInt64
		var bannedAt sql.NullTime
		var createdAt time.Time
		if err := rows.Scan(&id, &g.Name, &catID, &kind, &status, &source, &tier, &bannedAt, &createdAt); err != nil {
			return nil, err
		}
		g.ID = strconv.FormatInt(id, 10)
		g.Category = strconv.FormatInt(catID, 10)
		g.Kind = model.Kind(kind)
		g.Status = model.Status(status)
		g.Source = model.Source(source)
		if tier.Valid {
			t := int(tier.Int64)
			g.TierOverride = &t
		}
		if bannedAt.Valid {
			s := bannedAt.Time.UTC().Format(time.RFC3339)
			g.BannedAt = &s
		}
		g.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		out = append(out, g)
	}
	return out, rows.Err()
}

// loadSlots — все слоты каталога, сгруппированные по good_id (порядок по pos).
func loadSlots(q queryer) (map[string][]model.Slot, error) {
	rows, err := q.Query(
		`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slots := make(map[string][]model.Slot)
	for rows.Next() {
		var goodID, compID sql.NullInt64
		var pos, quantity int
		var reason sql.NullString
		var allow bool
		if err := rows.Scan(&goodID, &pos, &compID, &quantity, &reason, &allow); err != nil {
			return nil, err
		}
		s := model.Slot{Quantity: quantity, AllowResource: allow}
		if compID.Valid {
			s.GoodID = strconv.FormatInt(compID.Int64, 10)
		}
		if reason.Valid {
			s.Reason = reason.String
		}
		key := strconv.FormatInt(goodID.Int64, 10)
		slots[key] = append(slots[key], s)
	}
	return slots, rows.Err()
}