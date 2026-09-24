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

	// activeEffectUpsertSQL — INSERT (новая строка: load = 0, load_at = now,
	// М4) / DO UPDATE (load = load_new, load_at = now) — идемпотентность, §4.5.
	activeEffectUpsertSQL = `
		INSERT INTO active_effects (effect_type_id, owner_type, owner_id, owner_settlement_id, source_position, load, load_at)
		VALUES ($1, 'settlement', $2, $2, $3, 0, $4)
		ON CONFLICT (owner_type, owner_id, effect_type_id)
		DO UPDATE SET load = $5, load_at = $4, updated_at = NOW()`

	// categoryNamesSQL — словарь позиций корзины (categories.name_norm, любой kind).
	categoryNamesSQL = `SELECT name_norm FROM categories`

	// producerRatesSelectSQL — пары «тип × рецепт» с числом скорости (спека
	// 2026-09-23 §3.3 п.3): один запрос на пачку владельцев, карта
	// typeID → recipeID → *rate (ед/сутки/млрд) + набор пар. NULL/0 = не
	// объявлено → ветка инертна; nil-указатель отличает «числа нет» от
	// «объявленного нуля» для витрины (§11.2). Набор пар нужен признаку
	// «рецепт не в наборе стадии» (§3.5, §8.3).
	producerRatesSelectSQL = `
		SELECT producer_type_id, recipe_id, rate FROM producer_recipes
		WHERE producer_type_id = ANY($1)`

	branchTopUpInputSQL = `
		INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount)
		VALUES ($1, 'input', $2, 0) ON CONFLICT (branch_id, direction, good_id) DO NOTHING`

	branchWriteInputSQL = `
		UPDATE settlement_branch_buffers SET amount = $1, updated_at = NOW()
		WHERE branch_id = $2 AND direction = 'input' AND good_id = $3`

	branchWriteOutputSQL = `
		INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount)
		VALUES ($1, 'output', $2, $3)
		ON CONFLICT (branch_id, direction, good_id)
		DO UPDATE SET amount = EXCLUDED.amount, updated_at = NOW()`

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
	NDead           float64
	Branches        []models.SettlementBranch
	Effects         []models.ActiveEffect
	// Arithmetic — витрина арифметики по позициям на текущем населении
	// (спека 2026-09-23 §8.1/§8.2).
	Arithmetic []models.SettlementPositionArithmetic
	// Stage — витрина ступени поселения (спека 2026-09-23 §11.3): пороги
	// текущей ступени и вход следующей из ладдеры пачки; тип вне ладдеры → nil.
	Stage *models.SettlementStageView
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
// плюс витринные поля ветки (число скорости пары, признак «не в наборе стадии»)
// и снимок входа ДО прохода (для доли добора из залежей, §8.2).
type ownerBranchWrite struct {
	rec         *branchRecord
	input       map[int64]float64
	deposits    map[int64][]settlement.DepositLot
	produced    float64
	drawn       float64
	output      float64 // итоговый буфер O0_b + batches_b − drawn_b
	deltaSec    float64
	rate        *float64
	notInStage  bool
	inputBefore map[int64]float64
}

// ownerRun — полный результат owner-прохода (витрина + данные записи).
type ownerRun struct {
	result     OwnerResult
	writes     []ownerBranchWrite
	collapsed  bool
	deathInput settlement.PlanetInput
}

// SyncSettlements — owner-проход по поселениям (§4.1/§4.5): на каждое
// поселение — производство веток, слой потребности, нагрузка, пересчёт
// населения; персистентный путь — одна транзакция под advisory-локом.
func (r *BranchRepository) SyncSettlements(now time.Time, owners []OwnerSettlement) (map[string]OwnerResult, error) {
	out := make(map[string]OwnerResult, len(owners))
	if len(owners) == 0 {
		return out, nil
	}
	ctx := context.Background()

	catalog, err := r.loadEffectTypeCatalog(ctx)
	if err != nil {
		return nil, err
	}
	knownPositions, err := r.loadCategoryNames(ctx)
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
	data := ownerBatchData{rates: rates, recipes: recipes, types: types, ladder: ladder}

	for _, o := range owners {
		res, err := r.syncOwner(ctx, o, bySettlement[o.ID], stored[o.ID], catalog, knownPositions, data, now)
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
func (r *BranchRepository) syncOwner(ctx context.Context, o OwnerSettlement, branches []*branchRecord, stored []storedEffect, catalog map[string]effectTypeMeta, knownPositions map[string]bool, data ownerBatchData, now time.Time) (OwnerResult, error) {
	// Персистентный путь, если «событие» наступило хоть у одной чек-точки
	// владельца (население или ветка): обе продвигаются одним now (§4.5),
	// поэтому устаревание любой из них требует записи.
	persistent := now.Sub(o.ComputedAt) >= settlement.MinPersistInterval
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
		run, err := runOwnerPass(o, branches, stored, catalog, knownPositions, data, deposits, now)
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
	if err := topUpBranchInputs(ctx, tx, txRecs); err != nil {
		return OwnerResult{}, err
	}
	deposits, err := loadDepositsForUpdate(ctx, tx, o.PlanetID, allComponentGoodIDs(txRecs))
	if err != nil {
		return OwnerResult{}, err
	}

	// Оценка стадии — только на персистентном пути, по ХРАНИМОМУ населению, ДО
	// производства и ДО расчёта потребности (§4.2/§4.4): производство и
	// население этого прохода считаются по настройкам одной актуальной стадии.
	if newTypeID, changed := data.ladder.Select(o.SettlementTypeID, float64(population)); changed {
		if err := r.applyStageTransition(ctx, tx, &o, newTypeID, txRecs, data.types); err != nil {
			return OwnerResult{}, err
		}
		// Перечитать ветки: доборные — в составе, буферы сохранённых обнулены
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
	}

	run, err := runOwnerPass(o, txRecs, stored, catalog, knownPositions, data, deposits, now)
	if err != nil {
		return OwnerResult{}, err
	}
	if err := writeOwnerTx(ctx, tx, o, run, now); err != nil {
		return OwnerResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return OwnerResult{}, err
	}
	return run.result, nil
}

// runOwnerPass — производство → потребность → население (без записи): мутирует
// display-копии веток и собирает данные записи. data — карты пачки (числа
// скорости пар + набор рецептов стадии, §3.3).
func runOwnerPass(o OwnerSettlement, branches []*branchRecord, stored []storedEffect, catalog map[string]effectTypeMeta, knownPositions map[string]bool, data ownerBatchData, deposits map[int64][]settlement.DepositLot, now time.Time) (ownerRun, error) {
	population := float64(o.Population)

	// 1) Производство: снимок выходного буфера O0_b ДО ProcessBranch (finding 6).
	sources := make([]settlement.NeedsSource, 0, len(branches))
	arithmeticSources := make([]settlement.ArithmeticSource, 0, len(branches))
	writes := make([]ownerBranchWrite, 0, len(branches))
	for _, rec := range branches {
		base := branchOutputAmount(rec.branch.Output, rec.outputGoodID)
		deltaSec := now.Sub(rec.branch.ProcessedAt).Seconds()
		rate := rateForPair(data.rates, o.SettlementTypeID, rec.branch.RecipeID)
		p := settlement.ProcessBranch(rec.toBranch(population, base, rateValue(rate), deposits), now)
		sources = append(sources, settlement.NeedsSource{
			ID:         rec.branch.ID,
			Position:   rec.outputCategory,
			Batches:    p.ProducedLast,
			DeltaSec:   deltaSec,
			OutputBase: base,
			Since:      rec.branch.ProcessedAt, // ветка создана внутри [loadAt, now] → покрытие только с Since (§4.3, С1)
		})
		// Витрина арифметики: производим по позиции — расчётный выход ветки
		// (rate × население), независимо от фактического прохода (§8.2). Ветка
		// без объявленного числа (nil) позиции не создаёт — как проекция
		// студии; объявленный ноль (0) — создаёт с нулём.
		if rate != nil {
			arithmeticSources = append(arithmeticSources, settlement.ArithmeticSource{
				Position:             rec.outputCategory,
				RatePerDayPerBillion: *rate,
			})
		}
		writes = append(writes, ownerBranchWrite{
			rec: rec, input: p.Input, deposits: p.Deposits,
			produced: p.ProducedLast, output: p.Output, deltaSec: deltaSec,
			rate:        rate,
			notInStage:  notInStageSet(data.recipes, o.SettlementTypeID, rec.branch.RecipeID),
			inputBefore: branchInputAmounts(rec.branch.Input),
		})
	}

	// 2) Потребность: привязки позиций (params.effects) → спрос/покрытие/дефицит.
	bindings := buildBindings(o, catalog, knownPositions)

	storedLoad := map[int64]float64{}
	storedLoadAt := map[int64]time.Time{}
	for _, se := range stored {
		storedLoad[se.effectTypeID] = se.load
		storedLoadAt[se.effectTypeID] = se.loadAt
	}
	// Новая строка (нет базиса) — load = 0, load_at = now → первый интервал
	// считается с now (М4, §3.2): Δt = 0, накопления нет.
	for _, b := range bindings {
		if _, ok := storedLoadAt[b.EffectTypeID]; !ok {
			storedLoad[b.EffectTypeID] = 0
			storedLoadAt[b.EffectTypeID] = now
		}
	}
	needs := settlement.ComputeNeeds(settlement.NeedsInput{
		Population:   population,
		Bindings:     bindings,
		Sources:      sources,
		StoredLoad:   storedLoad,
		StoredLoadAt: storedLoadAt,
		Thresholds:   thresholdsFor(bindings),
		Recoveries:   recoveriesFor(bindings),
		ComputedAt:   o.ComputedAt,
		Now:          now,
		CurveLookup:  settlement.BalancerCurveLookup,
	})

	// 3) Ветки: итоговый выходной буфер — O0_b + batches_b − drawn_b (§4.2).
	branchModels := make([]models.SettlementBranch, 0, len(branches))
	for i := range writes {
		w := &writes[i]
		w.drawn = needs.DrawnBySource[w.rec.branch.ID]
		w.output -= w.drawn
		applyOwnerBranch(w, now, population)
		branchModels = append(branchModels, w.rec.branch)
	}

	// 4) Население: сила эффекта — кусочной траекторией [computed_at, now) (§5).
	input := o.Planet
	input.RaceID = o.RaceID
	input.Effects = collectForce(needs)
	// AsOf — точка «текущей силы» для точечных потребителей (DeathCause
	// читает effectsRateAt(input.Effects, AsOf); ChangeComponents/DeathTime/
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

	res := OwnerResult{
		Population:      pop,
		PopulationExact: next,
		ComputedAt:      now,
		RPerSec:         rPerSec,
		NDead:           settlement.NDead,
		Branches:        branchModels,
		Effects:         buildEffectModels(o, needs, catalog, now),
		Arithmetic:      arithmeticModels(settlement.ComputePositionArithmetic(population, o.EffectsByPosition, o.EatByPosition, arithmeticSources)),
		Stage:           stageView(data.ladder, o.SettlementTypeID),
	}
	return ownerRun{
		result:     res,
		writes:     writes,
		collapsed:  o.PopulationExact > settlement.NDead && next == 0,
		deathInput: input,
	}, nil
}

// applyOwnerBranch — вынести результат прохода ветки в display-модель: буферы,
// чек-точка, произведено, списано слоем потребности, витрина арифметики ветки
// (число скорости пары, признак «не в наборе стадии», «забираем», доля залежи).
func applyOwnerBranch(w *ownerBranchWrite, now time.Time, population float64) {
	rec := w.rec
	for i := range rec.branch.Input {
		if v, ok := w.input[rec.branch.Input[i].GoodID]; ok {
			rec.branch.Input[i].Amount = v
		}
	}
	for i := range rec.branch.Output {
		if rec.branch.Output[i].GoodID == rec.outputGoodID {
			rec.branch.Output[i].Amount = w.output
		}
	}
	rec.branch.ProcessedAt = now
	rec.branch.Produced = w.produced
	rec.branch.Eaten = w.drawn
	rec.branch.EatenRate = 0
	if w.deltaSec > 0 {
		rec.branch.EatenRate = w.drawn / w.deltaSec
	}
	rec.branch.RatePerDayPerBillion = w.rate
	rec.branch.NotInStageSet = w.notInStage
	rec.branch.Take = branchTakeModels(w, population)
	rec.branch.DepositShare = settlement.BranchDepositShare(w.produced, rec.components, w.inputBefore, w.input)
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

// arithmeticModels — доменная арифметика позиции → витрина ответа (§8.2).
func arithmeticModels(in []settlement.PositionArithmetic) []models.SettlementPositionArithmetic {
	if len(in) == 0 {
		return nil
	}
	out := make([]models.SettlementPositionArithmetic, 0, len(in))
	for _, a := range in {
		out = append(out, models.SettlementPositionArithmetic{
			Position:       a.Position,
			ProducedPerDay: a.ProducedPerDay,
			ConsumedPerDay: a.ConsumedPerDay,
			NetPerDay:      a.NetPerDay,
		})
	}
	return out
}

// branchInputAmounts — снимок входного буфера ветки «good_id → amount» ДО
// прохода (для доли добора из залежей, §8.2).
func branchInputAmounts(entries []models.BranchBufferEntry) map[int64]float64 {
	out := make(map[int64]float64, len(entries))
	for _, e := range entries {
		out[e.GoodID] = e.Amount
	}
	return out
}

// branchTakeModels — «забираем» по ветке в модель ответа (§8.2): расчётная
// производная рецепта (выход × quantity_i), имена компонентов — из входного
// буфера. Числа скорости нет → пусто (выход 0, забирать нечего).
func branchTakeModels(w *ownerBranchWrite, population float64) []models.SettlementBranchTake {
	if w.rate == nil {
		return nil
	}
	takes := settlement.BranchTakePerDay(rateValue(w.rate), population, w.rec.components)
	if len(takes) == 0 {
		return nil
	}
	nameByGood := make(map[int64]string, len(w.rec.branch.Input))
	for _, e := range w.rec.branch.Input {
		nameByGood[e.GoodID] = e.GoodName
	}
	out := make([]models.SettlementBranchTake, 0, len(takes))
	for _, t := range takes {
		out = append(out, models.SettlementBranchTake{
			GoodID:   t.GoodID,
			GoodName: nameByGood[t.GoodID],
			PerDay:   t.PerDay,
		})
	}
	return out
}

// buildBindings — привязки «позиция → тип эффекта» по params.effects (§4.2):
// тип резолвится по name_norm; норма — params.eat[позиция]/DefaultEatK;
// отсутствующая позиция — лог position_unknown (не тихий no-op, §7.4).
func buildBindings(o OwnerSettlement, catalog map[string]effectTypeMeta, knownPositions map[string]bool) []settlement.NeedsBinding {
	bindings := make([]settlement.NeedsBinding, 0, len(o.EffectsByPosition))
	for position, typeName := range o.EffectsByPosition {
		if !knownPositions[position] {
			log.Printf("⚠️ effect: позиция привязки %q отсутствует в categories — no-op (position_unknown)", position)
			continue
		}
		meta, ok := catalog[typeName]
		if !ok {
			log.Printf("⚠️ effect: тип эффекта %q не найден в каталоге (effect_type_unknown)", typeName)
			continue
		}
		bindings = append(bindings, settlement.NeedsBinding{
			Position:             position,
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

// writeOwnerTx — запись owner-прохода одной транзакцией (§4.5): вход/залежи +
// выходной буфер + UPSERT active_effects (load, load_at = now) + processed_at =
// now у всех веток + население (computed_at = now) + лог «Вымерло».
func writeOwnerTx(ctx context.Context, tx *sql.Tx, o OwnerSettlement, run ownerRun, now time.Time) error {
	for i := range run.writes {
		w := &run.writes[i]
		for _, c := range w.rec.components {
			if _, err := tx.ExecContext(ctx, branchWriteInputSQL, w.input[c.GoodID], w.rec.branch.ID, c.GoodID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, branchWriteOutputSQL, w.rec.branch.ID, w.rec.outputGoodID, w.output); err != nil {
			return err
		}
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

// loadCategoryNames — словарь позиций корзины (name_norm категорий, любой kind).
func (r *BranchRepository) loadCategoryNames(ctx context.Context) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, categoryNamesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
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
// (порядок id), с буферами.
func loadSettlementBranchesForUpdate(ctx context.Context, tx *sql.Tx, settlementID string) ([]*branchRecord, error) {
	rows, err := tx.QueryContext(ctx, branchSelectBySettlementForUpdateSQL, settlementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var recs []*branchRecord
	var branchIDs []string
	for rows.Next() {
		rec, err := scanBranchRow(rows)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
		branchIDs = append(branchIDs, rec.branch.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	if err := attachBranchBuffers(ctx, tx, branchIDs, recs); err != nil {
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

// topUpBranchInputs — строки входа для текущих заполненных компонентов (T14):
// новый компонент рецепта без строки дал бы affordable = 0 — ветка молча
// встала бы. ON CONFLICT DO NOTHING; появление строки — лог.
func topUpBranchInputs(ctx context.Context, tx *sql.Tx, recs []*branchRecord) error {
	for _, rec := range recs {
		for _, c := range rec.components {
			res, err := tx.ExecContext(ctx, branchTopUpInputSQL, rec.branch.ID, c.GoodID)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err == nil && n > 0 {
				log.Printf("🌿 ветка %s: новый компонент рецепта %d добавлен во вход (0)", rec.branch.ID, c.GoodID)
			}
		}
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
