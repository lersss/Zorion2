package npc

import (
	"log"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"zorion/internal/mapcache"
	"zorion/internal/models"
)

// AgentStore — хранилище агентов для планировщика. Реализация —
// *repository.NPCRepository (этап 1); интерфейс — для юнит-тестов.
type AgentStore interface {
	ListDueArrivals(now time.Time, after string, limit int) ([]models.NPCAgent, error)
	ListBatch(status models.NPCAgentStatus, after string, limit int) ([]models.NPCAgent, error)
	UpdateStatusBatch(updates []models.AgentStatusUpdate) error
	ListAll() ([]models.NPCAgent, error)
}

// WorldSource — миры галактики с координатами. Реализация — MapCacheSource
// (адаптер mapcache.Manager); снапшот статичен между генерациями вселенной.
type WorldSource interface {
	Snapshot() *mapcache.Snapshot
}

// MapCacheSource — адаптер mapcache.Manager к WorldSource.
type MapCacheSource struct {
	m *mapcache.Manager
}

func NewMapCacheSource(m *mapcache.Manager) *MapCacheSource {
	return &MapCacheSource{m: m}
}

func (s *MapCacheSource) Snapshot() *mapcache.Snapshot {
	if s == nil || s.m == nil {
		return nil
	}
	return s.m.Snapshot()
}

// SchedulerMetrics — метрики тика планировщика (спека 26a.1 §8.1):
// last_tick_ms — занятость тика (при 100к ListAll может быть секунды),
// full_batches — кумулятивный счётчик перегрузок (прибытия заняли весь batch).
type SchedulerMetrics struct {
	LastTickMs  float64   `json:"last_tick_ms"`
	LastTickAt  time.Time `json:"last_tick_at"`
	TicksTotal  int64     `json:"ticks_total"`
	FullBatches int64     `json:"full_batches"`
}

// BulkMetrics — отчёт последней пачки массовой генерации (§8.1, пишет
// RecordBulk по завершении джоба).
type BulkMetrics struct {
	Count      int64     `json:"count"`
	DurationMs int64     `json:"duration_ms"`
	FinishedAt time.Time `json:"finished_at"`
}

// ManagerMetrics — метрики менеджера для /admin/npc/metrics (§8.2).
// LastBulk = nil → в JSON «last_bulk»: null (пачки ещё не было).
type ManagerMetrics struct {
	Scheduler SchedulerMetrics `json:"scheduler"`
	LastBulk  *BulkMetrics     `json:"last_bulk"`
}

// Manager — фоновый планировщик NPC-агентов (спека 20a.1 §3.1).
// Одна горутина, тик каждые npcTickInterval. Единственный писатель
// состояния агентов (спека §9 И1): смены состояния — batch-транзакциями
// (UpdateStatusBatch). Отказоустойчивость: агент в полёте переживает
// рестарт сервера (arrive_at абсолютен, §3.1 «Отказной режим»).
type Manager struct {
	store    AgentStore
	worlds   WorldSource
	settings *Settings
	notifier Notifier

	positions PositionCache

	// Кэш агентов для интерполяции позиций (идея 26c A2, спека 20a.1 §2.2.B
	// «позиции in-memory»): map[id]состояние, нужное для интерполяции.
	// Загрузка — один ListAll при старте/инвалидации; дальше инкремент
	// стартами и прибытиями тика. Доступ ТОЛЬКО из горутины тика (И1);
	// хендлеры внешних мутаций ставят только agentsDirty (atomic).
	agentCache  map[string]models.NPCAgent
	agentsDirty atomic.Bool // внешняя мутация npc_agents → перезагрузка кэша тиком

	// Курсоры кругового обхода (спека §2.2.A): хранятся в памяти,
	// при старте сервера сбрасываются.
	arrivalCursor string
	idleCursor    string

	// Сетка миров: перестраивается только при смене снапшота mapcache
	// (после перегенерации вселенной), не на каждом тике (И2).
	// atomic.Pointer: сетку читает и хендлер (RandomWorld/RandomWorlds) из
	// другой горутины (AGENTS.md §0); после построения сетка immutable.
	// gridMu сериализует перестройку (tick + хендлер массовой генерации).
	gridPtr      atomic.Pointer[worldGrid]
	gridMu       sync.Mutex
	gridSnapshot *mapcache.Snapshot

	// Метрики поведения (спека 26a.1 §8): атомарные счётчики тика и
	// последней пачки массовой генерации; in-memory — при рестарте
	// сбрасываются (для инструмента замеров ок, §2 Р4-B).
	lastTickMs     atomic.Int64 // длительность последнего тика, мс
	lastTickAt     atomic.Int64 // unix-нано последнего тика (0 — тика не было)
	ticksTotal     atomic.Int64
	fullBatches    atomic.Int64 // раз прибытия заняли весь batchSize (очередь не разобрана)
	bulkCount      atomic.Int64 // последняя пачка: число агентов
	bulkDurationMs atomic.Int64 // последняя пачка: длительность, мс
	bulkFinishedAt atomic.Int64 // последняя пачка: unix-нано завершения (0 — пачки не было)

	stop chan struct{}
	done chan struct{}
}

// NewManager — создаёт планировщик. settings=nil → дефолты §2.4.
func NewManager(store AgentStore, worlds WorldSource, settings *Settings) *Manager {
	if settings == nil {
		settings = DefaultSettings()
	}
	return &Manager{
		store:    store,
		worlds:   worlds,
		settings: settings,
		notifier: LogNotifier{},
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// SetNotifier — подмена приёмника прибытий (тесты, этап 5 — WS-мост).
func (m *Manager) SetNotifier(n Notifier) {
	if n != nil {
		m.notifier = n
	}
}

// Start запускает фоновую горутину планировщика.
func (m *Manager) Start() {
	go m.loop()
}

// Stop останавливает планировщик и ждёт завершения текущего тика.
func (m *Manager) Stop() {
	close(m.stop)
	<-m.done
}

// Positions — снимок позиций агентов для карты (спека §8,
// GET /api/npc/positions). O(1), без блокировок (этап 4).
func (m *Manager) Positions() []InterpolatedPosition {
	return m.positions.Snapshot()
}

// SetPositions — подмена снимка позиций (тесты хендлеров карты; в проде
// снимок пересчитывает планировщик каждый тик).
func (m *Manager) SetPositions(pos []InterpolatedPosition) {
	m.positions.Replace(pos)
}

// Settings — настройки менеджера (админ-ручка /admin/npc/settings, §6).
func (m *Manager) Settings() *Settings {
	return m.settings
}

// RandomWorld — случайный мир галактики для стартовой позиции агента
// (спека §8: стартовый мир, если не указан). Источник — сетка миров
// (atomic.Pointer — чтение из хендлера, AGENTS.md §0): предвычисленный
// allIDs, O(1) без построения слайса на каждый вызов (спека 26a.1 §4.4).
// false — снапшот карты не готов или галактика пуста.
func (m *Manager) RandomWorld() (string, bool) {
	g := m.gridPtr.Load()
	if g == nil || len(g.allIDs) == 0 {
		return "", false
	}
	return g.allIDs[rand.Intn(len(g.allIDs))], true
}

// RandomWorlds — n случайных миров галактики для стартовых позиций пачки
// (спека 26a.1 §4.1, §4.4): O(1) на агента по предвычисленному allIDs,
// повторы допустимы. false — снапшот не готов или галактика пуста.
// Вызывается из хендлера (другая горутина) — перестройка сетки под gridMu.
func (m *Manager) RandomWorlds(n int) ([]string, bool) {
	m.refreshGrid()
	g := m.gridPtr.Load()
	if g == nil || len(g.allIDs) == 0 {
		return nil, false
	}
	// Локальный rand — общие *rand.Rand не потокобезопасны (§0).
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	return g.randomWorlds(rnd, n)
}

// WorldName — имя мира по id (спека §5: уведомления {"world": {id, name}}).
// Источник — сетка миров из снапшота mapcache; "" — мира нет в сетке.
func (m *Manager) WorldName(id string) string {
	g := m.gridPtr.Load()
	if g == nil {
		return ""
	}
	w, ok := g.byID[id]
	if !ok {
		return ""
	}
	return w.name
}

func (m *Manager) loop() {
	defer close(m.done)
	for {
		select {
		case <-m.stop:
			return
		case <-time.After(m.settings.TickInterval()):
			m.safeTick()
		}
	}
}

// safeTick — тик с защитой от паники: планировщик переживает ошибку и
// продолжает тикать (как runFlight в travel, спека §3.1 «Отказной режим»).
func (m *Manager) safeTick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔥 panic in NPCManager tick: %v", r)
		}
	}()
	m.tick()
}

// tick — один тик планировщика (спека §3.1):
//  1. прибытия (приоритет): flying с arrive_at <= now → idle + last_observed_at;
//  2. старты из остатка бюджета: idle → flying (маршрут §3.2);
//  3. пересчёт позиций всех агентов для карты (§2.2.B).
//
// Метрики (спека 26a.1 §8.1): длительность тика, full_batches — прибытия
// заняли весь batchSize (очередь не разобрана за тик).
func (m *Manager) tick() {
	start := time.Now()
	defer func() {
		m.lastTickMs.Store(time.Since(start).Milliseconds())
		m.lastTickAt.Store(start.UnixNano())
		m.ticksTotal.Add(1)
	}()

	m.refreshGrid()

	// Кэш агентов (идея 26c A2): загрузка при старте (первый тик) и
	// перезагрузка после внешних мутаций (MarkDirty/OnAgentsDeleted).
	// Единственный ListAll — здесь, а не на каждый тик; единственный
	// писатель кэша — тик (И1), гонок нет.
	if m.agentsDirty.Load() || m.agentCache == nil {
		m.reloadAgentCache()
	}

	now := time.Now()
	batchSize := m.settings.BatchSize()

	// Шаг 1 (приоритет бюджета, §2.2.A): прибытия.
	arrivals, err := m.store.ListDueArrivals(now, m.arrivalCursor, batchSize)
	if err != nil {
		log.Printf("❌ NPCManager: ListDueArrivals: %v", err)
		return // бюджет неизвестен — старты не запускаем
	}
	if len(arrivals) > 0 {
		m.processArrivals(arrivals, now)
		m.arrivalCursor = arrivals[len(arrivals)-1].ID
		if len(arrivals) < batchSize {
			m.arrivalCursor = "" // конец круга — следующий тик начнёт с начала
		} else {
			m.fullBatches.Add(1) // прибытия заняли весь бюджет — очередь не разобрана
		}
	} else {
		m.arrivalCursor = ""
	}

	// Шаг 2 (из остатка бюджета): старты.
	budget := batchSize - len(arrivals)
	if budget > 0 {
		m.processStarts(budget, now)
	}

	// Шаг 3: позиции для карты.
	m.refreshPositions(now)
}

// processArrivals — прибытия: смена состояния flying → idle одной
// batch-транзакцией + уведомления (спека §3.1 п.1, §4: без пересчёта
// населения, только last_observed_at).
func (m *Manager) processArrivals(arrivals []models.NPCAgent, now time.Time) {
	updates := make([]models.AgentStatusUpdate, 0, len(arrivals))
	for _, a := range arrivals {
		if a.TargetWorldID == nil {
			log.Printf("⚠️ NPCManager: агент %s в полёте без target — пропущен", a.ID)
			continue
		}
		updates = append(updates, models.AgentStatusUpdate{
			ID:             a.ID,
			Status:         models.NPCAgentStatusIdle,
			CurrentWorldID: *a.TargetWorldID, // цель полёта становится текущим миром
			LastObservedAt: &now,
		})
		// Гейт отправки (спека 26a.1 §7.3): уведомление ⇔ глобальный рубильник
		// И per-agent флаг. Логика тика не меняется — только условие.
		if m.settings.NotifyGlobalEnabled() && a.NotifyEnabled {
			m.notifier.NotifyArrival(a, now)
		}
	}
	if len(updates) == 0 {
		return
	}
	if err := m.store.UpdateStatusBatch(updates); err != nil {
		log.Printf("❌ NPCManager: UpdateStatusBatch (прибытия): %v", err)
		return // БД не изменилась — кэш не трогаем (источник правды — БД)
	}

	// Кэш позиций (идея 26c A2): прибытие — idle в мире цели, кортеж полёта
	// сбрасывается (позиция = координаты current_world_id).
	for _, u := range updates {
		a, ok := m.agentCache[u.ID]
		if !ok {
			continue // агента нет в кэше — не интерполируем
		}
		a.Status = models.NPCAgentStatusIdle
		a.CurrentWorldID = u.CurrentWorldID
		a.FromWorldID = nil
		a.TargetWorldID = nil
		a.DepartAt = nil
		a.ArriveAt = nil
		m.agentCache[u.ID] = a
	}
}

// processStarts — запуск полётов idle → flying из остатка бюджета тика
// (спека §3.1 п.2, §3.2): маршрут по сетке миров, длительность
// max(3с, dist × npcSpeedFactor), без потолка (вариант A).
func (m *Manager) processStarts(budget int, now time.Time) {
	g := m.gridPtr.Load()
	if g == nil {
		return // снапшот карты не готов — полёты не запускаем
	}

	idle, err := m.store.ListBatch(models.NPCAgentStatusIdle, m.idleCursor, budget)
	if err != nil {
		log.Printf("❌ NPCManager: ListBatch(idle): %v", err)
		return
	}
	if len(idle) == 0 {
		m.idleCursor = ""
		return
	}

	// Локальный rand на тик — общие *rand.Rand не потокобезопасны (§0).
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	speedFactor := m.settings.SpeedFactor()

	updates := make([]models.AgentStatusUpdate, 0, len(idle))
	for _, a := range idle {
		cx, cy, ok := g.coordsOf(a.CurrentWorldID)
		if !ok {
			continue // мира нет в снапшоте (перегенерация вселенной) — полёт не запускаем
		}
		targetID, ok := g.pickTarget(cx, cy, a.CurrentWorldID, rnd)
		if !ok {
			continue // галактика пуста
		}
		tx, ty, _ := g.coordsOf(targetID)
		duration := FlightDuration(math.Hypot(tx-cx, ty-cy), speedFactor)
		depart := now
		arrive := now.Add(duration)
		updates = append(updates, models.AgentStatusUpdate{
			ID:            a.ID,
			Status:        models.NPCAgentStatusFlying,
			FromWorldID:   a.CurrentWorldID,
			TargetWorldID: targetID,
			DepartAt:      &depart,
			ArriveAt:      &arrive,
		})
	}

	if len(updates) > 0 {
		if err := m.store.UpdateStatusBatch(updates); err != nil {
			log.Printf("❌ NPCManager: UpdateStatusBatch (старты): %v", err)
		} else {
			// Кэш позиций (идея 26c A2): старт — flying с кортежем полёта
			// (интерполяция from → target по depart/arrive). Обновляем только
			// при успехе БД (источник правды — БД).
			for _, u := range updates {
				a, ok := m.agentCache[u.ID]
				if !ok {
					continue // агента нет в кэше — не интерполируем
				}
				a.Status = models.NPCAgentStatusFlying
				// Локальные копии: переменная цикла u переиспользуется (go 1.21,
				// per-iteration variables — с 1.22), &u.FromWorldID дал бы один
				// указатель на всех стартовавших батча (фикс ревью 26c A2).
				from := u.FromWorldID
				target := u.TargetWorldID
				a.FromWorldID = &from
				a.TargetWorldID = &target
				a.DepartAt = u.DepartAt
				a.ArriveAt = u.ArriveAt
				m.agentCache[u.ID] = a
			}
		}
	}

	// Курсор продвигается всегда (пропущенные агенты — кандидаты
	// следующего круга).
	m.idleCursor = idle[len(idle)-1].ID
	if len(idle) < budget {
		m.idleCursor = ""
	}
}

// refreshGrid — перестраивает сетку миров только при смене снапшота
// mapcache (после генерации вселенной), не на каждом тике (спека §9 И2).
// gridMu сериализует перестройку: сетку строит и тик, и хендлер массовой
// генерации (RandomWorlds) — gridSnapshot иначе был бы data race (§0).
func (m *Manager) refreshGrid() {
	m.gridMu.Lock()
	defer m.gridMu.Unlock()
	snap := m.worlds.Snapshot()
	if snap == nil {
		return // снапшот карты ещё не готов
	}
	if snap == m.gridSnapshot {
		return
	}
	m.gridSnapshot = snap
	m.gridPtr.Store(buildGrid(snap.Worlds()))
}

// refreshPositions — пересчёт позиций всех агентов на момент now
// (спека §2.2.B: 100k интерполяций на тик — тривиально для CPU);
// источник правды — in-memory кэш агентов (идея 26c A2), координаты
// миров — сетка. ListAll из горячего пути тика убран: кэш загружается
// один раз (старт/инвалидация) и обновляется инкрементально.
func (m *Manager) refreshPositions(now time.Time) {
	// Внешняя мутация во время тика (dirty) — не пишем позиции из
	// устаревшего кэша: хендлер уже сбросил Positions (OnAgentsDeleted),
	// следующий тик перезагрузит кэш. Иначе на карту вернулись бы
	// «призраки» удалённых.
	if m.agentsDirty.Load() {
		return
	}
	g := m.gridPtr.Load() // nil обрабатывается coordsOf (позиция не отдаётся)
	positions := make([]InterpolatedPosition, 0, len(m.agentCache))
	for _, a := range m.agentCache {
		p, ok := interpolatePosition(a, g, now)
		if !ok {
			continue // мира нет в сетке — позицию не отдаём
		}
		positions = append(positions, p)
	}
	m.positions.Replace(positions)
}

// ==================== МЕТРИКИ (спека 26a.1 §8) ====================

// RecordBulk — метрика завершённой пачки массовой генерации (§8.1:
// пишет джоб по завершении; last_bulk виден в админке и в отчёте джоба).
func (m *Manager) RecordBulk(count int, duration time.Duration) {
	m.bulkCount.Store(int64(count))
	m.bulkDurationMs.Store(duration.Milliseconds())
	m.bulkFinishedAt.Store(time.Now().UnixNano())
}

// OnAgentsDeleted — очистка in-memory состояния после массового удаления
// агентов (правка 2026-09-15): позиции для карты — чтобы между DELETE и
// следующим тиком (5с) снапшот не показывал «призраков» удалённых; метрика
// last_bulk сбрасывается (пачки больше нет); кэш агентов инвалидируется
// (dirty — следующий тик перезагрузит его из БД, идея 26c A2). Счётчики
// планировщика (тики/полные батчи) не трогаем — это метрики работы, не
// данных. Сам кэш не мутируем (И1: единственный писатель — тик).
func (m *Manager) OnAgentsDeleted() {
	m.positions.Replace(nil)
	m.bulkCount.Store(0)
	m.bulkDurationMs.Store(0)
	m.bulkFinishedAt.Store(0)
	m.agentsDirty.Store(true)
}

// MarkDirty — инвалидация кэша агентов после внешней мутации npc_agents
// (хендлеры админки, другие горутины — AGENTS.md §0): следующий тик
// перезагрузит кэш одним ListAll. Только atomic-флаг (И1: единственный
// писатель состояния — тик).
func (m *Manager) MarkDirty() {
	m.agentsDirty.Store(true)
}

// IsAgentsDirty — флаг инвалидации кэша агентов (внешняя мутация ждёт
// перезагрузки тиком). Для диагностики и тестов.
func (m *Manager) IsAgentsDirty() bool {
	return m.agentsDirty.Load()
}

// Metrics — срез метрик для /admin/npc/metrics (§8.2): in-memory счётчики
// тика и последней пачки. LastBulk = nil, пока пачка не запускалась
// (с рестарта — снова nil: метрики in-memory, для инструмента замеров ок).
func (m *Manager) Metrics() ManagerMetrics {
	mm := ManagerMetrics{
		Scheduler: SchedulerMetrics{
			LastTickMs:  float64(m.lastTickMs.Load()),
			LastTickAt:  time.Unix(0, m.lastTickAt.Load()),
			TicksTotal:  m.ticksTotal.Load(),
			FullBatches: m.fullBatches.Load(),
		},
	}
	if m.bulkFinishedAt.Load() != 0 {
		mm.LastBulk = &BulkMetrics{
			Count:      m.bulkCount.Load(),
			DurationMs: m.bulkDurationMs.Load(),
			FinishedAt: time.Unix(0, m.bulkFinishedAt.Load()),
		}
	}
	return mm
}