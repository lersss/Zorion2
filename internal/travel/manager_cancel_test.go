// internal/travel/manager_cancel_test.go
// Регресс двойного закрытия CancelChan (ЧК3, хвост): CancelFlight убирает запись
// из flights ВМЕСТЕ с close под одним локом — повторный CancelFlight/StartFlight/
// BoostFlight не закрывает канал второй раз (иначе «close of closed channel» —
// паника, Go убивает процесс, §0). Полёт вставляется в map напрямую (без
// горутины runFlight) — закрытие детерминированно, без гонки планировщика.
package travel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// insertFlightForTest — полёт прямо в map, без запуска runFlight.
func (m *Manager) insertFlightForTest(f *TravelInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flights[f.UserID] = f
}

func testCancelFlight(userID string) *TravelInfo {
	return &TravelInfo{
		UserID:     userID,
		FromWorld:  "A",
		ToWorld:    "B",
		StartTime:  time.Now(),
		Duration:   time.Hour,
		CancelChan: make(chan struct{}),
	}
}

// Повторный CancelFlight → false, без паники; запись убрана вместе с close.
func TestCancelFlightTwiceNoPanic(t *testing.T) {
	m := NewManager(nil)
	m.insertFlightForTest(testCancelFlight("u1"))

	require.True(t, m.CancelFlight("u1"))
	require.False(t, m.IsInFlight("u1"), "запись убрана вместе с close")
	require.NotPanics(t, func() {
		require.False(t, m.CancelFlight("u1"))
	})
}

// StartFlight после CancelFlight не закрывает уже закрытый канал.
func TestStartFlightAfterCancelNoPanic(t *testing.T) {
	m := NewManager(nil)
	m.insertFlightForTest(testCancelFlight("u1"))
	require.True(t, m.CancelFlight("u1"))

	require.NotPanics(t, func() {
		m.StartFlight("u1", "A", "C", 0, 0, time.Hour, nil)
	})
	f := m.GetFlight("u1")
	require.NotNil(t, f)
	require.Equal(t, "C", f.ToWorld)
	m.CancelFlight("u1") // уборка горутины
}

// BoostFlight после CancelFlight — нет сегмента, буст не применён, без паники.
func TestBoostFlightAfterCancelNoPanic(t *testing.T) {
	m := NewManager(nil)
	m.SetBooster(&fakeBooster{applied: true})
	f := testCancelFlight("u1")
	m.insertFlightForTest(f)
	require.True(t, m.CancelFlight("u1"))

	var applied bool
	var err error
	require.NotPanics(t, func() {
		applied, _, err = m.BoostFlight("u1", f.StartTime, 1, 1, time.Minute, 25, nil)
	})
	require.NoError(t, err)
	require.False(t, applied, "сегмента нет — буст не применён")
	require.False(t, m.IsInFlight("u1"))
}
