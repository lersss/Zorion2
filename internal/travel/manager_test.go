// internal/travel/manager_test.go
package travel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== БАЗОВЫЕ ПРОВЕРКИ ====================

func TestNewManagerEmpty(t *testing.T) {
	m := NewManager()
	assert.False(t, m.IsInFlight("user-1"))
	assert.Nil(t, m.GetFlight("user-1"))
}

// ==================== ПОЛЁТ ДО ЗАВЕРШЕНИЯ ====================

func TestStartFlightRegistersFlight(t *testing.T) {
	m := NewManager()

	const userID, from, to = "user-1", "world-A", "world-B"
	m.StartFlight(userID, from, to, time.Hour, nil)

	flight := m.GetFlight(userID)
	require.NotNil(t, flight, "полёт должен зарегистрироваться сразу")
	assert.Equal(t, userID, flight.UserID)
	assert.Equal(t, from, flight.FromWorld)
	assert.Equal(t, to, flight.ToWorld)
	assert.Equal(t, time.Hour, flight.Duration)
	assert.False(t, flight.StartTime.IsZero())
	assert.True(t, m.IsInFlight(userID))
}

func TestArrivalCallsCallback(t *testing.T) {
	m := NewManager()

	arrived := make(chan string, 1)
	m.StartFlight("user-1", "world-A", "world-B", 30*time.Millisecond,
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
	m := NewManager()

	flightRemoved := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	m.StartFlight("user-2", "A", "B", 20*time.Millisecond,
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
	m := NewManager()

	oldArrived := make(chan string, 1)
	m.StartFlight("user-1", "A", "old", time.Hour,
		func(userID, worldID string) { oldArrived <- worldID })

	// Сразу заменяем коротким полётом.
	newArrived := make(chan string, 1)
	m.StartFlight("user-1", "A", "new", 30*time.Millisecond,
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
	m := NewManager()

	m.StartFlight("user-1", "A", "old", time.Hour, nil)
	m.StartFlight("user-1", "A", "new", time.Hour, nil)

	flight := m.GetFlight("user-1")
	require.NotNil(t, flight)
	assert.Equal(t, "new", flight.ToWorld)
}

// ==================== НЕСКОЛЬКО ПОЛЬЗОВАТЕЛЕЙ ====================

func TestConcurrentUsersIndependent(t *testing.T) {
	m := NewManager()

	arrivedA := make(chan string, 1)
	arrivedB := make(chan string, 1)
	m.StartFlight("user-A", "W1", "W2", 20*time.Millisecond,
		func(userID, worldID string) { arrivedA <- userID })
	m.StartFlight("user-B", "W9", "W8", 40*time.Millisecond,
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