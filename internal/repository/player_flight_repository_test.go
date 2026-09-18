// internal/repository/player_flight_repository_test.go
// Тесты репозитория активных полётов игроков (идея 97a): Upsert
// (INSERT ON CONFLICT DO UPDATE — редирект 61a заменяет сегмент),
// Delete, ListAll. Хелпер now() — общий с user_repository_test.go.
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// Upsert: INSERT с ON CONFLICT (user_id) DO UPDATE — все колонки сегмента
// заменяются (новые start_x/y = точка P, start_time, arrive_at).
func TestPlayerFlightUpsert(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO player_flights \(user_id, from_world_id, to_world_id, start_x, start_y, start_time, arrive_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7\) ON CONFLICT \(user_id\) DO UPDATE SET`).
		WithArgs("u1", "w1", "w2", 1.5, 2.5, now(), now().Add(time.Hour)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = NewPlayerFlightRepository(db).Upsert(models.PlayerFlight{
		UserID: "u1", FromWorld: "w1", ToWorld: "w2",
		StartX: 1.5, StartY: 2.5, StartTime: now(), ArriveAt: now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPlayerFlightDelete(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM player_flights WHERE user_id = \$1`).
		WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewPlayerFlightRepository(db).Delete("u1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ListAll: все активные полёты (восстановление при старте сервера).
func TestPlayerFlightListAll(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT user_id, from_world_id, to_world_id, start_x, start_y, start_time, arrive_at FROM player_flights`).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "from_world_id", "to_world_id", "start_x", "start_y", "start_time", "arrive_at",
		}).AddRow("u1", "w1", "w2", 1.5, 2.5, now(), now().Add(time.Hour)))

	flights, err := NewPlayerFlightRepository(db).ListAll()
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, flights, 1)
	require.Equal(t, "u1", flights[0].UserID)
	require.Equal(t, "w1", flights[0].FromWorld)
	require.Equal(t, "w2", flights[0].ToWorld)
	require.Equal(t, 1.5, flights[0].StartX)
	require.Equal(t, 2.5, flights[0].StartY)
	require.Equal(t, now(), flights[0].StartTime)
	require.Equal(t, now().Add(time.Hour), flights[0].ArriveAt)
}