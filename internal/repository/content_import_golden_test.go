// internal/repository/content_import_golden_test.go
// Golden-тест сухого прогона импорта (эквивалентность при распиле
// content_import.go): минимальный снимок → dry_run на пустой цели, дифф
// побайтово равен эталону. sqlmock: BEGIN + advisory lock, 10 чтений
// loadImportState пустыми наборами, ROLLBACK (dry_run пишет ничего).
package repository

import (
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/contentio"
)

// minimalGoldenSnapshot — валидный минимальный снимок: категория + товар +
// постройка + предмет + эффект, без рецептов/листьев (все ссылки резолвятся).
func minimalGoldenSnapshot() *contentio.Snapshot {
	return &contentio.Snapshot{
		SchemaVersion: contentio.SupportedSchemaVersion,
		Categories:    []contentio.Category{{Name: "Металлы", Kind: "good", IsSystem: false}},
		Goods: []contentio.Good{{
			Name: "Тест-слиток", Kind: "good", Category: "Металлы",
			Source: "manual", Code: "g_0001",
		}},
		ProducerTypes: []contentio.ProducerType{{Name: "Тест-завод", Kind: "factory", Code: "p_0001"}},
		Items:         []contentio.Item{{Name: "Тест-модуль", SlotType: "модуль", Code: "i_0001"}},
		EffectTypes:   []contentio.EffectType{{Name: "Тест-эффект", Impact: "голод", Code: "e_0001"}},
	}
}

// expectEmptyImportState — 10 чтений loadImportState пустыми наборами.
func expectEmptyImportState(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "kind", "code", "is_system"}))
	mock.ExpectQuery(`FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "category_id", "kind", "source", "description", "volume", "weight", "props", "code"}))
	mock.ExpectQuery(`FROM recipes ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "good_id", "complexity"}))
	mock.ExpectQuery(`FROM recipe_components ORDER BY recipe_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id", "pos", "component_id", "quantity", "reason", "allow_resource"}))
	mock.ExpectQuery(`FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "kind", "category_id", "parent_id", "race_family", "race", "output", "input", "params", "hidden", "section", "code"}))
	mock.ExpectQuery(`FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "slot_type", "unlocks", "params", "code"}))
	mock.ExpectQuery(`FROM producer_slots ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id", "category_id", "race_family", "race", "hidden"}))
	mock.ExpectQuery(`FROM producer_recipes pr JOIN recipes`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "good_id", "rate"}))
	mock.ExpectQuery(`FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}))
	mock.ExpectQuery(`FROM effect_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "params", "code"}))
}

// TestContentImportGoldenDryRun — эталон диффа dry_run на пустой цели.
func TestContentImportGoldenDryRun(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	expectEmptyImportState(mock)
	mock.ExpectRollback()

	res, err := NewGoodsRepository(db).ImportContent(minimalGoldenSnapshot(), true)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Diff)
	require.Equal(t, "managed", res.Mode)
	require.True(t, res.DryRun)

	diffJSON, err := json.Marshal(res.Diff)
	require.NoError(t, err)
	const golden = `{"mode":"managed","create":{"categories":1,"effect_types":1,"goods":1,"items":1,"producer_items":0,"producer_recipes":0,"producer_slots":0,"producer_types":1},"update":{},"delete":[],"blocked":[],"unmatched":null,"remap":[]}`
	require.JSONEq(t, golden, string(diffJSON))
	require.NoError(t, mock.ExpectationsWereMet())
}
