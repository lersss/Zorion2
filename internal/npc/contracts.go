// internal/npc/contracts.go
//
// NPC-агент как исполнитель контракта-перелёта (спека
// 2026-09-22-контракт-перелёт-и-доска §1.5, B2b). Отдельного поиска контрактов
// у агента нет (распоряжение менеджера, О-п8): планировщик выбирает маршрут
// сам, агент берёт лишь совпадающий открытый перелёт с целью-системой. Работает
// только в горутине тика (§0: состояние агентов пишет один тик).
package npc

import (
	"log"
	"time"

	"zorion/internal/models"
)

// ContractStore — контракты-перелёты для агента-исполнителя (спека перелёта
// §1.5, B2b). Реализация — *repository.ContractRepository; интерфейс — для
// юнит-тестов планировщика.
type ContractStore interface {
	// ListOpenSystemTravels — открытые перелёты с целью-системой (одна выборка
	// на тик, инвариант 2: не по агенту).
	ListOpenSystemTravels() ([]models.TravelContractRef, error)
	// TakeTravel — атомарное взятие перелёта агентом (false — гонка/истёк);
	// expiresAt != nil перебазирует срок от взятия (§4.3).
	TakeTravel(contractID, agentID string, expiresAt *time.Time) (bool, error)
	// CloseArrivedTravels — закрытие перелётов агентов по прибытии (одна tx).
	CloseArrivedTravels(arrivals []models.AgentTravelArrival) (int, error)
}

// SetContracts — подключает репозиторий контрактов (спека перелёта §1.5,
// B2b). Вызывать до Start (как SetNotifier): поле читает горутина тика.
func (m *Manager) SetContracts(c ContractStore) {
	m.contracts = c
}

// travelRoutes — индекс открытых перелётов-системы по маршруту from→dest на
// тик (одна выборка вместо запроса на агента). nil — контракты не подключены
// или ошибка чтения: старты идут как обычно, взятия нет.
func (m *Manager) travelRoutes() map[string][]string {
	if m.contracts == nil {
		return nil
	}
	list, err := m.contracts.ListOpenSystemTravels()
	if err != nil {
		log.Printf("❌ NPCManager: ListOpenSystemTravels: %v", err)
		return nil
	}
	if len(list) == 0 {
		return nil
	}
	index := make(map[string][]string, len(list))
	for _, t := range list {
		k := routeKey(t.FromWorldID, t.DestWorldID)
		index[k] = append(index[k], t.ID)
	}
	return index
}

// takeMatchingTravel — агент берёт открытый перелёт, совпадающий с выбранным
// маршрутом (from → dest) (спека перелёта §1.5). Срок перебазируется от взятия
// (models.TravelDeadline по dist, §4.3) — как у игрока; dist считает планировщик
// по своей сетке миров (отдельного чтения координат из БД нет). Контрактов на
// маршрут может быть несколько: пробуем по очереди; гонку отсекает атомарный
// TakeTravel (false — контракт уже взят другим агентом).
func (m *Manager) takeMatchingTravel(index map[string][]string, agentID, from, dest string, dist float64, now time.Time) {
	if index == nil {
		return
	}
	rebase := now.Add(models.TravelDeadline(dist))
	for _, contractID := range index[routeKey(from, dest)] {
		taken, err := m.contracts.TakeTravel(contractID, agentID, &rebase)
		if err != nil {
			log.Printf("❌ NPCManager: TakeTravel (агент %s, контракт %s): %v", agentID, contractID, err)
			return
		}
		if taken {
			return
		}
	}
}

// closeArrivedTravels — закрытие перелётов агентов по прибытии (спека §1.5):
// прибытия тика (updates, CurrentWorldID = цель полёта) — одним вызовом,
// одна транзакция на батч. Nil-безопасно.
func (m *Manager) closeArrivedTravels(updates []models.AgentStatusUpdate) {
	if m.contracts == nil || len(updates) == 0 {
		return
	}
	arrivals := make([]models.AgentTravelArrival, 0, len(updates))
	for _, u := range updates {
		arrivals = append(arrivals, models.AgentTravelArrival{AgentID: u.ID, WorldID: u.CurrentWorldID})
	}
	if _, err := m.contracts.CloseArrivedTravels(arrivals); err != nil {
		log.Printf("❌ NPCManager: CloseArrivedTravels: %v", err)
	}
}

// routeKey — ключ маршрута from→dest (разделитель — NUL: id миров его не
// содержат, коллизии ключей нет).
func routeKey(from, dest string) string {
	return from + "\x00" + dest
}
