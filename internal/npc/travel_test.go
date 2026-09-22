// internal/npc/travel_test.go
// Тесты формулы длительности полёта агента (спека 20a.1 §3.2, вариант A):
// duration = dist × speedFactor, минимум 3с, без потолка 20с.
package npc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFlightDurationProportional(t *testing.T) {
	// Эталон из спеки: средний перелёт в радиусе 1000 (dist=667) ≈ 200с.
	// Общая формула models.TravelDuration округляет до целых секунд (как /travel).
	d := FlightDuration(667, 0.3)
	require.Equal(t, 200*time.Second, d)
}

func TestFlightDurationMin3s(t *testing.T) {
	// dist=1 → 0.3с < мин 3с.
	require.Equal(t, 3*time.Second, FlightDuration(1, 0.3))
	// dist=0 → 0с < мин 3с.
	require.Equal(t, 3*time.Second, FlightDuration(0, 0.3))
	// dist=9 → 2.7с < мин 3с; dist=11 → 3.3с → 3с (целые секунды ≥ мин).
	require.Equal(t, 3*time.Second, FlightDuration(9, 0.3))
	require.Equal(t, 3*time.Second, FlightDuration(11, 0.3))
}

func TestFlightDurationNoCap(t *testing.T) {
	// Без потолка 20с (UX /travel к агентам не применяется, §3.2/В3/П4):
	// длительность строго ∝ расстоянию, потолка нет.
	d1 := FlightDuration(1000, 0.3)
	d2 := FlightDuration(20000, 0.3)
	require.InDelta(t, 20, d2.Seconds()/d1.Seconds(), 0.01)
	require.Greater(t, d2, 20*time.Second, "далёкий полёт длится дольше потолка /travel")
}