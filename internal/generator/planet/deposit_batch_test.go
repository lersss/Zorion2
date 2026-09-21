// internal/generator/planet/deposit_batch_test.go
//
// T3: залежи несутся в COPY-буфер вместе с планетой (спека 2026-09-22-
// поселение-... §3.4) — второй буфер рядом с planets, 8 колонок на залежь.
package planet

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

func TestBatchBuffersCarryDeposits(t *testing.T) {
	pd := &PlanetData{
		ID: "p1", WorldID: "w1", Name: "P", OrbitIndex: 1, Data: []byte(`{}`),
		Deposits: []models.SurfaceDeposit{
			{ID: "d1", PlanetID: "p1", GoodID: 358, Stratum: "surface", Wealth: 0.5, Amount: 1000},
			{ID: "d2", PlanetID: "p1", GoodID: 359, Stratum: "surface", Wealth: 0.6, Amount: 900},
		},
	}
	b := newBatchBuffers(1)
	b.addPlanet(pd)
	require.Len(t, b.planetRows, 1)
	require.Len(t, b.depositRows, 2, "залежи несутся в отдельный COPY-буфер")
	require.Len(t, flatten(b.depositRows), 16, "8 колонок на залежь")
	require.False(t, b.isEmpty())

	b.reset()
	require.True(t, b.isEmpty(), "reset чистит оба буфера")
}

// T3: залежи пишутся COPY-ом в таблицу deposits (8 колонок) — вместе с
// планетой в той же транзакции (§3.4).
func TestCopyInDepositsUsesDepositsTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectPrepare(`COPY "deposits"`)
	mock.ExpectExec(`COPY "deposits"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`COPY "deposits"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	now := time.Now()
	rows := []interface{}{
		[]interface{}{"d1", "p1", int64(358), "surface", 0.5, 1000.0, now, now},
	}
	require.NoError(t, NewGenerator(db, 1).copyInDeposits(tx, flatten(rows)))
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}
