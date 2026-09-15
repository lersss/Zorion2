// internal/handlers/admin_regenerate_planets_test.go
// Тесты взаимной блокировки пересчёта планет с генерацией вселенной/планет
// и очисткой (AGENTS.md §23, 99.2.3 §5: живые прогоны не запускать поверх
// чужого — джобы пишут в одни таблицы и не знают о соседе).
package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator"
	"zorion/internal/generator/planet"
)

// TestClearPlanetsOfUsesPqArray — баг #2 (прогон @tester): world_id = ANY($1)
// с []string давал "sql: converting argument $1 type: unsupported type []string".
// Для lib/pq нужен pq.Array(ids) — как в economy_repository.pqStringArray.
func TestClearPlanetsOfUsesPqArray(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT(*) FROM planets WHERE world_id = ANY($1)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectExec(`DELETE FROM planets WHERE world_id = ANY($1)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 3))

	h := &AdminHandlers{db: db}
	n, err := h.clearPlanetsOf([]planet.WorldInfo{{ID: "w1"}, {ID: "w2"}})
	require.NoError(t, err, "[]string без pq.Array давал ошибку конвертации")
	require.Equal(t, 3, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRegeneratePlanetsBlockedByUniverse — крутится генерация вселенной →
// пересчёт планет отвечает 409 ещё до выборки миров (fail fast).
func TestRegeneratePlanetsBlockedByUniverse(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	_, cancel := context.WithCancel(context.Background())
	require.True(t, statusManager.TryStart(generator.JobGenerateUniverse, 1, cancel))
	t.Cleanup(func() {
		cancel()
		statusManager.Cancel(generator.JobGenerateUniverse)
	})

	h := &AdminHandlers{db: db}
	body := `{"min_planets":0,"max_planets":3,"include_normal":true,"include_binary":true,"include_exotic":true}`
	req := httptest.NewRequest(http.MethodPost, "/admin/regenerate-planets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.RegeneratePlanets(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code, "пересчёт не стартует поверх генерации вселенной")
	require.NoError(t, mock.ExpectationsWereMet(), "до 409 не должно быть запросов к БД")
}

// TestRegeneratePlanetsBlockedByPlanets — крутится генерация планет → 409.
func TestRegeneratePlanetsBlockedByPlanets(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	_, cancel := context.WithCancel(context.Background())
	require.True(t, statusManager.TryStart(generator.JobGeneratePlanets, 1, cancel))
	t.Cleanup(func() {
		cancel()
		statusManager.Cancel(generator.JobGeneratePlanets)
	})

	h := &AdminHandlers{db: db}
	body := `{"min_planets":0,"max_planets":3,"include_normal":true,"include_binary":true,"include_exotic":true}`
	req := httptest.NewRequest(http.MethodPost, "/admin/regenerate-planets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.RegeneratePlanets(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestClearUniverseBlockedByRegenerate — крутится пересчёт планет →
// очистка вселенной отвечает 409 (раньше не видела новый джоб).
func TestClearUniverseBlockedByRegenerate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	_, cancel := context.WithCancel(context.Background())
	require.True(t, statusManager.TryStart(generator.JobRegeneratePlanets, 1, cancel))
	t.Cleanup(func() {
		cancel()
		statusManager.Cancel(generator.JobRegeneratePlanets)
	})

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodPost, "/admin/clear", nil)
	rec := httptest.NewRecorder()

	h.ClearUniverse(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code, "очистка не стартует поверх пересчёта планет")
	require.NoError(t, mock.ExpectationsWereMet(), "до 409 не должно быть запросов к БД")
}

// TestGenerateUniverseBlockedByRegenerate — крутится пересчёт планет →
// генерация вселенной отвечает 409 (обратная сторона блокировки).
func TestGenerateUniverseBlockedByRegenerate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	_, cancel := context.WithCancel(context.Background())
	require.True(t, statusManager.TryStart(generator.JobRegeneratePlanets, 1, cancel))
	t.Cleanup(func() {
		cancel()
		statusManager.Cancel(generator.JobRegeneratePlanets)
	})

	h := &AdminHandlers{db: db}
	body := `{"world_count":10,"cluster_count":1,"map_size":1000,"min_dist":100,"cluster_radius":100,"cluster_spacing":200,"outlier_percent":10}`
	req := httptest.NewRequest(http.MethodPost, "/admin/generate", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.GenerateUniverse(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code, "генерация вселенной не стартует поверх пересчёта планет")
	require.NoError(t, mock.ExpectationsWereMet())
}