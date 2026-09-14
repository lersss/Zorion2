package npc

import (
	"log"
	"math"
	"math/rand"
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

	// Курсоры кругового обхода (спека §2.2.A): хранятся в памяти,
	// при старте сервера сбрасываются.
	arrivalCursor string
	idleCursor    string

	// Сетка миров: перестраивается только при смене снапшота mapcache
	// (после перегенерации вселенной), не на каждом тике (И2).
	// atomic.Pointer: сетку читает и хендлер (RandomWorld) из другой
	// горутины (AGENTS.md §0); после построения сетка immutable.
	gridPtr      atomic.Pointer[worldGrid]
	gridSnapshot *mapcache.Snapshot

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

// Settings — настройки менеджера (админ-ручка /admin/npc/settings, §6).
func (m *Manager) Settings() *Settings {
	return m.settings
}

// RandomWorld — случайный мир галактики для стартовой позиции агента
// (спека §8: стартовый мир, если не указан). Источник — сетка миров
// (atomic.Pointer — чтение из хендлера, AGENTS.md §0). false — снапшот
// карты не готов или галактика пуста.
func (m *Manager) RandomWorld() (string, bool) {
	g := m.gridPtr.Load()
	if g == nil || len(g.byID) == 0 {
		return "", false
	}
	ids := make([]string, 0, len(g.byID))
	for id := range g.byID {
		ids = append(ids, id)
	}
	return ids[rand.Intn(len(ids))], true
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
func (m *Manager) tick() {
	m.refreshGrid()

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
		if a.NotifyEnabled {
			m.notifier.NotifyArrival(a, now)
		}
	}
	if len(updates) == 0 {
		return
	}
	if err := m.store.UpdateStatusBatch(updates); err != nil {
		log.Printf("❌ NPCManager: UpdateStatusBatch (прибытия): %v", err)
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
func (m *Manager) refreshGrid() {
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
// источник правды — БД (ListAll), координаты миров — сетка.
func (m *Manager) refreshPositions(now time.Time) {
	agents, err := m.store.ListAll()
	if err != nil {
		log.Printf("❌ NPCManager: ListAll: %v", err)
		return
	}
	g := m.gridPtr.Load() // nil обрабатывается coordsOf (позиция не отдаётся)
	positions := make([]InterpolatedPosition, 0, len(agents))
	for _, a := range agents {
		p, ok := interpolatePosition(a, g, now)
		if !ok {
			continue // мира нет в сетке — позицию не отдаём
		}
		positions = append(positions, p)
	}
	m.positions.Replace(positions)
}