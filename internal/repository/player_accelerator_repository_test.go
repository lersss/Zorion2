// internal/repository/player_accelerator_repository_test.go
// Тесты репозитория отката ускорителя (спека ускорителя §3.3/§4.1): GetState
// (строки нет / строка) и ApplyBoost — гейты already_active/cooldown под
// row-lock, успех одной транзакцией, ошибка БД без списания отката.
package repository

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

const accelStateQuery = `SELECT last_boost_at, last_cooldown_min FROM player_accelerator WHERE user_id = \$1`

func TestPlayerAcceleratorGetStateNoRow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(accelStateQuery).WithArgs("u1").WillReturnError(sql.ErrNoRows)

	st, err := NewPlayerAcceleratorRepository(db).GetState("u1")
	require.NoError(t, err, "отсутствие строки — не ошибка")
	require.Nil(t, st.LastBoostAt)
	require.Nil(t, st.LastCooldownMin)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPlayerAcceleratorGetStateRow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	boostAt := now()
	mock.ExpectQuery(accelStateQuery).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"last_boost_at", "last_cooldown_min"}).AddRow(boostAt, 25))

	st, err := NewPlayerAcceleratorRepository(db).GetState("u1")
	require.NoError(t, err)
	require.NotNil(t, st.LastBoostAt)
	require.Equal(t, boostAt.UnixMilli(), st.LastBoostAt.UnixMilli())
	require.NotNil(t, st.LastCooldownMin)
	require.Equal(t, 25, *st.LastCooldownMin)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Reset обнуляет строку пользователя (спека §13); строки нет (0 строк) —
// не ошибка (идемпотентно).
func TestPlayerAcceleratorReset(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE player_accelerator SET last_boost_at = NULL, last_cooldown_min = NULL, updated_at = NOW\(\) WHERE user_id = \$1`).
		WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, NewPlayerAcceleratorRepository(db).Reset("u1"), "нет строки — не ошибка")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ошибка БД на Reset → ошибка.
func TestPlayerAcceleratorResetDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE player_accelerator SET last_boost_at = NULL, last_cooldown_min = NULL, updated_at = NOW\(\) WHERE user_id = \$1`).
		WithArgs("u1").
		WillReturnError(errors.New("boom"))

	require.Error(t, NewPlayerAcceleratorRepository(db).Reset("u1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Первый запуск (строки нет) — ускорение применяется, откат кладётся снимком.
func TestPlayerAcceleratorApplyBoostFirstRun(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	t0 := now()
	seg := models.PlayerFlight{
		UserID: "u1", FromWorld: "A", ToWorld: "B",
		StartX: 3, StartY: 4, StartTime: t0, ArriveAt: t0.Add(20 * time.Minute),
	}

	mock.ExpectBegin()
	mock.ExpectQuery(accelStateQuery + ` FOR UPDATE`).WithArgs("u1").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO player_flights \(user_id, from_world_id, to_world_id, start_x, start_y, start_time, arrive_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs("u1", "A", "B", 3.0, 4.0, t0, t0.Add(20*time.Minute)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO player_accelerator \(user_id, last_boost_at, last_cooldown_min, updated_at\) VALUES \(\$1, \$2, \$3, NOW\(\)\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs("u1", t0, 25).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	applied, reason, err := NewPlayerAcceleratorRepository(db).ApplyBoost("u1", t0, seg, 25, t0)
	require.NoError(t, err)
	require.True(t, applied)
	require.Empty(t, reason)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Повтор на том же сегменте (last_boost_at == expectStartTime) → already_active,
// откат НЕ списывается (И-9).
func TestPlayerAcceleratorApplyBoostAlreadyActive(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	t0 := now()
	mock.ExpectBegin()
	mock.ExpectQuery(accelStateQuery + ` FOR UPDATE`).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"last_boost_at", "last_cooldown_min"}).AddRow(t0, 25))
	mock.ExpectRollback()

	applied, reason, err := NewPlayerAcceleratorRepository(db).ApplyBoost("u1", t0,
		models.PlayerFlight{UserID: "u1", StartTime: t0, ArriveAt: t0.Add(time.Minute)}, 25, t0)
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, "already_active", reason)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Откат не истёк → cooldown, откат НЕ списывается.
func TestPlayerAcceleratorApplyBoostCooldown(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	t0 := now()
	mock.ExpectBegin()
	mock.ExpectQuery(accelStateQuery + ` FOR UPDATE`).WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"last_boost_at", "last_cooldown_min"}).AddRow(t0.Add(-time.Minute), 25))
	mock.ExpectRollback()

	applied, reason, err := NewPlayerAcceleratorRepository(db).ApplyBoost("u1", t0,
		models.PlayerFlight{UserID: "u1", StartTime: t0, ArriveAt: t0.Add(time.Minute)}, 25, t0)
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, "cooldown", reason)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ошибка БД на гейте → (false,"",err), изменения не применены.
func TestPlayerAcceleratorApplyBoostDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	t0 := now()
	mock.ExpectBegin()
	mock.ExpectQuery(accelStateQuery + ` FOR UPDATE`).WithArgs("u1").WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	applied, reason, err := NewPlayerAcceleratorRepository(db).ApplyBoost("u1", t0,
		models.PlayerFlight{UserID: "u1", StartTime: t0, ArriveAt: t0.Add(time.Minute)}, 25, t0)
	require.Error(t, err)
	require.False(t, applied)
	require.Empty(t, reason)
	require.NoError(t, mock.ExpectationsWereMet())
}
