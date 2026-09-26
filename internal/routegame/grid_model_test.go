package routegame

// grid_model_test.go — тесты производственной модели v9 «Планшет» (§14):
// детерминизм (secret+seed), инварианты принятого поля, валидность пути и
// причины отказов, границы кривой bonus (в т.ч. отрицательный), вскрытие
// сектора только своим secret.

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Поле — детерминированная функция от (seed, secret, dist, passport).
func TestGridGenerate_Deterministic(t *testing.T) {
	secret := []byte("route-secret-A")
	a, okA := GenerateGridField(42, secret, 12345, calmPassport())
	b, okB := GenerateGridField(42, secret, 12345, calmPassport())
	if okA != okB {
		t.Fatalf("hasGame differs: %v vs %v", okA, okB)
	}
	if !okA {
		t.Skip("seed 42 даёт «игры нет»")
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("field not deterministic")
	}
}

func TestGridGenerate_DifferentSeed(t *testing.T) {
	secret := []byte("route-secret-A")
	a, okA := GenerateGridField(1, secret, 12000, calmPassport())
	b, okB := GenerateGridField(2, secret, 12000, calmPassport())
	if !okA || !okB {
		t.Skip("один из seed даёт «игры нет»")
	}
	if reflect.DeepEqual(a, b) {
		t.Errorf("different seeds produced identical field")
	}
}

// Скрытое содержимое зависит от secret; видимый слой — от seed.
func TestGridGenerate_SecretDrivesContent(t *testing.T) {
	diff := 0
	for seed := int64(0); seed < 20; seed++ {
		a, okA := GenerateGridField(seed, []byte("alpha"), 15000, calmPassport())
		b, okB := GenerateGridField(seed, []byte("beta"), 15000, calmPassport())
		if !okA || !okB || len(a.Sectors) != len(b.Sectors) {
			continue
		}
		for i := range a.Sectors {
			if a.Sectors[i].content != b.Sectors[i].content {
				diff++
				break
			}
		}
	}
	if diff == 0 {
		t.Errorf("содержимое не зависит от secret")
	}
}

// Инварианты принятого поля: якоря единой шкалы §14.2 и объекты/секторы §14.3.
func TestGridGenerate_Invariants(t *testing.T) {
	seeds := 30
	if testing.Short() {
		seeds = 8
	}
	secret := []byte("invariants")
	cfg := defaultGridConfig()
	withGame := 0
	for seed := int64(0); seed < int64(seeds); seed++ {
		f, ok := GenerateGridField(seed, secret, 20000, restlessPassport())
		if !ok {
			continue
		}
		withGame++
		if f.iOf(f.Start) != 0 || f.iOf(f.Finish) != f.N-1 {
			t.Fatalf("seed %d: endpoints not on border columns", seed)
		}
		if len(f.Beacons) < cfg.KMin || len(f.Beacons) > cfg.KMax {
			t.Fatalf("seed %d: beacons %d out of [%d,%d]", seed, len(f.Beacons), cfg.KMin, cfg.KMax)
		}
		// guard §14.2: L_naive > L_safe; верх не выше безопасного (L_risk ≤ L_safe).
		if f.LNaive <= f.LSafe+1e-9 {
			t.Fatalf("seed %d: L_naive %.2f <= L_safe %.2f", seed, f.LNaive, f.LSafe)
		}
		if f.LRisk > f.LSafe+1e-9 {
			t.Fatalf("seed %d: L_risk %.2f > L_safe %.2f", seed, f.LRisk, f.LSafe)
		}
		if len(f.realized) != f.N*f.N || len(f.Visible) != f.N*f.N {
			t.Fatalf("seed %d: maps size mismatch", seed)
		}
		if len(f.Sectors) != cfg.SectorCount {
			t.Fatalf("seed %d: sectors %d, want %d", seed, len(f.Sectors), cfg.SectorCount)
		}
		noisy := 0
		for _, s := range f.Sectors {
			if s.Sig < GridSigQuiet || s.Sig > GridSigNoisy {
				t.Fatalf("seed %d: bad sig %d", seed, s.Sig)
			}
			if s.Sig == GridSigNoisy {
				noisy++
			}
			if s.content < GridContentEmpty || s.content > GridContentUnstable {
				t.Fatalf("seed %d: bad content %d", seed, s.content)
			}
			if len(s.Cells) == 0 {
				t.Fatalf("seed %d: empty sector", seed)
			}
		}
		if noisy == 0 {
			t.Fatalf("seed %d: no noisy sector", seed)
		}
	}
	if withGame < seeds/2 {
		t.Errorf("too few fields with game: %d/%d", withGame, seeds)
	}
}

// Кривая bonus(C): якоря и клампинг §14.2 (низ через m0 = 0.10·L_naive),
// бонус может быть отрицательным.
func TestGridBonus_Curve(t *testing.T) {
	f := GridField{LNaive: 40, LSafe: 30, LRisk: 20, M0Frac: 0.10}
	cases := []struct{ C, want float64 }{
		{40, 0.10},        // ленивый = пол
		{30, 0.30},        // безопасный обход = +0.30
		{20, 0.50},        // реализованный оптимум со срезом
		{35, 0.20},        // середина u = 5/10
		{25, 0.40},        // середина w = 5/10
		{44, 0.10 - 0.30}, // ниже пола на пределе m0=4 → −0.20
		{80, 0.10 - 0.30}, // кламп снизу
	}
	for _, tc := range cases {
		if got := f.bonusCostGrid(tc.C); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("bonusCost(%v) = %v, want %v", tc.C, got, tc.want)
		}
	}
	if got := f.bonusCostGrid(44); got >= 0 {
		t.Errorf("низ кривой должен быть отрицательным, got %v", got)
	}
	if got := f.bonusCostGrid(1e6); math.Abs(got-(-0.20)) > 1e-9 {
		t.Errorf("кламп снизу = %v, want −0.20", got)
	}
	prev := math.Inf(1)
	for C := 15.0; C <= 50.0; C += 0.25 {
		b := f.bonusCostGrid(C)
		if b > prev+1e-12 {
			t.Fatalf("bonus not non-increasing at C=%.2f (%.4f > %.4f)", C, b, prev)
		}
		prev = b
	}
}

// Нет благоприятного среза → L_risk = L_safe → потолок +0.30.
func TestGridBonus_NoShortcutCap(t *testing.T) {
	f := GridField{LNaive: 40, LSafe: 30, LRisk: 30, M0Frac: 0.10}
	if got := f.bonusCostGrid(25); math.Abs(got-0.30) > 1e-9 {
		t.Errorf("no-shortcut cap = %v, want 0.30", got)
	}
}

// Валидность пути и причины отказов (§14.2).
func TestGridEvaluate_PathValidity(t *testing.T) {
	secret := []byte("eval")
	var f GridField
	found := false
	for seed := int64(0); seed < 50 && !found; seed++ {
		if ff, ok := GenerateGridField(seed, secret, 20000, calmPassport()); ok {
			f, found = ff, true
		}
	}
	if !found {
		t.Skip("нет принятого поля")
	}

	valid := f.visitAllGrid(f.Visible, true).path
	if len(valid) < 2 {
		t.Fatalf("поиск не дал пути")
	}
	bonus, ok, reason := EvaluateGridPath(f, valid)
	if !ok || reason != "" {
		t.Fatalf("валидный путь отвергнут: %q", reason)
	}
	if bonus < -0.20-1e-9 || bonus > 0.50+1e-9 {
		t.Errorf("bonus %.3f вне [-0.20, 0.50]", bonus)
	}

	check := func(name string, cells []int, wantReason string) {
		t.Helper()
		if _, ok, r := EvaluateGridPath(f, cells); ok || r != wantReason {
			t.Errorf("%s: ok=%v reason=%q, want %q", name, ok, r, wantReason)
		}
	}
	check("too_few", []int{f.Start}, GridReasonTooFewCells)
	check("out_of_bounds", []int{f.Start, f.N * f.N}, GridReasonCellOutOfBounds)
	check("start_mismatch", []int{f.Finish, f.Start}, GridReasonStartMismatch)
	check("finish_mismatch", []int{f.Start, f.Start + 1}, GridReasonFinishMismatch)
	check("not_adjacent", []int{f.Start, f.Start + 2, f.Finish}, GridReasonNotAdjacent)

	// Минимальный шаг (анти-бот §6.6): та же клетка дважды подряд → step_too_small.
	if dup := f.staircaseGrid(f.Start, f.Finish); len(dup) >= 2 {
		dup = append([]int{dup[0], dup[1]}, dup[1:]...)
		check("step_too_small", dup, GridReasonStepTooSmall)
	}

	// Лестница Start→Finish проходит не все маяки → beacon_missing.
	stair := f.staircaseGrid(f.Start, f.Finish)
	onPath := map[int]bool{}
	for _, c := range stair {
		onPath[c] = true
	}
	misses := false
	for _, b := range f.Beacons {
		if !onPath[b] {
			misses = true
		}
	}
	if misses {
		check("beacon_missing", stair, GridReasonBeaconMissing)
	}
}

// Сектор вскрывается только своим secret: свой воспроизводит содержимое,
// чужой — даёт отличие хотя бы в одном поле серии; индекс вне диапазона — ошибка.
func TestGridReveal_OwnSecretOnly(t *testing.T) {
	secret := []byte("route-secret-1")
	f, ok := GenerateGridField(7, secret, 15000, calmPassport())
	if !ok || len(f.Sectors) == 0 {
		t.Skip("нет поля/секторов")
	}
	layout := f.Layout()
	for i := range f.Sectors {
		got, err := RevealGridSector(secret, layout, i)
		if err != nil {
			t.Fatalf("reveal %d: %v", i, err)
		}
		if got != f.Sectors[i].content.String() {
			t.Errorf("сектор %d: reveal %q != хранимое %q", i, got, f.Sectors[i].content)
		}
	}
	if _, err := RevealGridSector(secret, layout, len(f.Sectors)); err == nil {
		t.Errorf("индекс за границей не дал ошибки")
	}
	if _, err := RevealGridSector(secret, layout, -1); err == nil {
		t.Errorf("отрицательный индекс не дал ошибки")
	}

	diff := 0
	for seed := int64(0); seed < 25; seed++ {
		ff, ok := GenerateGridField(seed, secret, 15000, calmPassport())
		if !ok || len(ff.Sectors) == 0 {
			continue
		}
		lay := ff.Layout()
		for i := range ff.Sectors {
			wrong, err := RevealGridSector([]byte("other-secret"), lay, i)
			if err != nil {
				t.Fatalf("reveal чужим secret: %v", err)
			}
			if wrong != ff.Sectors[i].content.String() {
				diff++
				break
			}
		}
	}
	if diff == 0 {
		t.Errorf("чужой secret вскрыл те же секторы во всех полях серии")
	}
}

// Скрытое содержимое и realized не утекают в JSON (public layout).
func TestGridLayout_NoContentLeak(t *testing.T) {
	f, ok := GenerateGridField(3, []byte("leak-check"), 15000, calmPassport())
	if !ok {
		t.Skip("нет поля")
	}
	raw, err := json.Marshal(f.Layout())
	if err != nil {
		t.Fatalf("marshal layout: %v", err)
	}
	if strings.Contains(string(raw), "content") || strings.Contains(string(raw), "realized") {
		t.Errorf("layout раскрывает скрытое: %s", raw)
	}
	fraw, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal field: %v", err)
	}
	if strings.Contains(string(fraw), "realized") || strings.Contains(string(fraw), "content") {
		t.Errorf("field раскрывает скрытое: %s", fraw)
	}
}
