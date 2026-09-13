package settlement

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Нет ни одной пригодной планеты — генератор не трогает таблицы.
func TestGenerateSettlementsNoSuitable(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`
		SELECT id, data FROM planets
	`).WillReturnRows(sqlmock.NewRows([]string{"id", "data"}).
		AddRow("p1", `{"water_percent":5,"temperature":288,"atmosphere":"азотно-кислородная","life":false}`).
		AddRow("p2", `{"water_percent":70,"temperature":400,"atmosphere":"азотно-кислородная","life":true}`))

	g := NewGenerator(db, 1)
	count, err := g.GenerateSettlements(context.Background(), DefaultPreset(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Шанс заселения 0 — пригодные планеты не заселяются.
func TestGenerateSettlementsChanceZero(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`
		SELECT id, data FROM planets
	`).WillReturnRows(sqlmock.NewRows([]string{"id", "data"}).
		AddRow("p1", `{"water_percent":70,"temperature":288,"atmosphere":"азотно-кислородная","life":true}`))

	p := DefaultPreset()
	p.Suitability.Chance = 0.0

	g := NewGenerator(db, 1)
	count, err := g.GenerateSettlements(context.Background(), p, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Пригодная планета без жизни заселяется (жизнь не обязательна).
func TestSuitableLifeNotRequired(t *testing.T) {
	p := DefaultPreset()
	assert.True(t, p.Suitability.suitable(70, 288, "азотно-кислородная", false, false, false))
}