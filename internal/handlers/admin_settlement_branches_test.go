// internal/handlers/admin_settlement_branches_test.go
//
// Тесты админ-ручек ветки поселения (спека 2026-09-22-поселение-ветка-буферы-
// переработка §5, T2–T4/T11): создание ветки (404/422/409), гейты мутаций,
// «добавить ресурсы во вход» (инкремент, 422 на не-компонент, 404), контракт
// ответа {settlement_id, planet_id, branches[]}, скрытие входа игроку.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator"
	"zorion/internal/models"
)

func branchReq(method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, strings.NewReader(body))
}

// mockBranchAdvisoryLock — advisory-лок поселения первым действием AddBranch
// (единый порядок локов с owner-проходом, спека 2026-09-23 §6.3).
func mockBranchAdvisoryLock(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`pg_advisory_xact_lock`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
}

// mockBranchLoadRows — ожидания чтения блока веток (loadBranches): ветка +
// компоненты рецепта + буферы. Возвращает одну ветку «Пища» (s1) с входом
// Мясо=97 и выходом Пища=30.
func mockBranchLoadRows(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`FROM settlement_branches b`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}).
			AddRow("b1", "s1", int64(69), time.Now(), int64(378), "Пища", int64(1), "продовольствие"))
	mock.ExpectQuery(`FROM recipe_components rc`).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"}).
			AddRow(int64(69), int64(359), 1))
	mock.ExpectQuery(`FROM settlement_branch_buffers bb`).
		WillReturnRows(sqlmock.NewRows([]string{"branch_id", "direction", "good_id", "name", "amount"}).
			AddRow("b1", "output", int64(378), "Пища", 30.0).
			AddRow("b1", "input", int64(359), "Мясо", 97.0))
}

// T2: успешное создание ветки → 200 и блок branches[] с посеянными строками.
func TestAddBranchHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mockBranchAdvisoryLock(mock)
	mock.ExpectQuery(`SELECT planet_id FROM settlements WHERE id = \$1`).
		WithArgs("s1").WillReturnRows(sqlmock.NewRows([]string{"planet_id"}).AddRow("p1"))
	mock.ExpectQuery(`SELECT good_id, complexity FROM recipes WHERE id = \$1`).
		WithArgs(int64(69)).WillReturnRows(sqlmock.NewRows([]string{"good_id", "complexity"}).AddRow(int64(378), int64(1)))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM recipe_components WHERE recipe_id = \$1 AND component_id IS NOT NULL`).
		WithArgs(int64(69)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM settlement_branches WHERE settlement_id = \$1 AND recipe_id = \$2\)`).
		WithArgs("s1", int64(69)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO settlement_branches`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// CreateBranchTx: посев буферов (компоненты + выход).
	mock.ExpectQuery(`SELECT component_id FROM recipe_components`).
		WithArgs(int64(69)).WillReturnRows(sqlmock.NewRows([]string{"component_id"}).AddRow(int64(359)))
	mock.ExpectExec(`INSERT INTO settlement_branch_buffers`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT good_id FROM recipes WHERE id = \$1`).
		WithArgs(int64(69)).WillReturnRows(sqlmock.NewRows([]string{"good_id"}).AddRow(int64(378)))
	mock.ExpectExec(`INSERT INTO settlement_branch_buffers`).WillReturnResult(sqlmock.NewResult(0, 1))
	mockBranchLoadRows(mock)
	mock.ExpectCommit()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranch(rec, branchReq(http.MethodPost, "/admin/settlements/s1/branches", `{"recipe_id":69}`))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body struct {
		SettlementID string                    `json:"settlement_id"`
		PlanetID     string                    `json:"planet_id"`
		Branches     []models.SettlementBranch `json:"branches"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "s1", body.SettlementID)
	assert.Equal(t, "p1", body.PlanetID)
	require.Len(t, body.Branches, 1)
	assert.Equal(t, int64(69), body.Branches[0].RecipeID)
	assert.Equal(t, "Пища", body.Branches[0].RecipeName)
	assert.Len(t, body.Branches[0].Output, 1)
	assert.Len(t, body.Branches[0].Input, 1)
}

// T2: поселения нет → 404.
func TestAddBranchSettlementNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mockBranchAdvisoryLock(mock)
	mock.ExpectQuery(`SELECT planet_id FROM settlements WHERE id = \$1`).
		WithArgs("sX").WillReturnRows(sqlmock.NewRows([]string{"planet_id"}))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranch(rec, branchReq(http.MethodPost, "/admin/settlements/sX/branches", `{"recipe_id":69}`))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T2: рецепта нет → 422.
func TestAddBranchRecipeNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mockBranchAdvisoryLock(mock)
	mock.ExpectQuery(`SELECT planet_id FROM settlements WHERE id = \$1`).
		WithArgs("s1").WillReturnRows(sqlmock.NewRows([]string{"planet_id"}).AddRow("p1"))
	mock.ExpectQuery(`SELECT good_id, complexity FROM recipes WHERE id = \$1`).
		WithArgs(int64(999)).WillReturnRows(sqlmock.NewRows([]string{"good_id", "complexity"}))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranch(rec, branchReq(http.MethodPost, "/admin/settlements/s1/branches", `{"recipe_id":999}`))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T2: рецепт без заполненных компонентов → 422.
func TestAddBranchNoComponents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mockBranchAdvisoryLock(mock)
	mock.ExpectQuery(`SELECT planet_id FROM settlements WHERE id = \$1`).
		WithArgs("s1").WillReturnRows(sqlmock.NewRows([]string{"planet_id"}).AddRow("p1"))
	mock.ExpectQuery(`SELECT good_id, complexity FROM recipes WHERE id = \$1`).
		WithArgs(int64(69)).WillReturnRows(sqlmock.NewRows([]string{"good_id", "complexity"}).AddRow(int64(378), nil))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM recipe_components WHERE recipe_id = \$1 AND component_id IS NOT NULL`).
		WithArgs(int64(69)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranch(rec, branchReq(http.MethodPost, "/admin/settlements/s1/branches", `{"recipe_id":69}`))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T2: ветка на этот рецепт уже есть → 409.
func TestAddBranchDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mockBranchAdvisoryLock(mock)
	mock.ExpectQuery(`SELECT planet_id FROM settlements WHERE id = \$1`).
		WithArgs("s1").WillReturnRows(sqlmock.NewRows([]string{"planet_id"}).AddRow("p1"))
	mock.ExpectQuery(`SELECT good_id, complexity FROM recipes WHERE id = \$1`).
		WithArgs(int64(69)).WillReturnRows(sqlmock.NewRows([]string{"good_id", "complexity"}).AddRow(int64(378), int64(1)))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM recipe_components WHERE recipe_id = \$1 AND component_id IS NOT NULL`).
		WithArgs(int64(69)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM settlement_branches WHERE settlement_id = \$1 AND recipe_id = \$2\)`).
		WithArgs("s1", int64(69)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranch(rec, branchReq(http.MethodPost, "/admin/settlements/s1/branches", `{"recipe_id":69}`))
	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T2: recipe_id отсутствует/≤ 0 → 422 без обращений к БД.
func TestAddBranchRecipeIDInvalid(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	for _, body := range []string{`{}`, `{"recipe_id":0}`, `{"recipe_id":-3}`} {
		rec := httptest.NewRecorder()
		h.AddBranch(rec, branchReq(http.MethodPost, "/admin/settlements/s1/branches", body))
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "тело %s", body)
	}
}

// T3: пакман ест миры → 409, ветка не создаётся.
func TestAddBranchPacmanGate(t *testing.T) {
	startJob(t, generator.JobPacman)
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranch(rec, branchReq(http.MethodPost, "/admin/settlements/s1/branches", `{"recipe_id":69}`))
	require.Equal(t, http.StatusConflict, rec.Code)
}

// T3: universeMutationMu занят → 409.
func TestAddBranchMutexGate(t *testing.T) {
	universeMutationMu.Lock()
	defer universeMutationMu.Unlock()
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranch(rec, branchReq(http.MethodPost, "/admin/settlements/s1/branches", `{"recipe_id":69}`))
	require.Equal(t, http.StatusConflict, rec.Code)
}

// T4: инкремент входа + ответ — блок branches[].
func TestAddBranchInputHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT settlement_id, recipe_id FROM settlement_branches WHERE id = \$1 FOR UPDATE`).
		WithArgs("b1").WillReturnRows(sqlmock.NewRows([]string{"settlement_id", "recipe_id"}).AddRow("s1", int64(69)))
	mock.ExpectQuery(`SELECT id, name FROM goods WHERE id = \$1`).
		WithArgs(int64(359)).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(359), "Мясо"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipe_components WHERE recipe_id = \$1 AND component_id = \$2\)`).
		WithArgs(int64(69), int64(359)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`INSERT INTO settlement_branch_buffers`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT planet_id FROM settlements WHERE id = \$1`).
		WithArgs("s1").WillReturnRows(sqlmock.NewRows([]string{"planet_id"}).AddRow("p1"))
	mockBranchLoadRows(mock)
	mock.ExpectCommit()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranchInput(rec, branchReq(http.MethodPost, "/admin/branches/b1/input", `{"good_id":359,"amount":50}`))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body struct {
		SettlementID string                    `json:"settlement_id"`
		PlanetID     string                    `json:"planet_id"`
		Branches     []models.SettlementBranch `json:"branches"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "s1", body.SettlementID)
	assert.Equal(t, "p1", body.PlanetID)
	require.Len(t, body.Branches, 1)
}

// T4: amount ≤ 0/нет → 422 без обращений к БД.
func TestAddBranchInputAmountInvalid(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	h := &AdminHandlers{db: db}
	for _, body := range []string{`{"good_id":359}`, `{"good_id":359,"amount":0}`, `{"good_id":359,"amount":-5}`} {
		rec := httptest.NewRecorder()
		h.AddBranchInput(rec, branchReq(http.MethodPost, "/admin/branches/b1/input", body))
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "тело %s", body)
	}
}

// T4: ресурс не компонент рецепта ветки → 422 (фильтр по component_id).
func TestAddBranchInputNotComponent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT settlement_id, recipe_id FROM settlement_branches WHERE id = \$1 FOR UPDATE`).
		WithArgs("b1").WillReturnRows(sqlmock.NewRows([]string{"settlement_id", "recipe_id"}).AddRow("s1", int64(69)))
	mock.ExpectQuery(`SELECT id, name FROM goods WHERE id = \$1`).
		WithArgs(int64(358)).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(358), "Растения"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipe_components WHERE recipe_id = \$1 AND component_id = \$2\)`).
		WithArgs(int64(69), int64(358)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranchInput(rec, branchReq(http.MethodPost, "/admin/branches/b1/input", `{"good_id":358,"amount":50}`))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T4: ветки нет → 404.
func TestAddBranchInputBranchNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT settlement_id, recipe_id FROM settlement_branches WHERE id = \$1 FOR UPDATE`).
		WithArgs("bX").WillReturnRows(sqlmock.NewRows([]string{"settlement_id", "recipe_id"}))
	mock.ExpectRollback()

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.AddBranchInput(rec, branchReq(http.MethodPost, "/admin/branches/bX/input", `{"good_id":359,"amount":50}`))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T11: stripPlanetDetails обнуляет Input у веток (защита в глубину), даже если
// ветка оказалась в модели; Output при этом не трогается.
func TestStripPlanetDetailsHidesBranchInput(t *testing.T) {
	p := models.Planet{
		Settlements: []models.Settlement{{
			ID: "s1",
			Branches: []models.SettlementBranch{{
				ID: "b1", RecipeID: 69,
				Output: []models.BranchBufferEntry{{GoodID: 378, Amount: 30}},
				Input:  []models.BranchBufferEntry{{GoodID: 359, Amount: 97}},
			}},
		}},
	}
	out := stripPlanetDetails(p, nil)
	// Поселения игроку без знания не отдаются вовсе.
	require.Nil(t, out.Settlements)
}

// T11: вход ветки обнуляется до сброса поселений (stripBranchInputs) — защита
// в глубину; выход (Output) не трогается.
func TestStripBranchInputsNilsInput(t *testing.T) {
	p := models.Planet{
		Settlements: []models.Settlement{{
			ID: "s1",
			Branches: []models.SettlementBranch{{
				ID:     "b1",
				Output: []models.BranchBufferEntry{{GoodID: 378, Amount: 30}},
				Input:  []models.BranchBufferEntry{{GoodID: 359, Amount: 97}},
			}},
		}},
	}
	stripBranchInputs(&p)
	require.Len(t, p.Settlements[0].Branches, 1)
	require.Nil(t, p.Settlements[0].Branches[0].Input, "входной буфер должен быть обнулён")
	require.Len(t, p.Settlements[0].Branches[0].Output, 1, "выход остаётся частью деталей поселения")
}
