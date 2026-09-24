// internal/repository/content_export.go
// Снимок контента каталога для переноса dev → прод (спека
// 2026-09-24-каталог-экспорт-импорт-контента-на-прод §4, итерация И2):
// наборы строк контентных таблиц (categories/goods/recipes/recipe_components/
// producer_types/items/producer_slots/producer_recipes/producer_items/
// effect_types + ссылка-якорь generation_config.default_settlement_type_id)
// в одной транзакции чтения REPEATABLE READ (по образцу CatalogSnapshot §8.1).
// Репозиторий отдаёт СЫРЫЕ строки с id — резолв ссылок по метке (§3) и
// сборку natural-key формата делает слой обработчиков (studio_content.go).
// Каталог — только чтение: advisory-лок не берём (мутации — beginMutation).
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"zorion/internal/models"
)

// ContentGoodRow — строка goods в снимке контента: добавка к model.Good —
// props (в model.Good поля нет) и code (метка переноса, §3.1).
type ContentGoodRow struct {
	ID          int64
	Name        string
	Kind        string
	CategoryID  int64
	Description sql.NullString
	Volume      sql.NullFloat64
	Weight      sql.NullFloat64
	Props       []byte // JSONB
	Source      string
	Code        sql.NullString
}

// ContentRecipeRow — рецепт товара (recipes): выход — good_id, сложность (§4).
type ContentRecipeRow struct {
	GoodID     int64
	Complexity sql.NullInt64
}

// ContentComponentRow — позиция состава рецепта (recipe_components):
// good_id — выход товара рецепта (джойн), pos — позиция, component_id —
// ссылка на составляющую (NULL = пустой слот).
type ContentComponentRow struct {
	GoodID        int64
	Pos           int
	ComponentID   sql.NullInt64
	Quantity      int
	Reason        sql.NullString
	AllowResource bool
}

// ContentProducerRecipeRow — привязка «постройка × рецепт» (producer_recipes):
// good_id — выход рецепта (для ссылки по метке товара, §4).
type ContentProducerRecipeRow struct {
	ProducerTypeID int64
	GoodID         int64
	Rate           sql.NullFloat64
}

// ContentEffectTypeRow — тип эффекта (effect_types) с ПОЛНЫМ params: в формате
// §4 params переносится дословно; EffectTypeRow отдаёт только curve.
type ContentEffectTypeRow struct {
	ID     int64
	Name   string
	Impact string
	Params []byte // JSONB
	Code   sql.NullString
}

// ContentExportRows — согласованный снимок контентных таблиц (одна транзакция
// REPEATABLE READ). Переиспользует существующие строки каталога (CategoryRow,
// ProducerTypeRow, ItemRow, ProducerSlotRow, ProducerItemRow) — их загрузчики
// уже отдают метку code (000084).
type ContentExportRows struct {
	Categories      []CategoryRow
	Goods           []ContentGoodRow
	Recipes         []ContentRecipeRow
	Components      []ContentComponentRow
	ProducerTypes   []ProducerTypeRow
	Items           []ItemRow
	ProducerSlots   []ProducerSlotRow
	ProducerRecipes []ContentProducerRecipeRow
	ProducerItems   []ProducerItemRow
	EffectTypes     []ContentEffectTypeRow
	// DefaultTypeID — id записи-якоря базового типа поселения
	// (generation_config.default_settlement_type_id); 0 — ключа нет/не число.
	DefaultTypeID int64
}

// ContentExport — снимок контента каталога (§4) в одной транзакции чтения
// REPEATABLE READ. Только чтение (адвизори-лок мутаций не берётся).
func (r *GoodsRepository) ContentExport() (*ContentExportRows, error) {
	tx, err := r.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	out := &ContentExportRows{}
	if out.Categories, err = loadCategories(tx); err != nil {
		return nil, err
	}
	if out.Goods, err = loadContentGoods(tx); err != nil {
		return nil, err
	}
	if out.Recipes, out.Components, err = loadContentRecipes(tx); err != nil {
		return nil, err
	}
	if out.ProducerTypes, err = loadProducerTypes(tx); err != nil {
		return nil, err
	}
	if out.Items, err = loadItems(tx); err != nil {
		return nil, err
	}
	if out.ProducerSlots, err = loadProducerSlots(tx); err != nil {
		return nil, err
	}
	if out.ProducerRecipes, err = loadContentProducerRecipes(tx); err != nil {
		return nil, err
	}
	if out.ProducerItems, err = loadProducerItems(tx); err != nil {
		return nil, err
	}
	if out.EffectTypes, err = loadContentEffectTypes(tx); err != nil {
		return nil, err
	}
	if out.DefaultTypeID, err = loadDefaultSettlementTypeIDRow(tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// loadContentGoods — товары/ресурсы с props и меткой (§4: props переносится
// дословно).
func loadContentGoods(q queryer) ([]ContentGoodRow, error) {
	rows, err := q.Query(
		`SELECT id, name, kind, category_id, description, volume, weight, props, source, code
		 FROM goods ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContentGoodRow
	for rows.Next() {
		var g ContentGoodRow
		if err := rows.Scan(&g.ID, &g.Name, &g.Kind, &g.CategoryID, &g.Description,
			&g.Volume, &g.Weight, &g.Props, &g.Source, &g.Code); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// loadContentRecipes — рецепты и их состав (§4): recipes отдельной секцией,
// recipe_components — позициями с good_id выхода рецепта.
func loadContentRecipes(q queryer) ([]ContentRecipeRow, []ContentComponentRow, error) {
	rs, err := q.Query(`SELECT good_id, complexity FROM recipes ORDER BY good_id`)
	if err != nil {
		return nil, nil, err
	}
	defer rs.Close()

	var recipes []ContentRecipeRow
	for rs.Next() {
		var r ContentRecipeRow
		if err := rs.Scan(&r.GoodID, &r.Complexity); err != nil {
			return nil, nil, err
		}
		recipes = append(recipes, r)
	}
	if err := rs.Err(); err != nil {
		return nil, nil, err
	}

	cs, err := q.Query(
		`SELECT r.good_id, c.pos, c.component_id, c.quantity, c.reason, c.allow_resource
		 FROM recipe_components c JOIN recipes r ON r.id = c.recipe_id
		 ORDER BY r.good_id, c.pos`)
	if err != nil {
		return nil, nil, err
	}
	defer cs.Close()

	var components []ContentComponentRow
	for cs.Next() {
		var c ContentComponentRow
		if err := cs.Scan(&c.GoodID, &c.Pos, &c.ComponentID, &c.Quantity, &c.Reason, &c.AllowResource); err != nil {
			return nil, nil, err
		}
		components = append(components, c)
	}
	return recipes, components, cs.Err()
}

// loadContentProducerRecipes — привязки рецептов к постройкам с выходом
// рецепта (good_id) и числом скорости пары (§4: producer_recipes {producer,
// good, rate}).
func loadContentProducerRecipes(q queryer) ([]ContentProducerRecipeRow, error) {
	rows, err := q.Query(
		`SELECT pr.producer_type_id, r.good_id, pr.rate
		 FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id
		 ORDER BY pr.producer_type_id, r.good_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContentProducerRecipeRow
	for rows.Next() {
		var b ContentProducerRecipeRow
		if err := rows.Scan(&b.ProducerTypeID, &b.GoodID, &b.Rate); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// loadContentEffectTypes — типы эффектов с полным params (§4).
func loadContentEffectTypes(q queryer) ([]ContentEffectTypeRow, error) {
	rows, err := q.Query(`SELECT id, name, impact, params, code FROM effect_types ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContentEffectTypeRow
	for rows.Next() {
		var e ContentEffectTypeRow
		if err := rows.Scan(&e.ID, &e.Name, &e.Impact, &e.Params, &e.Code); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// loadDefaultSettlementTypeIDRow — id записи-якоря базового типа поселения из
// generation_config (§1.3/§4). Ключа нет или payload не число — 0 (как
// ResolveDefaultSettlementTypeID, без ошибки).
func loadDefaultSettlementTypeIDRow(q queryer) (int64, error) {
	var raw []byte
	err := q.QueryRow(
		`SELECT payload FROM generation_config WHERE key = $1`,
		models.DefaultSettlementTypeIDKey,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, nil
	}
	return id, nil
}
