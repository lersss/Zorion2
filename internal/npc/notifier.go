package npc

import (
	"log"
	"time"

	"zorion/internal/models"
)

// Notifier — получатель прибытий агентов (спека §5). Реальная реализация —
// WS batch с дросселем (npcNotificationMaxBatch, npcNotifyInterval) — этап 5;
// на этапе 2 — заглушка LogNotifier.
type Notifier interface {
	// NotifyArrival — агент прибыл в мир (только при notify_enabled).
	// Мир прибытия — *agent.TargetWorldID (на момент прибытия).
	NotifyArrival(agent models.NPCAgent, observedAt time.Time)
}

// LogNotifier — заглушка-приёмник прибытий до этапа 5 (логирует).
type LogNotifier struct{}

func (LogNotifier) NotifyArrival(agent models.NPCAgent, observedAt time.Time) {
	target := ""
	if agent.TargetWorldID != nil {
		target = *agent.TargetWorldID
	}
	log.Printf("👀 NPC-агент %s прибыл в мир %s (%s)", agent.Name, target,
		observedAt.Format(time.RFC3339))
}