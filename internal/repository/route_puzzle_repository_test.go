// internal/repository/route_puzzle_repository_test.go
// Тесты репозитория состояния сегментной задачи «Прокладка маршрута» (спека
// ускорителя §14.1): Get (строки нет / строка), Ensure (условный upsert +
// перечитывание), Reveal (успех и гейт «нет импульсов»), Delete.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

const routePuzzleGetQuery = `SELECT user_id, kind, segment_hash, secret, layout, revealed, pings_left, created_at FROM player_route_puzzle WHERE user_id = $1 AND kind = $2`

func TestRoutePuzzleGetNoRow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(routePuzzleGetQuery)).
		WithArgs("u1", "route").
		WillReturnError(sql.ErrNoRows)

	st, err := NewRoutePuzzleRepository(db).Get(context.Background(), "u1", "route")
	require.NoError(t, err, "отсутствие строки — не ошибка")
	require.Nil(t, st)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoutePuzzleGetRow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	created := now()
	mock.ExpectQuery(regexp.QuoteMeta(routePuzzleGetQuery)).
		WithArgs("u1", "route").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "kind", "segment_hash", "secret", "layout", "revealed", "pings_left", "created_at"}).
			AddRow("u1", "route", []byte{0x01, 0x02}, []byte{0x03}, []byte(`{"w":1}`), []byte(`[{"sector":2}]`), 3, created))

	st, err := NewRoutePuzzleRepository(db).Get(context.Background(), "u1", "route")
	require.NoError(t, err)
	require.NotNil(t, st)
	require.Equal(t, "u1", st.UserID)
	require.Equal(t, "route", st.Kind)
	require.Equal(t, []byte{0x01, 0x02}, st.SegmentHash)
	require.Equal(t, []byte{0x03}, st.Secret)
	require.Equal(t, []byte(`{"w":1}`), st.Layout)
	require.Equal(t, []byte(`[{"sector":2}]`), st.Revealed)
	require.Equal(t, 3, st.PingsLeft)
	require.Equal(t, created.UnixMilli(), st.CreatedAt.UnixMilli())
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ensure — условный upsert (обновляет только при смене segment_hash) +
// перечитывание КАНОНИЧЕСКОЙ строки (фикс гонки Get+Replace, спека ускорителя
// §14.1): конкурентные запросы одного игрока получают одну доску.
func TestRoutePuzzleEnsureUpsertAndReread(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO player_route_puzzle (user_id, kind, segment_hash, secret, layout, revealed, pings_left, created_at)
		VALUES ($1, $2, $3, $4, $5, '[]'::jsonb, $6, NOW())
		ON CONFLICT (user_id, kind) DO UPDATE SET
			segment_hash = EXCLUDED.segment_hash,
			secret = EXCLUDED.secret,
			layout = EXCLUDED.layout,
			revealed = '[]'::jsonb,
			pings_left = EXCLUDED.pings_left,
			created_at = NOW()
		WHERE player_route_puzzle.segment_hash IS DISTINCT FROM EXCLUDED.segment_hash`)).
		WithArgs("u1", "route", []byte{0x0a}, []byte{0x0b}, []byte(`{"w":2}`), 5).
		WillReturnResult(sqlmock.NewResult(1, 1))

	created := now()
	mock.ExpectQuery(regexp.QuoteMeta(routePuzzleGetQuery)).
		WithArgs("u1", "route").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "kind", "segment_hash", "secret", "layout", "revealed", "pings_left", "created_at"}).
			AddRow("u1", "route", []byte{0x0a}, []byte{0x0b}, []byte(`{"w":2}`), []byte(`[]`), 5, created))

	st, err := NewRoutePuzzleRepository(db).Ensure(context.Background(), &models.RoutePuzzle{
		UserID:      "u1",
		Kind:        "route",
		SegmentHash: []byte{0x0a},
		Secret:      []byte{0x0b},
		Layout:      []byte(`{"w":2}`),
		PingsLeft:   5,
	})
	require.NoError(t, err)
	require.NotNil(t, st)
	require.Equal(t, []byte{0x0a}, st.SegmentHash)
	require.Equal(t, 5, st.PingsLeft)
	require.Equal(t, []byte(`[]`), st.Revealed)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Reveal — успех: импульс списан, сектор дописан, возвращены revealed/pings.
func TestRoutePuzzleRevealSuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE player_route_puzzle
		SET pings_left = pings_left - 1,
		    revealed = revealed || jsonb_build_object('sector', $3::int, 'content', $4::jsonb)
		WHERE user_id = $1 AND kind = $2 AND pings_left > 0
		RETURNING revealed, pings_left`)).
		WithArgs("u1", "route", 4, `{"kind":"mud"}`).
		WillReturnRows(sqlmock.NewRows([]string{"revealed", "pings_left"}).
			AddRow([]byte(`[{"sector":4,"content":{"kind":"mud"}}]`), 2))

	revealed, pingsLeft, err := NewRoutePuzzleRepository(db).Reveal(context.Background(), "u1", "route", 4, `{"kind":"mud"}`)
	require.NoError(t, err)
	require.Equal(t, []byte(`[{"sector":4,"content":{"kind":"mud"}}]`), revealed)
	require.Equal(t, 2, pingsLeft)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Reveal при отсутствии импульсов / строки → ErrNoPingsLeft, состояние не изменено.
func TestRoutePuzzleRevealNoPings(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE player_route_puzzle
		SET pings_left = pings_left - 1,
		    revealed = revealed || jsonb_build_object('sector', $3::int, 'content', $4::jsonb)
		WHERE user_id = $1 AND kind = $2 AND pings_left > 0
		RETURNING revealed, pings_left`)).
		WithArgs("u1", "route", 4, `{"kind":"mud"}`).
		WillReturnError(sql.ErrNoRows)

	revealed, pingsLeft, err := NewRoutePuzzleRepository(db).Reveal(context.Background(), "u1", "route", 4, `{"kind":"mud"}`)
	require.ErrorIs(t, err, ErrNoPingsLeft)
	require.Nil(t, revealed)
	require.Equal(t, 0, pingsLeft)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Reveal при ошибке БД → ошибка (не подменяется на ErrNoPingsLeft).
func TestRoutePuzzleRevealDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`UPDATE player_route_puzzle`).
		WithArgs("u1", "route", 4, `{"kind":"mud"}`).
		WillReturnError(errors.New("boom"))

	_, _, err = NewRoutePuzzleRepository(db).Reveal(context.Background(), "u1", "route", 4, `{"kind":"mud"}`)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNoPingsLeft)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoutePuzzleDelete(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM player_route_puzzle WHERE user_id = $1 AND kind = $2`)).
		WithArgs("u1", "route").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewRoutePuzzleRepository(db).Delete(context.Background(), "u1", "route"))
	require.NoError(t, mock.ExpectationsWereMet())
}
