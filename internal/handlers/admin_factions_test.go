// Тесты генерации фракций/столиц (спека 2026-09-21-фабрики-релиз-2-столицы-
// фракций §3/§7): порядок гейта Пакмана/мьютекса (C3), аддитивное поле
// capitals в ответе, buildings в truncateTables.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// expectEmptyDeposits — ожидание attachDeposits (пустая выборка залежей,
// спека 2026-09-22-поселение-... §5.1) для моков с regexp-матчером.
func expectEmptyDeposits(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`FROM deposits d\s+JOIN goods g ON g.id = d.good_id\s+WHERE d.planet_id = ANY\(\$1\) ORDER BY d.planet_id, d.id`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "good_id", "name", "stratum", "wealth", "amount"}))
}

// expectRaceCandidateCount — счётчик заселённых рас (знаменатель джоба).
func expectRaceCandidateCount(mock sqlmock.Sqlmock, n int) {
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT s\.race_id\) FROM settlements s`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(n))
}

// TestGenerateFactionsEmptyUniverseSyncCapitals — ветка «нет заселённых рас»
// (total == 0): синхронный догон столиц под гейтом, ответ содержит
// аддитивное поле capitals = возврат EnsureCapitals (M).
func TestGenerateFactionsEmptyUniverseSyncCapitals(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectRaceCandidateCount(mock, 0)
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
	require.Equal(t, 0, resp.Total, "total — число рас-кандидатов, не фракций")
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

	expectRaceCandidateCount(mock, 0)
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

	expectRaceCandidateCount(mock, 0)

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

// T9: deposits обязана быть в truncateTables (FK deposits.planet_id → planets;
// без неё TRUNCATE planets упадёт «cannot truncate a table referenced in a
// foreign key constraint»).
func TestTruncateTablesIncludesDeposits(t *testing.T) {
	require.Contains(t, truncateTables, "deposits",
		"deposits обязана быть в truncateTables (admin_universe.go)")
}

// T1 (итерация 2): обе таблицы ветки обязаны быть в truncateTables — без
// settlement_branches упадёт TRUNCATE settlements (FK settlement_id), без
// settlement_branch_buffers — TRUNCATE на ветке.
func TestTruncateTablesIncludesSettlementBranches(t *testing.T) {
	require.Contains(t, truncateTables, "settlement_branches",
		"settlement_branches обязана быть в truncateTables (FK settlements)")
	require.Contains(t, truncateTables, "settlement_branch_buffers",
		"settlement_branch_buffers обязана быть в truncateTables (FK settlement_branches)")
}

// B12: system_belts обязана быть в truncateTables (спека поясов §4.6: FK
// system_belts.world_id → worlds; без неё TRUNCATE worlds падёт — та же
// ловушка, что у buildings/deposits).
func TestTruncateTablesIncludesSystemBelts(t *testing.T) {
	require.Contains(t, truncateTables, "system_belts",
		"system_belts обязана быть в truncateTables (admin_universe.go)")
}

// ЧК1 (спека 2026-09-23-контракт-ленивая-доска-пакет-и-снабжение §8):
// contract_board_state ссылается на planets (FK planet_id ON DELETE CASCADE),
// без неё TRUNCATE planets падает «cannot truncate a table referenced in a
// foreign key constraint».
func TestTruncateTablesIncludesContractBoardState(t *testing.T) {
	require.Contains(t, truncateTables, "contract_board_state",
		"contract_board_state обязана быть в truncateTables (admin_universe.go): FK planet_id → planets")
}

// ЧК1: миграция 000071 создаёт чек-точку доски (§3.1) и колонки/индексы
// пакета (§4.2). Проверка по тексту миграции — БД-независимо (паттерн
// TestPlayerCargoCascadeFKs); живое применение — на dev-БД.
func TestMigrationContractBoard(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000071_contract_board.sql"))
	require.NoError(t, err, "миграция 000071_contract_board.sql должна существовать")
	s := string(src)
	require.Contains(t, s, "CREATE TABLE IF NOT EXISTS contract_board_state",
		"чек-точка доски — отдельная таблица (§3.1)")
	require.Contains(t, s, "REFERENCES planets (id) ON DELETE CASCADE",
		"каскад worlds → planets → contract_board_state")
	require.Contains(t, s, "ADD COLUMN IF NOT EXISTS package_key TEXT NULL")
	require.Contains(t, s, "ADD COLUMN IF NOT EXISTS share_index INTEGER NULL")
	require.Contains(t, s, "uq_contracts_package_open_share",
		"идемпотентность: одна открытая доля на индекс пакета")
	require.Contains(t, s, "uq_contracts_package_taken_executor",
		"«один игрок — одна взятая доля пакета»")
	require.Contains(t, s, "idx_contracts_package_open",
		"поиск открытых долей пакета при сверке")
}
