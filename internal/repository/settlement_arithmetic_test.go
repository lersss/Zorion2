// internal/repository/settlement_arithmetic_test.go
// Витрина арифметики поселения (спека 2026-09-23-стадии-поселения §8.1/§8.2) и
// настройка видимости игроку (§8.3/§10): owner-проход несёт блок арифметики по
// позициям и новые поля ветки (число скорости, признак «не в наборе стадии»,
// «забираем»); ключ generation_config читается (дефолт true) и пишется.
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

// T-А2 (витрина): owner-проход несёт арифметику по позициям на текущем
// населении и витринные поля ветки. Население 1e9, число пары 667.2 → производим
// 667.2 ед/сутки, потребляем 600 (DefaultEatK) → сверх +67.2.
func TestSyncSettlementsArithmeticShowcase(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	processedAt := now.Add(-time.Minute)
	expectOwnerPassWithBranches(mock, ownerBranchRows(processedAt, "пища"), ownerComponentRows(), ownerBufferRows(), nil)
	mock.ExpectQuery(depositMemorySelectSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"planet_id", "id", "good_id", "amount"}).AddRow("p1", "dep1", int64(359), 5000.0))

	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{ownerInput(now.Add(-time.Minute))})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Arithmetic, 1, "блок арифметики по позиции")
	a := res.Arithmetic[0]
	require.Equal(t, "пища", a.Position)
	require.InDelta(t, ownerTestRate, a.ProducedPerDay, 1e-6)
	require.InDelta(t, settlement.DefaultEatK, a.ConsumedPerDay, 1e-6)
	require.InDelta(t, ownerTestRate-settlement.DefaultEatK, a.NetPerDay, 1e-6)

	require.Len(t, res.Branches, 1)
	b := res.Branches[0]
	require.NotNil(t, b.RatePerDayPerBillion, "число скорости пары отдаётся витрине")
	require.InDelta(t, ownerTestRate, *b.RatePerDayPerBillion, 1e-6)
	require.False(t, b.NotInStageSet, "рецепт 69 в наборе стадии 148")
	require.Len(t, b.Take, 1, "«забираем» по ветке (выход × quantity)")
	require.Equal(t, int64(359), b.Take[0].GoodID)
	require.InDelta(t, ownerTestRate, b.Take[0].PerDay, 1e-6)
}

// §3.5: рецепт не из набора стадии — ветка помечена (NotInStageSet), числа
// скорости нет (nil); она остаётся живой и инертной.
func TestSyncSettlementsRecipeNotInStageSet(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	processedAt := now.Add(-time.Minute)
	expectOwnerPassWithBranches(mock, ownerBranchRows(processedAt, "пища"), ownerComponentRows(), ownerBufferRows(), nil)
	mock.ExpectQuery(depositMemorySelectSQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"planet_id", "id", "good_id", "amount"}).AddRow("p1", "dep1", int64(359), 5000.0))

	o := ownerInput(now.Add(-time.Minute))
	o.SettlementTypeID = 999 // пары/набора (999, 69) нет
	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	res := out["s1"]
	require.Len(t, res.Branches, 1)
	b := res.Branches[0]
	require.True(t, b.NotInStageSet, "рецепт не в наборе стадии — помечен")
	require.Nil(t, b.RatePerDayPerBillion, "числа скорости нет → nil (не объявлено)")
	require.Nil(t, b.Take, "не производит → забирать нечего")
}

// §8.3/§10: отсутствие ключа → видимость включена по умолчанию.
func TestSettlementArithmeticVisibleDefaultTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT payload FROM generation_config WHERE key = \$1`).
		WithArgs(models.SettlementArithmeticVisibleKey).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}))

	got, err := SettlementArithmeticVisibleToPlayer(db)
	require.NoError(t, err)
	require.True(t, got, "дефолт — включено")
	require.NoError(t, mock.ExpectationsWereMet())
}

// §8.3: ключ со значением false → выключено (сервер не сериализует блок).
func TestSettlementArithmeticVisibleStoredFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT payload FROM generation_config WHERE key = \$1`).
		WithArgs(models.SettlementArithmeticVisibleKey).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow([]byte("false")))

	got, err := SettlementArithmeticVisibleToPlayer(db)
	require.NoError(t, err)
	require.False(t, got)
}

// §10: запись настройки — upsert ключа generation_config.
func TestSetSettlementArithmeticVisibleToPlayer(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO generation_config \(key, payload, updated_at\)\s+VALUES \(\$1, to_jsonb\(\$2::boolean\), NOW\(\)\)\s+ON CONFLICT \(key\) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW\(\)`).
		WithArgs(models.SettlementArithmeticVisibleKey, false).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, SetSettlementArithmeticVisibleToPlayer(db, false))
	require.NoError(t, mock.ExpectationsWereMet())
}
