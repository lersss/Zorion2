// internal/npc/manager_test.go
// Тесты планировщика (спека 20a.1 §3.1, §2.2.A, этап 2): переходы
// idle→flying→idle, курсорная batch-обработка, приоритет прибытий над
// стартами, recover не роняет менеджера, позиции для карты, уведомления.
package npc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zorion/internal/mapcache"
	"zorion/internal/models"
)

var errTest = errors.New("test error")

// ==================== FAKE-ЗАВИСИМОСТИ ====================

// fakeStore — хранилище агентов в памяти с семантикой курсорных выборок
// репозитория (ListDueArrivals/ListBatch фильтруют по id > after и limit).
type fakeStore struct {
	agents       []models.NPCAgent
	idleCalls    int
	dueCalls     int
	updateBat    int
	listAllCalls int // вызовы ListAll (идея 26c A2: кэш загружается один раз, не на тик)
	panic        bool // паника в ListDueArrivals (тест recover)
	dueError     bool // ошибка в ListDueArrivals (мягкая деградация)
	updateErr    bool
}

func (f *fakeStore) ListDueArrivals(now time.Time, after string, limit int) ([]models.NPCAgent, error) {
	f.dueCalls++
	if f.panic {
		panic("boom")
	}
	if f.dueError {
		return nil, errTest
	}
	var out []models.NPCAgent
	for _, a := range f.agents {
		if a.Status == models.NPCAgentStatusFlying && a.ArriveAt != nil &&
			!a.ArriveAt.After(now) && a.ID > after {
			out = append(out, a)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeStore) ListBatch(status models.NPCAgentStatus, after string, limit int) ([]models.NPCAgent, error) {
	f.idleCalls++
	var out []models.NPCAgent
	for _, a := range f.agents {
		if a.Status == status && a.ID > after {
			out = append(out, a)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeStore) UpdateStatusBatch(updates []models.AgentStatusUpdate) error {
	f.updateBat++
	if f.updateErr {
		return errTest
	}
	byID := map[string]int{}
	for i := range f.agents {
		byID[f.agents[i].ID] = i
	}
	for _, u := range updates {
		i, ok := byID[u.ID]
		if !ok {
			continue
		}
		switch u.Status {
		case models.NPCAgentStatusIdle: // прибытие (как SQL репозитория)
			f.agents[i].Status = models.NPCAgentStatusIdle
			f.agents[i].CurrentWorldID = u.CurrentWorldID
			f.agents[i].LastObservedAt = u.LastObservedAt
		case models.NPCAgentStatusFlying: // старт
			f.agents[i].Status = models.NPCAgentStatusFlying
			// Локальные копии — та же защита от алиасинга переменной цикла
			// (go 1.21), что и в инкременте кэша manager.go (фикс ревью 26c A2).
			from := u.FromWorldID
			target := u.TargetWorldID
			f.agents[i].FromWorldID = &from
			f.agents[i].TargetWorldID = &target
			f.agents[i].DepartAt = u.DepartAt
			f.agents[i].ArriveAt = u.ArriveAt
		}
	}
	return nil
}

func (f *fakeStore) ListAll() ([]models.NPCAgent, error) {
	f.listAllCalls++
	return f.agents, nil
}

// fakeWorlds — WorldSource без снапшота (grid подкладывается в тесте).
type fakeWorlds struct{}

func (fakeWorlds) Snapshot() *mapcache.Snapshot { return nil }

// snapshotWorlds — WorldSource с готовым снапшотом (тесты пула расы).
type snapshotWorlds struct{ snap *mapcache.Snapshot }

func (s snapshotWorlds) Snapshot() *mapcache.Snapshot { return s.snap }

// fakeRaceSource — RaceHomeworldSource в памяти (пул «раса → родной мир»).
type fakeRaceSource struct {
	origins []RaceHomeworld
	calls   int
	err     error
}

func (f *fakeRaceSource) RaceHomeworlds() ([]RaceHomeworld, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.origins, nil
}

type silentNotifier struct{}

func (silentNotifier) NotifyArrival(models.NPCAgent, time.Time) {}

type recordingNotifier struct{ calls int }

func (r *recordingNotifier) NotifyArrival(models.NPCAgent, time.Time) { r.calls++ }

func newTestManager(store AgentStore) *Manager {
	m := NewManager(store, fakeWorlds{}, nil, DefaultSettings())
	m.SetNotifier(silentNotifier{})
	return m
}

// ==================== ПЕРЕХОДЫ ====================

// Полный цикл: idle → flying (кортеж полёта, длительность ∝ dist) →
// idle (current = цель, last_observed_at = now, без пересчёта населения).
func TestManagerFullCycleIdleFlyingIdle(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Name: "A", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)

	// Тик 1: старт.
	m.tick()
	a := store.agents[0]
	require.Equal(t, models.NPCAgentStatusFlying, a.Status)
	require.Equal(t, "w1", *a.FromWorldID)
	require.Equal(t, "w2", *a.TargetWorldID, "цель — единственный мир в радиусе, не текущий")
	require.NotNil(t, a.DepartAt)
	require.NotNil(t, a.ArriveAt)
	// dist=100 → duration = 100 × 0.3 = 30с (мин. 3с не задействован).
	require.InDelta(t, 30.0, a.ArriveAt.Sub(*a.DepartAt).Seconds(), 0.01)

	// Агент «долетел». Тик 2 с бюджетом 1: бюджет уходит на прибытие,
	// старты не запускаются — состояние «прибыл» наблюдаемо.
	store.agents[0].ArriveAt = ptrTime(time.Now().Add(-time.Second))
	m.settings.SetBatchSize(1)
	m.tick()
	a = store.agents[0]
	require.Equal(t, models.NPCAgentStatusIdle, a.Status)
	require.Equal(t, "w2", a.CurrentWorldID, "цель полёта становится текущим миром")
	require.NotNil(t, a.LastObservedAt, "наблюдение — только отметка last_observed_at (спека §4)")
}

// Курсорная обработка по батчам: бюджет 2, три idle-агента — за два тика
// обработаны все, курсор продвигается и сбрасывается в конце круга.
func TestManagerCursorBatches(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
		{ID: "a2", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
		{ID: "a3", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)
	m.settings.SetBatchSize(2)

	m.tick()
	require.Equal(t, models.NPCAgentStatusFlying, store.agents[0].Status, "a1 стартовал")
	require.Equal(t, models.NPCAgentStatusFlying, store.agents[1].Status, "a2 стартовал")
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[2].Status, "a3 вне бюджета тика")
	require.Equal(t, "a2", m.idleCursor, "курсор — последний обработанный id")

	m.tick()
	require.Equal(t, models.NPCAgentStatusFlying, store.agents[2].Status, "a3 стартовал вторым тиком")
	require.Equal(t, "", m.idleCursor, "batch меньше лимита — конец круга, курсор сброшен")
}

// Приоритет прибытий: полный бюджет съедают прибытия — старты не
// запускаются (ListBatch(idle) не вызывается).
func TestManagerArrivalsPriorityOverStarts(t *testing.T) {
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			TargetWorldID: ptrStr("w2"), ArriveAt: ptrTime(time.Now().Add(-time.Second))},
		{ID: "a2", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	m := newTestManager(store)
	m.settings.SetBatchSize(1)

	m.tick()
	require.Equal(t, 1, store.dueCalls)
	require.Equal(t, 0, store.idleCalls, "бюджет исчерпан прибытиями — старты не запускались")
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[0].Status, "прибытие обработано")
	require.Equal(t, "w2", store.agents[0].CurrentWorldID)
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[1].Status, "idle-агент ждёт следующего тика")
}

// Мягкая деградация: ошибка чтения прибытий — тик не падает, старты не
// запускаются (бюджет неизвестен), следующий тик работает.
func TestManagerArrivalsErrorDegrades(t *testing.T) {
	store := &fakeStore{dueError: true}
	m := newTestManager(store)
	m.settings.SetBatchSize(1)

	m.tick() // ошибка ListDueArrivals
	require.Equal(t, 0, store.idleCalls, "при ошибке прибытий старты не запускаются")

	store.dueError = false
	store.agents = []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			TargetWorldID: ptrStr("w2"), ArriveAt: ptrTime(time.Now().Add(-time.Second))},
	}
	m.tick()
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[0].Status, "следующий тик работает")
}

// ==================== RECOVER ====================

// Паника внутри тика не роняет менеджера: recover в safeTick, следующий
// тик работает (спека §3.1 «Отказной режим»).
func TestManagerRecoverKeepsTicking(t *testing.T) {
	store := &fakeStore{panic: true}
	m := newTestManager(store)

	require.NotPanics(t, func() { m.safeTick() }, "паника в ListDueArrivals поглощается")

	store.panic = false
	store.agents = []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			TargetWorldID: ptrStr("w2"), ArriveAt: ptrTime(time.Now().Add(-time.Second))},
	}
	require.NotPanics(t, func() { m.safeTick() }, "после паники тик продолжает работать")
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[0].Status)
}

// ==================== ПОЗИЦИИ ====================

// Позиции пересчитываются на тик: flying — интерполяция, idle — текущий
// мир (спека §2.2.B).
func TestManagerPositionsRefreshed(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	depart := time.Now().Add(-50 * time.Second)
	arrive := time.Now().Add(50 * time.Second)
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Name: "A", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			FromWorldID: ptrStr("w1"), TargetWorldID: ptrStr("w2"),
			DepartAt: &depart, ArriveAt: &arrive},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)

	m.tick()
	pos := m.Positions()
	require.Len(t, pos, 1)
	require.Equal(t, "a1", pos[0].ID)
	require.InDelta(t, 50.0, pos[0].X, 0.5, "середина пути w1→w2")
	require.InDelta(t, 0.0, pos[0].Y, 0.001)
}

// ==================== УВЕДОМЛЕНИЯ ====================

// Уведомление шлётся только при notify_enabled (спека §5) И включённом
// глобальном рубильнике (спека 26a.1 §7.3 — здесь рубильник включён,
// проверяется per-agent флаг).
func TestManagerNotifyOnlyWhenEnabled(t *testing.T) {
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			TargetWorldID: ptrStr("w2"), ArriveAt: ptrTime(time.Now().Add(-time.Second)),
			NotifyEnabled: true},
		{ID: "a2", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			TargetWorldID: ptrStr("w3"), ArriveAt: ptrTime(time.Now().Add(-time.Second)),
			NotifyEnabled: false},
	}}
	notifier := &recordingNotifier{}
	m := newTestManager(store)
	m.SetNotifier(notifier)
	m.settings.SetBatchSize(1)
	m.settings.SetNotifyGlobalEnabled(true) // рубильник включён — гейт по per-agent

	m.tick() // бюджет 1: прибытие a1 (notify=true)
	m.tick() // прибытие a2 (notify=false)
	require.Equal(t, 1, notifier.calls, "уведомление только при notify_enabled")
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[0].Status)
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[1].Status)
}

// ==================== СТАРТ БЕЗ МИРОВ ====================

// Нет сетки миров (снапшот карты не готов / галактика пуста) — старты не
// запускаются, агент остаётся idle, паники нет.
func TestManagerStartSkipsWithoutWorlds(t *testing.T) {
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	m := newTestManager(store)
	// gridPtr = nil — мир w1 отсутствует в сетке.

	m.tick()
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[0].Status, "нет мира — полёт не запускается")
}

// ==================== СЛУЧАЙНЫЙ СТАРТОВЫЙ МИР ====================

// WorldName — имя мира для уведомлений из сетки (спека §5).
func TestManagerWorldName(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", Name: "Sirius", X: 0, Y: 0}})
	m := newTestManager(&fakeStore{})

	require.Equal(t, "", m.WorldName("w1"), "сетки нет — имени нет")
	m.gridPtr.Store(grid)
	require.Equal(t, "Sirius", m.WorldName("w1"))
	require.Equal(t, "", m.WorldName("nope"), "нет мира — пустое имя")
}

// ==================== МЕТРИКИ (спека 26a.1 §8) ====================

// Метрики тика: после тика ticks_total/last_tick_at/last_tick_ms заполнены;
// прибытия заняли весь batch — full_batches растёт (очередь не разобрана).
// last_bulk = null, пока пачка не запускалась.
func TestManagerTickMetrics(t *testing.T) {
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			TargetWorldID: ptrStr("w2"), ArriveAt: ptrTime(time.Now().Add(-time.Second))},
	}}
	m := newTestManager(store)
	m.settings.SetBatchSize(1)

	m.tick()

	mm := m.Metrics()
	require.Equal(t, int64(1), mm.Scheduler.TicksTotal)
	require.NotZero(t, mm.Scheduler.LastTickAt, "last_tick_at — время последнего тика")
	require.GreaterOrEqual(t, mm.Scheduler.LastTickMs, float64(0))
	require.Equal(t, int64(1), mm.Scheduler.FullBatches,
		"прибытия заняли весь batchSize — очередь не разобрана за тик")
	require.Nil(t, mm.LastBulk, "пачки массовой генерации не было — last_bulk null")

	m.RecordBulk(100, 2300*time.Millisecond)
	mm = m.Metrics()
	require.NotNil(t, mm.LastBulk)
	require.Equal(t, int64(100), mm.LastBulk.Count)
	require.Equal(t, int64(2300), mm.LastBulk.DurationMs)
	require.NotZero(t, mm.LastBulk.FinishedAt)
}

// ==================== ПУЛ «РАСА → РОДНОЙ МИР» (спека 2026-09-23 §5) ==========

// RandomRaceHomeworlds — происхождения агентов: случайная раса, а мир — родной
// мир этой расы (§5.2). Пул подгружается из источника при первом чтении;
// повторное чтение без смены снапшота источник не дёргает (И8).
func TestManagerRandomRaceHomeworlds(t *testing.T) {
	snap := mapcache.NewSnapshot([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	src := &fakeRaceSource{origins: []RaceHomeworld{
		{RaceID: "humans", HomeworldID: "w1"},
		{RaceID: "coastal", HomeworldID: "w2"},
	}}
	m := NewManager(&fakeStore{}, snapshotWorlds{snap}, src, DefaultSettings())
	m.SetNotifier(silentNotifier{})

	out, ok := m.RandomRaceHomeworlds(4)
	require.True(t, ok)
	require.Len(t, out, 4)
	byRace := map[string]string{"humans": "w1", "coastal": "w2"}
	for _, o := range out {
		require.Equal(t, byRace[o.RaceID], o.WorldID, "мир агента — родной мир его расы")
	}
	require.Equal(t, 1, src.calls, "снапшот не менялся — повторное чтение источника не нужно")

	m.RandomRaceHomeworlds(1)
	require.Equal(t, 1, src.calls, "пул берётся из снимка, источник не дёргается")
}

// Пул пуст (фракции не сгенерированы / источник пуст) → false (§5.4: отказ,
// не пустая пачка).
func TestManagerRandomRaceHomeworldsEmptyPool(t *testing.T) {
	snap := mapcache.NewSnapshot([]mapcache.World{{ID: "w1"}})
	m := NewManager(&fakeStore{}, snapshotWorlds{snap}, &fakeRaceSource{}, DefaultSettings())

	_, ok := m.RandomRaceHomeworlds(3)
	require.False(t, ok, "пустой пул — происхождений нет")
}

// Смена снапшота карты (Пакман/перегенерация) → пул перестраивается: раса,
// чей родной мир съеден, из пула выпадает (спека §5.1, триггер 1 + валидация).
func TestManagerRacePoolDropsRemovedWorlds(t *testing.T) {
	mc := mapcache.NewManager()
	mc.Replace(mapcache.NewSnapshot([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}}))
	src := &fakeRaceSource{origins: []RaceHomeworld{
		{RaceID: "humans", HomeworldID: "w1"},
		{RaceID: "coastal", HomeworldID: "w2"},
	}}
	m := NewManager(&fakeStore{}, NewMapCacheSource(mc), src, DefaultSettings())

	out, ok := m.RandomRaceHomeworlds(1)
	require.True(t, ok)
	require.Len(t, m.racePoolPtr.Load().origins, 2, "до вайпа — обе расы в пуле")

	// w2 съеден: новый снапшот без него.
	mc.Replace(mapcache.NewSnapshot([]mapcache.World{{ID: "w1", X: 0, Y: 0}}))

	out, ok = m.RandomRaceHomeworlds(1)
	require.True(t, ok)
	require.Len(t, m.racePoolPtr.Load().origins, 1, "мир выпал — запись пула отброшена")
	require.Equal(t, "humans", out[0].RaceID)
	require.Equal(t, "w1", out[0].WorldID)
}

// RefreshRaceHomeworlds — явная инвалидация перечитывает источник, не меняя
// снапшот (спека §5.1, триггер 2: после генерации фракций).
func TestManagerRefreshRaceHomeworlds(t *testing.T) {
	mc := mapcache.NewManager()
	mc.Replace(mapcache.NewSnapshot([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}}))
	src := &fakeRaceSource{origins: []RaceHomeworld{{RaceID: "humans", HomeworldID: "w1"}}}
	m := NewManager(&fakeStore{}, NewMapCacheSource(mc), src, DefaultSettings())

	_, ok := m.RandomRaceHomeworlds(1)
	require.True(t, ok)
	require.Equal(t, 1, src.calls)

	// Фракции достроились: источник обогатился, снапшот карты не менялся.
	src.origins = append(src.origins, RaceHomeworld{RaceID: "coastal", HomeworldID: "w2"})
	m.RefreshRaceHomeworlds()

	require.Equal(t, 2, src.calls, "явная инвалидация перечитала источник")
	require.Len(t, m.racePoolPtr.Load().origins, 2)
}

// Источник пула не подключён (nil) — методы не паникуют, void-инвалидация
// безопасна (харднессы без фракций).
func TestManagerRacePoolWithoutSource(t *testing.T) {
	snap := mapcache.NewSnapshot([]mapcache.World{{ID: "w1"}})
	m := NewManager(&fakeStore{}, snapshotWorlds{snap}, nil, DefaultSettings())

	require.NotPanics(t, func() { m.RefreshRaceHomeworlds() })
	m.ensureRaceHomeworlds()
	_, ok := m.RandomRaceHomeworlds(1)
	require.False(t, ok)
}

// ==================== КЭШ ПОЗИЦИЙ (идея 26c A2) ====================

// (а) Стартовая загрузка кэша: первый тик делает ОДИН ListAll, позиции
// интерполируются из кэша (ListAll из тика ушёл).
func TestManagerCacheLoadsOnFirstTick(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	depart := time.Now().Add(-50 * time.Second)
	arrive := time.Now().Add(50 * time.Second)
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Name: "A", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			FromWorldID: ptrStr("w1"), TargetWorldID: ptrStr("w2"),
			DepartAt: &depart, ArriveAt: &arrive},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)

	require.Nil(t, m.agentCache, "кэш пуст до первого тика")
	require.Equal(t, 0, store.listAllCalls, "ListAll не вызывается до тика")

	m.tick()

	require.Equal(t, 1, store.listAllCalls, "первый тик загружает кэш одним ListAll")
	require.Contains(t, m.agentCache, "a1")
	pos := m.Positions()
	require.Len(t, pos, 1)
	require.Equal(t, "a1", pos[0].ID)
	require.InDelta(t, 50.0, pos[0].X, 0.5, "позиция интерполируется из кэша (середина пути)")
}

// (б) Инкрементальное обновление кэша при прибытии: idle в мире цели,
// кортеж полёта сброшен (позиция = координаты current_world_id). Бюджет 1 —
// весь уходит на прибытие, стартов нет (a1 не перезахватывается).
func TestManagerCacheArrivalUpdatesCache(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	depart := time.Now().Add(-50 * time.Second)
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Name: "A", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			FromWorldID: ptrStr("w1"), TargetWorldID: ptrStr("w2"),
			DepartAt: &depart, ArriveAt: ptrTime(time.Now().Add(-time.Second))},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)
	m.settings.SetBatchSize(1)

	m.tick()

	require.Equal(t, 1, store.listAllCalls, "загрузка кэша — один ListAll, дальше инкремент")

	a1 := m.agentCache["a1"]
	require.Equal(t, models.NPCAgentStatusIdle, a1.Status, "прибытие обновило кэш без ListAll")
	require.Equal(t, "w2", a1.CurrentWorldID, "цель полёта становится текущим миром")
	require.Nil(t, a1.FromWorldID, "кортеж полёта сброшен — статус idle")
	require.Nil(t, a1.TargetWorldID)
	require.Nil(t, a1.DepartAt)
	require.Nil(t, a1.ArriveAt)

	pos := m.Positions()
	require.Len(t, pos, 1)
	require.InDelta(t, 100.0, pos[0].X, 0.001, "idle — координаты текущего мира (цель полёта)")
}

// (б) Инкрементальное обновление кэша при старте: flying с кортежем полёта
// (интерполяция from → target по depart/arrive).
func TestManagerCacheStartUpdatesCache(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a2", Name: "B", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)
	m.settings.SetBatchSize(1)

	m.tick()

	require.Equal(t, 1, store.listAllCalls, "загрузка кэша — один ListAll, дальше инкремент")

	a2 := m.agentCache["a2"]
	require.Equal(t, models.NPCAgentStatusFlying, a2.Status, "старт обновил кэш без ListAll")
	require.Equal(t, "w1", *a2.FromWorldID)
	require.Equal(t, "w2", *a2.TargetWorldID)
	require.NotNil(t, a2.DepartAt)
	require.NotNil(t, a2.ArriveAt)

	pos := m.Positions()
	require.Len(t, pos, 1)
	require.InDelta(t, 0.0, pos[0].X, 1.0, "только стартовал — прогресс ~0, начало пути")
}

// Регрессия ревью (26c A2): алиасинг переменной цикла при go 1.21 —
// &u.FromWorldID в инкременте кэша давал бы ВСЕМ стартовавшим в одном тике
// from/target ПОСЛЕДНЕГО агента батча. Два агента из разных миров стартуют
// в одном тике: у каждого в кэше и на карте должен быть СВОЙ from.
func TestManagerStartCacheTuplesPerAgent(t *testing.T) {
	grid := buildGrid([]mapcache.World{
		{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}, {ID: "w3", X: 0, Y: 100},
	})
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Name: "A", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
		{ID: "a2", Name: "B", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w2"},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)
	m.settings.SetBatchSize(2)

	m.tick() // оба стартуют в одном тике

	a1 := m.agentCache["a1"]
	a2 := m.agentCache["a2"]
	require.Equal(t, models.NPCAgentStatusFlying, a1.Status, "a1 стартовал")
	require.Equal(t, models.NPCAgentStatusFlying, a2.Status, "a2 стартовал")

	// from у каждого — ЕГО стартовый мир, а не последний агент батча.
	require.Equal(t, "w1", *a1.FromWorldID, "a1 летит из w1 — свой кортеж")
	require.Equal(t, "w2", *a2.FromWorldID, "a2 летит из w2 — свой кортеж")
	require.NotEqual(t, *a1.FromWorldID, *a2.FromWorldID, "from разных агентов не алиасятся")

	// На карте: стартовавший летит ОТ своего мира (progress ≈ 0 → x = from.x).
	byID := map[string]InterpolatedPosition{}
	for _, p := range m.Positions() {
		byID[p.ID] = p
	}
	require.InDelta(t, 0.0, byID["a1"].X, 1.0, "a1 стартовал от w1 (x=0), а не от последнего агента батча")
	require.InDelta(t, 100.0, byID["a2"].X, 1.0, "a2 стартовал от w2 (x=100)")
}

// (в) MarkDirty (внешняя мутация — хендлер админки) → следующий тик
// перезагружает кэш одним ListAll; новые данные видны в позициях.
func TestManagerMarkDirtyReloadsCache(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	depart := time.Now().Add(-50 * time.Second)
	arrive := time.Now().Add(50 * time.Second)
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Name: "A", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			FromWorldID: ptrStr("w1"), TargetWorldID: ptrStr("w2"),
			DepartAt: &depart, ArriveAt: &arrive},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)

	m.tick() // загрузка кэша
	require.Equal(t, 1, store.listAllCalls)
	require.Len(t, m.Positions(), 1)

	// Внешняя мутация: a1 удалён, появился a3.
	store.agents = []models.NPCAgent{
		{ID: "a3", Name: "C", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}
	m.MarkDirty()
	require.True(t, m.IsAgentsDirty())

	m.tick() // dirty → перезагрузка кэша одним ListAll
	require.Equal(t, 2, store.listAllCalls, "dirty → ровно один ListAll на перезагрузку")
	require.NotContains(t, m.agentCache, "a1")
	require.Contains(t, m.agentCache, "a3")
	pos := m.Positions()
	require.Len(t, pos, 1)
	require.Equal(t, "a3", pos[0].ID, "позиции после перезагрузки — из нового состояния")
}

// MarkDirty — только atomic-флаг: сам кэш не трогает (И1: единственный
// писатель состояния агентов — тик; хендлеры — только dirty).
func TestManagerMarkDirtyFlag(t *testing.T) {
	m := newTestManager(&fakeStore{})

	require.False(t, m.IsAgentsDirty())
	m.MarkDirty()
	require.True(t, m.IsAgentsDirty())
}

// ==================== МАССОВОЕ УДАЛЕНИЕ (правка 2026-09-15) ====================

// OnAgentsDeleted — после массового удаления очищается in-memory: позиции
// для карты (между DELETE и следующим тиком нет «призраков»), last_bulk
// (пачки больше нет) и кэш агентов инвалидируется (dirty — следующий тик
// перезагрузит его из БД, идея 26c A2).
func TestManagerOnAgentsDeleted(t *testing.T) {
	m := newTestManager(&fakeStore{})

	// Позиции были (карта показывала агентов) и метрика пачки есть.
	m.positions.Replace([]InterpolatedPosition{{ID: "a1", X: 1, Y: 2}})
	m.RecordBulk(100, time.Second)
	require.NotNil(t, m.Metrics().LastBulk, "метрика пачки была")

	m.OnAgentsDeleted()

	require.Nil(t, m.Positions(), "позиции очищены — карта без призраков")
	require.Nil(t, m.Metrics().LastBulk, "last_bulk сброшен")
	require.True(t, m.IsAgentsDirty(), "кэш агентов инвалидирован — следующий тик перезагрузит из БД")
}

// ==================== ГЛОБАЛЬНЫЙ РУБИЛЬНИК ПУШЕЙ (спека 26a.1 §7.3) ====================

// Уведомление ⇔ notifyGlobalEnabled И agent.notify_enabled. Рубильник
// выключен по умолчанию: даже notify_enabled=true не шлёт; после включения —
// только включённые агенты.
func TestManagerNotifyGlobalGate(t *testing.T) {
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			TargetWorldID: ptrStr("w2"), ArriveAt: ptrTime(time.Now().Add(-time.Second)),
			NotifyEnabled: true},
		{ID: "a2", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			TargetWorldID: ptrStr("w3"), ArriveAt: ptrTime(time.Now().Add(-time.Second)),
			NotifyEnabled: false},
	}}
	notifier := &recordingNotifier{}
	m := newTestManager(store)
	m.SetNotifier(notifier)
	m.settings.SetBatchSize(1)

	// Рубильник off (дефолт) — уведомлений нет даже у включённых.
	m.tick()
	m.tick()
	require.Equal(t, 0, notifier.calls, "рубильник выключен — уведомлений нет")

	// Включили: шлёт только notify_enabled=true.
	store.agents[0].Status = models.NPCAgentStatusFlying
	store.agents[0].CurrentWorldID = "w1"
	store.agents[0].ArriveAt = ptrTime(time.Now().Add(-time.Second))
	store.agents[1].Status = models.NPCAgentStatusFlying
	store.agents[1].CurrentWorldID = "w1"
	store.agents[1].ArriveAt = ptrTime(time.Now().Add(-time.Second))
	m.settings.SetNotifyGlobalEnabled(true)

	m.tick() // a1: notify=true → шлёт
	m.tick() // a2: notify=false → не шлёт
	require.Equal(t, 1, notifier.calls, "при включённом рубильнике шлёт только включённых")
}