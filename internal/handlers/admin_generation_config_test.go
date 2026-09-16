// internal/handlers/admin_generation_config_test.go
// Тесты конфига генерации (99.2.3 §3/§4.5): дефолты при пустой БД,
// валидация весов (409 при нулевой/отрицательной сумме) и средних
// (422 при mean вне [0, 8] — не клампится!), сохранение upsert'ом.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator/galaxy"
	"zorion/internal/generator/planet"
)

// TestGetGenerationConfigDefaults — пустая БД → дефолты (99.2.4 §4.1/§4.2/§5.2).
func TestGetGenerationConfigDefaults(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT key, payload FROM generation_config`).
		WillReturnRows(sqlmock.NewRows([]string{"key", "payload"}))

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodGet, "/admin/generation/config", nil)
	rec := httptest.NewRecorder()

	h.GetGenerationConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var cfg GenerationConfigPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	require.Equal(t, DefaultGenerationConfig(), cfg, "дефолты: веса 85/5/2/1/1/2/1/3, mean M=2.5")
	require.InDelta(t, 32.0, cfg.StarWeights.Spectral["M"], 0.001)
	require.InDelta(t, 85.0, cfg.StarWeights.SystemTypes["single"], 0.001)
	require.InDelta(t, 2.5, cfg.PlanetMeans.M, 0.001)
}

// TestGetGenerationConfigMergesStored — сохранённые ключи накладываются поверх дефолтов.
func TestGetGenerationConfigMergesStored(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT key, payload FROM generation_config`).
		WillReturnRows(sqlmock.NewRows([]string{"key", "payload"}).
			AddRow("planet_means", `{"O":0.3,"B":0.3,"A":1,"F":1.75,"G":1.75,"K":2,"M":3,"L":1,"T":1,"Y":1,"black_hole":0.1,"neutron":0.05,"white_dwarf":0.3,"protostar":0,"exotic":0.1,"binary_wide_factor":0.9,"binary_close_mean":0.2,"multiple_factor":0.9,"max":8}`))

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodGet, "/admin/generation/config", nil)
	rec := httptest.NewRecorder()

	h.GetGenerationConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var cfg GenerationConfigPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	require.InDelta(t, 3.0, cfg.PlanetMeans.M, 0.001, "сохранённый mean M перекрывает дефолт")
	require.InDelta(t, 85.0, cfg.StarWeights.SystemTypes["single"], 0.001, "несохранённый ключ — дефолт")
}

// TestPutGenerationConfigConflictZeroSum — нулевая сумма весов → 409.
func TestPutGenerationConfigConflictZeroSum(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	body := `{"star_weights":{"spectral":{"O":0,"B":0,"A":0,"F":0,"G":0,"K":0,"M":0,"L":0,"T":0,"Y":0},"system_types":{"single":85,"binary":5,"multiple":2,"black_hole":1,"neutron":1,"white_dwarf":2,"protostar":1,"exotic":3}},"planet_means":{}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/generation/config", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.PutGenerationConfig(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code, "нулевая сумма спектральных весов отклоняется 409")
}

// TestPutGenerationConfigUnprocessableMean — mean > 8 → 422 (не клампится, 99.2.3 §4.3).
func TestPutGenerationConfigUnprocessableMean(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	body := `{"star_weights":{"spectral":{"O":0.5,"B":2,"A":4,"F":7,"G":12,"K":17,"M":32,"L":8,"T":8,"Y":9.5},"system_types":{"single":85,"binary":5,"multiple":2,"black_hole":1,"neutron":1,"white_dwarf":2,"protostar":1,"exotic":3}},"planet_means":{"O":9}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/generation/config", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.PutGenerationConfig(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "mean 9 > 8 отклоняется 422")
}

// TestPutGenerationConfigNegativeWeight — отрицательный вес → 409.
func TestPutGenerationConfigNegativeWeight(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	body := `{"star_weights":{"spectral":{"O":-1,"B":2,"A":4,"F":7,"G":12,"K":17,"M":32,"L":8,"T":8,"Y":9.5},"system_types":{"single":85,"binary":5,"multiple":2,"black_hole":1,"neutron":1,"white_dwarf":2,"protostar":1,"exotic":3}},"planet_means":{}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/generation/config", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.PutGenerationConfig(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code, "отрицательный вес отклоняется 409")
}

// TestPutGenerationConfigSaves — валидный конфиг upsert'ится тремя ключами.
func TestPutGenerationConfigSaves(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("star_weights", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("planet_means", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("stellar_mass_ranges", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("race_tuning_softness", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("race_cluster_planet_count_mult", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := &AdminHandlers{db: db}
	body := `{"star_weights":{"spectral":{"O":0.5,"B":2,"A":4,"F":7,"G":12,"K":17,"M":32,"L":8,"T":8,"Y":9.5},"system_types":{"single":85,"binary":5,"multiple":2,"black_hole":1,"neutron":1,"white_dwarf":2,"protostar":1,"exotic":3}},"planet_means":` + planetMeansJSON(t) + `}`
	req := httptest.NewRequest(http.MethodPut, "/admin/generation/config", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.PutGenerationConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp GenerationConfigPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.InDelta(t, 2.5, resp.PlanetMeans.M, 0.001)
}

// planetMeansJSON — валидный JSON средних (дефолты).
func planetMeansJSON(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(planet.DefaultPlanetMeans())
	require.NoError(t, err)
	return string(b)
}

// TestPutGenerationConfigWithoutMaxDefaultsTo8 — баг #3 (прогон @tester):
// фронт PLANET_MEAN_KEYS не содержит max, PUT без max сохранял max:0 —
// конфиг врал. Сервер обязан не затирать потолок нулём (дефолт 8, Kepler-90).
func TestPutGenerationConfigWithoutMaxDefaultsTo8(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("star_weights", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("planet_means", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("stellar_mass_ranges", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("race_tuning_softness", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("race_cluster_planet_count_mult", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := &AdminHandlers{db: db}
	// Тело без "max" — ровно то, что шлёт фронт (PLANET_MEAN_KEYS без max).
	body := `{"star_weights":{"spectral":{"O":0.5,"B":2,"A":4,"F":7,"G":12,"K":17,"M":32,"L":8,"T":8,"Y":9.5},"system_types":{"single":85,"binary":5,"multiple":2,"black_hole":1,"neutron":1,"white_dwarf":2,"protostar":1,"exotic":3}},"planet_means":{"O":0.3,"B":0.3,"A":1,"F":1.75,"G":1.75,"K":2,"M":2.5,"L":1,"T":1,"Y":1,"black_hole":0.1,"neutron":0.05,"white_dwarf":0.3,"protostar":0,"exotic":0.1,"binary_wide_factor":0.9,"binary_close_mean":0.2,"multiple_factor":0.9}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/generation/config", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.PutGenerationConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp GenerationConfigPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 8, resp.PlanetMeans.Max, "max не затирается нулём — дефолт 8 (Kepler-90)")
}

// TestValidateGenerationConfigMassRangeInvalid — баг валидации масс (29a §4м):
// min ≤ 0 / min > max / max > 100 → 422.
func TestValidateGenerationConfigMassRangeInvalid(t *testing.T) {
	cfg := DefaultGenerationConfig()

	// min ≤ 0.
	bad := cfg
	bad.StellarMassRanges = map[string]galaxy.MassRange{"O": {Min: 0, Max: 60}}
	status, _ := validateGenerationConfig(&bad)
	require.Equal(t, http.StatusUnprocessableEntity, status, "min ≤ 0 отклоняется 422")

	// min > max.
	bad = cfg
	bad.StellarMassRanges = map[string]galaxy.MassRange{"G": {Min: 2, Max: 1}}
	status, _ = validateGenerationConfig(&bad)
	require.Equal(t, http.StatusUnprocessableEntity, status, "min > max отклоняется 422")

	// max > 100.
	bad = cfg
	bad.StellarMassRanges = map[string]galaxy.MassRange{"exotic": {Min: 10, Max: 200}}
	status, _ = validateGenerationConfig(&bad)
	require.Equal(t, http.StatusUnprocessableEntity, status, "max > 100 отклоняется 422")

	// Валидный диапазон — проходит.
	good := cfg
	good.StellarMassRanges = map[string]galaxy.MassRange{"M": {Min: 0.1, Max: 0.6}}
	status, _ = validateGenerationConfig(&good)
	require.Zero(t, status)
}

// TestPutGenerationConfigWithoutMassRangesDefaults — PUT без stellar_mass_ranges
// не затирает конфиг: в ответе дефолтные диапазоны масс (29a §4м, как с max).
func TestPutGenerationConfigWithoutMassRangesDefaults(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	// loadGenerationConfig при пустой БД → дефолты; сохраняем 3 ключа.
	mock.ExpectQuery(`SELECT key, payload FROM generation_config`).
		WillReturnRows(sqlmock.NewRows([]string{"key", "payload"}))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("star_weights", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("planet_means", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("stellar_mass_ranges", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("race_tuning_softness", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`).
		WithArgs("race_cluster_planet_count_mult", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := &AdminHandlers{db: db}
	// Тело без stellar_mass_ranges (старый клиент/пустая форма).
	body := `{"star_weights":{"spectral":{"O":0.5,"B":2,"A":4,"F":7,"G":12,"K":17,"M":32,"L":8,"T":8,"Y":9.5},"system_types":{"single":85,"binary":5,"multiple":2,"black_hole":1,"neutron":1,"white_dwarf":2,"protostar":1,"exotic":3}},"planet_means":` + planetMeansJSON(t) + `}`
	req := httptest.NewRequest(http.MethodPut, "/admin/generation/config", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.PutGenerationConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var resp GenerationConfigPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, galaxy.DefaultStellarMassRanges(), resp.StellarMassRanges,
		"без масс в PUT — дефолтные диапазоны, не nil/пусто")
}

// TestValidateGenerationConfigSum — типы систем без black_hole: сумма > 0, валидно.
func TestValidateGenerationConfigTypesPartial(t *testing.T) {
	cfg := DefaultGenerationConfig()
	cfg.StarWeights.SystemTypes = map[string]float64{"single": 100}
	status, _ := validateGenerationConfig(&cfg)
	require.Zero(t, status, "один тип с весом 100 — валидно")
}

// TestDefaultGenerationConfigMatchesSpec — дефолты 99.2.4 §4.1 (сумма 100).
func TestDefaultGenerationConfigMatchesSpec(t *testing.T) {
	cfg := DefaultGenerationConfig()
	sum := 0.0
	for _, v := range cfg.StarWeights.SystemTypes {
		sum += v
	}
	require.InDelta(t, 100.0, sum, 0.001)
	// Проверка ключей совпадает с galaxy.DefaultWeights.
	require.Equal(t, galaxy.DefaultWeights(), cfg.StarWeights)
}