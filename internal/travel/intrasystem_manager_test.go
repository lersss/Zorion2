// internal/travel/intrasystem_manager_test.go
// Тесты IntrasystemManager (спека 99.2.27 §3.3): старт пишет, редирект
// обновляет, прибытие удаляет после onArrival, cancel удаляет, Restore:
// будущий/прошлый/битая цель, порядок Restore — межзвёздная побеждает
// (С-1), intra удаляется без onArrival.
package travel

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// fakeIntraStore — in-memory реализация IntraFlightStore для тестов.
type fakeIntraStore struct {
	mu   sync.Mutex
	rows map[string]models.PlayerIntrasystemFlight
}

func newFakeIntraStore() *fakeIntraStore {
	return &fakeIntraStore{rows: map[string]models.PlayerIntrasystemFlight{}}
}

func (s *fakeIntraStore) Upsert(f models.PlayerIntrasystemFlight) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[f.UserID] = f
	return nil
}

func (s *fakeIntraStore) Delete(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, userID)
	return nil
}

func (s *fakeIntraStore) ListAll() ([]models.PlayerIntrasystemFlight, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.PlayerIntrasystemFlight, 0, len(s.rows))
	for _, f := range s.rows {
		out = append(out, f)
	}
	return out, nil
}

func (s *fakeIntraStore) get(userID string) (models.PlayerIntrasystemFlight, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.rows[userID]
	return f, ok
}

func (s *fakeIntraStore) has(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.rows[userID]
	return ok
}

// ==================== БАЗОВЫЕ ПРОВЕРКИ ====================

func TestNewIntrasystemManagerEmpty(t *testing.T) {
	m := NewIntrasystemManager(nil)
	assert.False(t, m.IsInIntraFlight("user-1"))
	assert.Nil(t, m.GetIntraFlight("user-1"))
}

// ==================== СТАРТ ====================

func TestStartIntraFlightRegistersFlight(t *testing.T) {
	m := NewIntrasystemManager(nil)

	const userID, worldID = "user-1", "world-A"
	m.StartIntraFlight(userID, worldID, "star", worldID, "planet", "p1", time.Hour, nil)

	flight := m.GetIntraFlight(userID)
	require.NotNil(t, flight, "полёт должен зарегистрироваться сразу")
	assert.Equal(t, userID, flight.UserID)
	assert.Equal(t, worldID, flight.WorldID)
	assert.Equal(t, "star", flight.FromType)
	assert.Equal(t, worldID, flight.FromID)
	assert.Equal(t, "planet", flight.ToType)
	assert.Equal(t, "p1", flight.ToID)
	assert.Equal(t, time.Hour, flight.ArriveAt.Sub(flight.StartTime))
	assert.True(t, m.IsInIntraFlight(userID))
}

// Прибытие вызывает onArrival с информацией полёта (to_type/to_id для позиции).
func TestIntraArrivalCallsCallback(t *testing.T) {
	m := NewIntrasystemManager(nil)

	arrived := make(chan string, 1)
	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", 30*time.Millisecond,
		func(userID string, f *IntraFlightInfo) {
			arrived <- userID + ":" + f.ToType + ":" + f.ToID
		})

	select {
	case msg := <-arrived:
		assert.Equal(t, "user-1:planet:p1", msg)
	case <-time.After(2 * time.Second):
		t.Fatalf("onArrival не вызван в течение 2 секунд")
	}

	require.Eventually(t, func() bool { return !m.IsInIntraFlight("user-1") }, time.Second, 5*time.Millisecond)
	assert.Nil(t, m.GetIntraFlight("user-1"))
}

// ==================== РЕДИРЕКТ ====================

// Редирект (другая цель при активном полёте): старый полёт отменяется
// (onArrival не вызывается), новый стартует от объекта отправления.
func TestIntraRedirectCancelsOldFlight(t *testing.T) {
	m := NewIntrasystemManager(nil)

	oldArrived := make(chan string, 1)
	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", time.Hour,
		func(userID string, f *IntraFlightInfo) { oldArrived <- f.ToID })

	newArrived := make(chan string, 1)
	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p2", 30*time.Millisecond,
		func(userID string, f *IntraFlightInfo) { newArrived <- f.ToID })

	select {
	case to := <-newArrived:
		assert.Equal(t, "p2", to)
	case <-time.After(2 * time.Second):
		t.Fatal("новый полёт не прибыл за 2 секунды")
	}

	select {
	case to := <-oldArrived:
		t.Fatalf("старый полёт вызвал onArrival: %s", to)
	case <-time.After(150 * time.Millisecond):
	}

	require.False(t, m.IsInIntraFlight("user-1"))
}

// Старая горутина редиректа НЕ удаляет строку нового полёта (ревью 99.2.27,
// интеграционный): после отмены старого полёта строка остаётся строкой нового
// сегмента, удаляется только прибытием нового полёта.
func TestIntraOldGoroutineDoesNotDeleteAfterRedirect(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", time.Hour, nil)
	require.True(t, store.has("user-1"))

	// Редирект: новый полёт к p2 (длиннее окна проверки старой горутины).
	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p2", 200*time.Millisecond, nil)

	// Даём старой горутине проснуться (CancelChan закрыт) — БД она не трогает.
	time.Sleep(50 * time.Millisecond)
	row, ok := store.get("user-1")
	require.True(t, ok, "старая горутина не должна удалять строку нового полёта")
	assert.Equal(t, "p2", row.ToID)

	// Новый полёт долетает — строка удаляется.
	require.Eventually(t, func() bool { return !store.has("user-1") }, time.Second, 5*time.Millisecond)
}

// Отмена предотвращает onArrival даже если таймер успел сработать (ревью
// 99.2.27, transient-позиция): CancelIntraFlight удаляет полёт из map СРАЗУ —
// горутина при пробуждении видит, что полёт не «наш», и не пишет orbit
// вместо NULL (после CancelAtomic).
func TestIntraCancelPreventsArrivalAfterTimer(t *testing.T) {
	m := NewIntrasystemManager(nil)

	arrived := make(chan string, 1)
	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", 10*time.Millisecond,
		func(userID string, f *IntraFlightInfo) { arrived <- f.ToID })

	// Отменяем сразу — до истечения таймера.
	require.True(t, m.CancelIntraFlight("user-1"))
	require.False(t, m.IsInIntraFlight("user-1"), "полёт удалён из map сразу")

	// Ждём дольше длительности полёта: onArrival не должен прийти.
	select {
	case to := <-arrived:
		t.Fatalf("onArrival вызван после отмены: %s", to)
	case <-time.After(150 * time.Millisecond):
	}
}

// Гонка CancelIntraFlight + StartIntraFlight не роняет процесс (double-close
// panic, ревью 99.2.27): CancelIntraFlight удаляет полёт из map — StartIntraFlight
// не находит старый и не закрывает канал повторно; sync.Once — страховка.
func TestIntraCancelThenStartNoPanic(t *testing.T) {
	m := NewIntrasystemManager(nil)

	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", time.Hour, nil)
	require.True(t, m.CancelIntraFlight("user-1"))

	// Новый полёт после отмены — без panic (канал старого уже закрыт).
	require.NotPanics(t, func() {
		m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p2", time.Hour, nil)
	})
	require.True(t, m.IsInIntraFlight("user-1"))
	require.Equal(t, "p2", m.GetIntraFlight("user-1").ToID)
}

// ==================== ОТМЕНА ====================

func TestIntraCancelFlight(t *testing.T) {
	m := NewIntrasystemManager(nil)

	assert.False(t, m.CancelIntraFlight("user-1"))

	arrived := make(chan string, 1)
	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", time.Hour,
		func(userID string, f *IntraFlightInfo) { arrived <- f.ToID })

	assert.True(t, m.CancelIntraFlight("user-1"))
	require.Eventually(t, func() bool { return !m.IsInIntraFlight("user-1") }, time.Second, 5*time.Millisecond)

	select {
	case to := <-arrived:
		t.Fatalf("onArrival вызван после отмены: %s", to)
	case <-time.After(150 * time.Millisecond):
	}
}

// ==================== ПЕРСИСТЕНТНОСТЬ ====================

// Старт пишет в БД: строка с корректным сегментом (start_time, arrive_at).
func TestIntraStartPersists(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", time.Hour, nil)

	row, ok := store.get("user-1")
	require.True(t, ok, "StartIntraFlight должен писать в БД")
	assert.Equal(t, "w1", row.WorldID)
	assert.Equal(t, "star", row.FromType)
	assert.Equal(t, "planet", row.ToType)
	assert.Equal(t, "p1", row.ToID)
	flight := m.GetIntraFlight("user-1")
	require.NotNil(t, flight)
	assert.Equal(t, flight.StartTime, row.StartTime)
	assert.Equal(t, flight.ArriveAt, row.ArriveAt)
}

// Прибытие удаляет строку ПОСЛЕ onArrival (паттерн 97a).
func TestIntraArrivalDeletesFromStoreAfterOnArrival(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	rowStillThere := false
	callbackRan := make(chan struct{}, 1)
	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", 30*time.Millisecond,
		func(userID string, f *IntraFlightInfo) {
			_, ok := store.get(userID)
			rowStillThere = ok
			callbackRan <- struct{}{}
		})

	<-callbackRan
	require.True(t, rowStillThere, "строка БД должна удаляться ПОСЛЕ onArrival")
	require.Eventually(t, func() bool { return !store.has("user-1") }, time.Second, 5*time.Millisecond)
}

// Cancel чистит БД.
func TestIntraCancelDeletesFromStore(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", time.Hour, nil)
	require.True(t, store.has("user-1"))

	assert.True(t, m.CancelIntraFlight("user-1"))
	require.False(t, store.has("user-1"), "CancelIntraFlight чистит БД")
	require.Eventually(t, func() bool { return !m.IsInIntraFlight("user-1") }, time.Second, 5*time.Millisecond)
}

// Редирект обновляет строку — новый сегмент (новый to, новый start_time).
func TestIntraRedirectUpdatesStoreRow(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p1", time.Hour, nil)
	first, ok := store.get("user-1")
	require.True(t, ok)

	// Windows: time.Now() гранулярность ~0.5 мс — нужен sleep (PITFALLS).
	time.Sleep(5 * time.Millisecond)

	m.StartIntraFlight("user-1", "w1", "star", "w1", "planet", "p2", 30*time.Millisecond, nil)

	row, ok := store.get("user-1")
	require.True(t, ok)
	assert.Equal(t, "p2", row.ToID, "редирект заменяет сегмент в БД")
	assert.NotEqual(t, first.StartTime, row.StartTime, "новый start_time")
	assert.NotEqual(t, first.ArriveAt, row.ArriveAt, "новый arrive_at")
}

// ==================== RESTORE (паттерн 97a, спека §3.5) ====================

// Будущий arrive_at → полёт в памяти с ОРИГИНАЛЬНЫМИ временами, горутина ждёт
// остаток и засчитывает прибытие; строка удаляется по прибытии.
func TestIntraRestoreFutureArrival(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	start := time.Now().Add(-time.Hour)
	arrive := time.Now().Add(50 * time.Millisecond)
	store.rows["user-1"] = models.PlayerIntrasystemFlight{
		UserID: "user-1", WorldID: "w1",
		FromType: "star", FromID: "w1", ToType: "planet", ToID: "p1",
		StartTime: start, ArriveAt: arrive,
	}

	arrived := make(chan string, 1)
	m.RestoreIntra(time.Now(),
		func(worldID, objType, objID string) bool { return true },
		func(userID string) bool { return false },
		func(userID string) string { return "w1" },
		func(userID string, f *IntraFlightInfo) { arrived <- userID + ":" + f.ToID },
		nil)

	flight := m.GetIntraFlight("user-1")
	require.NotNil(t, flight, "будущий arrive_at → полёт перерегистрируется")
	assert.Equal(t, start, flight.StartTime, "StartTime — оригинальный из БД")
	assert.Equal(t, arrive, flight.ArriveAt, "ArriveAt — оригинальный")

	select {
	case msg := <-arrived:
		assert.Equal(t, "user-1:p1", msg)
	case <-time.After(2 * time.Second):
		t.Fatal("восстановленный полёт не прибыл")
	}
	require.Eventually(t, func() bool { return !store.has("user-1") }, time.Second, 5*time.Millisecond)
}

// Прошлый arrive_at → onArrival вызывается сразу, строка удаляется.
func TestIntraRestorePastArrival(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	store.rows["user-1"] = models.PlayerIntrasystemFlight{
		UserID: "user-1", WorldID: "w1",
		FromType: "star", FromID: "w1", ToType: "planet", ToID: "p1",
		StartTime: time.Now().Add(-2 * time.Hour), ArriveAt: time.Now().Add(-time.Hour),
	}

	arrived := make(chan string, 1)
	m.RestoreIntra(time.Now(),
		func(worldID, objType, objID string) bool { return true },
		func(userID string) bool { return false },
		func(userID string) string { return "w1" },
		func(userID string, f *IntraFlightInfo) { arrived <- userID + ":" + f.ToID },
		nil)

	select {
	case msg := <-arrived:
		assert.Equal(t, "user-1:p1", msg)
	case <-time.After(2 * time.Second):
		t.Fatal("onArrival не вызван для просроченного полёта")
	}
	assert.False(t, m.IsInIntraFlight("user-1"), "просроченный полёт не восстанавливается")
	require.False(t, store.has("user-1"), "строка просроченного полёта удалена")
}

// Битая цель (планета удалена перегенерацией) → полёт НЕ восстанавливается,
// строка удаляется, onArrival не вызывается, игрок возвращается на «орбиту
// звезды» системы (resetPosition, B27).
func TestIntraRestoreBrokenTarget(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	store.rows["user-1"] = models.PlayerIntrasystemFlight{
		UserID: "user-1", WorldID: "w1",
		FromType: "star", FromID: "w1", ToType: "planet", ToID: "GONE",
		StartTime: time.Now().Add(-time.Minute), ArriveAt: time.Now().Add(time.Hour),
	}

	called := false
	resetCalls := 0
	var resetUser, resetWorld string
	m.RestoreIntra(time.Now(),
		func(worldID, objType, objID string) bool { return objID != "GONE" },
		func(userID string) bool { return false },
		func(userID string) string { return "w1" },
		func(userID string, f *IntraFlightInfo) { called = true },
		func(userID, worldID string) { resetCalls++; resetUser, resetWorld = userID, worldID })

	assert.False(t, called, "битый полёт не засчитывает прибытие")
	assert.False(t, m.IsInIntraFlight("user-1"))
	require.False(t, store.has("user-1"), "строка с битой целью удалена")
	assert.Equal(t, 1, resetCalls, "resetPosition вызван ровно один раз")
	assert.Equal(t, "user-1", resetUser)
	assert.Equal(t, "w1", resetWorld)
}

// С-1: у игрока есть межзвёздная строка → внутрисистемная удаляется БЕЗ
// вызова onArrival (иначе позиция {planet, старый мир} запишется при новом
// current_world_id — нарушение ИП-1).
func TestIntraRestoreInterstellarWins(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	store.rows["user-1"] = models.PlayerIntrasystemFlight{
		UserID: "user-1", WorldID: "w1",
		FromType: "star", FromID: "w1", ToType: "planet", ToID: "p1",
		StartTime: time.Now().Add(-time.Minute), ArriveAt: time.Now().Add(time.Hour),
	}

	called := false
	resetCalled := false
	m.RestoreIntra(time.Now(),
		func(worldID, objType, objID string) bool { return true },
		func(userID string) bool { return userID == "user-1" }, // межзвёздная есть
		func(userID string) string { return "w1" },
		func(userID string, f *IntraFlightInfo) { called = true },
		func(userID, worldID string) { resetCalled = true })

	assert.False(t, called, "межзвёздная строка побеждает: onArrival не вызывается")
	assert.False(t, resetCalled, "позицией владеет межзвёздный полёт: resetPosition не вызывается")
	assert.False(t, m.IsInIntraFlight("user-1"))
	require.False(t, store.has("user-1"), "intra удаляется без onArrival")
}

// ИП-1: игрок уже в другой системе (межзвёздная прибыла при Restore) →
// внутрисистемная удаляется без onArrival.
func TestIntraRestoreMovedSystem(t *testing.T) {
	store := newFakeIntraStore()
	m := NewIntrasystemManager(store)

	store.rows["user-1"] = models.PlayerIntrasystemFlight{
		UserID: "user-1", WorldID: "w1",
		FromType: "star", FromID: "w1", ToType: "planet", ToID: "p1",
		StartTime: time.Now().Add(-time.Minute), ArriveAt: time.Now().Add(time.Hour),
	}

	called := false
	resetCalled := false
	m.RestoreIntra(time.Now(),
		func(worldID, objType, objID string) bool { return true },
		func(userID string) bool { return false },
		func(userID string) string { return "w2" }, // игрок уже в другой системе
		func(userID string, f *IntraFlightInfo) { called = true },
		func(userID, worldID string) { resetCalled = true })

	assert.False(t, called, "игрок в другой системе: onArrival не вызывается")
	assert.False(t, resetCalled, "позицию записал прибывший межзвёздный полёт: resetPosition не вызывается")
	assert.False(t, m.IsInIntraFlight("user-1"))
	require.False(t, store.has("user-1"), "intra удаляется без onArrival")
}