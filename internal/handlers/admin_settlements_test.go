// Тесты на генерацию поселений (admin_settlements.go).
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator"
	"zorion/internal/mapcache"
)

// TestGenerateSettlementsNotCanceledOnResponse — регрессия: контекст фоновой
// задачи НЕ должен наследоваться от контекста запроса (в реальном сервере
// net/http отменяет r.Context(), когда handler возвращает ответ — из-за этого
// генерация мгновенно «останавливалась»).
//
// Хитрость теста: httptest сам не отменяет контекст запроса, поэтому тест
// отменяет его вручную (имитация возврата из handler) сразу после вызова.
// WillDelayFor удерживает запрос к планетам, чтобы отмена успела наступить.
//
// Проверка двойная:
//   - статус задачи после отмены контекста запроса должен дойти до "done",
//     а не "canceled"/"error";
//   - mock.ExpectationsWereMet() доказывает, что горутина реально выполнила
//     запрос к планетам (при баге она умирала на старте и до запроса не доходила).
func TestGenerateSettlementsNotCanceledOnResponse(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	// 1 планета есть (чтобы задача стартовала), но пригодных 0 — генерация
	// завершается быстро без записи в БД.
	mock.ExpectQuery(`SELECT COUNT(*) FROM planets`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT id, data FROM planets`).
		WillDelayFor(300 * time.Millisecond).
		WillReturnRows(sqlmock.NewRows([]string{"id", "data"}).
			AddRow("p1", `{"water_percent":0,"temperature":100,"atmosphere":"ядовитая","life":false}`))

	h := &AdminHandlers{db: db, mapCache: mapcache.NewManager()}

	// Контекст запроса, который будет отменён, как только handler «вернёт ответ».
	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/admin/generate-settlements", nil).WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.GenerateSettlements(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)
	cancel() // сервер отменяет контекст запроса после возврата из handler

	// Ждём, пока фоновая задача дойдёт до терминального состояния.
	deadline := time.Now().Add(2 * time.Second)
	status := ""
	for time.Now().Before(deadline) {
		_, _, status, _ = statusManager.GetStatus(generator.JobGenerateSettlements)
		if status != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	require.Equal(t, "done", status,
		"генерация не должна «останавливаться» из-за отмены контекста запроса")
	require.NoError(t, mock.ExpectationsWereMet(),
		"горутина должна реально выполнить запрос к планетам")
}

// GenerateSettlements отклоняет битое JSON-тело модели.
func TestGenerateSettlementsBadOverrideBody(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/generate-settlements",
		strings.NewReader("{не-json"))
	rec := httptest.NewRecorder()

	h.GenerateSettlements(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// GenerateSettlements отклоняет невалидную модель (например, битый диапазон
// населения) до запросов в БД.
func TestGenerateSettlementsInvalidModel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db, mapCache: mapcache.NewManager()}

	req := httptest.NewRequest(http.MethodPost, "/admin/generate-settlements",
		strings.NewReader(`{"mode":"simple","chance":0.5,"population":{"kind":"random","min":100,"max":10}}`))
	rec := httptest.NewRecorder()

	h.GenerateSettlements(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet(), "невалидная модель не должна трогать БД")
}

// SettlementFields отдаёт реестр полей planet.data для формы правил.
func TestSettlementFields(t *testing.T) {
	h := &AdminHandlers{}

	req := httptest.NewRequest(http.MethodGet, "/admin/settlement-fields", nil)
	rec := httptest.NewRecorder()

	h.SettlementFields(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var fields []struct {
		Key  string `json:"key"`
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fields))
	require.NotEmpty(t, fields)
	require.Contains(t, "temperature atmosphere is_gas_giant radioactive life water_percent type",
		fields[0].Key, "реестр должен начинаться с известных полей")
	for _, f := range fields {
		require.NotEmpty(t, f.Key)
		require.NotEmpty(t, f.Type)
	}
}

// ClearSettlements удаляет все поселения и возвращает число удалённых.
func TestClearSettlements(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT(*) FROM settlements`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectExec(`DELETE FROM settlements`).
		WillReturnResult(sqlmock.NewResult(0, 3))

	h := &AdminHandlers{db: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/clear-settlements", nil)
	rec := httptest.NewRecorder()

	h.ClearSettlements(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Deleted int `json:"deleted"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 3, resp.Deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}