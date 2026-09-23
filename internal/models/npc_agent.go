package models

import "time"

// NPCAgentStatus — статус NPC-агента (спека 20a.1 §2.1).
type NPCAgentStatus string

const (
	NPCAgentStatusIdle      NPCAgentStatus = "idle"      // на миру, ждёт запуска полёта
	NPCAgentStatusFlying    NPCAgentStatus = "flying"    // в пути (время-функция: позиция от depart_at/arrive_at, §3.1)
	NPCAgentStatusObserving NPCAgentStatus = "observing" // наблюдение; в v1 транзиентно — сводится к last_observed_at (§4)
)

// NPCAgent — наблюдатель-агент (спека 20a.1 §2.1). Автономно перемещается
// между мирами; по прибытии помечает мир посещённым (last_observed_at),
// пересчёт населения не запускает (спека §4). Единственный писатель
// состояния — NPCManager (спека §9 И1).
type NPCAgent struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Status         NPCAgentStatus `json:"status"`
	CurrentWorldID string         `json:"current_world_id"`          // idle: мир, где агент стоит; flying: откуда летит
	FromWorldID    *string        `json:"from_world_id,omitempty"`   // старт текущего полёта
	TargetWorldID  *string        `json:"target_world_id,omitempty"` // цель текущего полёта
	DepartAt       *time.Time     `json:"depart_at,omitempty"`       // вылет
	ArriveAt       *time.Time     `json:"arrive_at,omitempty"`       // прибытие (абсолютно — переживает рестарт)
	NotifyEnabled  bool           `json:"notify_enabled"`
	RaceID         string         `json:"race_id,omitempty"`          // раса агента (спека 2026-09-23 §4.1); пусто = не задана → нейтральный корабль
	LastObservedAt *time.Time     `json:"last_observed_at,omitempty"` // последнее посещение мира
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// AgentStatusUpdate — смена состояния агента планировщиком (спека 20a.1
// §2.2.A: «Всё изменение одного агента — одной транзакцией»). Значимые поля
// зависят от Status:
//   - прибытие (flying → idle): CurrentWorldID = цель полёта,
//     LastObservedAt = now;
//   - старт (idle → flying): FromWorldID, TargetWorldID, DepartAt, ArriveAt.
type AgentStatusUpdate struct {
	ID             string
	Status         NPCAgentStatus
	CurrentWorldID string     // прибытие: цель становится текущим миром
	LastObservedAt *time.Time // прибытие: момент наблюдения
	FromWorldID    string     // старт
	TargetWorldID  string     // старт
	DepartAt       *time.Time // старт
	ArriveAt       *time.Time // старт
}