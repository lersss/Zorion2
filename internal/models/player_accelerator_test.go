package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBoostActiveUnixMilli(t *testing.T) {
	segment := time.UnixMilli(1_700_000_000_123)

	require.False(t, BoostActive(nil, segment), "нет применения — не активно")

	// Один усечённый момент: лишние микросекунды (из памяти vs БД) не мешают.
	same := time.UnixMilli(1_700_000_000_123).Add(500 * time.Microsecond)
	require.True(t, BoostActive(&same, segment))

	// Другая миллисекунда (новый сегмент: /travel, разворот 61a, Restore) — сброс.
	other := time.UnixMilli(1_700_000_000_124)
	require.False(t, BoostActive(&other, segment))
}

func TestCooldownRemaining(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	require.Zero(t, CooldownRemaining(nil, nil, base), "строки нет — готов")

	cd := 25
	boostAt := base.Add(-5 * time.Minute)
	require.Equal(t, 20*time.Minute, CooldownRemaining(&boostAt, &cd, base))

	// Истёк.
	old := base.Add(-30 * time.Minute)
	require.Zero(t, CooldownRemaining(&old, &cd, base))

	// Обратный край: точная граница — уже готов.
	exact := base.Add(-25 * time.Minute)
	require.Zero(t, CooldownRemaining(&exact, &cd, base))
}
