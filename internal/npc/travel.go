package npc

import (
	"time"

	"zorion/internal/models"
)

// FlightDuration — длительность полёта агента (спека §3.2, вариант A):
// общая формула models.TravelDuration (dist·speedFactor, минимум 3с, без
// потолка — потолок 20с /travel для игрока к фоновым агентам не применяется).
// speedFactor — npcSpeedFactor (своя настройка агентов, спека 91a §7.1).
func FlightDuration(dist float64, speedFactor float64) time.Duration {
	return models.TravelDuration(dist, speedFactor)
}
