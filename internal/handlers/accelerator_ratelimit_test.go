// internal/handlers/accelerator_ratelimit_test.go
// Тесты per-user token-bucket ручек мини-игры ускорителя (анти-бот, спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута §6.6): всплеск в ёмкость,
// восстановление токенов во времени, блокировка сверх частоты.
package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAcceleratorRateLimiterBurst(t *testing.T) {
	t0 := time.Now()
	l := newAcceleratorRateLimiter(1.0, 3)
	for i := 0; i < 3; i++ {
		require.Truef(t, l.allow("u1", t0), "всплеск в пределах ёмкости, токен %d", i)
	}
	require.False(t, l.allow("u1", t0), "ёмкость исчерпана — блок")
	// Другой игрок не затронут (лимит per-user).
	require.True(t, l.allow("u2", t0))
}

func TestAcceleratorRateLimiterRefill(t *testing.T) {
	t0 := time.Now()
	l := newAcceleratorRateLimiter(1.0, 1)
	require.True(t, l.allow("u1", t0))
	require.False(t, l.allow("u1", t0.Add(500*time.Millisecond)), "не долился целый токен")
	require.True(t, l.allow("u1", t0.Add(time.Second)), "через секунду токен восстановлен")
}

// Нулевая частота — строгий burst без восстановления (для тестов ручек).
func TestAcceleratorRateLimiterZeroRate(t *testing.T) {
	t0 := time.Now()
	l := newAcceleratorRateLimiter(0, 1)
	require.True(t, l.allow("u1", t0))
	require.False(t, l.allow("u1", t0.Add(time.Hour)), "нулевая частота не восстанавливает токены")
}
