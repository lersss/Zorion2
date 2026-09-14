package npc

import "time"

// minFlightDuration — нижняя граница длительности полёта агента (спека §3.2).
const minFlightDuration = 3 * time.Second

// FlightDuration — длительность полёта агента (спека §3.2, вариант A):
// duration = dist × speedFactor, минимум 3с, без потолка. Потолок 20с —
// UX-особенность /travel для игрока, к фоновым агентам не применяется.
func FlightDuration(dist float64, speedFactor float64) time.Duration {
	d := time.Duration(dist * speedFactor * float64(time.Second))
	if d < minFlightDuration {
		return minFlightDuration
	}
	return d
}