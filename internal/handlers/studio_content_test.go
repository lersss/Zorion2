// internal/handlers/studio_content_test.go
// Формат снимка контента (спека 2026-09-24-каталог-экспорт-импорт-контента-
// на-прод §4, итерация И2): секции в топологическом порядке, ссылки — по
// метке/натуральному ключу, `code` у контентных записей обязателен, `id` в
// файле отсутствует; producer_types — родители раньше детей.
package handlers

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
)

// contentFixture — минимальный снимок строк: 2 категории, 2 товара (рецепт с
// составляющей и пустым слотом), 3 типа производителей (дети ВПЕРЁД родителя
// во входном срезе), предмет, слот, привязки, эффект, якорь.
func contentFixture() *repository.ContentExportRows {
	five := 5.0
	return &repository.ContentExportRows{
		Categories: []repository.CategoryRow{
			{ID: 1, Name: "Продовольствие", Kind: "good"},
			{ID: 2, Name: "Вода", Kind: "resource", Code: sql.NullString{String: "water", Valid: true}},
		},
		Goods: []repository.ContentGoodRow{
			{ID: 10, Name: "Пища", Kind: "good", CategoryID: 1, Source: "manual",
				Code: sql.NullString{String: "g_0001", Valid: true}},
			{ID: 11, Name: "Вода неочищенная", Kind: "resource", CategoryID: 2, Source: "manual",
				Props: []byte(`{"family":"water"}`),
				Code:  sql.NullString{String: "g_0002", Valid: true}},
		},
		Recipes: []repository.ContentRecipeRow{
			{GoodID: 10, Complexity: sql.NullInt64{Int64: 3, Valid: true}},
		},
		Components: []repository.ContentComponentRow{
			{GoodID: 10, Pos: 0, ComponentID: sql.NullInt64{Int64: 11, Valid: true}, Quantity: 2,
				Reason: sql.NullString{String: "вода", Valid: true}, AllowResource: true},
			{GoodID: 10, Pos: 1, Quantity: 1},
		},
		ProducerTypes: []repository.ProducerTypeRow{
			{ID: 2, Name: "Форпост", Kind: "goods", ParentID: sql.NullInt64{Int64: 1, Valid: true},
				Params: []byte(`{"stage":{"enter":0}}`), Code: sql.NullString{String: "p_0002", Valid: true}},
			{ID: 1, Name: "Колония", Kind: "goods", Code: sql.NullString{String: "p_0001", Valid: true}},
			{ID: 3, Name: "Поселение", Kind: "goods", ParentID: sql.NullInt64{Int64: 1, Valid: true},
				Code: sql.NullString{String: "p_0003", Valid: true}},
		},
		Items: []repository.ItemRow{
			{ID: 1, Name: "Чертёж", SlotType: "чертёж", Unlocks: []byte(`[{"producer_type_id":1,"category_id":1}]`),
				Code: sql.NullString{String: "i_0001", Valid: true}},
		},
		ProducerSlots: []repository.ProducerSlotRow{
			{ID: 1, ParentID: 1, CategoryID: 1},
		},
		ProducerRecipes: []repository.ContentProducerRecipeRow{
			{ProducerTypeID: 2, GoodID: 10, Rate: sql.NullFloat64{Float64: five, Valid: true}},
		},
		ProducerItems: []repository.ProducerItemRow{
			{ProducerTypeID: 1, ItemID: 1},
		},
		EffectTypes: []repository.ContentEffectTypeRow{
			{ID: 1, Name: "Голод", Impact: "population_rate", Params: []byte(`{"curve":"hunger"}`),
				Code: sql.NullString{String: "e_0001", Valid: true}},
		},
		DefaultTypeID: 1,
	}
}

// TestBuildContentSnapshotFormat — секции/порядок/метки/отсутствие id.
func TestBuildContentSnapshotFormat(t *testing.T) {
	snap, err := buildContentSnapshot(contentFixture(), time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, 1, snap.SchemaVersion)
	require.Equal(t, "dev", snap.Source)
	require.Equal(t, "2026-09-25T12:00:00Z", snap.GeneratedAt)
	require.Equal(t, 2, snap.Counts["goods"])
	require.Equal(t, 3, snap.Counts["producer_types"])
	require.Equal(t, 1, snap.Counts["effect_types"])
	require.Equal(t, "p_0001", snap.GenerationConfig.DefaultSettlementTypeID)

	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	out := string(data)

	// Порядок секций (§4, топологический по FK). Ищем ТОЛЬКО ключи верхнего
	// уровня (отступ 2 пробела): вложенный counts несёт те же имена в
	// алфавитном порядке и ломал бы наивный поиск.
	order := []string{
		`"categories"`, `"goods"`, `"recipes"`, `"recipe_components"`,
		`"producer_types"`, `"items"`, `"producer_slots"`, `"producer_recipes"`,
		`"producer_items"`, `"effect_types"`, `"generation_config"`,
	}
	prev := -1
	for _, key := range order {
		i := topKeyIdx(out, key)
		require.Greaterf(t, i, prev, "секция %s не в топологическом порядке (§4)", key)
		prev = i
	}
	// Уточнение критики: items после producer_types и до producer_items.
	require.Less(t, topKeyIdx(out, `"producer_types"`), topKeyIdx(out, `"items"`))
	require.Less(t, topKeyIdx(out, `"items"`), topKeyIdx(out, `"producer_items"`))

	// Никаких id (§4): в файле нет ключа "id".
	require.False(t, strings.Contains(out, `"id"`), "снимок не должен нести id:\n%s", out)
	// unlocks — по метке/натуральному ключу, не по внутренним id (§4/T15).
	require.NotContains(t, out, "producer_type_id")
	require.NotContains(t, out, "category_id")
	require.Contains(t, out, `"producer": "p_0001"`)
	// Метки на месте.
	require.Contains(t, out, `"code": "g_0001"`)
	require.Contains(t, out, `"code": "p_0002"`)
	require.Contains(t, out, `"code": "i_0001"`)
	require.Contains(t, out, `"code": "e_0001"`)

	// producer_types: родители раньше детей.
	var ptypes []ContentProducerType
	require.NoError(t, json.Unmarshal(mustJSON(t, snap, "producer_types"), &ptypes))
	require.Equal(t, []string{"p_0001", "p_0002", "p_0003"}, []string{
		ptypes[0].Code, ptypes[1].Code, ptypes[2].Code,
	})
	require.Equal(t, "", ptypes[0].Parent)
	require.Equal(t, "p_0001", ptypes[1].Parent)

	// unlocks: id-форма БД → метка/натуральный ключ в снимке (§4/T15).
	require.Len(t, snap.Items, 1)
	require.Equal(t, []ContentUnlock{{
		Producer: "p_0001",
		Category: ContentUnlockCategory{Kind: "good", Name: "Продовольствие"},
	}}, snap.Items[0].Unlocks)
}

// TestBuildContentSnapshotUnlocksUnresolvedFails — нерезолвимый id в unlocks
// (producer_type_id/category_id) → отказ, не молчаливый пропуск (§4/T15).
func TestBuildContentSnapshotUnlocksUnresolvedFails(t *testing.T) {
	rows := contentFixture()
	rows.Items[0].Unlocks = []byte(`[{"producer_type_id":999,"category_id":1}]`)
	_, err := buildContentSnapshot(rows, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "producer_type_id 999")

	rows = contentFixture()
	rows.Items[0].Unlocks = []byte(`[{"producer_type_id":1,"category_id":999}]`)
	_, err = buildContentSnapshot(rows, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "category_id 999")
}

// TestBuildContentSnapshotEmptyUnlocksOmitted — пустой unlocks (`[]`/NULL) не
// попадает в снимок (§4: пусто → пусто, omitempty).
func TestBuildContentSnapshotEmptyUnlocksOmitted(t *testing.T) {
	rows := contentFixture()
	rows.Items[0].Unlocks = []byte(`[]`)
	snap, err := buildContentSnapshot(rows, time.Now())
	require.NoError(t, err)
	require.Empty(t, snap.Items[0].Unlocks)
	b, err := json.Marshal(snap)
	require.NoError(t, err)
	require.NotContains(t, string(b), "unlocks")
}

// TestBuildContentSnapshotMissingCodeFails — контентная запись без метки —
// ошибка (снимок обязан быть полным, §3.1/§4).
func TestBuildContentSnapshotMissingCodeFails(t *testing.T) {
	rows := contentFixture()
	rows.Goods[0].Code = sql.NullString{}
	_, err := buildContentSnapshot(rows, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "goods")
	require.Contains(t, err.Error(), "метка")
}

// TestBuildContentSnapshotSection — снимок несёт раздел постройки
// (producer_types.section, спека 2026-09-25 §4): непустой section попадает в
// JSON, NULL/пустой — не пишется (omitempty), иначе импорт на проде затирал бы
// раздел в NULL.
func TestBuildContentSnapshotSection(t *testing.T) {
	rows := contentFixture()
	rows.ProducerTypes[1].Section = sql.NullString{String: "colony", Valid: true}
	snap, err := buildContentSnapshot(rows, time.Now())
	require.NoError(t, err)

	var ptypes []ContentProducerType
	require.NoError(t, json.Unmarshal(mustJSON(t, snap, "producer_types"), &ptypes))
	// Порядок снимка: p_0001 (Колония, index 1 во входе), p_0002, p_0003.
	require.Equal(t, "colony", ptypes[0].Section)
	require.Equal(t, "", ptypes[1].Section)

	data, err := json.Marshal(snap)
	require.NoError(t, err)
	out := string(data)
	require.Contains(t, out, `"section":"colony"`)
	// Ровно одна запись с разделом: пустой Section не пишется (§4/omitempty).
	require.Equal(t, 1, strings.Count(out, `"section"`))
}

// mustJSON — секция снимка как сырой JSON (через повторную маршализацию).
func mustJSON(t *testing.T, snap *ContentSnapshot, section string) []byte {
	t.Helper()
	switch section {
	case "producer_types":
		b, err := json.Marshal(snap.ProducerTypes)
		require.NoError(t, err)
		return b
	}
	t.Fatalf("неизвестная секция %s", section)
	return nil
}

// topKeyIdx — позиция ключа ВЕРХНЕГО уровня (отступ 2 пробела в
// json.MarshalIndent) — отличает секции от одноимённых ключей counts.
func topKeyIdx(out, quotedKey string) int {
	return strings.Index(out, "\n  "+quotedKey+":")
}
