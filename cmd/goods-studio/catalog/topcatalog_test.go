package catalog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/cmd/goods-studio/model"
)

// testCatalogJSON — малая валидная фикстура каталога (не реальный файл):
// 2 топ-товара, 1 общий внешний good-компонент, ресурсы по именам.
func testCatalogJSON() string {
	return `{
  "schema_version": 1,
  "catalog": "top-goods",
  "generated_at": "2026-09-19T12:00:00Z",
  "summary": {"total": 2, "per_category": {}, "demand": {"continuous": 2, "spike": 0}},
  "goods": [
    {"id": "tg_0001", "name": "Стейк", "category": "продовольствие", "why_top": "статусный стол",
     "constituents": [
       {"name": "Белки", "type": "resource"},
       {"name": "ароматический комплекс", "type": "good"}
     ]},
    {"id": "tg_0002", "name": "Паёк", "category": "продовольствие", "why_top": "стандарт флота",
     "constituents": [
       {"name": "Питательная паста", "type": "good"},
       {"name": "Белки", "type": "resource"}
     ]}
  ]
}`
}

// TestParseTopCatalogValid — валидная фикстура → структуры и маппинг полей.
func TestParseTopCatalogValid(t *testing.T) {
	tc, err := ParseTopCatalog([]byte(testCatalogJSON()))
	require.NoError(t, err)
	require.Equal(t, 1, tc.SchemaVersion)
	require.Len(t, tc.Goods, 2)
	require.Equal(t, "tg_0001", tc.Goods[0].ID)
	require.Equal(t, "Стейк", tc.Goods[0].Name)
	require.Equal(t, "продовольствие", tc.Goods[0].Category)
	require.Equal(t, "статусный стол", tc.Goods[0].WhyTop) // парсер читает, импорт выкидывает
	require.Len(t, tc.Goods[0].Constituents, 2)
	require.Equal(t, "Белки", tc.Goods[0].Constituents[0].Name)
	require.Equal(t, "resource", tc.Goods[0].Constituents[0].Type)
	require.Equal(t, "good", tc.Goods[0].Constituents[1].Type)
}

// TestParseTopCatalogDupID — дубликат id (после trim) → ошибка.
func TestParseTopCatalogDupID(t *testing.T) {
	raw := `{"schema_version":1,"goods":[
		{"id":"tg_0001","name":"A","category":"продовольствие","constituents":[{"name":"Белки","type":"resource"}]},
		{"id":" tg_0001 ","name":"B","category":"топливо","constituents":[{"name":"Сахароза","type":"resource"}]}
	]}`
	_, err := ParseTopCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "дубликат id")
}

// TestParseTopCatalogDupName — дубликат имени (после нормализации) → ошибка.
func TestParseTopCatalogDupName(t *testing.T) {
	raw := `{"schema_version":1,"goods":[
		{"id":"tg_0001","name":"Стейк","category":"продовольствие","constituents":[{"name":"Белки","type":"resource"}]},
		{"id":"tg_0002","name":"  стейк ","category":"топливо","constituents":[{"name":"Сахароза","type":"resource"}]}
	]}`
	_, err := ParseTopCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "дубликат имени")
}

// TestParseTopCatalogEmptyConstituents — пустой состав → ошибка (развилка 4).
func TestParseTopCatalogEmptyConstituents(t *testing.T) {
	raw := `{"schema_version":1,"goods":[
		{"id":"tg_0001","name":"Стейк","category":"продовольствие","constituents":[]}
	]}`
	_, err := ParseTopCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "пустой состав")
}

// TestParseTopCatalogBadType — невалидный тип составляющей → ошибка.
func TestParseTopCatalogBadType(t *testing.T) {
	raw := `{"schema_version":1,"goods":[
		{"id":"tg_0001","name":"Стейк","category":"продовольствие","constituents":[{"name":"Белки","type":"mineral"}]}
	]}`
	_, err := ParseTopCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "невалидный тип")
}

// TestParseTopCatalogBadJSON — битый JSON → ошибка.
func TestParseTopCatalogBadJSON(t *testing.T) {
	_, err := ParseTopCatalog([]byte(`{"schema_version":1,"goods":[`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "невалидный JSON")
}

// TestParseTopCatalogBadSchema — schema_version ≠ 1 → ошибка.
func TestParseTopCatalogBadSchema(t *testing.T) {
	raw := `{"schema_version":2,"goods":[{"id":"tg_0001","name":"A","category":"продовольствие","constituents":[{"name":"Белки","type":"resource"}]}]}`
	_, err := ParseTopCatalog([]byte(raw))
	require.Error(t, err)
	require.Contains(t, err.Error(), "неподдерживаемая версия")
}

// TestParseTopCatalogEmptyGoods — пустой goods → ошибка.
func TestParseTopCatalogEmptyGoods(t *testing.T) {
	_, err := ParseTopCatalog([]byte(`{"schema_version":1,"goods":[]}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "каталог пуст")
}

// testState — состояние с ресурсами (один забаненный), черновиком и
// категориями по умолчанию.
func testState() *model.State {
	st := model.NewState()
	banTime := "2026-09-18T12:00:00Z"
	st.Goods = append(st.Goods,
		model.Good{ID: "res:belki", Name: "Белки", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "belki"}},
		model.Good{ID: "res:zhiry", Name: "Жиры/масла", Category: "mineral", Status: model.StatusBanned, BannedAt: &banTime, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "zhiry"}},
		model.Good{ID: "g1", Name: "Черновик", Category: "c1", Status: model.StatusDraft, Kind: model.KindGood, Source: model.SourceManual},
	)
	return st
}

// TestApplyTopCatalog — замена: ресурсы сохраняются со статусами/банами,
// черновики сносятся, топ-товары approved, внешние компоненты draft,
// резолв по имени (ресурс → res:*, good → ext:*), категории 1:1.
func TestApplyTopCatalog(t *testing.T) {
	st := testState()
	tc, err := ParseTopCatalog([]byte(testCatalogJSON()))
	require.NoError(t, err)

	rep := ApplyTopCatalog(st, tc)
	require.Equal(t, 2, rep.Imported)
	require.Equal(t, 2, rep.ComponentsCreated) // ароматический комплекс + Питательная паста
	require.Equal(t, 1, rep.GoodsRemoved)      // черновик g1
	require.Equal(t, 0, rep.CategoriesCreated) // продовольствие уже есть (c1)

	// ресурсы на месте, статусы/баны сохранены
	byID := map[string]*model.Good{}
	for i := range st.Goods {
		byID[st.Goods[i].ID] = &st.Goods[i]
	}
	require.NotNil(t, byID["res:belki"])
	require.Equal(t, model.StatusResource, byID["res:belki"].Status)
	require.NotNil(t, byID["res:zhiry"])
	require.Equal(t, model.StatusBanned, byID["res:zhiry"].Status)
	require.NotNil(t, byID["res:zhiry"].BannedAt)

	// топ-товары: approved, рецепты заполнены, quantity=1, резолв по имени
	tg1 := byID["tg_0001"]
	require.NotNil(t, tg1)
	require.Equal(t, model.StatusApproved, tg1.Status)
	require.Equal(t, model.SourceImport, tg1.Source)
	require.Equal(t, "c1", tg1.Category)
	require.Len(t, tg1.Recipe, 2)
	require.Equal(t, "res:belki", tg1.Recipe[0].GoodID)
	require.Equal(t, 1, tg1.Recipe[0].Quantity)
	require.Equal(t, "ext:ароматический-комплекс", tg1.Recipe[1].GoodID)

	tg2 := byID["tg_0002"]
	require.NotNil(t, tg2)
	require.Equal(t, "ext:питательная-паста", tg2.Recipe[0].GoodID)
	require.Equal(t, "res:belki", tg2.Recipe[1].GoodID)

	// внешние компоненты: draft, пустые рецепты, категория первого потребителя
	ext := byID["ext:ароматический-комплекс"]
	require.NotNil(t, ext)
	require.Equal(t, model.StatusDraft, ext.Status)
	require.Empty(t, ext.Recipe)
	require.Equal(t, "c1", ext.Category) // первый потребитель tg_0001 → продовольствие
	require.Equal(t, "ароматический комплекс", ext.Name)

	// черновик снесён
	require.Nil(t, byID["g1"])
}

// TestApplyTopCatalogUnknownCategory — неизвестная категория каталога
// создаётся (решение создателя п.4 «новые категории — создавать»).
func TestApplyTopCatalogUnknownCategory(t *testing.T) {
	st := testState()
	raw := `{"schema_version":1,"goods":[
		{"id":"tg_0001","name":"Стейк","category":"Новая категория","constituents":[{"name":"Белки","type":"resource"}]}
	]}`
	tc, err := ParseTopCatalog([]byte(raw))
	require.NoError(t, err)
	rep := ApplyTopCatalog(st, tc)
	require.Equal(t, 1, rep.CategoriesCreated)
	var found bool
	for _, c := range st.Categories {
		if c.Name == "Новая категория" {
			found = true
		}
	}
	require.True(t, found)
	// товар получил id новой категории
	byID := map[string]*model.Good{}
	for i := range st.Goods {
		byID[st.Goods[i].ID] = &st.Goods[i]
	}
	require.NotEqual(t, "", byID["tg_0001"].Category)
}

// TestApplyTopCatalogIdempotent — повторный импорт «заменой всё» идемпотентен:
// те же id (tg_XXXX, ext:<slug>), то же состояние (кроме created_at).
func TestApplyTopCatalogIdempotent(t *testing.T) {
	st := testState()
	tc, err := ParseTopCatalog([]byte(testCatalogJSON()))
	require.NoError(t, err)
	ApplyTopCatalog(st, tc)
	first := snapshotIDs(st)

	ApplyTopCatalog(st, tc)
	second := snapshotIDs(st)
	require.Equal(t, first, second)
	// количество товаров не растёт: 2 ресурса + 2 ext + 2 tg
	require.Len(t, st.Goods, 6)
}

// TestApplyTopCatalogWarnings — ожидаемые предупреждения после импорта
// (99a.2 §9): внешние компоненты draft с пустыми рецептами, на которые
// ссылаются approved топ-товары → missing_resource + non_approved_ref.
func TestApplyTopCatalogWarnings(t *testing.T) {
	st := testState()
	tc, err := ParseTopCatalog([]byte(testCatalogJSON()))
	require.NoError(t, err)
	rep := ApplyTopCatalog(st, tc)
	// 2 внешних компонента (draft, листья, входящая степень > 0) → 2× missing_resource
	// 2 топ-товара ссылаются на draft → 2× non_approved_ref
	require.Len(t, rep.Warnings, 4)
}

// snapshotIDs — id товаров в порядке следования (для сравнения идемпотентности).
func snapshotIDs(st *model.State) []string {
	out := make([]string, 0, len(st.Goods))
	for i := range st.Goods {
		out = append(out, st.Goods[i].ID)
	}
	return out
}

// TestImportReportJSON — отчёт сериализуется в ожидаемую форму (§7).
func TestImportReportJSON(t *testing.T) {
	rep := &ImportReport{Imported: 100, ComponentsCreated: 8, GoodsRemoved: 9, CategoriesCreated: 0, Backup: "goods_data/state.json.bak", Warnings: []string{"w1"}}
	data, err := json.Marshal(rep)
	require.NoError(t, err)
	var back map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &back))
	require.Equal(t, float64(100), back["imported"])
	require.Equal(t, float64(8), back["components_created"])
	require.Equal(t, float64(9), back["goods_removed"])
	require.Equal(t, float64(0), back["categories_created"])
	require.Equal(t, "goods_data/state.json.bak", back["backup"])
	require.Len(t, back["warnings"], 1)
}