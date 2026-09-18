package repository

import (
	"database/sql"

	"zorion/internal/models"
)

// PlayerFlightRepository — доступ к таблице player_flights (идея 97a):
// активный полёт игрока, одна запись на игрока (PK user_id), переживает
// рестарт сервера.
type PlayerFlightRepository struct {
	db *sql.DB
}

func NewPlayerFlightRepository(db *sql.DB) *PlayerFlightRepository {
	return &PlayerFlightRepository{db: db}
}

// playerFlightColumns — порядок колонок SELECT по player_flights (совпадает
// со скан-порядком ListAll).
const playerFlightColumns = `user_id, from_world_id, to_world_id, start_x, start_y, start_time, arrive_at`

// Upsert — запись/замена активного полёта игрока (INSERT ON CONFLICT DO
// UPDATE): обычный старт — новая строка, редирект 61a — замена сегмента
// (новые start_x/y = точка P, новый start_time, новый arrive_at).
func (r *PlayerFlightRepository) Upsert(f models.PlayerFlight) error {
	query := `
		INSERT INTO player_flights (user_id, from_world_id, to_world_id, start_x, start_y, start_time, arrive_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO UPDATE SET
			from_world_id = EXCLUDED.from_world_id,
			to_world_id = EXCLUDED.to_world_id,
			start_x = EXCLUDED.start_x,
			start_y = EXCLUDED.start_y,
			start_time = EXCLUDED.start_time,
			arrive_at = EXCLUDED.arrive_at
	`
	_, err := r.db.Exec(query, f.UserID, f.FromWorld, f.ToWorld, f.StartX, f.StartY, f.StartTime, f.ArriveAt)
	return err
}

// Delete — удаляет активный полёт игрока (прибытие, отмена, Restore).
func (r *PlayerFlightRepository) Delete(userID string) error {
	_, err := r.db.Exec(`DELETE FROM player_flights WHERE user_id = $1`, userID)
	return err
}

// ListAll — все активные полёты (восстановление при старте сервера, 97a).
func (r *PlayerFlightRepository) ListAll() ([]models.PlayerFlight, error) {
	rows, err := r.db.Query(`SELECT ` + playerFlightColumns + ` FROM player_flights`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flights []models.PlayerFlight
	for rows.Next() {
		var f models.PlayerFlight
		if err := rows.Scan(&f.UserID, &f.FromWorld, &f.ToWorld, &f.StartX, &f.StartY, &f.StartTime, &f.ArriveAt); err != nil {
			return nil, err
		}
		flights = append(flights, f)
	}
	return flights, rows.Err()
}