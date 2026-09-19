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
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}).
			AddRow(int64(1), "Сталь", int64(1), "good", "approved", "manual", nil, nil, time.Now()).
			AddRow(int64(2), "Железо Fe", int64(7), "resource", "approved", "palette", nil, nil, time.Now()).
			AddRow(int64(3), "Забанен", int64(1), "good", "banned", "manual", nil, time.Now(), time.Now()))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
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
	require.Len(t, view.Banned, 1, "забаненный — в banned")
	require.Len(t, view.Unused, 1, "Сталь: in-degree 0 (из неё ничего не делают) — unused")
	require.Equal(t, "Сталь", view.Unused[0].Name)
	require.False(t, view.Generating)
}

// TestStudioDeleteGoodClearedLinks — DELETE /studio/api/goods/1:
// ответ {deleted, cleared_links}.
func TestStudioDeleteGoodClearedLinks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE goods_slots SET component_id = NULL, reason = '' WHERE component_id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM goods WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodDelete, "/studio/api/goods/1", nil)
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, float64(1), body["deleted"])
	require.Equal(t, float64(1), body["cleared_links"])
}

// TestStudioGoodStatus — POST /studio/api/goods/1/status {status: "banned"}.
func TestStudioGoodStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE goods SET status = 'banned', banned_at = NOW\(\) WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/1/status", strings.NewReader(`{"status":"banned"}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioGoodTier — PUT /studio/api/goods/1/tier {tier: 3}.
func TestStudioGoodTier(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE goods SET tier_override = \$1 WHERE id = \$2`).
		WithArgs(3, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPut, "/studio/api/goods/1/tier", strings.NewReader(`{"tier":3}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
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
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, status, source\) VALUES \(\$1, \$2, \$3, 'good', 'draft', 'manual'\) RETURNING id`).
		WithArgs("Крейсер", "крейсер", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectExec(`INSERT INTO goods_slots \(good_id, pos, quantity\) VALUES \(\$1, 0, 1\)`).
		WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
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

// TestStudioSlotCycle409 — PUT слота с циклом → 409.
func TestStudioSlotCycle409(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods_slots WHERE good_id = \$1 AND pos = \$2\)`).
		WithArgs(int64(2), 0).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}).
			AddRow(int64(1), "A", int64(1), "good", "draft", "manual", nil, nil, time.Now()).
			AddRow(int64(2), "B", int64(1), "good", "draft", "manual", nil, nil, time.Now()))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPut, "/studio/api/goods/2/slots/0", strings.NewReader(`{"good_id":1}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioSlotResource403 — слот ресурсу → 403.
func TestStudioSlotResource403(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("resource"))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPut, "/studio/api/goods/3/slots/0", strings.NewReader(`{"good_id":1}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
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

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
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

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
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
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, status, source\) VALUES \(\$1, \$2, \$3, 'good', 'draft', 'manual'\) RETURNING id`).
		WithArgs("Крейсер", "крейсер", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectExec(`INSERT INTO goods_slots \(good_id, pos, quantity\) VALUES \(\$1, 0, 1\)`).
		WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
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
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "draft", "manual", nil, nil, time.Now()))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, nil, 1, nil, false))
	mock.ExpectCommit()
}

// newFillHandlers — хендлеры с недостижимым ИИ-клиентом (fill не тестируем
// на успех ИИ — «Ошибка ИИ» детерминирована).
func newFillHandlers(db *sql.DB) *StudioHandlers {
	return NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
}

// TestStudioFillNotFound404 — fill несуществующего товара → 404.
func TestStudioFillNotFound404(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}))
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}))
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
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "draft", "manual", nil, nil, time.Now()))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))
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
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "draft", "manual", nil, nil, time.Now()))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, nil, 1, nil, false))
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, status, source\) VALUES \(\$1, \$2, \$3, 'good', 'draft', 'ai'\) RETURNING id`).
		WithArgs("Сталь", "сталь", int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectExec(`UPDATE goods_slots SET component_id = \$1, reason = \$2 WHERE good_id = \$3 AND pos = \$4`).
		WithArgs(int64(10), "", int64(1), 0).
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
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}).
			AddRow(int64(2), "Другой", int64(5), "good", "draft", "manual", nil, nil, time.Now()))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
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
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "draft", "manual", nil, nil, time.Now()))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, nil, 1, nil, false))
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