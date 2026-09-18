package models

import "time"

// PlayerFlight — активный полёт игрока (таблица player_flights, идея 97a):
// одна запись на игрока (PK user_id), переживает рестарт сервера.
// start_time/arrive_at — абсолютные времена сегмента (61a: редирект
// заменяет сегмент новыми start_x/y = точка P, start_time, arrive_at).
type PlayerFlight struct {
	UserID    string
	FromWorld string
	ToWorld   string
	StartX    float64
	StartY    float64
	StartTime time.Time
	ArriveAt  time.Time
}