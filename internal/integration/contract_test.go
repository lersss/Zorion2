package integration

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// itReward — цена тестового контракта (больше нуля: CHECK contracts_live_has_escrow).
const itReward = 300

// seedWorldPlanet — минимальные world+planet: FK contracts.publication_planet_id.
func seedWorldPlanet(t *testing.T, db *sql.DB) string {
	t.Helper()
	worldID := uuid.New().String()
	planetID := uuid.New().String()
	if _, err := db.Exec(
		`INSERT INTO worlds (id, name, coord_x, coord_y) VALUES ($1, 'IT', 0, 0)`, worldID,
	); err != nil {
		t.Fatalf("insert world: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO planets (id, world_id, name, orbit_index) VALUES ($1, $2, 'IT', 1)`,
		planetID, worldID,
	); err != nil {
		t.Fatalf("insert planet: %v", err)
	}
	return planetID
}

// publishOpenContract — публикация открытого контракта игрока-автора.
func publishOpenContract(t *testing.T, repo *repository.ContractRepository, planetID, authorID string, expiresAt time.Time) *models.Contract {
	t.Helper()
	c, err := repo.Publish(repository.PublishContractParams{
		Type:                "delivery",
		AuthorType:          models.ContractActorPlayer,
		AuthorID:            authorID,
		PublicationPlanetID: planetID,
		Title:               "IT contract",
		Reward:              itReward,
		ExpiresAt:           expiresAt,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return c
}

// contractStatus — статус контракта из БД (истина из таблицы, не из модели).
func contractStatus(t *testing.T, db *sql.DB, contractID string) string {
	t.Helper()
	return mustStr(t, db, `SELECT status FROM contracts WHERE id = $1`, contractID)
}

// accountBalance — balance/withdrawable счёта (нет счёта — фатально).
func accountBalance(t *testing.T, db *sql.DB, ownerType, ownerID string) (int64, int64) {
	t.Helper()
	var balance, withdrawable int64
	err := db.QueryRow(`SELECT balance, withdrawable FROM accounts
		WHERE owner_type = $1 AND owner_id = $2`,
		ownerType, ownerID).Scan(&balance, &withdrawable)
	if err != nil {
		t.Fatalf("account %s/%s: %v", ownerType, ownerID, err)
	}
	return balance, withdrawable
}

// logTypes — журнал жизни контракта: тип записи → число записей.
func logTypes(t *testing.T, db *sql.DB, contractID string) map[string]int64 {
	t.Helper()
	rows, err := db.Query(`SELECT type, COUNT(*) FROM contract_log
		WHERE contract_id = $1 GROUP BY type`, contractID)
	if err != nil {
		t.Fatalf("contract_log: %v", err)
	}
	defer rows.Close()
	got := map[string]int64{}
	for rows.Next() {
		var kind string
		var n int64
		if err := rows.Scan(&kind, &n); err != nil {
			t.Fatalf("scan contract_log: %v", err)
		}
		got[kind] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("contract_log rows: %v", err)
	}
	return got
}

// moneyOp — движения по счёту одного вида: (число записей, сумма delta,
// balance_after последней записи).
func moneyOp(t *testing.T, db *sql.DB, ownerType, ownerID, kind string) (int64, int64, int64) {
	t.Helper()
	var n, delta int64
	var after sql.NullInt64
	err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(delta), 0), MAX(balance_after)
		FROM money_operations
		WHERE owner_type = $1 AND owner_id = $2 AND kind = $3`,
		ownerType, ownerID, kind).Scan(&n, &delta, &after)
	if err != nil {
		t.Fatalf("money_operations %s: %v", kind, err)
	}
	return n, delta, after.Int64
}

// TestContractEscrowLifecycle — публикация (залог заперт) → взятие → закрытие
// (залог освобождён исполнителю); балансы, статусы, contract_log, money_operations.
func TestContractEscrowLifecycle(t *testing.T) {
	s := openMigrated(t)
	planetID := seedWorldPlanet(t, s.db)
	repo := repository.NewContractRepository(s.db)

	authorID := uuid.New().String()
	executorID := uuid.New().String()
	contract := publishOpenContract(t, repo, planetID, authorID, time.Now().Add(time.Hour))

	// Публикация: залог заперт со счёта автора, операция записана.
	if contract.Status != models.ContractStatusOpen || contract.EscrowAmount != itReward {
		t.Fatalf("после Publish: status=%q escrow=%d", contract.Status, contract.EscrowAmount)
	}
	authorBalance, authorWithdrawable := accountBalance(t, s.db, models.AccountOwnerPlayer, authorID)
	if authorBalance != models.PlayerBalanceSeed-itReward || authorWithdrawable != 0 {
		t.Fatalf("счёт автора после Publish: balance=%d withdrawable=%d",
			authorBalance, authorWithdrawable)
	}
	logs := logTypes(t, s.db, contract.ID)
	if logs[models.ContractLogPublished] != 1 || logs[models.ContractLogEscrowLocked] != 1 {
		t.Fatalf("contract_log после Publish: %v", logs)
	}
	n, delta, after := moneyOp(t, s.db, models.AccountOwnerPlayer, authorID, models.MoneyOpEscrowLock)
	if n != 1 || delta != -itReward || after != authorBalance {
		t.Fatalf("escrow_lock: n=%d delta=%d balance_after=%d (ожидалось 1/%d/%d)",
			n, delta, after, -itReward, authorBalance)
	}

	// Взятие: атомарный flip open→taken + запись в журнал.
	ok, err := repo.Take(contract.ID, models.ContractExecutorPlayer, executorID, nil)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if !ok {
		t.Fatal("Take вернул false на открытом контракте")
	}
	if got := contractStatus(t, s.db, contract.ID); got != models.ContractStatusTaken {
		t.Fatalf("статус после Take: %q, ожидался %q", got, models.ContractStatusTaken)
	}
	if got := mustStr(t, s.db, `SELECT executor_id::text FROM contracts WHERE id = $1`, contract.ID); got != executorID {
		t.Fatalf("executor_id: %q, ожидался %q", got, executorID)
	}
	if logs = logTypes(t, s.db, contract.ID); logs[models.ContractLogTaken] != 1 {
		t.Fatalf("contract_log после Take: %v", logs)
	}

	// Закрытие: flip taken→completed + залог освобождён исполнителю.
	ok, err = repo.Complete(contract.ID, executorID)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !ok {
		t.Fatal("Complete вернул false на взятом контракте")
	}
	if got := contractStatus(t, s.db, contract.ID); got != models.ContractStatusCompleted {
		t.Fatalf("статус после Complete: %q, ожидался %q", got, models.ContractStatusCompleted)
	}
	executorBalance, _ := accountBalance(t, s.db, models.AccountOwnerPlayer, executorID)
	if executorBalance != itReward {
		t.Fatalf("счёт исполнителя после Complete: %d, ожидался %d", executorBalance, itReward)
	}
	if authorBalance, _ = accountBalance(t, s.db, models.AccountOwnerPlayer, authorID); authorBalance != models.PlayerBalanceSeed-itReward {
		t.Fatalf("счёт автора после Complete: %d (залог уже выпущен)", authorBalance)
	}
	logs = logTypes(t, s.db, contract.ID)
	if logs[models.ContractLogCompleted] != 1 || logs[models.ContractLogEscrowReleased] != 1 {
		t.Fatalf("contract_log после Complete: %v", logs)
	}
	n, delta, after = moneyOp(t, s.db, models.AccountOwnerPlayer, executorID, models.MoneyOpEscrowRelease)
	if n != 1 || delta != itReward || after != itReward {
		t.Fatalf("escrow_release: n=%d delta=%d balance_after=%d (ожидалось 1/%d/%d)",
			n, delta, after, itReward, itReward)
	}
}

// TestContractTakeRace — два параллельных взятия одного контракта: атомарный
// flip open→taken обязан пропустить ровно одного (мультиплеер, AGENTS.md §0).
func TestContractTakeRace(t *testing.T) {
	s := openMigrated(t)
	planetID := seedWorldPlanet(t, s.db)
	repo := repository.NewContractRepository(s.db)

	authorID := uuid.New().String()
	contract := publishOpenContract(t, repo, planetID, authorID, time.Now().Add(time.Hour))

	executors := []string{uuid.New().String(), uuid.New().String()}
	oks := make([]bool, len(executors))
	errs := make([]error, len(executors))
	var wg sync.WaitGroup
	for i := range executors {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			oks[i], errs[i] = repo.Take(contract.ID, models.ContractExecutorPlayer, executors[i], nil)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Take[%d]: %v", i, err)
		}
	}
	successes := 0
	for _, ok := range oks {
		if ok {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("успешных взятий %d, ожидалось ровно 1 (результаты: %v)", successes, oks)
	}
	if got := contractStatus(t, s.db, contract.ID); got != models.ContractStatusTaken {
		t.Fatalf("статус после гонки: %q, ожидался %q", got, models.ContractStatusTaken)
	}
	if logs := logTypes(t, s.db, contract.ID); logs[models.ContractLogTaken] != 1 {
		t.Fatalf("записей taken в журнале %d, ожидалась 1: %v",
			logs[models.ContractLogTaken], logs)
	}
}

// TestContractExpireAfterTake — взятый контракт с истёкшим сроком: ленивое
// истечение (лог failed, залог возвращён автору).
func TestContractExpireAfterTake(t *testing.T) {
	s := openMigrated(t)
	planetID := seedWorldPlanet(t, s.db)
	repo := repository.NewContractRepository(s.db)

	authorID := uuid.New().String()
	executorID := uuid.New().String()
	contract := publishOpenContract(t, repo, planetID, authorID, time.Now().Add(time.Hour))

	// Взятие перебазирует срок в прошлое (путь «взял перед истечением»):
	// WHERE пропускает по старому сроку, в строке остаётся истёкший.
	past := time.Now().Add(-time.Minute)
	ok, err := repo.Take(contract.ID, models.ContractExecutorPlayer, executorID, &past)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if !ok {
		t.Fatal("Take вернул false")
	}

	n, err := repo.ExpireDue(repository.ContractScope{ContractID: contract.ID})
	if err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}
	if n != 1 {
		t.Fatalf("истекло контрактов %d, ожидался 1", n)
	}
	if got := contractStatus(t, s.db, contract.ID); got != models.ContractStatusExpired {
		t.Fatalf("статус после ExpireDue: %q, ожидался %q", got, models.ContractStatusExpired)
	}

	logs := logTypes(t, s.db, contract.ID)
	if logs[models.ContractLogFailed] != 1 {
		t.Fatalf("ожидался лог failed (взятый контракт), журнал: %v", logs)
	}
	if logs[models.ContractLogEscrowReturned] != 1 {
		t.Fatalf("ожидался лог escrow_returned, журнал: %v", logs)
	}

	// Залог возвращён автору полностью.
	authorBalance, authorWithdrawable := accountBalance(t, s.db, models.AccountOwnerPlayer, authorID)
	if authorBalance != models.PlayerBalanceSeed || authorWithdrawable != 0 {
		t.Fatalf("счёт автора после истечения: balance=%d withdrawable=%d",
			authorBalance, authorWithdrawable)
	}
	ops, delta, after := moneyOp(t, s.db, models.AccountOwnerPlayer, authorID, models.MoneyOpEscrowReturn)
	if ops != 1 || delta != itReward || after != models.PlayerBalanceSeed {
		t.Fatalf("escrow_return: n=%d delta=%d balance_after=%d (ожидалось 1/%d/%d)",
			ops, delta, after, itReward, models.PlayerBalanceSeed)
	}
}
