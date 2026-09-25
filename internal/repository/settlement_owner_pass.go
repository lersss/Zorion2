// internal/repository/settlement_owner_pass.go
//
// Owner-проход поселения (спека 2026-09-22-эффекты-снабжения-задержка-голод
// §4.1/§4.5): производство (ветки) → потребность (спрос/покрытие/дефицит →
// сила `w` → нагрузка `load`) → население, в ОДНОЙ транзакции на поселение
// под advisory-локом. Порядок локов: advisory(owner) → settlements(id) →
// ветки(id) → залежи(id). Один `now` на проход → инвариант `load_at ==
// processed_at_b` у всех веток и `computed_at == load_at`; путь «в памяти»
// (Δt < MinPersistInterval у всех веток) не пишет и базис не двигает.
package repository

import (
	"context"
	"database/sql"
	"log"
	"math"
	"sort"
	"time"

	"github.com/lib/pq"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

const (
	// advisoryOwnerLockSQL — сериализация owner-прохода по поселению (§4.5).
	// Тот же ключ использует админ-правка нагрузки (SetSettlementEffectLoad).
	advisoryOwnerLockSQL = `SELECT pg_advisory_xact_lock(hashtext($1))`

	// settlementLockSQL — читает чек-точку населения под блокировкой строки.
	// settlement_type_id читается и здесь (спека 2026-09-23 §3.3 п.2): путь
	// «событие» обязан увидеть тип, сменившийся между чтением и локом, — иначе
	// число скорости пары возьмётся по устаревшему типу.
	settlementLockSQL = `
		SELECT population, population_exact, computed_at, created_at, COALESCE(race_id, ''),
		       COALESCE(settlement_type_id, 0)
		FROM settlements WHERE id = $1 FOR UPDATE`

	// settlementPopulationWriteSQL — запись населения тем же now, что load_at.
	settlementPopulationWriteSQL = `
		UPDATE settlements SET population = $1, population_exact = $2, computed_at = $3, updated_at = NOW()
		WHERE id = $4`

	// effectTypeCatalogSQL — каталог типов эффектов (резолв ссылки params.effects).
	// name — для player-safe DTO эффекта (спека 2026-09-23 §5.4).
	effectTypeCatalogSQL = `
		SELECT id, name, name_norm, impact, COALESCE(params->>'curve', '') FROM effect_types`

	// activeEffectsSelectSQL — хранимый базис нагрузки владельцев.
	activeEffectsSelectSQL = `
		SELECT ae.effect_type_id, COALESCE(ae.source_position, ''), ae.load, ae.load_at,
		       et.impact, COALESCE(et.params->>'curve', ''), ae.owner_id
		FROM active_effects ae
		JOIN effect_types et ON et.id = ae.effect_type_id
		WHERE ae.owner_type = 'settlement' AND ae.owner_id = ANY($1)`

	// activeEffectUpsertSQL — INSERT (новая строка: load = вычисленный,
	// load_at = базис) / DO UPDATE (load = load_new, load_at = базис) —
	// идемпотентность, §4.5. Обе ветки пишут ОДНО вычисленное значение ($5):
	// иначе первая персистентная запись обнуляла бы нагрузку, накопленную с
	// рождения (решение создателя 2026-09-25). Прежнее М4 «load = 0,
	// load_at = now» переопределено: поздняя привязка и так даёт вычисленный 0.
	activeEffectUpsertSQL = `
		INSERT INTO active_effects (effect_type_id, owner_type, owner_id, owner_settlement_id, source_position, load, load_at)
		VALUES ($1, 'settlement', $2, $2, $3, $5, $4)
		ON CONFLICT (owner_type, owner_id, effect_type_id)
		DO UPDATE SET load = $5, load_at = $4, updated_at = NOW()`

	// goodsNamesSQL — словарь товаров: id, имя, позиция (name_norm) и метка
	// `code` (стабильный ключ иконок товара, спека ЧК2а §4.5/§8, T19). Ключ
	// params.eat/params.effects читается только как товар (спека 2026-09-24
	// §6.1/§9.4); id нужен для ячеек хранилища (позиция → good_id), имя — для
	// витрины «забираем».
	goodsNamesSQL = `SELECT id, name, name_norm, COALESCE(code, '') FROM goods`

	// recipeComponentOccurrencesSQL — число вхождений товара во ВХОДЫ рецептов
	// (F3, §1.3): вес ячейки по умолчанию. Один запрос на пачку.
	recipeComponentOccurrencesSQL = `
		SELECT component_id, COUNT(*) FROM recipe_components
		WHERE component_id IS NOT NULL GROUP BY component_id`

	// producerRatesSelectSQL — пары «тип × рецепт» с числом скорости (спека
	// 2026-09-23 §3.3 п.3): один запрос на пачку владельцев, карта
	// typeID → recipeID → *rate (ед/сутки/млрд) + набор пар. NULL/0 = не
	// объявлено → ветка инертна; nil-указатель отличает «числа нет» от
	// «объявленного нуля» для витрины (§11.2). Набор пар нужен признаку
	// «рецепт не в наборе стадии» (§3.5, §8.3).
	producerRatesSelectSQL = `
		SELECT producer_type_id, recipe_id, rate FROM producer_recipes
		WHERE producer_type_id = ANY($1)`

	branchWriteCheckpointSQL = `
		UPDATE settlement_branches SET processed_at = $1, updated_at = NOW() WHERE id = $2`
)

// OwnerSettlement — вход owner-прохода по одному поселению (§4.1/§4.5): чек-точка
// населения, тип поселения (нормы/привязки + множитель «чей рецепт» для числа
// скорости, спека 2026-09-23 §3.3), физика планеты.
type OwnerSettlement struct {
	ID              string
	PlanetID        string
	Population      int
	PopulationExact float64
	ComputedAt      time.Time
	CreatedAt       time.Time
	RaceID          string
	// SettlementTypeID — тип поселения (settlements.settlement_type_id) вместе с
	// recipe_id ветки задаёт пару для числа скорости producer_recipes.rate (§3.3).
	SettlementTypeID  int64
	Planet            settlement.PlanetInput
	EatByPosition     map[string]float64
	EffectsByPosition map[string]string
}

// OwnerResult — результат owner-прохода: пересчитанные население, ветки,
// витрина эффектов, арифметика на текущем населении и мгновенная скорость
// изменения (для r_per_sec).
type OwnerResult struct {
	Population      int
	PopulationExact float64
	ComputedAt      time.Time
	RPerSec         float64
	// RBreakdown — админская витрина состава R_total (идея 2026-09-25): ряды
	// среды из settlement.EnvBreakdown + по ряду на эффект (мгновенная сила
	// последнего сегмента, как lastRate). Сумма значений == RPerSec.
	RBreakdown []models.RComponent
	NDead           float64
	Branches        []models.SettlementBranch
	Effects         []models.ActiveEffect
	// Arithmetic — витрина арифметики по позициям на текущем населении
	// (спека 2026-09-23 §8.1/§8.2).
	Arithmetic []models.SettlementPositionArithmetic
	// Stage — витрина ступени поселения (спека 2026-09-23 §11.3): пороги
	// текущей ступени и вход следующей из ладдеры пачки; тип вне ладдеры → nil.
	Stage *models.SettlementStageView
	// Storage — витрина внутреннего хранилища (спека ЧК2а §4.5/§8): размер и
	// ячейки по товарам с порогом/долей/видом нужды/дефицитом. nil — потребностей
	// нет (ячеек нет).
	Storage *models.SettlementStorage
}

// effectTypeMeta — запись каталога типов эффектов (резолв по name_norm).
type effectTypeMeta struct {
	ID     int64
	Name   string
	Impact string
	Curve  string
}

// storedEffect — хранимый базис нагрузки владельца (строка active_effects).
type storedEffect struct {
	effectTypeID   int64
	sourcePosition string
	load           float64
	loadAt         time.Time
	impact         string
	curve          string
}

// ownerBranchWrite — данные записи по одной ветке (после слоя потребности):
// плюс витринные поля ветки (число скорости пары, признак «не в наборе стадии»).
// input — распределённая доля входных ячеек (F2, ДО ProcessBranch), after —
// остаток доли после производства (для доли добора из залежей, §8.2).
type ownerBranchWrite struct {
	rec        *branchRecord
	input      map[int64]float64
	after      map[int64]float64
	deposits   map[int64][]settlement.DepositLot
	produced   float64
	drawn      float64
	deltaSec   float64
	rate       *float64
	notInStage bool
}

// ownerRun — полный результат owner-прохода (витрина + данные записи).
type ownerRun struct {
	result     OwnerResult
	writes     []ownerBranchWrite
	collapsed  bool
	deathInput settlement.PlanetInput
	// cellFinals — итоговое количество каждой ячейки за проход (§5.7):
	// basis + ΣProduced − Σconsumed_by_branches − drawn_pop (кламп ≥ 0).
	cellFinals map[int64]float64
	// cellBasis — количество ячейки на начало прохода (для дельты записи).
	cellBasis map[int64]float64
	// storageSize — размер хранилища поселения (пишется owner-проходом).
	storageSize float64
}

// SyncSettlements — owner-проход по поселениям (§4.1/§4.5): на каждое
// поселение — производство веток, слой потребности, нагрузка, пересчёт
// населения; персистентный путь — одна транзакция под advisory-локом.
func (r *BranchRepository) SyncSettlements(now time.Time, owners []OwnerSettlement) (map[string]OwnerResult, error) {
	return r.syncSettlements(now, owners, false)
}

// SyncSettlementsCommitIfChanged — owner-проход в режиме commitIfChanged (F1,
// спека ЧК2а §7 пп.1–2): всегда считает, но пишет в БД только если числа
// изменились (население, нагрузка эффектов, количества ячеек, размер
// хранилища, набор/веса ячеек). Не изменилось — no-op без записи. Вызывается
// путём чтения доски ДО её транзакции (свежесть источника).
func (r *BranchRepository) SyncSettlementsCommitIfChanged(now time.Time, owners []OwnerSettlement) (map[string]OwnerResult, error) {
	return r.syncSettlements(now, owners, true)
}

// syncSettlements — общее тело owner-прохода; commitIfChanged — режим записи
// «только при изменении» (F1).
func (r *BranchRepository) syncSettlements(now time.Time, owners []OwnerSettlement, commitIfChanged bool) (map[string]OwnerResult, error) {
	out := make(map[string]OwnerResult, len(owners))
	if len(owners) == 0 {
		return out, nil
	}
	ctx := context.Background()

	catalog, err := r.loadEffectTypeCatalog(ctx)
	if err != nil {
		return nil, err
	}
	goods, err := r.loadGoodsCatalog(ctx)
	if err != nil {
		return nil, err
	}
	occurrences, err := r.loadRecipeComponentOccurrences(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(owners))
	for _, o := range owners {
		ids = append(ids, o.ID)
	}
	recs, err := loadBranches(ctx, r.db, ids)
	if err != nil {
		return nil, err
	}
	bySettlement := map[string][]*branchRecord{}
	for _, rec := range recs {
		bySettlement[rec.settlementID] = append(bySettlement[rec.settlementID], rec)
	}
	stored, err := r.loadActiveEffects(ctx, ids)
	if err != nil {
		return nil, err
	}
	// Ладдера стадий — одним предикатом класса на пачку (спека 2026-09-23
	// §3.3 п.3/§4.3). Типы ладдеры объединяются с типами владельцев: после
	// перехода число/нормы берутся по НОВОМУ типу, иначе пары (тип, рецепт) не
	// нашлись бы в картах.
	ladder, ladderTypeIDs, err := r.loadStageLadder(ctx)
	if err != nil {
		return nil, err
	}
	typeIDs := ownerTypeIDs(owners)
	for _, id := range ladderTypeIDs {
		typeIDs = appendUniqueInt64(typeIDs, id)
	}
	types, err := r.loadProducerTypes(ctx, typeIDs)
	if err != nil {
		return nil, err
	}
	// Числа скорости пар (тип × рецепт) — карта typeID → recipeID → *rate
	// (ед/сутки/млрд) + набор пар. Хватает на обоих путях (персистентный и
	// «в памяти»).
	rates, recipes, err := r.loadProducerRates(ctx, typeIDs)
	if err != nil {
		return nil, err
	}
	data := ownerBatchData{rates: rates, recipes: recipes, types: types, ladder: ladder, occurrences: occurrences}

	for _, o := range owners {
		res, err := r.syncOwner(ctx, o, bySettlement[o.ID], stored[o.ID], catalog, goods, data, now, commitIfChanged)
		if err != nil {
			return nil, err
		}
		out[o.ID] = res
	}
	return out, nil
}

// ownerTypeIDs — уникальные непустые типы поселений пачки (множитель «чей
// рецепт» для числа скорости).
func ownerTypeIDs(owners []OwnerSettlement) []int64 {
	var out []int64
	for _, o := range owners {
		if o.SettlementTypeID == 0 {
			continue
		}
		out = appendUniqueInt64(out, o.SettlementTypeID)
	}
	return out
}

// rateForPair — число скорости пары (тип поселения, рецепт) из карты пачки:
// нет типа/пары/числа → nil («не объявлено» → ветка инертна, §3.2/§3.5).
func rateForPair(rates map[int64]map[int64]*float64, typeID, recipeID int64) *float64 {
	if typeID == 0 {
		return nil
	}
	return rates[typeID][recipeID]
}

// rateValue — число скорости как скаляр: nil (не объявлено) → 0.
func rateValue(rate *float64) float64 {
	if rate == nil {
		return 0
	}
	return *rate
}

// notInStageSet — рецепт отсутствует в наборе рецептов стадии (producer_recipes
// текущего типа поселения, §3.5): ветка не производит.
func notInStageSet(recipes map[int64]map[int64]bool, typeID, recipeID int64) bool {
	if typeID == 0 {
		return false
	}
	return !recipes[typeID][recipeID]
}

// syncOwner — проход по одному поселению: персистентный путь или «в памяти».
// commitIfChanged (F1) — всегда персистентный путь, но запись только при
// изменении чисел.
func (r *BranchRepository) syncOwner(ctx context.Context, o OwnerSettlement, branches []*branchRecord, stored []storedEffect, catalog map[string]effectTypeMeta, goods goodsCatalog, data ownerBatchData, now time.Time, commitIfChanged bool) (OwnerResult, error) {
	// Персистентный путь, если «событие» наступило хоть у одной чек-точки
	// владельца (население или ветка): обе продвигаются одним now (§4.5),
	// поэтому устаревание любой из них требует записи. Режим commitIfChanged
	// считает всегда (запись — по изменению, ниже).
	persistent := commitIfChanged || now.Sub(o.ComputedAt) >= settlement.MinPersistInterval
	for _, b := range branches {
		if now.Sub(b.branch.ProcessedAt) >= settlement.MinPersistInterval {
			persistent = true
		}
	}
	if !persistent {
		deposits, err := r.loadMemoryDepositsForSettlement(ctx, o.PlanetID, branches)
		if err != nil {
			return OwnerResult{}, err
		}
		cells, err := r.loadStorageCells(ctx, o.ID)
		if err != nil {
			return OwnerResult{}, err
		}
		_, size := storagePlan(o, branches, goods, data)
		// На пути «в памяти» стадия не оценивается (спека 2026-09-23 §4.4):
		// сброса базиса по стадии нет; ячейки не пишутся.
		run, err := runOwnerPass(o, branches, stored, catalog, goods, data, deposits, cells, size, now, false)
		if err != nil {
			return OwnerResult{}, err
		}
		return run.result, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return OwnerResult{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, advisoryOwnerLockSQL, o.ID); err != nil {
		return OwnerResult{}, err
	}

	var population int
	var populationExact float64
	var computedAt, createdAt time.Time
	var raceID string
	var settlementTypeID int64
	err = tx.QueryRowContext(ctx, settlementLockSQL, o.ID).
		Scan(&population, &populationExact, &computedAt, &createdAt, &raceID, &settlementTypeID)
	if err == sql.ErrNoRows {
		// Поселение удалено между чтением и локом — no-op (не роняем карточку).
		return OwnerResult{}, nil
	}
	if err != nil {
		return OwnerResult{}, err
	}
	o.Population = population
	o.PopulationExact = populationExact
	o.ComputedAt = computedAt
	o.CreatedAt = createdAt
	o.SettlementTypeID = settlementTypeID
	if raceID != "" {
		o.RaceID = raceID
	}

	txRecs, err := loadSettlementBranchesForUpdate(ctx, tx, o.ID)
	if err != nil {
		return OwnerResult{}, err
	}
	if err := loadBranchComponentsForRecords(ctx, tx, txRecs); err != nil {
		return OwnerResult{}, err
	}
	deposits, err := loadDepositsForUpdate(ctx, tx, o.PlanetID, allComponentGoodIDs(txRecs))
	if err != nil {
		return OwnerResult{}, err
	}

	// Оценка стадии — только на персистентном пути, по ХРАНИМОМУ населению, ДО
	// производства и ДО расчёта потребности (§4.2/§4.4): производство и
	// население этого прохода считаются по настройкам одной актуальной стадии.
	basisReset := false
	stageChanged := false
	if newTypeID, changed := data.ladder.Select(o.SettlementTypeID, float64(population)); changed {
		if err := r.applyStageTransition(ctx, tx, &o, newTypeID, txRecs, data.types); err != nil {
			return OwnerResult{}, err
		}
		stageChanged = true
		// Перечитать ветки: доборные — в составе, ячейки сохранённых обнулены
		// (§5.1 пп.3–4). Базис нагрузки сброшен и в ПАМЯТИ (§5.1 п.5): stored =
		// nil → ComputeNeeds получит load = 0, load_at = now в этом же проходе.
		txRecs, err = loadSettlementBranchesForUpdate(ctx, tx, o.ID)
		if err != nil {
			return OwnerResult{}, err
		}
		if err := loadBranchComponentsForRecords(ctx, tx, txRecs); err != nil {
			return OwnerResult{}, err
		}
		stored = nil
		// Явный признак сброса-по-стадии: базис обнулён намеренно, счёт идёт с
		// текущего момента — даже если поселение впервые проходит проход и его
		// чек-точка совпадает с рождением (иначе получило бы старт с created_at).
		basisReset = true
	}

	// Реестр нужд (§4.3): сверка набора ячеек с потребностями (эффекты +
	// компоненты + выходы веток), веса — по F3. Обычный путь создаёт/добирает
	// ячейки ДО чтения (базис прохода); режим commitIfChanged читает ячейки
	// как есть и создаёт недостающие только при изменении (ниже).
	weights, size := storagePlan(o, txRecs, goods, data)
	cellsRepo := NewStorageCellRepository(r.db)
	var cells []models.StorageCell
	if commitIfChanged {
		cells, err = cellsRepo.GetStorageCellsTx(ctx, tx, StorageOwnerSettlement, o.ID)
	} else {
		if err = cellsRepo.EnsureStorageCellsTx(ctx, tx, StorageOwnerSettlement, o.ID, weights); err == nil {
			cells, err = cellsRepo.GetStorageCellsTx(ctx, tx, StorageOwnerSettlement, o.ID)
		}
	}
	if err != nil {
		return OwnerResult{}, err
	}

	run, err := runOwnerPass(o, txRecs, stored, catalog, goods, data, deposits, cells, size, now, basisReset)
	if err != nil {
		return OwnerResult{}, err
	}

	if commitIfChanged {
		var storedSize float64
		if err := tx.QueryRowContext(ctx, settlementStorageSizeSelectSQL, o.ID).Scan(&storedSize); err != nil {
			return OwnerResult{}, err
		}
		if !stageChanged && !ownerPassChanged(o, run, cells, weights, stored, storedSize) {
			// Числа не изменились — no-op без записи (F1): чек-точки не
			// двигаем, пустые строки не плодим. defer tx.Rollback() закроет tx.
			return run.result, nil
		}
		if err := cellsRepo.EnsureStorageCellsTx(ctx, tx, StorageOwnerSettlement, o.ID, weights); err != nil {
			return OwnerResult{}, err
		}
	}

	if err := r.writeOwnerTx(ctx, tx, o, run, now); err != nil {
		return OwnerResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return OwnerResult{}, err
	}
	return run.result, nil
}

// ownerPassChanged — изменились ли числа owner-прохода относительно хранимого
// состояния (режим commitIfChanged, F1): население/population_exact, размер
// хранилища, количества ячеек, набор/веса ячеек, нагрузка эффектов. false →
// запись не нужна (no-op без записи).
func ownerPassChanged(o OwnerSettlement, run ownerRun, cells []models.StorageCell, weights map[int64]float64, stored []storedEffect, storedSize float64) bool {
	if run.result.Population != o.Population || run.result.PopulationExact != o.PopulationExact {
		return true
	}
	if run.storageSize != storedSize {
		return true
	}
	// Количества ячеек: итог прохода против базиса (basis = хранимое количество).
	for gid, final := range run.cellFinals {
		if final != run.cellBasis[gid] {
			return true
		}
	}
	// Набор/веса ячеек: недостающая ячейка под потребность, изменившийся вес,
	// пустая ячейка вне набора (будет удалена) — всё это изменение.
	cellByGood := make(map[int64]models.StorageCell, len(cells))
	for _, c := range cells {
		cellByGood[c.GoodID] = c
	}
	for gid, w := range weights {
		c, ok := cellByGood[gid]
		if !ok || c.CapShare != w {
			return true
		}
	}
	for _, c := range cells {
		if _, ok := weights[c.GoodID]; !ok && c.Amount == 0 {
			return true
		}
	}
	// Нагрузка эффектов: набор и значения.
	if len(run.result.Effects) != len(stored) {
		return true
	}
	storedLoad := make(map[int64]float64, len(stored))
	for _, se := range stored {
		storedLoad[se.effectTypeID] = se.load
	}
	for _, e := range run.result.Effects {
		if storedLoad[e.EffectTypeID] != e.Load {
			return true
		}
	}
	return false
}

// loadStorageCells — ячейки поселения вне транзакции (путь «в памяти»).
func (r *BranchRepository) loadStorageCells(ctx context.Context, settlementID string) ([]models.StorageCell, error) {
	return NewStorageCellRepository(r.db).GetStorageCells(StorageOwnerSettlement, settlementID)
}

// storagePlan — реестр нужд поселения и веса ячеек (F3, §4.3): набор =
// keys(params.effects) ∪ компоненты рецептов веток ∪ выходы веток (выход
// приходит в ячейку товара-выхода, §1.2). Вес — GoodWeights (ручка shares >
// вхождения во входы рецептов, для товара с эффектом — max(occ,1)). Размер —
// ручка params.storage.size типа текущей ступени, иначе StorageSizeDefault.
func storagePlan(o OwnerSettlement, branches []*branchRecord, goods goodsCatalog, data ownerBatchData) (map[int64]float64, float64) {
	needs := map[int64]bool{}
	for pos := range o.EffectsByPosition {
		if gid, ok := goods.byPosition[pos]; ok {
			needs[gid] = true
		}
	}
	for _, rec := range branches {
		for _, c := range rec.components {
			needs[c.GoodID] = true
		}
		needs[rec.outputGoodID] = true
	}
	// occurrences заполняется под КАЖДУЮ потребность (0 у выхода без вхождений):
	// GoodWeights строит ключи из переданных карт, поэтому выход рецепта обязан
	// попасть в набор — иначе у него не будет ячейки для прихода (§1.2).
	occ := map[int64]int{}
	for g := range needs {
		occ[g] = data.occurrences[g]
	}
	eff := map[int64]bool{}
	for pos := range o.EffectsByPosition {
		if gid, ok := goods.byPosition[pos]; ok {
			eff[gid] = true
		}
	}
	meta := data.types[o.SettlementTypeID]
	exp := map[int64]float64{}
	for g, s := range meta.StorageShares {
		if needs[g] {
			exp[g] = s
		}
	}
	weights := settlement.GoodWeights(exp, occ, eff)
	size := meta.StorageSize
	if size <= 0 {
		size = settlement.StorageSizeDefault
	}
	return weights, size
}

// runOwnerPass — производство → потребность → население (без записи): собирает
// данные записи и итоговые количества ячеек. data — карты пачки (числа скорости
// пар + набор рецептов стадии, §3.3). basisReset — явный признак сброса базиса
// нагрузки сменой стадии в этом же проходе (§5.1 п.5): базис обнулён намеренно,
// старт счёта — с текущего момента.
//
// Порядок (F2/F4/F6, §5.7/§7.1): базис ячейки берётся ОДИН раз на товар;
// вход-ячейка делится между потребителями ∝ потребности (F2); ветки стартуют
// от базиса на начало прохода, свежий выход этого прохода потребителям не виден
// (F4); итог ячейки = basis + ΣProduced − Σconsumed_by_branches − drawn_pop.
func runOwnerPass(o OwnerSettlement, branches []*branchRecord, stored []storedEffect, catalog map[string]effectTypeMeta, goods goodsCatalog, data ownerBatchData, deposits map[int64][]settlement.DepositLot, cells []models.StorageCell, storageSize float64, now time.Time, basisReset bool) (ownerRun, error) {
	population := float64(o.Population)

	// Базис ячеек на начало прохода (F6): один раз на товар.
	basis := make(map[int64]float64, len(cells))
	for _, c := range cells {
		basis[c.GoodID] = c.Amount
	}

	// Числовой ключ потребителя-ветки (F2): стабильный порядок по id (UUID) —
	// тай-брейк AllocateProportional при равных потребностях детерминирован.
	sortedBranches := append([]*branchRecord(nil), branches...)
	sort.Slice(sortedBranches, func(i, j int) bool { return sortedBranches[i].branch.ID < sortedBranches[j].branch.ID })
	branchIndex := make(map[string]int64, len(sortedBranches))
	for i, rec := range sortedBranches {
		branchIndex[rec.branch.ID] = int64(i + 1)
	}

	// 1) Потребности потребителей входных ячеек (F2): ветки-компоненты + население.
	consumerNeeds := map[int64]map[settlement.Consumer]float64{}
	addNeed := func(gid int64, c settlement.Consumer, need float64) {
		if need <= 0 {
			return
		}
		if consumerNeeds[gid] == nil {
			consumerNeeds[gid] = map[settlement.Consumer]float64{}
		}
		consumerNeeds[gid][c] += need
	}
	for _, rec := range branches {
		rate := rateForPair(data.rates, o.SettlementTypeID, rec.branch.RecipeID)
		if rate == nil {
			continue // ветка инертна — потребности нет
		}
		perSec := settlement.PerSecond(*rate, population)
		for _, c := range aggregateBranchComponents(rec.components) {
			addNeed(c.GoodID, settlement.Consumer{Kind: settlement.ConsumerBranch, ID: branchIndex[rec.branch.ID]}, perSec*float64(c.Quantity))
		}
	}
	bindings := buildBindings(o, catalog, goods)
	for _, b := range bindings {
		gid, ok := goods.byPosition[b.Position]
		if !ok {
			continue
		}
		addNeed(gid, settlement.Consumer{Kind: settlement.ConsumerPopulation}, settlement.PerSecond(b.NormPerDayPerBillion, population))
	}
	// Деление базиса ∝ потребности (F2): один потребитель → весь базис.
	alloc := map[int64]map[settlement.Consumer]float64{}
	for gid, needs := range consumerNeeds {
		alloc[gid] = settlement.AllocateProportional(basis[gid], needs)
	}

	// 2) Производство (F4): ветка стартует от базиса на начало прохода; вход —
	// распределённая доля (F2). Свежий выход этого прохода потребителям не виден.
	sources := make([]settlement.NeedsSource, 0, len(branches))
	arithmeticSources := make([]settlement.ArithmeticSource, 0, len(branches))
	writes := make([]ownerBranchWrite, 0, len(branches))
	for _, rec := range branches {
		deltaSec := now.Sub(rec.branch.ProcessedAt).Seconds()
		rate := rateForPair(data.rates, o.SettlementTypeID, rec.branch.RecipeID)
		input := make(map[int64]float64, len(rec.components))
		for _, c := range aggregateBranchComponents(rec.components) {
			input[c.GoodID] = alloc[c.GoodID][settlement.Consumer{Kind: settlement.ConsumerBranch, ID: branchIndex[rec.branch.ID]}]
		}
		p := settlement.ProcessBranch(rec.toBranch(population, input, basis[rec.outputGoodID], rateValue(rate), deposits), now)
		sources = append(sources, settlement.NeedsSource{
			ID:       rec.branch.ID,
			Position: rec.outputPosition,
			Batches:  p.ProducedLast,
			DeltaSec: deltaSec,
			Since:    rec.branch.ProcessedAt, // ветка создана внутри [loadAt, now] → покрытие только с Since (§4.3, С1)
		})
		// Витрина арифметики: производим по позиции — расчётный выход ветки
		// (rate × население), независимо от фактического прохода (§8.2). Ветка
		// без объявленного числа (nil) позиции не создаёт — как проекция
		// студии; объявленный ноль (0) — создаёт с нулём.
		if rate != nil {
			arithmeticSources = append(arithmeticSources, settlement.ArithmeticSource{
				Position:             rec.outputPosition,
				RatePerDayPerBillion: *rate,
			})
		}
		writes = append(writes, ownerBranchWrite{
			rec: rec, input: input, after: p.Input, deposits: p.Deposits,
			produced: p.ProducedLast, deltaSec: deltaSec,
			rate:       rate,
			notInStage: notInStageSet(data.recipes, o.SettlementTypeID, rec.branch.RecipeID),
		})
	}

	// 3) Потребность: привязки позиций (params.effects) → спрос/покрытие/дефицит.
	// Базис позиции для населения — его доля alloc_pop (F2), не Σ OutputBase.
	basisByPosition := map[string]float64{}
	for _, b := range bindings {
		gid, ok := goods.byPosition[b.Position]
		if !ok {
			continue
		}
		basisByPosition[b.Position] = alloc[gid][settlement.Consumer{Kind: settlement.ConsumerPopulation}]
	}

	storedLoad := map[int64]float64{}
	storedLoadAt := map[int64]time.Time{}
	for _, se := range stored {
		storedLoad[se.effectTypeID] = se.load
		storedLoadAt[se.effectTypeID] = se.loadAt
	}
	// Новая строка (нет сохранённого базиса) — load = 0, а load_at выбирается
	// явно (решение создателя 2026-09-25, переопределяет М4 для новорождённых):
	//   - новорождённое поселение (чек-точка ещё не двигалась — computed_at
	//     совпадает с created_at) и привязка есть с самого появления → счёт с
	//     created_at (с рождения);
	//   - иначе (поселение уже пересчитывалось — привязка появилась позже) →
	//     счёт с now, без бэкдейта к рождению;
	//   - сброс базиса сменой стадии (basisReset) → всегда с now, даже если
	//     поселение новорождённое (базис обнулён намеренно).
	fromBirth := !basisReset && !o.ComputedAt.After(o.CreatedAt)
	for _, b := range bindings {
		if _, ok := storedLoadAt[b.EffectTypeID]; !ok {
			storedLoad[b.EffectTypeID] = 0
			if fromBirth {
				storedLoadAt[b.EffectTypeID] = o.CreatedAt
			} else {
				storedLoadAt[b.EffectTypeID] = now
			}
		}
	}
	needs := settlement.ComputeNeeds(settlement.NeedsInput{
		Population:      population,
		Bindings:        bindings,
		Sources:         sources,
		BasisByPosition: basisByPosition,
		StoredLoad:      storedLoad,
		StoredLoadAt:    storedLoadAt,
		Thresholds:      thresholdsFor(bindings),
		Recoveries:      recoveriesFor(bindings),
		ComputedAt:      o.ComputedAt,
		Now:             now,
		CurveLookup:     settlement.BalancerCurveLookup,
	})

	// 4) Ветки: витрина (Eaten ∝ Batches, §5.7) — физически списывается из ячейки.
	branchModels := make([]models.SettlementBranch, 0, len(branches))
	for i := range writes {
		w := &writes[i]
		w.drawn = needs.DrawnBySource[w.rec.branch.ID]
		applyOwnerBranch(w, now, population, goods)
		branchModels = append(branchModels, w.rec.branch)
	}

	// 5) Итог ячеек за проход (§5.7): basis + ΣProduced − Σconsumed_by_branches
	// − drawn_pop; кламп ≥ 0. Базис учтён один раз — двойного счёта нет.
	cellFinals := make(map[int64]float64, len(basis))
	for gid, amt := range basis {
		cellFinals[gid] = amt
	}
	for i := range writes {
		w := &writes[i]
		cellFinals[w.rec.outputGoodID] += w.produced
		for _, c := range aggregateBranchComponents(w.rec.components) {
			consumed := w.input[c.GoodID] - w.after[c.GoodID]
			if consumed < 0 {
				consumed = 0
			}
			cellFinals[c.GoodID] -= consumed
		}
	}
	for pos, drawn := range needs.DrawnByPosition {
		gid, ok := goods.byPosition[pos]
		if !ok {
			continue
		}
		cellFinals[gid] -= drawn
	}
	for gid, v := range cellFinals {
		if v < 0 {
			cellFinals[gid] = 0
		}
	}

	// 6) Население: сила эффекта — кусочной траекторией [computed_at, now) (§5).
	input := o.Planet
	input.RaceID = o.RaceID
	input.Effects = collectForce(needs)
	// AsOf — точка «текущей силы» для точечных потребителей (DeathCause
	// читает dominantEffect(input.Effects, AsOf); ChangeComponents/DeathTime/
	// Projection — p.RateAt(AsOf)). Буква §5.1 (`AsOf = computed_at`) здесь НЕ
	// подходит: RateAt сегмента полуоткрыт [Since, Until), на computed_at он
	// вернул бы силу ПЕРВОГО сегмента траектории (нагрузку на начало интервала),
	// а на now — 0 (Until последнего сегмента = now). DeathCause же нужна
	// ИМЕННО текущая сила (§5.2 — «r как текущая сила»), поэтому берём точку
	// заведомо внутри последнего сегмента [fs, now): now − 1нс (сегмент всегда
	// положительной длины, §5.1; при пустой траектории вклад и так 0).
	input.AsOf = now.Add(-time.Nanosecond)
	next := settlement.Recompute(input, o.PopulationExact, o.ComputedAt, now, o.CreatedAt)

	rPerSec := settlement.EnvComponents(input) + lastRate(needs)
	pop := int(math.Round(math.Min(next, settlement.MaxInt4Population)))

	// Витрина арифметики по позициям (§8.2) + строка нужды (§10.2): покрытие/
	// дефицит берём из слоя потребности (w), привязки дают ключ/тип нужды и норму.
	arithmetic := settlement.ComputePositionArithmetic(population, o.EffectsByPosition, o.EatByPosition, arithmeticSources)
	arithmetic = settlement.AttachNeedArithmetic(arithmetic, bindings, needs.Effects)

	// Вид нужды и эффект ячейки хранилища (T5/§1.4): товар с привязкой эффекта —
	// потребность населения, иначе — потребность производства.
	effectByGood := make(map[int64]string, len(bindings))
	for _, b := range bindings {
		if gid, ok := goods.byPosition[b.Position]; ok {
			effectByGood[gid] = b.EffectTypeName
		}
	}

	res := OwnerResult{
		Population:      pop,
		PopulationExact: next,
		ComputedAt:      now,
		RPerSec:         rPerSec,
		RBreakdown:      rBreakdownComponents(input, needs, catalog),
		NDead:           settlement.NDead,
		Branches:        branchModels,
		Effects:         buildEffectModels(o, needs, catalog, now),
		Arithmetic:      arithmeticModels(arithmetic),
		Stage:           stageView(data.ladder, o.SettlementTypeID),
		Storage:         storageView(goods, cells, cellFinals, storageSize, effectByGood),
	}
	return ownerRun{
		result:      res,
		writes:      writes,
		collapsed:   o.PopulationExact > settlement.NDead && next == 0,
		deathInput:  input,
		cellFinals:  cellFinals,
		cellBasis:   basis,
		storageSize: storageSize,
	}, nil
}

// aggregateBranchComponents — свёртка компонентов рецепта по good_id (дубликат
// component_id на разных pos — один компонент с суммарной нормой, §3.2).
func aggregateBranchComponents(comps []settlement.BranchComponent) []settlement.BranchComponent {
	if len(comps) == 0 {
		return nil
	}
	idx := make(map[int64]int, len(comps))
	out := make([]settlement.BranchComponent, 0, len(comps))
	for _, c := range comps {
		if i, ok := idx[c.GoodID]; ok {
			out[i].Quantity += c.Quantity
			continue
		}
		idx[c.GoodID] = len(out)
		out = append(out, c)
	}
	return out
}

// applyOwnerBranch — вынести результат прохода ветки в display-модель:
// чек-точка, произведено, списано слоем потребности, витрина арифметики ветки
// (число скорости пары, признак «не в наборе стадии», «забираем», доля залежи).
// Буферов у ветки больше нет — запас живёт в ячейках хранилища.
func applyOwnerBranch(w *ownerBranchWrite, now time.Time, population float64, goods goodsCatalog) {
	rec := w.rec
	rec.branch.ProcessedAt = now
	rec.branch.Produced = w.produced
	rec.branch.Eaten = w.drawn
	rec.branch.EatenRate = 0
	if w.deltaSec > 0 {
		rec.branch.EatenRate = w.drawn / w.deltaSec
	}
	rec.branch.RatePerDayPerBillion = w.rate
	rec.branch.NotInStageSet = w.notInStage
	rec.branch.Take = branchTakeModels(w, population, goods)
	rec.branch.DepositShare = settlement.BranchDepositShare(w.produced, rec.components, w.input, w.after)
}

// stageView — витрина ступени поселения (спека 2026-09-23 §11.3): пороги
// текущей ступени и вход следующей из ладдеры пачки. Тип вне ладдеры (0 —
// легаси/тип без params.stage) → nil: карточка рисует только имя типа.
func stageView(ladder settlement.StageLadder, typeID int64) *models.SettlementStageView {
	v, ok := ladder.CurrentView(typeID)
	if !ok {
		return nil
	}
	return &models.SettlementStageView{Enter: v.Enter, Exit: v.Exit, NextEnter: v.NextEnter}
}

// storageView — витрина блока `storage` поселения (спека ЧК2а §4.5/§8): размер,
// ячейки по товарам с порогом (StorageCaps), долей порога от размера, видом
// нужды, name_norm эффекта (только population) и складским дефицитом. Порог
// считается от cap_share самих ячеек (хранимое состояние — источник правды,
// §1.3); amount — итог прохода (cellFinals), у товара без итога — хранимое
// количество. Порядок ячеек детерминирован (good_id). nil — ячеек нет.
func storageView(goods goodsCatalog, cells []models.StorageCell, finals map[int64]float64, size float64, effectByGood map[int64]string) *models.SettlementStorage {
	if len(cells) == 0 {
		return nil
	}
	weights := make(map[int64]float64, len(cells))
	for _, c := range cells {
		weights[c.GoodID] = c.CapShare
	}
	caps := settlement.StorageCaps(size, weights)
	out := &models.SettlementStorage{Size: size, Cells: make([]models.SettlementStorageCell, 0, len(cells))}
	for _, c := range cells {
		amount, ok := finals[c.GoodID]
		if !ok {
			amount = c.Amount
		}
		capValue := caps[c.GoodID]
		share := 0.0
		if size > 0 {
			share = capValue / size
		}
		effect := effectByGood[c.GoodID]
		out.Cells = append(out.Cells, models.SettlementStorageCell{
			Position: goods.positionByID[c.GoodID],
			GoodID:   c.GoodID,
			Code:     goods.codeByID[c.GoodID],
			Amount:   amount,
			Cap:      capValue,
			Share:    share,
			NeedKind: string(settlement.CellKindFor(effect != "")),
			Effect:   effect,
			Deficit:  settlement.CellDeficit(capValue, amount),
		})
	}
	sort.Slice(out.Cells, func(i, j int) bool { return out.Cells[i].GoodID < out.Cells[j].GoodID })
	return out
}

// arithmeticModels — доменная арифметика позиции → витрина ответа (§8.2).
func arithmeticModels(in []settlement.PositionArithmetic) []models.SettlementPositionArithmetic {
	if len(in) == 0 {
		return nil
	}
	out := make([]models.SettlementPositionArithmetic, 0, len(in))
	for _, a := range in {
		out = append(out, models.SettlementPositionArithmetic{
			Position:             a.Position,
			NormPerDayPerBillion: a.NormPerDayPerBillion,
			ProducedPerDay:       a.ProducedPerDay,
			ConsumedPerDay:       a.ConsumedPerDay,
			NetPerDay:            a.NetPerDay,
			Need:                 a.Need,
			Effect:               a.Effect,
			CoveredShare:         a.CoveredShare,
			DeficitShare:         a.DeficitShare,
		})
	}
	return out
}

// branchTakeModels — «забираем» по ветке в модель ответа (§8.2): расчётная
// производная рецепта (выход × quantity_i), имена компонентов — из каталога
// товаров. Числа скорости нет → пусто (выход 0, забирать нечего).
func branchTakeModels(w *ownerBranchWrite, population float64, goods goodsCatalog) []models.SettlementBranchTake {
	if w.rate == nil {
		return nil
	}
	takes := settlement.BranchTakePerDay(rateValue(w.rate), population, w.rec.components)
	if len(takes) == 0 {
		return nil
	}
	out := make([]models.SettlementBranchTake, 0, len(takes))
	for _, t := range takes {
		out = append(out, models.SettlementBranchTake{
			GoodID:   t.GoodID,
			GoodName: goods.nameByID[t.GoodID],
			PerDay:   t.PerDay,
		})
	}
	return out
}

// buildBindings — привязки «позиция → тип эффекта» по params.effects (§4.2):
// позиция — name_norm ТОВАРА (спека 2026-09-24 §6.1/§9.4), категория как
// позиция снята; тип резолвится по name_norm; норма —
// params.eat[позиция]/DefaultEatK; отсутствующая позиция — лог
// position_unknown (не тихий no-op, §7.4).
func buildBindings(o OwnerSettlement, catalog map[string]effectTypeMeta, goods goodsCatalog) []settlement.NeedsBinding {
	bindings := make([]settlement.NeedsBinding, 0, len(o.EffectsByPosition))
	for position, typeName := range o.EffectsByPosition {
		if _, ok := goods.byPosition[position]; !ok {
			log.Printf("⚠️ effect: позиция привязки %q отсутствует в товарах — no-op (position_unknown)", position)
			continue
		}
		meta, ok := catalog[typeName]
		if !ok {
			log.Printf("⚠️ effect: тип эффекта %q не найден в каталоге (effect_type_unknown)", typeName)
			continue
		}
		bindings = append(bindings, settlement.NeedsBinding{
			Position:             position,
			EffectTypeName:       typeName,
			EffectTypeID:         meta.ID,
			Impact:               meta.Impact,
			Curve:                meta.Curve,
			NormPerDayPerBillion: settlement.EatK(o.EatByPosition, position),
		})
	}
	return bindings
}

// thresholdsFor — пороги кривых привязанных эффектов (нулевой префикс, §4.4).
func thresholdsFor(bindings []settlement.NeedsBinding) map[string]float64 {
	out := map[string]float64{}
	for _, b := range bindings {
		if _, ok := out[b.Curve]; ok {
			continue
		}
		if thr, ok := settlement.BalancerCurveThreshold(b.Curve); ok {
			out[b.Curve] = thr
		}
	}
	return out
}

// recoveriesFor — скаляры recovery (сило-ч/ч) по КРИВОЙ эффекта (§7.1):
// ключ — b.Curve, не жёстко hunger. Второй эффект со своей кривой не получает
// скорость восстановления голода. Не-эффект-компонента/неизвестная кривая →
// 0 («без восстановления», безопасный фолбэк, §4.5).
func recoveriesFor(bindings []settlement.NeedsBinding) map[string]float64 {
	out := map[string]float64{}
	for _, b := range bindings {
		if _, ok := out[b.Curve]; ok {
			continue
		}
		v, _ := settlement.ComponentScalar(b.Curve) // ok=false → v=0
		out[b.Curve] = v
	}
	return out
}

// collectForce — объединение траекторий силы всех эффектов владельца (§5.1).
func collectForce(needs settlement.NeedsResult) []settlement.EffectForcePoint {
	var out []settlement.EffectForcePoint
	for _, run := range needs.Effects {
		out = append(out, run.Force...)
	}
	return out
}

// lastRate — сила последнего сегмента нагрузки (мгновенная, для r_per_sec).
func lastRate(needs settlement.NeedsResult) float64 {
	var out float64
	for _, run := range needs.Effects {
		if n := len(run.Force); n > 0 {
			out += run.Force[n-1].Rate
		}
	}
	return out
}

// rBreakdownComponents — состав R_total для админской витрины (идея 2026-09-25):
// сначала ряды среды (settlement.EnvBreakdown — единый источник, сумма ==
// EnvComponents), затем по ряду на эффект — мгновенная сила последнего сегмента
// нагрузки (та же математика, что lastRate; сумма == lastRate). Code эффекта —
// name_norm типа, Name — человекочитаемое имя из каталога (effect_types.name,
// как в buildEffectModels). Инвариант: сумма значений == r_per_sec.
func rBreakdownComponents(input settlement.PlanetInput, needs settlement.NeedsResult, catalog map[string]effectTypeMeta) []models.RComponent {
	nameByID := make(map[int64]string, len(catalog))
	normByID := make(map[int64]string, len(catalog))
	for norm, m := range catalog {
		nameByID[m.ID] = m.Name
		normByID[m.ID] = norm
	}
	out := make([]models.RComponent, 0, len(needs.Effects)+6)
	for _, c := range settlement.EnvBreakdown(input) {
		out = append(out, models.RComponent{Code: c.Code, Value: c.Value})
	}
	for _, run := range needs.Effects {
		var v float64
		if n := len(run.Force); n > 0 {
			v = run.Force[n-1].Rate
		}
		out = append(out, models.RComponent{
			Code:  normByID[run.EffectTypeID],
			Name:  nameByID[run.EffectTypeID],
			Value: v,
		})
	}
	return out
}

// buildEffectModels — витрина эффектов владельца (§6): нагрузка, порог,
// состояние (load ≥ порог), текущая сила, сила условия `w`. Имя типа эффекта
// (catalog.name) кладётся в витрину для player-safe DTO (спека 2026-09-23 §5.4);
// в админском JSON имя не сериализуется (ActiveEffect.Name json:"-").
func buildEffectModels(o OwnerSettlement, needs settlement.NeedsResult, catalog map[string]effectTypeMeta, now time.Time) []models.ActiveEffect {
	nameByID := make(map[int64]string, len(catalog))
	for _, m := range catalog {
		nameByID[m.ID] = m.Name
	}
	out := make([]models.ActiveEffect, 0, len(needs.Effects))
	for _, run := range needs.Effects {
		pos := ""
		if len(run.Positions) > 0 {
			pos = run.Positions[0]
		}
		e := models.ActiveEffect{
			EffectTypeID: run.EffectTypeID,
			Name:         nameByID[run.EffectTypeID],
			OwnerType:    "settlement",
			OwnerID:      o.ID,
			Load:         run.Load,
			LoadAt:       now,
			Curve:        run.Curve,
			Impact:       run.Impact,
			Threshold:    run.Threshold,
			Rate:         settlement.EffectRate(run.Impact, run.Curve, run.Load, settlement.BalancerCurveLookup),
			W:            run.W,
			Enabled:      run.Load >= run.Threshold,
		}
		if pos != "" {
			p := pos
			e.SourcePosition = &p
		}
		settlementID := o.ID
		e.OwnerSettlementID = &settlementID
		out = append(out, e)
	}
	return out
}

// writeOwnerTx — запись owner-прохода одной транзакцией (§4.5): ячейки
// хранилища (дельты) + размер + залежи + UPSERT active_effects (load,
// load_at = now) + processed_at = now у всех веток + население
// (computed_at = now) + лог «Вымерло».
func (r *BranchRepository) writeOwnerTx(ctx context.Context, tx *sql.Tx, o OwnerSettlement, run ownerRun, now time.Time) error {
	cells := NewStorageCellRepository(r.db)
	goodIDs := make([]int64, 0, len(run.cellFinals))
	for gid := range run.cellFinals {
		goodIDs = append(goodIDs, gid)
	}
	sort.Slice(goodIDs, func(i, j int) bool { return goodIDs[i] < goodIDs[j] })
	for _, gid := range goodIDs {
		delta := run.cellFinals[gid] - run.cellBasis[gid]
		if delta == 0 {
			continue
		}
		if err := cells.IncrementStorageCellTx(ctx, tx, StorageOwnerSettlement, o.ID, gid, delta); err != nil {
			return err
		}
	}
	if err := cells.SetSettlementStorageSizeTx(ctx, tx, o.ID, run.storageSize); err != nil {
		return err
	}
	for i := range run.writes {
		w := &run.writes[i]
		for _, lot := range sortedDepositLots(w.deposits) {
			amount := lot.Amount
			if amount < 0 {
				amount = 0
			}
			if _, err := tx.ExecContext(ctx, depositWriteAmountSQL, amount, lot.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, branchWriteCheckpointSQL, now, w.rec.branch.ID); err != nil {
			return err
		}
	}

	for _, e := range run.result.Effects {
		pos := interface{}(nil)
		if e.SourcePosition != nil {
			pos = nullIfEmpty(*e.SourcePosition)
		}
		if _, err := tx.ExecContext(ctx, activeEffectUpsertSQL, e.EffectTypeID, o.ID, pos, now, e.Load); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, settlementPopulationWriteSQL, run.result.Population, run.result.PopulationExact, now, o.ID); err != nil {
		return err
	}

	if run.collapsed {
		deathAt, ok := settlement.DeathTime(o.PopulationExact, run.result.RPerSec, o.ComputedAt, o.CreatedAt)
		if ok {
			cause := settlement.DeathCause(run.deathInput)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO settlement_log (settlement_id, type, occurred_at, cause)
				VALUES ($1, 'extinct', $2, $3) ON CONFLICT DO NOTHING`,
				o.ID, deathAt, cause,
			); err != nil {
				return err
			}
		}
	}

	// Корневая очистка сирот active_effects (§5.4): ключ — тип эффекта из
	// текущих привязок стадии. Пустой набор → удаляются все строки владельца.
	effectTypeIDs := make([]int64, 0, len(run.result.Effects))
	for _, e := range run.result.Effects {
		effectTypeIDs = appendUniqueInt64(effectTypeIDs, e.EffectTypeID)
	}
	if err := cleanupOrphanEffects(ctx, tx, o.ID, effectTypeIDs); err != nil {
		return err
	}
	return nil
}

// loadEffectTypeCatalog — каталог типов эффектов по name_norm (owner-проход).
func (r *BranchRepository) loadEffectTypeCatalog(ctx context.Context) (map[string]effectTypeMeta, error) {
	return queryEffectTypeCatalog(ctx, r.db)
}

// queryEffectTypeCatalog — единый SQL каталога типов эффектов по name_norm
// (образец для читателей: owner-проход и гвард галактической сводки §5.4).
func queryEffectTypeCatalog(ctx context.Context, q branchRowsQueryer) (map[string]effectTypeMeta, error) {
	rows, err := q.QueryContext(ctx, effectTypeCatalogSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]effectTypeMeta{}
	for rows.Next() {
		var nameNorm string
		var m effectTypeMeta
		if err := rows.Scan(&m.ID, &m.Name, &nameNorm, &m.Impact, &m.Curve); err != nil {
			return nil, err
		}
		out[nameNorm] = m
	}
	return out, rows.Err()
}

// goodsCatalog — словарь товаров: позиция (name_norm) → good_id (ключ привязок
// owner-прохода, спека 2026-09-24 §6.1/§9.4), good_id → имя (витрина) и
// обратные карты good_id → позиция/метка `code` (блок storage, §4.5/§8, T19).
type goodsCatalog struct {
	byPosition   map[string]int64
	nameByID     map[int64]string
	positionByID map[int64]string
	codeByID     map[int64]string
}

// loadGoodsCatalog — словарь товаров одним запросом на пачку.
func (r *BranchRepository) loadGoodsCatalog(ctx context.Context) (goodsCatalog, error) {
	rows, err := r.db.QueryContext(ctx, goodsNamesSQL)
	if err != nil {
		return goodsCatalog{}, err
	}
	defer rows.Close()
	out := goodsCatalog{
		byPosition:   map[string]int64{},
		nameByID:     map[int64]string{},
		positionByID: map[int64]string{},
		codeByID:     map[int64]string{},
	}
	for rows.Next() {
		var id int64
		var name, nameNorm, code string
		if err := rows.Scan(&id, &name, &nameNorm, &code); err != nil {
			return goodsCatalog{}, err
		}
		out.byPosition[nameNorm] = id
		out.nameByID[id] = name
		out.positionByID[id] = nameNorm
		out.codeByID[id] = code
	}
	return out, rows.Err()
}

// loadRecipeComponentOccurrences — число вхождений товара во входы рецептов
// (F3, §1.3): вес ячейки по умолчанию. Один запрос на пачку.
func (r *BranchRepository) loadRecipeComponentOccurrences(ctx context.Context) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, recipeComponentOccurrencesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var gid int64
		var n int
		if err := rows.Scan(&gid, &n); err != nil {
			return nil, err
		}
		out[gid] = n
	}
	return out, rows.Err()
}

// loadProducerRates — пары «тип × рецепт» (producer_recipes) одним запросом на
// пачку (спека 2026-09-23 §3.3 п.3). Пустой список типов — пустые карты без
// запроса. Возвращает rates: typeID → recipeID → *rate (ед/сутки/млрд; nil =
// число не объявлено) и recipes: typeID → recipeID → true (набор рецептов
// стадии — признак «не в наборе», §3.5).
func (r *BranchRepository) loadProducerRates(ctx context.Context, typeIDs []int64) (map[int64]map[int64]*float64, map[int64]map[int64]bool, error) {
	rates := map[int64]map[int64]*float64{}
	recipes := map[int64]map[int64]bool{}
	if len(typeIDs) == 0 {
		return rates, recipes, nil
	}
	rows, err := r.db.QueryContext(ctx, producerRatesSelectSQL, pq.Array(typeIDs))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var typeID, recipeID int64
		var rate sql.NullFloat64
		if err := rows.Scan(&typeID, &recipeID, &rate); err != nil {
			return nil, nil, err
		}
		if recipes[typeID] == nil {
			recipes[typeID] = map[int64]bool{}
		}
		recipes[typeID][recipeID] = true
		if !rate.Valid {
			continue
		}
		v := rate.Float64
		if rates[typeID] == nil {
			rates[typeID] = map[int64]*float64{}
		}
		rates[typeID][recipeID] = &v
	}
	return rates, recipes, rows.Err()
}

// loadActiveEffects — хранимые базисы нагрузки поселений по settlement_id.
func (r *BranchRepository) loadActiveEffects(ctx context.Context, ids []string) (map[string][]storedEffect, error) {
	out := map[string][]storedEffect{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, activeEffectsSelectSQL, pqStringArray(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ownerID string
		var e storedEffect
		if err := rows.Scan(&e.effectTypeID, &e.sourcePosition, &e.load, &e.loadAt, &e.impact, &e.curve, &ownerID); err != nil {
			return nil, err
		}
		out[ownerID] = append(out[ownerID], e)
	}
	return out, rows.Err()
}

// loadSettlementBranchesForUpdate — ветки одного поселения под блокировкой
// (порядок id). Физический запас — в ячейках хранилища, не в ветке.
func loadSettlementBranchesForUpdate(ctx context.Context, tx *sql.Tx, settlementID string) ([]*branchRecord, error) {
	rows, err := tx.QueryContext(ctx, branchSelectBySettlementForUpdateSQL, settlementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var recs []*branchRecord
	for rows.Next() {
		rec, err := scanBranchRow(rows)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return recs, nil
}

// loadBranchComponentsForRecords — состав рецептов загруженных веток (в tx).
func loadBranchComponentsForRecords(ctx context.Context, tx *sql.Tx, recs []*branchRecord) error {
	var recipeIDs []int64
	for _, rec := range recs {
		recipeIDs = appendUniqueInt64(recipeIDs, rec.branch.RecipeID)
	}
	if len(recipeIDs) == 0 {
		return nil
	}
	comps, err := loadBranchComponents(ctx, tx, recipeIDs)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		rec.components = comps[rec.branch.RecipeID]
	}
	return nil
}

// allComponentGoodIDs — good_id компонентов всех веток без дублей (фильтр залежей).
func allComponentGoodIDs(recs []*branchRecord) []int64 {
	var out []int64
	for _, rec := range recs {
		for _, gid := range componentGoodIDs(rec.components) {
			out = appendUniqueInt64(out, gid)
		}
	}
	return out
}

// loadMemoryDepositsForSettlement — залежи планеты веток пути «в памяти» (одним
// запросом без блокировки, §4.2): deposits в этом пути не пишется.
func (r *BranchRepository) loadMemoryDepositsForSettlement(ctx context.Context, planetID string, recs []*branchRecord) (map[int64][]settlement.DepositLot, error) {
	if planetID == "" {
		return nil, nil
	}
	goodIDs := allComponentGoodIDs(recs)
	if len(goodIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, depositMemorySelectSQL, pqStringArray([]string{planetID}), pq.Array(goodIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]settlement.DepositLot{}
	for rows.Next() {
		var pid, lotID string
		var goodID int64
		var amount float64
		if err := rows.Scan(&pid, &lotID, &goodID, &amount); err != nil {
			return nil, err
		}
		out[goodID] = append(out[goodID], settlement.DepositLot{ID: lotID, GoodID: goodID, Amount: amount})
	}
	return out, rows.Err()
}
