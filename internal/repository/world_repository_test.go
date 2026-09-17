// internal/repository/world_repository_test.go
// PickSpawnWorld — стартовый мир для нового игрока (решение создателя
// 2026-09-17: «давай его пока к людям кидать»): мир с поселением расы humans
// (ближайший к центру), фолбэк — ближайший к центру вообще, миров нет — nil.
package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPickSpawnWorldHumansPreferred(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Мир с людьми найден — фолбэк (ближайший) НЕ вызывается.
	mock.ExpectQuery(`SELECT w.id FROM worlds w WHERE EXISTS.*race_id = 'humans'.*ORDER BY \(w.coord_x \* w.coord_x \+ w.coord_y \* w.coord_y\) LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("w-humans"))

	id, err := NewWorldRepository(db).PickSpawnWorld(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, id)
	require.Equal(t, "w-humans", *id, "мир с людьми приоритетнее ближайшего без людей")
}

func TestPickSpawnWorldFallbackClosest(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Миров с людьми нет — фолбэк на ближайший к центру.
	mock.ExpectQuery(`SELECT w.id FROM worlds w WHERE EXISTS.*race_id = 'humans'.*LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT id FROM worlds ORDER BY \(coord_x \* coord_x \+ coord_y \* coord_y\) LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("w-closest"))

	id, err := NewWorldRepository(db).PickSpawnWorld(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, id)
	require.Equal(t, "w-closest", *id, "фолбэк — ближайший к центру мир")
}

func TestPickSpawnWorldNoWorlds(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Миров нет вообще — (nil, nil): регистрация не ломается.
	mock.ExpectQuery(`SELECT w.id FROM worlds w WHERE EXISTS.*race_id = 'humans'.*LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT id FROM worlds ORDER BY \(coord_x \* coord_x \+ coord_y \* coord_y\) LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	id, err := NewWorldRepository(db).PickSpawnWorld(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Nil(t, id, "миров нет — nil, не ошибка")
}