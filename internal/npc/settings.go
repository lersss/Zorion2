package npc

import (
	"sync"
	"time"
)

// Settings — конфигурируемые лимиты NPCManager (спека §2.4).
//
// Хранятся в памяти менеджера, меняются админ-ручкой (этап 4, вкладка
// «NPC», §6). После рестарта сервера — снова дефолты: механизма БД-настроек
// в проекте нет, отдельная таблица ради пяти чисел для v1 не вводится.
// Конкурентность: тик читает, админ-ручка пишет — RWMutex (AGENTS.md §0).
type Settings struct {
	mu                  sync.RWMutex
	tickInterval        time.Duration // npcTickInterval, default 5s
	batchSize           int           // npcBatchSize, default 2000
	speedFactor         float64       // npcSpeedFactor, default 0.3 (скорость игрока)
	notifyInterval      time.Duration // npcNotifyInterval, default 5s (этап 5)
	notificationMaxBatch int          // npcNotificationMaxBatch, default 100 (этап 5)
}

// DefaultSettings — дефолты спеки §2.4.
func DefaultSettings() *Settings {
	return &Settings{
		tickInterval:         5 * time.Second,
		batchSize:            2000,
		speedFactor:          0.3,
		notifyInterval:       5 * time.Second,
		notificationMaxBatch: 100,
	}
}

func (s *Settings) TickInterval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tickInterval
}

func (s *Settings) BatchSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.batchSize
}

func (s *Settings) SpeedFactor() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.speedFactor
}

// NotifyInterval — интервал дросселя уведомлений (этап 5; на этапе 4 —
// для чтения админ-ручкой /admin/npc/settings).
func (s *Settings) NotifyInterval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.notifyInterval
}

// NotificationMaxBatch — максимум деталей в WS-сообщении (этап 5).
func (s *Settings) NotificationMaxBatch() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.notificationMaxBatch
}

// SetSpeedFactor — коэффициент длительности полёта из админки (этап 4, §6).
func (s *Settings) SetSpeedFactor(f float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f > 0 {
		s.speedFactor = f
	}
}

// SetTickInterval — интервал тика из админки (этап 4, §6). Минимум 1с —
// защита от случайного нуля/минуса.
func (s *Settings) SetTickInterval(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d >= time.Second {
		s.tickInterval = d
	}
}

// SetBatchSize — размер batch из админки (этап 4, §6).
func (s *Settings) SetBatchSize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > 0 {
		s.batchSize = n
	}
}