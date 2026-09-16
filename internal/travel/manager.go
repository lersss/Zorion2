// internal/travel/manager.go
package travel

import (
	"log"
	"sync"
	"time"
)

// TravelInfo — информация о текущем полёте.
type TravelInfo struct {
	UserID     string
	FromWorld  string
	ToWorld    string
	StartX     float64 // координаты стартовой точки сегмента (61a)
	StartY     float64
	StartTime  time.Time
	Duration   time.Duration
	CancelChan chan struct{}
}

// Manager управляет активными полётами.
type Manager struct {
	mu      sync.RWMutex
	flights map[string]*TravelInfo
}

func NewManager() *Manager {
	return &Manager{
		flights: make(map[string]*TravelInfo),
	}
}

// StartFlight — запускает полёт для пользователя.
//
// startX/startY — координаты стартовой точки сегмента: при обычном старте
// это координаты FromWorld, при редиректе (61a) — текущая точка P маршрута.
//
// Если у пользователя уже был активный полёт:
//   - старый полёт сигнализируется об отмене (close CancelChan);
//   - заменяется новым.
//
// Горутина старого полёта при пробуждении проверяет, что она удаляет
// ИМЕННО СВОЙ полёт (сравнение указателей) — иначе не трогает map.
func (m *Manager) StartFlight(
	userID, fromWorldID, toWorldID string,
	startX, startY float64,
	duration time.Duration,
	onArrival func(userID, worldID string),
) {
	m.mu.Lock()
	if existing, ok := m.flights[userID]; ok {
		close(existing.CancelChan)
	}
	flight := &TravelInfo{
		UserID:     userID,
		FromWorld:  fromWorldID,
		ToWorld:    toWorldID,
		StartX:     startX,
		StartY:     startY,
		StartTime:  time.Now(),
		Duration:   duration,
		CancelChan: make(chan struct{}),
	}
	m.flights[userID] = flight
	m.mu.Unlock()

	go runFlight(m, flight, onArrival)
}

// runFlight — фоновая горутина одного полёта.
//
// Ждёт либо истечения таймера, либо сигнала отмены.
// При завершении:
//  1. Под мьютексом удаляет полёт, ТОЛЬКО если он всё ещё актуален.
//  2. Если долетел (не отменён) — вызывает onArrival.
func runFlight(m *Manager, flight *TravelInfo, onArrival func(userID, worldID string)) {
	// Защита от паники в колбэке — не валим процесс.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔥 panic in flight goroutine (user %s): %v", flight.UserID, r)
		}
	}()

	arrived := false
	select {
	case <-time.After(flight.Duration):
		arrived = true
	case <-flight.CancelChan:
		arrived = false
	}

	// Удаляем полёт, только если это всё ещё НАШ полёт.
	m.mu.Lock()
	if current, ok := m.flights[flight.UserID]; ok && current == flight {
		delete(m.flights, flight.UserID)
	}
	m.mu.Unlock()

	if arrived {
		log.Printf("Travel completed: user %s -> world %s", flight.UserID, flight.ToWorld)
		if onArrival != nil {
			onArrival(flight.UserID, flight.ToWorld)
		}
	} else {
		log.Printf("Travel cancelled: user %s", flight.UserID)
	}
}

// CancelFlight — отменяет активный полёт пользователя: корабль остаётся
// в мире отправления, onArrival не вызывается. Возвращает true, если
// полёт был активен.
func (m *Manager) CancelFlight(userID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.flights[userID]
	if !ok {
		return false
	}
	close(f.CancelChan) // runFlight проснётся, удалит полёт, onArrival не вызовет
	return true
}

// GetFlight — возвращает информацию о текущем полёте пользователя.
//
// ВАЖНО: возвращается указатель на неизменяемую структуру. Если в будущем
// поля TravelInfo начнут мутировать (например, прогресс), потребуется
// либо возвращать копию, либо добавить внутренний мьютекс.
func (m *Manager) GetFlight(userID string) *TravelInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.flights[userID]
}

// IsInFlight — проверяет, находится ли пользователь в полёте.
func (m *Manager) IsInFlight(userID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.flights[userID]
	return ok
}