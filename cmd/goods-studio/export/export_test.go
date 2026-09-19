package export

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/cmd/goods-studio/model"
)

func mkState(goods ...model.Good) *model.State {
	return &model.State{
		SchemaVersion: 1,
		Categories: []model.Category{
			{ID: "c1", Name: "Корабли"},
			{ID: "c2", Name: "Конструкционные материалы"},
		},
		Goods: goods,
	}
}

func good(id, name, cat string, status model.Status, recipe ...model.Slot) model.Good {
	return model.Good{ID: id, Name: name, Category: cat, Status: status, Kind: model.KindGood, Recipe: recipe}
}

func res(id, name, cat, catalog, catalogID string) model.Good {
	return model.Good{
		ID: id, Name: name, Category: cat, Status: model.StatusResource, Kind: model.KindResource,
		ResourceRef: &model.ResourceRef{Catalog: catalog, CatalogID: catalogID},
	}
}

// TestExportOnlyApproved — в выгрузку идут только согласованные.
func TestExportOnlyApproved(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", "c2", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		good("g2", "Черновик", "c2", model.StatusDraft),
		good("g3", "Исключённый", "c2", model.StatusExcluded),
		good("g4", "Забаненный", "c2", model.StatusBanned),
		res("res:zhelezo", "Железо Fe", "mineral", "real", "zhelezo"),
	)
	ex := Build(st)
	require.Len(t, ex.Goods, 1)
	require.Equal(t, "g1", ex.Goods[0].ID)
}

// TestExportComplexity — complexity = тир товара в графе (ресурс = 0,
// базовый товар из ресурсов = 1).
func TestExportComplexity(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", "c2", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}, model.Slot{GoodID: "res:uglerod"}),
		good("g2", "Каркас", "c2", model.StatusApproved, model.Slot{GoodID: "g1"}),
		res("res:zhelezo", "Железо Fe", "mineral", "real", "zhelezo"),
		res("res:uglerod", "Углерод C", "mineral", "real", "uglerod"),
	)
	ex := Build(st)
	require.Len(t, ex.Goods, 2)
	require.Equal(t, 1, ex.Goods[0].Complexity) // g1: 1 + max(0,0)
	require.Equal(t, 2, ex.Goods[1].Complexity) // g2: 1 + max(1)
}

// TestExportComplexityOverride — complexity = эффективный тир (99a.3 §12):
// оверрайд если задан, иначе вычисленный.
func TestExportComplexityOverride(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", "c2", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe", "mineral", "real", "zhelezo"),
	)
	// без оверрайда — вычисленный (1)
	ex := Build(st)
	require.Equal(t, 1, ex.Goods[0].Complexity)
	// с оверрайдом 7 — 7 (в т.ч. ниже/выше вычисленного)
	ov := 7
	st.Goods[0].TierOverride = &ov
	ex = Build(st)
	require.Equal(t, 7, ex.Goods[0].Complexity)
	// очистка (null) — снова вычисленный
	st.Goods[0].TierOverride = nil
	ex = Build(st)
	require.Equal(t, 1, ex.Goods[0].Complexity)
}

// TestExportQuantityOne — quantity = 1 (1 слот = 1 единица, §6.4);
// пустые слоты не входят.
func TestExportQuantityOne(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", "c2", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}, model.Slot{}),
		res("res:zhelezo", "Железо Fe", "mineral", "real", "zhelezo"),
	)
	ex := Build(st)
	require.Len(t, ex.Goods[0].Recipe, 1) // пустой слот не входит
	require.Equal(t, "res:zhelezo", ex.Goods[0].Recipe[0].GoodID)
	require.Equal(t, 1, ex.Goods[0].Recipe[0].Quantity)
}

// TestExportQuantityReal — quantity = реальное количество слота (99a.2 §6.3):
// 3 → 3; 0 (старый слот без миграции) → 1.
func TestExportQuantityReal(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", "c2", model.StatusApproved,
			model.Slot{GoodID: "res:zhelezo", Quantity: 3},
			model.Slot{GoodID: "res:uglerod"}), // quantity 0 → 1
		res("res:zhelezo", "Железо Fe", "mineral", "real", "zhelezo"),
		res("res:uglerod", "Углерод C", "mineral", "real", "uglerod"),
	)
	ex := Build(st)
	require.Len(t, ex.Goods[0].Recipe, 2)
	require.Equal(t, 3, ex.Goods[0].Recipe[0].Quantity)
	require.Equal(t, 1, ex.Goods[0].Recipe[1].Quantity)
}

// TestExportResources — ресурсы: только те, на которые ссылаются
// выгруженные рецепты, с источником каталога.
func TestExportResources(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", "c2", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe", "mineral", "real", "zhelezo"),
		res("res:water", "вода-ресурс", "water", "layer", "water"), // не используется
	)
	ex := Build(st)
	require.Len(t, ex.Resources, 1)
	require.Equal(t, "res:zhelezo", ex.Resources[0].ID)
	require.Equal(t, "real", ex.Resources[0].Catalog)
	require.Equal(t, "zhelezo", ex.Resources[0].CatalogID)
}

// TestExportCategories — категории выгружаются как есть, только
// используемые выгруженными товарами.
func TestExportCategories(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", "c2", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe", "mineral", "real", "zhelezo"),
	)
	ex := Build(st)
	require.Len(t, ex.Categories, 1)
	require.Equal(t, "c2", ex.Categories[0].ID)
	require.Equal(t, "Конструкционные материалы", ex.Categories[0].Name)
}

// TestExportWarnings — warnings — вывод валидаторов при экспорте.
func TestExportWarnings(t *testing.T) {
	st := mkState(
		good("g1", "Согласован без рецепта", "c2", model.StatusApproved),
	)
	ex := Build(st)
	require.NotEmpty(t, ex.Warnings)
	require.Equal(t, "incomplete_chain", ex.Warnings[0].Code)
}

// TestExportValidJSON — выгрузка — валидный JSON (критерий приёмки 5).
func TestExportValidJSON(t *testing.T) {
	st := mkState(
		good("g1", "Сталь", "c2", model.StatusApproved, model.Slot{GoodID: "res:zhelezo"}),
		res("res:zhelezo", "Железо Fe", "mineral", "real", "zhelezo"),
	)
	ex := Build(st)
	data, err := json.Marshal(ex)
	require.NoError(t, err)
	var back map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &back))
	require.Equal(t, float64(1), back["schema_version"])
}