// internal/repository/contract_delivery.go
//
// Точка сдачи груза по supply-контракту (спека
// 2026-09-25-сдача-груза-и-зачёт-ЧК2б §5.2): ОДНА транзакция — списание из трюма
// исполнителя → приход в ячейку хранилища автора → уменьшение остатка требования
// + пропорциональная выплата из остатка залога (частичная сдача) или полное
// закрытие существующим completeTx (полная сдача). Переиспользует приватные
// помощники пакета (completeTx/insertContractLog/insertMoneyOp/releaseEscrowSQL,
// resolveExecutorAccount/ensureAccount) и settlement.StorageCaps; трюм
// инъектируется узким интерфейсом CargoTaker — без импорта internal/cargo.
//
// Порядок локов (§5.2): contracts → contract_requirements → player_cargo(users)
// → settlement_storage_cells → accounts. Цикла с owner-проходом
// (advisory → settlements → ветки → залежи → ячейки) и доской
// (contract_board_state → contracts → accounts) нет.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

// Ошибки точки сдачи — маппятся хендлером в HTTP-коды (§5.3).
var (
	// ErrDeliveryNotAvailable — под локом контракт не supply/taken/не свой/истёк:
	// гонка с истечением или другой сдачей. 409.
	ErrDeliveryNotAvailable = errors.New("контракт недоступен для сдачи")
	// ErrDeliveryAuthorUnsupported — автор без хранилища (player/faction). 422.
	ErrDeliveryAuthorUnsupported = errors.New("заказ не принимает сдачу в хранилище")
	// ErrDeliveryBuildingUnsupported — автор-строение: источника размера
	// хранилища и реестра ячеек у строений сегодня нет (заказов от строений тоже
	// нет — задел, §0 п.15). Честный отказ вместо ложного «хранилище
	// переполнено» (size = 0 давал бы 409). 422.
	ErrDeliveryBuildingUnsupported = errors.New("сдача от строений пока не поддерживается")
	// ErrContractDamaged — не ровно одно goods-требование (0/несколько) либо
	// битые данные требования (quantity, subject). 422, fail-closed.
	ErrContractDamaged = errors.New("контракт повреждён")
	// ErrDeliveryGoodUnavailable — битый вес товара (weight <= 0). 422.
	ErrDeliveryGoodUnavailable = errors.New("товар недоступен для сдачи")
	// ErrDeliveryNoCargo — нужного товара в трюме нет. 422.
	ErrDeliveryNoCargo = errors.New("в трюме нет нужного товара")
	// ErrDeliveryStorageCellMissing — ячейки владельца по товару нет (вне
	// реестра нужд / удалена). 409, fail-closed (авто-создания ячейки нет, T14).
	ErrDeliveryStorageCellMissing = errors.New("заказ устарел")
	// ErrDeliveryStorageFull — двойной порог ячейки (2·cap) достигнут. 409.
	ErrDeliveryStorageFull = errors.New("хранилище переполнено")
	// ErrDeliveryNothing — сдавать нечего (delivered <= 0). 422.
	ErrDeliveryNothing = errors.New("нечего сдавать")
)

// CargoTaker — узкий интерфейс трюма (метод *cargo.Service.TakeCargoTx),
// инъектируется в ContractRepository (SetCargo), чтобы пакет repository не
// импортировал internal/cargo.
type CargoTaker interface {
	TakeCargoTx(tx *sql.Tx, userID string, goodID int64, qty float64) (float64, error)
}

// DeliveryResult — результат сдачи (§6). Денежные внутренности (escrow/балансы)
// не раскрываются (канон 14_money §14.4).
type DeliveryResult struct {
	Status     string `json:"status"` // completed | taken
	ContractID string `json:"contract_id"`
	Delivered  int64  `json:"delivered"`
	Remaining  int64  `json:"remaining"`
	Paid       int64  `json:"paid"`
	GoodID     int64  `json:"good_id"`
}

// deliveryEpsilon — допуск сравнения единиц/массы (§5.2 п.6, F7).
const deliveryEpsilon = 1e-9

// deliveryCellDoubleThreshold — множитель двойного порога ячейки (§0 п.13,
// фиксированный, не калибруется).
const deliveryCellDoubleThreshold = 2.0

const (
	// lockDeliveryContractSQL — лок контракта-цели (§5.2 п.1): только supply/
	// taken/свой исполнитель/не истёк. 0 строк → ErrDeliveryNotAvailable (гонка).
	lockDeliveryContractSQL = `
		SELECT author_type, author_id, escrow_amount, escrow_withdrawable, funding
		FROM contracts
		WHERE id = $1 AND type = 'supply' AND status = 'taken' AND executor_id = $2
		  AND expires_at > NOW()
		FOR UPDATE`

	// lockDeliveryUsersSQL — лок строки исполнителя (§0 п.6/§5.2 п.4): сериализует
	// мутации трюма одного игрока с TryAddCargo (тот же лок users).
	lockDeliveryUsersSQL = `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`

	// lockDeliveryCargoSQL — чтение трюма под локом (единицы, §5.2 п.4).
	lockDeliveryCargoSQL = `
		SELECT quantity FROM player_cargo WHERE user_id = $1 AND good_id = $2 FOR UPDATE`

	// lockDeliveryRequirementSQL — ровно одно goods-требование под локом (§5.2 п.2).
	lockDeliveryRequirementSQL = `
		SELECT id, subject, quantity
		FROM contract_requirements
		WHERE contract_id = $1 AND kind = 'goods' AND op = 'in'
		ORDER BY pos
		FOR UPDATE`

	// deliveryGoodWeightSQL — вес товара требования (§5.2 п.3).
	deliveryGoodWeightSQL = `SELECT weight FROM goods WHERE id = $1`

	// deliveryStorageSizeSQL — размер хранилища автора-поселения (§5.2 п.5).
	deliveryStorageSizeSQL = `SELECT storage_size FROM settlements WHERE id = $1`

	// deliveryRequirementZeroSQL — обнуление требования при полной сдаче (§5.2 п.10).
	deliveryRequirementZeroSQL = `UPDATE contract_requirements SET quantity = 0 WHERE id = $1`

	// deliveryRequirementDecrementSQL — уменьшение требования на принятое (§5.2 п.11).
	deliveryRequirementDecrementSQL = `
		UPDATE contract_requirements SET quantity = quantity - $2 WHERE id = $1`

	// deliveryEscrowUpdateSQL — новый остаток залога и пропорционально выводимая
	// доля при частичной сдаче (§5.2 п.11).
	deliveryEscrowUpdateSQL = `
		UPDATE contracts SET escrow_amount = $2, escrow_withdrawable = $3, updated_at = NOW()
		WHERE id = $1`
)

// Deliver — точка сдачи (§5.2): проверки под локами + одна транзакция. Возвращает
// результат сдачи либо sentinel-ошибку (§5.3). Сбой на любом шаге откатывает всё.
func (r *ContractRepository) Deliver(executorID, contractID string) (*DeliveryResult, error) {
	if r.cargo == nil {
		return nil, fmt.Errorf("deliver: трюм не подключён")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := time.Now()

	// 1. Лок контракта-цели.
	var authorType, authorID, funding string
	var escrowAmount, escrowWithdrawable int64
	err = tx.QueryRow(lockDeliveryContractSQL, contractID, executorID).
		Scan(&authorType, &authorID, &escrowAmount, &escrowWithdrawable, &funding)
	if err == sql.ErrNoRows {
		return nil, ErrDeliveryNotAvailable
	}
	if err != nil {
		return nil, fmt.Errorf("deliver: лок контракта: %w", err)
	}
	// 6. Автор — общий (§0 п.15): поселение/строение; иначе хранилища нет.
	if authorType != models.ContractActorSettlement && authorType != models.ContractActorBuilding {
		return nil, ErrDeliveryAuthorUnsupported
	}
	// Задел (§0 п.15): у строений сегодня нет источника размера хранилища и
	// реестра ячеек, заказов от строений тоже нет. Честный отказ вместо ложного
	// «хранилище переполнено» (размер-0 давал бы 409). Общий метод сдачи не
	// дробим — точка расширения здесь, когда у строений появятся заказы/размер.
	if authorType == models.ContractActorBuilding {
		return nil, ErrDeliveryBuildingUnsupported
	}

	// 2. Ровно одно goods-требование (fail-closed; первый молча не выбирается).
	reqRows, err := tx.Query(lockDeliveryRequirementSQL, contractID)
	if err != nil {
		return nil, fmt.Errorf("deliver: требования: %w", err)
	}
	var (
		reqID    int64
		reqSubj  string
		reqQty   sql.NullInt64
		reqCount int
	)
	for reqRows.Next() {
		var id int64
		var subj string
		var qty sql.NullInt64
		if err := reqRows.Scan(&id, &subj, &qty); err != nil {
			reqRows.Close()
			return nil, fmt.Errorf("deliver: скан требования: %w", err)
		}
		if reqCount == 0 {
			reqID, reqSubj, reqQty = id, subj, qty
		}
		reqCount++
	}
	if err := reqRows.Err(); err != nil {
		reqRows.Close()
		return nil, fmt.Errorf("deliver: перебор требований: %w", err)
	}
	reqRows.Close()
	if reqCount != 1 || !reqQty.Valid || reqQty.Int64 <= 0 {
		return nil, ErrContractDamaged
	}
	remaining := reqQty.Int64
	goodID, err := strconv.ParseInt(reqSubj, 10, 64)
	if err != nil || goodID <= 0 {
		return nil, ErrContractDamaged
	}

	// 3. Вес товара (единицы — операционные, масса — страховка канона, F7).
	var weight float64
	err = tx.QueryRow(deliveryGoodWeightSQL, goodID).Scan(&weight)
	if err == sql.ErrNoRows {
		return nil, ErrDeliveryGoodUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("deliver: вес товара: %w", err)
	}
	if weight <= 0 {
		return nil, ErrDeliveryGoodUnavailable
	}

	// 4. Трюм: лок users (сериализация мутаций трюма) + чтение под локом.
	if _, err := tx.Exec(lockDeliveryUsersSQL, executorID); err != nil {
		return nil, fmt.Errorf("deliver: лок игрока: %w", err)
	}
	var have float64
	err = tx.QueryRow(lockDeliveryCargoSQL, executorID, goodID).Scan(&have)
	if err == sql.ErrNoRows {
		return nil, ErrDeliveryNoCargo
	}
	if err != nil {
		return nil, fmt.Errorf("deliver: чтение трюма: %w", err)
	}
	if have <= deliveryEpsilon {
		return nil, ErrDeliveryNoCargo
	}

	// 5. Ячейки владельца под локом + двойной порог целевой ячейки.
	cellsRepo := NewStorageCellRepository(r.db)
	cells, err := cellsRepo.GetStorageCellsForUpdateTx(context.Background(), tx, authorType, authorID)
	if err != nil {
		return nil, err
	}
	weights := make(map[int64]float64, len(cells))
	var target *models.StorageCell
	for i := range cells {
		weights[cells[i].GoodID] = cells[i].CapShare
		if cells[i].GoodID == goodID {
			target = &cells[i]
		}
	}
	if target == nil {
		return nil, ErrDeliveryStorageCellMissing
	}
	size, err := r.deliveryStorageSize(tx, authorType, authorID)
	if err != nil {
		return nil, err
	}
	cap := settlement.StorageCaps(size, weights)[goodID]
	allowedCap := deliveryCellDoubleThreshold*cap - target.Amount
	if allowedCap <= deliveryEpsilon {
		return nil, ErrDeliveryStorageFull
	}

	// 6. Объём сдачи: min(трюм, остаток требования, 2·cap − amount), целые единицы.
	candidate := have
	if float64(remaining) < candidate {
		candidate = float64(remaining)
	}
	if allowedCap < candidate {
		candidate = allowedCap
	}
	delivered := int64(math.Floor(candidate + deliveryEpsilon))
	if delivered <= 0 {
		return nil, ErrDeliveryNothing
	}

	// 7. Списание ровно delivered из трюма (fail-closed на рассинхроне).
	taken, err := r.cargo.TakeCargoTx(tx, executorID, goodID, float64(delivered))
	if err != nil {
		return nil, err
	}
	if math.Abs(taken-float64(delivered)) > deliveryEpsilon {
		return nil, fmt.Errorf("deliver: рассинхрон трюма: снято %v, ожидалось %d", taken, delivered)
	}

	// 8. Приход в ячейку (единицы) — существующий относительный инкремент.
	if err := cellsRepo.IncrementStorageCellTx(context.Background(), tx, authorType, authorID, goodID, float64(delivered)); err != nil {
		if errors.Is(err, ErrStorageCellNotFound) {
			return nil, ErrDeliveryStorageCellMissing
		}
		return nil, err
	}

	execActor := models.ContractExecutorPlayer
	newRemaining := remaining - delivered
	var paid int64
	if newRemaining == 0 {
		// 10. Полная сдача: требование обнуляется, существующий completeTx
		// выпускает весь остаток залога и переводит в completed.
		if _, err := tx.Exec(deliveryRequirementZeroSQL, reqID); err != nil {
			return nil, fmt.Errorf("deliver: обнуление требования: %w", err)
		}
		ok, err := completeTx(tx, contractID, executorID, now)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrDeliveryNotAvailable
		}
		paid = escrowAmount
	} else {
		// 11. Частичная сдача: пропорциональная выплата из остатка залога.
		paid = escrowAmount * delivered / remaining
		newEscrow := escrowAmount - paid
		newWithdrawable := int64(0)
		if escrowAmount > 0 {
			newWithdrawable = escrowWithdrawable * newEscrow / escrowAmount
		}
		if _, err := tx.Exec(deliveryEscrowUpdateSQL, contractID, newEscrow, newWithdrawable); err != nil {
			return nil, fmt.Errorf("deliver: остаток залога: %w", err)
		}
		if _, err := tx.Exec(deliveryRequirementDecrementSQL, reqID, delivered); err != nil {
			return nil, fmt.Errorf("deliver: уменьшение требования: %w", err)
		}
		if paid > 0 {
			ownerType, ownerID, err := resolveExecutorAccount(execActor, executorID)
			if err != nil {
				return nil, err
			}
			if err := ensureAccount(tx, ownerType, ownerID, 0); err != nil {
				return nil, err
			}
			wDelta := int64(0)
			kind := models.MoneyOpEscrowRelease
			if funding == models.ContractFundingContractWork {
				wDelta = paid
				kind = models.MoneyOpContractWork
			}
			var balanceAfter int64
			if err := tx.QueryRow(releaseEscrowSQL, ownerType, ownerID, paid, wDelta).
				Scan(&balanceAfter); err != nil {
				return nil, fmt.Errorf("deliver: выплата: %w", err)
			}
			if err := insertMoneyOp(tx, ownerType, ownerID, paid, balanceAfter, kind, contractID, now); err != nil {
				return nil, err
			}
		}
	}

	// 9. Лог delivered всегда — фактически принятое/остаток/выплата/товар.
	if err := insertContractLog(tx, contractID, models.ContractLogDelivered, &execActor, &executorID,
		map[string]interface{}{
			"delivered": delivered,
			"remaining": newRemaining,
			"paid":      paid,
			"good_id":   goodID,
		}, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	status := models.ContractStatusTaken
	if newRemaining == 0 {
		status = models.ContractStatusCompleted
	}
	return &DeliveryResult{
		Status:     status,
		ContractID: contractID,
		Delivered:  delivered,
		Remaining:  newRemaining,
		Paid:       paid,
		GoodID:     goodID,
	}, nil
}

// deliveryStorageSize — размер хранилища автора (§5.2 п.5). Пишет owner-проход
// (settlements.storage_size). Автор-строение сюда не доходит: отказ выше
// (ErrDeliveryBuildingUnsupported, задел §0 п.15). Иной автор —
// ErrDeliveryAuthorUnsupported.
func (r *ContractRepository) deliveryStorageSize(q querier, authorType, authorID string) (float64, error) {
	switch authorType {
	case models.ContractActorSettlement:
		var size float64
		err := q.QueryRow(deliveryStorageSizeSQL, authorID).Scan(&size)
		if err == sql.ErrNoRows {
			return 0, ErrDeliveryStorageCellMissing
		}
		if err != nil {
			return 0, fmt.Errorf("deliver: размер хранилища: %w", err)
		}
		return size, nil
	default:
		return 0, ErrDeliveryAuthorUnsupported
	}
}
