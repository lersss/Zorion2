// internal/repository/contract_repository_test.go
//
// Контракт: эскроу и атомарные переходы (спеки 2026-09-22-контракт-модель-сущности
// §4–§6, 2026-09-22-деньги-и-эскроу §3–§4). Тесты: запирание залога при публикации
// (lock + escrow_withdrawable), атомарный flip взятия, возврат залога при отмене,
// ленивое истечение (ExpireDue), возврат при удалении (ReturnEscrowForContractsTx),
// резолв плательщика постройки/агента (§4).
package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

const contractSelectRe = `SELECT id, type, author_type, author_id, publication_planet_id`

// contractRow — строка contracts для sqlmock (порядок contractColumns).
func contractRow(id, authorType, authorID string, escrowAmount, escrowWithdrawable int64) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "type", "author_type", "author_id", "publication_planet_id", "title", "description",
		"payload", "reward", "funding", "escrow_amount", "escrow_withdrawable", "escrow_kind",
		"status", "visibility", "direct_target_type", "direct_target_id", "executor_type",
		"executor_id", "taken_at", "expires_at", "created_at", "updated_at",
	}).AddRow(id, "travel", authorType, authorID, "planet-1", "T", "", []byte("{}"),
		escrowAmount, "regular", escrowAmount, escrowWithdrawable, "deposit",
		"open", "public", nil, nil, nil, nil, nil, now, now, now)
}

func emptyRequirementsRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "contract_id", "pos", "kind", "subject", "op",
		"threshold_num", "threshold_text", "quantity"})
}

func rowSet6() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "author_type", "author_id", "executor_id",
		"escrow_amount", "escrow_withdrawable"})
}

const escrowSweepRe = `UPDATE contracts\s+SET status = CASE WHEN executor_id IS NULL`

// Публикация: залог списывается одним оператором, escrow_withdrawable берётся из
// CTE (LEAST(withdrawable, amount)), пишутся contract_log и money_operations.
func TestContractPublishLocksEscrow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	const authorID, planetID = "u1", "p1"

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("player", authorID, int64(10000)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Залог 500, выводимая доля 50 (из CTE).
	mock.ExpectQuery(`WITH acc AS`).
		WithArgs("player", authorID, int64(500)).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "withdrawable", "least"}).AddRow(9500, 0, 50))
	mock.ExpectExec(`INSERT INTO contracts`).
		WithArgs(sqlmock.AnyArg(), "travel", "player", authorID, planetID, "T", "",
			sqlmock.AnyArg(), int64(500), "regular", int64(500), int64(50),
			"deposit", "open", "public", nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // published
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_locked
	mock.ExpectExec(`INSERT INTO money_operations`).
		WithArgs("player", authorID, int64(-500), int64(9500), "escrow_lock", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// Publish в конце читает контракт обратно (GetByID).
	mock.ExpectQuery(contractSelectRe).
		WillReturnRows(contractRow("c1", "player", authorID, 500, 50))
	mock.ExpectQuery(`SELECT id, contract_id, pos, kind, subject, op,`).WillReturnRows(emptyRequirementsRows())

	repo := NewContractRepository(db)
	c, err := repo.Publish(PublishContractParams{
		Type: "travel", AuthorType: "player", AuthorID: authorID,
		PublicationPlanetID: planetID, Title: "T", Reward: 500,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, int64(500), c.EscrowAmount)
	require.Equal(t, int64(50), c.EscrowWithdrawable)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ноль строк при запирании (нехватка средств) → ErrInsufficientFunds, откат.
func TestContractPublishInsufficientFunds(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO accounts`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`WITH acc AS`).
		WillReturnRows(sqlmock.NewRows([]string{"balance", "withdrawable", "least"}))
	mock.ExpectRollback()

	repo := NewContractRepository(db)
	_, err = repo.Publish(PublishContractParams{
		Type: "travel", AuthorType: "player", AuthorID: "u1",
		PublicationPlanetID: "p1", Title: "T", Reward: 500,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.ErrorIs(t, err, ErrInsufficientFunds)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Взятие: один условный UPDATE с проверкой старого статуса.
func TestContractTakeAtomic(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE contracts\s+SET status = 'taken'`).
		WithArgs("c1", "player", "u1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := NewContractRepository(db).Take("c1", "player", "u1")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Взятие гонкой: 0 строк → false, откат (контракт уже взят/истёк).
func TestContractTakeConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE contracts\s+SET status = 'taken'`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	ok, err := NewContractRepository(db).Take("c1", "player", "u1")
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Отмена: flip open→cancelled только для автора + возврат залога (баланс и
// выводимая доля восстанавливаются) + журнал.
func TestContractCancelReturnsEscrow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE contracts\s+SET status = 'cancelled'`).
		WithArgs("c1", "player", "u1").
		WillReturnRows(rowSet6().AddRow("c1", "player", "u1", nil, 500, 50))
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("player", "u1", int64(10000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3, withdrawable = withdrawable \+ \$4`).
		WithArgs("player", "u1", int64(500), int64(50)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(1500))
	mock.ExpectExec(`INSERT INTO money_operations`).
		WithArgs("player", "u1", int64(500), int64(1500), "escrow_return", "c1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // cancelled
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_returned
	mock.ExpectCommit()

	ok, err := NewContractRepository(db).Cancel("c1", "player", "u1")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ленивое истечение: взятый контракт → статус expired, залог возвращается
// автору, лог failed (executor_id непуст).
func TestContractExpireDueReturnsEscrow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE contracts\s+SET status = 'expired'`).
		WillReturnRows(rowSet6().AddRow("c1", "faction", "f1", "u9", 700, 0))
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("faction", "f1", int64(1000000000000000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3`).
		WithArgs("faction", "f1", int64(700), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(1000000000000700))
	mock.ExpectExec(`INSERT INTO money_operations`).
		WithArgs("faction", "f1", int64(700), sqlmock.AnyArg(), "escrow_return", "c1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // failed
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_returned
	mock.ExpectCommit()

	n, err := NewContractRepository(db).ExpireDue(ContractScope{PlanetID: "p1"})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Возврат при удалении (§6.5): атомарный статусный flip в уже открытой tx;
// open → cancelled, лог cancelled (executor_id пуст).
func TestReturnEscrowForContractsTxFlip(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(escrowSweepRe).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rowSet6().AddRow("c1", "player", "u1", nil, 300, 0))
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("player", "u1", int64(10000)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3`).
		WithArgs("player", "u1", int64(300), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(1300))
	mock.ExpectExec(`INSERT INTO money_operations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // cancelled
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_returned
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	n, err := ReturnEscrowForContractsTx(tx, ContractScope{WorldIDs: []string{"w1"}}, models.EscrowReasonWorldDeleted)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

// Выполнение: flip taken→completed + выпуск залога исполнителю. Обычный
// контракт (funding=regular) — зачисляется только в balance, withdrawable = 0.
func TestContractCompleteReleasesEscrow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)UPDATE contracts\s+SET status = 'completed'.*expires_at > NOW\(\)`).
		WithArgs("c1", "u1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "author_type", "author_id", "executor_type", "executor_id",
			"escrow_amount", "escrow_withdrawable", "funding",
		}).AddRow("c1", "player", "u2", "player", "u1", 500, 0, "regular"))
	mock.ExpectExec(`INSERT INTO accounts`).
		WithArgs("player", "u1", int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`UPDATE accounts\s+SET balance = balance \+ \$3`).
		WithArgs("player", "u1", int64(500), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(500))
	mock.ExpectExec(`INSERT INTO money_operations`).
		WithArgs("player", "u1", int64(500), int64(500), "escrow_release", "c1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // completed
	mock.ExpectExec(`INSERT INTO contract_log`).WillReturnResult(sqlmock.NewResult(0, 1)) // escrow_released
	mock.ExpectCommit()

	ok, err := NewContractRepository(db).Complete("c1", "u1")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Просроченный взятый контракт НЕ завершается: условие expires_at > NOW() в том
// же атомарном UPDATE даёт 0 строк → false, откат (залог вернёт ExpireDue).
func TestContractCompleteExpiredNotCompleted(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)UPDATE contracts\s+SET status = 'completed'.*expires_at > NOW\(\)`).
		WithArgs("c1", "u1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "author_type", "author_id", "executor_type", "executor_id",
			"escrow_amount", "escrow_withdrawable", "funding",
		}))
	mock.ExpectRollback()

	ok, err := NewContractRepository(db).Complete("c1", "u1")
	require.NoError(t, err)
	require.False(t, ok, "просроченный taken не завершается (0 строк)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Резолв плательщика-постройки (§4): owner_type/owner_id постройки → счёт владельца.
func TestResolvePayerBuilding(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT owner_type, owner_id FROM buildings WHERE id = \$1`).
		WithArgs("b1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_type", "owner_id"}).AddRow("faction", "f1"))

	ownerType, ownerID, err := resolvePayerAccountQ(db, "building", "b1")
	require.NoError(t, err)
	require.Equal(t, "faction", ownerType)
	require.Equal(t, "f1", ownerID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Резолв плательщика-агента (§6): owner_faction_id → счёт фракции.
func TestResolvePayerAgent(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT owner_faction_id FROM npc_agents WHERE id = \$1`).
		WithArgs("a1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_faction_id"}).AddRow("f1"))

	ownerType, ownerID, err := resolvePayerAccountQ(db, "agent", "a1")
	require.NoError(t, err)
	require.Equal(t, "faction", ownerType)
	require.Equal(t, "f1", ownerID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Место публикации по автору (спека перелёта §3): фракция → homeworld_id,
// постройка → planet_id, агент → ошибка (нет планетного слоя). Автор-player
// резолвится вызывающим по позиции игрока (тест — на слое хендлеров).
func TestResolvePublicationPlanet(t *testing.T) {
	t.Run("фракция — homeworld_id", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectQuery(`SELECT homeworld_id FROM factions WHERE id = \$1`).
			WithArgs("f1").
			WillReturnRows(sqlmock.NewRows([]string{"homeworld_id"}).AddRow("p1"))
		got, err := NewContractRepository(db).ResolvePublicationPlanet("faction", "f1")
		require.NoError(t, err)
		require.Equal(t, "p1", got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("постройка — planet_id", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectQuery(`SELECT planet_id FROM buildings WHERE id = \$1`).
			WithArgs("b1").
			WillReturnRows(sqlmock.NewRows([]string{"planet_id"}).AddRow("p2"))
		got, err := NewContractRepository(db).ResolvePublicationPlanet("building", "b1")
		require.NoError(t, err)
		require.Equal(t, "p2", got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("агент — не поддержан", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer db.Close()
		_, err = NewContractRepository(db).ResolvePublicationPlanet("agent", "a1")
		require.ErrorIs(t, err, ErrPublicationPlanetUnresolved)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
