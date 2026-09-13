// Тесты конвейра «Проверка гипотез» (admin_hypothesis.go,
// specs/hypothesis_testing.md §8, тесты 6–7).
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator"
	"zorion/internal/generator/planet"
	"zorion/internal/generator/settlement"
	"zorion/internal/mapcache"
)

// testTwinSpec — крошечный TwinSpec для детерминированного теста конвейра:
func testTwinSpec() planet.TwinSpec {
	return planet.TwinSpec{
		ID: "test_mini",
		Base: map[string]interface{}{
			"temperature": 288.0, "water_percent": 70.0,
			"mass": 1.0, "size": 1.0, "density": 1.0, "gravity": 1.0,
			"atmosphere": "азотно-кислородная", "hydrosphere": "океаны",
			"biosphere": "растительная", "life": true,
			"type": "землеподобная", "surface_dominant": "океаны",
			"archetype": "умеренный", "system_age": 1.0,
			"moons": 1, "development_level": 0.5,
			"surface_composition":    map[string]interface{}{"океаны": 60.0, "скалы": 40.0},
			"subterrain_composition": map[string]interface{}{"породы": 100.0},
		},
		Groups: []planet.TwinGroup{{
			ID: "g", Name: "g", PlanetsPerWorld: 2,
			Overrides: map[string]interface{}{"system_age": 1.0},
			Settlement: planet.SettlementSpec{
				Chance:     1.0,
				Population: settlement.Population{Kind: "fixed", Fixed: 12345},
			},
		}},
	}
}

// Тест 6: конвейр с очисткой → генерацией миров/планет → поселениями.
// Поселения создаются напрямую (без заводов и товаров): любой insert в
// factories/goods_batches был бы неожиданным для sqlmock → ошибка → провал
// задачи. Кэш статистики пересчитывается после коммита.
func TestRunHypothesisJobPipeline(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// 1. Очистка вселенной (clearUniverseTx).
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET current_world_id = NULL WHERE current_world_id IS NOT NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_current_world_id_fkey`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`TRUNCATE TABLE ` + truncateTables).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`ALTER TABLE users ADD CONSTRAINT users_current_world_id_fkey`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// 2. Звезда группы (1), планеты (2), поселения (2 — шанс 1.0).
	mock.ExpectExec(`INSERT INTO worlds \(id, name, coord_x, coord_y, spectral_class, temperature`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	for i := 0; i < 2; i++ {
		mock.ExpectExec(`INSERT INTO planets \(id, world_id, name, orbit_index, data`).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	for i := 0; i < 2; i++ {
		mock.ExpectExec(`INSERT INTO settlements \(id, planet_id, population, population_exact, stability, computed_at`).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	// 3. Пересчёт статистики (recomputePlanetStats) — 3 запроса.
	mock.ExpectQuery(`SELECT id, spectral_class, temperature FROM worlds`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "spectral_class", "temperature"}))
	mock.ExpectQuery(`SELECT id, world_id, data FROM planets`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "data"}))
	mock.ExpectQuery(`SELECT planet_id, SUM\(population\) FROM settlements GROUP BY planet_id`).
		WillReturnRows(sqlmock.NewRows([]string{"planet_id", "population"}))

	// 4. mapCache.LoadAsync запускается в фоне; его запросы намеренно не
	// ожидаются (как в TestGenerateSettlementsNotCanceledOnResponse) — они
	// ошибутся тихо и в лог, на результат теста не влияют.

	h := &AdminHandlers{db: db, mapCache: mapcache.NewManager()}

	settled, report, err := h.runHypothesisJob(context.Background(), testTwinSpec())
	require.NoError(t, err)
	require.Equal(t, 2, settled,
		"шанс заселения 1.0 на 2 планетах → 2 поселения")
	require.Contains(t, report, "невозможных: 0",
		"умеренный шаблон (temp 288 K) не должен давать невозможных планет")
	require.NoError(t, mock.ExpectationsWereMet(),
		"конвейр должен пройти очистку → миры → планеты → поселения → коммит → статистику")
}

// Тест 7: повторный запуск при занятом джобе → 409.
func TestRunHypothesisConflict(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	_, cancel := context.WithCancel(context.Background())
	require.True(t, statusManager.TryStart(generator.JobHypothesis, 10, cancel))
	defer statusManager.Cancel(generator.JobHypothesis)

	h := &AdminHandlers{db: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/hypothesis/run",
		strings.NewReader(`{"id":"x","base":{"temperature":288},"groups":[`+
			`{"id":"g","planets_per_world":1,"settlement":{"chance":1,"population":{"kind":"fixed","fixed":1}}}]}`))
	rec := httptest.NewRecorder()

	h.RunHypothesis(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
}

// Невалидный spec (пустой шаблон/группы) → 400, до запросов в БД.
func TestRunHypothesisInvalidSpec(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/hypothesis/run",
		strings.NewReader(`{"id":"x"}`))
	rec := httptest.NewRecorder()

	h.RunHypothesis(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet(), "невалидный spec не должен трогать БД")
}

// Битое JSON-тело → 400.
func TestRunHypothesisBadJSON(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}

	req := httptest.NewRequest(http.MethodPost, "/admin/hypothesis/run",
		strings.NewReader("{не-json"))
	rec := httptest.NewRecorder()

	h.RunHypothesis(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// planetFromData — PlanetData с JSON из map (для тестов аудита).
func planetFromData(t *testing.T, data map[string]interface{}) *planet.PlanetData {
	t.Helper()
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	return &planet.PlanetData{ID: aStr(data, "id"), Name: aStr(data, "name"), Data: raw}
}

func aStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// Полные океаны при кипящей температуре — глобальная невозможность → счётчик 1.
func TestCountImpossiblePlanets(t *testing.T) {
	hot := planetFromData(t, map[string]interface{}{
		"id": "p_hot", "name": "Горячий", "world_id": "w",
		"size": 1.0, "mass": 1.0, "density": 1.0,
		"temperature": 573.0, "water_percent": 80.0,
		"type": "океаническая", "surface_dominant": "океаны",
		"archetype": "умеренный", "atmosphere": "азотно-кислородная",
		"hydrosphere": "океаны", "biosphere": "растительная",
		"life": false, "is_gas_giant": false, "radioactive": false,
		"surface_composition":    map[string]interface{}{"океаны": 80.0, "скалы": 20.0},
		"subterrain_composition": map[string]interface{}{"породы": 100.0},
	})
	// Умеренная океаническая — не невозможная.
	clean := planetFromData(t, map[string]interface{}{
		"id": "p_ok", "name": "Умеренный", "world_id": "w",
		"size": 1.0, "mass": 1.0, "density": 1.0,
		"temperature": 288.0, "water_percent": 70.0,
		"type": "океаническая", "surface_dominant": "океаны",
		"archetype": "умеренный", "atmosphere": "азотно-кислородная",
		"hydrosphere": "океаны", "biosphere": "растительная",
		"life": false, "is_gas_giant": false, "radioactive": false,
		"surface_composition":    map[string]interface{}{"океаны": 80.0, "скалы": 20.0},
		"subterrain_composition": map[string]interface{}{"породы": 100.0},
	})

	total, impossible, highCodes := countImpossiblePlanets([]*planet.PlanetData{hot, clean})
	require.Equal(t, 2, total)
	require.Equal(t, 1, impossible, "только «океаны на жаре» — глобальная невозможность")
	require.Equal(t, 1, highCodes["oceans_in_heat"])
	require.Empty(t, highCodes["life_without_water"])
}

// Оазис (≤1% поверхности) на горячей планете — объясним, не в счёте.
func TestCountImpossiblePlanetsOasisNotCounted(t *testing.T) {
	oasis := planetFromData(t, map[string]interface{}{
		"id": "p_oasis", "name": "Оазис", "world_id": "w",
		"size": 1.0, "mass": 1.0, "density": 1.0,
		"temperature": 600.0, "water_percent": 15.0,
		"type": "пустынная", "surface_dominant": "пески_пустыни",
		"archetype": "экстремальный", "atmosphere": "разреженная",
		"hydrosphere": "сухая", "biosphere": "стерильная",
		"life": false, "is_gas_giant": false, "radioactive": false,
		"surface_composition":    map[string]interface{}{"океаны": 1.0, "пески_пустыни": 99.0},
		"subterrain_composition": map[string]interface{}{"породы": 100.0},
	})

	_, impossible, _ := countImpossiblePlanets([]*planet.PlanetData{oasis})
	require.Equal(t, 0, impossible, "океаны ≤1% — локальный оазис, не невозможная планета")
}

// Отчёт: нулевое число невозможных — ✅; иначе — ⚠️ с разбивкой по кодам.
func TestBuildHypothesisReport(t *testing.T) {
	require.Equal(t, "✅ Создано планет: 10, поселений: 8, невозможных: 0",
		buildHypothesisReport(10, 8, 0, map[string]int{}))

	got := buildHypothesisReport(10, 8, 3, map[string]int{"oceans_in_heat": 2, "life_without_water": 1})
	require.Contains(t, got, "⚠️ Создано планет: 10, поселений: 8, невозможных: 3")
	require.Contains(t, got, "oceans_in_heat ×2")
	require.Contains(t, got, "life_without_water ×1")
}