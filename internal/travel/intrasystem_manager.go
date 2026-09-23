// internal/travel/intrasystem_manager.go
// Менеджер внутрисистемных полётов (спека 99.2.27 §3.3): тот же пакет travel,
// паттерн travel.Manager (97a) — карта под sync.RWMutex, горутина-таймер с
// CancelChan, персистентность через store, TOCTOU-гвард удаления строки.
// Взаимоисключение с межзвёздным полётом — в хендлерах (§4.1/§4.2):
// одновременно активны не могут (И3).
package travel

import (
	"log"
	"sync"
	"time"

	"zorion/internal/models"
)

// IntraFlightInfo — информация о текущем внутрисистемном полёте.
type IntraFlightInfo struct {
	UserID     string
	WorldID    string // система (current_world_id на момент старта)
	FromType   string // star|planet|satellite
	FromID     string // UUID или синтетический id компаньона
	ToType     string
	ToID       string
	StartTime  time.Time
	ArriveAt   time.Time
	waitFor    time.Duration // сколько ждать горутине (Restore: остаток, ArriveAt не уменьшается)
	CancelChan chan struct{}
	closeOnce  sync.Once // защита от double-close (гонка CancelIntraFlight+StartIntraFlight, ревью 99.2.27)
}

// cancel — закрывает CancelChan ровно один раз (sync.Once): узкая гонка
// CancelIntraFlight + StartIntraFlight не может закрыть закрытый канал → panic.
func (f *IntraFlightInfo) cancel() {
	f.closeOnce.Do(func() { close(f.CancelChan) })
}

// IntraFlightStore — хранилище активных внутрисистемных полётов (паттерн 97a).
// Реализация — *repository.PlayerIntrasystemFlightRepository; интерфейс — для
// юнит-тестов.
type IntraFlightStore interface {
	Upsert(f models.PlayerIntrasystemFlight) error
	Delete(userID string) error
	ListAll() ([]models.PlayerIntrasystemFlight, error)
}

// IntraArrivalFunc — колбэк прибытия внутрисистемного полёта (спека §3.6):
// пишет current_position = orbit на цели (атомарно с удалением строки, С-1)
// и авто-знание (source=presence, С6). Реализация — в handlers (общая для
// хендлера и Restore в main.go).
type IntraArrivalFunc func(userID string, f *IntraFlightInfo)

// IntrasystemManager управляет активными внутрисистемными полётами.
type IntrasystemManager struct {
	mu      sync.RWMutex
	flights map[string]*IntraFlightInfo
	store   IntraFlightStore // персистентность (99.2.27); nil — без БД (юнит-тесты)
}

// NewIntrasystemManager создаёт менеджер внутрисистемных полётов.
// store — хранилище (best-effort: ошибка БД логируется, полёт в памяти
// продолжает работать); nil — без персистентности (юнит-тесты).
func NewIntrasystemManager(store IntraFlightStore) *IntrasystemManager {
	return &IntrasystemManager{
		flights: make(map[string]*IntraFlightInfo),
		store:   store,
	}
}

// StartIntraFlight — запускает внутрисистемный полёт для пользователя.
// Редирект (другая цель при активном полёте): старый полёт сигнализируется
// об отмене (close CancelChan) и заменяется новым — от объекта отправления
// (спека §3.5, упрощение против 61a: полёты короткие, интерполяция в а.е. —
// оверинжиниринг). Горутина старого полёта при пробуждении проверяет, что
// удаляет ИМЕННО СВОЙ полёт (сравнение указателей).
//
// Персистентность: запись в БД (INSERT ON CONFLICT DO UPDATE). Атомарность
// «строка + current_position = in_flight» — на стороне хендлера (StartAtomic,
// С-1); здесь — best-effort дубль (как 97a).
func (m *IntrasystemManager) StartIntraFlight(
	userID, worldID, fromType, fromID, toType, toID string,
	duration time.Duration,
	onArrival IntraArrivalFunc,
) *IntraFlightInfo {
	m.mu.Lock()
	if existing, ok := m.flights[userID]; ok {
		existing.cancel()
	}
	now := time.Now()
	flight := &IntraFlightInfo{
		UserID:     userID,
		WorldID:    worldID,
		FromType:   fromType,
		FromID:     fromID,
		ToType:     toType,
		ToID:       toID,
		StartTime:  now,
		ArriveAt:   now.Add(duration),
		waitFor:    duration,
		CancelChan: make(chan struct{}),
	}
	m.flights[userID] = flight
	m.mu.Unlock()

	if m.store != nil {
		if err := m.store.Upsert(models.PlayerIntrasystemFlight{
			UserID:    userID,
			WorldID:   worldID,
			FromType:  fromType,
			FromID:    fromID,
			ToType:    toType,
			ToID:      toID,
			StartTime: flight.StartTime,
			ArriveAt:  flight.ArriveAt,
		}); err != nil {
			log.Printf("⚠️ intrasystem: upsert flight (user %s): %v", userID, err)
		}
	}

	go runIntraFlight(m, flight, onArrival)
	return flight
}

// runIntraFlight — фоновая горутина одного внутрисистемного полёта.
// Ждёт либо истечения таймера (waitFor: полная длительность или остаток
// после Restore), либо сигнала отмены. При завершении:
//  1. Под мьютексом удаляет полёт, ТОЛЬКО если он всё ещё актуален.
//  2. onArrival вызывается ТОЛЬКО если полёт всё ещё актуален (current) —
//     гвард от transient-позиции (ревью 99.2.27): при совпадении таймера с
//     отменой (CancelAtomic уже NULL-нул позицию) старая горутина не пишет
//     orbit вместо NULL; при редиректе старая горутина не пишет позицию
//     нового полёта.
//  3. БД-строку удаляет ТОЛЬКО актуальный полёт и ПОСЛЕ onArrival под ОДНИМ
//     локом (TOCTOU-гвард 97a): старая горутина редиректа строку не трогает.
func runIntraFlight(m *IntrasystemManager, flight *IntraFlightInfo, onArrival IntraArrivalFunc) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔥 panic in intrasystem flight goroutine (user %s): %v", flight.UserID, r)
		}
	}()

	arrived := false
	select {
	case <-time.After(flight.waitFor):
		arrived = true
	case <-flight.CancelChan:
		arrived = false
	}

	m.mu.Lock()
	current := false
	if cur, ok := m.flights[flight.UserID]; ok && cur == flight {
		delete(m.flights, flight.UserID)
		current = true
	}
	m.mu.Unlock()

	if arrived && current {
		log.Printf("Intrasystem flight completed: user %s -> %s %s", flight.UserID, flight.ToType, flight.ToID)
		if onArrival != nil {
			onArrival(flight.UserID, flight)
		}
		// Проверка и Delete под одним локом (TOCTOU-фикс 97a): иначе между
		// проверкой и удалением новый StartIntraFlight успел бы вставить
		// полёт в map и сделать Upsert — старая горутина удалила бы строку
		// нового сегмента. DB-вызов под локом приемлемо (как CancelIntraFlight).
		m.mu.Lock()
		_, replaced := m.flights[flight.UserID]
		if !replaced && m.store != nil {
			if err := m.store.Delete(flight.UserID); err != nil {
				log.Printf("⚠️ intrasystem: delete flight (user %s): %v", flight.UserID, err)
			}
		}
		m.mu.Unlock()
	} else {
		log.Printf("Intrasystem flight cancelled: user %s", flight.UserID)
	}
}

// CancelIntraFlight — отменяет активный внутрисистемный полёт: onArrival не
// вызывается. Возвращает true, если полёт был активен.
//
// Удаляет полёт из map СРАЗУ (не ждёт горутину, в отличие от 97a): после
// отмены полёт не «наш» — горутина при пробуждении не вызовет onArrival
// (transient-позиция orbit после CancelAtomic, ревью 99.2.27) и не закроет
// канал повторно (double-close panic при гонке CancelIntraFlight+StartIntraFlight).
// Чистит БД под локом (97a): иначе DELETE мог бы снести строку нового полёта,
// стартовавшего между разблокировкой и удалением.
func (m *IntrasystemManager) CancelIntraFlight(userID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.flights[userID]
	if !ok {
		return false
	}
	delete(m.flights, userID)
	f.cancel()
	if m.store != nil {
		if err := m.store.Delete(userID); err != nil {
			log.Printf("⚠️ intrasystem: cancel delete flight (user %s): %v", userID, err)
		}
	}
	return true
}

// GetIntraFlight — возвращает информацию о текущем внутрисистемном полёте
// пользователя (указатель на неизменяемую структуру, как 97a).
func (m *IntrasystemManager) GetIntraFlight(userID string) *IntraFlightInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.flights[userID]
}

// IsInIntraFlight — проверяет, находится ли пользователь во внутрисистемном полёте.
func (m *IntrasystemManager) IsInIntraFlight(userID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.flights[userID]
	return ok
}

// RestoreIntra — восстановление активных внутрисистемных полётов при старте
// сервера (паттерн 97a, спека §3.5). Вызывается ПОСЛЕ Restore межзвёздных
// (С-1): сначала player_flights, затем внутрисистемные.
//
//   - у игрока есть межзвёздная строка (hasInterstellar) → внутрисистемная
//     удаляется БЕЗ вызова onArrival (иначе позиция {planet, старый мир}
//     запишется при новом current_world_id — нарушение ИП-1);
//   - игрок уже в другой системе (currentWorldID != row.WorldID — межзвёздная
//     прибыла при Restore) → то же;
//   - битая система/цель (validTarget = false) → строка удаляется, а игрок
//     возвращается на «орбиту звезды» системы (resetPosition, B27): иначе он
//     остаётся в статусе in_flight без строки полёта и застревает;
//   - arrive_at <= now → onArrival сразу + удаление строки;
//   - arrive_at > now → перерегистрация в памяти с ОРИГИНАЛЬНЫМИ временами,
//     горутина ждёт остаток (waitFor = arrive_at - now).
//
// resetPosition вызывается ТОЛЬКО в ветке битой цели; в ветках «межзвёздная
// побеждает» и «игрок уже в другой системе» позицией владеет межзвёздный полёт
// (current_position = NULL / уже записан прибытием) — трогать её нельзя.
func (m *IntrasystemManager) RestoreIntra(
	now time.Time,
	validTarget func(worldID, objType, objID string) bool,
	hasInterstellar func(userID string) bool,
	currentWorldID func(userID string) string,
	onArrival IntraArrivalFunc,
	resetPosition func(userID, worldID string),
) {
	if m.store == nil {
		return
	}
	rows, err := m.store.ListAll()
	if err != nil {
		log.Printf("❌ intrasystem: RestoreIntra ListAll: %v", err)
		return
	}
	for _, row := range rows {
		// С-1: межзвёздная строка побеждает — intra удаляется без onArrival.
		if hasInterstellar != nil && hasInterstellar(row.UserID) {
			if err := m.store.Delete(row.UserID); err != nil {
				log.Printf("⚠️ intrasystem: Restore delete (interstellar, user %s): %v", row.UserID, err)
			}
			continue
		}
		// ИП-1: игрок уже в другой системе (межзвёздная прибыла при Restore).
		if currentWorldID != nil {
			if cw := currentWorldID(row.UserID); cw != "" && cw != row.WorldID {
				if err := m.store.Delete(row.UserID); err != nil {
					log.Printf("⚠️ intrasystem: Restore delete (moved system, user %s): %v", row.UserID, err)
				}
				continue
			}
		}
		// Битая система/цель (перегенерация) — полёт не восстанавливаем;
		// игрок возвращается на «орбиту звезды» системы (B27), иначе он
		// застревает в in_flight без строки полёта.
		if validTarget != nil && !validTarget(row.WorldID, row.ToType, row.ToID) {
			if err := m.store.Delete(row.UserID); err != nil {
				log.Printf("⚠️ intrasystem: Restore delete (broken target, user %s): %v", row.UserID, err)
			}
			if resetPosition != nil {
				resetPosition(row.UserID, row.WorldID)
			}
			continue
		}
		if !row.ArriveAt.After(now) {
			// Полёт уже должен был завершиться — засчитываем прибытие сразу.
			if onArrival != nil {
				onArrival(row.UserID, &IntraFlightInfo{
					UserID:    row.UserID,
					WorldID:   row.WorldID,
					FromType:  row.FromType,
					FromID:    row.FromID,
					ToType:    row.ToType,
					ToID:      row.ToID,
					StartTime: row.StartTime,
					ArriveAt:  row.ArriveAt,
				})
			}
			if err := m.store.Delete(row.UserID); err != nil {
				log.Printf("⚠️ intrasystem: Restore delete (arrived, user %s): %v", row.UserID, err)
			}
			continue
		}
		// Будущий arrive_at — перерегистрируем полёт в памяти.
		flight := &IntraFlightInfo{
			UserID:     row.UserID,
			WorldID:    row.WorldID,
			FromType:   row.FromType,
			FromID:     row.FromID,
			ToType:     row.ToType,
			ToID:       row.ToID,
			StartTime:  row.StartTime,
			ArriveAt:   row.ArriveAt,
			waitFor:    row.ArriveAt.Sub(now),
			CancelChan: make(chan struct{}),
		}
		m.mu.Lock()
		m.flights[row.UserID] = flight
		m.mu.Unlock()
		go runIntraFlight(m, flight, onArrival)
	}
}