// internal/handlers/ws_notifier.go
// Уведомления о прибытии NPC-агентов через WebSocket (спека 20a.1 §5):
// реализация npc.Notifier — NotificationBatch (дроссель §2.2.C) +
// WSHub.Broadcast всем подключённым (ролей пока нет; по ролям — после
// 99.2.14). Имя мира — из сетки менеджера (npc.Manager.WorldName).
package handlers

import (
	"time"

	"zorion/internal/models"
	"zorion/internal/npc"
)

// WSNotifier — приёмник прибытий агентов: копит в batch, шлёт не чаще
// раза в npcNotifyInterval (таймер), ≤ npcNotificationMaxBatch деталей +
// total. Замена заглушки LogNotifier (этап 2).
type WSNotifier struct {
	hub     *WebSocketHub
	manager *npc.Manager
	batch   *npc.NotificationBatch
	tick    time.Duration // период проверки дросселя (interval/2, мин 100мс)
	stop    chan struct{}
	done    chan struct{}
}

// NewWSNotifier — создаёт отправитель. interval/maxBatch — из Settings
// менеджера (npcNotifyInterval, npcNotificationMaxBatch, §2.4); параметрами
// — для тестов (нотификации из админки не меняются, этап 4).
func NewWSNotifier(hub *WebSocketHub, manager *npc.Manager, interval time.Duration, maxBatch int) *WSNotifier {
	tick := interval / 2
	if tick < 100*time.Millisecond {
		tick = 100 * time.Millisecond
	}
	return &WSNotifier{
		hub:     hub,
		manager: manager,
		batch:   npc.NewNotificationBatch(interval, maxBatch),
		tick:    tick,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start запускает таймер отправки накопленных прибытий.
func (n *WSNotifier) Start() {
	go n.loop()
}

// Stop останавливает таймер.
func (n *WSNotifier) Stop() {
	close(n.stop)
	<-n.done
}

// NotifyArrival — реализация npc.Notifier: вызывается планировщиком только
// при notify_enabled агента (проверка на этапе 2, processArrivals).
func (n *WSNotifier) NotifyArrival(agent models.NPCAgent, observedAt time.Time) {
	if agent.TargetWorldID == nil {
		return // битый кортеж — мир неизвестен, уведомлять нечего
	}
	n.batch.Add(npc.ArrivalEvent{
		AgentID:    agent.ID,
		AgentName:  agent.Name,
		WorldID:    *agent.TargetWorldID,
		WorldName:  n.manager.WorldName(*agent.TargetWorldID),
		ObservedAt: observedAt,
	})
}

func (n *WSNotifier) loop() {
	defer close(n.done)
	timer := time.NewTimer(n.tick)
	defer timer.Stop()
	for {
		select {
		case <-n.stop:
			return
		case <-timer.C:
			n.flush()
			timer.Reset(n.tick)
		}
	}
}

// flush — отправка накопленного, если дроссель разрешил (FlushDue).
func (n *WSNotifier) flush() {
	msg, ok := n.batch.FlushDue(time.Now())
	if !ok {
		return
	}
	n.hub.Broadcast(msg)
}