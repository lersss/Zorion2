package npc

import (
	"encoding/json"
	"sync"
	"time"
)

// ArrivalEvent — одно прибытие агента для уведомления (спека §5):
// агент + мир (имя — из сетки миров менеджера) + момент наблюдения.
type ArrivalEvent struct {
	AgentID    string
	AgentName  string
	WorldID    string
	WorldName  string
	ObservedAt time.Time
}

// NotificationBatch — буфер прибытий с дросселем (спека §2.2.C):
//   - сообщение отправляется не чаще раза в interval (npcNotifyInterval, 5с);
//   - в одном сообщении до maxBatch (npcNotificationMaxBatch, 100) детальных
//     записей; прибытия сверх 100 за окно НЕ хранятся — только счётчик total
//     (спека: «избыток — в поле total»);
//   - since — начало окна (первое прибытие окна).
//
// Чистая структура без WS: добавление — Add (из тика менеджера), отправка —
// FlushDue (по таймеру WSNotifier). Конкурентность: две горутины (тик
// менеджера + таймер отправителя) — sync.Mutex (AGENTS.md §0).
type NotificationBatch struct {
	mu       sync.Mutex
	arrivals []ArrivalEvent // ≤ maxBatch деталей текущего окна
	total    int            // прибытий за окно сверх maxBatch (счётчик)
	since    time.Time      // начало окна
	lastSent time.Time      // время последней отправки (дроссель)
	interval time.Duration
	maxBatch int
}

func NewNotificationBatch(interval time.Duration, maxBatch int) *NotificationBatch {
	return &NotificationBatch{
		interval: interval,
		maxBatch: maxBatch,
	}
}

// Interval — интервал дросселя (для настройки таймера отправителя).
func (b *NotificationBatch) Interval() time.Duration {
	return b.interval
}

// Add — прибытие в буфер. Детали хранятся до maxBatch, избыток окна —
// в total.
func (b *NotificationBatch) Add(e ArrivalEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.arrivals) == 0 && b.total == 0 {
		b.since = e.ObservedAt // начало нового окна
	}
	if len(b.arrivals) < b.maxBatch {
		b.arrivals = append(b.arrivals, e)
	} else {
		b.total++
	}
}

// FlushDue — собирает сообщение, если пора: прошло ≥ interval с прошлой
// отправки И в окне есть данные. Возвращает JSON-байты сообщения
// {"type":"npc_arrivals_batch","since":...,"arrivals":[...],"total":N}.
// Окно сбрасывается — следующее начнётся с нового прибытия.
func (b *NotificationBatch) FlushDue(now time.Time) ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.arrivals) == 0 && b.total == 0 {
		return nil, false
	}
	if now.Sub(b.lastSent) < b.interval {
		return nil, false // дроссель: раньше интервала не шлём
	}

	msg := batchMessage{
		Type:     "npc_arrivals_batch",
		Since:    b.since,
		Arrivals: make([]arrivalJSON, 0, len(b.arrivals)),
		Total:    b.total,
	}
	for _, a := range b.arrivals {
		msg.Arrivals = append(msg.Arrivals, arrivalJSON{
			Agent:      agentRef{ID: a.AgentID, Name: a.AgentName},
			World:      worldRef{ID: a.WorldID, Name: a.WorldName},
			ObservedAt: a.ObservedAt,
		})
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, false // не достижимо для этой структуры
	}

	b.arrivals = nil
	b.total = 0
	b.lastSent = now
	return data, true
}

// ==================== ФОРМАТ СООБЩЕНИЯ (спека §5) ====================

type batchMessage struct {
	Type     string        `json:"type"`
	Since    time.Time     `json:"since"`
	Arrivals []arrivalJSON `json:"arrivals"`
	Total    int           `json:"total"`
}

type arrivalJSON struct {
	Agent      agentRef  `json:"agent"`
	World      worldRef  `json:"world"`
	ObservedAt time.Time `json:"observed_at"`
}

type agentRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type worldRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}