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

// ErrPublicationPlanetUnresolved — не удалось определить планету публикации по
// автору (спека перелёта §3): игрок не стоит на планете, у фракции/постройки
// нет родной планеты, агент-автор не поддержан в итерации 1.
var ErrPublicationPlanetUnresolved = errors.New("не удалось определить планету публикации")

// ErrPackageShareTaken — у игрока уже есть взятая доля этого пакета (§4.3
// спеки 2026-09-23-контракт-ленивая-доска-пакет-и-снабжение; несёт частичный
// уникальный индекс uq_contracts_package_taken_executor). Отличается от
// обычной гонки взятия (0 строк).
var ErrPackageShareTaken = errors.New("у вас уже есть взятая доля этого пакета")

// isPackageShareTakenViolation — нарушение уникальности «один игрок — одна
// взятая доля пакета» (23505 + конкретный индекс). Различаем явно: обычная
// гонка/истёк срок дают 0 строк без ошибки (§4.3).
func isPackageShareTakenViolation(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return pqErr.Code == "23505" && pqErr.Constraint == "uq_contracts_package_taken_executor"
}

// ErrPackageShareOpenDuplicate — открытая доля этого пакета с таким номером уже
// существует (§4.2 спеки 2026-09-23-контракт-ленивая-доска-пакет-и-снабжение;
// несёт частичный уникальный индекс uq_contracts_package_open_share). Отличается
// от прочих ошибок публикации — маппится в 409.
var ErrPackageShareOpenDuplicate = errors.New("открытая доля этого пакета с таким номером уже существует")

// isPackageShareOpenDuplicateViolation — нарушение уникальности «одна открытая
// доля на (package_key, share_index)» (23505 + конкретный индекс).
func isPackageShareOpenDuplicateViolation(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return pqErr.Code == "23505" && pqErr.Constraint == "uq_contracts_package_open_share"
}

// querier — общее для *sql.DB и *sql.Tx: методы репозитория работают и в
// одиночной транзакции, и внутри уже открытой (пути удаления, §6.5).
type querier interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// ContractScope — область действия ленивого истечения (§6.3) и возврата залога
// (§6.5). Задаётся ровно одна область; приоритет — по порядку проверки.
// Пустая область (все поля нулевые) = вся таблица (очистка вселенной).
type ContractScope struct {
	ContractID          string   // один контракт (проверка при прибытии)
	PlanetID            string   // доска планеты (publication_planet_id)
	WorldIDs            []string // планеты миров (удаление миров/планет)
	PayloadDestWorldIDs []string // payload->>'dest_world_id' (мёртвая цель пакмана)
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
	// PackageKey/ShareIndex — «пакет контрактов» (§4.2 спеки 2026-09-23-контракт-
	// ленивая-доска-пакет-и-снабжение): у обычных публикаций (игрок, перелёт) NULL.
	PackageKey   *string
	ShareIndex   *int
	ExpiresAt    time.Time
	Requirements []models.ContractRequirement
}

const contractColumns = `id, type, author_type, author_id, publication_planet_id, title, description,
	payload, reward, funding, escrow_amount, escrow_withdrawable, escrow_kind, status, visibility,
	direct_target_type, direct_target_id, executor_type, executor_id, package_key, share_index,
	taken_at, expires_at, created_at, updated_at`

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
		status, visibility, direct_target_type, direct_target_id, package_key, share_index,
		expires_at, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`

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
// expires_at перебазируется, если передан новый срок ($5 != NULL) — правило
// типа travel (спека перелёта §4.3: срок исполнения отсчитывается от взятия,
// капкан «взял перед истечением»). У других типов $5 = NULL → срок не трогаем.
const takeContractSQL = `
	UPDATE contracts
	SET status = 'taken', executor_type = $2, executor_id = $3, taken_at = $4,
	    expires_at = COALESCE($5::timestamptz, expires_at), updated_at = $4
	WHERE id = $1 AND status = 'open' AND expires_at > NOW()`

// Отмена автором: атомарный flip open→cancelled только для автора.
const cancelContractSQL = `
	UPDATE contracts
	SET status = 'cancelled', updated_at = NOW()
	WHERE id = $1 AND status = 'open' AND author_type = $2 AND author_id = $3
	RETURNING id, author_type, author_id, executor_id, escrow_amount, escrow_withdrawable`

// Выполнение: атомарный flip taken→completed с проверкой исполнителя и срока.
// Просроченный взятый контракт НЕ завершается (0 строк): его закрывает ленивое
// истечение ExpireDue (залог возвращается автору, лог failed, §6.3/§1.4).
// Точка прибытия обязана сперва вызвать ExpireDue, затем Complete.
const completeContractSQL = `
	UPDATE contracts
	SET status = 'completed', updated_at = NOW()
	WHERE id = $1 AND status = 'taken' AND executor_id = $2 AND expires_at > NOW()
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
	// boardNeeds — источник нужд планеты для ленивой материализации доски
	// (§5.1, Поставка 2). v1: nil = нужд нет (см. contract_board_repository.go).
	boardNeeds boardNeedsFunc
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
// и money_operations('escrow_lock'). Supply с нулевой наградой публикуется
// бесплатно: без залога и money_op (escrow_amount = 0, §5.5).
func (r *ContractRepository) Publish(p PublishContractParams) (*models.Contract, error) {
	// D5 (спека 2026-09-25-внутреннее-хранилище §5.5): supply с нулевой наградой
	// публикуется бесплатно (у владельца нет денег — снабжение не встаёт); прочие
	// типы и отрицательные значения — прежний отказ.
	if p.Reward < 0 || (p.Reward == 0 && p.Type != models.ContractTypeSupply) {
		return nil, ErrInvalidReward
	}
	paid := p.Reward > 0
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

	// Бесплатный путь (reward = 0): эскроу не запирается, счёт не трогается —
	// шаги lockEscrow/money_op пропускаются целиком (§5.5).
	var balanceAfter, escrowWithdrawable int64
	if paid {
		var withdrawableAfter int64
		err = tx.QueryRow(lockEscrowSQL, ownerType, ownerID, p.Reward).
			Scan(&balanceAfter, &withdrawableAfter, &escrowWithdrawable)
		if err == sql.ErrNoRows {
			return nil, ErrInsufficientFunds
		}
		if err != nil {
			return nil, fmt.Errorf("escrow lock: %w", err)
		}
		_ = withdrawableAfter
	}

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
		p.PackageKey, p.ShareIndex,
		p.ExpiresAt, now, now,
	); err != nil {
		if isPackageShareOpenDuplicateViolation(err) {
			return nil, ErrPackageShareOpenDuplicate
		}
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
	// escrow_locked пишется только при реальном залоге (не врём о залоге в
	// бесплатной публикации, §5.5); money_op — там же.
	if paid {
		if err := insertContractLog(tx, id, models.ContractLogEscrowLocked, &actorType, &actorID,
			map[string]interface{}{"amount": p.Reward, "withdrawable": escrowWithdrawable}, now); err != nil {
			return nil, err
		}
		if err := insertMoneyOp(tx, ownerType, ownerID, -p.Reward, balanceAfter,
			models.MoneyOpEscrowLock, id, now); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetByID(id)
}

// Take — взятие контракта: атомарный flip open→taken (0 строк = гонка/истёк).
// expiresAt != nil — перебазирование срока при взятии (правило типа travel,
// спека перелёта §4.3): срок исполнения отсчитывается от взятия. Счёт
// агента-исполнителя гарантируется здесь (§3.4: агент не аутентифицируется).
func (r *ContractRepository) Take(contractID, executorType, executorID string, expiresAt *time.Time) (bool, error) {
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

	res, err := tx.Exec(takeContractSQL, contractID, executorType, executorID, now, expiresAt)
	if err != nil {
		if isPackageShareTakenViolation(err) {
			return false, ErrPackageShareTaken
		}
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
	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	ok, err := completeTx(tx, contractID, executorID, time.Now())
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return true, tx.Commit()
}

// completeTx — тело завершения над querier (tx или БД): атомарный flip
// taken→completed + выпуск залога исполнителю + лог. 0 строк (просрочен/не наш)
// → (false, nil) без записи. Вынесено для CloseTravelArrivals (одна tx).
func completeTx(q querier, contractID, executorID string, now time.Time) (bool, error) {
	var (
		id, authorType, authorID string
		executorType             string
		eid                      string
		amount, withdrawable     int64
		funding                  string
	)
	err := q.QueryRow(completeContractSQL, contractID, executorID).
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
	if err := ensureAccount(q, ownerType, ownerID, 0); err != nil {
		return false, err
	}
	wDelta := int64(0)
	kind := models.MoneyOpEscrowRelease
	if funding == models.ContractFundingContractWork {
		wDelta = amount
		kind = models.MoneyOpContractWork
	}
	var balanceAfter int64
	if err := q.QueryRow(releaseEscrowSQL, ownerType, ownerID, amount, wDelta).
		Scan(&balanceAfter); err != nil {
		return false, fmt.Errorf("escrow release: %w", err)
	}
	if err := insertMoneyOp(q, ownerType, ownerID, amount, balanceAfter, kind, id, now); err != nil {
		return false, err
	}

	execActor := executorType
	if err := insertContractLog(q, id, models.ContractLogCompleted, &execActor, &eid,
		map[string]interface{}{}, now); err != nil {
		return false, err
	}
	if err := insertContractLog(q, id, models.ContractLogEscrowReleased, &execActor, &eid,
		map[string]interface{}{"amount": amount, "funding": funding}, now); err != nil {
		return false, err
	}
	return true, nil
}

// CloseTravelArrivals — закрытие контрактов-перелётов исполнителя по прибытии
// (спека перелёта §1.1/§1.4, B2a). Две точки: цель-система (planetID == "",
// payload->>'dest_planet_id' IS NULL) и цель-планета (planetID задан). Для
// каждого найденного контракта СПЕРВА истечение (просроченный — провал, залог
// автору), затем завершение (не просроченный, залог исполнителю); завершение
// не трогает просроченный (AND expires_at > NOW()) — 0 строк.
// Всё — ОДНОЙ транзакцией (инвариант «либо всё, либо ничего»): сбой на любом
// контракте откатывает проход, иначе после успешного истечения сбой завершения
// оставил бы контракт taken и прибытие (одноразовое) потеряло бы награду.
// Возвращает число фактически выполненных контрактов.
func (r *ContractRepository) CloseTravelArrivals(executorID, worldID, planetID string) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	query := `SELECT id FROM contracts
		WHERE type = 'travel' AND status = 'taken' AND executor_type = $1 AND executor_id = $2
		  AND payload->>'dest_world_id' = $3`
	args := []interface{}{models.ContractExecutorPlayer, executorID, worldID}
	if planetID == "" {
		query += ` AND payload->>'dest_planet_id' IS NULL`
	} else {
		query += ` AND payload->>'dest_planet_id' = $4`
		args = append(args, planetID)
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return 0, fmt.Errorf("close travel arrivals: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	now := time.Now()
	completed := 0
	for _, id := range ids {
		if _, err := expireDueTx(tx, ContractScope{ContractID: id}); err != nil {
			return 0, err
		}
		ok, err := completeTx(tx, id, executorID, now)
		if err != nil {
			return 0, err
		}
		if ok {
			completed++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return completed, nil
}

// ExpireDue — ленивое истечение (§6.3): одна транзакция — flip по сроку в
// области + возврат залога автору + лог (failed при непустом executor_id, иначе
// expired). Возвращает число истёкших контрактов.
func (r *ContractRepository) ExpireDue(scope ContractScope) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	n, err := expireDueTx(tx, scope)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// expireDueTx — тело ленивого истечения над querier (tx или БД): flip по сроку
// в области + возврат залога автору + лог. Вынесено для CloseTravelArrivals
// (одна tx: истечение и завершение атомарны сообща).
func expireDueTx(q querier, scope ContractScope) (int, error) {
	where, args := scope.clause()
	// RETURNING обязателен: sweepEscrow обрабатывает только возвращённые строки.
	// Без него UPDATE проходит, но залог автору не возвращается и лог не пишется.
	sqlText := expireDueSQL + where + `
		RETURNING id, author_type, author_id, executor_id, escrow_amount, escrow_withdrawable`
	return sweepEscrow(q, sqlText, args, models.ContractLogExpired, models.EscrowReasonExpired)
}

// ReturnEscrowForContractsTx — возврат залога всех живых контрактов области
// атомарным статусным flip'ом (§6.5) в уже открытой транзакции вызывающего
// (пути удаления контрактов: возврат и DELETE/TRUNCATE — одна транзакция, §6.5).
// Форма по статусу: open→cancelled (лог cancelled), taken→expired (лог failed).
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

// ResolvePublicationPlanet — место публикации по автору (спека перелёта §3):
// faction — factions.homeworld_id; building — buildings.planet_id; settlement —
// settlements.planet_id (спека ЧК2а §1.5/§6); agent — не поддержан в итерации 1
// (у агента нет планетного слоя, только current_world_id)
// → ErrPublicationPlanetUnresolved. Автор-player резолвится вызывающим по
// позиции игрока (то же правило, что в игровом пути публикации).
func (r *ContractRepository) ResolvePublicationPlanet(authorType, authorID string) (string, error) {
	switch authorType {
	case models.ContractActorFaction:
		var planetID sql.NullString
		err := r.db.QueryRow(`SELECT homeworld_id FROM factions WHERE id = $1`, authorID).Scan(&planetID)
		if err == sql.ErrNoRows || !planetID.Valid {
			return "", fmt.Errorf("%w: у фракции %s нет родной планеты", ErrPublicationPlanetUnresolved, authorID)
		}
		if err != nil {
			return "", fmt.Errorf("resolve faction homeworld: %w", err)
		}
		return planetID.String, nil
	case models.ContractActorBuilding:
		var planetID string
		err := r.db.QueryRow(`SELECT planet_id FROM buildings WHERE id = $1`, authorID).Scan(&planetID)
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("%w: постройка %s не найдена", ErrPublicationPlanetUnresolved, authorID)
		}
		if err != nil {
			return "", fmt.Errorf("resolve building planet: %w", err)
		}
		return planetID, nil
	case models.ContractActorSettlement:
		var planetID string
		err := r.db.QueryRow(`SELECT planet_id FROM settlements WHERE id = $1`, authorID).Scan(&planetID)
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("%w: поселение %s не найдено", ErrPublicationPlanetUnresolved, authorID)
		}
		if err != nil {
			return "", fmt.Errorf("resolve settlement planet: %w", err)
		}
		return planetID, nil
	default:
		return "", fmt.Errorf("%w: автор типа %q", ErrPublicationPlanetUnresolved, authorType)
	}
}

// resolvePayerAccountQ — резолв плательщика (спека денег §4), глубина 1:
// player/faction — сам владелец; building → владелец постройки; agent →
// фракция-владелец; settlement → владелец поселения, при владельце-агенте —
// счёт его фракции (D4, спека ЧК2а §1.5/§6). Возвращает (owner_type, owner_id) счёта.
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
	case models.ContractActorSettlement:
		// Автор-поселение: платит его владелец (owner_type/owner_id). Легаси без
		// владельца (Г1) заказчиком быть не может → ErrPayerUnresolved.
		var ownerType, ownerID sql.NullString
		err := q.QueryRow(`SELECT owner_type, owner_id FROM settlements WHERE id = $1`, authorID).
			Scan(&ownerType, &ownerID)
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("%w: поселение %s не найдено", ErrPayerUnresolved, authorID)
		}
		if err != nil {
			return "", "", fmt.Errorf("resolve settlement owner: %w", err)
		}
		if !ownerType.Valid || !ownerID.Valid {
			return "", "", fmt.Errorf("%w: у поселения %s нет владельца", ErrPayerUnresolved, authorID)
		}
		switch ownerType.String {
		case models.AccountOwnerPlayer, models.AccountOwnerFaction:
			return ownerType.String, ownerID.String, nil
		case models.AccountOwnerAgent:
			// D4: владелец-агент платит со счёта фракции (своего бюджета у агента нет).
			var factionID sql.NullString
			err := q.QueryRow(`SELECT owner_faction_id FROM npc_agents WHERE id = $1`, ownerID.String).
				Scan(&factionID)
			if err == sql.ErrNoRows {
				return "", "", fmt.Errorf("%w: агент %s не найден", ErrPayerUnresolved, ownerID.String)
			}
			if err != nil {
				return "", "", fmt.Errorf("resolve settlement agent owner: %w", err)
			}
			if !factionID.Valid {
				return "", "", fmt.Errorf("%w: у агента %s нет фракции-владельца", ErrPayerUnresolved, ownerID.String)
			}
			return models.AccountOwnerFaction, factionID.String, nil
		default:
			return "", "", fmt.Errorf("%w: владелец поселения типа %q", ErrPayerUnresolved, ownerType.String)
		}
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
	var pkg sql.NullString
	var share sql.NullInt64
	var takenAt sql.NullTime
	if err := sc.Scan(
		&c.ID, &c.Type, &c.AuthorType, &c.AuthorID, &c.PublicationPlanetID,
		&c.Title, &c.Description, &payloadRaw, &c.Reward, &c.Funding,
		&c.EscrowAmount, &c.EscrowWithdrawable, &c.EscrowKind, &c.Status, &c.Visibility,
		&dtt, &dti, &et, &eid, &pkg, &share, &takenAt, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt,
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
	if pkg.Valid {
		c.PackageKey = &pkg.String
	}
	if share.Valid {
		idx := int(share.Int64)
		c.ShareIndex = &idx
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
