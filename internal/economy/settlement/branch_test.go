// internal/economy/settlement/branch_test.go
//
// Юнит-тесты чистой функции переработки ветки (спека 2026-09-22-поселение-
// ветка-буферы-переработка §4, тесты T5–T7/T9/T10): Δt/дефицит входа/
// идемпотентность/скорость по населению и сложности/независимость веток.
package settlement

import (
	"math"
	"testing"
	"time"
)

func ptrInt(v int) *int { return &v }

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
