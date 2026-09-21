// Тесты генерации фракций/столиц (спека 2026-09-21-фабрики-релиз-2-столицы-
// фракций §3/§7): порядок гейта Пакмана/мьютекса (C3), аддитивное поле
// capitals в ответе, buildings в truncateTables.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator"
)

// expectEmptyFactionsBuildings — ожидания attachFactionsAndBuildings (пустые
// выборки фракций и строений) для моков с regexp-матчером (дефолт sqlmock).
func expectEmptyFactionsBuildings(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`FROM factions WHERE homeworld_id = ANY\(\$1\) ORDER BY name ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "color", "description", "homeworld_id"}))
	mock.ExpectQuery(`FROM buildings WHERE planet_id = ANY\(\$1\) ORDER BY building_type ASC, id ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "building_type", "owner_type", "owner_id"}))
}

// expectEmptyFactionsBuildingsExact — то же для моков с QueryMatcherEqual
// (SQL указан дословно, как в planet_repo.go).
func expectEmptyFactionsBuildingsExact(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`
		SELECT id, name, type, color, description, homeworld_id
		FROM factions WHERE homeworld_id = ANY($1) ORDER BY name ASC
	`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "color", "description", "homeworld_id"}))
	mock.ExpectQuery(`
		SELECT id, planet_id, building_type, owner_type, owner_id
		FROM buildings WHERE planet_id = ANY($1) ORDER BY building_type ASC, id ASC
	`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "building_type", "owner_type", "owner_id"}))
}

// expectSettledPlanetsCount — счётчик обитаемых планет (знаменатель джоба).
func expectSettledPlanetsCount(mock sqlmock.Sqlmock, n int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM planets p\s+WHERE EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(n))
}

// TestGenerateFactionsEmptyUniverseSyncCapitals — ветка «нет обитаемых планет»
// (total == 0): синхронный догон столиц под гейтом, ответ содержит
// аддитивное поле capitals = возврат EnsureCapitals (M).
func TestGenerateFactionsEmptyUniverseSyncCapitals(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectSettledPlanetsCount(mock, 0)
	mock.ExpectExec(`INSERT INTO buildings \(planet_id, building_type, owner_type, owner_id\)`).
		WillReturnResult(sqlmock.NewResult(0, 3))

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.GenerateFactions(rec, httptest.NewRequest(http.MethodPost, "/admin/generate-factions", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Status   string `json:"status"`
		Total    int    `json:"total"`
		Capitals int    `json:"capitals"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "factions_generated", resp.Status)
	require.Equal(t, 0, resp.Total, "total — число планет с населением, не фракций")
	require.Equal(t, 3, resp.Capitals, "capitals — из возврата EnsureCapitals")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsEmptyUniversePacmanGate — C3: при занятом пакмане ветка
// total == 0 отдаёт 409 ДО догона (гейт раньше ветки), записи в buildings нет.
func TestGenerateFactionsEmptyUniversePacmanGate(t *testing.T) {
	startJob(t, generator.JobPacman)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectSettledPlanetsCount(mock, 0)
	// INSERT в buildings не ожидается: сработай догон — был бы 500, не 409.

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.GenerateFactions(rec, httptest.NewRequest(http.MethodPost, "/admin/generate-factions", nil))

	require.Equal(t, http.StatusConflict, rec.Code, "пакман ест — догон столиц не стартует")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsEmptyUniverseMutexGate — C3: мутации вселенной заняты
// (ClearUniverse/пакман под universeMutationMu) → 409 и на ветке total == 0.
func TestGenerateFactionsEmptyUniverseMutexGate(t *testing.T) {
	universeMutationMu.Lock()
	defer universeMutationMu.Unlock()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectSettledPlanetsCount(mock, 0)

	h := &AdminHandlers{db: db}
	rec := httptest.NewRecorder()
	h.GenerateFactions(rec, httptest.NewRequest(http.MethodPost, "/admin/generate-factions", nil))

	require.Equal(t, http.StatusConflict, rec.Code, "мьютекс мутаций занят — догон не стартует")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestTruncateTablesIncludesBuildings — страховка от TRUNCATE-ловушки:
// buildings ссылается на planets (ON DELETE CASCADE), без неё TRUNCATE падает
// «cannot truncate a table referenced in a foreign key constraint».
func TestTruncateTablesIncludesBuildings(t *testing.T) {
	require.Contains(t, truncateTables, "buildings",
		"buildings обязана быть в truncateTables (admin_universe.go): FK buildings.planet_id → planets")
}

// B12: system_belts обязана быть в truncateTables (спека поясов §4.6: FK
// system_belts.world_id → worlds; без неё TRUNCATE worlds падёт — та же
// ловушка, что у buildings).
func TestTruncateTablesIncludesSystemBelts(t *testing.T) {
	require.Contains(t, truncateTables, "system_belts",
		"system_belts обязана быть в truncateTables (admin_universe.go)")
}
