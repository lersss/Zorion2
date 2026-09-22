// internal/repository/branch_repository.go
//
// Ветки поселения (спека 2026-09-22-поселение-ветка-буферы-переработка §3.1/
// §4/§9): таблица-носитель буферов settlement_branch_buffers, чтение веток и
// буферов по поселению (`= ANY($1)`), состав рецепта, ленивый синк переработки
// (tx + FOR UPDATE на строке ветки), создание ветки с посевом буферов,
// админский инкремент входа и счётчик веток по товару-выходу (§8/T14-О3).
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/lib/pq"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

// ErrBranchNotFound — ветки нет (админ-ручки отдают 404).
var ErrBranchNotFound = errors.New("ветка не найдена")

const (
	branchSelectBySettlementsSQL = `
		SELECT b.id, b.settlement_id, b.recipe_id, b.processed_at,
		       r.good_id, og.name, r.complexity
		FROM settlement_branches b
		JOIN recipes r ON r.id = b.recipe_id
		JOIN goods og ON og.id = r.good_id
		WHERE b.settlement_id = ANY($1)
		ORDER BY b.created_at ASC, b.id ASC`

	// branchSelectByIDSQL — чтение одной ветки под блокировкой (§4.2: единая
	// точка сериализации — FOR UPDATE только на строке ветки, порядок ветка →
	// буферы). `FOR UPDATE OF b` (не голый FOR UPDATE): запрос джойнит recipes/
	// goods, и голый FOR UPDATE залочил бы и их строки — лишние блокировки и
	// риск дедлока с DeleteGood (goods → recipe → branch). Лочим ровно ветку.
	branchSelectByIDSQL = `
		SELECT b.id, b.settlement_id, b.recipe_id, b.processed_at,
		       r.good_id, og.name, r.complexity
		FROM settlement_branches b
		JOIN recipes r ON r.id = b.recipe_id
		JOIN goods og ON og.id = r.good_id
		WHERE b.id = $1 FOR UPDATE OF b`

	branchBuffersSelectSQL = `
		SELECT bb.branch_id, bb.direction, bb.good_id, g.name, bb.amount
		FROM settlement_branch_buffers bb
		JOIN goods g ON g.id = bb.good_id
		WHERE bb.branch_id = ANY($1)
		ORDER BY bb.branch_id, bb.direction, bb.good_id`

	branchComponentsSelectSQL = `
		SELECT rc.recipe_id, rc.component_id, rc.quantity
		FROM recipe_components rc
		WHERE rc.recipe_id = ANY($1) AND rc.component_id IS NOT NULL
		ORDER BY rc.recipe_id, rc.pos`

	// depositExtractionSelectSQL — залежи планеты под блокировкой для добычи
	// (спека итерации 3 §4.4): фильтр по ресурсам рецепта и запасу > 0; порядок
	// строго по `id` (детерминированный порядок локов — дедлоков нет между
	// синками), выбор «от крупной к мелкой» делает чистая функция (§3.2).
	depositExtractionSelectSQL = `
		SELECT id, good_id, amount FROM deposits
		WHERE planet_id = $1 AND good_id = ANY($2) AND amount > 0
		ORDER BY id
		FOR UPDATE`

	// depositMemorySelectSQL — залежи для пути «в памяти» (Δt <
	// MinPersistInterval): только чтение, без блокировки и записи (§4.2).
	depositMemorySelectSQL = `
		SELECT planet_id, id, good_id, amount FROM deposits
		WHERE planet_id = ANY($1) AND good_id = ANY($2) AND amount > 0
		ORDER BY planet_id, id`

	// depositWriteAmountSQL — абсолютная запись остатка залежи (кламп ≥ 0 —
	// страховка БД CHECK (amount >= 0); штатно не срабатывает, §4.4).
	depositWriteAmountSQL = `
		UPDATE deposits SET amount = $1, updated_at = NOW() WHERE id = $2`
)

// countBranchesByGoodSQL — число веток, у которых товар-выход рецепта = good_id
// (§8/О3, предупреждение студии при удалении товара — образец deposits-count).
const countBranchesByGoodSQL = `
	SELECT COUNT(*) FROM settlement_branches b
	JOIN recipes r ON r.id = b.recipe_id
	WHERE r.good_id = $1`

// branchRowsQueryer — общее для *sql.DB и *sql.Tx (чтение веток/буферов).
type branchRowsQueryer interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// BranchRepository — доступ к веткам поселения и их буферам.
type BranchRepository struct {
	db *sql.DB
}

func NewBranchRepository(db *sql.DB) *BranchRepository {
	return &BranchRepository{db: db}
}

// branchRecord — загруженная ветка: display-модель + служебные данные для
// переработки (состав рецепта, выход, имена буферов).
type branchRecord struct {
	branch       models.SettlementBranch
	settlementID string
	outputGoodID int64
	components   []settlement.BranchComponent
}

// toBranch — состояние ветки для чистой функции переработки. deposits —
// залежи своей планеты по good_id (источник добычи, спека итерации 3 §4).
func (rec *branchRecord) toBranch(population float64, deposits map[int64][]settlement.DepositLot) settlement.Branch {
	input := make(map[int64]float64, len(rec.branch.Input))
	for _, e := range rec.branch.Input {
		input[e.GoodID] = e.Amount
	}
	var complexity *int
	if rec.branch.Complexity > 0 {
		c := rec.branch.Complexity
		complexity = &c
	}
	return settlement.Branch{
		Population:  population,
		Complexity:  complexity,
		Components:  rec.components,
		Input:       input,
		Output:      branchOutputAmount(rec.branch.Output, rec.outputGoodID),
		ProcessedAt: rec.branch.ProcessedAt,
		Deposits:    deposits,
	}
}

// applyProcessed — вынести результат переработки в display-модель. Новых строк
// не создаёт (путь «в памяти» записей в БД не делает — §4.2/T14): обновляются
// только уже загруженные записи.
func (rec *branchRecord) applyProcessed(p settlement.Branch) {
	for i := range rec.branch.Input {
		if v, ok := p.Input[rec.branch.Input[i].GoodID]; ok {
			rec.branch.Input[i].Amount = v
		}
	}
	for i := range rec.branch.Output {
		if rec.branch.Output[i].GoodID == rec.outputGoodID {
			rec.branch.Output[i].Amount = p.Output
		}
	}
	rec.branch.ProcessedAt = p.ProcessedAt
}

// branchOutputAmount — накопленное количество товара-выхода рецепта.
func branchOutputAmount(entries []models.BranchBufferEntry, goodID int64) float64 {
	for _, e := range entries {
		if e.GoodID == goodID {
			return e.Amount
		}
	}
	return 0
}

// --- чтение ---

// GetBranchesBySettlementIDs — ветки поселений, сгруппированные по settlement_id
// (без синка — для чтения в тестах/служебных путей). Пустой вход — пустая карта.
func (r *BranchRepository) GetBranchesBySettlementIDs(ids []string) (map[string][]models.SettlementBranch, error) {
	recs, err := loadBranches(context.Background(), r.db, ids)
	if err != nil {
		return nil, err
	}
	return branchesBySettlement(recs), nil
}

// GetBranchesBySettlementIDsTx — то же внутри транзакции (админ-ручки: чтение
// блока веток после записи в одной tx).
func (r *BranchRepository) GetBranchesBySettlementIDsTx(ctx context.Context, tx *sql.Tx, ids []string) (map[string][]models.SettlementBranch, error) {
	recs, err := loadBranches(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	return branchesBySettlement(recs), nil
}

// branchesBySettlement — группировка записей по поселению с конвертацией в
// display-модель.
func branchesBySettlement(recs []*branchRecord) map[string][]models.SettlementBranch {
	out := map[string][]models.SettlementBranch{}
	for _, rec := range recs {
		out[rec.settlementID] = append(out[rec.settlementID], rec.branch)
	}
	return out
}

// loadBranches — ветки поселений + состав рецептов + буферы (три запроса).
func loadBranches(ctx context.Context, q branchRowsQueryer, settlementIDs []string) ([]*branchRecord, error) {
	if len(settlementIDs) == 0 {
		return nil, nil
	}
	rows, err := q.QueryContext(ctx, branchSelectBySettlementsSQL, pqStringArray(settlementIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query branches: %w", err)
	}
	var recs []*branchRecord
	var recipeIDs []int64
	var branchIDs []string
	for rows.Next() {
		rec, err := scanBranchRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		recs = append(recs, rec)
		recipeIDs = appendUniqueInt64(recipeIDs, rec.branch.RecipeID)
		branchIDs = append(branchIDs, rec.branch.ID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("branches iteration error: %w", err)
	}
	rows.Close()
	if len(recs) == 0 {
		return nil, nil
	}

	comps, err := loadBranchComponents(ctx, q, recipeIDs)
	if err != nil {
		return nil, err
	}
	for _, rec := range recs {
		rec.components = comps[rec.branch.RecipeID]
	}
	if err := attachBranchBuffers(ctx, q, branchIDs, recs); err != nil {
		return nil, err
	}
	return recs, nil
}

// scanBranchRow — общий разбор строки ветки (b.id, b.settlement_id, b.recipe_id,
// b.processed_at, output good_id, output good name, complexity).
func scanBranchRow(rows *sql.Rows) (*branchRecord, error) {
	rec := &branchRecord{}
	var complexity sql.NullInt64
	if err := rows.Scan(
		&rec.branch.ID, &rec.settlementID, &rec.branch.RecipeID, &rec.branch.ProcessedAt,
		&rec.outputGoodID, &rec.branch.RecipeName, &complexity,
	); err != nil {
		return nil, fmt.Errorf("failed to scan branch: %w", err)
	}
	if complexity.Valid {
		rec.branch.Complexity = int(complexity.Int64)
	}
	return rec, nil
}

// loadBranchComponents — заполненные компоненты рецептов (recipe_id → компоненты).
func loadBranchComponents(ctx context.Context, q branchRowsQueryer, recipeIDs []int64) (map[int64][]settlement.BranchComponent, error) {
	out := map[int64][]settlement.BranchComponent{}
	if len(recipeIDs) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, branchComponentsSelectSQL, pq.Array(recipeIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query recipe components: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var recipeID, componentID int64
		var quantity int
		if err := rows.Scan(&recipeID, &componentID, &quantity); err != nil {
			return nil, fmt.Errorf("failed to scan recipe component: %w", err)
		}
		out[recipeID] = append(out[recipeID], settlement.BranchComponent{GoodID: componentID, Quantity: quantity})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("components iteration error: %w", err)
	}
	return out, nil
}

// attachBranchBuffers — буферы веток в display-модель (вход/выход раздельно).
func attachBranchBuffers(ctx context.Context, q branchRowsQueryer, branchIDs []string, recs []*branchRecord) error {
	if len(branchIDs) == 0 {
		return nil
	}
	byID := make(map[string]*branchRecord, len(recs))
	for _, rec := range recs {
		byID[rec.branch.ID] = rec
	}
	rows, err := q.QueryContext(ctx, branchBuffersSelectSQL, pqStringArray(branchIDs))
	if err != nil {
		return fmt.Errorf("failed to query branch buffers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var branchID, direction, name string
		var amount float64
		var gid int64
		if err := rows.Scan(&branchID, &direction, &gid, &name, &amount); err != nil {
			return fmt.Errorf("failed to scan branch buffer: %w", err)
		}
		rec := byID[branchID]
		if rec == nil {
			continue
		}
		e := models.BranchBufferEntry{GoodID: gid, GoodName: name, Amount: amount}
		if direction == "input" {
			rec.branch.Input = append(rec.branch.Input, e)
		} else {
			rec.branch.Output = append(rec.branch.Output, e)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("buffers iteration error: %w", err)
	}
	return nil
}

// --- ленивый синк переработки (§4.2) ---

// SyncBranches — загрузка веток поселений и ленивая переработка вход → выход
// (§4.2) с добором недостающего из залежей своей планеты (спека итерации 3 §4):
// добыча — первая половина того же прохода. Для каждой ветки: Δt <
// MinPersistInterval (общий кадэнс ленивых петель) — пересчёт в памяти без
// записи и блокировки (и залежи читаются без блокировки, deposits не пишется);
// иначе — персистентный синк (syncBranchTx: FOR UPDATE ветки → FOR UPDATE
// залежей → запись буферов, залежей и processed_at). planetBySettlement даёт
// planet_id каждого поселения (без нового JOIN-лока, §4.2). Возвращает
// display-модели, сгруппированные по settlement_id.
func (r *BranchRepository) SyncBranches(settlementIDs []string, populationBySettlement map[string]float64, planetBySettlement map[string]string, now time.Time) (map[string][]models.SettlementBranch, error) {
	out := map[string][]models.SettlementBranch{}
	if len(settlementIDs) == 0 {
		return out, nil
	}
	recs, err := loadBranches(context.Background(), r.db, settlementIDs)
	if err != nil {
		return nil, err
	}
	// Залежи для веток пути «в памяти» — читаются один раз без блокировки
	// (§4.2): в БД этот путь не пишет, но карточка показывает добор в памяти.
	memDeposits, err := r.loadMemoryDeposits(recs, planetBySettlement, now)
	if err != nil {
		return nil, err
	}
	for _, rec := range recs {
		population := populationBySettlement[rec.settlementID]
		planetID := planetBySettlement[rec.settlementID]
		if now.Sub(rec.branch.ProcessedAt) < settlement.MinPersistInterval {
			rec.applyProcessed(settlement.ProcessBranch(rec.toBranch(population, memDeposits[planetID]), now))
		} else {
			updated, err := r.syncBranchTx(context.Background(), rec.branch.ID, planetID, population, now)
			// Ветку удалили между загрузкой и блокировкой (чужая мутация/
			// каскад) — не роняем чтение карточки: ветки больше нет, no-op.
			if errors.Is(err, ErrBranchNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			rec.branch = updated
		}
		out[rec.settlementID] = append(out[rec.settlementID], rec.branch)
	}
	return out, nil
}

// componentGoodIDs — good_id заполненных компонентов рецепта без дублей (фильтр
// залежей по составу рецепта, §3.2).
func componentGoodIDs(comps []settlement.BranchComponent) []int64 {
	var out []int64
	for _, c := range comps {
		out = appendUniqueInt64(out, c.GoodID)
	}
	return out
}

// loadMemoryDeposits — залежи планет веток пути «в памяти» (Δt <
// MinPersistInterval), одним запросом без блокировки (§4.2, deposits не
// пишется). Запрос — только если такие ветки есть и у них есть компоненты;
// возвращает planet_id → good_id → залежи.
func (r *BranchRepository) loadMemoryDeposits(recs []*branchRecord, planetBySettlement map[string]string, now time.Time) (map[string]map[int64][]settlement.DepositLot, error) {
	var planetIDs []string
	var goodIDs []int64
	for _, rec := range recs {
		if now.Sub(rec.branch.ProcessedAt) >= settlement.MinPersistInterval {
			continue
		}
		planetID := planetBySettlement[rec.settlementID]
		if planetID == "" {
			continue
		}
		planetIDs = appendStringUnique(planetIDs, planetID)
		for _, gid := range componentGoodIDs(rec.components) {
			goodIDs = appendUniqueInt64(goodIDs, gid)
		}
	}
	if len(planetIDs) == 0 || len(goodIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(context.Background(), depositMemorySelectSQL, pqStringArray(planetIDs), pq.Array(goodIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query deposits: %w", err)
	}
	defer rows.Close()
	out := make(map[string]map[int64][]settlement.DepositLot, len(planetIDs))
	for rows.Next() {
		var planetID, lotID string
		var goodID int64
		var amount float64
		if err := rows.Scan(&planetID, &lotID, &goodID, &amount); err != nil {
			return nil, fmt.Errorf("failed to scan deposit: %w", err)
		}
		if out[planetID] == nil {
			out[planetID] = map[int64][]settlement.DepositLot{}
		}
		out[planetID][goodID] = append(out[planetID][goodID], settlement.DepositLot{ID: lotID, GoodID: goodID, Amount: amount})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deposits iteration error: %w", err)
	}
	return out, nil
}

// loadDepositsForUpdate — залежи планеты под блокировкой для персистентного
// синка (§4.4): порядок «ветка → залежи», `FOR UPDATE` + `ORDER BY id` —
// детерминированный порядок локов между синками (дедлоков нет). Фильтр —
// ресурсы рецепта и запас > 0 (нулевые не блокируются и не трогаются).
func loadDepositsForUpdate(ctx context.Context, tx *sql.Tx, planetID string, goodIDs []int64) (map[int64][]settlement.DepositLot, error) {
	if planetID == "" || len(goodIDs) == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, depositExtractionSelectSQL, planetID, pq.Array(goodIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to lock deposits: %w", err)
	}
	defer rows.Close()
	out := map[int64][]settlement.DepositLot{}
	for rows.Next() {
		var lotID string
		var goodID int64
		var amount float64
		if err := rows.Scan(&lotID, &goodID, &amount); err != nil {
			return nil, fmt.Errorf("failed to scan deposit: %w", err)
		}
		out[goodID] = append(out[goodID], settlement.DepositLot{ID: lotID, GoodID: goodID, Amount: amount})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deposits iteration error: %w", err)
	}
	return out, nil
}

// appendStringUnique — добавить значение в срез, если его ещё нет.
func appendStringUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// syncBranchTx — персистентный путь «события» (§4.2): транзакция, FOR UPDATE на
// строке ветки → топ-ап строк входа для текущих компонентов (T14) → FOR UPDATE
// залежей планеты (добыча, спека итерации 3 §4) → пересчёт по сохранённым
// буферам и залежам → абсолютная запись буферов, залежей и processed_at →
// COMMIT. Порядок блокировок единый: ветка → залежи.
func (r *BranchRepository) syncBranchTx(ctx context.Context, branchID, planetID string, population float64, now time.Time) (models.SettlementBranch, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return models.SettlementBranch{}, err
	}
	defer tx.Rollback()

	rec, err := loadBranchForUpdate(ctx, tx, branchID)
	if err != nil {
		return models.SettlementBranch{}, err
	}
	comps, err := loadBranchComponents(ctx, tx, []int64{rec.branch.RecipeID})
	if err != nil {
		return models.SettlementBranch{}, err
	}
	rec.components = comps[rec.branch.RecipeID]

	// Топ-ап строк входа для текущих заполненных компонентов (правка @critic №8,
	// T14): новый компонент рецепта без строки дал бы affordable = 0 — ветка
	// молча встала бы. ON CONFLICT DO NOTHING; появление строки — лог.
	for _, c := range rec.components {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount)
			VALUES ($1, 'input', $2, 0)
			ON CONFLICT (branch_id, direction, good_id) DO NOTHING`, branchID, c.GoodID)
		if err != nil {
			return models.SettlementBranch{}, fmt.Errorf("branch top-up input: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			log.Printf("🌿 ветка %s: новый компонент рецепта %d добавлен во вход (0)", branchID, c.GoodID)
		}
	}
	if err := attachBranchBuffers(ctx, tx, []string{branchID}, []*branchRecord{rec}); err != nil {
		return models.SettlementBranch{}, err
	}

	// Залежи планеты под блокировкой — после ветки (единый порядок локов).
	deposits, err := loadDepositsForUpdate(ctx, tx, planetID, componentGoodIDs(rec.components))
	if err != nil {
		return models.SettlementBranch{}, err
	}

	processed := settlement.ProcessBranch(rec.toBranch(population, deposits), now)

	// Абсолютная запись входа по текущим компонентам (под блокировкой ветки —
	// инкремент админа не теряется, §4.2).
	for _, c := range rec.components {
		if _, err := tx.ExecContext(ctx, `
			UPDATE settlement_branch_buffers SET amount = $1, updated_at = NOW()
			WHERE branch_id = $2 AND direction = 'input' AND good_id = $3`,
			processed.Input[c.GoodID], branchID, c.GoodID,
		); err != nil {
			return models.SettlementBranch{}, fmt.Errorf("branch write input: %w", err)
		}
	}
	// Выход — upsert строки товара-выхода рецепта.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount)
		VALUES ($1, 'output', $2, $3)
		ON CONFLICT (branch_id, direction, good_id)
		DO UPDATE SET amount = EXCLUDED.amount, updated_at = NOW()`,
		branchID, rec.outputGoodID, processed.Output,
	); err != nil {
		return models.SettlementBranch{}, fmt.Errorf("branch write output: %w", err)
	}
	// Убыль залежей — абсолютная запись остатка (кламп ≥ 0 страховкой).
	for _, lot := range sortedDepositLots(processed.Deposits) {
		amount := lot.Amount
		if amount < 0 {
			amount = 0
		}
		if _, err := tx.ExecContext(ctx, depositWriteAmountSQL, amount, lot.ID); err != nil {
			return models.SettlementBranch{}, fmt.Errorf("branch write deposit: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE settlement_branches SET processed_at = $1, updated_at = NOW() WHERE id = $2`,
		now, branchID,
	); err != nil {
		return models.SettlementBranch{}, fmt.Errorf("branch advance checkpoint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return models.SettlementBranch{}, err
	}

	rec.applyProcessed(processed)
	return rec.branch, nil
}

// sortedDepositLots — залежи результата в детерминированном порядке (good_id
// ASC, затем порядок среза — «от крупной к мелкой»): запись залежей в БД не
// зависит от порядка обхода map.
func sortedDepositLots(deposits map[int64][]settlement.DepositLot) []settlement.DepositLot {
	if len(deposits) == 0 {
		return nil
	}
	goodIDs := make([]int64, 0, len(deposits))
	for gid := range deposits {
		goodIDs = append(goodIDs, gid)
	}
	sort.Slice(goodIDs, func(i, j int) bool { return goodIDs[i] < goodIDs[j] })
	var out []settlement.DepositLot
	for _, gid := range goodIDs {
		out = append(out, deposits[gid]...)
	}
	return out
}

// loadBranchForUpdate — одна ветка под блокировкой строки (§4.2).
func loadBranchForUpdate(ctx context.Context, tx *sql.Tx, branchID string) (*branchRecord, error) {
	rec := &branchRecord{}
	var complexity sql.NullInt64
	err := tx.QueryRowContext(ctx, branchSelectByIDSQL, branchID).Scan(
		&rec.branch.ID, &rec.settlementID, &rec.branch.RecipeID, &rec.branch.ProcessedAt,
		&rec.outputGoodID, &rec.branch.RecipeName, &complexity,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBranchNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock branch: %w", err)
	}
	if complexity.Valid {
		rec.branch.Complexity = int(complexity.Int64)
	}
	return rec, nil
}

// --- запись (админ-ручки) ---

// CreateBranchTx — вставка ветки и посев буферов нулями: по строке input на
// каждый текущий заполненный компонент рецепта и одна строка output на
// товар-выход (§5). Компоненты, добавленные в рецепт позже, дополняются при
// персистентном синке (top-up, §4.2).
func (r *BranchRepository) CreateBranchTx(ctx context.Context, tx *sql.Tx, branchID, settlementID string, recipeID int64) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settlement_branches (id, settlement_id, recipe_id)
		VALUES ($1, $2, $3)`, branchID, settlementID, recipeID,
	); err != nil {
		return fmt.Errorf("failed to insert branch: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT component_id FROM recipe_components
		WHERE recipe_id = $1 AND component_id IS NOT NULL ORDER BY pos`, recipeID)
	if err != nil {
		return fmt.Errorf("failed to read recipe components: %w", err)
	}
	var comps []int64
	for rows.Next() {
		var gid int64
		if err := rows.Scan(&gid); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan recipe component: %w", err)
		}
		comps = append(comps, gid)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("components iteration error: %w", err)
	}
	rows.Close()

	for _, gid := range comps {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount)
			VALUES ($1, 'input', $2, 0) ON CONFLICT DO NOTHING`, branchID, gid,
		); err != nil {
			return fmt.Errorf("failed to seed input buffer: %w", err)
		}
	}
	var outputGoodID int64
	if err := tx.QueryRowContext(ctx, `SELECT good_id FROM recipes WHERE id = $1`, recipeID).Scan(&outputGoodID); err != nil {
		return fmt.Errorf("failed to read recipe output: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount)
		VALUES ($1, 'output', $2, 0) ON CONFLICT DO NOTHING`, branchID, outputGoodID,
	); err != nil {
		return fmt.Errorf("failed to seed output buffer: %w", err)
	}
	return nil
}

// LockBranchTx — единая точка сериализации с синком (§4.2): первым действием
// транзакции берёт FOR UPDATE на строке ветки; возвращает settlement_id и
// recipe_id. Ветки нет — ErrBranchNotFound (404).
func (r *BranchRepository) LockBranchTx(ctx context.Context, tx *sql.Tx, branchID string) (string, int64, error) {
	var settlementID string
	var recipeID int64
	err := tx.QueryRowContext(ctx, `
		SELECT settlement_id, recipe_id FROM settlement_branches WHERE id = $1 FOR UPDATE`, branchID,
	).Scan(&settlementID, &recipeID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrBranchNotFound
	}
	if err != nil {
		return "", 0, fmt.Errorf("failed to lock branch: %w", err)
	}
	return settlementID, recipeID, nil
}

// IncrementBranchInputTx — атомарный инкремент входа (upsert под блокировкой
// ветки, §5): относительный UPDATE — правка админа не теряется синком.
func (r *BranchRepository) IncrementBranchInputTx(ctx context.Context, tx *sql.Tx, branchID string, goodID int64, amount float64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount)
		VALUES ($1, 'input', $2, $3)
		ON CONFLICT (branch_id, direction, good_id)
		DO UPDATE SET amount = settlement_branch_buffers.amount + EXCLUDED.amount, updated_at = NOW()`,
		branchID, goodID, amount,
	)
	if err != nil {
		return fmt.Errorf("failed to increment branch input: %w", err)
	}
	return nil
}

// CountBranchesByGood — число веток с товаром-выходом = goodID во всех мирах
// (§8/О3): предупреждение студии при удалении товара.
func (r *BranchRepository) CountBranchesByGood(goodID int64) (int, error) {
	return countBranchesByGood(r.db, goodID)
}

// countBranchesByGood — единый SQL числа веток (студия + предпроверка удаления).
func countBranchesByGood(q rowQueryer, goodID int64) (int, error) {
	var n int
	if err := q.QueryRow(countBranchesByGoodSQL, goodID).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count branches: %w", err)
	}
	return n, nil
}

// appendUniqueInt64 — добавить значение в срез, если его ещё нет.
func appendUniqueInt64(s []int64, v int64) []int64 {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
