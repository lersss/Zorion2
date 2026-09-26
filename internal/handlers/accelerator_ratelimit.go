// internal/handlers/accelerator_ratelimit.go
// Простейший per-user token-bucket для ручек мини-игры ускорителя
// (анти-бот, спека 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута §6.6):
// offer/scan/boost одного игрока ограничены общей частотой. Игрок один — одна
// корзина; взрыв в ёмкость, дальше восстановление токенов по времени. Карта
// под мьютексом (AGENTS.md §0: map из нескольких горутин-обработчиков).
package handlers

import (
	"math"
	"sync"
	"time"
)

// acceleratorRateLimiter — token-bucket на игрока. rate — токенов в секунду,
// burst — ёмкость корзины (максимум токенов).
type acceleratorRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*acceleratorBucket
	rate    float64
	burst   float64
}

// acceleratorBucket — состояние корзины одного игрока.
type acceleratorBucket struct {
	tokens float64
	last   time.Time
}

func newAcceleratorRateLimiter(rate, burst float64) *acceleratorRateLimiter {
	return &acceleratorRateLimiter{
		buckets: make(map[string]*acceleratorBucket),
		rate:    rate,
		burst:   burst,
	}
}

// allow списывает один токен (now передаётся параметром — детерминизм в тестах).
// false — частота превышена. Потокобезопасно.
func (l *acceleratorRateLimiter) allow(userID string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.buckets[userID]
	if b == nil {
		b = &acceleratorBucket{tokens: l.burst, last: now}
		l.buckets[userID] = b
	} else {
		b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// allowAccelRequest — гейт частоты для offer/scan/boost. Лимитер не подключён
// (тесты/конфигурации без него) → запрос пропускается.
func (h *TravelHandlers) allowAccelRequest(userID string, now time.Time) bool {
	if h.accelRate == nil {
		return true
	}
	return h.accelRate.allow(userID, now)
}
