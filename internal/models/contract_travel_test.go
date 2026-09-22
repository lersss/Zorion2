package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Формула срока перелёта (спека 2026-09-22-контракт-перелёт-и-доска §4.3):
// flight_time_ref = max(dist·speed_factor_ref, 3с); срок = ref·(1+reserve) = ref·1.5;
// окно предложения = ref·10. Единый источник для флоу игрока и агента.
func TestTravelDeadlineAndOfferWindow(t *testing.T) {
	// dist=100 → flight 30с → срок 45с, окно 300с.
	require.Equal(t, 30*time.Second, TravelFlightTimeRef(100))
	require.Equal(t, 45*time.Second, TravelDeadline(100))
	require.Equal(t, 300*time.Second, TravelOfferWindow(100))
	// dist=1 → пол минимум 3с → срок 4.5с, окно 30с.
	require.Equal(t, 3*time.Second, TravelFlightTimeRef(1))
	require.Equal(t, 4500*time.Millisecond, TravelDeadline(1))
	require.Equal(t, 30*time.Second, TravelOfferWindow(1))
}

// Длительность полёта — общая формула (speedFactor = скорость двигателя,
// меньше — быстрее); совпадает с calcTravelDuration /travel (спека §4.1).
func TestTravelDuration(t *testing.T) {
	require.Equal(t, 30*time.Second, TravelDuration(100, 0.3))
	require.Equal(t, 3*time.Second, TravelDuration(1, 0.3), "пол минимум 3с")
	require.Equal(t, 150*time.Second, TravelDuration(100, 1.5), "медленнее — дольше")
}
