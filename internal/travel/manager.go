// internal/travel/manager.go
package travel

import (
	"log"
	"sync"
	"time"

	"zorion/internal/models"
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
	waitFor    time.Duration // сколько ждать горутине (97a: при Restore — остаток, Duration не уменьшается)
	CancelChan chan struct{}
}

// FlightStore — хранилище активных полётов игроков (идея 97a). Реализация —
// *repository.PlayerFlightRepository; интерфейс — для юнит-тестов.
type FlightStore interface {
	Upsert(f models.PlayerFlight) error
	Delete(userID string) error
	ListAll() ([]models.PlayerFlight, error)
}

// Manager управляет активными полётами.
type Manager struct {
	mu      sync.RWMutex
	flights map[string]*TravelInfo
	store   FlightStore // персистентность (97a); nil — без БД (юнит-тесты)
}

// NewManager создаёт менеджер полётов. store — хранилище активных полётов
// (персистентность best-effort: ошибка БД логируется, полёт в памяти
// продолжает работать); nil — без персистентности (юнит-тесты).
func NewManager(store FlightStore) *Manager {
	return &Manager{
		flights: make(map[string]*TravelInfo),
		store:   store,
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
//
// Персистентность (97a): запись в БД (INSERT ON CONFLICT DO UPDATE —
// редирект заменяет сегмент). Best-effort: ошибка БД не валит полёт.
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
		waitFor:    duration,
		CancelChan: make(chan struct{}),
	}
	m.flights[userID] = flight
	m.mu.Unlock()

	if m.store != nil {
		if err := m.store.Upsert(models.PlayerFlight{
			UserID:    userID,
			FromWorld: fromWorldID,
			ToWorld:   toWorldID,
			StartX:    startX,
			StartY:    startY,
			StartTime: flight.StartTime,
			ArriveAt:  flight.StartTime.Add(duration),
		}); err != nil {
			log.Printf("⚠️ travel: upsert flight (user %s): %v", userID, err)
		}
	}

	go runFlight(m, flight, onArrival)
}

// runFlight — фоновая горутина одного полёта.
//
// Ждёт либо истечения таймера (waitFor: полная длительность или остаток
// после Restore), либо сигнала отмены.
// При завершении:
//  1. Под мьютексом удаляет полёт, ТОЛЬКО если он всё ещё актуален.
//  2. Если долетел (не отменён) — вызывает onArrival.
//  3. БД-строку удаляет ТОЛЬКО актуальный полёт и ПОСЛЕ onArrival
//     (критик 97a, среднее 1 + мелкое 3): старая горутина редиректа строку
//     не трогает (её уже заменил новый StartFlight), а краш между удалением
//     и апдейтом мира не теряет полёт. Проверка актуальности и Delete — под
//     ОДНИМ локом (TOCTOU-фикс ревью 97a): иначе между проверкой и удалением
//     новый StartFlight успел бы вставить полёт в map и сделать Upsert —
//     старая горутина удалила бы строку нового сегмента.
func runFlight(m *Manager, flight *TravelInfo, onArrival func(userID, worldID string)) {
	// Защита от паники в колбэке — не валим процесс.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔥 panic in flight goroutine (user %s): %v", flight.UserID, r)
		}
	}()

	arrived := false
	select {
	case <-time.After(flight.waitFor):
		arrived = true
	case <-flight.CancelChan:
		arrived = false
	}

	// Удаляем полёт, только если это всё ещё НАШ полёт.
	m.mu.Lock()
	current := false
	if cur, ok := m.flights[flight.UserID]; ok && cur == flight {
		delete(m.flights, flight.UserID)
		current = true
	}
	m.mu.Unlock()

	if arrived {
		log.Printf("Travel completed: user %s -> world %s", flight.UserID, flight.ToWorld)
		if onArrival != nil {
			onArrival(flight.UserID, flight.ToWorld)
		}
		if current {
			// Новый полёт уже стартовал (Upsert заменил строку) — не удаляем.
			// Проверка и Delete под одним локом (TOCTOU-фикс ревью 97a):
			// иначе между проверкой и удалением новый StartFlight успел бы
			// вставить полёт в map и сделать Upsert — старая горутина удалила
			// бы строку нового сегмента. DB-вызов под локом приемлемо — так
			// уже делает CancelFlight.
			m.mu.Lock()
			_, replaced := m.flights[flight.UserID]
			if !replaced && m.store != nil {
				if err := m.store.Delete(flight.UserID); err != nil {
					log.Printf("⚠️ travel: delete flight (user %s): %v", flight.UserID, err)
				}
			}
			m.mu.Unlock()
		}
	} else {
		log.Printf("Travel cancelled: user %s", flight.UserID)
	}
}

// CancelFlight — отменяет активный полёт пользователя: корабль остаётся
// в мире отправления, onArrival не вызывается. Возвращает true, если
// полёт был активен. Чистит БД (97a, критик, мелкое 5) — под локом,
// чтобы не удалить строку нового полёта, стартовавшего между
// разблокировкой и DELETE.
func (m *Manager) CancelFlight(userID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.flights[userID]
	if !ok {
		return false
	}
	close(f.CancelChan) // runFlight проснётся, удалит полёт, onArrival не вызовет
	if m.store != nil {
		if err := m.store.Delete(userID); err != nil {
			log.Printf("⚠️ travel: cancel delete flight (user %s): %v", userID, err)
		}
	}
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

// Restore — восстановление активных полётов при старте сервера (97a).
// Вызывается ДО старта HTTP-сервера (гонок нет).
//
//   - arrive_at <= now → onArrival(userID, toWorldID) сразу, строка удаляется
//     (как у NPC: прибытие засчитывается по абсолютному времени);
//   - arrive_at > now → полёт перерегистрируется в памяти с ОРИГИНАЛЬНЫМИ
//     StartTime/Duration/StartX/Y/FromWorld/ToWorld из БД; горутина ждёт
//     remaining = arrive_at - now (поле waitFor), Duration НЕ уменьшается —
//     иначе интерполяция visibility.go (timeSince(StartTime)/Duration) и
//     точка P редиректа 61a разъедутся (критик, мелкое 4);
//   - любой битый from/to (мир удалён перегенерацией) → полёт НЕ
//     восстанавливается, строка удаляется (фолбэк 42a/61a, критик, среднее 2).
func (m *Manager) Restore(now time.Time, worldExists func(id string) bool, onArrival func(userID, worldID string)) {
	if m.store == nil {
		return
	}
	rows, err := m.store.ListAll()
	if err != nil {
		log.Printf("❌ travel: Restore ListAll: %v", err)
		return
	}
	for _, row := range rows {
		if !worldExists(row.FromWorld) || !worldExists(row.ToWorld) {
			// Мир удалён перегенерацией — полёт не восстанавливаем.
			if err := m.store.Delete(row.UserID); err != nil {
				log.Printf("⚠️ travel: Restore delete (broken world, user %s): %v", row.UserID, err)
			}
			continue
		}
		if !row.ArriveAt.After(now) {
			// Полёт уже должен был завершиться — засчитываем прибытие сразу.
			if onArrival != nil {
				onArrival(row.UserID, row.ToWorld)
			}
			if err := m.store.Delete(row.UserID); err != nil {
				log.Printf("⚠️ travel: Restore delete (arrived, user %s): %v", row.UserID, err)
			}
			continue
		}
		// Будущий arrive_at — перерегистрируем полёт в памяти.
		flight := &TravelInfo{
			UserID:     row.UserID,
			FromWorld:  row.FromWorld,
			ToWorld:    row.ToWorld,
			StartX:     row.StartX,
			StartY:     row.StartY,
			StartTime:  row.StartTime,
			Duration:   row.ArriveAt.Sub(row.StartTime),
			waitFor:    row.ArriveAt.Sub(now),
			CancelChan: make(chan struct{}),
		}
		m.mu.Lock()
		m.flights[row.UserID] = flight
		m.mu.Unlock()
		go runFlight(m, flight, onArrival)
	}
}