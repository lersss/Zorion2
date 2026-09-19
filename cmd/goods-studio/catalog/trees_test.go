package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/model"
)

// testTreesJSON — малая валидная фикстура деревьев (не реальный файл):
// 2 дерева, общая база s_0001 (продублирована в обоих деревьях с тем же id),
// quantity в слотах, ресурсы по именам. Все ref замкнуты, тиры сходятся
// (сверка вычисленного тира с файловым — без предупреждений).
func testTreesJSON() string {
	return `{
  "schema_version": 2,
  "catalog": "top-goods-trees-t8",
  "generated_at": "2026-09-19T18:00:00Z",
  "rework": "тест",
  "trees": [
    {
      "root": {"name": "Топливо «Гелиос»", "tier": 3, "category": "топливо"},
      "nodes": [
        {"id":"h_0001","name":"Топливо «Гелиос»","tier":3,"category":"топливо","constituents":[
          {"ref":"h_0002","name":"Сборки","type":"good"},
          {"name":"Уран-оксид UO₂","type":"resource"}
        ]},
        {"id":"h_0002","name":"Сборки","tier":2,"category":"комплектующие","constituents":[
          {"ref":"s_0001","name":"База","type":"good","quantity":4},
          {"name":"Плутоний Pu","type":"resource"}
        ]},
        {"id":"s_0001","name":"База","tier":1,"category":"комплектующие","constituents":[
          {"name":"Глина","type":"resource"},
          {"name":"Вода H₂O","type":"resource"}
        ]}
      ],
      "leaves_resources": ["Уран-оксид UO₂","Плутоний Pu","Глина","Вода H₂O"]
    },
    {
      "root": {"name": "Микропроцессор «Кремний-9»", "tier": 3, "category": "комплектующие"},
      "nodes": [
        {"id":"k_0001","name":"Микропроцессор «Кремний-9»","tier":3,"category":"комплектующие","constituents":[
          {"ref":"k_0002","name":"Ядра","type":"good"},
          {"name":"Кремний Si","type":"resource"}
        ]},
        {"id":"k_0002","name":"Ядра","tier":2,"category":"комплектующие","constituents":[
          {"ref":"s_0001","name":"База","type":"good","quantity":6},
          {"name":"Графит C","type":"resource"}
        ]},
        {"id":"s_0001","name":"База","tier":1,"category":"комплектующие","constituents":[
          {"name":"Глина","type":"resource"},
          {"name":"Вода H₂O","type":"resource"}
        ]}
      ],
      "leaves_resources": ["Кремний Si","Графит C","Глина","Вода H₂O"]
    }
  ],
  "pool": {"note": "не импортируется"},
  "summary": {"trees": 2}
}`
}

// TestParseTreesCatalogValid — валидная фикстура → структуры и маппинг полей.
func TestParseTreesCatalogValid(t *testing.T) {
	tc, err := ParseTreesCatalog([]byte(testTreesJSON()))
	require.NoError(t, err)
	require.Equal(t, 2, tc.SchemaVersion)
	require.Equal(t, "top-goods-trees-t8", tc.Catalog)
	require.Len(t, tc.Trees, 2)
	require.Equal(t, "Топливо «Гелиос»", tc.Trees[0].Root.Name)
	require.Equal(t, 3, tc.Trees[0].Root.Tier)
	require.Equal(t, "топливо", tc.Trees[0].Root.Category)
	require.Len(t, tc.Trees[0].Nodes, 3)
	// узел: id/name/tier/category + составляющие (ref/name/type/quantity)
	n0 := tc.Trees[0].Nodes[0]
	require.Equal(t, "h_0001", n0.ID)
	require.Equal(t, "Топливо «Гелиос»", n0.Name)
	require.Equal(t, 3, n0.Tier)
	require.Equal(t, "топливо", n0.Category)
	require.Len(t, n0.Constituents, 2)
	require.Equal(t, "h_0002", n0.Constituents[0].Ref)
	require.Equal(t, "Сборки", n0.Constituents[0].Name)
	require.Equal(t, "good", n0.Constituents[0].Type)
	require.Equal(t, 0, n0.Constituents[0].Quantity) // не задано → 1 при импорте
	require.Equal(t, "", n0.Constituents[1].Ref)     // ресурс — лист, ref нет
	require.Equal(t, "resource", n0.Constituents[1].Type)
	// quantity в слоте читается
	require.Equal(t, 4, tc.Trees[0].Nodes[1].Constituents[0].Quantity)
	// общая база продублирована в обоих деревьях (дедуп — на импорте)
	require.Equal(t, "s_0001", tc.Trees[0].Nodes[2].ID)
	require.Equal(t, "s_0001", tc.Trees[1].Nodes[2].ID)
}

// TestParseTreesCatalogBadSchema — schema_version ≠ 2 → ошибка.
func TestParseTreesCatalogBadSchema(t *testing.T) {
	raw := `{"schema_version":1,"trees":[{"root":{"name":"A","tier":1,"category":"топливо"},"nodes":[{"id":"h_0001","name":"A","tier":1,"category":"топливо","constituents":[{"name":"Глина","type":"resource"}]}]}]}`
	_, err := ParseTreesCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "неподдерживаемая версия")
}

// TestParseTreesCatalogEmptyTrees — пустой trees → ошибка.
func TestParseTreesCatalogEmptyTrees(t *testing.T) {
	_, err := ParseTreesCatalog([]byte(`{"schema_version":2,"trees":[]}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "каталог пуст")
}

// TestParseTreesCatalogEmptyRootName — пустое имя корня → ошибка.
func TestParseTreesCatalogEmptyRootName(t *testing.T) {
	raw := `{"schema_version":2,"trees":[{"root":{"name":"","tier":1,"category":"топливо"},"nodes":[{"id":"h_0001","name":"A","tier":1,"category":"топливо","constituents":[{"name":"Глина","type":"resource"}]}]}]}`
	_, err := ParseTreesCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "пустое имя корня")
}

// TestParseTreesCatalogDupID — дубликат id узла в пределах одного дерева → ошибка.
func TestParseTreesCatalogDupID(t *testing.T) {
	raw := `{"schema_version":2,"trees":[{"root":{"name":"A","tier":1,"category":"топливо"},"nodes":[
		{"id":"h_0001","name":"A","tier":1,"category":"топливо","constituents":[{"name":"Глина","type":"resource"}]},
		{"id":"h_0001","name":"B","tier":1,"category":"топливо","constituents":[{"name":"Вода H₂O","type":"resource"}]}
	]}]}`
	_, err := ParseTreesCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "дубликат id")
}

// TestParseTreesCatalogDupIDCrossTreeDiff — один id в двух деревьях с РАЗНЫМ
// содержимым → ошибка (общая база s_* дублируется только дословно).
func TestParseTreesCatalogDupIDCrossTreeDiff(t *testing.T) {
	raw := `{"schema_version":2,"trees":[
		{"root":{"name":"A","tier":1,"category":"топливо"},"nodes":[{"id":"s_0001","name":"База","tier":1,"category":"комплектующие","constituents":[{"name":"Глина","type":"resource"}]}]},
		{"root":{"name":"B","tier":1,"category":"топливо"},"nodes":[{"id":"s_0001","name":"Другая база","tier":1,"category":"комплектующие","constituents":[{"name":"Вода H₂O","type":"resource"}]}]}
	]}`
	_, err := ParseTreesCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "дубликат id")
}

// TestParseTreesCatalogEmptyConstituents — пустой состав узла → ошибка.
func TestParseTreesCatalogEmptyConstituents(t *testing.T) {
	raw := `{"schema_version":2,"trees":[{"root":{"name":"A","tier":1,"category":"топливо"},"nodes":[{"id":"h_0001","name":"A","tier":1,"category":"топливо","constituents":[]}]}]}`
	_, err := ParseTreesCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "пустой состав")
}

// TestParseTreesCatalogBadType — невалидный тип составляющей → ошибка.
func TestParseTreesCatalogBadType(t *testing.T) {
	raw := `{"schema_version":2,"trees":[{"root":{"name":"A","tier":1,"category":"топливо"},"nodes":[{"id":"h_0001","name":"A","tier":1,"category":"топливо","constituents":[{"name":"Глина","type":"mineral"}]}]}]}`
	_, err := ParseTreesCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "невалидный тип")
}

// TestParseTreesCatalogBadJSON — битый JSON → ошибка.
func TestParseTreesCatalogBadJSON(t *testing.T) {
	_, err := ParseTreesCatalog([]byte(`{"schema_version":2,"trees":[`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "невалидный JSON")
}

// testTreesState — состояние с ресурсами по именам фикстуры, забаненным
// ресурсом вне рецептов (Асбест — бан сохраняется, валидатор не ругается),
// черновиком и категориями по умолчанию.
func testTreesState() *model.State {
	st := model.NewState()
	banTime := "2026-09-18T12:00:00Z"
	st.Goods = append(st.Goods,
		model.Good{ID: "res:uran-oksid", Name: "Уран-оксид UO₂", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "uran-oksid"}},
		model.Good{ID: "res:plutoniy", Name: "Плутоний Pu", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "plutoniy"}},
		model.Good{ID: "res:glina", Name: "Глина", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "glina"}},
		model.Good{ID: "res:voda", Name: "Вода H₂O", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "voda"}},
		model.Good{ID: "res:kremniy", Name: "Кремний Si", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "kremniy"}},
		model.Good{ID: "res:grafit", Name: "Графит C", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "grafit"}},
		model.Good{ID: "res:asbest", Name: "Асбест", Category: "mineral", Status: model.StatusBanned, BannedAt: &banTime, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "asbest"}},
		model.Good{ID: "g1", Name: "Черновик", Category: "c1", Status: model.StatusDraft, Kind: model.KindGood, Source: model.SourceManual},
	)
	return st
}

// TestApplyTreesCatalog — замена: ресурсы сохраняются со статусами/банами,
// черновики сносятся, все узлы approved, дедуп общей базы (s_0001 один раз),
// рецепты с quantity, резолв good по ref и resource по имени, категории 1:1.
func TestApplyTreesCatalog(t *testing.T) {
	st := testTreesState()
	tc, err := ParseTreesCatalog([]byte(testTreesJSON()))
	require.NoError(t, err)

	rep := ApplyTreesCatalog(st, tc)
	require.Equal(t, 2, rep.Trees)
	require.Equal(t, 5, rep.NodesCreated)      // h_0001, h_0002, s_0001, k_0001, k_0002
	require.Equal(t, 5, rep.Imported)          // для деревьев Imported = узлов создано
	require.Equal(t, 1, rep.BaseDeduplicated)  // s_0001 во втором дереве
	require.Equal(t, 1, rep.GoodsRemoved)      // черновик g1
	require.Equal(t, 0, rep.CategoriesCreated) // топливо/комплектующие уже есть
	require.Empty(t, rep.Warnings)             // тиры сходятся, валидаторы чисты

	byID := map[string]*model.Good{}
	for i := range st.Goods {
		byID[st.Goods[i].ID] = &st.Goods[i]
	}

	// ресурсы на месте, статусы/баны сохранены
	require.NotNil(t, byID["res:uran-oksid"])
	require.Equal(t, model.StatusResource, byID["res:uran-oksid"].Status)
	require.NotNil(t, byID["res:asbest"])
	require.Equal(t, model.StatusBanned, byID["res:asbest"].Status)
	require.NotNil(t, byID["res:asbest"].BannedAt)

	// все узлы: approved, kind=good, source=import
	for _, id := range []string{"h_0001", "h_0002", "s_0001", "k_0001", "k_0002"} {
		g := byID[id]
		require.NotNil(t, g, id)
		require.Equal(t, model.StatusApproved, g.Status, id)
		require.Equal(t, model.KindGood, g.Kind, id)
		require.Equal(t, model.SourceImport, g.Source, id)
	}

	// дедуп общей базы: s_0001 ровно один, рецепт заполнен ОДИН раз
	// (без дублей слотов от второго дерева — регрессия двойного рецепта)
	require.NotNil(t, byID["s_0001"])
	require.Equal(t, "База", byID["s_0001"].Name)
	require.Len(t, byID["s_0001"].Recipe, 2) // Глина + Вода H₂O
	require.Equal(t, "res:glina", byID["s_0001"].Recipe[0].GoodID)
	require.Equal(t, "res:voda", byID["s_0001"].Recipe[1].GoodID)

	// рецепты: good по ref, resource по имени, quantity из файла или 1
	h1 := byID["h_0001"]
	require.Len(t, h1.Recipe, 2)
	require.Equal(t, "h_0002", h1.Recipe[0].GoodID)         // good по ref
	require.Equal(t, 1, h1.Recipe[0].Quantity)              // без quantity → 1
	require.Equal(t, "res:uran-oksid", h1.Recipe[1].GoodID) // resource по имени
	require.Equal(t, 1, h1.Recipe[1].Quantity)

	h2 := byID["h_0002"]
	require.Len(t, h2.Recipe, 2)
	require.Equal(t, "s_0001", h2.Recipe[0].GoodID)
	require.Equal(t, 4, h2.Recipe[0].Quantity) // quantity из файла
	require.Equal(t, "res:plutoniy", h2.Recipe[1].GoodID)

	k2 := byID["k_0002"]
	require.Equal(t, "s_0001", k2.Recipe[0].GoodID)
	require.Equal(t, 6, k2.Recipe[0].Quantity)

	// черновик снесён
	require.Nil(t, byID["g1"])
}

// TestApplyTreesCatalogUnknownCategory — неизвестная категория создаётся.
func TestApplyTreesCatalogUnknownCategory(t *testing.T) {
	st := testTreesState()
	raw := `{"schema_version":2,"trees":[{"root":{"name":"A","tier":1,"category":"Новая категория"},"nodes":[{"id":"h_0001","name":"A","tier":1,"category":"Новая категория","constituents":[{"name":"Глина","type":"resource"}]}]}]}`
	tc, err := ParseTreesCatalog([]byte(raw))
	require.NoError(t, err)
	rep := ApplyTreesCatalog(st, tc)
	require.Equal(t, 1, rep.CategoriesCreated)
	var found bool
	for _, c := range st.Categories {
		if c.Name == "Новая категория" {
			found = true
		}
	}
	require.True(t, found)
}

// TestApplyTreesCatalogBrokenRef — ref на отсутствующий узел → пустой слот +
// предупреждение (свойство среза, не ошибка).
func TestApplyTreesCatalogBrokenRef(t *testing.T) {
	st := testTreesState()
	raw := `{"schema_version":2,"trees":[{"root":{"name":"A","tier":2,"category":"топливо"},"nodes":[
		{"id":"h_0001","name":"A","tier":2,"category":"топливо","constituents":[
			{"ref":"h_0002","name":"Сборки","type":"good"},
			{"ref":"h_9999","name":"Нет такого узла","type":"good"},
			{"name":"Уран-оксид UO₂","type":"resource"}
		]},
		{"id":"h_0002","name":"Сборки","tier":1,"category":"комплектующие","constituents":[{"name":"Глина","type":"resource"}]}
	]}]}`
	tc, err := ParseTreesCatalog([]byte(raw))
	require.NoError(t, err)
	rep := ApplyTreesCatalog(st, tc)
	require.Contains(t, rep.Warnings, "не найден компонент по ref: h_9999 (узел h_0001)")
	byID := map[string]*model.Good{}
	for i := range st.Goods {
		byID[st.Goods[i].ID] = &st.Goods[i]
	}
	require.Len(t, byID["h_0001"].Recipe, 3)
	require.Equal(t, "h_0002", byID["h_0001"].Recipe[0].GoodID)
	require.Empty(t, byID["h_0001"].Recipe[1].GoodID) // битый ref → пустой слот
	require.Equal(t, "res:uran-oksid", byID["h_0001"].Recipe[2].GoodID)
}

// TestApplyTreesCatalogTierMismatch — вычисленный тир ≠ тиру из файла →
// предупреждение (не ошибка).
func TestApplyTreesCatalogTierMismatch(t *testing.T) {
	st := testTreesState()
	raw := `{"schema_version":2,"trees":[{"root":{"name":"A","tier":5,"category":"топливо"},"nodes":[
		{"id":"h_0001","name":"A","tier":5,"category":"топливо","constituents":[
			{"ref":"h_0002","name":"Сборки","type":"good"},
			{"name":"Уран-оксид UO₂","type":"resource"}
		]},
		{"id":"h_0002","name":"Сборки","tier":1,"category":"комплектующие","constituents":[{"name":"Глина","type":"resource"}]}
	]}]}`
	tc, err := ParseTreesCatalog([]byte(raw))
	require.NoError(t, err)
	rep := ApplyTreesCatalog(st, tc)
	require.Contains(t, rep.Warnings, "тир узла h_0001: вычисленный 2 ≠ из файла 5")
}

// TestApplyTreesCatalogIdempotent — повторный импорт «заменой всё» идемпотентен:
// те же id (h_*/k_*/s_*), то же состояние (кроме created_at).
func TestApplyTreesCatalogIdempotent(t *testing.T) {
	st := testTreesState()
	tc, err := ParseTreesCatalog([]byte(testTreesJSON()))
	require.NoError(t, err)
	ApplyTreesCatalog(st, tc)
	first := snapshotIDs(st)

	ApplyTreesCatalog(st, tc)
	second := snapshotIDs(st)
	require.Equal(t, first, second)
	// количество не растёт: 7 ресурсов + 5 узлов
	require.Len(t, st.Goods, 12)
}
