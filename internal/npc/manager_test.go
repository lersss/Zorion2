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
	agents     []models.NPCAgent
	idleCalls  int
	dueCalls   int
	updateBat  int
	panic      bool // паника в ListDueArrivals (тест recover)
	dueError   bool // ошибка в ListDueArrivals (мягкая деградация)
	updateErr  bool
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
			f.agents[i].FromWorldID = &u.FromWorldID
			f.agents[i].TargetWorldID = &u.TargetWorldID
			f.agents[i].DepartAt = u.DepartAt
			f.agents[i].ArriveAt = u.ArriveAt
		}
	}
	return nil
}

func (f *fakeStore) ListAll() ([]models.NPCAgent, error) { return f.agents, nil }

// fakeWorlds — WorldSource без снапшота (grid подкладывается в тесте).
type fakeWorlds struct{}

func (fakeWorlds) Snapshot() *mapcache.Snapshot { return nil }

type silentNotifier struct{}

func (silentNotifier) NotifyArrival(models.NPCAgent, time.Time) {}

type recordingNotifier struct{ calls int }

func (r *recordingNotifier) NotifyArrival(models.NPCAgent, time.Time) { r.calls++ }

func newTestManager(store AgentStore) *Manager {
	m := NewManager(store, fakeWorlds{}, DefaultSettings())
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

// Уведомление шлётся только при notify_enabled (спека §5).
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

// RandomWorld — случайный мир из сетки (спека §8: стартовый мир, если не
// указан); false, если сетки нет.
func TestManagerRandomWorld(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	m := newTestManager(&fakeStore{})

	_, ok := m.RandomWorld()
	require.False(t, ok, "сетки нет — мира нет")

	m.gridPtr.Store(grid)
	id, ok := m.RandomWorld()
	require.True(t, ok)
	require.Contains(t, []string{"w1", "w2"}, id)
}

// WorldName — имя мира для уведомлений из сетки (спека §5).
func TestManagerWorldName(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", Name: "Sirius", X: 0, Y: 0}})
	m := newTestManager(&fakeStore{})

	require.Equal(t, "", m.WorldName("w1"), "сетки нет — имени нет")
	m.gridPtr.Store(grid)
	require.Equal(t, "Sirius", m.WorldName("w1"))
	require.Equal(t, "", m.WorldName("nope"), "нет мира — пустое имя")
}