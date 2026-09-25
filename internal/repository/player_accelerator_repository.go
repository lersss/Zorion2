package repository

import (
	"database/sql"
	"time"

	"zorion/internal/models"
)

// PlayerAcceleratorRepository — состояние отката ускорителя игрока и
// атомарное применение ускорения (спека ускорителя §3.3/§4.1): одна
// транзакция «новый сегмент player_flights + снимок отката player_accelerator».
// Таблица player_accelerator НЕ входит в truncateTables (состояние игрока,
// переживает очистку вселенной).
type PlayerAcceleratorRepository struct {
	db *sql.DB
}

func NewPlayerAcceleratorRepository(db *sql.DB) *PlayerAcceleratorRepository {
	return &PlayerAcceleratorRepository{db: db}
}

// GetState — состояние отката игрока. Строки нет → пустое состояние (nil-поля),
// без ошибки.
func (r *PlayerAcceleratorRepository) GetState(userID string) (*models.PlayerAcceleratorState, error) {
	st := &models.PlayerAcceleratorState{UserID: userID}
	err := r.db.QueryRow(
		`SELECT last_boost_at, last_cooldown_min FROM player_accelerator WHERE user_id = $1`,
		userID,
	).Scan(&st.LastBoostAt, &st.LastCooldownMin)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

// Reset — админский сброс СОБСТВЕННОГО таймера ускорителя вызывающего игрока
// (идея ускорителя §13): обнуляет откат и признак «ускорение действует»
// (BoostActive сравнивает last_boost_at со StartTime). Идемпотентно: строки
// нет → UPDATE не трогает ноль строк, без ошибки.
func (r *PlayerAcceleratorRepository) Reset(userID string) error {
	_, err := r.db.Exec(
		`UPDATE player_accelerator SET last_boost_at = NULL, last_cooldown_min = NULL, updated_at = NOW() WHERE user_id = $1`,
		userID,
	)
	return err
}

// ApplyBoost — атомарное применение ускорения (спека §4.1 шаг 3). Под row-lock
// строки отката (FOR UPDATE):
//   - last_boost_at.UnixMilli() == expectStartTime.UnixMilli() → already_active
//     (не списываем: одно активное ускорение на сегмент, И-9);
//   - иначе откат не истёк → cooldown (не списываем);
//   - иначе upsert нового сегмента player_flights + снимок отката одной
//     транзакцией → applied.
//
// Любая ошибка БД → (false, "", err): откат не списан, сегмент не изменён.
// Первый запуск (строки нет) FOR UPDATE не покрывает — сериализуется мьютексом
// менеджера (BoostFlight) и ON CONFLICT (user_id) DO UPDATE (М-1, §4.2).
func (r *PlayerAcceleratorRepository) ApplyBoost(userID string, expectStartTime time.Time, seg models.PlayerFlight, cooldownMin int, now time.Time) (bool, string, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return false, "", err
	}
	defer tx.Rollback()

	var lastBoostAt *time.Time
	var lastCooldown *int
	err = tx.QueryRow(
		`SELECT last_boost_at, last_cooldown_min FROM player_accelerator WHERE user_id = $1 FOR UPDATE`,
		userID,
	).Scan(&lastBoostAt, &lastCooldown)
	if err != nil && err != sql.ErrNoRows {
		return false, "", err
	}
	if err == nil {
		if models.BoostActive(lastBoostAt, expectStartTime) {
			return false, "already_active", nil
		}
		if models.CooldownRemaining(lastBoostAt, lastCooldown, now) > 0 {
			return false, "cooldown", nil
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO player_flights (user_id, from_world_id, to_world_id, start_x, start_y, start_time, arrive_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO UPDATE SET
			from_world_id = EXCLUDED.from_world_id,
			to_world_id = EXCLUDED.to_world_id,
			start_x = EXCLUDED.start_x,
			start_y = EXCLUDED.start_y,
			start_time = EXCLUDED.start_time,
			arrive_at = EXCLUDED.arrive_at`,
		seg.UserID, seg.FromWorld, seg.ToWorld, seg.StartX, seg.StartY, seg.StartTime, seg.ArriveAt,
	); err != nil {
		return false, "", err
	}

	if _, err := tx.Exec(`
		INSERT INTO player_accelerator (user_id, last_boost_at, last_cooldown_min, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			last_boost_at = EXCLUDED.last_boost_at,
			last_cooldown_min = EXCLUDED.last_cooldown_min,
			updated_at = NOW()`,
		userID, now, cooldownMin,
	); err != nil {
		return false, "", err
	}

	if err := tx.Commit(); err != nil {
		return false, "", err
	}
	return true, "", nil
}
