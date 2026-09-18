// internal/travel/manager_test.go
package travel

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// fakeStore — in-memory реализация FlightStore для тестов (97a).
type fakeStore struct {
	mu   sync.Mutex
	rows map[string]models.PlayerFlight
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[string]models.PlayerFlight{}}
}

func (s *fakeStore) Upsert(f models.PlayerFlight) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[f.UserID] = f
	return nil
}

func (s *fakeStore) Delete(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, userID)
	return nil
}

func (s *fakeStore) ListAll() ([]models.PlayerFlight, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.PlayerFlight, 0, len(s.rows))
	for _, f := range s.rows {
		out = append(out, f)
	}
	return out, nil
}

func (s *fakeStore) get(userID string) (models.PlayerFlight, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.rows[userID]
	return f, ok
}

func (s *fakeStore) has(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.rows[userID]
	return ok
}

// ==================== БАЗОВЫЕ ПРОВЕРКИ ====================

func TestNewManagerEmpty(t *testing.T) {
	m := NewManager(nil)
	assert.False(t, m.IsInFlight("user-1"))
	assert.Nil(t, m.GetFlight("user-1"))
}

// ==================== ПОЛЁТ ДО ЗАВЕРШЕНИЯ ====================

func TestStartFlightRegistersFlight(t *testing.T) {
	m := NewManager(nil)

	const userID, from, to = "user-1", "world-A", "world-B"
	m.StartFlight(userID, from, to, 1.5, 2.5, time.Hour, nil)

	flight := m.GetFlight(userID)
	require.NotNil(t, flight, "полёт должен зарегистрироваться сразу")
	assert.Equal(t, userID, flight.UserID)
	assert.Equal(t, from, flight.FromWorld)
	assert.Equal(t, to, flight.ToWorld)
	assert.Equal(t, 1.5, flight.StartX)
	assert.Equal(t, 2.5, flight.StartY)
	assert.Equal(t, time.Hour, flight.Duration)
	assert.False(t, flight.StartTime.IsZero())
	assert.True(t, m.IsInFlight(userID))
}

func TestArrivalCallsCallback(t *testing.T) {
	m := NewManager(nil)

	arrived := make(chan string, 1)
	m.StartFlight("user-1", "world-A", "world-B", 0, 0, 30*time.Millisecond,
		func(userID, worldID string) {
			arrived <- userID + ":" + worldID
		})

	select {
	case msg := <-arrived:
		assert.Equal(t, "user-1:world-B", msg)
	case <-time.After(2 * time.Second):
		t.Fatalf("onArrival не вызван в течение 2 секунд")
	}

	// Полёт должен удалиться из map после прибытия.
	require.Eventually(t, func() bool { return !m.IsInFlight("user-1") }, time.Second, 5*time.Millisecond)
	assert.Nil(t, m.GetFlight("user-1"))
}

func TestArrivalCallsCallbackAfterRemoval(t *testing.T) {
	m := NewManager(nil)

	flightRemoved := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	m.StartFlight("user-2", "A", "B", 0, 0, 20*time.Millisecond,
		func(userID, worldID string) {
			close(flightRemoved)
			done <- struct{}{}
		})

	<-flightRemoved
	assert.False(t, m.IsInFlight("user-2"), "полёт должен удаляться до вызова колбэка")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("колбэк не сработал")
	}
}

// ==================== ЗАМЕНА ПОЛЁТА ====================

// TestReplaceCancelsOldFlight — новый полёт отменяет старый:
// колбэк старого не вызывается, в map остаётся только новый.
func TestReplaceCancelsOldFlight(t *testing.T) {
	m := NewManager(nil)

	oldArrived := make(chan string, 1)
	m.StartFlight("user-1", "A", "old", 0, 0, time.Hour,
		func(userID, worldID string) { oldArrived <- worldID })

	// Сразу заменяем коротким полётом.
	newArrived := make(chan string, 1)
	m.StartFlight("user-1", "A", "new", 0, 0, 30*time.Millisecond,
		func(userID, worldID string) { newArrived <- worldID })

	// Новый полёт должен долететь.
	select {
	case world := <-newArrived:
		assert.Equal(t, "new", world)
	case <-time.After(2 * time.Second):
		t.Fatal("новый полёт не прибыл за 2 секунды")
	}

	// Старый — не должен.
	select {
	case world := <-oldArrived:
		t.Fatalf("старый полёт вызвал колбэк: %s", world)
	case <-time.After(150 * time.Millisecond):
	}

	// Финальное состояние — без полёта.
	require.False(t, m.IsInFlight("user-1"))
}

// TestGetFlightShowsNewestFlight — после замены GetFlight возвращает новый.
func TestGetFlightShowsNewestFlight(t *testing.T) {
	m := NewManager(nil)

	m.StartFlight("user-1", "A", "old", 0, 0, time.Hour, nil)
	m.StartFlight("user-1", "A", "new", 0, 0, time.Hour, nil)

	flight := m.GetFlight("user-1")
	require.NotNil(t, flight)
	assert.Equal(t, "new", flight.ToWorld)
}

// ==================== ОТМЕНА ПОЛЁТА ====================

func TestCancelFlight(t *testing.T) {
	m := NewManager(nil)

	// Нет полёта — false.
	assert.False(t, m.CancelFlight("user-1"))

	// Активный полёт — true, полёт удаляется, onArrival не вызывается.
	arrived := make(chan string, 1)
	m.StartFlight("user-1", "A", "B", 0, 0, time.Hour,
		func(userID, worldID string) { arrived <- worldID })

	assert.True(t, m.CancelFlight("user-1"))
	require.Eventually(t, func() bool { return !m.IsInFlight("user-1") }, time.Second, 5*time.Millisecond)
	assert.Nil(t, m.GetFlight("user-1"))

	select {
	case w := <-arrived:
		t.Fatalf("onArrival вызван после отмены: %s", w)
	case <-time.After(150 * time.Millisecond):
	}
}

// ==================== НЕСКОЛЬКО ПОЛЬЗОВАТЕЛЕЙ ====================

func TestConcurrentUsersIndependent(t *testing.T) {
	m := NewManager(nil)

	arrivedA := make(chan string, 1)
	arrivedB := make(chan string, 1)
	m.StartFlight("user-A", "W1", "W2", 0, 0, 20*time.Millisecond,
		func(userID, worldID string) { arrivedA <- userID })
	m.StartFlight("user-B", "W9", "W8", 0, 0, 40*time.Millisecond,
		func(userID, worldID string) { arrivedB <- userID })

	select {
	case uid := <-arrivedA:
		assert.Equal(t, "user-A", uid)
	case <-time.After(2 * time.Second):
		t.Fatal("полёт A не прибыл")
	}

	assert.True(t, m.IsInFlight("user-B"), "полёт B не должен трогаться полётом A")

	select {
	case uid := <-arrivedB:
		assert.Equal(t, "user-B", uid)
	case <-time.After(2 * time.Second):
		t.Fatal("полёт B не прибыл")
	}

	assert.False(t, m.IsInFlight("user-A"))
	assert.False(t, m.IsInFlight("user-B"))
}

// ==================== ПЕРСИСТЕНТНОСТЬ (97a) ====================

// StartFlight пишет в БД: строка с корректным сегментом (start_time,
// arrive_at = start_time + duration).
func TestStartFlightPersists(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)

	m.StartFlight("user-1", "A", "B", 1.5, 2.5, time.Hour, nil)

	row, ok := store.get("user-1")
	require.True(t, ok, "StartFlight должен писать в БД")
	assert.Equal(t, "A", row.FromWorld)
	assert.Equal(t, "B", row.ToWorld)
	assert.Equal(t, 1.5, row.StartX)
	assert.Equal(t, 2.5, row.StartY)
	flight := m.GetFlight("user-1")
	require.NotNil(t, flight)
	assert.Equal(t, flight.StartTime, row.StartTime)
	assert.Equal(t, flight.StartTime.Add(time.Hour), row.ArriveAt)
}

// Прибытие удаляет строку ПОСЛЕ onArrival (критик, мелкое 3): в момент
// колбэка строка ещё на месте, после — удалена.
func TestArrivalDeletesFromStoreAfterOnArrival(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)

	rowStillThere := false
	callbackRan := make(chan struct{}, 1)
	m.StartFlight("user-1", "A", "B", 0, 0, 30*time.Millisecond,
		func(userID, worldID string) {
			_, ok := store.get(userID)
			rowStillThere = ok
			callbackRan <- struct{}{}
		})

	<-callbackRan
	require.True(t, rowStillThere, "строка БД должна удаляться ПОСЛЕ onArrival")
	require.Eventually(t, func() bool { return !store.has("user-1") }, time.Second, 5*time.Millisecond)
}

// CancelFlight чистит БД (критик, мелкое 5).
func TestCancelDeletesFromStore(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)

	m.StartFlight("user-1", "A", "B", 0, 0, time.Hour, nil)
	require.True(t, store.has("user-1"))

	assert.True(t, m.CancelFlight("user-1"))
	require.False(t, store.has("user-1"), "CancelFlight чистит БД")
	require.Eventually(t, func() bool { return !m.IsInFlight("user-1") }, time.Second, 5*time.Millisecond)
}

// Редирект (61a): повторный StartFlight обновляет строку — новый сегмент
// (точка P, новый start_time, новый arrive_at).
func TestRedirectUpdatesStoreRow(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)

	m.StartFlight("user-1", "A", "B", 0, 0, time.Hour, nil)
	first, ok := store.get("user-1")
	require.True(t, ok)

	// Windows: time.Now() гранулярность ~0.5 мс — нужен sleep, чтобы
	// start_time нового сегмента гарантированно отличался (PITFALLS).
	time.Sleep(5 * time.Millisecond)

	m.StartFlight("user-1", "A", "C", 5, 5, 30*time.Millisecond, nil)

	row, ok := store.get("user-1")
	require.True(t, ok)
	assert.Equal(t, "C", row.ToWorld, "редирект заменяет сегмент в БД")
	assert.Equal(t, 5.0, row.StartX, "новый start_x = точка P")
	assert.Equal(t, 5.0, row.StartY)
	assert.NotEqual(t, first.StartTime, row.StartTime, "новый start_time")
	assert.NotEqual(t, first.ArriveAt, row.ArriveAt, "новый arrive_at")
}

// Старая горутина редиректа НЕ удаляет строку нового полёта (критик,
// среднее 1): после отмены старого полёта строка остаётся строкой нового
// сегмента, удаляется только прибытием нового полёта.
func TestOldGoroutineDoesNotDeleteAfterRedirect(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)

	m.StartFlight("user-1", "A", "B", 0, 0, time.Hour, nil)
	require.True(t, store.has("user-1"))

	// Редирект: новый полёт A → C (длиннее окна проверки старой горутины).
	m.StartFlight("user-1", "A", "C", 5, 5, 200*time.Millisecond, nil)

	// Даём старой горутине проснуться (CancelChan закрыт) — БД она не трогает.
	time.Sleep(50 * time.Millisecond)
	row, ok := store.get("user-1")
	require.True(t, ok, "старая горутина не должна удалять строку нового полёта")
	assert.Equal(t, "C", row.ToWorld)

	// Новый полёт долетает — строка удаляется.
	require.Eventually(t, func() bool { return !store.has("user-1") }, time.Second, 5*time.Millisecond)
}

// blockingDeleteStore — fakeStore с барьером на Delete: сигналит о вызове
// и ждёт разрешения. Детерминированная симуляция окна TOCTOU (ревью 97a):
// старая горутина дошла до Delete, но ещё не удалила строку. UpsertDone
// сигналит о завершении записи нового сегмента.
type blockingDeleteStore struct {
	*fakeStore
	deleteCalled  chan struct{}
	deleteRelease chan struct{}
	upsertDone    chan struct{}
}

func (s *blockingDeleteStore) Upsert(f models.PlayerFlight) error {
	err := s.fakeStore.Upsert(f)
	s.upsertDone <- struct{}{}
	return err
}

func (s *blockingDeleteStore) Delete(userID string) error {
	s.deleteCalled <- struct{}{}
	<-s.deleteRelease
	return s.fakeStore.Delete(userID)
}

// TestArrivalDeleteDoesNotRemoveNewFlightRow — TOCTOU-окно (ревью 97a):
// между проверкой актуальности и Delete старой горутины стартует новый
// полёт (вставка в map + Upsert строки). Старая горутина не должна удалить
// строку НОВОГО сегмента. Барьер на Delete фиксирует межблокировку:
//   - баг-путь (проверка и Delete в разных локах): B успевает сделать
//     Upsert, пока Delete A заблокирован барьером → Delete A удаляет строку B;
//   - фикс-путь (проверка и Delete под одним локом): B блокируется на m.mu,
//     который A держит в Delete → Upsert B происходит ПОСЛЕ Delete A.
//
// Точное окно «между проверкой и Delete» без хука в runFlight не
// воспроизвести, но барьер воспроизводит его эквивалент: Delete A
// «завис» после проверки, B успел записать строку. Таймаут ожидания
// Upsert B — только для фикс-пути (B гарантированно заблокирован локом,
// Upsert не придёт); в баг-пути Upsert приходит за микросекунды.
func TestArrivalDeleteDoesNotRemoveNewFlightRow(t *testing.T) {
	store := &blockingDeleteStore{
		fakeStore:     newFakeStore(),
		deleteCalled:  make(chan struct{}, 1),
		deleteRelease: make(chan struct{}),
		upsertDone:    make(chan struct{}, 1),
	}
	m := NewManager(store)

	arrived := make(chan struct{}, 1)
	m.StartFlight("user-1", "A", "B", 0, 0, 30*time.Millisecond,
		func(userID, worldID string) { arrived <- struct{}{} })

	<-arrived
	// Старая горутина дошла до Delete и заблокирована барьером.
	<-store.deleteCalled
	// Дренируем сигнал Upsert от StartFlight A (буфер мог захватить его) —
	// иначе тест принял бы его за Upsert B.
	select {
	case <-store.upsertDone:
	default:
	}

	// Новый полёт B стартует в отдельной горутине (в тестовой горутине
	// он бы заблокировался на m.mu в фикс-пути — deadlock с барьером).
	go m.StartFlight("user-1", "B", "C", 5, 5, time.Hour, nil)

	// Ждём Upsert B: баг-путь — лок свободен, Upsert проходит сразу;
	// фикс-путь — B заблокирован на m.mu, Upsert не придёт (таймаут).
	select {
	case <-store.upsertDone:
		// B успел записать строку до завершения Delete A — окно TOCTOU.
	case <-time.After(200 * time.Millisecond):
		// B заблокирован локом — проверка и Delete атомарны (фикс).
	}
	close(store.deleteRelease)

	// Строка в БД — сегмент B (не удалена старой горутиной).
	require.Eventually(t, func() bool {
		row, ok := store.get("user-1")
		return ok && row.ToWorld == "C"
	}, time.Second, 5*time.Millisecond)
}

// ==================== RESTORE (97a) ====================

// Будущий arrive_at → полёт в памяти с ОРИГИНАЛЬНЫМИ start_time/duration
// (мелкое 4: Duration не уменьшается), горутина ждёт остаток и засчитывает
// прибытие; строка удаляется по прибытии.
func TestRestoreFutureArrival(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)

	start := time.Now().Add(-time.Hour)
	arrive := time.Now().Add(50 * time.Millisecond)
	store.rows["user-1"] = models.PlayerFlight{
		UserID: "user-1", FromWorld: "A", ToWorld: "B",
		StartX: 1.5, StartY: 2.5, StartTime: start, ArriveAt: arrive,
	}

	arrived := make(chan string, 1)
	m.Restore(time.Now(),
		func(id string) bool { return id == "A" || id == "B" },
		func(userID, worldID string) { arrived <- userID + ":" + worldID })

	// Полёт в памяти с оригинальными полями.
	flight := m.GetFlight("user-1")
	require.NotNil(t, flight, "будущий arrive_at → полёт перерегистрируется")
	assert.Equal(t, "A", flight.FromWorld)
	assert.Equal(t, "B", flight.ToWorld)
	assert.Equal(t, 1.5, flight.StartX)
	assert.Equal(t, 2.5, flight.StartY)
	assert.Equal(t, start, flight.StartTime, "StartTime — оригинальный из БД")
	assert.Equal(t, arrive.Sub(start), flight.Duration, "Duration — оригинальный (не уменьшен)")

	// Прибытие по остатку времени.
	select {
	case msg := <-arrived:
		assert.Equal(t, "user-1:B", msg)
	case <-time.After(2 * time.Second):
		t.Fatal("восстановленный полёт не прибыл")
	}
	require.Eventually(t, func() bool { return !store.has("user-1") }, time.Second, 5*time.Millisecond)
}

// Прошлый arrive_at → onArrival вызывается сразу, строка удаляется,
// полёт в памяти не регистрируется (как у NPC).
func TestRestorePastArrival(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)

	store.rows["user-1"] = models.PlayerFlight{
		UserID: "user-1", FromWorld: "A", ToWorld: "B",
		StartX: 1.5, StartY: 2.5,
		StartTime: time.Now().Add(-2 * time.Hour), ArriveAt: time.Now().Add(-time.Hour),
	}

	arrived := make(chan string, 1)
	m.Restore(time.Now(),
		func(id string) bool { return true },
		func(userID, worldID string) { arrived <- userID + ":" + worldID })

	select {
	case msg := <-arrived:
		assert.Equal(t, "user-1:B", msg)
	case <-time.After(2 * time.Second):
		t.Fatal("onArrival не вызван для просроченного полёта")
	}
	assert.False(t, m.IsInFlight("user-1"), "просроченный полёт не восстанавливается")
	require.False(t, store.has("user-1"), "строка просроченного полёта удалена")
}

// Битый from/to (мир удалён перегенерацией) → полёт НЕ восстанавливается,
// строка удаляется, onArrival не вызывается (фолбэк 42a/61a, критик,
// среднее 2).
func TestRestoreBrokenWorld(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)

	store.rows["user-1"] = models.PlayerFlight{
		UserID: "user-1", FromWorld: "A", ToWorld: "GONE", // цель удалена
		StartX: 1.5, StartY: 2.5,
		StartTime: time.Now().Add(-time.Minute), ArriveAt: time.Now().Add(time.Hour),
	}
	store.rows["user-2"] = models.PlayerFlight{
		UserID: "user-2", FromWorld: "GONE", ToWorld: "B", // источник удалён
		StartX: 1.5, StartY: 2.5,
		StartTime: time.Now().Add(-time.Minute), ArriveAt: time.Now().Add(time.Hour),
	}

	called := false
	m.Restore(time.Now(),
		func(id string) bool { return id == "A" || id == "B" },
		func(userID, worldID string) { called = true })

	assert.False(t, called, "битый полёт не засчитывает прибытие")
	assert.False(t, m.IsInFlight("user-1"))
	assert.False(t, m.IsInFlight("user-2"))
	require.False(t, store.has("user-1"), "строка с битой целью удалена")
	require.False(t, store.has("user-2"), "строка с битым источником удалена")
}