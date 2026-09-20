// internal/handlers/admin_pacman.go
//
// Пакман — админ-джоб «вайп галактики» (спека 2026-09-20-pacman-galaxy-wipe.md):
// порциями (батч 200 миров, одна транзакция) съедает миры целиком (звёзды +
// планеты + поселения + фракции + локации + NPC-агенты + знания игроков) по
// детерминированной траектории — полярной спирали от центра (или змейке).
// Событие видно всем игрокам на карте через WebSocket Broadcast (PacmanNotifier).
// Игроки (users) остаются: у игрока, чей мир съеден, обнуляются
// current_world_id/current_position + персональное уведомление; полёт к
// съеденному миру прерывается с уведомлением.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/lib/pq"

	"zorion/internal/generator"
	"zorion/internal/mapcache"
	"zorion/internal/regionprofile"
)

// Параметры пакмана (спека §8): дефолты и диапазоны из тела запроса.
const (
	pacmanDefaultWorldsPerSecond = 1700
	// Нижней границы нет (решение создателя 2026-09-20: «снизу не
	// ограничивать — нормальный ивент»): любая положительная скорость,
	// хоть 0.01 мир/с («кинорежим»); 0 и отрицательные отклоняются.
	pacmanMaxWorldsPerSecond = 10000
	pacmanDefaultBatchSize   = 200
	pacmanMinBatchSize       = 100
	pacmanMaxBatchSize       = 500
	// Дефолт — nearest (решение создателя 2026-09-20 «пусть он всё-таки как-то
	// ближайшие кушает»): жадный ближайший сосед через пространственную сетку;
	// spiral/snake — опции.
	pacmanDefaultTrajectory = "nearest"

	// batchRetries / batchBackoff — ретраи батча при FK-violation (спека §3.3):
	// гонка с NPC-тиком (агент стартовал/прибыл в мир батча в окне между
	// шагом 2 и шагом 5) или внутрисистемным прибытием.
	batchRetries = 3
	batchBackoff = 100 * time.Millisecond
)

// pacmanConfig — параметры запуска пакмана (тело запроса, дефолты §8).
type pacmanConfig struct {
	WorldsPerSecond float64 `json:"worlds_per_second"`
	BatchSize       int     `json:"batch_size"`
	Trajectory      string  `json:"trajectory"`
}

// pacmanBatchStats — счётчики удалённого за один батч (отчёт §3.4).
type pacmanBatchStats struct {
	worlds      int
	planets     int
	settlements int
	agents      int
	knowledge   int
	users       int
}

// StartPacman — POST /admin/pacman/start (спека §2.1).
//
// Тело: {"worlds_per_second": 1700, "batch_size": 200, "trajectory": "spiral"}
// (все поля опциональны, дефолты §8) → 202 {"status":"started"}; конфликт →
// 409; миров нет → 200 {"status":"done","eaten":0} (без джоба).
// Отмена и статус — существующие ручки /admin/generate-cancel?job=pacman и
// /admin/generate-status?job=pacman.
func (h *AdminHandlers) StartPacman(w http.ResponseWriter, r *http.Request) {
	var req pacmanConfig
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "Bad JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	cfg := pacmanConfig{
		WorldsPerSecond: pacmanDefaultWorldsPerSecond,
		BatchSize:       pacmanDefaultBatchSize,
		Trajectory:      pacmanDefaultTrajectory,
	}
	if req.WorldsPerSecond != 0 {
		cfg.WorldsPerSecond = req.WorldsPerSecond
	}
	if req.BatchSize != 0 {
		cfg.BatchSize = req.BatchSize
	}
	if req.Trajectory != "" {
		cfg.Trajectory = req.Trajectory
	}
	if cfg.WorldsPerSecond <= 0 || cfg.WorldsPerSecond > pacmanMaxWorldsPerSecond {
		http.Error(w, fmt.Sprintf("worlds_per_second: >0..%d", pacmanMaxWorldsPerSecond), http.StatusBadRequest)
		return
	}
	if cfg.BatchSize < pacmanMinBatchSize || cfg.BatchSize > pacmanMaxBatchSize {
		http.Error(w, fmt.Sprintf("batch_size: %d..%d", pacmanMinBatchSize, pacmanMaxBatchSize), http.StatusBadRequest)
		return
	}
	if cfg.Trajectory != "nearest" && cfg.Trajectory != "spiral" && cfg.Trajectory != "snake" {
		http.Error(w, "trajectory: nearest | spiral | snake", http.StatusBadRequest)
		return
	}

	// Разделяемый мьютекс с ClearUniverse (спека §2.2): пакман держит lock
	// до конца джоба, ClearUniverse — на время операции. TRUNCATE и
	// порционный DELETE не пересекаются; окно гонки «клик Clear в момент
	// старта пакмана» закрыто.
	if !universeMutationMu.TryLock() {
		http.Error(w, "Pacman is eating, stop it first", http.StatusConflict)
		return
	}

	// Пакман стартует только при пустых 8 джобах генерации (спека §2.2):
	// джобы пишут в одни таблицы и не знают о соседе (AGENTS.md §23).
	if statusManager.IsRunning(generator.JobGenerateUniverse) ||
		statusManager.IsRunning(generator.JobGeneratePlanets) ||
		statusManager.IsRunning(generator.JobRegeneratePlanets) ||
		statusManager.IsRunning(generator.JobGenerateNPC) ||
		statusManager.IsRunning(generator.JobHypothesis) ||
		statusManager.IsRunning(generator.JobGenerateFactions) ||
		statusManager.IsRunning(generator.JobGenerateRaceSettlements) ||
		statusManager.IsRunning(generator.JobGenerateSettlements) {
		universeMutationMu.Unlock()
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}

	var total int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM worlds`).Scan(&total); err != nil {
		universeMutationMu.Unlock()
		log.Printf("❌ StartPacman: count worlds: %v", err)
		http.Error(w, "Failed to count worlds", http.StatusInternalServerError)
		return
	}
	if total == 0 {
		universeMutationMu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"done","eaten":0}`))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobPacman, total, cancel) {
		cancel()
		universeMutationMu.Unlock()
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	go h.runPacman(ctx, cfg, total)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}

// runPacman — тело джоба: траектория → батчи → отчёт (спека §2.3). Одна
// горутина. Deferred-очистка: Unlock мьютекса, OnAgentsDeleted (кэш позиций
// агентов), notifier.end(status). Пересчёт регионов — в каждом исходе
// (done/cancel/fail) до SetReport: отчёт включает число удалённых регионов.
func (h *AdminHandlers) runPacman(ctx context.Context, cfg pacmanConfig, total int) {
	status := "done"
	eatenTotal := 0
	var planetsEaten, settlementsEaten, agentsEaten, knowledgeEaten, usersAffected int
	startTime := time.Now()

	defer func() {
		universeMutationMu.Unlock()
		// Агенты в съеденных мирах удалены (шаг 2 §3.2): позиции и кэш
		// агентов сбросить сразу (идея 26c A2, как ClearAllAgents).
		if h.npcManager != nil {
			h.npcManager.OnAgentsDeleted()
		}
		if h.pacmanNotifier != nil {
			h.pacmanNotifier.End(status, eatenTotal, total, time.Since(startTime).Milliseconds())
		}
		// Снапшот карты — пересборка из БД (в цикле не пересобирали, чтобы
		// клиенты видели поедание по событиям eaten, а не «сами по себе»).
		h.mapCache.LoadAsync(h.db)
	}()

	// 1. Миры из снапшота карты (in-memory, без SQL) → траектория (§4.1).
	snap := h.mapCache.Snapshot()
	if snap == nil || snap.Len() == 0 {
		status = "error"
		statusManager.SetReport(generator.JobPacman, "Снапшот карты пуст — миров не найдено")
		statusManager.Fail(generator.JobPacman, "снапшот карты пуст")
		return
	}
	trajectory := buildPacmanTrajectory(snap.Worlds(), cfg.Trajectory)

	// 2. Старт события (спека §5.2): Broadcast pacman_start.
	if h.pacmanNotifier != nil {
		h.pacmanNotifier.StartEvent(total, cfg.WorldsPerSecond)
	}

	// 3. Цикл по батчам траектории. Адаптивный батч (решение создателя
	// 2026-09-20): при WorldsPerSecond < BatchSize эффективный батч = 1 —
	// пакман ест ПО ОДНОЙ звезде («это не для скорости, а для вайба»):
	// центроид батча = сама звезда, частицы поедания ровно в ней, пакман
	// визуально «жрёт» каждую. На больших скоростях (1700/с) — батчами,
	// иначе не успеть за минуту.
	batchSize := cfg.BatchSize
	if cfg.WorldsPerSecond < float64(batchSize) {
		batchSize = 1
	}
	batchBudget := time.Duration(float64(batchSize) / cfg.WorldsPerSecond * float64(time.Second))
	for i := 0; i < len(trajectory); i += batchSize {
		if ctx.Err() != nil {
			// Отмена между батчами (спека §2.3): отчёт «Съедено N из M».
			status = "canceled"
			regionsDeleted := h.recomputeRegionCounts()
			statusManager.SetReport(generator.JobPacman, fmt.Sprintf(
				"Остановлено. Съедено %d из %d миров (регионов удалено: %d)", eatenTotal, total, regionsDeleted))
			return
		}
		end := i + batchSize
		if end > len(trajectory) {
			end = len(trajectory)
		}
		batch := trajectory[i:end]

		batchStart := time.Now()
		stats, err := h.eatPacmanBatch(batch)
		if err != nil {
			status = "error"
			regionsDeleted := h.recomputeRegionCounts()
			statusManager.SetReport(generator.JobPacman, fmt.Sprintf(
				"Батч %d не удалён (%v). Съедено %d из %d миров. Повторите запуск пакмана для остатка (регионов удалено: %d)",
				i/batchSize+1, err, eatenTotal, total, regionsDeleted))
			statusManager.Fail(generator.JobPacman, err.Error())
			return
		}

		eatenTotal += stats.worlds
		planetsEaten += stats.planets
		settlementsEaten += stats.settlements
		agentsEaten += stats.agents
		knowledgeEaten += stats.knowledge
		usersAffected += stats.users

		statusManager.Progress(generator.JobPacman, eatenTotal)

		// Снапшот карты НЕ пересобирается на каждый батч (было rebuildSnapshot):
		// перезапросы области у клиентов возвращали бы свежий снапшот без
		// съеденных — звёзды «лопались сами», не дожидаясь пакмана (создатель
		// 2026-09-20). Полный снапшот живёт до конца джоба; съеденные миры
		// скрывает клиент по событиям eaten (+ фильтр eatenIds при перезапросе).
		// Пересборка из БД — LoadAsync в deferred (конец джоба).

		// WS: поедание + позиция (центроид батча = сама звезда при батче 1, §4.2/§5.2).
		if h.pacmanNotifier != nil {
			cx, cy := batchCentroid(batch)
			h.pacmanNotifier.Eaten(batch, eatenTotal, total, cx, cy)
			h.pacmanNotifier.Position(cx, cy)
		}

		// Пауза до конца батч-бюджета (batch_size / worlds_per_second):
		// темп задаёт worlds_per_second; БД медленнее — паузы нет.
		elapsed := time.Since(batchStart)
		if elapsed < batchBudget {
			select {
			case <-ctx.Done():
				// отмена во время паузы — цикл завершится на следующей итерации
			case <-time.After(batchBudget - elapsed):
			}
		}
	}

	// 4. Done: пересчёт регионов + отчёт + Done (спека §3.4).
	regionsDeleted := h.recomputeRegionCounts()
	duration := time.Since(startTime)
	wps := 0.0
	if duration.Seconds() > 0 {
		wps = float64(eatenTotal) / duration.Seconds()
	}
	statusManager.SetReport(generator.JobPacman, fmt.Sprintf(
		"Пакман съел миров: %d, планет: %d, поселений: %d, агентов: %d, записей знаний: %d, игроков без мира: %d, регионов удалено: %d; время: %.1f с (%.0f миров/с)",
		eatenTotal, planetsEaten, settlementsEaten, agentsEaten, knowledgeEaten, usersAffected, regionsDeleted, duration.Seconds(), wps))
	statusManager.Done(generator.JobPacman)
}

// eatPacmanBatch — съедает батч миров одной транзакцией (спека §3.2).
// Порядок обязателен: зависимые NO ACTION удаляются ДО worlds. Ретраи
// 3×100 мс при FK-violation (гонка с NPC-тиком, §3.3). Отмена полётов —
// ПОСЛЕ COMMIT, вне транзакции (шаг 6): при rollback/ретрае не выполняется,
// иначе полёты к выжившему миру отменились бы впустую (Н3 критика).
// Батч в полёте не прерывается (спека §2.3): транзакция короткая, отмена
// замечается между батчами — контекст отмены в транзакцию не передаётся.
func (h *AdminHandlers) eatPacmanBatch(batch []mapcache.World) (pacmanBatchStats, error) {
	ids := make([]string, len(batch))
	for i, w := range batch {
		ids[i] = w.ID
	}
	var stats pacmanBatchStats
	var lastErr error
	for attempt := 0; attempt < batchRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(batchBackoff)
		}
		stats, lastErr = h.eatPacmanBatchOnce(ids)
		if lastErr == nil {
			return stats, nil
		}
		log.Printf("⚠️ Пакман: батч %d миров, попытка %d/%d: %v", len(ids), attempt+1, batchRetries, lastErr)
	}
	return stats, lastErr
}

// eatPacmanBatchOnce — одна попытка батча (одна транзакция, §3.2).
func (h *AdminHandlers) eatPacmanBatchOnce(ids []string) (pacmanBatchStats, error) {
	var stats pacmanBatchStats
	tx, err := h.db.BeginTx(context.Background(), nil)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	// 1. Игроки в съеденных мирах: без мира и без внутрисистемной позиции.
	// RETURNING u.id, w.name — PK users = id (колонки user_id нет, Н2 критика);
	// имя мира — для персонального уведомления (§5.3).
	rows, err := tx.QueryContext(context.Background(), `
		UPDATE users u SET current_world_id = NULL, current_position = NULL
		FROM worlds w
		WHERE u.current_world_id = ANY($1) AND w.id = u.current_world_id
		RETURNING u.id, w.name`, pq.Array(ids))
	if err != nil {
		return stats, err
	}
	type affectedUser struct{ id, worldName string }
	var affected []affectedUser
	for rows.Next() {
		var u affectedUser
		if err := rows.Scan(&u.id, &u.worldName); err != nil {
			rows.Close()
			return stats, err
		}
		affected = append(affected, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return stats, err
	}
	stats.users = len(affected)

	// 2. NPC-агенты в съеденных мирах / летящие в них — исчезают (решение
	// создателя): FK npc_agents → worlds NO ACTION (000026), явный DELETE.
	res, err := tx.ExecContext(context.Background(), `
		DELETE FROM npc_agents
		WHERE current_world_id = ANY($1) OR from_world_id = ANY($1) OR target_world_id = ANY($1)`,
		pq.Array(ids))
	if err != nil {
		return stats, err
	}
	if n, err := res.RowsAffected(); err == nil {
		stats.agents = int(n)
	}

	// 3. Знания игроков о съеденных планетах стираются (решение создателя):
	// FK player_planet_knowledge → planets NO ACTION (000040), явный DELETE.
	res, err = tx.ExecContext(context.Background(), `
		DELETE FROM player_planet_knowledge
		WHERE planet_id IN (SELECT id FROM planets WHERE world_id = ANY($1))`,
		pq.Array(ids))
	if err != nil {
		return stats, err
	}
	if n, err := res.RowsAffected(); err == nil {
		stats.knowledge = int(n)
	}

	// 4. Внутрисистемные полёты в съеденных мирах — строки долой (без FK,
	// 000046; in-memory самозалечивается, §7.2).
	res, err = tx.ExecContext(context.Background(), `
		DELETE FROM player_intrasystem_flights WHERE world_id = ANY($1)`,
		pq.Array(ids))
	if err != nil {
		return stats, err
	}

	// Счётчики планет/поселений — до удаления миров (каскад не отдаёт
	// RowsAffected; отчёт §3.4).
	if err := tx.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM planets WHERE world_id = ANY($1)`, pq.Array(ids)).Scan(&stats.planets); err != nil {
		return stats, err
	}
	if err := tx.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM settlements
		WHERE planet_id IN (SELECT id FROM planets WHERE world_id = ANY($1))`, pq.Array(ids)).Scan(&stats.settlements); err != nil {
		return stats, err
	}

	// 5. Миры — каскад на всё остальное (planets/locations/assignments/
	// settlements/factions/planet_resources/settlement_log).
	res, err = tx.ExecContext(context.Background(), `DELETE FROM worlds WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return stats, err
	}
	if n, err := res.RowsAffected(); err == nil {
		stats.worlds = int(n)
	}

	if err := tx.Commit(); err != nil {
		return stats, err
	}

	// 6. ПОСЛЕ коммита, вне транзакции: отмена полётов к съеденным мирам
	// (in-memory + отдельное подключение; при rollback/ретрае не выполняется).
	h.cancelFlightsTo(ids)

	// Персональные уведомления игрокам без мира (§5.3).
	for _, u := range affected {
		if h.pacmanNotifier != nil {
			msg := "Ваш мир съеден пакманом"
			if u.worldName != "" {
				msg += " («" + u.worldName + "»)"
			}
			h.pacmanNotifier.Notice(u.id, msg)
		}
	}

	return stats, nil
}

// cancelFlightsTo — отмена межзвёздных полётов к съеденным мирам (спека §7.2,
// шаг 6 §3.2): SELECT из player_flights (PK = user_id) → CancelFlight
// (in-memory отмена, строку удаляет runFlight, onArrival не вызывается —
// паттерн 97a) + уведомление «Полёт прерван — цель съедена».
func (h *AdminHandlers) cancelFlightsTo(ids []string) {
	if h.travelManager == nil {
		return
	}
	rows, err := h.db.Query(`SELECT user_id FROM player_flights WHERE to_world_id = ANY($1)`, pq.Array(ids))
	if err != nil {
		log.Printf("⚠️ Пакман: select player_flights: %v", err)
		return
	}
	defer rows.Close()
	var userIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		userIDs = append(userIDs, uid)
	}
	if err := rows.Err(); err != nil {
		log.Printf("⚠️ Пакман: player_flights scan: %v", err)
		return
	}
	for _, uid := range userIDs {
		if h.travelManager.CancelFlight(uid) && h.pacmanNotifier != nil {
			h.pacmanNotifier.Notice(uid, "Полёт прерван — цель съедена")
		}
	}
}

// buildPacmanTrajectory — детерминированная траектория поедания (спека §4.1):
// nearest — жадный ближайший сосед через пространственную сетку (дефолт,
// решение создателя 2026-09-20 «есть ближайшие»); spiral — полярная спираль
// от центра (сортировка по углу atan2(y,x), затем по радиусу², tiebreak id);
// snake — змейка строками (x, затем y, tiebreak id). Одинаковый вход →
// одинаковый порядок.
func buildPacmanTrajectory(worlds []mapcache.World, mode string) []mapcache.World {
	out := make([]mapcache.World, len(worlds))
	copy(out, worlds)
	switch mode {
	case "snake":
		sort.Slice(out, func(i, j int) bool {
			if out[i].X != out[j].X {
				return out[i].X < out[j].X
			}
			if out[i].Y != out[j].Y {
				return out[i].Y < out[j].Y
			}
			return out[i].ID < out[j].ID
		})
		return out
	case "nearest":
		return nearestTrajectory(out)
	default: // spiral
		sort.Slice(out, func(i, j int) bool {
			ai := math.Atan2(out[i].Y, out[i].X)
			aj := math.Atan2(out[j].Y, out[j].X)
			if ai != aj {
				return ai < aj
			}
			ri := out[i].X*out[i].X + out[i].Y*out[i].Y
			rj := out[j].X*out[j].X + out[j].Y*out[j].Y
			if ri != rj {
				return ri < rj
			}
			return out[i].ID < out[j].ID
		})
		return out
	}
}

// nearestTrajectory — жадный ближайший сосед через пространственную сетку
// (решение создателя 2026-09-20 «пусть он всё-таки как-то ближайшие кушает»):
// пакман ест БЛИЖАЙШИЕ миры, а не «мечтает по всей галактике» (спираль прыгает
// между кольцами). Сетка cell = 2·span/√N (span = max разброса X/Y, мин 1.0);
// старт — мир, ближайший к центру (0,0); от текущего — расширяющийся поиск по
// кольцам ячеек (Chebyshev r = 0,1,2,…), кандидаты — невизиченные миры в
// окрестности, выбирается минимальное евклидово расстояние (tiebreak id).
// Остановка кольца: (r−1)·cell > bestD — дальше заведомо не ближе (точный NN
// в пределах сетки). Детерминизм: одинаковый вход → одинаковый порядок.
// Суммарно ~O(N·k), k — кандидатов в окрестности; на 100к построение маршрута —
// секунды (в памяти, до цикла поедания).
func nearestTrajectory(worlds []mapcache.World) []mapcache.World {
	n := len(worlds)
	if n <= 1 {
		return worlds
	}

	// Разброс координат → размер ячейки сетки.
	minX, maxX := worlds[0].X, worlds[0].X
	minY, maxY := worlds[0].Y, worlds[0].Y
	for _, w := range worlds {
		if w.X < minX {
			minX = w.X
		}
		if w.X > maxX {
			maxX = w.X
		}
		if w.Y < minY {
			minY = w.Y
		}
		if w.Y > maxY {
			maxY = w.Y
		}
	}
	span := maxX - minX
	if spanY := maxY - minY; spanY > span {
		span = spanY
	}
	if span < 1.0 {
		span = 1.0
	}
	cell := 2 * span / math.Sqrt(float64(n))
	if cell <= 0 {
		cell = 1.0
	}

	// Индекс: ячейка → индексы миров (порядок вставки детерминирован).
	cellOf := func(x, y float64) [2]int {
		return [2]int{int(math.Floor(x / cell)), int(math.Floor(y / cell))}
	}
	grid := make(map[[2]int][]int, n)
	for i, w := range worlds {
		c := cellOf(w.X, w.Y)
		grid[c] = append(grid[c], i)
	}

	visited := make([]bool, n)

	// Старт: мир, ближайший к центру (0,0); tiebreak id.
	start := 0
	bestDist := math.Inf(1)
	for i, w := range worlds {
		d := w.X*w.X + w.Y*w.Y
		if d < bestDist || (d == bestDist && worlds[i].ID < worlds[start].ID) {
			bestDist = d
			start = i
		}
	}

	out := make([]mapcache.World, 0, n)
	cur := start
	visited[cur] = true
	out = append(out, worlds[cur])

	for len(out) < n {
		curCell := cellOf(worlds[cur].X, worlds[cur].Y)
		bestIdx := -1
		bestD := math.Inf(1)
		for r := 0; ; r++ {
			// Кольцо радиуса r заведомо не ближе (r−1)·cell — дальше искать нечего.
			if r > 0 {
				ringMin := float64(r-1) * cell
				if ringMin*ringMin > bestD {
					break
				}
			}
			// Chebyshev-кольцо: ячейки с max(|dx|,|dy|) == r.
			for dx := -r; dx <= r; dx++ {
				for dy := -r; dy <= r; dy++ {
					if r > 0 {
						adx, ady := dx, dy
						if adx < 0 {
							adx = -adx
						}
						if ady < 0 {
							ady = -ady
						}
						if adx > ady {
							if adx != r {
								continue
							}
						} else if ady != r {
							continue
						}
					}
					c := [2]int{curCell[0] + dx, curCell[1] + dy}
					for _, idx := range grid[c] {
						if visited[idx] {
							continue
						}
						w := worlds[idx]
						d := (w.X-worlds[cur].X)*(w.X-worlds[cur].X) +
							(w.Y-worlds[cur].Y)*(w.Y-worlds[cur].Y)
						if bestIdx == -1 || d < bestD || (d == bestD && w.ID < worlds[bestIdx].ID) {
							bestD = d
							bestIdx = idx
						}
					}
				}
			}
		}
		visited[bestIdx] = true
		out = append(out, worlds[bestIdx])
		cur = bestIdx
	}
	return out
}

// batchCentroid — центроид батча: позиция пакмана (спека §4.2).
func batchCentroid(batch []mapcache.World) (float64, float64) {
	if len(batch) == 0 {
		return 0, 0
	}
	var sx, sy float64
	for _, w := range batch {
		sx += w.X
		sy += w.Y
	}
	return sx / float64(len(batch)), sy / float64(len(batch))
}

// recomputeRegionCounts — пересчёт world_count регионов по NearestRegionIndex
// + удаление полностью опустевших (спека §3.4, решение создателя 2026-09-20:
// «регионы тоже должны куда-то деваться — съеденные»). Пересчёт, а не
// декремент — единый источник правила «мир принадлежит ближайшему региону».
// Возвращает число удалённых регионов (для отчёта).
func (h *AdminHandlers) recomputeRegionCounts() int {
	regions, err := h.loadRegionsWithProfiles()
	if err != nil {
		log.Printf("⚠️ Пакман: load regions: %v", err)
		return 0
	}
	if len(regions) == 0 {
		return 0
	}
	snap := h.mapCache.Snapshot()
	if snap == nil {
		return 0
	}
	counts := make([]int, len(regions))
	for _, w := range snap.Worlds() {
		if idx := regionprofile.NearestRegionIndex(w.X, w.Y, regions); idx >= 0 {
			counts[idx]++
		}
	}
	now := time.Now()
	for i, r := range regions {
		if _, err := h.db.Exec(`UPDATE regions SET world_count = $2, updated_at = $3 WHERE id = $1`,
			r.ID, counts[i], now); err != nil {
			log.Printf("⚠️ Пакман: update region %s: %v", r.Name, err)
		}
	}
	res, err := h.db.Exec(`DELETE FROM regions WHERE world_count = 0`)
	if err != nil {
		log.Printf("⚠️ Пакман: delete empty regions: %v", err)
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}