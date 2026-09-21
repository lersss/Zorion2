// internal/handlers/studio_handlers_test.go
// Тесты хендлеров студии товаров (спека переноса-студии-товаров-iterA §7/§11):
// контракт API — state, DELETE с cleared_links, статусы, тир, bulk-отчёт,
// ошибки (403 слот ресурсу / системная категория, 409 цикл).
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/ai"
	"zorion/internal/races"
)

// expectStudioMutation — Begin + advisory lock (как в репозитории).
func expectStudioMutation(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// TestStudioState — GET /studio/api/state: полное состояние (категории,
// товары со слотами, banned, unused, warnings).
func TestStudioState(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(1), "Корабли", "good", nil, false).
			AddRow(int64(7), "Минералы", "resource", "mineral", true))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "Сталь", int64(1), "good", "manual", nil, nil, time.Now(), 1.0, 1.0, "Прочный сплав.").
			AddRow(int64(2), "Железо Fe", int64(7), "resource", "palette", nil, nil, time.Now(), 1.0, 1.0, nil).
			AddRow(int64(3), "Топливо", int64(1), "good", "manual", nil, nil, time.Now(), 1.0, 1.0, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))
	mock.ExpectQuery(`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "hidden", "created_at"}).
			AddRow(int64(1), "Лаборатория исследовательская", "items", nil, nil, nil, nil, []byte(`{"items":["Чертёж"]}`), []byte(`{}`), []byte(`{}`), false, time.Now()))
	mock.ExpectQuery(`SELECT id, name, slot_type, unlocks, params, created_at FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "unlocks", "params", "created_at"}).
			AddRow(int64(1), "Чертёж", "чертёж", []byte(`[]`), []byte(`{}`), time.Now()))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}).
			AddRow(int64(1), int64(1), nil))
	mock.ExpectQuery(`SELECT id, parent_id, category_id, race_family, race, hidden, created_at FROM producer_slots ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}))
	mock.ExpectQuery(`SELECT pr.producer_type_id, pr.recipe_id, r.good_id FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, pr.recipe_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "good_id"}))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodGet, "/studio/api/state", nil)
	rec := httptest.NewRecorder()
	h.State(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var view StateView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.Len(t, view.Categories, 2)
	require.Equal(t, "mineral", view.Categories[1].Code)
	require.True(t, view.Categories[1].IsSystem)
	require.Len(t, view.Goods, 3)
	require.Equal(t, int64(1), view.Goods[0].CategoryID)
	require.Len(t, view.Goods[0].Recipe, 1)
	require.Equal(t, "Железо Fe", view.Goods[0].Recipe[0].Name)
	require.Len(t, view.Unused, 2, "Сталь и Топливо: in-degree 0 — unused (списка «бан» нет)")
	require.Equal(t, "Сталь", view.Unused[0].Name)
	require.False(t, view.Generating)
}

// TestStudioGoodBranchesCount — GET /studio/api/goods/{id}/branches-count:
// предпроверка числа веток поселений с этим товаром-выходом (§8/О3 итерации 2).
func TestStudioGoodBranchesCount(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM settlement_branches b`).
		WithArgs(int64(378)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodGet, "/studio/api/goods/378/branches-count", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]int
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 4, body["count"])
}

// TestStudioDeleteGoodClearedLinks — DELETE /studio/api/goods/1:
// ответ {deleted, cleared_links, deposits} (deposits — число залежей, T14).
func TestStudioDeleteGoodClearedLinks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM deposits WHERE good_id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`FROM settlement_branches b`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`UPDATE recipe_components SET component_id = NULL, reason = '' WHERE component_id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM goods WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodDelete, "/studio/api/goods/1", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, float64(1), body["deleted"])
	require.Equal(t, float64(1), body["cleared_links"])
	require.Equal(t, float64(2), body["deposits"], "число залежей ресурса в ответе удаления (T14)")
	require.Equal(t, float64(1), body["branches"], "число веток с этим товаром-выходом (О3 итерации 2)")
}

// TestStudioGoodDepositsCount — GET /studio/api/goods/{id}/deposits-count:
// предпроверка числа залежей ресурса перед удалением (§3.3/T14).
func TestStudioGoodDepositsCount(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM deposits WHERE good_id = \$1`).
		WithArgs(int64(359)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodGet, "/studio/api/goods/359/deposits-count", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]int
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 3, body["count"])
}

// TestStudioRecipeComplexity — PUT /studio/api/recipes/1 {complexity: 3}
// (спека 2026-09-21-рецепт-сущность §5: тир-роут у goods снят).
func TestStudioRecipeComplexity(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE recipes SET complexity = \$1 WHERE id = \$2`).
		WithArgs(3, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPut, "/studio/api/recipes/1", strings.NewReader(`{"complexity":3}`))
	rec := httptest.NewRecorder()
	h.RecipeByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioGoodSlotsRouteGone — старый роут слотов у goods снят → 404
// (одна модель — один адрес, спека §5).
func TestStudioGoodSlotsRouteGone(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPut, "/studio/api/goods/1/slots/0", strings.NewReader(`{"good_id":2}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioBulk — POST /studio/api/goods/bulk: частичный успех, отчёт.
func TestStudioBulk(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT id, name FROM categories WHERE kind = 'good'`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(1), "Корабли"))
	mock.ExpectQuery(`SELECT name FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Сталь"))
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source, description\) VALUES \(\$1, \$2, \$3, 'good', 'manual', \$4\) RETURNING id`).
		WithArgs("Крейсер", "крейсер", int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectQuery(`INSERT INTO recipes \(good_id\) VALUES \(\$1\) RETURNING id`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(50)))
	mock.ExpectExec(`INSERT INTO recipe_components \(recipe_id, pos, quantity\) VALUES \(\$1, 0, 1\)`).
		WithArgs(int64(50)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/bulk",
		strings.NewReader(`{"lines":["Крейсер | Корабли","Сталь | Корабли","Без разделителя"]}`))
	rec := httptest.NewRecorder()
	h.Goods(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var rep repositoryBulkReport
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rep))
	require.Len(t, rep.Created, 1)
	require.Equal(t, "Крейсер", rep.Created[0].Name)
	require.Len(t, rep.Skipped, 1, "дубликат «Сталь» — пропуск")
	require.Len(t, rep.Errors, 1, "строка без разделителя — ошибка")
}

// TestStudioRecipeComponentCycle409 — PUT компонента рецепта с циклом → 409.
func TestStudioRecipeComponentCycle409(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT good_id FROM recipes WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"good_id"}).AddRow(int64(2)))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipe_components WHERE recipe_id = \$1 AND pos = \$2\)`).
		WithArgs(int64(2), 0).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "A", int64(1), "good", "manual", nil, nil, time.Now(), nil, nil, nil).
			AddRow(int64(2), "B", int64(1), "good", "manual", nil, nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPut, "/studio/api/recipes/2/components/0", strings.NewReader(`{"good_id":1}`))
	rec := httptest.NewRecorder()
	h.RecipeByID(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioRecipeAddComponent404 — рецепта нет → 404.
func TestStudioRecipeAddComponent404(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT good_id FROM recipes WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"good_id"}))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/recipes/3/components", nil)
	rec := httptest.NewRecorder()
	h.RecipeByID(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioCategorySystem403 — переименование системной категории → 403.
func TestStudioCategorySystem403(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind, is_system FROM categories WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "is_system"}).AddRow("resource", true))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPut, "/studio/api/categories/7", strings.NewReader(`{"name":"Новое имя"}`))
	rec := httptest.NewRecorder()
	h.CategoryByID(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioGoodNotFound404 — DELETE несуществующего товара → 404.
func TestStudioGoodNotFound404(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodDelete, "/studio/api/goods/99", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioBulkRoute — регрессия ревью iterA: POST /studio/api/goods/bulk
// через реальный роутер (паттерны как в cmd/server/main.go) попадает в
// Goods → bulk-отчёт, а не в GoodByID (parseID("bulk") → 404). Без роута
// /studio/api/goods/bulk запрос уходит в subtree /studio/api/goods/.
func TestStudioBulkRoute(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT id, name FROM categories WHERE kind = 'good'`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(1), "Корабли"))
	mock.ExpectQuery(`SELECT name FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Сталь"))
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source, description\) VALUES \(\$1, \$2, \$3, 'good', 'manual', \$4\) RETURNING id`).
		WithArgs("Крейсер", "крейсер", int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectQuery(`INSERT INTO recipes \(good_id\) VALUES \(\$1\) RETURNING id`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(50)))
	mock.ExpectExec(`INSERT INTO recipe_components \(recipe_id, pos, quantity\) VALUES \(\$1, 0, 1\)`).
		WithArgs(int64(50)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	// паттерны роутов как в cmd/server/main.go (без auth — ортогонально роутингу)
	mux := http.NewServeMux()
	mux.HandleFunc("/studio/api/goods", h.Goods)
	mux.HandleFunc("/studio/api/goods/bulk", h.Goods)
	mux.HandleFunc("/studio/api/goods/", h.GoodByID)

	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/bulk",
		strings.NewReader(`{"lines":["Крейсер | Корабли"]}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var rep repositoryBulkReport
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rep))
	require.Len(t, rep.Created, 1)
	require.Equal(t, "Крейсер", rep.Created[0].Name)
}

// repositoryBulkReport — проекция repository.BulkReport для проверки JSON.
type repositoryBulkReport struct {
	Created []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"created"`
	Skipped []struct {
		Line   int    `json:"line"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"skipped"`
	Errors []struct {
		Line   int    `json:"line"`
		Reason string `json:"reason"`
	} `json:"errors"`
}

// --- fill: «заполнить комплектующие» (спека iterC §5/§11) ---

// fillSnapshotRows — снимок каталога для fill: категория 5 (good), товар 1
// «Корабль» с одним пустым слотом (pos 0).
func fillSnapshotRows(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(5), "Комплектующие", "good", nil, false))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "manual", nil, nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, nil, 1, nil, false))
	mock.ExpectQuery(`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "hidden", "output", "input", "params", "status", "created_at"}))
	mock.ExpectQuery(`SELECT id, name, slot_type, unlocks, params, created_at FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "status", "unlocks", "params", "created_at"}))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}))
	mock.ExpectQuery(`SELECT id, parent_id, category_id, race_family, race, hidden, created_at FROM producer_slots ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}))
	mock.ExpectQuery(`SELECT pr.producer_type_id, pr.recipe_id, r.good_id FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, pr.recipe_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "good_id"}))
	mock.ExpectCommit()
}

// newFillHandlers — хендлеры с недостижимым ИИ-клиентом (fill не тестируем
// на успех ИИ — «Ошибка ИИ» детерминирована).
func newFillHandlers(db *sql.DB) *StudioHandlers {
	return NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
}

// TestStudioFillNotFound404 — fill несуществующего товара → 404.
func TestStudioFillNotFound404(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}))
	mock.ExpectQuery(`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "hidden", "output", "input", "params", "status", "created_at"}))
	mock.ExpectQuery(`SELECT id, name, slot_type, unlocks, params, created_at FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "status", "unlocks", "params", "created_at"}))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}))
	mock.ExpectQuery(`SELECT id, parent_id, category_id, race_family, race, hidden, created_at FROM producer_slots ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}))
	mock.ExpectQuery(`SELECT pr.producer_type_id, pr.recipe_id, r.good_id FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, pr.recipe_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "good_id"}))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/99/fill", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioFillNoEmptySlots400 — fill товара без пустых слотов → 400.
func TestStudioFillNoEmptySlots400(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(5), "Комплектующие", "good", nil, false))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "manual", nil, nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))
	mock.ExpectQuery(`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "hidden", "output", "input", "params", "status", "created_at"}))
	mock.ExpectQuery(`SELECT id, name, slot_type, unlocks, params, created_at FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "status", "unlocks", "params", "created_at"}))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}))
	mock.ExpectQuery(`SELECT id, parent_id, category_id, race_family, race, hidden, created_at FROM producer_slots ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}))
	mock.ExpectQuery(`SELECT pr.producer_type_id, pr.recipe_id, r.good_id FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, pr.recipe_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "good_id"}))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioFillAccepted202 — fill → 202; goroutine завершается «Ошибка ИИ»
// (opencode недоступен — честная ошибка); state: generating=false, report
// непуст, proposals пусты.
func TestStudioFillAccepted202(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	fillSnapshotRows(mock)

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	// дождаться завершения goroutine (недостижимый URL → «Ошибка ИИ»)
	require.Eventually(t, func() bool {
		h.fillMu.Lock()
		defer h.fillMu.Unlock()
		return !h.fillGenerating && len(h.fillReport) > 0
	}, 5*time.Second, 10*time.Millisecond)
	h.fillMu.Lock()
	require.Contains(t, h.fillReport[0], "Ошибка ИИ")
	require.Empty(t, h.fillProposals)
	h.fillMu.Unlock()
}

// TestStudioFillAlreadyGenerating409 — повторный fill во время генерации →
// 409 (TryStart).
func TestStudioFillAlreadyGenerating409(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	fillSnapshotRows(mock)

	h := newFillHandlers(db)
	require.True(t, h.tryStartFill()) // генерация уже идёт
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioFillApply409NoProposals — apply без предложений → 409.
func TestStudioFillApply409NoProposals(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill/apply",
		strings.NewReader(`{"accepted":[{"i":0}]}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioFillApply409Generating — apply во время генерации → 409.
func TestStudioFillApply409Generating(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	require.True(t, h.tryStartFill())
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill/apply",
		strings.NewReader(`{"accepted":[{"i":0}]}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioFillApply400EmptyAccepted — accepted пуст → 400.
func TestStudioFillApply400EmptyAccepted(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	h.setProposals([]ProposalView{{Slot: 0, Name: "Сталь", Kind: "new", CategoryID: 5, CategoryValid: true}}, "1")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill/apply",
		strings.NewReader(`{"accepted":[]}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioFillApply400BadIndex — индекс вне диапазона → 400.
func TestStudioFillApply400BadIndex(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	h.setProposals([]ProposalView{{Slot: 0, Name: "Сталь", Kind: "new", CategoryID: 5, CategoryValid: true}}, "1")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill/apply",
		strings.NewReader(`{"accepted":[{"i":5}]}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioFillApply400NoCategory — kind=new без category_id → 400.
func TestStudioFillApply400NoCategory(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	h.setProposals([]ProposalView{{Slot: 0, Name: "Сталь", Kind: "new"}}, "1")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill/apply",
		strings.NewReader(`{"accepted":[{"i":0}]}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioFillApply200 — apply успешен: repo.ApplyProposals (INSERT нового
// товара + UPDATE слота) → 200 {"applied": 1}, отчёт «Применено: 1»,
// proposals сброшены.
func TestStudioFillApply200(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(5), "Комплектующие", "good", nil, false))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "manual", int64(100), nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, nil, 1, nil, false))
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source, description\) VALUES \(\$1, \$2, \$3, 'good', 'ai', \$4\) RETURNING id`).
		WithArgs("Сталь", "сталь", int64(5), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectExec(`INSERT INTO recipes \(good_id\) VALUES \(\$1\)`).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE recipe_components SET component_id = \$1, reason = \$2 WHERE recipe_id = \$3 AND pos = \$4`).
		WithArgs(int64(10), "", int64(100), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	h.setProposals([]ProposalView{{Slot: 0, Name: "Сталь", Kind: "new", CategoryID: 5, CategoryValid: true}}, "1")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill/apply",
		strings.NewReader(`{"accepted":[{"i":0,"category_id":5}]}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, float64(1), body["applied"])
	h.fillMu.Lock()
	require.Empty(t, h.fillProposals, "proposals сброшены после apply")
	require.Len(t, h.fillReport, 1)
	require.Contains(t, h.fillReport[0], "Применено: 1")
	h.fillMu.Unlock()
}

// TestStudioFillApplyGoodGoneM2 — товар удалён между фазами → 200 + отчёт
// «товар не найден» (М2), не 404.
func TestStudioFillApplyGoodGoneM2(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(5), "Комплектующие", "good", nil, false))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(2), "Другой", int64(5), "good", "manual", nil, nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}))
	mock.ExpectRollback()

	h := newFillHandlers(db)
	h.setProposals([]ProposalView{{Slot: 0, Name: "Сталь", Kind: "new", CategoryID: 5, CategoryValid: true}}, "1")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill/apply",
		strings.NewReader(`{"accepted":[{"i":0,"category_id":5}]}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, float64(0), body["applied"])
	h.fillMu.Lock()
	require.Contains(t, h.fillReport[0], "товар не найден")
	h.fillMu.Unlock()
}

// TestStudioFillCancel200 — cancel → 200, proposals сброшены.
func TestStudioFillCancel200(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	fillSnapshotRows(mock)

	h := newFillHandlers(db)
	h.setProposals([]ProposalView{{Slot: 0, Name: "Сталь", Kind: "new", CategoryID: 5, CategoryValid: true}}, "1")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill/cancel", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
	h.fillMu.Lock()
	require.Empty(t, h.fillProposals, "proposals сброшены после cancel")
	require.Empty(t, h.fillProposalsGoodID)
	h.fillMu.Unlock()
}

// TestStudioFillCancel409Generating — cancel во время генерации → 409 (М5).
func TestStudioFillCancel409Generating(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	require.True(t, h.tryStartFill())
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/fill/cancel", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioStateFillFields — StateView несёт model/generating/report/
// proposals/proposals_good_id (спека iterC §5.3, аддитивно к iterA §7).
func TestStudioStateFillFields(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(5), "Комплектующие", "good", nil, false))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "manual", nil, nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, nil, 1, nil, false))
	mock.ExpectQuery(`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "hidden", "output", "input", "params", "status", "created_at"}))
	mock.ExpectQuery(`SELECT id, name, slot_type, unlocks, params, created_at FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "status", "unlocks", "params", "created_at"}))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}))
	mock.ExpectQuery(`SELECT id, parent_id, category_id, race_family, race, hidden, created_at FROM producer_slots ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}))
	mock.ExpectQuery(`SELECT pr.producer_type_id, pr.recipe_id, r.good_id FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, pr.recipe_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "good_id"}))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	h.setProposals([]ProposalView{{Slot: 0, Name: "Сталь", Kind: "new", CategoryID: 5, CategoryValid: true}}, "1")
	h.finishFill([]string{"дроп: цикл"})

	req := httptest.NewRequest(http.MethodGet, "/studio/api/state", nil)
	rec := httptest.NewRecorder()
	h.State(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var view StateView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.Equal(t, "test-model", view.Model)
	require.False(t, view.Generating)
	require.Equal(t, []string{"дроп: цикл"}, view.Report)
	require.Len(t, view.Proposals, 1)
	require.Equal(t, "Сталь", view.Proposals[0].Name)
	require.Equal(t, "1", view.ProposalsGoodID)
}

// --- уровни расовости (дерево построек, спека 2026-09-21 §3) ---

// TestStudioRaces — GET /studio/api/races: семейства F0–F9 + robotic и расы
// (id, name, family) из каталога (Go-конфиги, единый источник).
func TestStudioRaces(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	require.NoError(t, races.LoadLore("../../config/race_lore.json"))

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodGet, "/studio/api/races", nil)
	rec := httptest.NewRecorder()
	h.Races(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var view RacesView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.Len(t, view.Families, 11, "F0–F9 + robotic")
	require.Equal(t, "F0", view.Families[0].ID)
	require.Equal(t, "Люди", view.Families[0].Name)
	require.Equal(t, "robotic", view.Families[10].ID)
	require.Equal(t, "Роботы", view.Families[10].Name)
	require.Len(t, view.Races, 60, "60 рас каталога")
	for _, rc := range view.Races {
		require.NotEmpty(t, rc.Family, "у каждой расы есть семейство из лора")
	}
	// Люди — единственная раса F0; водные F1 — ровно расы 2–4.
	famByRace := map[string]string{}
	for _, rc := range view.Races {
		famByRace[rc.ID] = rc.Family
	}
	require.Equal(t, "F0", famByRace["humans"], "люди вынесены в F0")
	require.Equal(t, "F1", famByRace["oceanids"])
	require.Equal(t, "F1", famByRace["deep_dwellers"])
	require.Equal(t, "F1", famByRace["coastal"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestValidateRaceFamily — инвариант §1.2 п.6: раса задана → семейство задано
// и соответствует каталогу (config/race_lore.json).
func TestValidateRaceFamily(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	require.NoError(t, races.LoadLore("../../config/race_lore.json"))

	// раса не задана — ок (универсальный).
	require.NoError(t, validateRaceFamily(nil, nil))
	require.NoError(t, validateRaceFamily(strp(""), strp("F1")))
	// раса задана, семейство нет — 400.
	err := validateRaceFamily(strp("humans"), nil)
	require.Error(t, err)
	// раса не в каталоге — 400.
	err = validateRaceFamily(strp("nope"), strp("F1"))
	require.Error(t, err)
	// семейство не соответствует расе — 400.
	err = validateRaceFamily(strp("humans"), strp("F2"))
	require.Error(t, err)
	// ок: humans → F0; робот → robotic.
	require.NoError(t, validateRaceFamily(strp("humans"), strp("F0")))
	require.NoError(t, validateRaceFamily(strp("archivists"), strp("robotic")))
}

func strp(s string) *string { return &s }
