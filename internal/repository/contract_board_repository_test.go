// internal/repository/contract_board_repository_test.go
//
// Р›РµРЅРёРІР°СЏ РјР°С‚РµСЂРёР°Р»РёР·Р°С†РёСЏ РґРѕСЃРєРё РїР»Р°РЅРµС‚С‹ (СЃРїРµРєР°
// 2026-09-23-РєРѕРЅС‚СЂР°РєС‚-Р»РµРЅРёРІР°СЏ-РґРѕСЃРєР°-РїР°РєРµС‚-Рё-СЃРЅР°Р±Р¶РµРЅРёРµ В§3/В§4.4). РўРµСЃС‚С‹:
// СЃРїР°СЂСЃРµРЅРЅРѕСЃС‚СЊ (РЅРµС‚ РЅСѓР¶Рґ вЂ” РЅРµС‚ SQL), РєР°РґСЌРЅСЃ (РїРµСЂРІС‹Р№ РїСЂРѕРіРѕРЅ Р±РµР·СѓСЃР»РѕРІРµРЅ,
// РїРѕРІС‚РѕСЂРЅРѕРµ С‡С‚РµРЅРёРµ РІ РїСЂРµРґРµР»Р°С… РєР°РґСЌРЅСЃР° вЂ” no-op), СЃРІРµСЂРєР° (РїСѓР±Р»РёРєР°С†РёСЏ/РѕС‚РјРµРЅР°),
// РёРґРµРјРїРѕС‚РµРЅС‚РЅРѕСЃС‚СЊ, РѕС‚РєР°С‚ РїСЂРё РЅРµС…РІР°С‚РєРµ Р·Р°Р»РѕРіР°, РґРµС‚РµСЂРјРёРЅРёР·Рј, 409 РїР°РєРµС‚Р°.
package repository

import (
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"zorion/internal/contracts"
	"zorion/internal/economy/settlement"
)

// boardNeed вЂ” РЅСѓР¶РґР° РѕРґРЅРѕРіРѕ РїР°РєРµС‚Р° (Р°РІС‚РѕСЂ b1, РїР»Р°РЅРµС‚Р° p1, РїРѕР·РёС†РёСЏ g1).
func boardNeed(target, actual float64, capacities []float64) BoardNeed {
	return BoardNeed{
		AuthorID:     "b1",
		PlanetID:     "p1",
		GoodID:       "g1",
		Target:       target,
		Actual:       actual,
		Capacities:   capacities,
		BaseReward:   10,
		WindowPreset: time.Hour,
	}
}

// withNeeds вЂ” РёРЅСЉРµРєС†РёСЏ С€РІР° РЅСѓР¶Рґ РІ СЂРµРїРѕР·РёС‚РѕСЂРёР№ (unexported, С‚РµСЃС‚С‹ РІ РїР°РєРµС‚Рµ).
func withNeeds(repo *ContractRepository, needs ...BoardNeed) {
	repo.boardNeeds = func(planetID string) ([]BoardNeed, error) { return needs, nil }
}

func emptyOpenShares() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "package_key", "share_index", "quantity"})
}

// expectBoardStateNew вЂ” СЃС‚СЂРѕРєРё С‡РµРє-С‚РѕС‡РєРё РЅРµ Р±С‹Р»Рѕ (INSERT Р·Р°С‚СЂРѕРЅСѓР» 1 СЃС‚СЂРѕРєСѓ).
func expectBoardStateNew(mock sqlmock.Sqlmock, materializedAt time.Time) {
	mock.ExpectExec(`INSERT INTO contract_board_state`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT materialized_at FROM contract_board_state`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"materialized_at"}).AddRow(materializedAt))
}

// expectBoardStateExisting вЂ” СЃС‚СЂРѕРєР° Р±С‹Р»Р° (INSERT вЂ” 0 СЃС‚СЂРѕРє).
func expectBoardStateExisting(mock sqlmock.Sqlmock, materializedAt time.Time) {
	mock.ExpectExec(`INSERT INTO contract_board_state`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT materialized_at FROM contract_board_state`).
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"materialized_at"}).AddRow(materializedAt))
}

// expectPublishSupplyShare вЂ” С†РµРїРѕС‡РєР° РїСѓР±Р»РёРєР°С†РёРё РґРѕР»Рё: contracts (INSERT
// РєРѕРЅС‚СЂР°РєС‚Р°/С‚СЂРµР±РѕРІР°РЅРёСЏ) в†’ accounts (Р·Р°Р»РѕРі), РѕР±СЂР°С‚РЅС‹Р№ С…РµРЅРґР»РµСЂРЅРѕРјСѓ Publish
// РїРѕСЂСЏРґРѕРє (В§3.3).
func expectPublishSupplyShare(mock sqlmock.Sqlmock, reward, withdrawable int64) {
	expectPublishSupplyShareFor(mock, "b1", "g1", reward, withdrawable)
}

// expectPublishSupplyShareFor — то же для произвольного автора/позиции.
func expectPublishSupplyShareFor(mock sqlmock.Sqlmock, authorID, goodID string, reward, withdrawable int64) {
	mock.ExpectQuery(`SELECT owner_type, owner_id FROM buildings WHERE id = \$1`).
		WithArgs(authorID).
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id"}).AddRow("faction", "f1"))
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("faction", "f1", int64(1000000000000000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO contracts`).
		WithArgs(sqlmock.AnyArg(), "supply", "building", authorID, "p1", supplyShareTitle, "", "{}",
			reward, "regular", reward, int64(0), "deposit", "open", "public", nil, nil,
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_requirements`).
		WithArgs(sqlmock.AnyArg(), 1, "goods", goodID, "in", nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`WITH acc AS`).
		WithArgs("faction", "f1", reward).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "withdrawable", "least"}).
			AddRow(int64(1000000000000000), 0, withdrawable))
	mock.ExpectExec(`UPDATE contracts SET escrow_withdrawable`).
		WithArgs(sqlmock.AnyArg(), withdrawable).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // published
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_locked
	mock.ExpectExec(`INSERT INTO money_operations`).
		WithArgs("faction", "f1", -reward, sqlmock.AnyArg(), "escrow_lock", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// T8/T15: РїР»Р°РЅРµС‚Р° Р±РµР· РЅСѓР¶Рґ вЂ” РјР°С‚РµСЂРёР°Р»РёР·Р°С†РёСЏ РЅРµ РґРµР»Р°РµС‚ РќР РћР”РќРћР“Рћ SQL (РґРѕСЃРєР° РєР°Рє
// РїСЂРµР¶РґРµ, СЃС‚СЂРѕРєР° С‡РµРє-С‚РѕС‡РєРё РЅРµ СЂР°СЃС‚С‘С‚).
func TestMaterializeBoardNoNeedsNoSQL(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	ok, err := NewContractRepository(db).MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T15: СЃС‚СЂРѕРєРё С‡РµРє-С‚РѕС‡РєРё РЅРµС‚ в†’ РјР°С‚РµСЂРёР°Р»РёР·Р°С†РёСЏ РёРґС‘С‚ Р±РµР·СѓСЃР»РѕРІРЅРѕ (DEFAULT NOW() РµС‘
// РЅРµ Р±Р»РѕРєРёСЂСѓРµС‚), РЅРµСЃРјРѕС‚СЂСЏ РЅР° В«СЃРІРµР¶РёР№В» materialized_at.
func TestMaterializeBoardFirstRunIgnoresCadence(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 100, []float64{100})) // РґРµС„РёС†РёС‚ 0 в†’ РЅРёС‡РµРіРѕ РїСѓР±Р»РёРєРѕРІР°С‚СЊ

	mock.ExpectBegin()
	expectBoardStateNew(mock, time.Now()) // СЃС‚СЂРѕРєР° С‚РѕР»СЊРєРѕ С‡С‚Рѕ СЃРѕР·РґР°РЅР°, РЅРѕ РїСЂРѕРіРѕРЅ Р±РµР·СѓСЃР»РѕРІРµРЅ
	mock.ExpectQuery(`c.publication_planet_id = \$1`).WillReturnRows(emptyOpenShares())
	mock.ExpectExec(`UPDATE contract_board_state SET materialized_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.True(t, ok, "РїРµСЂРІР°СЏ РјР°С‚РµСЂРёР°Р»РёР·Р°С†РёСЏ Р±РµР·СѓСЃР»РѕРІРЅР°")
	require.NoError(t, mock.ExpectationsWereMet())
}

// T1/T2 (double-check): СЃС‚СЂРѕРєР° Р±С‹Р»Р° + РєР°РґСЌРЅСЃ РЅРµ РёСЃС‚С‘Рє в†’ РІС‹С…РѕРґ Р±РµР· Р·Р°РїРёСЃРё.
func TestMaterializeBoardSkipsWithinCadence(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 0, []float64{100}))

	mock.ExpectBegin()
	expectBoardStateExisting(mock, time.Now().Add(-time.Minute))
	mock.ExpectRollback()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.False(t, ok, "РІ РїСЂРµРґРµР»Р°С… РєР°РґСЌРЅСЃР° вЂ” РЅРѕР»СЊ Р·Р°РїРёСЃРµР№")
	require.NoError(t, mock.ExpectationsWereMet())
}

// T2: СЃС‚СЂРѕРєР° Р±С‹Р»Р° + РєР°РґСЌРЅСЃ РёСЃС‚С‘Рє в†’ РјР°С‚РµСЂРёР°Р»РёР·Р°С†РёСЏ РІС‹РїРѕР»РЅСЏРµС‚СЃСЏ.
func TestMaterializeBoardMaterializesWhenCadenceElapsed(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 100, []float64{100}))

	mock.ExpectBegin()
	expectBoardStateExisting(mock, time.Now().Add(-2*settlement.MinPersistInterval))
	mock.ExpectQuery(`c.publication_planet_id = \$1`).WillReturnRows(emptyOpenShares())
	mock.ExpectExec(`UPDATE contract_board_state SET materialized_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// РџСѓР±Р»РёРєР°С†РёСЏ РЅРµРґРѕСЃС‚Р°СЋС‰РµР№ РґРѕР»Рё (РґРµС„РёС†РёС‚ 100, С‚СЂСЋРј 100 в†’ РѕРґРЅР° РґРѕР»СЏ).
func TestMaterializeBoardPublishesMissingShares(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 0, []float64{100}))
	reward := contracts.ShareReward(100, 10)

	mock.ExpectBegin()
	expectBoardStateNew(mock, time.Now())
	mock.ExpectQuery(`c.publication_planet_id = \$1`).WillReturnRows(emptyOpenShares())
	expectPublishSupplyShare(mock, reward, 0)
	mock.ExpectExec(`UPDATE contract_board_state SET materialized_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T4: РІР·СЏС‚С‹Рµ РґРѕР»Рё СЃРІРµСЂРєР° РЅРµ С‚СЂРѕРіР°РµС‚ вЂ” SQL РѕС‚РєСЂС‹С‚С‹С… РґРѕР»РµР№ С„РёР»СЊС‚СЂСѓРµС‚ status='open',
// РїРѕСЌС‚РѕРјСѓ РІР·СЏС‚Р°СЏ РґРѕР»СЏ РІ РІС‹Р±РѕСЂРєСѓ РЅРµ РїРѕРїР°РґР°РµС‚ Рё РѕС‚РјРµРЅС‹ РЅРµ РїСЂРѕРёСЃС…РѕРґРёС‚.
func TestMaterializeBoardIgnoresTakenShares(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 0, []float64{100}))
	reward := contracts.ShareReward(100, 10)

	mock.ExpectBegin()
	expectBoardStateNew(mock, time.Now())
	// РћС‚РєСЂС‹С‚С‹С… РґРѕР»РµР№ РЅРµС‚ (РІР·СЏС‚Р°СЏ РґРѕР»СЏ вЂ” status='taken', РІ РІС‹Р±РѕСЂРєСѓ РЅРµ РІС…РѕРґРёС‚).
	mock.ExpectQuery(`WHERE c.status = 'open' AND c.publication_planet_id = \$1`).WillReturnRows(emptyOpenShares())
	expectPublishSupplyShare(mock, reward, 0)
	mock.ExpectExec(`UPDATE contract_board_state SET materialized_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T7: РїРѕРІС‚РѕСЂРЅР°СЏ РјР°С‚РµСЂРёР°Р»РёР·Р°С†РёСЏ СЃ СЃРѕРІРїР°РґР°СЋС‰РёРј РЅР°Р±РѕСЂРѕРј РґРѕР»РµР№ вЂ” no-op РїРѕ contracts
// (РЅРё INSERT, РЅРё РѕС‚РјРµРЅС‹; expires_at РѕС‚РєСЂС‹С‚РѕР№ РґРѕР»Рё РЅРµ С‚СЂРѕРіР°РµС‚СЃСЏ).
func TestMaterializeBoardNoopWhenSharesMatch(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 0, []float64{100}))

	mock.ExpectBegin()
	expectBoardStateExisting(mock, time.Now().Add(-2*settlement.MinPersistInterval))
	mock.ExpectQuery(`c.publication_planet_id = \$1`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "package_key", "share_index", "quantity"}).
			AddRow("c1", "supply:p1:b1:g1", 1, int64(100)))
	mock.ExpectExec(`UPDATE contract_board_state SET materialized_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T5: РґРµС„РёС†РёС‚ СѓРїР°Р» РґРѕ РЅСѓР»СЏ в†’ РїР°РєРµС‚ СЃС…Р»РѕРїС‹РІР°РµС‚СЃСЏ: РѕС‚РєСЂС‹С‚Р°СЏ РґРѕР»СЏ РѕС‚РјРµРЅРµРЅР°, Р·Р°Р»РѕРі
// РІРѕР·РІСЂР°С‰С‘РЅ Р°РІС‚РѕСЂСѓ-РїРѕСЃС‚СЂРѕР№РєРµ (contracts в†’ accounts), РїСЂРёС‡РёРЅР° 'superseded'.
func TestMaterializeBoardCancelsWhenDeficitGone(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 100, []float64{100})) // РґРµС„РёС†РёС‚ 0

	mock.ExpectBegin()
	expectBoardStateExisting(mock, time.Now().Add(-2*settlement.MinPersistInterval))
	mock.ExpectQuery(`c.publication_planet_id = \$1`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "package_key", "share_index", "quantity"}).
			AddRow("c1", "supply:p1:b1:g1", 1, int64(100)))
	mock.ExpectQuery(`UPDATE contracts SET status = 'cancelled'`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rowSet6().AddRow("c1", "building", "b1", nil, 500, 0))
	mock.ExpectQuery(`SELECT owner_type, owner_id FROM buildings WHERE id = \$1`).
		WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id"}).AddRow("faction", "f1"))
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("faction", "f1", int64(1000000000000000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3, withdrawable = withdrawable \+ \$4`).
		WithArgs("faction", "f1", int64(500), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(int64(1000000000000500)))
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // cancelled
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_returned
	mock.ExpectExec(`UPDATE contract_board_state SET materialized_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// РќРµС…РІР°С‚РєР° СЃСЂРµРґСЃС‚РІ РїСЂРё СЃРїРёСЃР°РЅРёРё Р·Р°Р»РѕРіР° (0 СЃС‚СЂРѕРє) в†’ РѕС‚РєР°С‚ РІСЃРµР№ tx (В§3.3).
func TestMaterializeBoardRollsBackWhenEscrowFails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 0, []float64{100}))
	reward := contracts.ShareReward(100, 10)

	mock.ExpectBegin()
	expectBoardStateNew(mock, time.Now())
	mock.ExpectQuery(`c.publication_planet_id = \$1`).WillReturnRows(emptyOpenShares())
	mock.ExpectQuery(`SELECT owner_type, owner_id FROM buildings WHERE id = \$1`).
		WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id"}).AddRow("faction", "f1"))
	mock.ExpectExec(`INSERT INTO accounts`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO contracts`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_requirements`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`WITH acc AS`).
		WithArgs("faction", "f1", reward).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "withdrawable", "least"}))
	mock.ExpectRollback()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.ErrorIs(t, err, ErrInsufficientFunds)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T3: С†РµР»РµРІРѕР№ РЅР°Р±РѕСЂ Рё РЅР°РіСЂР°РґС‹ РґРµС‚РµСЂРјРёРЅРёСЂРѕРІР°РЅС‹ (С‡РёСЃС‚С‹Рµ С„СѓРЅРєС†РёРё, RNG РѕС‚СЃСѓС‚СЃС‚РІСѓРµС‚).
func TestTargetSharesAndRewardDeterministic(t *testing.T) {
	need := boardNeed(250, 0, []float64{20, 100})
	first, second := targetShares(need), targetShares(need)
	require.True(t, reflect.DeepEqual(first, second), "РЅР°СЂРµР·РєР° РЅРµРґРµС‚РµСЂРјРёРЅРёСЂРѕРІР°РЅР°: %+v != %+v", first, second)
	// РћР¶РёРґР°РµРјР°СЏ РЅР°СЂРµР·РєР° РїСЂРё 250 Рё РІРјРµСЃС‚РёРјРѕСЃС‚СЏС… 20/100: 100/100/50 (ОЈ = вЊЉРґРµС„РёС†РёС‚вЊ‹).
	require.Equal(t, []contracts.Share{
		{Index: 1, Quantity: 100, Capacity: 100},
		{Index: 2, Quantity: 100, Capacity: 100},
		{Index: 3, Quantity: 50, Capacity: 100},
	}, first)
}

// T5/T7 (Р»РѕРіРёРєР° СЃРІРµСЂРєРё): СЃРѕРІРїР°РґРµРЅРёРµ вЂ” no-op; РґСЂСѓРіРѕР№ РѕР±СЉС‘Рј вЂ” СЃРЅСЏС‚СЊ Рё РїРµСЂРµРёР·РґР°С‚СЊ;
// Р»РёС€РЅРёР№ РёРЅРґРµРєСЃ вЂ” СЃРЅСЏС‚СЊ; РїСѓСЃС‚РѕР№ С†РµР»РµРІРѕР№ РЅР°Р±РѕСЂ вЂ” СЃРЅСЏС‚СЊ РІСЃС‘.
// F1: пакет выпал из источника нужд, но у планеты есть его открытые доли →
// целевой набор пуст → все доли пакета снимаются (залог возвращён, superseded).
// Запрос открытых долей — по планете (не по ключам нужд), поэтому выпавший
// пакет виден сверке.
func TestMaterializeBoardCancelsPackageDroppedFromNeeds(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	// Нужда есть, но у ДРУГОГО пакета (b2/g2) — пакет b1/g1 выпал.
	other := boardNeed(100, 0, []float64{100})
	other.AuthorID = "b2"
	other.GoodID = "g2"
	withNeeds(repo, other)

	mock.ExpectBegin()
	expectBoardStateNew(mock, time.Now())
	// Открытая доля выпавшего пакета видна (выборка по планете).
	mock.ExpectQuery(`c.publication_planet_id = \$1`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "package_key", "share_index", "quantity"}).
			AddRow("c1", "supply:p1:b1:g1", 1, int64(100)))
	// Снятие: contracts → accounts, причина superseded.
	mock.ExpectQuery(`UPDATE contracts SET status = 'cancelled'`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rowSet6().AddRow("c1", "building", "b1", nil, 500, 0))
	mock.ExpectQuery(`SELECT owner_type, owner_id FROM buildings WHERE id = \$1`).
		WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id"}).AddRow("faction", "f1"))
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("faction", "f1", int64(1000000000000000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3, withdrawable = withdrawable \+ \$4`).
		WithArgs("faction", "f1", int64(500), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(int64(1000000000000500)))
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // cancelled
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_returned
	// Публикация доли второго пакета (b2/g2).
	expectPublishSupplyShareFor(mock, "b2", "g2", contracts.ShareReward(100, 10), 0)
	mock.ExpectExec(`UPDATE contract_board_state SET materialized_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// F1: контракты вне пакета (package_key IS NULL — перелёты, ручные публикации)
// в сверку не входят: фильтр запроса их отсекает, отмены не происходит.
func TestMaterializeBoardIgnoresUnpackagedContracts(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 100, []float64{100})) // дефицит 0 → ничего не публикуем

	mock.ExpectBegin()
	expectBoardStateNew(mock, time.Now())
	// Выборка отсекает package_key IS NULL — «ручной» контракт в неё не попадает.
	mock.ExpectQuery(`WHERE c.status = 'open' AND c.publication_planet_id = \$1\s+AND c.package_key IS NOT NULL`).
		WillReturnRows(emptyOpenShares())
	mock.ExpectExec(`UPDATE contract_board_state SET materialized_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// F2: дубликаты нужд по ключу пакета сворачиваются (первая запись побеждает) —
// один пакет, один комплект долей, без двойной публикации и отката.
func TestBuildBoardPackagesDeduplicatesNeeds(t *testing.T) {
	first := boardNeed(100, 0, []float64{100})
	dup := boardNeed(100, 0, []float64{100}) // тот же ключ supply:p1:b1:g1
	other := boardNeed(50, 0, []float64{100})
	other.GoodID = "g2"

	pkgs := buildBoardPackages([]BoardNeed{first, dup, other})
	require.Len(t, pkgs, 2, "дубликат нужды не должен давать второй пакет")
	require.Equal(t, "supply:p1:b1:g1", pkgs[0].key)
	require.Equal(t, "supply:p1:b1:g2", pkgs[1].key)
	require.Equal(t, []contracts.Share{{Index: 1, Quantity: 100, Capacity: 100}}, pkgs[0].shares)
}

// F2 (сквозной): две одинаковые нужды → ровно один комплект долей, один INSERT
// контракта (без нарушения уникального индекса и отката).
func TestMaterializeBoardDeduplicatesNeeds(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewContractRepository(db)
	withNeeds(repo, boardNeed(100, 0, []float64{100}), boardNeed(100, 0, []float64{100}))
	reward := contracts.ShareReward(100, 10)

	mock.ExpectBegin()
	expectBoardStateNew(mock, time.Now())
	mock.ExpectQuery(`c.publication_planet_id = \$1`).WillReturnRows(emptyOpenShares())
	expectPublishSupplyShare(mock, reward, 0) // ровно одна публикация
	mock.ExpectExec(`UPDATE contract_board_state SET materialized_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := repo.MaterializeBoard("p1", time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// F1 (логика обхода): ключи сверки — объединение целевых и найденных, отсортированы.
func TestReconcileKeys(t *testing.T) {
	pkgs := []boardPackage{{key: "supply:p1:b2:g2"}, {key: "supply:p1:b1:g1"}}
	current := map[string]map[int]openShare{
		"supply:p1:b9:g9": {1: {id: "c9", index: 1, qty: 10}},
		"supply:p1:b1:g1": {1: {id: "c1", index: 1, qty: 10}},
	}
	require.Equal(t, []string{
		"supply:p1:b1:g1", "supply:p1:b2:g2", "supply:p1:b9:g9",
	}, reconcileKeys(pkgs, current))
}

func TestReconcileShares(t *testing.T) {
	t.Run("СЃРѕРІРїР°Р»Рѕ вЂ” no-op", func(t *testing.T) {
		cancel, publish := reconcileShares(
			[]contracts.Share{{Index: 1, Quantity: 100}},
			map[int]openShare{1: {id: "c1", index: 1, qty: 100}})
		require.Empty(t, cancel)
		require.Empty(t, publish)
	})

	t.Run("РѕР±СЉС‘Рј РёР·РјРµРЅРёР»СЃСЏ вЂ” СЃРЅСЏС‚СЊ Рё РїРµСЂРµРёР·РґР°С‚СЊ", func(t *testing.T) {
		cancel, publish := reconcileShares(
			[]contracts.Share{{Index: 1, Quantity: 100}},
			map[int]openShare{1: {id: "c1", index: 1, qty: 80}})
		require.Equal(t, []string{"c1"}, cancel)
		require.Equal(t, []contracts.Share{{Index: 1, Quantity: 100}}, publish)
	})

	t.Run("РЅРµРґРѕСЃС‚Р°СЋС‰РёРµ Рё Р»РёС€РЅРёРµ РёРЅРґРµРєСЃС‹", func(t *testing.T) {
		cancel, publish := reconcileShares(
			[]contracts.Share{{Index: 1, Quantity: 100}, {Index: 2, Quantity: 50}},
			map[int]openShare{1: {id: "c1", index: 1, qty: 100}, 3: {id: "c3", index: 3, qty: 70}})
		require.Equal(t, []string{"c3"}, cancel)
		require.Equal(t, []contracts.Share{{Index: 2, Quantity: 50}}, publish)
	})

	t.Run("РЅСѓР»РµРІРѕР№ РґРµС„РёС†РёС‚ вЂ” СЃРЅСЏС‚СЊ РІСЃРµ", func(t *testing.T) {
		cancel, publish := reconcileShares(nil,
			map[int]openShare{1: {id: "c1", index: 1, qty: 10}, 2: {id: "c2", index: 2, qty: 20}})
		require.Equal(t, []string{"c1", "c2"}, cancel)
		require.Empty(t, publish)
	})
}

// T6: РІС‚РѕСЂРѕР№ Take РґРѕР»Рё С‚РѕРіРѕ Р¶Рµ РїР°РєРµС‚Р° С‚РµРј Р¶Рµ РёРіСЂРѕРєРѕРј вЂ” РЅР°СЂСѓС€РµРЅРёРµ СѓРЅРёРєР°Р»СЊРЅРѕСЃС‚Рё
// uq_contracts_package_taken_executor в†’ ErrPackageShareTaken (РЅРµ РіРѕРЅРєР°).
func TestTakePackageShareTaken(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE contracts\s+SET status = 'taken'`).
		WithArgs("c1", "player", "u1", sqlmock.AnyArg(), nil).
		WillReturnError(&pq.Error{
			Code:       "23505",
			Constraint: "uq_contracts_package_taken_executor",
			Message:    "duplicate key value violates unique constraint",
		})
	mock.ExpectRollback()

	ok, err := NewContractRepository(db).Take("c1", "player", "u1", nil)
	require.False(t, ok)
	require.ErrorIs(t, err, ErrPackageShareTaken)
	require.NoError(t, mock.ExpectationsWereMet())
}

// РћР±С‹С‡РЅР°СЏ РѕС€РёР±РєР° Р‘Р” РїСЂРё РІР·СЏС‚РёРё вЂ” РќР• РїРѕРґРјРµРЅСЏРµС‚СЃСЏ РЅР° ErrPackageShareTaken.
func TestTakeOtherErrorNotMapped(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE contracts\s+SET status = 'taken'`).
		WithArgs("c1", "player", "u1", sqlmock.AnyArg(), nil).
		WillReturnError(&pq.Error{Code: "23503", Constraint: "other"})
	mock.ExpectRollback()

	_, err = NewContractRepository(db).Take("c1", "player", "u1", nil)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrPackageShareTaken)
	require.NoError(t, mock.ExpectationsWereMet())
}
