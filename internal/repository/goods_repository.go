// internal/repository/goods_repository.go
// SQL-доступ к каталогу товаров/ресурсов студии (спека
// переноса-студии-товаров-iterA §7; рецепт как сущность — спека
// 2026-09-21-рецепт-сущность §2.3/§5): categories/goods/recipes/
// recipe_components/producer_recipes.
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
	"unicode/utf8"

	"github.com/lib/pq"

	"zorion/internal/goodsstudio/ai"
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
// REPEATABLE READ, §8.1): категории + товары с составом рецептов + типы
// производителей + предметы + связи + слоты родителя + привязки рецептов
// (спека 2026-09-20-фабрики §3.1 + спека скрытых §1.1 + спека
// 2026-09-21-рецепт-сущность §5).
type CatalogSnapshot struct {
	Categories    []CategoryRow
	Goods         []model.Good
	ProducerTypes []ProducerTypeRow
	Items         []ItemRow
	ProducerItems []ProducerItemRow
	ProducerSlots []ProducerSlotRow
	Bindings      []RecipeBindingRow
}

// RecipeBindingRow — привязка рецепта к конкретной фабрике (producer_recipes,
// спека 2026-09-21-рецепт-сущность §2.3): good_id рецепта — джойном, для
// проекции в State.Bindings и верхнеуровневого producer_recipes в state.
type RecipeBindingRow struct {
	ProducerTypeID int64
	RecipeID       int64
	GoodID         int64
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

// LayerResourceRow — layer-ресурс «Базового слоя» из БД (спека iterC §7.2):
// goods kind=resource с props ? 'closes' + code категории (джойн).
type LayerResourceRow struct {
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
	QueryRow(query string, args ...interface{}) *sql.Row
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
	producerTypes, err := loadProducerTypes(tx)
	if err != nil {
		return nil, err
	}
	items, err := loadItems(tx)
	if err != nil {
		return nil, err
	}
	producerItems, err := loadProducerItems(tx)
	if err != nil {
		return nil, err
	}
	producerSlots, err := loadProducerSlots(tx)
	if err != nil {
		return nil, err
	}
	bindings, err := loadBindings(tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &CatalogSnapshot{
		Categories:    cats,
		Goods:         goods,
		ProducerTypes: producerTypes,
		Items:         items,
		ProducerItems: producerItems,
		ProducerSlots: producerSlots,
		Bindings:      bindings,
	}, nil
}

// Resources — палитра: ресурсы kind=resource (скрытия у ресурсов нет — видны
// всегда, спека 2026-09-21 §4.1).
func (r *GoodsRepository) Resources() ([]ResourceView, error) {
	rows, err := r.db.Query(
		`SELECT id, name, category_id FROM goods WHERE kind = 'resource' ORDER BY id`)
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

// LayerResources — layer-ресурсы «Базового слоя» (спека iterC §7.2): источник —
// БД (С1), не LayerCatalog. Дискриминатор — props ? 'closes' (есть у всех 20
// layer-ресурсов сида, нет у real и пользовательских — props NULL).
func (r *GoodsRepository) LayerResources() ([]LayerResourceRow, error) {
	rows, err := r.db.Query(
		`SELECT g.id, g.name, c.code, g.props
		 FROM goods g
		 JOIN categories c ON c.id = g.category_id
		 WHERE g.kind = 'resource' AND g.props ? 'closes'
		 ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LayerResourceRow
	for rows.Next() {
		var v LayerResourceRow
		if err := rows.Scan(&v.ID, &v.Name, &v.Category, &v.Props); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ApplyProposals — применение принятых предложений ИИ (спека iterC §5.4):
// одна транзакция с advisory lock (сериализация с другими мутациями каталога).
// Снимок каталога собирается в tx (свежий — между фазами никто не вмешается
// под lock), ai.ApplyProposals решает на нём (per-slot, дропы С1–С3 в отчёт),
// write-back: новые товары INSERT (source=ai, категория из попапа —
// всегда валидна), слоты UPDATE по конкретным pos (component_id из
// мутированного state: "gN" → realID по карте созданных, иначе числовой id).
// Возвращает applied (число фактически записанных пунктов, дропы не входят,
// М3) + отчёт. Товар удалён между фазами — отчёт «товар не найден», не ошибка
// (М2: ответ 200).
func (r *GoodsRepository) ApplyProposals(goodID int64, items []ai.ProposalItem) (int, []string, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()

	cats, err := loadCategories(tx)
	if err != nil {
		return 0, nil, err
	}
	goods, err := loadGoods(tx)
	if err != nil {
		return 0, nil, err
	}
	slots, err := loadSlots(tx)
	if err != nil {
		return 0, nil, err
	}
	for i := range goods {
		goods[i].Recipe = slots[goods[i].ID]
	}
	st := &model.State{
		SchemaVersion: model.SchemaVersion,
		Categories:    make([]model.Category, 0, len(cats)),
		Goods:         goods,
	}
	for _, c := range cats {
		st.Categories = append(st.Categories, model.Category{
			ID: strconv.FormatInt(c.ID, 10), Name: c.Name, Kind: model.Kind(c.Kind),
		})
	}
	origLen := len(st.Goods)

	targetID := strconv.FormatInt(goodID, 10)
	gi := indexOfGood(st.Goods, targetID)
	if gi < 0 {
		return 0, []string{"товар не найден"}, nil // М2: 200 + отчёт, не 404
	}
	// слоты целевого товара до применения — для diff write-back (заполнены
	// только те, что были пусты и приняты; C3-дропы не трогают слот)
	before := make([]string, len(st.Goods[gi].Recipe))
	for i, s := range st.Goods[gi].Recipe {
		before[i] = s.GoodID
	}

	applied, report := ai.ApplyProposals(st, targetID, items)

	// write-back 1: новые товары (в конец st.Goods) → INSERT, карта "gN"→realID
	gNToReal := make(map[string]int64, len(st.Goods)-origLen)
	failedG := make(map[string]bool)
	for i := origLen; i < len(st.Goods); i++ {
		g := st.Goods[i]
		catID, err := strconv.ParseInt(g.Category, 10, 64)
		if err != nil {
			return 0, nil, fmt.Errorf("apply: категория нового товара %s: %w", g.Name, err)
		}
		var id int64
		err = tx.QueryRow(
			`INSERT INTO goods (name, name_norm, category_id, kind, source, description) VALUES ($1, $2, $3, 'good', 'ai', $4) RETURNING id`,
			g.Name, graph.NormalizeName(g.Name), catID, nullIfEmpty(g.Description),
		).Scan(&id)
		if err != nil {
			if isUniqueViolation(err) {
				// страховка от гонки (как BulkCreateGoods): дубликат имени — дроп
				// пункта + отчёт; слот не заполняется (failedG)
				applied--
				report = append(report, fmt.Sprintf("товар с таким именем уже есть: %s — пропущено", g.Name))
				failedG[g.ID] = true
				continue
			}
			return 0, nil, err
		}
		gNToReal[g.ID] = id
		// рецепт для ИИ-товара (спека 2026-09-21-рецепт-сущность §8.3):
		// создаётся обязательно (иначе recipe_id в state пуст). Состав не
		// создаётся — не-регресс: сегодня ApplyProposals товары без слотов.
		if _, err := tx.Exec(`INSERT INTO recipes (good_id) VALUES ($1)`, id); err != nil {
			return 0, nil, err
		}
	}

	// write-back 2: слоты целевого товара (diff: был пуст → заполнен) — по
	// (recipe_id, pos) рецепта цели (спека 2026-09-21-рецепт-сущность §5).
	targetRecipeID := st.Goods[gi].RecipeID
	for _, item := range items {
		if item.Slot < 0 || item.Slot >= len(st.Goods[gi].Recipe) {
			continue
		}
		if before[item.Slot] != "" {
			continue // слот был заполнен до применения — не наш (C3-дроп)
		}
		compID := st.Goods[gi].Recipe[item.Slot].GoodID
		if compID == "" {
			continue // не применён (дроп)
		}
		var realID int64
		if real, ok := gNToReal[compID]; ok {
			realID = real // создан в этом прогоне (М4: маппинг по id слота)
		} else if failedG[compID] {
			continue // INSERT упал (23505) — слот не заполняем
		} else {
			realID, err = strconv.ParseInt(compID, 10, 64)
			if err != nil {
				return 0, nil, fmt.Errorf("apply: component_id %s: %w", compID, err)
			}
		}
		if _, err := tx.Exec(
			`UPDATE recipe_components SET component_id = $1, reason = $2 WHERE recipe_id = $3 AND pos = $4`,
			realID, st.Goods[gi].Recipe[item.Slot].Reason, targetRecipeID, item.Slot,
		); err != nil {
			return 0, nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return applied, report, nil
}

// DescItem — принятый пункт попапа «Описания ИИ»: id записи каталога +
// нормализованный текст (trim, непустой, ≤ MaxDescriptionRunes).
type DescItem struct {
	ID   int64
	Text string
}

// UpdateDescriptions — запись описаний (спека §6.6): одна транзакция с
// advisory lock (beginMutation). onlyIfEmpty — режим джоба (И4):
//
//	true  — пакетный прогон (scope:"missing"): на каждый пункт SELECT name,
//	        COALESCE(description, '') FROM goods WHERE id = $1 FOR UPDATE —
//	        записи нет → «запись N не найдена — пропущено»; описание непустое →
//	        «описание уже заполнено: <имя> — пропущено»; иначе UPDATE;
//	false — явный прогон (good_ids: одиночный после создания / по кнопке):
//	        guard непустого описания снят — SELECT (наличие записи) и UPDATE
//	        поверх («запись N не найдена — пропущено» только если записи нет).
//
// Возвращает applied (фактически записанные) + отчёт по строкам.
func (r *GoodsRepository) UpdateDescriptions(items []DescItem, onlyIfEmpty bool) (int, []string, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()

	applied := 0
	var report []string
	for _, it := range items {
		var name, current string
		err := tx.QueryRow(
			`SELECT name, COALESCE(description, '') FROM goods WHERE id = $1 FOR UPDATE`, it.ID,
		).Scan(&name, &current)
		if errors.Is(err, sql.ErrNoRows) {
			report = append(report, fmt.Sprintf("запись %d не найдена — пропущено", it.ID))
			continue
		}
		if err != nil {
			return 0, nil, err
		}
		if onlyIfEmpty && strings.TrimSpace(current) != "" {
			report = append(report, fmt.Sprintf("описание уже заполнено: %s — пропущено", name))
			continue
		}
		if _, err := tx.Exec(`UPDATE goods SET description = $1 WHERE id = $2`, it.Text, it.ID); err != nil {
			return 0, nil, err
		}
		applied++
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return applied, report, nil
}

// nullIfEmpty — пустая строка → nil (в БД «нет описания» = NULL, И3).
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// indexOfGood — индекс товара по id (для ApplyProposals-транзакции).
func indexOfGood(goods []model.Good, id string) int {
	for i := range goods {
		if goods[i].ID == id {
			return i
		}
	}
	return -1
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

	var id, recipeID int64
	var createdAt time.Time
	if err := tx.QueryRow(
		`INSERT INTO goods (name, name_norm, category_id, kind, source) VALUES ($1, $2, $3, $4, 'manual') RETURNING id, created_at`,
		name, graph.NormalizeName(name), categoryID, string(kind),
	).Scan(&id, &createdAt); err != nil {
		if isUniqueViolation(err) {
			return model.Good{}, errCatalog(409, "товар с таким именем уже есть")
		}
		return model.Good{}, err
	}
	if kind == model.KindGood {
		// рецепт создаётся вместе с товаром (спека 2026-09-21-рецепт-сущность
		// §2.3/§11.3): пустой состав = один пустой компонент pos=0, quantity=1
		// (как раньше goods_slots (good_id,0,1)).
		if err := tx.QueryRow(
			`INSERT INTO recipes (good_id) VALUES ($1) RETURNING id`, id,
		).Scan(&recipeID); err != nil {
			return model.Good{}, err
		}
		if _, err := tx.Exec(
			`INSERT INTO recipe_components (recipe_id, pos, quantity) VALUES ($1, 0, 1)`, recipeID,
		); err != nil {
			return model.Good{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Good{}, err
	}

	// volume/weight — NOT NULL DEFAULT 1 (Р2, 2026-09-21): новые строки
	// получают 1 из DEFAULT; возвращаем то же значение в модель.
	one := 1.0
	g := model.Good{
		ID:        strconv.FormatInt(id, 10),
		Name:      name,
		Category:  strconv.FormatInt(categoryID, 10),
		Kind:      kind,
		Source:    model.SourceManual,
		Volume:    &one,
		Weight:    &one,
		CreatedAt: createdAt.UTC().Format(time.RFC3339),
	}
	if kind == model.KindGood {
		g.RecipeID = recipeID
		g.Recipe = []model.Slot{{Quantity: 1}}
	}
	return g, nil
}

// UpdateGood — переименование/смена категории/веса/объёма/описания (спека §7):
// категория должна соответствовать kind — 400; дубликат имени — 409.
// volume/weight — данные каталога (3b.6.4): nil = не трогать (значение есть
// всегда, Р2 2026-09-21); отрицательные значения — 400. Для ресурсов
// разрешено (С1: без привилегий). description — данные каталога (спека
// 2026-09-21-каталог-описание §6.3): nil = не трогать, пусто/пробелы →
// NULL (очистка, И3), длиннее MaxDescriptionRunes рун — 400.
func (r *GoodsRepository) UpdateGood(id int64, name *string, categoryID *int64, volume, weight *float64, description *string) error {
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
	if volume != nil {
		if *volume < 0 {
			return errCatalog(400, "объём не может быть отрицательным")
		}
		if _, err := tx.Exec(`UPDATE goods SET volume = $1 WHERE id = $2`, *volume, id); err != nil {
			return err
		}
	}
	if weight != nil {
		if *weight < 0 {
			return errCatalog(400, "вес не может быть отрицательным")
		}
		if _, err := tx.Exec(`UPDATE goods SET weight = $1 WHERE id = $2`, *weight, id); err != nil {
			return err
		}
	}
	if description != nil {
		trimmed := strings.TrimSpace(*description)
		if utf8.RuneCountInString(trimmed) > ai.MaxDescriptionRunes {
			return errCatalog(400, "описание длиннее 2000 символов")
		}
		if trimmed == "" {
			if _, err := tx.Exec(`UPDATE goods SET description = NULL WHERE id = $1`, id); err != nil {
				return err
			}
		} else if _, err := tx.Exec(`UPDATE goods SET description = $1 WHERE id = $2`, trimmed, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GoodDeleteResult — итог удаления товара/ресурса: ClearedLinks — число
// очищенных ссылок (component_id → NULL в чужих рецептах, для UI-
// подтверждения), Deposits — число залежей ресурса, снесённых каскадом
// (спека 2026-09-22-поселение-... §3.3/T14: студия предупреждает числом),
// Branches — число веток поселений, у которых этот товар — выход рецепта
// (каскад recipes → settlement_branches; §8/О3 итерации 2).
type GoodDeleteResult struct {
	ClearedLinks int
	Deposits     int
	Branches     int
}

// DeleteGood — удаление товара/ресурса (спека §7, решение гейта №2):
// всегда (в т.ч. ресурсы — без привилегий); компоненты рецептов других
// товаров, ссылающиеся на него, очищаются (component_id → NULL, reason → ”)
// в той же транзакции; свой рецепт и его компоненты удаляются каскадом
// (recipes → recipe_components, producer_recipes). Возвращает число
// очищенных ссылок (cleared_links) и число залежей ресурса (deposits,
// §3.3/T14 — предпроверка до DELETE, число берётся по общему SQL
// countDepositsByGood; FK deposits.good_id ON DELETE CASCADE).
func (r *GoodsRepository) DeleteGood(id int64) (GoodDeleteResult, error) {
	var out GoodDeleteResult
	tx, err := r.beginMutation()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()

	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM goods WHERE id = $1 FOR UPDATE)`, id).Scan(&exists)
	if err != nil {
		return out, err
	}
	if !exists {
		return out, errCatalog(404, "товар не найден")
	}
	// Предпроверка залежей до удаления (§3.3/T14): FK CASCADE сносит их молча.
	out.Deposits, err = countDepositsByGood(tx, id)
	if err != nil {
		return out, err
	}
	// Предпроверка веток до удаления (§8/О3 итерации 2): товар-выход сносит
	// рецепт → ветки поселений с буферами каскадом; студия предупреждает числом.
	out.Branches, err = countBranchesByGood(tx, id)
	if err != nil {
		return out, err
	}
	res, err := tx.Exec(
		`UPDATE recipe_components SET component_id = NULL, reason = '' WHERE component_id = $1`, id,
	)
	if err != nil {
		return out, err
	}
	cleared, err := res.RowsAffected()
	if err != nil {
		return out, err
	}
	out.ClearedLinks = int(cleared)
	if _, err := tx.Exec(`DELETE FROM goods WHERE id = $1`, id); err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	return out, nil
}

// CreateRecipe — восстановление рецепта товара (POST /studio/api/recipes,
// спека 2026-09-21-рецепт-сущность §5): товар kind='good' (400 иначе),
// дубликат рецепта на товар — 409; новый рецепт получает один пустой
// компонент (pos=0, quantity=1 — как при создании товара). Возвращает id
// рецепта.
func (r *GoodsRepository) CreateRecipe(goodID int64, complexity *int) (int64, error) {
	tx, err := r.beginMutation()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var kind string
	err = tx.QueryRow(`SELECT kind FROM goods WHERE id = $1 FOR UPDATE`, goodID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errCatalog(404, "товар не найден")
	}
	if err != nil {
		return 0, err
	}
	if kind != "good" {
		return 0, errCatalog(400, "рецепт — только у товара (kind=good)")
	}
	if complexity != nil && *complexity < 0 {
		return 0, errCatalog(400, "сложность не может быть отрицательной")
	}
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM recipes WHERE good_id = $1)`, goodID).Scan(&exists); err != nil {
		return 0, err
	}
	if exists {
		return 0, errCatalog(409, "у товара уже есть рецепт")
	}
	var recipeID int64
	if err := tx.QueryRow(
		`INSERT INTO recipes (good_id, complexity) VALUES ($1, $2) RETURNING id`, goodID, complexity,
	).Scan(&recipeID); err != nil {
		if isUniqueViolation(err) {
			return 0, errCatalog(409, "у товара уже есть рецепт")
		}
		return 0, err
	}
	if _, err := tx.Exec(
		`INSERT INTO recipe_components (recipe_id, pos, quantity) VALUES ($1, 0, 1)`, recipeID,
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return recipeID, nil
}

// SetRecipeComplexity — сложность рецепта (спека 2026-09-21-рецепт-сущность
// §5, PUT /studio/api/recipes/{id}): ≥ 0, иначе 400; null — очистить
// (сложность вычисляется по графу). Рецепта нет — 404.
func (r *GoodsRepository) SetRecipeComplexity(recipeID int64, complexity *int) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM recipes WHERE id = $1 FOR UPDATE)`, recipeID,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "рецепт не найден")
	}
	if complexity != nil && *complexity < 0 {
		return errCatalog(400, "сложность не может быть отрицательной")
	}
	if complexity != nil {
		_, err = tx.Exec(`UPDATE recipes SET complexity = $1 WHERE id = $2`, *complexity, recipeID)
	} else {
		_, err = tx.Exec(`UPDATE recipes SET complexity = NULL WHERE id = $1`, recipeID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// --- состав рецепта (спека 2026-09-21-рецепт-сущность §5) ---

// checkRecipe — рецепт существует (404); возвращает good_id рецепта (выход).
func checkRecipe(tx *sql.Tx, recipeID int64) (int64, error) {
	var goodID int64
	err := tx.QueryRow(`SELECT good_id FROM recipes WHERE id = $1 FOR UPDATE`, recipeID).Scan(&goodID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errCatalog(404, "рецепт не найден")
	}
	if err != nil {
		return 0, err
	}
	return goodID, nil
}

// AddRecipeComponent — добавить пустой компонент (pos = max+1, quantity=1).
func (r *GoodsRepository) AddRecipeComponent(recipeID int64) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := checkRecipe(tx, recipeID); err != nil {
		return err
	}
	var maxPos sql.NullInt64
	if err := tx.QueryRow(
		`SELECT MAX(pos) FROM recipe_components WHERE recipe_id = $1`, recipeID,
	).Scan(&maxPos); err != nil {
		return err
	}
	next := 0
	if maxPos.Valid {
		next = int(maxPos.Int64) + 1
	}
	if _, err := tx.Exec(
		`INSERT INTO recipe_components (recipe_id, pos, quantity) VALUES ($1, $2, 1)`, recipeID, next,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// PutRecipeComponent — поставить составляющую / изменить количество (спека
// §5): цикл — 409 (проверка достижимости на снимке графа из той же
// транзакции); quantity ≥ 1 (иначе 1); componentID=0 — только количество.
func (r *GoodsRepository) PutRecipeComponent(recipeID int64, pos int, componentID int64, quantity *int) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	targetGoodID, err := checkRecipe(tx, recipeID)
	if err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM recipe_components WHERE recipe_id = $1 AND pos = $2)`, recipeID, pos,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "компонент не найден")
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
		// цикл-проверка внутри транзакции: между проверкой и UPDATE никто не
		// вставит ребро (advisory lock + FOR UPDATE на рецепте).
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
		if graph.WouldCreateCycle(strconv.FormatInt(componentID, 10), strconv.FormatInt(targetGoodID, 10), byID) {
			return errCatalog(409, "цикл: товар не может быть составляющей самого себя (рёбра только вверх)")
		}
		if _, err := tx.Exec(
			`UPDATE recipe_components SET component_id = $1, reason = '' WHERE recipe_id = $2 AND pos = $3`,
			componentID, recipeID, pos,
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
			`UPDATE recipe_components SET quantity = $1 WHERE recipe_id = $2 AND pos = $3`, q, recipeID, pos,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteRecipeComponent — удалить компонент (сдвиг pos в той же транзакции).
func (r *GoodsRepository) DeleteRecipeComponent(recipeID int64, pos int) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := checkRecipe(tx, recipeID); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM recipe_components WHERE recipe_id = $1 AND pos = $2)`, recipeID, pos,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "компонент не найден")
	}
	if _, err := tx.Exec(
		`DELETE FROM recipe_components WHERE recipe_id = $1 AND pos = $2`, recipeID, pos,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE recipe_components SET pos = pos - 1 WHERE recipe_id = $1 AND pos > $2`, recipeID, pos,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearRecipeComponent — очистить компонент (товар остаётся в реестре).
func (r *GoodsRepository) ClearRecipeComponent(recipeID int64, pos int) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := checkRecipe(tx, recipeID); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM recipe_components WHERE recipe_id = $1 AND pos = $2)`, recipeID, pos,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "компонент не найден")
	}
	if _, err := tx.Exec(
		`UPDATE recipe_components SET component_id = NULL, reason = '' WHERE recipe_id = $1 AND pos = $2`,
		recipeID, pos,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// SetRecipeComponentAllowResource — галка «заполнять ресурсом» (подсказка
// ИИ для пустого компонента, спека §2.5; поведение не меняется).
func (r *GoodsRepository) SetRecipeComponentAllowResource(recipeID int64, pos int, allow bool) error {
	tx, err := r.beginMutation()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := checkRecipe(tx, recipeID); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM recipe_components WHERE recipe_id = $1 AND pos = $2)`, recipeID, pos,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errCatalog(404, "компонент не найден")
	}
	if _, err := tx.Exec(
		`UPDATE recipe_components SET allow_resource = $1 WHERE recipe_id = $2 AND pos = $3`,
		allow, recipeID, pos,
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
		parts := strings.SplitN(trimmed, "|", 3)
		if len(parts) < 2 {
			rep.Errors = append(rep.Errors, BulkError{Line: line, Reason: "нет разделителя „|"})
			continue
		}
		name := strings.TrimSpace(parts[0])
		catName := strings.TrimSpace(parts[1])
		// третья колонка — описание (спека 2026-09-21-каталог-описание §6.4):
		// может быть пустой; «|» внутри описания допустим (не режется дальше).
		description := ""
		if len(parts) == 3 {
			description = strings.TrimSpace(parts[2])
		}
		if name == "" {
			rep.Errors = append(rep.Errors, BulkError{Line: line, Reason: "пустое имя"})
			continue
		}
		if catName == "" {
			rep.Errors = append(rep.Errors, BulkError{Line: line, Reason: "пустая категория"})
			continue
		}
		if utf8.RuneCountInString(description) > ai.MaxDescriptionRunes {
			rep.Errors = append(rep.Errors, BulkError{Line: line, Reason: "описание длиннее 2000 символов"})
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
			`INSERT INTO goods (name, name_norm, category_id, kind, source, description) VALUES ($1, $2, $3, 'good', 'manual', $4) RETURNING id`,
			name, graph.NormalizeName(name), catID, nullIfEmpty(description),
		).Scan(&id); err != nil {
			if isUniqueViolation(err) {
				// страховка от гонки: дубликат в пачке/параллельная вставка — пропуск строки
				rep.Skipped = append(rep.Skipped, BulkSkipped{Line: line, Name: name, Reason: "товар с таким именем уже есть"})
				continue
			}
			return rep, err
		}
		// рецепт создаётся вместе с товаром (спека 2026-09-21-рецепт-сущность
		// §2.3/§11.3): пустой состав = один пустой компонент pos=0, quantity=1.
		var recipeID int64
		if err := tx.QueryRow(
			`INSERT INTO recipes (good_id) VALUES ($1) RETURNING id`, id,
		).Scan(&recipeID); err != nil {
			return rep, err
		}
		if _, err := tx.Exec(
			`INSERT INTO recipe_components (recipe_id, pos, quantity) VALUES ($1, 0, 1)`, recipeID,
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

// loadGoods — все товары каталога (id/категория — строки для graph) с
// проекцией рецепта: id рецепта и сложность (LEFT JOIN recipes — у ресурса
// рецепта нет, recipe_id/complexity = NULL).
func loadGoods(q queryer) ([]model.Good, error) {
	rows, err := q.Query(
		`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description
		 FROM goods g LEFT JOIN recipes r ON r.good_id = g.id
		 ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Good
	for rows.Next() {
		var g model.Good
		var id, catID int64
		var kind, source string
		var recipeID, complexity sql.NullInt64
		var createdAt time.Time
		var volume, weight sql.NullFloat64
		var description sql.NullString
		if err := rows.Scan(&id, &g.Name, &catID, &kind, &source, &recipeID, &complexity, &createdAt, &volume, &weight, &description); err != nil {
			return nil, err
		}
		g.ID = strconv.FormatInt(id, 10)
		g.Category = strconv.FormatInt(catID, 10)
		g.Kind = model.Kind(kind)
		g.Source = model.Source(source)
		if recipeID.Valid {
			g.RecipeID = recipeID.Int64
		}
		if complexity.Valid {
			t := int(complexity.Int64)
			g.Complexity = &t
		}
		if volume.Valid {
			v := volume.Float64
			g.Volume = &v
		}
		if weight.Valid {
			w := weight.Float64
			g.Weight = &w
		}
		g.Description = description.String
		g.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		out = append(out, g)
	}
	return out, rows.Err()
}

// loadSlots — состав рецептов каталога (recipe_components), сгруппированный
// по good_id выхода (порядок по pos) — проекция Good.Recipe (спека
// 2026-09-21-рецепт-сущность §5: источник — recipe_components, не goods_slots).
func loadSlots(q queryer) (map[string][]model.Slot, error) {
	rows, err := q.Query(
		`SELECT r.good_id, c.pos, c.component_id, c.quantity, c.reason, c.allow_resource
		 FROM recipe_components c JOIN recipes r ON r.id = c.recipe_id
		 ORDER BY r.good_id, c.pos`)
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

// loadBindings — привязки рецептов к фабрикам (producer_recipes) с good_id
// выхода — проекция CatalogSnapshot.Bindings (спека 2026-09-21-рецепт-сущность
// §5).
func loadBindings(q queryer) ([]RecipeBindingRow, error) {
	rows, err := q.Query(
		`SELECT pr.producer_type_id, pr.recipe_id, r.good_id
		 FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id
		 ORDER BY pr.producer_type_id, pr.recipe_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecipeBindingRow
	for rows.Next() {
		var b RecipeBindingRow
		if err := rows.Scan(&b.ProducerTypeID, &b.RecipeID, &b.GoodID); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
