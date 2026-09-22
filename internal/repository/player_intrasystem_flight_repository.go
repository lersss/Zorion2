// internal/repository/player_intrasystem_flight_repository.go
// Доступ к таблице player_intrasystem_flights (спека 99.2.27 §3.2): активный
// внутрисистемный полёт игрока, одна запись на игрока (PK user_id), переживает
// рестарт сервера (паттерн 97a). Атомарные операции (С-1): старт (строка +
// current_position = in_flight), прибытие (current_position = orbit + удаление
// строки), отмена при старте межзвёздного (удаление строки + позиция NULL).
package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"zorion/internal/models"
)

// PlayerIntrasystemFlightRepository — доступ к player_intrasystem_flights.
type PlayerIntrasystemFlightRepository struct {
	db *sql.DB
}

func NewPlayerIntrasystemFlightRepository(db *sql.DB) *PlayerIntrasystemFlightRepository {
	return &PlayerIntrasystemFlightRepository{db: db}
}

// intraFlightColumns — порядок колонок SELECT по player_intrasystem_flights
// (совпадает со скан-порядком ListAll).
const intraFlightColumns = `user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at`

// Upsert — запись/замена активного внутрисистемного полёта (INSERT ON CONFLICT
// DO UPDATE): обычный старт — новая строка, редирект — замена сегмента.
func (r *PlayerIntrasystemFlightRepository) Upsert(f models.PlayerIntrasystemFlight) error {
	query := `
		INSERT INTO player_intrasystem_flights (user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id) DO UPDATE SET
			world_id = EXCLUDED.world_id,
			from_type = EXCLUDED.from_type,
			from_id = EXCLUDED.from_id,
			to_type = EXCLUDED.to_type,
			to_id = EXCLUDED.to_id,
			start_time = EXCLUDED.start_time,
			arrive_at = EXCLUDED.arrive_at
	`
	_, err := r.db.Exec(query, f.UserID, f.WorldID, f.FromType, f.FromID, f.ToType, f.ToID, f.StartTime, f.ArriveAt)
	return err
}

// Delete — удаляет активный внутрисистемный полёт игрока (прибытие, отмена,
// Restore).
func (r *PlayerIntrasystemFlightRepository) Delete(userID string) error {
	_, err := r.db.Exec(`DELETE FROM player_intrasystem_flights WHERE user_id = $1`, userID)
	return err
}

// ListAll — все активные внутрисистемные полёты (Restore при старте сервера).
func (r *PlayerIntrasystemFlightRepository) ListAll() ([]models.PlayerIntrasystemFlight, error) {
	rows, err := r.db.Query(`SELECT ` + intraFlightColumns + ` FROM player_intrasystem_flights`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flights []models.PlayerIntrasystemFlight
	for rows.Next() {
		var f models.PlayerIntrasystemFlight
		if err := rows.Scan(&f.UserID, &f.WorldID, &f.FromType, &f.FromID, &f.ToType, &f.ToID, &f.StartTime, &f.ArriveAt); err != nil {
			return nil, err
		}
		flights = append(flights, f)
	}
	return flights, rows.Err()
}

// StartAtomic — старт внутрисистемного полёта одной транзакцией (С-1):
// строка полёта + current_position = {status: in_flight} не расходятся
// (краш-окно закрыто; модалка/my_position сразу видят полёт).
func (r *PlayerIntrasystemFlightRepository) StartAtomic(f models.PlayerIntrasystemFlight, pos *models.CurrentPosition) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("start intra flight: begin: %w", err)
	}
	defer tx.Rollback()

	if err := r.StartAtomicTx(tx, f, pos); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("start intra flight: commit: %w", err)
	}
	return nil
}

// StartAtomicTx — тело StartAtomic в транзакции вызывающего (спека поясов
// этап 3 §6.5: перелив буфера захода в трюм — в ОДНОЙ транзакции со стартом
// полёта и записью новой позиции).
func (r *PlayerIntrasystemFlightRepository) StartAtomicTx(tx *sql.Tx, f models.PlayerIntrasystemFlight, pos *models.CurrentPosition) error {
	posJSON, err := json.Marshal(pos)
	if err != nil {
		return fmt.Errorf("start intra flight: marshal position: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO player_intrasystem_flights (user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id) DO UPDATE SET
			world_id = EXCLUDED.world_id,
			from_type = EXCLUDED.from_type,
			from_id = EXCLUDED.from_id,
			to_type = EXCLUDED.to_type,
			to_id = EXCLUDED.to_id,
			start_time = EXCLUDED.start_time,
			arrive_at = EXCLUDED.arrive_at
	`, f.UserID, f.WorldID, f.FromType, f.FromID, f.ToType, f.ToID, f.StartTime, f.ArriveAt); err != nil {
		return fmt.Errorf("start intra flight: upsert: %w", err)
	}
	if _, err := tx.Exec(`UPDATE users SET current_position = $1, updated_at = NOW() WHERE id = $2`, posJSON, f.UserID); err != nil {
		return fmt.Errorf("start intra flight: update position: %w", err)
	}
	return nil
}

// ArriveAtomic — прибытие внутрисистемного полёта одной транзакцией (С-1):
// current_position = {status: orbit} на цели + удаление строки. Удаление
// ограничено сегментом (start_time/arrive_at) — TOCTOU-гвард: если редирект
// уже заменил строку новым сегментом, строка нового полёта не удаляется.
func (r *PlayerIntrasystemFlightRepository) ArriveAtomic(userID string, pos *models.CurrentPosition, startTime, arriveAt interface{}) error {
	posJSON, err := json.Marshal(pos)
	if err != nil {
		return fmt.Errorf("arrive intra flight: marshal position: %w", err)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("arrive intra flight: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE users SET current_position = $1, updated_at = NOW() WHERE id = $2`, posJSON, userID); err != nil {
		return fmt.Errorf("arrive intra flight: update position: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM player_intrasystem_flights WHERE user_id = $1 AND start_time = $2 AND arrive_at = $3`,
		userID, startTime, arriveAt,
	); err != nil {
		return fmt.Errorf("arrive intra flight: delete row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("arrive intra flight: commit: %w", err)
	}
	return nil
}

// CancelAtomic — отмена внутрисистемного полёта при старте межзвёздного (С1,
// спека 99.2.27 §4.2): удаление строки + current_position = NULL одной
// транзакцией — не остаётся окна, где intra отменён, а позиция ещё не NULL.
// Дельта 99.2.30 §3.3 (M1): в той же транзакции очищается намерение
// композитного маршрута (pending_destination = NULL) — любой /travel-старт
// без объекта цели снимает намерение (ИН-1 «намерение только в сегменте»).
func (r *PlayerIntrasystemFlightRepository) CancelAtomic(userID string) error {
	return r.cancelAtomic(userID, nil)
}

// CancelAtomicWithDestination — отмена внутрисистемного полёта при старте
// межзвёздного с намерением композитного маршрута (спека 99.2.30 §3.2, ИН-4):
// одна транзакция — DELETE строки player_intrasystem_flights + UPDATE users
// SET current_position = NULL, pending_destination = $dest. Намерение пишется
// атомарно со стартом /travel; окно «намерение без полёта» (краш между
// транзакцией и StartFlight) закрыто Restore-обработкой (§4.5).
func (r *PlayerIntrasystemFlightRepository) CancelAtomicWithDestination(userID string, dest *models.PendingDestination) error {
	return r.cancelAtomic(userID, dest)
}

// cancelAtomic — общая транзакция отмены intra + позиция NULL + намерение.
// dest == nil → pending_destination = NULL (M1); dest != nil → запись (ИН-4).
func (r *PlayerIntrasystemFlightRepository) cancelAtomic(userID string, dest *models.PendingDestination) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("cancel intra flight: begin: %w", err)
	}
	defer tx.Rollback()

	if err := r.CancelAtomicWithDestinationTx(tx, userID, dest); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cancel intra flight: commit: %w", err)
	}
	return nil
}

// CancelAtomicWithDestinationTx — тело отмены в транзакции вызывающего (спека
// поясов этап 3 §6.5: перелив буфера захода в трюм — в ОДНОЙ транзакции с
// обнулением позиции при межзвёздном старте).
func (r *PlayerIntrasystemFlightRepository) CancelAtomicWithDestinationTx(tx *sql.Tx, userID string, dest *models.PendingDestination) error {
	destJSON, err := marshalDestination(dest)
	if err != nil {
		return fmt.Errorf("cancel intra flight: marshal destination: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM player_intrasystem_flights WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("cancel intra flight: delete row: %w", err)
	}
	if _, err := tx.Exec(`UPDATE users SET current_position = NULL, pending_destination = $1, updated_at = NOW() WHERE id = $2`, destJSON, userID); err != nil {
		return fmt.Errorf("cancel intra flight: clear position: %w", err)
	}
	return nil
}

// marshalDestination — JSONB-значение намерения: nil → NULL (SQL NULL).
func marshalDestination(dest *models.PendingDestination) (interface{}, error) {
	if dest == nil {
		return nil, nil
	}
	b, err := json.Marshal(dest)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
