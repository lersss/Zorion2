// internal/handlers/studio_producer_rate_test.go
// T-А1 (студия, итерация И1): число скорости пары «постройка × рецепт»
// (PUT /studio/api/producers/{id}/recipes/{recipe_id}) — 200/404/409/422; и
// типизированные поля редактора стадии (PUT /studio/api/producers/{id}) пишут
// params + признак eat_units, валидируют exit < enter, предупреждают о позиции
// в eat без эффекта, соблюдают приоритет полей M6 (спека 2026-09-23 §10).
package handlers

import (
	"database/sql"
	"database/sql/driver"
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

// jsonArgContains — matcher аргумента-строки JSON: строка обязана содержать все
// фрагменты (проверяем собранный params, не порядок ключей).
type jsonArgContains struct{ parts []string }

func (m jsonArgContains) Match(v driver.Value) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	for _, p := range m.parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func newRateTestHandlers(t *testing.T, db *sql.DB) *StudioHandlers {
	t.Helper()
	return NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
}

// TestStudioSetRecipeRateOK — PUT .../recipes/10 {rate: 600} → 200.
func TestStudioSetRecipeRateOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1\)`).
		WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1\)`).
		WithArgs(int64(10)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(5), int64(10)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_recipes SET rate = \$1::double precision WHERE producer_type_id = \$2 AND recipe_id = \$3`).
		WithArgs(600.0, int64(5), int64(10)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newRateTestHandlers(t, db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5/recipes/10", strings.NewReader(`{"rate":600}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioSetRecipeRateNull — rate:null → NULL (очистка числа) → 200.
func TestStudioSetRecipeRateNull(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1\)`).
		WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1\)`).
		WithArgs(int64(10)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(5), int64(10)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_recipes SET rate = \$1::double precision WHERE producer_type_id = \$2 AND recipe_id = \$3`).
		WithArgs(nil, int64(5), int64(10)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newRateTestHandlers(t, db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5/recipes/10", strings.NewReader(`{"rate":null}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioSetRecipeRateNegative — rate < 0 → 422 до похода в БД.
func TestStudioSetRecipeRateNegative(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := newRateTestHandlers(t, db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5/recipes/10", strings.NewReader(`{"rate":-1}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioSetRecipeRateTypeMissing — нет типа → 404.
func TestStudioSetRecipeRateTypeMissing(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1\)`).
		WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	h := newRateTestHandlers(t, db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5/recipes/10", strings.NewReader(`{"rate":600}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioSetRecipeRateNoPair — типа/рецепта нет в паре → 409 (сначала привязать).
func TestStudioSetRecipeRateNoPair(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1\)`).
		WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1\)`).
		WithArgs(int64(10)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(5), int64(10)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	h := newRateTestHandlers(t, db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5/recipes/10", strings.NewReader(`{"rate":600}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioProducerTypedParams — PUT producers/5 с типизированными eat/effects/
// stage: params собирается сервером, признак eat_units ставится.
func TestStudioProducerTypedParams(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// buildTypedProducerParams: текущий params типа (NULL) → словарь товаров →
	// каталог типов эффектов (для валидации позиций/типов).
	mock.ExpectQuery(`SELECT params FROM producer_types WHERE id = \$1`).
		WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"params"}).AddRow(nil))
	mock.ExpectQuery(`SELECT name_norm FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name_norm"}).AddRow("пища"))
	mock.ExpectQuery(`SELECT id, name, name_norm, impact, COALESCE\(params->>'curve', ''\), created_at, code FROM effect_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve", "created_at", "code"}).
			AddRow(int64(1), "Голод", "голод", "population_rate", "hunger", time.Now(), "e_0001"))

	// UpdateProducerType: begin+advisory, SELECT ... FOR UPDATE, UPDATE params.
	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", nil, nil, nil, nil))
	mock.ExpectExec(`UPDATE producer_types SET params = \$1 WHERE id = \$2`).
		WithArgs(jsonArgContains{parts: []string{`"eat_units":"per_day_per_billion"`, `"пища":600`, `"effects":{"пища":"голод"}`, `"stage":{"enter":100000000,"exit":50000000}`}}, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newRateTestHandlers(t, db)
	body := `{"eat":{"пища":600},"effects":{"пища":"голод"},"stage":{"enter":100000000,"exit":50000000}}`
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioProducerTypedCategoryRejected — позиция, совпавшая с именем
// категории, но не товара → 422 (спека 2026-09-24 §9.2: категория — не позиция).
func TestStudioProducerTypedCategoryRejected(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT params FROM producer_types WHERE id = \$1`).
		WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"params"}).AddRow(nil))
	// словарь товаров не содержит «продовольствие» — это имя категории
	mock.ExpectQuery(`SELECT name_norm FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name_norm"}).AddRow("пища"))

	h := newRateTestHandlers(t, db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5", strings.NewReader(`{"eat":{"продовольствие":600}}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioProducerTypedWarning — позиция в eat без эффекта → 200 + предупреждение.
func TestStudioProducerTypedWarning(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT params FROM producer_types WHERE id = \$1`).
		WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"params"}).AddRow(nil))
	mock.ExpectQuery(`SELECT name_norm FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name_norm"}).AddRow("пища"))
	mock.ExpectQuery(`SELECT id, name, name_norm, impact, COALESCE\(params->>'curve', ''\), created_at, code FROM effect_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve", "created_at", "code"}))

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", nil, nil, nil, nil))
	mock.ExpectExec(`UPDATE producer_types SET params = \$1 WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newRateTestHandlers(t, db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5", strings.NewReader(`{"eat":{"пища":600}}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Warnings, 1, "позиция в eat без эффекта → предупреждение (§8.2)")
	require.Contains(t, resp.Warnings[0], "пища")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioProducerTypedExitNotBelowEnter — без зазора (exit ≥ enter) → 422:
// требуется exit < enter (зазор гистерезиса, §4.3/§11.2).
func TestStudioProducerTypedExitNotBelowEnter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	for _, body := range []string{
		`{"stage":{"enter":50,"exit":100}}`,  // exit > enter
		`{"stage":{"enter":100,"exit":100}}`, // exit == enter
	} {
		mock.ExpectQuery(`SELECT params FROM producer_types WHERE id = \$1`).
			WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"params"}).AddRow(nil))
		mock.ExpectQuery(`SELECT name_norm FROM goods`).
			WillReturnRows(sqlmock.NewRows([]string{"name_norm"}).AddRow("пища"))
		mock.ExpectQuery(`SELECT id, name, name_norm, impact, COALESCE\(params->>'curve', ''\), created_at, code FROM effect_types ORDER BY id`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve", "created_at", "code"}))
		h := newRateTestHandlers(t, db)
		req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ProducerByID(rec, req)
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "exit ≥ enter → 422: %s", body)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioProducerTypedNegativeThreshold — отрицательный порог → 422 (§10).
func TestStudioProducerTypedNegativeThreshold(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT params FROM producer_types WHERE id = \$1`).
		WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"params"}).AddRow(nil))
	mock.ExpectQuery(`SELECT name_norm FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name_norm"}).AddRow("пища"))
	mock.ExpectQuery(`SELECT id, name, name_norm, impact, COALESCE\(params->>'curve', ''\), created_at, code FROM effect_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve", "created_at", "code"}))

	h := newRateTestHandlers(t, db)
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5", strings.NewReader(`{"stage":{"enter":-1,"exit":-2}}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioProducerTypedPriorityM6 — строка params + типизированный eat:
// типизированное побеждает (eat перезаписан целиком), прочие ключи сохранены,
// признак eat_units поставлен.
func TestStudioProducerTypedPriorityM6(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT name_norm FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name_norm"}).AddRow("пища"))
	mock.ExpectQuery(`SELECT id, name, name_norm, impact, COALESCE\(params->>'curve', ''\), created_at, code FROM effect_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve", "created_at", "code"}).
			AddRow(int64(1), "Голод", "голод", "population_rate", "hunger", time.Now(), "e_0001"))

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", nil, nil, nil, nil))
	mock.ExpectExec(`UPDATE producer_types SET params = \$1 WHERE id = \$2`).
		WithArgs(jsonArgContains{parts: []string{`"eat":{"пища":600}`, `"eat_units":"per_day_per_billion"`, `"keep":"x"`}}, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := newRateTestHandlers(t, db)
	body := `{"params":"{\"eat\":{\"старое\":1},\"keep\":\"x\"}","eat":{"пища":600}}`
	req := httptest.NewRequest(http.MethodPut, "/studio/api/producers/5", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
