// internal/repository/contract_board_repository.go
//
// Ленивая материализация доски планеты (спека
// 2026-09-23-контракт-ленивая-доска-пакет-и-снабжение §3): чтение доски
// превращает нужды планеты в открытые доли-контракты по кадэнсу — одной
// транзакцией, идемпотентно и детерминированно. Единый порядок захвата
// блокировок — contract_board_state → contracts → accounts (§3.3), поэтому
// публикация доли идёт contracts (INSERT) → accounts, обратно хендлерному
// Publish (инверсия названа осознанно, не «исправлять»).
package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"zorion/internal/contracts"
	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

// BoardNeed — минимальное описание нужды планеты (§5.1): из неё чистыми
// функциями internal/contracts строится целевой набор долей пакета.
//
// Источник нужд в проде — реальный (ячейки внутреннего хранилища поселений
// планеты, §1.6/§7 п.3); шов boardNeeds остаётся для тестов. НЕ добавлять
// запрос к buildings.data (колонки нет — упадёт рантаймом).
type BoardNeed struct {
	AuthorType   string        // тип автора-заказчика (contracts.author_type)
	AuthorID     string        // id автора-заказчика (поселение/постройка)
	PlanetID     string        // планета застройки (= доска и publication_planet_id)
	GoodID       string        // позиция нужды (contract_requirements.subject)
	Target       float64       // целевой уровень буфера
	Actual       float64       // текущий физический остаток
	Capacities   []float64     // вместимости трюмов в единицах товара
	BaseReward   float64       // объявленная цена за единицу (основа награды)
	WindowPreset time.Duration // пресет окна владельца (§5.5, F4-A)
}

// boardNeedsFunc — источник нужд планеты (шов §5.1, чтобы обеспечить
// тестируемость). nil = реальный источник (ячейки хранилища).
type boardNeedsFunc func(planetID string) ([]BoardNeed, error)

// boardNeedBaseReward — заглушка цены заказа (эталон цены — ЧК2в, §9):
// ненулевая, чтобы обычный платный путь был задействован.
const boardNeedBaseReward = 1.0

// boardNeedsSQL — нужды планеты из ячеек хранилища владельческих поселений
// (§1.6/§7 п.3): ownerless-поселения исключены (N1/§18 п.28). Одна строка на
// (поселение, товар); storage_size — размер хранилища поселения.
const boardNeedsSQL = `
	SELECT s.id, s.storage_size, c.good_id, c.amount, c.cap_share
	FROM settlements s
	JOIN settlement_storage_cells c
	  ON c.owner_type = 'settlement' AND c.owner_id = s.id
	WHERE s.planet_id = $1 AND s.owner_id IS NOT NULL
	ORDER BY s.id, c.good_id`

// supplyShareTitle — заголовок публикуемой доли (у доли нет своего названия).
const supplyShareTitle = "Снабжение"

// Точка сериализации и чек-точка доски (§3.3): INSERT первым — SELECT ... FOR
// UPDATE несуществующую строку не блокирует, потому признак «строки не было»
// берётся из RowsAffected INSERT'а.
const (
	upsertBoardStateSQL = `
		INSERT INTO contract_board_state (planet_id, materialized_at, updated_at)
		VALUES ($1, NOW(), NOW())
		ON CONFLICT (planet_id) DO NOTHING`

	lockBoardStateSQL = `
		SELECT materialized_at FROM contract_board_state WHERE planet_id = $1 FOR UPDATE`

	touchBoardStateSQL = `
		UPDATE contract_board_state SET materialized_at = $2, updated_at = $2 WHERE planet_id = $1`
)

// Открытые доли пакетов ПЛАНЕТЫ (§3.3 шаг 4: «текущие открытые доли этой
// планеты»): объём доли — contract_requirements.quantity (kind='goods', «один
// факт — одно место»). Фильтр status='open' — взятые и закрытые доли сверка не
// трогает никогда (§4.4). package_key IS NOT NULL — контракты вне пакета
// (перелёты, ручные публикации) не «лишние доли пакета» и в сверку не входят.
// Читаем по планете, а не по ключам нужд: пакет, выпавший из источника нужд,
// обязан сверяться и схлопываться (§4.4), иначе его доли висят до истечения.
const openSharesSQL = `
		SELECT c.id, c.package_key, c.share_index,
		       COALESCE((SELECT r.quantity FROM contract_requirements r
		                  WHERE r.contract_id = c.id AND r.kind = 'goods'
		                  ORDER BY r.pos LIMIT 1), 0)
		FROM contracts c
		WHERE c.status = 'open' AND c.publication_planet_id = $1
		  AND c.package_key IS NOT NULL
		ORDER BY c.package_key, c.share_index`

// Взятые доли пакетов планеты (§1.7/§5.3, N5): при непустых taken по ключу
// целевой набор долей пакета = пусто — новые не публикуются, существующие
// открытые снимаются сверкой. Читается по планете, сгруппировано по package_key.
const takenPackageKeysSQL = `
	SELECT DISTINCT package_key FROM contracts
	WHERE status = 'taken' AND publication_planet_id = $1 AND package_key IS NOT NULL`

// Снятие лишних/устаревших открытых долей: flip open→cancelled (contracts),
// затем возврат залога автору (accounts) — форма Cancel/sweepEscrow, причина
// лога 'superseded' (§3.3/§4.4).
const cancelSharesSQL = `
		UPDATE contracts SET status = 'cancelled', updated_at = NOW()
		WHERE id = ANY($1) AND status = 'open'
		RETURNING id, author_type, author_id, executor_id, escrow_amount, escrow_withdrawable`

// Публикация доли: contracts (INSERT) → accounts (залог). escrow_withdrawable
// выводится только из списания, поэтому дописывается сразу после него
// (INSERT идёт первым — порядок §3.3).
const insertShareContractSQL = `
		INSERT INTO contracts (
			id, type, author_type, author_id, publication_planet_id, title, description,
			payload, reward, funding, escrow_amount, escrow_withdrawable, escrow_kind,
			status, visibility, direct_target_type, direct_target_id, package_key, share_index,
			expires_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`

const setShareWithdrawableSQL = `
		UPDATE contracts SET escrow_withdrawable = $2 WHERE id = $1`

// payerBalanceSQL — читающая предварительная проверка баланса плательщика ДО
// INSERT (§5.5): не покрывает награду → free-mode. Счёт уже гарантирован
// ensurePayerAccount.
const payerBalanceSQL = `
		SELECT balance FROM accounts WHERE owner_type = $1 AND owner_id = $2`

// setShareFreeModeSQL — перевод уже вставленной доли в free-mode при гонке
// (баланс ушёл между чтением и локом, §5.5): эскроу не заперт, счёт не тронут.
const setShareFreeModeSQL = `
		UPDATE contracts SET reward = 0, escrow_amount = 0, updated_at = NOW() WHERE id = $1`

// MaterializeBoard — ленивая материализация доски планеты (§3.3): одна
// транзакция. Возвращает true, если проход выполнен, false — если работа не
// требовалась (нужд нет либо кадэнс не истёк).
func (r *ContractRepository) MaterializeBoard(planetID string, now time.Time) (bool, error) {
	// 1. Нужды — до блокировок. Нужд нет → без записи, без upsert, без локов
	// (§3.1/§3.3, спарсенность; T8).
	needs, err := r.boardNeedsFor(planetID)
	if err != nil {
		return false, err
	}
	if len(needs) == 0 {
		return false, nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// 2. Точка сериализации.
	res, err := tx.Exec(upsertBoardStateSQL, planetID)
	if err != nil {
		return false, fmt.Errorf("materialize board: upsert state: %w", err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	var materializedAt time.Time
	if err := tx.QueryRow(lockBoardStateSQL, planetID).Scan(&materializedAt); err != nil {
		return false, fmt.Errorf("materialize board: lock state: %w", err)
	}

	// 3. Повторная проверка кадэнса — только для существовавшей строки
	// (double-checked locking). Строки не было → первая материализация
	// безусловна: DEFAULT NOW() её не блокирует (T15).
	if inserted == 0 && now.Sub(materializedAt) < settlement.MinPersistInterval {
		return false, nil
	}

	pkgs := buildBoardPackages(needs)

	// 4. Текущие открытые доли пакетов планеты (по планете, не по ключам нужд).
	current, err := loadOpenShares(tx, planetID)
	if err != nil {
		return false, err
	}
	// Взятые доли планеты (§1.7/§5.3, N5): по ключу с непустыми taken цель = 0.
	takenKeys, err := loadTakenPackageKeys(tx, planetID)
	if err != nil {
		return false, err
	}

	// 5. Сверка (§4.4): лишние — отмена, недостающие — публикация. Порядок
	// захвата внутри обоих шагов contracts → accounts. Обход — по объединению
	// ключей (целевые пакеты нужд + все ключи открытых долей планеты), в
	// детерминированном порядке: ключ без нужды даёт пустой целевой набор →
	// все его открытые доли снимаются. Ключ с взятой долей тоже даёт пустой
	// целевой набор (N5): новых долей нет, открытые снимаются.
	for _, key := range reconcileKeys(pkgs, current) {
		var target []contracts.Share
		var need BoardNeed
		if !takenKeys[key] {
			for _, pkg := range pkgs {
				if pkg.key == key {
					target, need = pkg.shares, pkg.need
					break
				}
			}
		}
		cancelIDs, publish := reconcileShares(target, current[key])
		if len(cancelIDs) > 0 {
			if err := cancelBoardShares(tx, cancelIDs); err != nil {
				return false, err
			}
		}
		for _, share := range publish {
			if err := publishBoardShare(tx, need, key, share, now); err != nil {
				// «Мягкие» ошибки одной нужды (резолв плательщика, цена,
				// нехватка средств) не роняют публикацию остальных (N1/T2a):
				// иначе единичный сбой = молчаливая блокада доски всей планеты.
				if isSoftBoardPublishError(err) {
					log.Printf("materialize board: пропуск доли пакета %s: %v", key, err)
					continue
				}
				return false, err
			}
		}
	}

	// 6. Чек-точка.
	if _, err := tx.Exec(touchBoardStateSQL, planetID, now); err != nil {
		return false, fmt.Errorf("materialize board: touch state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// boardNeedsFor — нужды планеты: инъектированный шов (тесты) либо реальный
// источник — ячейки хранилища владельческих поселений планеты (§1.6/§7 п.3).
func (r *ContractRepository) boardNeedsFor(planetID string) ([]BoardNeed, error) {
	if r.boardNeeds != nil {
		return r.boardNeeds(planetID)
	}
	return r.loadBoardNeeds(planetID)
}

// loadBoardNeeds — реальный источник нужд (§1.6/§7 п.3): для каждого
// владельческого поселения планеты и каждой его ячейки строится BoardNeed.
// Target — порог ячейки (StorageCaps: size × cap_share/Σcap_share), Actual —
// количество. Capacities = nil: нарезка под трюмы — гипотеза, калибровка
// ЧК2в/@balancetester (новая сущность/запрос не вводится). WindowPreset = 0 —
// существующий дефолт окна (SupplyOfferWindowDefault = 24ч).
func (r *ContractRepository) loadBoardNeeds(planetID string) ([]BoardNeed, error) {
	rows, err := r.db.Query(boardNeedsSQL, planetID)
	if err != nil {
		return nil, fmt.Errorf("board needs: %w", err)
	}
	defer rows.Close()

	type cellRow struct {
		goodID   int64
		amount   float64
		capShare float64
	}
	type settRow struct {
		id    string
		size  float64
		cells []cellRow
	}
	bySett := map[string]*settRow{}
	var order []string
	for rows.Next() {
		var sid string
		var size float64
		var c cellRow
		if err := rows.Scan(&sid, &size, &c.goodID, &c.amount, &c.capShare); err != nil {
			return nil, err
		}
		s := bySett[sid]
		if s == nil {
			s = &settRow{id: sid, size: size}
			bySett[sid] = s
			order = append(order, sid)
		}
		s.cells = append(s.cells, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var needs []BoardNeed
	for _, sid := range order {
		s := bySett[sid]
		weights := make(map[int64]float64, len(s.cells))
		for _, c := range s.cells {
			weights[c.goodID] = c.capShare
		}
		caps := settlement.StorageCaps(s.size, weights)
		for _, c := range s.cells {
			needs = append(needs, BoardNeed{
				AuthorType:   models.ContractActorSettlement,
				AuthorID:     sid,
				PlanetID:     planetID,
				GoodID:       strconv.FormatInt(c.goodID, 10),
				Target:       caps[c.goodID],
				Actual:       c.amount,
				Capacities:   nil,
				BaseReward:   boardNeedBaseReward,
				WindowPreset: 0,
			})
		}
	}
	return needs, nil
}

// boardPackage — одна нужда, приведённая к пакету (ключ группировки + целевые
// доли). Нужды уникальны по ключу (автор + позиция на планете).
type boardPackage struct {
	key    string
	need   BoardNeed
	shares []contracts.Share
}

// buildBoardPackages — пакеты нужд. Нужды сворачиваются по ключу пакета
// (детерминированно: первая запись побеждает): дубль от датчика — дефект
// источника, а не «суммировать»; без сворачивания дубль дал бы двойную
// публикацию одного (package_key, share_index) → нарушение частичного
// уникального индекса → откат всей tx.
func buildBoardPackages(needs []BoardNeed) []boardPackage {
	out := make([]boardPackage, 0, len(needs))
	seen := make(map[string]bool, len(needs))
	for _, n := range needs {
		key := supplyPackageKey(n.PlanetID, n.AuthorID, n.GoodID)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, boardPackage{
			key:    key,
			need:   n,
			shares: targetShares(n),
		})
	}
	return out
}

// reconcileKeys — ключи пакетов для сверки: объединение целевых ключей нужд и
// ключей текущих открытых долей планеты, в детерминированном порядке
// (сортировка — поведение не зависит от порядка строк SELECT).
func reconcileKeys(pkgs []boardPackage, current map[string]map[int]openShare) []string {
	seen := make(map[string]bool, len(pkgs)+len(current))
	keys := make([]string, 0, len(pkgs)+len(current))
	for _, pkg := range pkgs {
		if !seen[pkg.key] {
			seen[pkg.key] = true
			keys = append(keys, pkg.key)
		}
	}
	for key := range current {
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// supplyPackageKey — ключ пакета (§4.2): <kind>:<planet_id>:<author_id>:<position>.
func supplyPackageKey(planetID, authorID, goodID string) string {
	return models.ContractNeedKindSupply + ":" + planetID + ":" + authorID + ":" + goodID
}

// targetShares — целевой набор долей нужды: чистая функция (§3.3/§5.3), без RNG
// и без now. Округление вниз до целого — внутри SliceShares.
func targetShares(need BoardNeed) []contracts.Share {
	return contracts.SliceShares(contracts.Deficit(need.Target, need.Actual), need.Capacities)
}

// openShare — текущая открытая доля пакета.
type openShare struct {
	id    string
	index int
	qty   int64
}

// loadOpenShares — текущие открытые доли пакетов планеты (по publication_planet_id).
func loadOpenShares(q querier, planetID string) (map[string]map[int]openShare, error) {
	out := map[string]map[int]openShare{}
	rows, err := q.Query(openSharesSQL, planetID)
	if err != nil {
		return nil, fmt.Errorf("materialize board: open shares: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, key string
		var index sql.NullInt64
		var qty int64
		if err := rows.Scan(&id, &key, &index, &qty); err != nil {
			return nil, err
		}
		if out[key] == nil {
			out[key] = map[int]openShare{}
		}
		// package_key без share_index (ручная публикация) индексом не считается
		// (0 не совпадает с целевыми 1..N) → доля снимается сверкой.
		out[key][int(index.Int64)] = openShare{id: id, index: int(index.Int64), qty: qty}
	}
	return out, rows.Err()
}

// loadTakenPackageKeys — ключи пакетов планеты, у которых есть взятая доля
// (§1.7/§5.3, N5). Пустой набор — взятых долей нет.
func loadTakenPackageKeys(q querier, planetID string) (map[string]bool, error) {
	rows, err := q.Query(takenPackageKeysSQL, planetID)
	if err != nil {
		return nil, fmt.Errorf("materialize board: taken keys: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		out[key] = true
	}
	return out, rows.Err()
}

// isSoftBoardPublishError — «мягкая» ошибка публикации доли (N1/T2a): резолв
// плательщика, цена, нехватка средств. Такие ошибки логируются и пропускаются,
// не роняя публикацию остальных нужд. Неожиданные SQL-ошибки — «жёсткие»:
// возвращаются с откатом (порчу БД не маскируем).
func isSoftBoardPublishError(err error) bool {
	return errors.Is(err, ErrPayerUnresolved) ||
		errors.Is(err, ErrInvalidReward) ||
		errors.Is(err, ErrInsufficientFunds)
}

// reconcileShares сравнивает целевой набор долей с текущими открытыми долями
// пакета по ключу (share_index, объём) (§4.4): возвращает id долей на отмену и
// доли к публикации. Взятые/закрытые сюда не попадают (в current только
// status='open'). Порядок детерминирован (индексы по возрастанию).
func reconcileShares(target []contracts.Share, current map[int]openShare) (cancelIDs []string, publish []contracts.Share) {
	want := make(map[int]int64, len(target))
	for _, s := range target {
		want[s.Index] = s.Quantity
	}
	idxs := make([]int, 0, len(current))
	for i := range current {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for _, i := range idxs {
		if q, ok := want[i]; !ok || q != current[i].qty {
			cancelIDs = append(cancelIDs, current[i].id)
		}
	}
	for _, s := range target {
		if cur, ok := current[s.Index]; ok && cur.qty == s.Quantity {
			continue
		}
		publish = append(publish, s)
	}
	return cancelIDs, publish
}

// cancelBoardShares — снятие лишних открытых долей: flip contracts → возврат
// залога автору (accounts) с причиной лога 'superseded' (§3.3).
func cancelBoardShares(q querier, ids []string) error {
	_, err := sweepEscrow(q, cancelSharesSQL, []interface{}{pq.Array(ids)},
		models.ContractLogCancelled, models.EscrowReasonSuperseded)
	return err
}

// publishBoardShare — публикация недостающей доли пакета (образец Publish, но
// своим порядком §3.3): contracts (INSERT контракта/требования) → accounts
// (списание залога). Автор и actor лога — из need.AuthorType (§6); плательщик —
// владелец автора, планета публикации — планета застройки.
//
// Бесплатная публикация (§5.5, D5): развилка режима — читающая, ДО INSERT.
// reward > 0 и баланс не покрывает → free-mode (reward = 0, escrow_amount = 0);
// reward = 0 изначально → сразу free-mode. Строка вставляется уже с нужными
// reward/escrow_amount; эскроу/счёт в free-mode не трогаются.
func publishBoardShare(q querier, need BoardNeed, pkgKey string, share contracts.Share, now time.Time) error {
	reward := contracts.ShareReward(share.Quantity, need.BaseReward)
	if reward < 0 {
		return fmt.Errorf("%w: доля пакета %s", ErrInvalidReward, pkgKey)
	}
	authorType := need.AuthorType
	if authorType == "" {
		authorType = models.ContractActorBuilding
	}
	ownerType, ownerID, err := resolvePayerAccountQ(q, authorType, need.AuthorID)
	if err != nil {
		return err
	}
	if err := ensurePayerAccount(q, ownerType, ownerID); err != nil {
		return err
	}

	// Развилка режима ДО INSERT (§5.5): reward > 0 — предварительная проверка
	// баланса; не покрывает → free-mode. reward = 0 — сразу free-mode.
	paid := reward > 0
	if paid {
		var balance int64
		err := q.QueryRow(payerBalanceSQL, ownerType, ownerID).Scan(&balance)
		if err == sql.ErrNoRows {
			paid = false
		} else if err != nil {
			return fmt.Errorf("share balance check: %w", err)
		} else if balance < reward {
			paid = false
		}
	}
	escrowAmount := reward
	if !paid {
		reward = 0
		escrowAmount = 0
	}

	id := uuid.New().String()
	if _, err := q.Exec(insertShareContractSQL,
		id, models.ContractTypeSupply, authorType, need.AuthorID,
		need.PlanetID, supplyShareTitle, "", "{}", reward, models.ContractFundingRegular,
		escrowAmount, 0, models.EscrowKindDeposit, models.ContractStatusOpen,
		models.ContractVisibilityPublic, nil, nil, pkgKey, share.Index,
		now.Add(models.SupplyOfferWindow(need.WindowPreset)), now, now,
	); err != nil {
		return fmt.Errorf("insert share contract: %w", err)
	}
	if _, err := q.Exec(insertRequirementSQL, id, 1, "goods", need.GoodID, "in",
		nil, nil, share.Quantity); err != nil {
		return fmt.Errorf("insert share requirement: %w", err)
	}

	actorType, actorID := authorType, need.AuthorID
	if err := insertContractLog(q, id, models.ContractLogPublished, &actorType, &actorID,
		map[string]interface{}{}, now); err != nil {
		return err
	}
	if !paid {
		return nil
	}

	var balanceAfter, withdrawableAfter, escrowWithdrawable int64
	err = q.QueryRow(lockEscrowSQL, ownerType, ownerID, reward).
		Scan(&balanceAfter, &withdrawableAfter, &escrowWithdrawable)
	if err == sql.ErrNoRows {
		// Гонка: баланс ушёл между чтением и локом (§5.5). Публикуем free-mode:
		// эскроу не заперт, счёт не трогаем; строку переводим в reward = 0.
		if _, uerr := q.Exec(setShareFreeModeSQL, id); uerr != nil {
			return fmt.Errorf("share free-mode fallback: %w", uerr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("share escrow lock: %w", err)
	}
	_ = withdrawableAfter
	if _, err := q.Exec(setShareWithdrawableSQL, id, escrowWithdrawable); err != nil {
		return fmt.Errorf("set share withdrawable: %w", err)
	}
	if err := insertContractLog(q, id, models.ContractLogEscrowLocked, &actorType, &actorID,
		map[string]interface{}{"amount": reward, "withdrawable": escrowWithdrawable}, now); err != nil {
		return err
	}
	return insertMoneyOp(q, ownerType, ownerID, -reward, balanceAfter,
		models.MoneyOpEscrowLock, id, now)
}
