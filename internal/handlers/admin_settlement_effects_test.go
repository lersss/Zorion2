// internal/handlers/admin_settlement_effects_test.go
// Тесты этапа 3 (T9/§6 F9): диспетчер /admin/settlements/ (…/branches →
// AddBranch, …/effects → задать нагрузку) и админ-ручка «задать нагрузку
// эффекта вручную» — валидация 422, эффекта нет → 404, гейты мутаций 409.
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator"
)

// Диспетчер: суффикс …/effects уходит в ручку нагрузки (валидация до БД),
// …/branches — в прежний AddBranch (recipe_id обязателен → 422 до БД).
func TestSettlementRouteDispatcher(t *testing.T) {
	h := &AdminHandlers{}

	rec := httptest.NewRecorder()
	h.HandleSettlementRoute(rec, branchReq(http.MethodPost, "/admin/settlements/s1/effects", `{}`))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "эффекты: нет effect_type_id → 422")

	rec = httptest.NewRecorder()
	h.HandleSettlementRoute(rec, branchReq(http.MethodPost, "/admin/settlements/s1/branches", `{}`))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "ветки: нет recipe_id → 422 (старое поведение)")
}

// Хвостовой слэш у эффект-пути (…/{id}/effects/) — осмысленный отказ, а не
// молчаливый уход в ветки (мелочь ревью этапа 3): ответ отличает эту опечатку
// от «settlement id required» веток.
func TestSettlementRouteTrailingSlashRefused(t *testing.T) {
	h := &AdminHandlers{}

	rec := httptest.NewRecorder()
	h.HandleSettlementRoute(rec, branchReq(http.MethodPost, "/admin/settlements/s1/effects/", `{}`))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "лишний слэш",
		"хвостовой слэш должен получить осмысленный отказ, а не ответ веток")
}

// Валидация до БД: отрицательная нагрузка, нет effect_type_id, не-POST, чужой путь.
func TestSetSettlementEffectLoadValidation(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"negative load", http.MethodPost, "/admin/settlements/s1/effects", `{"effect_type_id":1,"load":-1}`, http.StatusUnprocessableEntity},
		{"missing effect_type_id", http.MethodPost, "/admin/settlements/s1/effects", `{"load":5}`, http.StatusUnprocessableEntity},
		{"missing load", http.MethodPost, "/admin/settlements/s1/effects", `{"effect_type_id":1}`, http.StatusUnprocessableEntity},
		{"wrong method", http.MethodGet, "/admin/settlements/s1/effects", ``, http.StatusMethodNotAllowed},
		{"wrong path", http.MethodPost, "/admin/settlements/s1", `{"effect_type_id":1,"load":5}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.SetSettlementEffectLoad(rec, branchReq(c.method, c.path, c.body))
		require.Equal(t, c.want, rec.Code, c.name)
	}
}

// Успех: advisory-лок + UPDATE, ответ {settlement_id, effect_type_id, load}.
func TestSetSettlementEffectLoadHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock`).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE active_effects SET load`).
		WithArgs(42.0, "s1", int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.SetSettlementEffectLoad(rec, branchReq(http.MethodPost, "/admin/settlements/s1/effects", `{"effect_type_id":7,"load":42}`))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Contains(t, rec.Body.String(), `"settlement_id":"s1"`)
	require.Contains(t, rec.Body.String(), `"effect_type_id":7`)
	require.Contains(t, rec.Body.String(), `"load":42`)
}

// Эффекта нет → UPDATE 0 строк → 404 (это отдаёт репозиторий).
func TestSetSettlementEffectLoadNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock`).WithArgs("s1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE active_effects SET load`).
		WithArgs(1.0, "s1", int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.SetSettlementEffectLoad(rec, branchReq(http.MethodPost, "/admin/settlements/s1/effects", `{"effect_type_id":9,"load":1}`))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Гейты мутаций вселенной: пакман → 409; чужой мутатор занят → 409.
func TestSetSettlementEffectLoadGates(t *testing.T) {
	startJob(t, generator.JobPacman)
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.SetSettlementEffectLoad(rec, branchReq(http.MethodPost, "/admin/settlements/s1/effects", `{"effect_type_id":1,"load":5}`))
	require.Equal(t, http.StatusConflict, rec.Code)

	universeMutationMu.Lock()
	defer universeMutationMu.Unlock()
	rec = httptest.NewRecorder()
	h.SetSettlementEffectLoad(rec, branchReq(http.MethodPost, "/admin/settlements/s1/effects", `{"effect_type_id":1,"load":5}`))
	require.Equal(t, http.StatusConflict, rec.Code)
}

// Проверка, что ручка не принимает лишние пробелы/дробный id (регресс-страж
// контракта парсинга пути).
func TestSettlementEffectsID(t *testing.T) {
	if id, ok := settlementEffectsID("/admin/settlements/abc/effects"); !ok || id != "abc" {
		t.Errorf("settlementEffectsID валидного пути: %q ok=%v", id, ok)
	}
	for _, p := range []string{"/admin/settlements/abc", "/admin/settlements/abc/branches", "/admin/settlements//effects"} {
		if _, ok := settlementEffectsID(p); ok {
			t.Errorf("settlementEffectsID(%q) = ok, хочу false", p)
		}
	}
}
