// internal/handlers/pacman_notifier.go
//
// Нотификатор пакмана (спека 2026-09-20 §5): отдельная горутина с тикером
// 100 мс, копит состояние {последняя позиция, последний батч, eaten_total,
// total} и шлёт с лимитами по типам: позиция ≤10 Гц, поедание ≤1 событие/
// батч (≤200 миров в деталях), start/end по одному. Джоб пишет неблокирующе
// (мутекс на состояние), Broadcast — в горутине нотификатора: джоб никогда
// не блокируется на WS (медленный клиент не тормозит поедание).
package handlers

import (
	"encoding/json"
	"sync"
	"time"

	"zorion/internal/mapcache"
)

// Лимиты дросселя (спека §8).
const (
	pacmanPositionIntervalMs = 100 // позиция ≤10 Гц
	maxWorldsPerEatenEvent   = 200 // деталей поедания ≤200 миров
)

// pacmanWorldDTO — открытые данные мира в WS-событии (спека §5.2, 77a:
// имя/координаты/спектр видны всем; детали систем НЕ передаются).
type pacmanWorldDTO struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Spectral string  `json:"spectral"`
}

// pacmanNotice — персональное уведомление (SendToUser, §5.3).
type pacmanNotice struct {
	userID  string
	message string
}

// PacmanNotifier — приёмник событий пакмана: джоб пишет состояние
// (неблокирующе), горутина с тикером шлёт Broadcast с дросселем по типам.
type PacmanNotifier struct {
	hub  *WebSocketHub
	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}

	// start (1 раз)
	startPending bool
	total        int
	worldsPerSec float64
	startedAt    time.Time

	// позиция (≤10 Гц)
	posX, posY    float64
	posSeq        int
	posPending    bool
	lastPosSentAt time.Time

	// поедание (≤1 событие/батч)
	eatenTotal   int
	batchIndex   int
	eatenX       float64
	eatenY       float64
	eatenWorlds  []pacmanWorldDTO
	eatenPending bool

	// end (1 раз)
	endStatus     string
	endDurationMs int64
	endPending    bool

	// персональные уведомления
	notices []pacmanNotice
}

// NewPacmanNotifier — создаёт нотификатор (паттерн NewWSNotifier).
func NewPacmanNotifier(hub *WebSocketHub) *PacmanNotifier {
	return &PacmanNotifier{
		hub:  hub,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// Start запускает горутину рассылки (паттерн WSNotifier.Start).
func (n *PacmanNotifier) Start() {
	go n.loop()
}

// Stop останавливает горутину рассылки.
func (n *PacmanNotifier) Stop() {
	close(n.stop)
	<-n.done
}

// StartEvent — старт события (1 раз): сбрасывает состояние прошлого прогона.
func (n *PacmanNotifier) StartEvent(total int, worldsPerSec float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.total = total
	n.worldsPerSec = worldsPerSec
	n.startedAt = time.Now()
	n.eatenTotal = 0
	n.batchIndex = 0
	n.posSeq = 0
	n.posPending = false
	n.eatenPending = false
	n.endPending = false
	n.notices = nil
	n.startPending = true
}

// Position — новая позиция пакмана (центроид батча). Дроссель 10 Гц.
func (n *PacmanNotifier) Position(x, y float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.posX, n.posY = x, y
	n.posPending = true
}

// Eaten — батч съеден (≤1 событие/батч, ≤200 миров в деталях; счётчик
// eaten_total несёт остаток).
func (n *PacmanNotifier) Eaten(batch []mapcache.World, eatenTotal, total int, x, y float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.eatenTotal = eatenTotal
	n.total = total
	n.eatenX, n.eatenY = x, y
	n.batchIndex++
	n.eatenWorlds = make([]pacmanWorldDTO, 0, len(batch))
	for _, w := range batch {
		if len(n.eatenWorlds) >= maxWorldsPerEatenEvent {
			break
		}
		n.eatenWorlds = append(n.eatenWorlds, pacmanWorldDTO{
			ID: w.ID, Name: w.Name, X: w.X, Y: w.Y, Spectral: w.Spectral,
		})
	}
	n.eatenPending = true
}

// End — конец джоба (1 раз): done/canceled/error.
func (n *PacmanNotifier) End(status string, eatenTotal, total int, durationMs int64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.endStatus = status
	n.eatenTotal = eatenTotal
	n.total = total
	n.endDurationMs = durationMs
	n.endPending = true
}

// Notice — персональное уведомление игроку (SendToUser, §5.3).
func (n *PacmanNotifier) Notice(userID, message string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notices = append(n.notices, pacmanNotice{userID: userID, message: message})
}

func (n *PacmanNotifier) loop() {
	defer close(n.done)
	ticker := time.NewTicker(pacmanPositionIntervalMs * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-n.stop:
			return
		case <-ticker.C:
			n.flush()
		}
	}
}

// flush — отправка накопленного с дросселем по типам. Сообщения собираются
// под локом, Broadcast/SendToUser — вне лока (не блокируем джоб).
func (n *PacmanNotifier) flush() {
	var toBroadcast [][]byte
	var toSend []pacmanNotice

	n.mu.Lock()
	if n.startPending {
		n.startPending = false
		toBroadcast = append(toBroadcast, n.startMsg())
	}
	if n.posPending && time.Since(n.lastPosSentAt) >= pacmanPositionIntervalMs*time.Millisecond {
		n.posPending = false
		n.lastPosSentAt = time.Now()
		n.posSeq++
		toBroadcast = append(toBroadcast, n.positionMsg())
	}
	if n.eatenPending {
		n.eatenPending = false
		toBroadcast = append(toBroadcast, n.eatenMsg())
	}
	if n.endPending {
		n.endPending = false
		toBroadcast = append(toBroadcast, n.endMsg())
	}
	if len(n.notices) > 0 {
		toSend = n.notices
		n.notices = nil
	}
	n.mu.Unlock()

	for _, msg := range toBroadcast {
		n.hub.Broadcast(msg)
	}
	for _, nt := range toSend {
		msg, _ := json.Marshal(map[string]string{"type": "pacman_notice", "message": nt.message})
		n.hub.SendToUser(nt.userID, msg)
	}
}

// ==================== СБОРКА СООБЩЕНИЙ (под локом) ====================

func (n *PacmanNotifier) startMsg() []byte {
	msg, _ := json.Marshal(map[string]interface{}{
		"type":              "pacman_start",
		"total":             n.total,
		"worlds_per_second": n.worldsPerSec,
		"started_at":        n.startedAt.Format(time.RFC3339),
	})
	return msg
}

func (n *PacmanNotifier) positionMsg() []byte {
	msg, _ := json.Marshal(map[string]interface{}{
		"type": "pacman_position",
		"x":    n.posX,
		"y":    n.posY,
		"seq":  n.posSeq,
	})
	return msg
}

func (n *PacmanNotifier) eatenMsg() []byte {
	msg, _ := json.Marshal(map[string]interface{}{
		"type":        "pacman_eaten",
		"batch_index": n.batchIndex,
		"eaten_total": n.eatenTotal,
		"total":       n.total,
		"x":           n.eatenX,
		"y":           n.eatenY,
		"worlds":      n.eatenWorlds,
	})
	return msg
}

func (n *PacmanNotifier) endMsg() []byte {
	msg, _ := json.Marshal(map[string]interface{}{
		"type":        "pacman_end",
		"status":      n.endStatus,
		"eaten_total": n.eatenTotal,
		"total":       n.total,
		"duration_ms": n.endDurationMs,
	})
	return msg
}