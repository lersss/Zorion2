// internal/models/player_intrasystem_flight.go
// Внутрисистемный полёт игрока (таблица player_intrasystem_flights, спека
// 99.2.27 §3.2): одна запись на игрока (PK user_id), переживает рестарт
// сервера (паттерн 97a). from/to — объекты системы (star/planet/satellite);
// from_id/to_id — TEXT: UUID или синтетический id компаньона (§3.1).
package models

import "time"

// PlayerIntrasystemFlight — активный внутрисистемный полёт игрока.
type PlayerIntrasystemFlight struct {
	UserID    string
	WorldID   string // система (current_world_id на момент старта)
	FromType  string
	FromID    string
	ToType    string
	ToID      string
	StartTime time.Time
	ArriveAt  time.Time
}