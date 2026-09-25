// Тесты админской витрины r_breakdown owner-прохода (идея 2026-09-25 «R
// суммарный с составом в карточке поселения», §2/§4): r_breakdown непуст,
// сумма рядов == r_per_sec (инвариант единой точки сборки), ряд эффекта несёт
// человекочитаемое имя типа из каталога.
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// TestOwnerPassRBreakdownSum — витрина r_breakdown: ряды среды + ряд эффекта
// («Голод»), сумма == r_per_sec. Путь «в памяти» — без записи.
func TestOwnerPassRBreakdownSum(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	o := ownerInput(now.Add(-20 * time.Minute))
	o.Planet.TemperatureK = 365 // жара: вклад среды ненулевой

	expectOwnerPassNoBranches(mock, nil)
	mock.ExpectQuery(storageCellSelectSQL).WithArgs("settlement", "s1").WillReturnRows(storageCellRows())

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.NotEmpty(t, res.RBreakdown, "r_breakdown непуст")

	var sum float64
	var sawEffect bool
	for _, c := range res.RBreakdown {
		sum += c.Value
		if c.Name == "Голод" {
			sawEffect = true
		}
	}
	require.InDelta(t, res.RPerSec, sum, 1e-20, "сумма рядов == r_per_sec")
	require.True(t, sawEffect, "ряд эффекта несёт человекочитаемое имя типа (effect_types.name)")
}
