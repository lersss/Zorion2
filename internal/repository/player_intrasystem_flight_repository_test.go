// internal/repository/player_intrasystem_flight_repository_test.go
// Тесты репозитория внутрисистемных полётов (спека 99.2.27 §3.2/§3.6):
// Upsert (INSERT ON CONFLICT DO UPDATE — редирект заменяет сегмент), Delete,
// ListAll, атомарные операции С-1: StartAtomic (строка + позиция in_flight),
// ArriveAtomic (позиция orbit + удаление строки), CancelAtomic (удаление
// строки + позиция NULL). Хелпер now() — общий с user_repository_test.go.
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

func TestIntraFlightUpsert(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO player_intrasystem_flights \(user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs("u1", "w1", "star", "w1", "planet", "p1", now(), now().Add(time.Hour)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = NewPlayerIntrasystemFlightRepository(db).Upsert(models.PlayerIntrasystemFlight{
		UserID: "u1", WorldID: "w1",
		FromType: "star", FromID: "w1", ToType: "planet", ToID: "p1",
		StartTime: now(), ArriveAt: now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIntraFlightDelete(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1`).
		WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewPlayerIntrasystemFlightRepository(db).Delete("u1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIntraFlightListAll(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at FROM player_intrasystem_flights`).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "world_id", "from_type", "from_id", "to_type", "to_id", "start_time", "arrive_at",
		}).AddRow("u1", "w1", "star", "w1", "planet", "p1", now(), now().Add(time.Hour)))

	flights, err := NewPlayerIntrasystemFlightRepository(db).ListAll()
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, flights, 1)
	require.Equal(t, "u1", flights[0].UserID)
	require.Equal(t, "w1", flights[0].WorldID)
	require.Equal(t, "star", flights[0].FromType)
	require.Equal(t, "planet", flights[0].ToType)
	require.Equal(t, "p1", flights[0].ToID)
}

// StartAtomic (С-1): строка полёта + current_position = in_flight одной
// транзакцией — позиция и таблица не расходятся.
func TestIntraFlightStartAtomic(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO player_intrasystem_flights \(user_id, world_id, from_type, from_id, to_type, to_id, start_time, arrive_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs("u1", "w1", "star", "w1", "planet", "p1", now(), now().Add(time.Hour)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = NewPlayerIntrasystemFlightRepository(db).StartAtomic(models.PlayerIntrasystemFlight{
		UserID: "u1", WorldID: "w1",
		FromType: "star", FromID: "w1", ToType: "planet", ToID: "p1",
		StartTime: now(), ArriveAt: now().Add(time.Hour),
	}, models.InFlightPosition("star", "w1", "planet", "p1", now(), now().Add(time.Hour)))
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ArriveAtomic (С-1): current_position = orbit на цели + удаление строки
// (ограничено сегментом start_time/arrive_at — TOCTOU-гвард).
func TestIntraFlightArriveAtomic(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	start := now()
	arrive := now().Add(time.Hour)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET current_position = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(sqlmock.AnyArg(), "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1 AND start_time = \$2 AND arrive_at = \$3`).
		WithArgs("u1", start, arrive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = NewPlayerIntrasystemFlightRepository(db).ArriveAtomic("u1", models.OrbitPosition("planet", "p1"), start, arrive)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// CancelAtomic (С1): удаление строки + current_position = NULL одной
// транзакцией — не остаётся окна, где intra отменён, а позиция ещё не NULL.
func TestIntraFlightCancelAtomic(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM player_intrasystem_flights WHERE user_id = \$1`).
		WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET current_position = NULL, updated_at = NOW\(\) WHERE id = \$1`).
		WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = NewPlayerIntrasystemFlightRepository(db).CancelAtomic("u1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}