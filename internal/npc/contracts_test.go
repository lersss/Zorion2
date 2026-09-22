// internal/npc/contracts_test.go
//
// B2b: NPC-агент как исполнитель контракта-перелёта (спека
// 2026-09-22-контракт-перелёт-и-доска §1.5). Тесты планировщика: взятие только
// при совпадении маршрута, отсутствие отдельного поиска, гонка на маршруте,
// закрытие по прибытии, nil-безопасность и мягкая деградация при ошибке чтения.
package npc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zorion/internal/mapcache"
	"zorion/internal/models"
)

// fakeContractStore — контракты-перелёты в памяти: маршруты, взятия, прибытия.
type fakeContractStore struct {
	routes      []models.TravelContractRef
	takeResults map[string]bool // contractID → результат TakeTravel (по умолчанию true)
	taken       []struct {
		contractID, agentID string
		expiresAt           *time.Time
	}
	arrivals []models.AgentTravelArrival
	listErr  bool
}

func (f *fakeContractStore) ListOpenSystemTravels() ([]models.TravelContractRef, error) {
	if f.listErr {
		return nil, errTest
	}
	return f.routes, nil
}

func (f *fakeContractStore) TakeTravel(contractID, agentID string, expiresAt *time.Time) (bool, error) {
	f.taken = append(f.taken, struct {
		contractID, agentID string
		expiresAt           *time.Time
	}{contractID, agentID, expiresAt})
	return f.takeResults[contractID] || f.takeResults == nil, nil
}

func (f *fakeContractStore) CloseArrivedTravels(arrivals []models.AgentTravelArrival) (int, error) {
	f.arrivals = append(f.arrivals, arrivals...)
	return len(arrivals), nil
}

// (а) Маршрут совпал: агент берёт открытый перелёт из текущей системы в
// выбранную цель и летит как обычно.
func TestManagerAgentTakesMatchingTravel(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	contracts := &fakeContractStore{routes: []models.TravelContractRef{
		{ID: "c1", FromWorldID: "w1", DestWorldID: "w2"},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)
	m.SetContracts(contracts)
	m.settings.SetBatchSize(1)

	m.tick()

	require.Len(t, contracts.taken, 1, "маршрут совпал — контракт взят")
	require.Equal(t, "c1", contracts.taken[0].contractID)
	require.Equal(t, "a1", contracts.taken[0].agentID)
	// Срок перебазирован от взятия (§4.3): dist=100 → ref 30с → срок 45с.
	require.NotNil(t, contracts.taken[0].expiresAt, "срок перебазирован при взятии агентом")
	require.WithinDuration(t, time.Now().Add(models.TravelDeadline(100)),
		*contracts.taken[0].expiresAt, 2*time.Second)
	require.Equal(t, models.NPCAgentStatusFlying, store.agents[0].Status, "полёт стартует как обычно")
}

// (а) Маршрут не совпал (другой from или другой dest) — контракт не берётся.
func TestManagerAgentSkipsOtherRoute(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	contracts := &fakeContractStore{routes: []models.TravelContractRef{
		{ID: "c1", FromWorldID: "w1", DestWorldID: "wX"}, // не тот dest
		{ID: "c2", FromWorldID: "w7", DestWorldID: "w2"}, // не тот from
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)
	m.SetContracts(contracts)
	m.settings.SetBatchSize(1)

	m.tick()

	require.Empty(t, contracts.taken, "маршрут не совпал — контрактов не берём")
	require.Equal(t, models.NPCAgentStatusFlying, store.agents[0].Status)
}

// Гонка на маршруте: первый контракт занят (TakeTravel=false) → агент берёт
// следующий (атомарность взятия отсекает повторного исполнителя).
func TestManagerAgentTravelTakeRace(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	contracts := &fakeContractStore{
		routes: []models.TravelContractRef{
			{ID: "c1", FromWorldID: "w1", DestWorldID: "w2"},
			{ID: "c2", FromWorldID: "w1", DestWorldID: "w2"},
		},
		takeResults: map[string]bool{"c1": false, "c2": true},
	}
	m := newTestManager(store)
	m.gridPtr.Store(grid)
	m.SetContracts(contracts)
	m.settings.SetBatchSize(1)

	m.tick()

	require.Len(t, contracts.taken, 2, "занятый контракт пропущен, взят следующий")
	require.Equal(t, "c2", contracts.taken[1].contractID)
}

// (б) Прибытие агента закрывает его перелёт: в CloseArrivedTravels приходит
// пара (агент, мир прибытия = цель полёта).
func TestManagerAgentClosesTravelOnArrival(t *testing.T) {
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusFlying, CurrentWorldID: "w1",
			TargetWorldID: ptrStr("w2"), ArriveAt: ptrTime(time.Now().Add(-time.Second))},
	}}
	contracts := &fakeContractStore{}
	m := newTestManager(store)
	m.SetContracts(contracts)
	m.settings.SetBatchSize(1)

	m.tick()

	require.Len(t, contracts.arrivals, 1)
	require.Equal(t, "a1", contracts.arrivals[0].AgentID)
	require.Equal(t, "w2", contracts.arrivals[0].WorldID, "закрытие по цели полёта")
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[0].Status)
}

// Контракты не подключены (SetContracts не вызван) — планировщик работает
// как прежде, без паники (nil-safe).
func TestManagerWithoutContracts(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	m := newTestManager(store) // контракты не подключены
	m.gridPtr.Store(grid)

	require.NotPanics(t, func() { m.tick() })
	require.Equal(t, models.NPCAgentStatusFlying, store.agents[0].Status, "старт без контрактов")
}

// Ошибка чтения контрактов не мешает стартам: агент летит как обычно, взятий нет.
func TestManagerContractsListErrorDegrades(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	store := &fakeStore{agents: []models.NPCAgent{
		{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
	}}
	contracts := &fakeContractStore{listErr: true}
	m := newTestManager(store)
	m.gridPtr.Store(grid)
	m.SetContracts(contracts)

	require.NotPanics(t, func() { m.tick() })
	require.Empty(t, contracts.taken, "ошибка чтения — контрактов не берём")
	require.Equal(t, models.NPCAgentStatusFlying, store.agents[0].Status)
}

// Контракт берётся только ПОСЛЕ успешной записи flying: сбой UpdateStatusBatch
// → агент не полетел, контракт не взят (нет taken-висяка за нелетящим агентом).
func TestManagerAgentDoesNotTakeTravelOnStatusError(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	store := &fakeStore{
		agents: []models.NPCAgent{
			{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"},
		},
		updateErr: true,
	}
	contracts := &fakeContractStore{routes: []models.TravelContractRef{
		{ID: "c1", FromWorldID: "w1", DestWorldID: "w2"},
	}}
	m := newTestManager(store)
	m.gridPtr.Store(grid)
	m.SetContracts(contracts)
	m.settings.SetBatchSize(1)

	m.tick()

	require.Empty(t, contracts.taken, "запись flying не удалась — контракт не берём")
	require.Equal(t, models.NPCAgentStatusIdle, store.agents[0].Status, "статус не изменился")
}
