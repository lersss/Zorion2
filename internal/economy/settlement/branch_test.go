// internal/economy/settlement/branch_test.go
//
// Юнит-тесты чистой функции переработки ветки (спека 2026-09-22-поселение-
// ветка-буферы-переработка §4, тесты T5–T7/T9/T10): Δt/дефицит входа/
// идемпотентность/скорость по населению и сложности/независимость веток.
// Плюс добыча из залежей своей планеты (спека итерации 3 §12, T1–T6/T10/T16).
package settlement

import (
	"math"
	"testing"
	"time"
)

func ptrInt(v int) *int { return &v }

// depositLot — залежь для тестов добычи (спека итерации 3 §4).
func depositLot(id string, goodID int64, amount float64) DepositLot {
	return DepositLot{ID: id, GoodID: goodID, Amount: amount}
}

// noEat — нормы «этот товар не едят»: тесты производства/добычи не зависят от
// хвоста потребления (спека итерации 4 §3.2: явный 0 = не ест).
func noEat(b Branch) Branch {
	b.EatByGood = map[string]float64{"пища": 0}
	b.OutputGoodNorm = "пища"
	return b
}

// T1: базовый добор — пустой вход, большая залежь, Δt > 0 → залежь убыла на
// batches×quantity, выход вырос, вход не вырос (проходной).
func TestProcessBranchExtractsFromDeposit(t *testing.T) {
	now := time.Now()
	b := noEat(Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 5000)}},
		ProcessedAt: now.Add(-2 * time.Hour),
	})
	want := BranchRate(1e9, nil) * 2.0

	got := ProcessBranch(b, now)

	if math.Abs(got.Output-want) > 1e-9 {
		t.Fatalf("выход: got %v want %v", got.Output, want)
	}
	if got.Input[359] != 0 {
		t.Fatalf("вход не проходной: %v", got.Input[359])
	}
	if math.Abs(got.Deposits[359][0].Amount-(5000-want)) > 1e-9 {
		t.Fatalf("залежь убыла неверно: got %v want %v", got.Deposits[359][0].Amount, 5000-want)
	}
	if b.Deposits[359][0].Amount != 5000 {
		t.Fatalf("исходная залежь мутирована: %v", b.Deposits[359][0].Amount)
	}
}

// T2/F4-A: вход первым — нужда за Δt покрывается входом, залежь не тронута.
func TestProcessBranchInputFirst(t *testing.T) {
	now := time.Now()
	b := Branch{
		Population:  1e9, // за 2 часа хочется ≈55.6 батча
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{359: 1000},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 5000)}},
		ProcessedAt: now.Add(-2 * time.Hour),
	}
	got := ProcessBranch(b, now)
	if got.Deposits[359][0].Amount != 5000 {
		t.Fatalf("залежь тронута, хотя хватило входа: %v", got.Deposits[359][0].Amount)
	}
	if !(got.Input[359] < 1000 && got.Input[359] > 0) {
		t.Fatalf("вход убыл не на нужду: %v", got.Input[359])
	}
}

// T3: добор остатка — вход < потребности → вход обнулился, залежь убыла ровно
// на недостачу.
func TestProcessBranchDepositCoversRemainder(t *testing.T) {
	now := time.Now()
	b := noEat(Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{359: 2},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 5000)}},
		ProcessedAt: now.Add(-2 * time.Hour),
	})
	want := BranchRate(1e9, nil) * 2.0

	got := ProcessBranch(b, now)

	if got.Input[359] != 0 {
		t.Fatalf("вход не обнулился: %v", got.Input[359])
	}
	if math.Abs(got.Deposits[359][0].Amount-(5000-(want-2))) > 1e-9 {
		t.Fatalf("убыль залежи не равна недостаче: got %v want %v", got.Deposits[359][0].Amount, 5000-(want-2))
	}
	if math.Abs(got.Output-want) > 1e-9 {
		t.Fatalf("выход: got %v want %v", got.Output, want)
	}
}

// T4: нет минуса — при огромном Δt малая залежь → batches = affordable,
// amount = 0 (не ниже), выход = affordable.
func TestProcessBranchNoNegativeDeposit(t *testing.T) {
	now := time.Now()
	b := noEat(Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 3)}},
		ProcessedAt: now.Add(-10000 * time.Hour),
	})
	got := ProcessBranch(b, now)
	if got.Deposits[359][0].Amount != 0 {
		t.Fatalf("залежь в минус/не обнулилась: %v", got.Deposits[359][0].Amount)
	}
	if got.Output != 3 {
		t.Fatalf("выход = affordable: got %v want 3", got.Output)
	}
}

// T5: порядок «от крупной к мелкой» (amount DESC) — крупная расходуется
// первой; при равенстве запаса — меньший id первым (id ASC).
func TestProcessBranchDepositOrder(t *testing.T) {
	now := time.Now()
	start := now.Add(-2 * time.Hour)
	want := BranchRate(1e9, nil) * 2.0 // ≈55.6

	big := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("small", 359, 100), depositLot("big", 359, 300)}},
		ProcessedAt: start,
	}
	got := ProcessBranch(big, now)
	byID := map[string]float64{}
	for _, l := range got.Deposits[359] {
		byID[l.ID] = l.Amount
	}
	if math.Abs(byID["big"]-(300-want)) > 1e-9 {
		t.Fatalf("крупная залежь не израсходована первой: %v", byID["big"])
	}
	if byID["small"] != 100 {
		t.Fatalf("мелкая залежь тронута раньше крупной: %v", byID["small"])
	}

	// Ничья по запасу → меньший id первым.
	tie := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("b", 359, 100), depositLot("a", 359, 100)}},
		ProcessedAt: start,
	}
	tgot := ProcessBranch(tie, now)
	tieByID := map[string]float64{}
	for _, l := range tgot.Deposits[359] {
		tieByID[l.ID] = l.Amount
	}
	if math.Abs(tieByID["a"]-(100-want)) > 1e-9 {
		t.Fatalf("при ничьей меньший id не первый: a=%v", tieByID["a"])
	}
	if tieByID["b"] != 100 {
		t.Fatalf("при ничьей израсходован больший id: b=%v", tieByID["b"])
	}
}

// T6: лимит по компоненту — залежь одного компонента истощена → ветка стоит
// целиком, даже если второй компонент доступен.
func TestProcessBranchComponentLimitStops(t *testing.T) {
	now := time.Now()
	b := noEat(Branch{
		Population: 1e9,
		Components: []BranchComponent{
			{GoodID: 359, Quantity: 1},
			{GoodID: 1, Quantity: 1},
		},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 1000)}}, // второй компонент недоступен
		Output:      5,
		ProcessedAt: now.Add(-2 * time.Hour),
	})
	got := ProcessBranch(b, now)
	if got.Output != 5 {
		t.Fatalf("ветка не встала: output %v", got.Output)
	}
	if got.Deposits[359][0].Amount != 1000 {
		t.Fatalf("залежь тронута при дефиците другого компонента: %v", got.Deposits[359][0].Amount)
	}
}

// T7: идемпотентность — повторный вызов с тем же now не меняет ни буферы, ни
// залежи, ни processed_at.
func TestProcessBranchDepositIdempotent(t *testing.T) {
	now := time.Now()
	b := Branch{
		Population:  5e8,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{359: 10},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 1000)}},
		ProcessedAt: now.Add(-2 * time.Hour),
	}
	once := ProcessBranch(b, now)
	twice := ProcessBranch(once, now)
	if once.ProcessedAt != twice.ProcessedAt || once.Output != twice.Output ||
		once.Input[359] != twice.Input[359] || once.Deposits[359][0].Amount != twice.Deposits[359][0].Amount {
		t.Fatalf("не идемпотентна: once=%+v twice=%+v", once, twice)
	}
}

// T10: одна залежь, две ветки — суммарная убыль = сумме переработок; минуса и
// потерь нет (последовательное применение, как в репозитории).
func TestProcessBranchTwoBranchesShareDeposit(t *testing.T) {
	now := time.Now()
	start := now.Add(-2 * time.Hour)
	dep := map[int64][]DepositLot{359: {depositLot("d1", 359, 1000)}}

	a := Branch{Population: 1e9, Components: []BranchComponent{{GoodID: 359, Quantity: 1}}, Input: map[int64]float64{}, Deposits: dep, ProcessedAt: start}
	b := Branch{Population: 1e9, Components: []BranchComponent{{GoodID: 359, Quantity: 1}}, Input: map[int64]float64{}, Deposits: dep, ProcessedAt: start}

	ra := ProcessBranch(a, now)
	rb := ProcessBranch(b, now) // второй читатель общей залежи (не должен делить с первым)
	want := 2 * (BranchRate(1e9, nil) * 2.0)

	sumWithdraw := (1000 - ra.Deposits[359][0].Amount) + (1000 - rb.Deposits[359][0].Amount)
	if math.Abs(sumWithdraw-want) > 1e-9 {
		t.Fatalf("суммарный добор: got %v want %v", sumWithdraw, want)
	}
	if ra.Deposits[359][0].Amount < 0 || rb.Deposits[359][0].Amount < 0 {
		t.Fatalf("залежь в минусе")
	}
}

// T5: Δt = 0 → буферы и processed_at не меняются (первый вызов синка — no-op).
func TestProcessBranchZeroDelta(t *testing.T) {
	now := time.Now()
	b := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{359: 100},
		Output:      5,
		ProcessedAt: now,
	}
	got := ProcessBranch(b, now)
	if got.ProcessedAt != b.ProcessedAt {
		t.Fatalf("processed_at изменился при Δt=0: %v", got.ProcessedAt)
	}
	if got.Output != 5 || got.Input[359] != 100 {
		t.Fatalf("буферы изменились при Δt=0: input=%v output=%v", got.Input[359], got.Output)
	}
}

// T5: Δt > 0 → вход убывает на batches×quantity по каждому компоненту, выход
// прирастает на batches, processed_at = now.
func TestProcessBranchConverts(t *testing.T) {
	now := time.Now()
	start := now.Add(-1 * time.Hour)
	b := noEat(Branch{
		Population: 1e9,
		Components: []BranchComponent{
			{GoodID: 359, Quantity: 1},
			{GoodID: 1, Quantity: 1},
		},
		Input:       map[int64]float64{359: 1000, 1: 1000},
		Output:      0,
		ProcessedAt: start,
	})
	wantBatch := BranchRate(1e9, nil) * 1.0 // desired за 1 час, вход не дефицитен

	got := ProcessBranch(b, now)

	if math.Abs(got.Output-wantBatch) > 1e-9 {
		t.Fatalf("выход: got %v want %v", got.Output, wantBatch)
	}
	if math.Abs(got.Input[359]-(1000-wantBatch)) > 1e-9 {
		t.Fatalf("вход мяса: got %v", got.Input[359])
	}
	if math.Abs(got.Input[1]-(1000-wantBatch)) > 1e-9 {
		t.Fatalf("вход воды: got %v", got.Input[1])
	}
	if !got.ProcessedAt.Equal(now) {
		t.Fatalf("processed_at не продвинулся: %v", got.ProcessedAt)
	}
}

// T6: дефицит входа → batches = affordable, дефицитный компонент обнуляется,
// минуса нет, выход = affordable.
func TestProcessBranchInputDeficit(t *testing.T) {
	now := time.Now()
	b := noEat(Branch{
		Population: 1e9, // за час «хочется» ~27.8 батча
		Components: []BranchComponent{
			{GoodID: 359, Quantity: 1},
			{GoodID: 1, Quantity: 2},
		},
		Input:       map[int64]float64{359: 100, 1: 6}, // affordable = 6/2 = 3
		ProcessedAt: now.Add(-10 * time.Hour),
	})
	got := ProcessBranch(b, now)
	if got.Output != 3 {
		t.Fatalf("выход = affordable: got %v want 3", got.Output)
	}
	if got.Input[1] != 0 {
		t.Fatalf("дефицитный компонент не обнулился: %v", got.Input[1])
	}
	if got.Input[359] != 97 {
		t.Fatalf("мясо: got %v want 97", got.Input[359])
	}
	for id, amt := range got.Input {
		if amt < 0 {
			t.Fatalf("минус во входе good %d: %v", id, amt)
		}
	}
}

// T7: идемпотентность/чистота — второй вызов с тем же now не меняет состояние.
func TestProcessBranchIdempotent(t *testing.T) {
	now := time.Now()
	b := Branch{
		Population:  5e8,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{359: 1000},
		ProcessedAt: now.Add(-2 * time.Hour),
	}
	once := ProcessBranch(b, now)
	twice := ProcessBranch(once, now)
	if once.ProcessedAt != twice.ProcessedAt || once.Output != twice.Output || once.Input[359] != twice.Input[359] {
		t.Fatalf("не идемпотентна: once=%+v twice=%+v", once, twice)
	}
}

// T9: rate линеен по населению (вариант A) и не возрастает по сложности;
// complexity NULL → 1 и 0 → 1 дают ту же скорость, что 1.
func TestBranchRateLinearAndComplexity(t *testing.T) {
	if got, want := BranchRate(2e8, nil), 2*BranchRate(1e8, nil); math.Abs(got-want) > 1e-18 {
		t.Fatalf("rate не линеен по населению: got %v want %v", got, want)
	}
	if BranchRate(1e9, ptrInt(5)) >= BranchRate(1e9, ptrInt(1)) {
		t.Fatalf("сложность 5 не должна ускорять против сложности 1")
	}
	base := BranchRate(1e9, ptrInt(1))
	if BranchRate(1e9, nil) != base {
		t.Fatalf("complexity NULL → 1: got %v want %v", BranchRate(1e9, nil), base)
	}
	if BranchRate(1e9, ptrInt(0)) != base {
		t.Fatalf("complexity 0 → 1: got %v want %v", BranchRate(1e9, ptrInt(0)), base)
	}
	if BranchRate(1e9, ptrInt(4)) != base/4 {
		t.Fatalf("complexity 4 → делитель 4")
	}
}

// T10: ветки перерабатываются независимо; один ресурс как вход одной ветки и
// выход другой — конфликта нет (чистая функция не делит состояние).
func TestProcessBranchIndependent(t *testing.T) {
	now := time.Now()
	a := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 1, Quantity: 1}},
		Input:       map[int64]float64{1: 100},
		ProcessedAt: now.Add(-1 * time.Hour),
	}
	b := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 378, Quantity: 1}},
		Input:       map[int64]float64{378: 100},
		ProcessedAt: now.Add(-1 * time.Hour),
	}
	na := ProcessBranch(a, now)
	nb := ProcessBranch(b, now)
	if na.Output <= 0 || nb.Output <= 0 {
		t.Fatalf("ветки не переработали: %v / %v", na.Output, nb.Output)
	}
	if nb.Input[378] == b.Input[378] {
		t.Fatalf("ветка B не потребила свой вход")
	}
	if na.Input[1] == a.Input[1] {
		t.Fatalf("ветка A не потребила свой вход")
	}
}

// T16: дубликат component_id — один good_id на двух pos сворачивается в ОДИН
// компонент с суммарной нормой; запас списывается один раз, выход = batches (не
// завышен); при нехватке считает по агрегату.
func TestProcessBranchDuplicateComponentAggregated(t *testing.T) {
	now := time.Now()
	// Две строки одного ресурса: norms 1 + 2 = 3 за батч.
	b := noEat(Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}, {GoodID: 359, Quantity: 2}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 300)}},
		ProcessedAt: now.Add(-10000 * time.Hour), // desired ≫ запаса
	})
	got := ProcessBranch(b, now)

	if got.Output != 100 { // affordable = 300/3
		t.Fatalf("выход завышен/занижен при дубликате: got %v want 100", got.Output)
	}
	if got.Deposits[359][0].Amount != 0 {
		t.Fatalf("запас списан не на 300: %v", got.Deposits[359][0].Amount)
	}

	// Нехватка считается по агрегату: 30/3 = 10 батч, списание 30 (не 90).
	small := noEat(Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}, {GoodID: 359, Quantity: 2}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d2", 359, 30)}},
		ProcessedAt: now.Add(-10000 * time.Hour),
	})
	sgot := ProcessBranch(small, now)
	if sgot.Output != 10 {
		t.Fatalf("выход при нехватке: got %v want 10", sgot.Output)
	}
	if sgot.Deposits[359][0].Amount != 0 {
		t.Fatalf("запас при нехватке: %v", sgot.Deposits[359][0].Amount)
	}
}

// ============ ПОТРЕБЛЕНИЕ НАСЕЛЕНИЕМ (спека итерации 4 §4) ============

// consumeBranch — ветка с избытком входа (производство идёт) и заданным выходом:
// для тестов хвоста потребления (спека итерации 4 §4.1).
func consumeBranch(now time.Time, output, population, hours float64, eat map[string]float64, norm string) Branch {
	return Branch{
		Population:     population,
		Components:     []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:          map[int64]float64{359: 1e15},
		Output:         output,
		ProcessedAt:    now.Add(-time.Duration(hours * float64(time.Hour))),
		EatByGood:      eat,
		OutputGoodNorm: norm,
	}
}

// T5: хвост прохода — сначала выход растёт на batches, затем убывает на
// min(eat_k·P·Δt, output); ProducedLast/EatenLast несут результат прохода,
// output ≥ 0 (порядок «производство → потребление»).
func TestProcessBranchConsumesOutput(t *testing.T) {
	now := time.Now()
	b := consumeBranch(now, 100, 1e9, 1, map[string]float64{"пища": 2.5e-8}, "пища")
	produced := BranchRate(1e9, nil) * 1.0
	eaten := 2.5e-8 * 1e9 * 1.0

	got := ProcessBranch(b, now)

	if math.Abs(got.ProducedLast-produced) > 1e-9 {
		t.Fatalf("produced: got %v want %v", got.ProducedLast, produced)
	}
	if math.Abs(got.EatenLast-eaten) > 1e-9 {
		t.Fatalf("eaten: got %v want %v", got.EatenLast, eaten)
	}
	want := 100 + produced - eaten
	if math.Abs(got.Output-want) > 1e-9 {
		t.Fatalf("output: got %v want %v", got.Output, want)
	}
}

// T6/T19: норма берётся по товару-выходу ветки — две ветки разных товаров едят
// по своим нормам, значения не суммируются и не делятся.
func TestProcessBranchEatKByOutputGood(t *testing.T) {
	now := time.Now()
	eat := map[string]float64{"вода": 2.5e-8, "пища": 1e-8}

	w := ProcessBranch(consumeBranch(now, 1000, 1e9, 1, eat, "вода"), now)
	f := ProcessBranch(consumeBranch(now, 1000, 1e9, 1, eat, "пища"), now)

	if math.Abs(w.EatenLast-25) > 1e-9 {
		t.Fatalf("вода: eaten got %v want 25", w.EatenLast)
	}
	if math.Abs(f.EatenLast-10) > 1e-9 {
		t.Fatalf("пища: eaten got %v want 10", f.EatenLast)
	}
}

// T6: явный eat_k = 0 для товара → этот товар не едят (не фолбэк).
func TestProcessBranchExplicitZeroNoEat(t *testing.T) {
	now := time.Now()
	b := consumeBranch(now, 1000, 1e9, 1, map[string]float64{"пища": 0}, "пища")

	got := ProcessBranch(b, now)

	if got.EatenLast != 0 {
		t.Fatalf("явный 0 → не ест, got %v", got.EatenLast)
	}
	if math.Abs(got.Output-(1000+BranchRate(1e9, nil))) > 1e-9 {
		t.Fatalf("выход без еды: got %v", got.Output)
	}
}

// T8: записи для товара ветки нет → фолбэк DefaultEatK (не «не ест»).
func TestProcessBranchMissingEatKUsesDefault(t *testing.T) {
	now := time.Now()
	b := consumeBranch(now, 1000, 1e9, 1, map[string]float64{"вода": 1e-8}, "пища")

	got := ProcessBranch(b, now)

	if math.Abs(got.EatenLast-DefaultEatK*1e9) > 1e-9 {
		t.Fatalf("фолбэк: got %v want %v", got.EatenLast, DefaultEatK*1e9)
	}
}

// T8: тип поселения не задан (nil-структура) → фолбэк DefaultEatK.
func TestProcessBranchNoTypeUsesDefault(t *testing.T) {
	now := time.Now()
	b := consumeBranch(now, 1000, 1e9, 1, nil, "пища")

	got := ProcessBranch(b, now)

	if math.Abs(got.EatenLast-DefaultEatK*1e9) > 1e-9 {
		t.Fatalf("нет типа (nil): got %v want %v", got.EatenLast, DefaultEatK*1e9)
	}
}

// T7: кламп/дренаж — eaten ≤ output, минуса нет; output = 0 → eaten = 0
// (голода нет, п.35); производство 0 и накопленный выход → выход выедается.
func TestProcessBranchConsumptionClampAndDrain(t *testing.T) {
	now := time.Now()

	// Большой Δt, малый выход → съедается ровно накопленное, output = 0.
	b := Branch{
		Population:     1e9,
		Output:         5,
		ProcessedAt:    now.Add(-100 * time.Hour),
		EatByGood:      map[string]float64{"пища": 2.5e-8},
		OutputGoodNorm: "пища",
	}
	got := ProcessBranch(b, now)
	if got.Output != 0 {
		t.Fatalf("output должен обнулиться: %v", got.Output)
	}
	if math.Abs(got.EatenLast-5) > 1e-9 {
		t.Fatalf("eaten = накопленное: got %v", got.EatenLast)
	}

	// output = 0 → eaten = 0 (голода нет).
	zero := Branch{
		Population:     1e9,
		Output:         0,
		ProcessedAt:    now.Add(-time.Hour),
		EatByGood:      map[string]float64{"пища": 2.5e-8},
		OutputGoodNorm: "пища",
	}
	zgot := ProcessBranch(zero, now)
	if zgot.Output != 0 || zgot.EatenLast != 0 {
		t.Fatalf("нет выхода → нет еды: output=%v eaten=%v", zgot.Output, zgot.EatenLast)
	}

	// Производство 0 (нет компонентов), накопленный выход выедается.
	stall := Branch{
		Population:     1e9,
		Output:         100,
		ProcessedAt:    now.Add(-time.Hour),
		EatByGood:      map[string]float64{"пища": 2.5e-8},
		OutputGoodNorm: "пища",
	}
	sgot := ProcessBranch(stall, now)
	if math.Abs(sgot.EatenLast-25) > 1e-9 || math.Abs(sgot.Output-75) > 1e-9 {
		t.Fatalf("питание накопленным: eaten=%v output=%v", sgot.EatenLast, sgot.Output)
	}
}

// T17: при complexity = 1 отношение еда/производство = eat_k / BranchRateK и
// не зависит от населения; при complexity = 2 отношение = eat_k·2 / BranchRateK
// (производство вдвое медленнее, названный дренаж, §8). Выход взят большим,
// чтобы eaten не упирался в кламп и отношение было аналитическим.
func TestProcessBranchEatToProductionIndependentOfPopulation(t *testing.T) {
	now := time.Now()
	eat := map[string]float64{"пища": 2.5e-8}
	ratio := func(pop float64, complexity *int, hours float64) float64 {
		b := Branch{
			Population:     pop,
			Complexity:     complexity,
			Components:     []BranchComponent{{GoodID: 359, Quantity: 1}},
			Input:          map[int64]float64{359: 1e15},
			Output:         1e9, // большой остаток — кламп eaten ≤ output не срабатывает
			ProcessedAt:    now.Add(-time.Duration(hours * float64(time.Hour))),
			EatByGood:      eat,
			OutputGoodNorm: "пища",
		}
		got := ProcessBranch(b, now)
		if got.ProducedLast == 0 {
			t.Fatalf("нет производства для отношения")
		}
		return got.EatenLast / got.ProducedLast
	}

	r1 := ratio(1e5, nil, 1000)
	r2 := ratio(1e9, nil, 100)
	if math.Abs(r1-r2) > 1e-9 {
		t.Fatalf("при complexity=1 отношение не зависит от населения: %v vs %v", r1, r2)
	}
	// Аналитическое отношение при complexity=1: eat_k / BranchRateK.
	if want := 2.5e-8 / BranchRateK; math.Abs(r1-want) > 1e-9 {
		t.Fatalf("аналитическое отношение complexity=1: got %v want %v", r1, want)
	}
	// При complexity=2 производство вдвое медленнее → отношение = eat_k·2/BranchRateK.
	if want := 2.5e-8 * 2 / BranchRateK; math.Abs(ratio(1e9, ptrInt(2), 100)-want) > 1e-9 {
		t.Fatalf("аналитическое отношение complexity=2: got %v want %v", ratio(1e9, ptrInt(2), 100), want)
	}
}

// T19: нормы изолированы — изменение нормы одного товара не меняет еду ветки
// другого товара; ветка товара без записи ест по DefaultEatK.
func TestProcessBranchEatNormsIsolated(t *testing.T) {
	now := time.Now()
	eat1 := map[string]float64{"вода": 2.5e-8, "пища": 1e-8}
	eat2 := map[string]float64{"вода": 9e-8, "пища": 1e-8}

	f1 := ProcessBranch(consumeBranch(now, 1000, 1e9, 1, eat1, "пища"), now)
	f2 := ProcessBranch(consumeBranch(now, 1000, 1e9, 1, eat2, "пища"), now)
	if math.Abs(f1.EatenLast-f2.EatenLast) > 1e-12 {
		t.Fatalf("нормы изолированы: «пища» изменилась %v vs %v", f1.EatenLast, f2.EatenLast)
	}

	w := ProcessBranch(consumeBranch(now, 1000, 1e9, 1, map[string]float64{"пища": 1e-8}, "вода"), now)
	if math.Abs(w.EatenLast-DefaultEatK*1e9) > 1e-9 {
		t.Fatalf("ветка без записи → DefaultEatK: got %v", w.EatenLast)
	}
}
