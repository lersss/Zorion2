// internal/handlers/studio_descriptions_test.go
// Тесты поля «описание» каталога студии (спека
// 2026-09-21-каталог-описание-товаров-и-ресурсов §7/§11): PUT description,
// третья колонка bulk, три роута /studio/api/descriptions/*, desc-поля state.
package handlers

import (
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

// --- PUT /studio/api/goods/{id}: описание ---

// TestStudioGoodPutNoDescription — поле description отсутствует → UPDATE
// описания нет (только SELECT ... FOR UPDATE + commit).
func TestStudioGoodPutNoDescription(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/goods/1", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioGoodPutDescriptionClear — пустая/пробельная строка → очистка (NULL, И3).
func TestStudioGoodPutDescriptionClear(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectExec(`UPDATE goods SET description = NULL WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/goods/1", strings.NewReader(`{"description":"   "}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioGoodPutDescriptionSet — непустое значение пишется с trim.
func TestStudioGoodPutDescriptionSet(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("resource"))
	mock.ExpectExec(`UPDATE goods SET description = \$1 WHERE id = \$2`).
		WithArgs("Металл.", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/goods/1", strings.NewReader(`{"description":"  Металл.  "}`))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioGoodPutDescriptionTooLong — 2001 руна → 400, UPDATE нет.
func TestStudioGoodPutDescriptionTooLong(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	long := strings.Repeat("я", ai.MaxDescriptionRunes+1)
	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectRollback()

	h := newFillHandlers(db)
	body, _ := json.Marshal(map[string]string{"description": long})
	req := httptest.NewRequest(http.MethodPut, "/studio/api/goods/1", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.GoodByID(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- bulk: третья колонка описания ---

// expectBulkCatalog — предварительные чтения BulkCreateGoods (категории + имена).
func expectBulkCatalog(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id, name FROM categories WHERE kind = 'good'`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(1), "Корабли"))
	mock.ExpectQuery(`SELECT name FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Сталь"))
}

// expectBulkGoodInsert — INSERT товара из bulk (с описанием) + рецепт.
func expectBulkGoodInsert(mock sqlmock.Sqlmock, name, catNorm string, catID, id, recipeID int64, desc interface{}) {
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source, description\) VALUES \(\$1, \$2, \$3, 'good', 'manual', \$4\) RETURNING id`).
		WithArgs(name, catNorm, catID, desc).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
	mock.ExpectQuery(`INSERT INTO recipes \(good_id\) VALUES \(\$1\) RETURNING id`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(recipeID))
	mock.ExpectExec(`INSERT INTO recipe_components \(recipe_id, pos, quantity\) VALUES \(\$1, 0, 1\)`).
		WithArgs(recipeID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// bulkLines — прогон одной строки bulk и возврат отчёта.
func bulkLines(t *testing.T, line string) repositoryBulkReport {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	expectBulkCatalog(mock)
	expectBulkGoodInsert(mock, "Крейсер", "крейсер", 1, 5, 50, nil)
	mock.ExpectCommit()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/bulk",
		strings.NewReader(`{"lines":["`+line+`"]}`))
	rec := httptest.NewRecorder()
	h.Goods(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var rep repositoryBulkReport
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rep))
	return rep
}

// TestStudioBulkThreeColumnsDescription — третья колонка = описание.
func TestStudioBulkThreeColumnsDescription(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	expectBulkCatalog(mock)
	expectBulkGoodInsert(mock, "Крейсер", "крейсер", 1, 5, 50, "Прочный корабль")
	mock.ExpectCommit()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/bulk",
		strings.NewReader(`{"lines":["Крейсер | Корабли | Прочный корабль"]}`))
	rec := httptest.NewRecorder()
	h.Goods(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
	var rep repositoryBulkReport
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rep))
	require.Len(t, rep.Created, 1)
}

// TestStudioBulkPipeInDescription — «|» внутри описания не режется (третья часть
// берётся целиком).
func TestStudioBulkPipeInDescription(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	expectBulkCatalog(mock)
	expectBulkGoodInsert(mock, "Крейсер", "крейсер", 1, 5, 50, "до | после")
	mock.ExpectCommit()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/bulk",
		strings.NewReader(`{"lines":["Крейсер | Корабли | до | после"]}`))
	rec := httptest.NewRecorder()
	h.Goods(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioBulkEmptyThirdColumn — пустая третья колонка → NULL (И3).
func TestStudioBulkEmptyThirdColumn(t *testing.T) {
	rep := bulkLines(t, "Крейсер | Корабли |")
	require.Len(t, rep.Created, 1)
	require.Empty(t, rep.Errors)
}

// TestStudioBulkTwoColumns — 2 колонки валидны (описание пустое).
func TestStudioBulkTwoColumns(t *testing.T) {
	rep := bulkLines(t, "Крейсер | Корабли")
	require.Len(t, rep.Created, 1)
	require.Empty(t, rep.Errors)
}

// TestStudioBulkLongDescription — описание длиннее предела → строка в errors,
// запись не создаётся.
func TestStudioBulkLongDescription(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	long := strings.Repeat("я", ai.MaxDescriptionRunes+1)
	expectStudioMutation(mock)
	expectBulkCatalog(mock)
	mock.ExpectCommit()

	h := newFillHandlers(db)
	body, _ := json.Marshal(map[string][]string{"lines": {"Крейсер | Корабли | " + long}})
	req := httptest.NewRequest(http.MethodPost, "/studio/api/goods/bulk", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.Goods(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
	var rep repositoryBulkReport
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rep))
	require.Len(t, rep.Errors, 1)
	require.Contains(t, rep.Errors[0].Reason, "описание длиннее 2000")
	require.Empty(t, rep.Created)
}

// --- POST /studio/api/descriptions/fill ---

// descSnapshotRows — снимок каталога для тестов описаний: категория 5 (good),
// товар 1 «Корабль» с пустым описанием.
func descSnapshotRows(mock sqlmock.Sqlmock, description interface{}) {
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(5), "Комплектующие", "good", nil, false))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "manual", nil, nil, time.Now(), nil, nil, description))
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
	mock.ExpectQuery(`SELECT pr.producer_type_id, pr.recipe_id, pr.rate, r.good_id FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, pr.recipe_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "rate", "good_id"}))
	mock.ExpectCommit()
}

// TestStudioDescriptionsFill400BothFields — заданы и scope, и good_ids → 400.
func TestStudioDescriptionsFill400BothFields(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/fill",
		strings.NewReader(`{"scope":"missing","good_ids":["1"]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioDescriptionsFill400UnknownID — несуществующий id → 400.
func TestStudioDescriptionsFill400UnknownID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	descSnapshotRows(mock, nil)

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/fill",
		strings.NewReader(`{"good_ids":["99"]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioDescriptionsFill400NoMissing — scope=missing, но все записи с
// описанием → 400 «нет записей без описания».
func TestStudioDescriptionsFill400NoMissing(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	descSnapshotRows(mock, "уже есть")

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/fill",
		strings.NewReader(`{"scope":"missing"}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioDescriptionsFill409Generating — занят общий флаг И6 → 409.
func TestStudioDescriptionsFill409Generating(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	descSnapshotRows(mock, nil)

	h := newFillHandlers(db)
	require.True(t, h.tryStartFill()) // идёт любой ИИ-джоб
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/fill",
		strings.NewReader(`{"scope":"missing"}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioDescriptionsFill202 — старт явного джоба (good_ids) → 202;
// goroutine завершается «ошибка ИИ» (opencode недоступен); режим descExplicit=true.
func TestStudioDescriptionsFill202(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	descSnapshotRows(mock, nil)

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/fill",
		strings.NewReader(`{"good_ids":["1"]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "true", body["started"])
	require.Equal(t, float64(1), body["total"])

	h.fillMu.Lock()
	require.True(t, h.descExplicit, "good_ids → явный режим (И4)")
	h.fillMu.Unlock()

	require.Eventually(t, func() bool {
		h.fillMu.Lock()
		defer h.fillMu.Unlock()
		return !h.descGenerating && len(h.descReport) > 0
	}, 5*time.Second, 10*time.Millisecond)
	h.fillMu.Lock()
	require.Contains(t, h.descReport[0], "ошибка ИИ")
	h.fillMu.Unlock()
}

// TestStudioDescriptionsFillScopeBatchMode — старт пакетного джоба
// (scope:"missing") → 202, режим descExplicit=false (onlyIfEmpty=true).
func TestStudioDescriptionsFillScopeBatchMode(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	descSnapshotRows(mock, nil)

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/fill",
		strings.NewReader(`{"scope":"missing"}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
	h.fillMu.Lock()
	require.False(t, h.descExplicit, "scope:missing → пакетный режим (И4)")
	h.fillMu.Unlock()

	require.Eventually(t, func() bool {
		h.fillMu.Lock()
		defer h.fillMu.Unlock()
		return !h.descGenerating && len(h.descReport) > 0
	}, 5*time.Second, 10*time.Millisecond)
}

// --- POST /studio/api/descriptions/apply ---

// setDescProposals — прямое наполнение канала описаний (тесты в том же пакете).
func setDescProposals(h *StudioHandlers, ps []DescProposalView) {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	h.descProposals = ps
}

// TestStudioDescriptionsApply409NoProposals — нет предложений → 409.
func TestStudioDescriptionsApply409NoProposals(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/apply",
		strings.NewReader(`{"accepted":[{"i":0,"text":"x"}]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioDescriptionsApply400EmptyAccepted — accepted пуст → 400.
func TestStudioDescriptionsApply400EmptyAccepted(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	setDescProposals(h, []DescProposalView{{ID: 1, Name: "Сталь", Kind: "good", Text: "x"}})
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/apply",
		strings.NewReader(`{"accepted":[]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioDescriptionsApply400BadIndex — индекс вне диапазона → 400.
func TestStudioDescriptionsApply400BadIndex(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	setDescProposals(h, []DescProposalView{{ID: 1, Name: "Сталь", Kind: "good", Text: "x"}})
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/apply",
		strings.NewReader(`{"accepted":[{"i":5,"text":"x"}]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioDescriptionsApply200 — применение → UPDATE + 200 {"applied":1},
// предложения очищены, отчёт «Применено: 1».
func TestStudioDescriptionsApply200(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT name, COALESCE\(description, ''\) FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "coalesce"}).AddRow("Сталь", ""))
	mock.ExpectExec(`UPDATE goods SET description = \$1 WHERE id = \$2`).
		WithArgs("Прочный сплав.", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	setDescProposals(h, []DescProposalView{{ID: 1, Name: "Сталь", Kind: "good", Text: "x"}})
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/apply",
		strings.NewReader(`{"accepted":[{"i":0,"text":"Прочный сплав."}]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, float64(1), body["applied"])
	h.fillMu.Lock()
	require.Empty(t, h.descProposals, "предложения очищены после apply")
	require.Contains(t, strings.Join(h.descReport, " | "), "Применено: 1")
	h.fillMu.Unlock()
}

// TestStudioDescriptionsApplyGuardAlreadyFilled — пакетный режим (И4,
// onlyIfEmpty=true): запись с непустым описанием не перезаписывается —
// строка guard'а, UPDATE нет, applied=0.
func TestStudioDescriptionsApplyGuardAlreadyFilled(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT name, COALESCE\(description, ''\) FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "coalesce"}).AddRow("Сталь", "уже есть"))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	setDescProposals(h, []DescProposalView{{ID: 1, Name: "Сталь", Kind: "good", Text: "x"}})
	// descExplicit по умолчанию false — пакетный режим
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/apply",
		strings.NewReader(`{"accepted":[{"i":0,"text":"Новое."}]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, float64(0), body["applied"])
	h.fillMu.Lock()
	require.Contains(t, strings.Join(h.descReport, " | "), "описание уже заполнено")
	h.fillMu.Unlock()
}

// TestStudioDescriptionsApplyExplicitOverwrite — явный режим (И4,
// onlyIfEmpty=false): запись с непустым описанием перезаписывается — UPDATE
// выполняется, applied=1, строки guard'а нет.
func TestStudioDescriptionsApplyExplicitOverwrite(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT name, COALESCE\(description, ''\) FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "coalesce"}).AddRow("Сталь", "старое"))
	mock.ExpectExec(`UPDATE goods SET description = \$1 WHERE id = \$2`).
		WithArgs("Новое.", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newFillHandlers(db)
	setDescProposals(h, []DescProposalView{{ID: 1, Name: "Сталь", Kind: "good", Text: "x"}})
	h.fillMu.Lock()
	h.descExplicit = true // как после fill {good_ids:[1]}
	h.fillMu.Unlock()
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/apply",
		strings.NewReader(`{"accepted":[{"i":0,"text":"Новое."}]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, float64(1), body["applied"])
	h.fillMu.Lock()
	require.NotContains(t, strings.Join(h.descReport, " | "), "описание уже заполнено")
	require.Contains(t, strings.Join(h.descReport, " | "), "Применено: 1")
	h.fillMu.Unlock()
}

// TestStudioDescriptionsApplyEmptyText — пункт с пустым текстом не применяется,
// даёт строку «пустой текст — пропущено».
func TestStudioDescriptionsApplyEmptyText(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectCommit()

	h := newFillHandlers(db)
	setDescProposals(h, []DescProposalView{{ID: 1, Name: "Сталь", Kind: "good", Text: "x"}})
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/apply",
		strings.NewReader(`{"accepted":[{"i":0,"text":"   "}]}`))
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
	h.fillMu.Lock()
	require.Contains(t, strings.Join(h.descReport, " | "), "пустой текст")
	h.fillMu.Unlock()
}

// --- POST /studio/api/descriptions/cancel ---

// TestStudioDescriptionsCancelRunning — идущий джоб: ставится флаг остановки,
// 200 {cancelled:true, done, total}; предложения сохраняются (И7), discarded=false.
func TestStudioDescriptionsCancelRunning(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	require.True(t, h.tryStartDesc(5, false))
	setDescProposals(h, []DescProposalView{{ID: 1, Name: "Сталь", Kind: "good", Text: "x"}})
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/cancel", nil)
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "true", body["cancelled"])
	require.Equal(t, "false", body["discarded"])
	require.Equal(t, float64(5), body["total"])
	require.True(t, h.descCancelled())
	require.Len(t, h.descProposals, 1, "при идущем прогоне предложения сохраняются (И7)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioDescriptionsCancelIdle — джоб не идёт → отказ от набора: предложения
// выбрасываются (набор пуст, попап не всплывает после перезагрузки), discarded=true.
func TestStudioDescriptionsCancelIdle(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newFillHandlers(db)
	setDescProposals(h, []DescProposalView{{ID: 1, Name: "Сталь", Kind: "good", Text: "x"}})
	h.descTotal = 1
	h.descDone = 1
	req := httptest.NewRequest(http.MethodPost, "/studio/api/descriptions/cancel", nil)
	rec := httptest.NewRecorder()
	h.Descriptions(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "false", body["cancelled"])
	require.Equal(t, "true", body["discarded"])
	require.Empty(t, h.descProposals, "набор предложений отброшен")
	require.Zero(t, h.descTotal)
	require.Zero(t, h.descDone)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- state: desc-поля ---

// TestStudioStateDescFields — StateView несёт desc_generating/desc_report/
// desc_proposals/desc_total/desc_done (спека §7.2).
func TestStudioStateDescFields(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	descSnapshotRows(mock, nil)

	h := newFillHandlers(db)
	require.True(t, h.tryStartDesc(3, false))
	h.addDescProgress([]DescProposalView{{ID: 1, Name: "Корабль", Kind: "good", Text: "текст"}}, 1)

	req := httptest.NewRequest(http.MethodGet, "/studio/api/state", nil)
	rec := httptest.NewRecorder()
	h.State(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var view StateView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.True(t, view.DescGenerating)
	require.True(t, view.Generating, "generating = любой ИИ-джоб (И6)")
	require.Len(t, view.DescProposals, 1)
	require.Equal(t, "Корабль", view.DescProposals[0].Name)
	require.Equal(t, 3, view.DescTotal)
	require.Equal(t, 1, view.DescDone)
}
