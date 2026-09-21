// internal/handlers/admin_pacman_test.go
// Тесты пакмана (спека 2026-09-20-pacman-galaxy-wipe.md): траектория (§4.1),
// SQL батча (§3.2 — порядок, RETURNING id, RowsAffected), ретраи (§3.3),
// конфликты (§2.2 — 409 во всех комбинациях), пересчёт регионов (§3.4).
package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator"
	"zorion/internal/mapcache"
	"zorion/internal/races"
	"zorion/internal/repository"
	"zorion/internal/travel"
)

// ==================== ТРАЕКТОРИЯ (§4.1) ====================

func pacmanIDs(worlds []mapcache.World) []string {
	ids := make([]string, len(worlds))
	for i, w := range worlds {
		ids[i] = w.ID
	}
	return ids
}

// Полярная спираль: сортировка по (angle=atan2(y,x), radius²), tiebreak id.
func TestBuildPacmanTrajectorySpiral(t *testing.T) {
	worlds := []mapcache.World{
		{ID: "a", X: 0, Y: 10},  // angle π/2, r²=100
		{ID: "b", X: 10, Y: 0},  // angle 0, r²=100
		{ID: "c", X: 0, Y: -10}, // angle -π/2, r²=100
		{ID: "d", X: -10, Y: 0}, // angle π, r²=100
		{ID: "e", X: 1, Y: 0},   // angle 0, r²=1
		{ID: "f", X: 0, Y: 0},   // angle 0, r²=0
	}
	got := buildPacmanTrajectory(worlds, "spiral")
	// Углы: c(-π/2) < f(0) < e(0) < b(0) < a(π/2) < d(π);
	// внутри угла 0 — по радиусу²: f(0) < e(1) < b(100).
	require.Equal(t, []string{"c", "f", "e", "b", "a", "d"}, pacmanIDs(got))
}

// Детерминизм: одинаковый вход → одинаковый порядок; tiebreak по id.
func TestBuildPacmanTrajectoryDeterministic(t *testing.T) {
	worlds := []mapcache.World{
		{ID: "b", X: 3, Y: 4},   // r²=25, angle atan2(4,3)≈0.93
		{ID: "a", X: 3, Y: 4},   // та же точка — tiebreak id
		{ID: "c", X: -3, Y: -4}, // r²=25, angle atan2(-4,-3)≈-2.21
	}
	first := buildPacmanTrajectory(worlds, "spiral")
	second := buildPacmanTrajectory(worlds, "spiral")
	require.Equal(t, pacmanIDs(first), pacmanIDs(second), "детерминизм: одинаковый вход → одинаковый порядок")
	require.Equal(t, []string{"c", "a", "b"}, pacmanIDs(first), "tiebreak по id при равных (angle, radius²)")
}

// Змейка: сортировка по x, затем по y, tiebreak id.
func TestBuildPacmanTrajectorySnake(t *testing.T) {
	worlds := []mapcache.World{
		{ID: "b", X: 5, Y: 1},
		{ID: "a", X: 1, Y: 9},
		{ID: "c", X: 1, Y: 2},
	}
	got := buildPacmanTrajectory(worlds, "snake")
	require.Equal(t, []string{"c", "a", "b"}, pacmanIDs(got))
}

// nearest детерминирован: два вызова — одинаковый порядок (спека §4.1).
func TestNearestTrajectoryDeterministic(t *testing.T) {
	worlds := []mapcache.World{
		{ID: "b", X: 10, Y: 0},
		{ID: "a", X: 0, Y: 0},
		{ID: "c", X: 5, Y: 5},
		{ID: "d", X: -3, Y: 2},
	}
	first := nearestTrajectory(worlds)
	second := nearestTrajectory(worlds)
	require.Equal(t, pacmanIDs(first), pacmanIDs(second), "детерминизм: одинаковый вход → одинаковый порядок")
	require.Len(t, first, 4, "все миры в траектории")
}

// nearest — жадный ближайший сосед: старт у центра (0,0), каждая следующая
// точка — ближайшая к предыдущей из оставшихся (спека §4.1).
func TestNearestTrajectoryGreedy(t *testing.T) {
	worlds := []mapcache.World{
		{ID: "w0", X: 0, Y: 0},
		{ID: "w1", X: 1, Y: 0},
		{ID: "w2", X: 2, Y: 0},
		{ID: "w3", X: 3, Y: 0},
	}
	got := nearestTrajectory(worlds)
	require.Len(t, got, 4)
	require.Equal(t, "w0", got[0].ID, "старт — ближайший к центру (0,0)")
	for i := 1; i < len(got); i++ {
		prev := got[i-1]
		bestID := ""
		bestD := math.Inf(1)
		for _, w := range worlds {
			if w.ID == prev.ID {
				continue
			}
			already := false
			for j := 0; j < i; j++ {
				if got[j].ID == w.ID {
					already = true
					break
				}
			}
			if already {
				continue
			}
			d := (w.X-prev.X)*(w.X-prev.X) + (w.Y-prev.Y)*(w.Y-prev.Y)
			if d < bestD || (d == bestD && (bestID == "" || w.ID < bestID)) {
				bestD = d
				bestID = w.ID
			}
		}
		require.Equal(t, bestID, got[i].ID, "шаг %d: следующая — ближайшая к предыдущей из оставшихся", i)
	}
}

// Неизвестный режим траектории → 400 (спека §8: nearest | spiral | snake).
func TestStartPacmanInvalidTrajectory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodPost, "/admin/pacman/start", strings.NewReader(`{"trajectory":"teleport"}`))
	rec := httptest.NewRecorder()
	h.StartPacman(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet(), "до 400 не должно быть запросов к БД")
}

// ==================== SQL БАТЧА (§3.2) ====================

// expectPacmanBatchAttempt — ожидания одной попытки батча в sqlmock.
// failWorldsDelete — DELETE worlds падает с FK-violation (23503);
// worldsDeleted — RowsAffected от DELETE worlds (для отчёта «Съедено N»).
func expectPacmanBatchAttempt(mock sqlmock.Sqlmock, failWorldsDelete bool, worldsDeleted int) {
	mock.ExpectBegin()
	// 1. Игроки: UPDATE ... RETURNING u.id, w.name — PK users = id (Н2 критика:
	// колонки user_id нет; мок не валидирует колонки — проверяем явно).
	mock.ExpectQuery(`UPDATE users u SET current_world_id = NULL, current_position = NULL\s+FROM worlds w\s+WHERE u\.current_world_id = ANY\(\$1\) AND w\.id = u\.current_world_id\s+RETURNING u\.id, w\.name`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("user-1", "Мир 1"))
	// 2. NPC-агенты (NO ACTION 000026).
	mock.ExpectExec(`DELETE FROM npc_agents\s+WHERE current_world_id = ANY\(\$1\) OR from_world_id = ANY\(\$1\) OR target_world_id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 3))
	// 3. Знания (NO ACTION 000040).
	mock.ExpectExec(`DELETE FROM player_planet_knowledge\s+WHERE planet_id IN \(SELECT id FROM planets WHERE world_id = ANY\(\$1\)\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 5))
	// 4. Внутрисистемные полёты (без FK 000046).
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights\s+WHERE world_id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	// 4.5. Намерения композитного маршрута к съеденным мирам (спека 99.2.30 §5, M3).
	mock.ExpectExec(`UPDATE users SET pending_destination = NULL\s+WHERE \(pending_destination->>'world_id'\)::uuid = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 4.6. Возврат залога контрактов съеденных миров (§6.5) — до DELETE.
	mock.ExpectQuery(`UPDATE contracts\s+SET status = CASE WHEN executor_id IS NULL`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id", "escrow_amount", "escrow_withdrawable"}))
	// 4.7. Контракты с мёртвой целью (payload.dest_world_id съеденного мира).
	mock.ExpectQuery(`UPDATE contracts\s+SET status = CASE WHEN executor_id IS NULL`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id", "escrow_amount", "escrow_withdrawable"}))
	// Счётчики планет/поселений — до удаления миров (каскад не отдаёт RowsAffected).
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM planets WHERE world_id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM settlements\s+WHERE planet_id IN \(SELECT id FROM planets WHERE world_id = ANY\(\$1\)\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))
	// 5. Миры — каскад.
	if failWorldsDelete {
		mock.ExpectExec(`DELETE FROM worlds\s+WHERE id = ANY\(\$1\)`).
			WithArgs(sqlmock.AnyArg()).
			WillReturnError(&pq.Error{Code: "23503"})
		return
	}
	mock.ExpectExec(`DELETE FROM worlds\s+WHERE id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, int64(worldsDeleted)))
	mock.ExpectCommit()
	// 6. ПОСЛЕ коммита, вне транзакции: полёты к съеденным (PK player_flights = user_id).
	mock.ExpectQuery(`SELECT user_id FROM player_flights WHERE to_world_id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
}

// Порядок операций батча + RETURNING id + RowsAffected (спека §3.2, §9.8).
func TestEatPacmanBatchSQL(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectPacmanBatchAttempt(mock, false, 2)

	h := &AdminHandlers{db: db, travelManager: travel.NewManager(nil)}
	stats, err := h.eatPacmanBatchOnce([]string{"w1", "w2"})
	require.NoError(t, err)
	require.Equal(t, 2, stats.worlds, "RowsAffected от DELETE worlds")
	require.Equal(t, 7, stats.planets)
	require.Equal(t, 4, stats.settlements)
	require.Equal(t, 3, stats.agents)
	require.Equal(t, 5, stats.knowledge)
	require.Equal(t, 1, stats.users, "RETURNING id вернул одного игрока")
	require.NoError(t, mock.ExpectationsWereMet())
}

// M3 (спека 99.2.30 §5): намерение к съеденному миру очищается отдельным
// UPDATE — игрок с намерением летит из другой системы, шаг 1
// (current_world_id = ANY($1)) его не трогает. Каст (…)::uuid обязателен:
// ->> даёт text, в $1 — массив uuid.
func TestEatPacmanClearsPendingDestination(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET pending_destination = NULL\s+WHERE \(pending_destination->>'world_id'\)::uuid = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := &AdminHandlers{db: db}
	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, h.clearPendingDestinationsForWorlds(tx, []string{"w1", "w2"}))
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ретраи (§3.3): FK-violation (23503) на первой попытке → повторная попытка
// ловит новую строку → успех.
func TestEatPacmanBatchRetries(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectPacmanBatchAttempt(mock, true, 0)  // попытка 1: FK → rollback
	expectPacmanBatchAttempt(mock, false, 2) // попытка 2: успех

	h := &AdminHandlers{db: db, travelManager: travel.NewManager(nil)}
	stats, err := h.eatPacmanBatch([]mapcache.World{{ID: "w1"}, {ID: "w2"}})
	require.NoError(t, err)
	require.Equal(t, 2, stats.worlds)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ретраи исчерпаны (3×100 мс) → ошибка джоба (Fail с отчётом).
func TestEatPacmanBatchRetriesExhausted(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	for i := 0; i < batchRetries; i++ {
		expectPacmanBatchAttempt(mock, true, 0)
	}

	h := &AdminHandlers{db: db}
	_, err = h.eatPacmanBatch([]mapcache.World{{ID: "w1"}})
	require.Error(t, err, "3 неудачные попытки → ошибка")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ЦИКЛ ДЖОБА: ОТМЕНА МЕЖДУ БАТЧАМИ (§2.3, §9.8) ====================

// Отмена между батчами: первый батч съеден, ctx.Done() срабатывает во время
// паузы → статус canceled (не error), отчёт «Съедено N из M». Транзакция
// батча в полёте не прерывается (контекст отмены в неё не передаётся).
func TestRunPacmanCancelBetweenBatches(t *testing.T) {
	// Мьютекс берёт StartPacman, разлочивает deferred runPacman — тест
	// зеркалит прод-поток (повторный Unlock здесь = panic).
	universeMutationMu.Lock()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Батч 1 (200 миров) — полная последовательность §3.2; worldsDeleted=200
	// → отчёт «Съедено 200 из 250».
	expectPacmanBatchAttempt(mock, false, 200)
	// Отмена между батчами → recomputeRegionCounts: регионов нет → 0.
	mock.ExpectQuery(`SELECT id, name, center_x, center_y, radius, color, world_count, profile, profile_intensity, race_id FROM regions`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "center_x", "center_y", "radius", "color", "world_count", "profile", "profile_intensity", "race_id",
		}))

	// Снапшот: 250 миров → 2 батча (200 + 50).
	worlds := make([]mapcache.World, 250)
	for i := range worlds {
		worlds[i] = mapcache.World{ID: fmt.Sprintf("w%03d", i), X: float64(i), Y: 0}
	}
	mc := mapcache.NewManager()
	mc.Replace(mapcache.NewSnapshot(worlds))

	ctx, cancel := context.WithCancel(context.Background())
	require.True(t, statusManager.TryStart(generator.JobPacman, 250, cancel))
	t.Cleanup(func() { statusManager.Cancel(generator.JobPacman) })

	h := &AdminHandlers{db: db, mapCache: mc, travelManager: travel.NewManager(nil)}
	done := make(chan struct{})
	go func() {
		// wps=200 ≥ batch_size=200 → адаптивный батч НЕ сжимается до 1
		// (иначе пауза 10 мс и Eventually не успевает — флейк); батч 200,
		// пауза батч-бюджета 1 с — окно для отмены между батчами.
		h.runPacman(ctx, pacmanConfig{WorldsPerSecond: 200, BatchSize: 200, Trajectory: "spiral"}, 250)
		close(done)
	}()

	// Ждём, пока съеден первый батч (Progress = 200) — пауза батч-бюджета
	// (200/200 = 1 с) даёт окно для отмены между батчами.
	require.Eventually(t, func() bool {
		_, processed, _, _, _ := statusManager.GetStatus(generator.JobPacman)
		return processed >= 200
	}, 5*time.Second, 10*time.Millisecond)

	// Отмена между батчами: Cancel → CancelFunc → ctx.Done().
	statusManager.Cancel(generator.JobPacman)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runPacman не завершился после отмены")
	}

	_, _, status, _, report := statusManager.GetStatus(generator.JobPacman)
	require.Equal(t, "canceled", status, "отмена между батчами → canceled, не error")
	require.Contains(t, report, "Съедено 200 из 250")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пустой снапшот (нет миров в mapCache) → Fail с осмысленным отчётом
// (не пустым): /admin/generate-status?job=pacman вернёт report.
func TestRunPacmanEmptySnapshotReport(t *testing.T) {
	// Мьютекс берёт StartPacman, разлочивает deferred runPacman (синхронный
	// вызов — разлочен к моменту возврата).
	universeMutationMu.Lock()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.True(t, statusManager.TryStart(generator.JobPacman, 10, cancel))
	t.Cleanup(func() { statusManager.Cancel(generator.JobPacman) })

	// mapCache без снапшота (Snapshot() == nil) — ветка «снапшот пуст».
	h := &AdminHandlers{db: db, mapCache: mapcache.NewManager()}
	h.runPacman(ctx, pacmanConfig{WorldsPerSecond: 1700, BatchSize: 200, Trajectory: "spiral"}, 10)

	_, _, status, errMsg, report := statusManager.GetStatus(generator.JobPacman)
	require.Equal(t, "error", status)
	require.Contains(t, errMsg, "снапшот")
	require.Contains(t, report, "Снапшот карты пуст — миров не найдено")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== КОНФЛИКТЫ (§2.2) ====================

// startJob — запускает джоб в statusManager с очисткой (паттерн
// admin_regenerate_planets_test.go).
func startJob(t *testing.T, jt generator.JobType) {
	t.Helper()
	_, cancel := context.WithCancel(context.Background())
	require.True(t, statusManager.TryStart(jt, 1, cancel))
	t.Cleanup(func() {
		cancel()
		statusManager.Cancel(jt)
	})
}

// Пакман стартует только при пустых 8 джобах генерации → 409.
func TestStartPacmanBlockedByGenerationJobs(t *testing.T) {
	jobs := []generator.JobType{
		generator.JobGenerateUniverse,
		generator.JobGeneratePlanets,
		generator.JobRegeneratePlanets,
		generator.JobGenerateNPC,
		generator.JobHypothesis,
		generator.JobGenerateFactions,
		generator.JobGenerateRaceSettlements,
		generator.JobGenerateSettlements,
	}
	for _, jt := range jobs {
		t.Run(string(jt), func(t *testing.T) {
			startJob(t, jt)
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			h := &AdminHandlers{db: db}
			req := httptest.NewRequest(http.MethodPost, "/admin/pacman/start", strings.NewReader(`{}`))
			rec := httptest.NewRecorder()
			h.StartPacman(rec, req)
			require.Equal(t, http.StatusConflict, rec.Code)
			require.NoError(t, mock.ExpectationsWereMet(), "до 409 не должно быть запросов к БД")
		})
	}
}

// Двойной запуск: TryStart(JobPacman) атомарно → повторный → 409.
func TestStartPacmanConflictWhenRunning(t *testing.T) {
	startJob(t, generator.JobPacman)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM worlds`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodPost, "/admin/pacman/start", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.StartPacman(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Мьютекс занят (ClearUniverse идёт) → StartPacman 409.
func TestStartPacmanMutexHeld(t *testing.T) {
	universeMutationMu.Lock()
	defer universeMutationMu.Unlock()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodPost, "/admin/pacman/start", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.StartPacman(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ClearUniverse при идущем пакмане → 409 (гвард IsRunning(JobPacman)).
func TestClearUniverseBlockedByPacman(t *testing.T) {
	startJob(t, generator.JobPacman)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodPost, "/admin/clear", nil)
	rec := httptest.NewRecorder()
	h.ClearUniverse(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ClearUniverse при занятом мьютексе (пакман ест) → 409 «Pacman is eating».
func TestClearUniverseMutexHeldByPacman(t *testing.T) {
	universeMutationMu.Lock()
	defer universeMutationMu.Unlock()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodPost, "/admin/clear", nil)
	rec := httptest.NewRecorder()
	h.ClearUniverse(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Обратное направление: пакман ест → каждый писатель вселенной → 409.
func TestPacmanBlocksOtherHandlers(t *testing.T) {
	startJob(t, generator.JobPacman)
	// GenerateRaceSettlements проверяет каталог рас ДО гварда (500 без него).
	require.NoError(t, races.LoadCatalog("../../config/races.json"))

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// GeneratePlanets читает GetAll ДО гварда — мокаем один мир.
	mock.ExpectQuery(`SELECT id, name, coord_x, coord_y, COALESCE\(spectral_class,''\), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at FROM worlds ORDER BY name`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "coord_x", "coord_y", "spectral_class", "temperature",
			"star_type", "system_type", "stellar_mods", "stellar_mass", "age",
			"created_at", "updated_at",
		}).AddRow("w1", "Мир", 0, 0, "G", 5772, "star", "single", nil, nil, nil, time.Now(), time.Now()))
	// GenerateFactions/GenerateRaceSettlements/GenerateSettlements считают
	// планеты ДО гварда.
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM planets p\s+WHERE EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM planets`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM planets`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	h := &AdminHandlers{db: db, worldRepo: repository.NewWorldRepository(db)}

	cases := []struct {
		name string
		call func(*httptest.ResponseRecorder)
	}{
		{"GenerateUniverse", func(rec *httptest.ResponseRecorder) {
			h.GenerateUniverse(rec, httptest.NewRequest(http.MethodPost, "/admin/generate", strings.NewReader(`{"world_count":10}`)))
		}},
		{"GeneratePlanets", func(rec *httptest.ResponseRecorder) {
			h.GeneratePlanets(rec, httptest.NewRequest(http.MethodPost, "/admin/generate-planets", nil))
		}},
		{"RegeneratePlanets", func(rec *httptest.ResponseRecorder) {
			h.RegeneratePlanets(rec, httptest.NewRequest(http.MethodPost, "/admin/regenerate-planets", strings.NewReader(`{"min_planets":0,"max_planets":3,"include_normal":true,"include_binary":true,"include_exotic":true}`)))
		}},
		{"GeneratePrototypePlanet", func(rec *httptest.ResponseRecorder) {
			h.GeneratePrototypePlanet(rec, httptest.NewRequest(http.MethodPost, "/admin/generate-prototype-planet", nil))
		}},
		{"GenerateFactions", func(rec *httptest.ResponseRecorder) {
			h.GenerateFactions(rec, httptest.NewRequest(http.MethodPost, "/admin/generate-factions", nil))
		}},
		{"RunHypothesis", func(rec *httptest.ResponseRecorder) {
			h.RunHypothesis(rec, httptest.NewRequest(http.MethodPost, "/admin/hypothesis/run",
				strings.NewReader(`{"id":"x","base":{"temperature":288},"groups":[`+
					`{"id":"g","planets_per_world":1,"settlement":{"chance":1,"population":{"kind":"fixed","fixed":1}}}]}`)))
		}},
		{"GenerateRaceSettlements", func(rec *httptest.ResponseRecorder) {
			h.GenerateRaceSettlements(rec, httptest.NewRequest(http.MethodPost, "/admin/generate-race-settlements", nil))
		}},
		{"GenerateSettlements", func(rec *httptest.ResponseRecorder) {
			h.GenerateSettlements(rec, httptest.NewRequest(http.MethodPost, "/admin/generate-settlements", nil))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.call(rec)
			require.Equal(t, http.StatusConflict, rec.Code, "пакман ест — %s не стартует", tc.name)
		})
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

// Миров нет → 200 {done, eaten:0} без джоба (спека §2.1).
func TestStartPacmanNoWorlds(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM worlds`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodPost, "/admin/pacman/start", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.StartPacman(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"eaten":0`)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== РЕГИОНЫ (§3.4) ====================

// Пересчёт world_count по NearestRegionIndex + удаление опустевших регионов.
func TestRecomputeRegionCounts(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Регионы: R1 у (0,0), R2 у (1000,1000).
	mock.ExpectQuery(`SELECT id, name, center_x, center_y, radius, color, world_count, profile, profile_intensity, race_id FROM regions`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "center_x", "center_y", "radius", "color", "world_count", "profile", "profile_intensity", "race_id",
		}).
			AddRow("r1", "R1", 0, 0, 500, "#fff", 2, nil, 0, nil).
			AddRow("r2", "R2", 1000, 1000, 500, "#000", 1, nil, 0, nil))

	// Снапшот: оба мира ближе к R1 → R1=2, R2=0.
	mc := mapcache.NewManager()
	mc.Replace(mapcache.NewSnapshot([]mapcache.World{
		{ID: "w1", X: 0, Y: 0},
		{ID: "w2", X: 100, Y: 100},
	}))

	// UPDATE по регионам (порядок — как в списке).
	mock.ExpectExec(`UPDATE regions SET world_count = \$2, updated_at = \$3 WHERE id = \$1`).
		WithArgs("r1", 2, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE regions SET world_count = \$2, updated_at = \$3 WHERE id = \$1`).
		WithArgs("r2", 0, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// DELETE опустевших.
	mock.ExpectExec(`DELETE FROM regions WHERE world_count = 0`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	h := &AdminHandlers{db: db, mapCache: mc}
	deleted := h.recomputeRegionCounts()
	require.Equal(t, 1, deleted, "R2 опустел — удалён")
	require.NoError(t, mock.ExpectationsWereMet())
}
