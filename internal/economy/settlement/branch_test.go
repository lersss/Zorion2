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

// T1: базовый добор — пустой вход, большая залежь, Δt > 0 → залежь убыла на
// batches×quantity, выход вырос, вход не вырос (проходной).
func TestProcessBranchExtractsFromDeposit(t *testing.T) {
	now := time.Now()
	b := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 5000)}},
		ProcessedAt: now.Add(-2 * time.Hour),
	}
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
	b := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{359: 2},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 5000)}},
		ProcessedAt: now.Add(-2 * time.Hour),
	}
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
	b := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 3)}},
		ProcessedAt: now.Add(-10000 * time.Hour),
	}
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
	b := Branch{
		Population: 1e9,
		Components: []BranchComponent{
			{GoodID: 359, Quantity: 1},
			{GoodID: 1, Quantity: 1},
		},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 1000)}}, // второй компонент недоступен
		Output:      5,
		ProcessedAt: now.Add(-2 * time.Hour),
	}
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
	b := Branch{
		Population: 1e9,
		Components: []BranchComponent{
			{GoodID: 359, Quantity: 1},
			{GoodID: 1, Quantity: 1},
		},
		Input:       map[int64]float64{359: 1000, 1: 1000},
		Output:      0,
		ProcessedAt: start,
	}
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
	b := Branch{
		Population: 1e9, // за час «хочется» ~27.8 батча
		Components: []BranchComponent{
			{GoodID: 359, Quantity: 1},
			{GoodID: 1, Quantity: 2},
		},
		Input:       map[int64]float64{359: 100, 1: 6}, // affordable = 6/2 = 3
		ProcessedAt: now.Add(-10 * time.Hour),
	}
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
	b := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}, {GoodID: 359, Quantity: 2}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d1", 359, 300)}},
		ProcessedAt: now.Add(-10000 * time.Hour), // desired ≫ запаса
	}
	got := ProcessBranch(b, now)

	if got.Output != 100 { // affordable = 300/3
		t.Fatalf("выход завышен/занижен при дубликате: got %v want 100", got.Output)
	}
	if got.Deposits[359][0].Amount != 0 {
		t.Fatalf("запас списан не на 300: %v", got.Deposits[359][0].Amount)
	}

	// Нехватка считается по агрегату: 30/3 = 10 батч, списание 30 (не 90).
	small := Branch{
		Population:  1e9,
		Components:  []BranchComponent{{GoodID: 359, Quantity: 1}, {GoodID: 359, Quantity: 2}},
		Input:       map[int64]float64{},
		Deposits:    map[int64][]DepositLot{359: {depositLot("d2", 359, 30)}},
		ProcessedAt: now.Add(-10000 * time.Hour),
	}
	sgot := ProcessBranch(small, now)
	if sgot.Output != 10 {
		t.Fatalf("выход при нехватке: got %v want 10", sgot.Output)
	}
	if sgot.Deposits[359][0].Amount != 0 {
		t.Fatalf("запас при нехватке: %v", sgot.Deposits[359][0].Amount)
	}
}
