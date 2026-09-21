// internal/repository/contract_repository.go
//
// Контракт как сущность: состояние + эскроу (спеки
// 2026-09-22-контракт-модель-сущности §4–§6, 2026-09-22-деньги-и-эскроу §3–§4).
// Каждый переход статуса — ОДИН условный UPDATE с проверкой старого статуса
// (инвариант 1, AGENTS.md §0): RowsAffected == 0 = гонка, действие отвергнуто.
// Залог живёт на контракте: lock со счёта автора при публикации, release
// исполнителю при выполнении, return автору при отмене/истечении/удалении.
// escrow_amount не обнуляется — исторический факт (§4.1).
package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"zorion/internal/models"
)

// ErrInsufficientFunds — на счёте плательщика не хватает средств на залог.
var ErrInsufficientFunds = errors.New("недостаточно средств для залога")

// ErrInvalidReward — цена контракта обязана быть положительной (залог живого
// контракта > 0, CHECK contracts_live_has_escrow).
var ErrInvalidReward = errors.New("цена контракта должна быть больше нуля")

// ErrPayerUnresolved — не удалось свести автора к счёту-плательщику (спека
// денег §4: player/faction напрямую, building → владелец, agent → фракция).
var ErrPayerUnresolved = errors.New("не удалось определить счёт плательщика")

// querier — общее для *sql.DB и *sql.Tx: методы репозитория работают и в
// одиночной транзакции, и внутри уже открытой (пути удаления, §6.5).
type querier interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// ContractScope — область действия ленивого истечения (§6.3) и возврата залога
// (§6.5). Задаётся ровно одна область; приоритет — по порядку проверки.
type ContractScope struct {
	ContractID          string   // один контракт (проверка при прибытии)
	PlanetID            string   // доска планеты (publication_planet_id)
	WorldIDs            []string // планеты миров (удаление миров/планет)
	PayloadDestWorldIDs []string // payload->>'dest_world_id' (мёртвая цель пакмана)
	All                 bool     // вся таблица (очистка вселенной)
}

// clause — SQL-предикат и аргументы области. Плейсхолдеры начинаются с $1.
func (s ContractScope) clause() (string, []interface{}) {
	switch {
	case s.ContractID != "":
		return "id = $1", []interface{}{s.ContractID}
	case s.PlanetID != "":
		return "publication_planet_id = $1", []interface{}{s.PlanetID}
	case len(s.WorldIDs) > 0:
		return "publication_planet_id IN (SELECT id FROM planets WHERE world_id = ANY($1))",
			[]interface{}{pq.Array(s.WorldIDs)}
	case len(s.PayloadDestWorldIDs) > 0:
		return "payload->>'dest_world_id' = ANY($1)",
			[]interface{}{pq.Array(s.PayloadDestWorldIDs)}
	default:
		return "TRUE", nil
	}
}

// PublishContractParams — вход публикации контракта. Публикация и запирание
// залога идут ОДНОЙ транзакцией (§5): либо контракт open с залогом, либо отказ.
type PublishContractParams struct {
	ID                  string // пусто — сгенерируется
	Type                string
	AuthorType          string
	AuthorID            string
	PublicationPlanetID string
	Title               string
	Description         string
	Payload             map[string]interface{}
	Reward              int64
	Funding             string // пусто — regular
	Visibility          string // пусто — public
	DirectTargetType    *string
	DirectTargetID      *string
	ExpiresAt           time.Time
	Requirements        []models.ContractRequirement
}

const contractColumns = `id, type, author_type, author_id, publication_planet_id, title, description,
	payload, reward, funding, escrow_amount, escrow_withdrawable, escrow_kind, status, visibility,
	direct_target_type, direct_target_id, executor_type, executor_id, taken_at, expires_at, created_at, updated_at`

// contractMineLimit — верхняя граница «моих контрактов» (стоимость не растёт
// с числом контрактов, инвариант 2).
const contractMineLimit = 200

// Залог: одно действие — проверка достатка и списание атомарны (инвариант 1).
// CTE фиксирует строку счёта (FOR UPDATE) и отдаёт выводимую долю залога
// e = LEAST(withdrawable, amount), которая переносится на контракт.
const lockEscrowSQL = `
	WITH acc AS (
		SELECT balance, withdrawable FROM accounts
		WHERE owner_type = $1 AND owner_id = $2
		FOR UPDATE
	)
	UPDATE accounts a
	SET balance = a.balance - $3,
	    withdrawable = a.withdrawable - LEAST(a.withdrawable, $3),
	    updated_at = NOW()
	FROM acc
	WHERE a.owner_type = $1 AND a.owner_id = $2 AND acc.balance >= $3
	RETURNING a.balance, a.withdrawable, LEAST(acc.withdrawable, $3)`

// Возврат залога автору: баланс и выводимая доля восстанавливаются вместе
// (метка не сжигается, спека денег §3.3).
const returnEscrowSQL = `
	UPDATE accounts
	SET balance = balance + $3, withdrawable = withdrawable + $4, updated_at = NOW()
	WHERE owner_type = $1 AND owner_id = $2
	RETURNING balance`

// Начисление исполнителю: обычный контракт — только balance; подряд — ещё и
// withdrawable (contract_work_earn, спека денег §3.3).
const releaseEscrowSQL = `
	UPDATE accounts
	SET balance = balance + $3, withdrawable = withdrawable + $4, updated_at = NOW()
	WHERE owner_type = $1 AND owner_id = $2
	RETURNING balance`

const insertContractSQL = `
	INSERT INTO contracts (
		id, type, author_type, author_id, publication_planet_id, title, description,
		payload, reward, funding, escrow_amount, escrow_withdrawable, escrow_kind,
		status, visibility, direct_target_type, direct_target_id,
		expires_at, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`

const insertRequirementSQL = `
	INSERT INTO contract_requirements (contract_id, pos, kind, subject, op, threshold_num, threshold_text, quantity)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`

const insertContractLogSQL = `
	INSERT INTO contract_log (contract_id, type, actor_type, actor_id, data, occurred_at)
	VALUES ($1,$2,$3,$4,$5,$6)`

const insertMoneyOpSQL = `
	INSERT INTO money_operations (owner_type, owner_id, delta, balance_after, kind, contract_id, occurred_at)
	VALUES ($1,$2,$3,$4,$5,$6,$7)`

// Взятие: атомарный flip open→taken с проверкой срока (0 строк = гонка/истёк).
const takeContractSQL = `
	UPDATE contracts
	SET status = 'taken', executor_type = $2, executor_id = $3, taken_at = $4, updated_at = $4
	WHERE id = $1 AND status = 'open' AND expires_at > NOW()`

// Отмена автором: атомарный flip open→cancelled только для автора.
const cancelContractSQL = `
	UPDATE contracts
	SET status = 'cancelled', updated_at = NOW()
	WHERE id = $1 AND status = 'open' AND author_type = $2 AND author_id = $3
	RETURNING id, author_type, author_id, executor_id, escrow_amount, escrow_withdrawable`

// Выполнение: атомарный flip taken→completed с проверкой исполнителя.
const completeContractSQL = `
	UPDATE contracts
	SET status = 'completed', updated_at = NOW()
	WHERE id = $1 AND status = 'taken' AND executor_id = $2
	RETURNING id, author_type, author_id, executor_type, executor_id,
	          escrow_amount, escrow_withdrawable, funding`

// Ленивое истечение: атомарный flip open/taken→expired по сроку в области.
const expireDueSQL = `
	UPDATE contracts
	SET status = 'expired', updated_at = NOW()
	WHERE status IN ('open', 'taken') AND expires_at <= NOW() AND `

// Возврат залога при удалении контрактов: атомарный статусный flip (§6.5),
// форма по статусу (open→cancelled, taken→expired).
const returnEscrowForContractsSQL = `
	UPDATE contracts
	SET status = CASE WHEN executor_id IS NULL THEN 'cancelled' ELSE 'expired' END,
	    updated_at = NOW()
	WHERE `

// ContractRepository — доступ к contracts/contract_requirements/contract_log.
type ContractRepository struct {
	db *sql.DB
}

func NewContractRepository(db *sql.DB) *ContractRepository {
	return &ContractRepository{db: db}
}

// escrowRow — строка RETURNING для возврата/выпуска залога.
type escrowRow struct {
	id           string
	authorType   string
	authorID     string
	executorID   *string
	amount       int64
	withdrawable int64
}

// Publish — публикация контракта (draft→open): атомарно вставляет контракт,
// запирает залог со счёта автора, пишет contract_log (published+escrow_locked)
// и money_operations('escrow_lock').
func (r *ContractRepository) Publish(p PublishContractParams) (*models.Contract, error) {
	if p.Reward <= 0 {
		return nil, ErrInvalidReward
	}
	id := p.ID
	if id == "" {
		id = uuid.New().String()
	}
	funding := p.Funding
	if funding == "" {
		funding = models.ContractFundingRegular
	}
	visibility := p.Visibility
	if visibility == "" {
		visibility = models.ContractVisibilityPublic
	}
	now := time.Now()

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ownerType, ownerID, err := resolvePayerAccountQ(tx, p.AuthorType, p.AuthorID)
	if err != nil {
		return nil, err
	}
	if err := ensurePayerAccount(tx, ownerType, ownerID); err != nil {
		return nil, err
	}

	var balanceAfter, withdrawableAfter, escrowWithdrawable int64
	err = tx.QueryRow(lockEscrowSQL, ownerType, ownerID, p.Reward).
		Scan(&balanceAfter, &withdrawableAfter, &escrowWithdrawable)
	if err == sql.ErrNoRows {
		return nil, ErrInsufficientFunds
	}
	if err != nil {
		return nil, fmt.Errorf("escrow lock: %w", err)
	}
	_ = withdrawableAfter

	payloadJSON := []byte("{}")
	if p.Payload != nil {
		payloadJSON, err = json.Marshal(p.Payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
	}

	if _, err := tx.Exec(insertContractSQL,
		id, p.Type, p.AuthorType, p.AuthorID, p.PublicationPlanetID,
		p.Title, p.Description, string(payloadJSON), p.Reward, funding,
		p.Reward, escrowWithdrawable, models.EscrowKindDeposit,
		models.ContractStatusOpen, visibility, p.DirectTargetType, p.DirectTargetID,
		p.ExpiresAt, now, now,
	); err != nil {
		return nil, fmt.Errorf("insert contract: %w", err)
	}

	for i, req := range p.Requirements {
		pos := req.Pos
		if pos == 0 {
			pos = i + 1
		}
		if _, err := tx.Exec(insertRequirementSQL, id, pos, req.Kind, req.Subject, req.Op,
			req.ThresholdNum, req.ThresholdText, req.Quantity); err != nil {
			return nil, fmt.Errorf("insert requirement: %w", err)
		}
	}

	actorType, actorID := p.AuthorType, p.AuthorID
	if err := insertContractLog(tx, id, models.ContractLogPublished, &actorType, &actorID,
		map[string]interface{}{}, now); err != nil {
		return nil, err
	}
	if err := insertContractLog(tx, id, models.ContractLogEscrowLocked, &actorType, &actorID,
		map[string]interface{}{"amount": p.Reward, "withdrawable": escrowWithdrawable}, now); err != nil {
		return nil, err
	}
	if err := insertMoneyOp(tx, ownerType, ownerID, -p.Reward, balanceAfter,
		models.MoneyOpEscrowLock, id, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetByID(id)
}

// Take — взятие контракта: атомарный flip open→taken (0 строк = гонка/истёк).
// Счёт агента-исполнителя гарантируется здесь (§3.4: агент не аутентифицируется).
func (r *ContractRepository) Take(contractID, executorType, executorID string) (bool, error) {
	now := time.Now()
	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	if executorType == models.ContractExecutorAgent {
		if err := ensureAccount(tx, models.AccountOwnerAgent, executorID, 0); err != nil {
			return false, err
		}
	}

	res, err := tx.Exec(takeContractSQL, contractID, executorType, executorID, now)
	if err != nil {
		return false, fmt.Errorf("take contract: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	if err := insertContractLog(tx, contractID, models.ContractLogTaken, &executorType, &executorID,
		map[string]interface{}{}, now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// Cancel — отмена автором открытого контракта: flip open→cancelled + возврат
// залога автору + лог (cancelled + escrow_returned).
func (r *ContractRepository) Cancel(contractID, authorType, authorID string) (bool, error) {
	now := time.Now()
	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var row escrowRow
	var eid sql.NullString
	err = tx.QueryRow(cancelContractSQL, contractID, authorType, authorID).
		Scan(&row.id, &row.authorType, &row.authorID, &eid, &row.amount, &row.withdrawable)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cancel contract: %w", err)
	}
	if eid.Valid {
		row.executorID = &eid.String
	}
	if err := returnEscrowRow(tx, row, models.ContractLogCancelled, models.EscrowReasonCancelled, now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// Complete — выполнение взятого контракта: flip taken→completed + выпуск залога
// исполнителю (escrow_release / contract_work_earn) + лог. В B1 вызывается
// точками прибытия перелёта (B2) — метод существует уже здесь.
func (r *ContractRepository) Complete(contractID, executorID string) (bool, error) {
	now := time.Now()
	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var (
		id, authorType, authorID string
		executorType             string
		eid                      string
		amount, withdrawable     int64
		funding                  string
	)
	err = tx.QueryRow(completeContractSQL, contractID, executorID).
		Scan(&id, &authorType, &authorID, &executorType, &eid, &amount, &withdrawable, &funding)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("complete contract: %w", err)
	}

	ownerType, ownerID, err := resolveExecutorAccount(executorType, eid)
	if err != nil {
		return false, err
	}
	if err := ensureAccount(tx, ownerType, ownerID, 0); err != nil {
		return false, err
	}
	wDelta := int64(0)
	kind := models.MoneyOpEscrowRelease
	if funding == models.ContractFundingContractWork {
		wDelta = amount
		kind = models.MoneyOpContractWork
	}
	var balanceAfter int64
	if err := tx.QueryRow(releaseEscrowSQL, ownerType, ownerID, amount, wDelta).
		Scan(&balanceAfter); err != nil {
		return false, fmt.Errorf("escrow release: %w", err)
	}
	if err := insertMoneyOp(tx, ownerType, ownerID, amount, balanceAfter, kind, id, now); err != nil {
		return false, err
	}

	execActor := executorType
	if err := insertContractLog(tx, id, models.ContractLogCompleted, &execActor, &eid,
		map[string]interface{}{}, now); err != nil {
		return false, err
	}
	if err := insertContractLog(tx, id, models.ContractLogEscrowReleased, &execActor, &eid,
		map[string]interface{}{"amount": amount, "funding": funding}, now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ExpireDue — ленивое истечение (§6.3): одна транзакция — flip по сроку в
// области + возврат залога автору + лог (failed при непустом executor_id, иначе
// expired). Возвращает число истёкших контрактов.
func (r *ContractRepository) ExpireDue(scope ContractScope) (int, error) {
	where, args := scope.clause()
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	n, err := sweepEscrow(tx, expireDueSQL+where, args, models.ContractLogExpired, models.EscrowReasonExpired)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// ReturnEscrowForContracts — возврат залога всех живых контрактов области
// атомарным статусным flip'ом (§6.5), отдельной транзакцией. Форма по статусу:
// open→cancelled (лог cancelled), taken→expired (лог failed).
func (r *ContractRepository) ReturnEscrowForContracts(scope ContractScope, reason string) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n, err := returnEscrowForContractsQ(tx, scope, reason)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// ReturnEscrowForContractsTx — то же, но в уже открытой транзакции вызывающего
// (пути удаления контрактов: возврат и DELETE/TRUNCATE — одна транзакция, §6.5).
func ReturnEscrowForContractsTx(tx *sql.Tx, scope ContractScope, reason string) (int, error) {
	return returnEscrowForContractsQ(tx, scope, reason)
}

func returnEscrowForContractsQ(q querier, scope ContractScope, reason string) (int, error) {
	where, args := scope.clause()
	sqlText := returnEscrowForContractsSQL + where + ` AND status IN ('open', 'taken')
		RETURNING id, author_type, author_id, executor_id, escrow_amount, escrow_withdrawable`
	return sweepEscrow(q, sqlText, args, models.ContractLogCancelled, reason)
}

// sweepEscrow — выполняет UPDATE...RETURNING, собирает строки (на одном
// соединении tx нельзя параллелить запросы), затем возвращает залог по каждой.
func sweepEscrow(q querier, sqlText string, args []interface{}, openLogType, reason string) (int, error) {
	rows, err := q.Query(sqlText, args...)
	if err != nil {
		return 0, fmt.Errorf("escrow sweep: %w", err)
	}
	var pending []escrowRow
	for rows.Next() {
		var row escrowRow
		var eid sql.NullString
		if err := rows.Scan(&row.id, &row.authorType, &row.authorID, &eid, &row.amount, &row.withdrawable); err != nil {
			rows.Close()
			return 0, err
		}
		if eid.Valid {
			row.executorID = &eid.String
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	now := time.Now()
	for _, row := range pending {
		if err := returnEscrowRow(q, row, openLogType, reason, now); err != nil {
			return 0, err
		}
	}
	return len(pending), nil
}

// returnEscrowRow — возврат залога одной строки автору (по резолву §4) +
// money_operations('escrow_return') + лог. Форма лога: взятый контракт → failed.
func returnEscrowRow(q querier, row escrowRow, openLogType, reason string, now time.Time) error {
	ownerType, ownerID, err := resolvePayerAccountQ(q, row.authorType, row.authorID)
	if err != nil {
		return err
	}
	if err := ensurePayerAccount(q, ownerType, ownerID); err != nil {
		return err
	}
	var balanceAfter int64
	if err := q.QueryRow(returnEscrowSQL, ownerType, ownerID, row.amount, row.withdrawable).
		Scan(&balanceAfter); err != nil {
		return fmt.Errorf("escrow return: %w", err)
	}
	if err := insertMoneyOp(q, ownerType, ownerID, row.amount, balanceAfter,
		models.MoneyOpEscrowReturn, row.id, now); err != nil {
		return err
	}

	logType := openLogType
	if row.executorID != nil {
		logType = models.ContractLogFailed
	}
	system := "system"
	if err := insertContractLog(q, row.id, logType, &system, nil, map[string]interface{}{}, now); err != nil {
		return err
	}
	return insertContractLog(q, row.id, models.ContractLogEscrowReturned, &system, nil,
		map[string]interface{}{"amount": row.amount, "reason": reason}, now)
}

// GetByID — контракт по id (nil, nil — нет), с требованиями.
func (r *ContractRepository) GetByID(id string) (*models.Contract, error) {
	row := r.db.QueryRow(`SELECT `+contractColumns+` FROM contracts WHERE id = $1`, id)
	c, err := scanContract(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := attachRequirements(r.db, []*models.Contract{c}); err != nil {
		return nil, err
	}
	return c, nil
}

// ListBoard — доска планеты (§2.1): живые публичные контракты планеты.
// Прямые (visibility='direct') и закрытые/истёкшие не показываются.
func (r *ContractRepository) ListBoard(planetID string) ([]*models.Contract, error) {
	rows, err := r.db.Query(`SELECT `+contractColumns+` FROM contracts
		WHERE publication_planet_id = $1 AND status = 'open' AND expires_at > NOW() AND visibility = 'public'
		ORDER BY created_at DESC`, planetID)
	if err != nil {
		return nil, fmt.Errorf("list board: %w", err)
	}
	contracts, err := scanContracts(rows)
	if err != nil {
		return nil, err
	}
	if err := attachRequirements(r.db, contracts); err != nil {
		return nil, err
	}
	return contracts, nil
}

// ListMine — «мои контракты» (§2.2): где я автор или исполнитель, во всех
// статусах. Ограничено contractMineLimit (стоимость не растёт, инвариант 2).
func (r *ContractRepository) ListMine(ownerType, ownerID string) ([]*models.Contract, error) {
	rows, err := r.db.Query(`SELECT `+contractColumns+` FROM contracts
		WHERE (author_type = $1 AND author_id = $2) OR (executor_type = $1 AND executor_id = $2)
		ORDER BY created_at DESC
		LIMIT $3`, ownerType, ownerID, contractMineLimit)
	if err != nil {
		return nil, fmt.Errorf("list mine: %w", err)
	}
	contracts, err := scanContracts(rows)
	if err != nil {
		return nil, err
	}
	if err := attachRequirements(r.db, contracts); err != nil {
		return nil, err
	}
	return contracts, nil
}

// resolvePayerAccountQ — резолв плательщика (спека денег §4), глубина 1:
// player/faction — сам владелец; building → владелец постройки; agent →
// фракция-владелец. Возвращает (owner_type, owner_id) счёта.
func resolvePayerAccountQ(q querier, authorType, authorID string) (string, string, error) {
	switch authorType {
	case models.ContractActorPlayer:
		return models.AccountOwnerPlayer, authorID, nil
	case models.ContractActorFaction:
		return models.AccountOwnerFaction, authorID, nil
	case models.ContractActorBuilding:
		var ownerType, ownerID string
		err := q.QueryRow(`SELECT owner_type, owner_id FROM buildings WHERE id = $1`, authorID).
			Scan(&ownerType, &ownerID)
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("%w: постройка %s не найдена", ErrPayerUnresolved, authorID)
		}
		if err != nil {
			return "", "", fmt.Errorf("resolve building owner: %w", err)
		}
		switch ownerType {
		case models.AccountOwnerPlayer, models.AccountOwnerFaction, models.AccountOwnerAgent:
			return ownerType, ownerID, nil
		default:
			return "", "", fmt.Errorf("%w: владелец постройки типа %q", ErrPayerUnresolved, ownerType)
		}
	case models.ContractActorAgent:
		var factionID sql.NullString
		err := q.QueryRow(`SELECT owner_faction_id FROM npc_agents WHERE id = $1`, authorID).Scan(&factionID)
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("%w: агент %s не найден", ErrPayerUnresolved, authorID)
		}
		if err != nil {
			return "", "", fmt.Errorf("resolve agent owner: %w", err)
		}
		if !factionID.Valid {
			return "", "", fmt.Errorf("%w: у агента %s нет фракции-владельца", ErrPayerUnresolved, authorID)
		}
		return models.AccountOwnerFaction, factionID.String, nil
	default:
		return "", "", fmt.Errorf("%w: неизвестный тип автора %q", ErrPayerUnresolved, authorType)
	}
}

// resolveExecutorAccount — счёт исполнителя (спека денег §3.4/§6): игрок или
// агент, счёт гарантируется в пути контракта.
func resolveExecutorAccount(executorType, executorID string) (string, string, error) {
	switch executorType {
	case models.ContractExecutorPlayer:
		return models.AccountOwnerPlayer, executorID, nil
	case models.ContractExecutorAgent:
		return models.AccountOwnerAgent, executorID, nil
	default:
		return "", "", fmt.Errorf("unknown executor type %q", executorType)
	}
}

// ensurePayerAccount — идемпотентная страховка счёта плательщика (§3.4):
// игрок — со стартовым капиталом, фракция — с «бесконечной» казной (§5).
func ensurePayerAccount(q querier, ownerType, ownerID string) error {
	seed := int64(0)
	switch ownerType {
	case models.AccountOwnerPlayer:
		seed = models.PlayerBalanceSeed
	case models.AccountOwnerFaction:
		seed = models.FactionBalanceSeed
	}
	return ensureAccount(q, ownerType, ownerID, seed)
}

// insertContractLog — запись журнала жизни контракта (§4.3).
func insertContractLog(q querier, contractID, logType string, actorType, actorID *string,
	data map[string]interface{}, now time.Time) error {
	dataJSON := []byte("{}")
	if data != nil {
		var err error
		dataJSON, err = json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshal contract log data: %w", err)
		}
	}
	if _, err := q.Exec(insertContractLogSQL, contractID, logType, actorType, actorID,
		string(dataJSON), now); err != nil {
		return fmt.Errorf("insert contract log: %w", err)
	}
	return nil
}

// insertMoneyOp — запись журнала движения по счёту (§3.2).
func insertMoneyOp(q querier, ownerType, ownerID string, delta, balanceAfter int64,
	kind, contractID string, now time.Time) error {
	if _, err := q.Exec(insertMoneyOpSQL, ownerType, ownerID, delta, balanceAfter, kind,
		contractID, now); err != nil {
		return fmt.Errorf("insert money operation: %w", err)
	}
	return nil
}

// scanContract — скан строки contracts (contractColumns).
func scanContract(sc interface {
	Scan(dest ...interface{}) error
}) (*models.Contract, error) {
	var c models.Contract
	var payloadRaw []byte
	var dtt, dti, et, eid sql.NullString
	var takenAt sql.NullTime
	if err := sc.Scan(
		&c.ID, &c.Type, &c.AuthorType, &c.AuthorID, &c.PublicationPlanetID,
		&c.Title, &c.Description, &payloadRaw, &c.Reward, &c.Funding,
		&c.EscrowAmount, &c.EscrowWithdrawable, &c.EscrowKind, &c.Status, &c.Visibility,
		&dtt, &dti, &et, &eid, &takenAt, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(payloadRaw) > 0 && string(payloadRaw) != "null" {
		if err := json.Unmarshal(payloadRaw, &c.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal payload: %w", err)
		}
	}
	if dtt.Valid {
		c.DirectTargetType = &dtt.String
	}
	if dti.Valid {
		c.DirectTargetID = &dti.String
	}
	if et.Valid {
		c.ExecutorType = &et.String
	}
	if eid.Valid {
		c.ExecutorID = &eid.String
	}
	if takenAt.Valid {
		c.TakenAt = &takenAt.Time
	}
	return &c, nil
}

// scanContracts — скан набора строк + закрытие.
func scanContracts(rows *sql.Rows) ([]*models.Contract, error) {
	defer rows.Close()
	contracts := make([]*models.Contract, 0, 16)
	for rows.Next() {
		c, err := scanContract(rows)
		if err != nil {
			return nil, err
		}
		contracts = append(contracts, c)
	}
	return contracts, rows.Err()
}

// attachRequirements — догружает требования для набора контрактов одним
// запросом (без N+1). contract_id::text = ANY(text[]) — без угадывания типа.
func attachRequirements(q querier, contracts []*models.Contract) error {
	if len(contracts) == 0 {
		return nil
	}
	ids := make([]string, 0, len(contracts))
	index := make(map[string]*models.Contract, len(contracts))
	for _, c := range contracts {
		ids = append(ids, c.ID)
		index[c.ID] = c
	}
	rows, err := q.Query(`SELECT id, contract_id, pos, kind, subject, op,
			threshold_num, threshold_text, quantity
		FROM contract_requirements WHERE contract_id::text = ANY($1)
		ORDER BY contract_id, pos`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("attach requirements: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var req models.ContractRequirement
		var tn sql.NullFloat64
		var tt sql.NullString
		var qty sql.NullInt64
		if err := rows.Scan(&req.ID, &req.ContractID, &req.Pos, &req.Kind, &req.Subject, &req.Op,
			&tn, &tt, &qty); err != nil {
			return err
		}
		if tn.Valid {
			req.ThresholdNum = &tn.Float64
		}
		if tt.Valid {
			req.ThresholdText = &tt.String
		}
		if qty.Valid {
			req.Quantity = &qty.Int64
		}
		if c := index[req.ContractID]; c != nil {
			c.Requirements = append(c.Requirements, req)
		}
	}
	return rows.Err()
}
