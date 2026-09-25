// internal/travel/manager_boost_test.go
// Тесты BoostFlight (спека ускорителя §4.1, ЧК1): замена сегмента, гейты
// already_active/cooldown (через Booster), устаревший expectStartTime, ошибка БД
// без применения, одно ускорение на сегмент при параллельных submit, Restore
// сохраняет признак active (сравнение UnixMilli).
package travel

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// fakeBooster — настраиваемый Booster для тестов.
type fakeBooster struct {
	mu      sync.Mutex
	calls   int
	applied bool
	reason  string
	err     error
	lastSeg models.PlayerFlight
}

func (b *fakeBooster) ApplyBoost(userID string, expectStartTime time.Time, seg models.PlayerFlight, cooldownMin int, now time.Time) (bool, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	b.lastSeg = seg
	if b.err != nil {
		return false, "", b.err
	}
	return b.applied, b.reason, nil
}

func (b *fakeBooster) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func TestBoostFlightSuccess(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)
	b := &fakeBooster{applied: true}
	m.SetBooster(b)

	oldArrived := make(chan string, 1)
	m.StartFlight("u1", "A", "B", 1, 2, time.Hour, func(uid, w string) { oldArrived <- w })
	old := m.GetFlight("u1")
	require.NotNil(t, old)

	// Windows: time.Now() гранулярность ~0.5 мс — sleep, чтобы truncated start_time
	// нового сегмента гарантированно отличался от старого по UnixMilli (PITFALLS).
	time.Sleep(5 * time.Millisecond)

	applied, reason, err := m.BoostFlight("u1", old.StartTime, 3, 4, 20*time.Minute, 25, func(uid, w string) {})
	require.NoError(t, err)
	require.True(t, applied)
	require.Empty(t, reason)

	f := m.GetFlight("u1")
	require.NotNil(t, f)
	assert.NotEqual(t, old.StartTime.UnixMilli(), f.StartTime.UnixMilli(), "новый start_time сегмента")
	assert.Equal(t, 3.0, f.StartX)
	assert.Equal(t, 4.0, f.StartY)
	assert.Equal(t, 20*time.Minute, f.Duration)
	assert.Equal(t, "A", f.FromWorld)
	assert.Equal(t, "B", f.ToWorld)

	// Сегмент записан с теми же start_time/arrive_at, что в памяти (один момент).
	assert.Equal(t, f.StartTime, b.lastSeg.StartTime)
	assert.Equal(t, f.StartTime.Add(20*time.Minute), b.lastSeg.ArriveAt)

	// Старый полёт отменён — его onArrival не вызывается.
	select {
	case w := <-oldArrived:
		t.Fatalf("старый полёт вызвал onArrival: %s", w)
	case <-time.After(120 * time.Millisecond):
	}
}

// Повтор на том же сегменте → already_active, сегмент не изменён (И-9).
func TestBoostFlightAlreadyActive(t *testing.T) {
	m := NewManager(nil)
	m.SetBooster(&fakeBooster{applied: false, reason: "already_active"})
	m.StartFlight("u1", "A", "B", 0, 0, time.Hour, nil)
	old := m.GetFlight("u1")

	applied, reason, err := m.BoostFlight("u1", old.StartTime, 5, 5, time.Minute, 25, nil)
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, "already_active", reason)
	require.Same(t, old, m.GetFlight("u1"), "сегмент не заменён")
}

// Cooldown от Booster → отказ без замены сегмента.
func TestBoostFlightCooldown(t *testing.T) {
	m := NewManager(nil)
	m.SetBooster(&fakeBooster{applied: false, reason: "cooldown"})
	m.StartFlight("u1", "A", "B", 0, 0, time.Hour, nil)
	old := m.GetFlight("u1")

	applied, reason, err := m.BoostFlight("u1", old.StartTime, 5, 5, time.Minute, 25, nil)
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, "cooldown", reason)
	require.Same(t, old, m.GetFlight("u1"))
}

// Устаревший expectStartTime (смена цели/сегмента) → no-op, Booster не вызван.
func TestBoostFlightStaleSegment(t *testing.T) {
	m := NewManager(nil)
	b := &fakeBooster{applied: true}
	m.SetBooster(b)
	m.StartFlight("u1", "A", "B", 0, 0, time.Hour, nil)

	applied, reason, err := m.BoostFlight("u1", time.Now().Add(-time.Hour), 5, 5, time.Minute, 25, nil)
	require.NoError(t, err)
	require.False(t, applied)
	require.Empty(t, reason)
	assert.Equal(t, 0, b.callCount(), "устаревший сегмент — Booster не вызывается")
}

// Нет Booster (юнит-тесты без БД) → no-op.
func TestBoostFlightNoBooster(t *testing.T) {
	m := NewManager(nil)
	m.StartFlight("u1", "A", "B", 0, 0, time.Hour, nil)
	old := m.GetFlight("u1")

	applied, _, err := m.BoostFlight("u1", old.StartTime, 5, 5, time.Minute, 25, nil)
	require.NoError(t, err)
	require.False(t, applied)
}

// Ошибка Booster (БД) → ускорение не применено, сегмент не заменён.
func TestBoostFlightErrorNotApplied(t *testing.T) {
	m := NewManager(nil)
	m.SetBooster(&fakeBooster{err: errors.New("db down")})
	m.StartFlight("u1", "A", "B", 0, 0, time.Hour, nil)
	old := m.GetFlight("u1")

	applied, _, err := m.BoostFlight("u1", old.StartTime, 5, 5, time.Minute, 25, nil)
	require.Error(t, err)
	require.False(t, applied)
	require.Same(t, old, m.GetFlight("u1"), "при ошибке БД сегмент не заменён")
}

// onceBooster — применяет один раз на expectStartTime, дальше already_active.
type onceBooster struct {
	mu      sync.Mutex
	seen    map[string]bool
	applied int
}

func (b *onceBooster) ApplyBoost(userID string, expectStartTime time.Time, seg models.PlayerFlight, cooldownMin int, now time.Time) (bool, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	k := userID + "|" + expectStartTime.UTC().Format(time.RFC3339Nano)
	if b.seen[k] {
		return false, "already_active", nil
	}
	b.seen[k] = true
	b.applied++
	return true, "", nil
}

// Параллельные submit — ровно одно ускорение на сегмент (мьютекс BoostFlight +
// гейт already_active).
func TestBoostFlightConcurrentOneWins(t *testing.T) {
	m := NewManager(nil)
	m.SetBooster(&onceBooster{seen: map[string]bool{}})
	m.StartFlight("u1", "A", "B", 0, 0, time.Hour, nil)
	start := m.GetFlight("u1").StartTime

	const n = 8
	results := make([]bool, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			applied, _, err := m.BoostFlight("u1", start, 1, 1, time.Minute, 25, nil)
			if err != nil {
				t.Errorf("BoostFlight: %v", err)
			}
			results[i] = applied
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, r := range results {
		if r {
			wins++
		}
	}
	assert.Equal(t, 1, wins, "ровно одно ускорение на сегмент (И-9)")
}

// Restore сохраняет сегмент, а признак active восстанавливается сравнением
// UnixMilli (last_boost_at из БД == start_time сегмента).
func TestRestorePreservesBoostActive(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)

	start := time.Now().Truncate(time.Millisecond)
	store.rows["u1"] = models.PlayerFlight{
		UserID: "u1", FromWorld: "A", ToWorld: "B",
		StartX: 1, StartY: 2, StartTime: start, ArriveAt: start.Add(time.Hour),
	}

	m.Restore(time.Now(), func(id string) bool { return true }, nil)

	f := m.GetFlight("u1")
	require.NotNil(t, f)
	require.Equal(t, start.UnixMilli(), f.StartTime.UnixMilli(), "Restore — оригинальный start_time")
	lastBoost := start
	require.True(t, models.BoostActive(&lastBoost, f.StartTime), "active восстанавливается после рестарта")
}

// Разворот 61a сбрасывает ускорение: новый сегмент — другой start_time →
// active=false (решение 18, §6.8).
func TestBoostResetOnRedirect(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)
	b := &fakeBooster{applied: true}
	m.SetBooster(b)

	m.StartFlight("u1", "A", "B", 0, 0, time.Hour, nil)
	old := m.GetFlight("u1")
	time.Sleep(5 * time.Millisecond)
	applied, _, err := m.BoostFlight("u1", old.StartTime, 1, 1, 30*time.Minute, 25, nil)
	require.NoError(t, err)
	require.True(t, applied)
	boosted := m.GetFlight("u1")
	require.True(t, models.BoostActive(&boosted.StartTime, boosted.StartTime), "ускорение действует")

	// Разворот: новая цель из точки P — новый сегмент.
	time.Sleep(5 * time.Millisecond)
	m.StartFlight("u1", "A", "C", 5, 5, time.Hour, nil)
	f := m.GetFlight("u1")
	require.NotNil(t, f)
	require.False(t, models.BoostActive(&boosted.StartTime, f.StartTime),
		"разворот сбрасывает ускорение (active=false)")
}

// Прибытие завершает сегмент: следующий перелёт — новый start_time → active=false.
func TestBoostEndsOnArrival(t *testing.T) {
	store := newFakeStore()
	m := NewManager(store)
	b := &fakeBooster{applied: true}
	m.SetBooster(b)

	m.StartFlight("u1", "A", "B", 0, 0, time.Hour, nil)
	old := m.GetFlight("u1")
	time.Sleep(5 * time.Millisecond)
	applied, _, err := m.BoostFlight("u1", old.StartTime, 1, 1, 30*time.Millisecond, 25, nil)
	require.NoError(t, err)
	require.True(t, applied)
	boostedStart := b.lastSeg.StartTime

	require.Eventually(t, func() bool { return !m.IsInFlight("u1") }, 2*time.Second, 5*time.Millisecond)

	time.Sleep(5 * time.Millisecond)
	m.StartFlight("u1", "A", "D", 0, 0, time.Hour, nil)
	f := m.GetFlight("u1")
	require.NotNil(t, f)
	require.False(t, models.BoostActive(&boostedStart, f.StartTime), "прибытие завершает ускорение")
}
