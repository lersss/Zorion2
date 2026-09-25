package routegame

import (
	"math"
	"reflect"
	"testing"
)

func TestV9GenerateField_Deterministic(t *testing.T) {
	cfg := DefaultV9Config()
	a, okA := GenerateFieldV9(42, 12345, calmPassport(), cfg)
	b, okB := GenerateFieldV9(42, 12345, calmPassport(), cfg)
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

func TestV9GenerateField_DifferentSeed(t *testing.T) {
	cfg := DefaultV9Config()
	a, okA := GenerateFieldV9(1, 12000, calmPassport(), cfg)
	b, okB := GenerateFieldV9(2, 12000, calmPassport(), cfg)
	if !okA || !okB {
		t.Skip("один из seed даёт «игры нет»")
	}
	if reflect.DeepEqual(a, b) {
		t.Errorf("different seeds produced identical field")
	}
}

// Инварианты принятого поля: якоря единой шкалы §14.2 и объекты §14.3.
func TestV9GenerateField_Invariants(t *testing.T) {
	if testing.Short() {
		t.Skip("тяжёлый прогон: не -short")
	}
	cfg := DefaultV9Config()
	withGame := 0
	const seeds = 30
	for seed := int64(0); seed < seeds; seed++ {
		f, ok := GenerateFieldV9(seed, 20000, restlessPassport(), cfg)
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
		if len(f.Realized) != f.N*f.N || len(f.Visible) != f.N*f.N {
			t.Fatalf("seed %d: maps size mismatch", seed)
		}
		if len(f.Sectors) != cfg.SectorCount {
			t.Fatalf("seed %d: sectors %d, want %d", seed, len(f.Sectors), cfg.SectorCount)
		}
		noisy := 0
		for _, s := range f.Sectors {
			if s.Sig < SigQuietV9 || s.Sig > SigNoisyV9 {
				t.Fatalf("seed %d: bad sig %d", seed, s.Sig)
			}
			if s.Sig == SigNoisyV9 {
				noisy++
			}
			if s.Content < ContentEmptyV9 || s.Content > ContentUnstableV9 {
				t.Fatalf("seed %d: bad content %d", seed, s.Content)
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

// Кривая bonus(C): якоря и клампинг §14.2 (низ через m0 = 0.10·L_naive).
func TestV9Bonus_Curve(t *testing.T) {
	f := V9Field{LNaive: 40, LSafe: 30, LRisk: 20, M0Frac: 0.10}
	cases := []struct{ C, want float64 }{
		{40, 0.10},        // ленивый = пол
		{30, 0.30},        // безопасный обход = +0.30
		{20, 0.50},        // реализованный оптимум со срезом
		{35, 0.20},        // середина u = 5/10
		{25, 0.40},        // середина w = 5/10
		{44, 0.10 - 0.30}, // ниже пола на пределе m0=4
		{80, 0.10 - 0.30}, // кламп снизу
	}
	for _, tc := range cases {
		if got := f.bonusCostV9(tc.C); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("bonusCost(%v) = %v, want %v", tc.C, got, tc.want)
		}
	}
	prev := math.Inf(1)
	for C := 15.0; C <= 50.0; C += 0.25 {
		b := f.bonusCostV9(C)
		if b > prev+1e-12 {
			t.Fatalf("bonus not non-increasing at C=%.2f (%.4f > %.4f)", C, b, prev)
		}
		prev = b
	}
}

// Нет благоприятного среза → L_risk = L_safe → потолок +0.30.
func TestV9Bonus_NoShortcutCap(t *testing.T) {
	f := V9Field{LNaive: 40, LSafe: 30, LRisk: 30, M0Frac: 0.10}
	if got := f.bonusCostV9(25); math.Abs(got-0.30) > 1e-9 {
		t.Errorf("no-shortcut cap = %v, want 0.30", got)
	}
}

// Пол L0 = +0.10; Safe = +0.30; верх не выше +0.50; X не хуже/не лучше линии Safe.
func TestV9Analyze_Bounds(t *testing.T) {
	if testing.Short() {
		t.Skip("тяжёлый прогон: не -short")
	}
	cfg := DefaultV9Config()
	checked := 0
	for seed := int64(0); seed < 40 && checked < 12; seed++ {
		f, ok := GenerateFieldV9(seed, 30000, restlessPassport(), cfg)
		if !ok {
			continue
		}
		checked++
		m := AnalyzeV9(f, cfg)
		if math.Abs(m.BonusL0-0.10) > 1e-9 {
			t.Errorf("seed %d: L0 bonus %.3f, want +0.10", seed, m.BonusL0)
		}
		if math.Abs(m.BonusSafe-0.30) > 1e-9 {
			t.Errorf("seed %d: Safe bonus %.3f, want +0.30", seed, m.BonusSafe)
		}
		for name, b := range map[string]float64{"S": m.BonusS, "X": m.BonusX, "I": m.BonusI, "B": m.BonusB, "Oracle": m.BonusOracle} {
			if b > 0.50+1e-9 {
				t.Errorf("seed %d: %s bonus %.3f > +0.50", seed, name, b)
			}
			if b < -0.20-1e-9 {
				t.Errorf("seed %d: %s bonus %.3f < -0.20", seed, name, b)
			}
		}
	}
	if checked == 0 {
		t.Skip("нет принятых полей")
	}
}

// Подпись σ — публичная и детерминированная; содержимое зависит от secret.
func TestV9Sectors_SigPublicContentNoisy(t *testing.T) {
	cfg := DefaultV9Config()
	a, ok := GenerateFieldV9(7, 15000, calmPassport(), cfg)
	if !ok {
		t.Skip("нет поля")
	}
	b, _ := GenerateFieldV9(7, 15000, calmPassport(), cfg)
	if !reflect.DeepEqual(a.Sectors, b.Sectors) {
		t.Errorf("sectors not deterministic")
	}
	varied := false
	for seed := int64(8); seed < 25 && !varied; seed++ {
		c, okc := GenerateFieldV9(seed, 15000, calmPassport(), cfg)
		if okc && !reflect.DeepEqual(a.Sectors, c.Sectors) {
			varied = true
		}
	}
	if !varied {
		t.Errorf("sectors identical across seeds")
	}
}

// Громкая σ чаще даёт jackpot и trap; тихая — empty/decoy (статистика по seed).
func TestV9Content_NoisyIsLouder(t *testing.T) {
	if testing.Short() {
		t.Skip("тяжёлый прогон: не -short")
	}
	cfg := DefaultV9Config()
	var noisyJack, noisyTrap, quietJack, quietTrap, noisyN, quietN int
	for seed := int64(0); seed < 400; seed++ {
		f, ok := GenerateFieldV9(seed, 20000, calmPassport(), cfg)
		if !ok {
			continue
		}
		for _, s := range f.Sectors {
			switch s.Sig {
			case SigNoisyV9:
				noisyN++
				if s.Content == ContentJackpotV9 {
					noisyJack++
				}
				if s.Content == ContentTrapV9 {
					noisyTrap++
				}
			case SigQuietV9:
				quietN++
				if s.Content == ContentJackpotV9 {
					quietJack++
				}
				if s.Content == ContentTrapV9 {
					quietTrap++
				}
			}
		}
	}
	if noisyN < 50 || quietN < 50 {
		t.Skipf("мало данных: noisy=%d quiet=%d", noisyN, quietN)
	}
	pNoisyJack := float64(noisyJack) / float64(noisyN)
	pQuietJack := float64(quietJack) / float64(quietN)
	pNoisyTrap := float64(noisyTrap) / float64(noisyN)
	pQuietTrap := float64(quietTrap) / float64(quietN)
	if pNoisyJack <= pQuietJack {
		t.Errorf("noisy jackpot %.3f not above quiet %.3f", pNoisyJack, pQuietJack)
	}
	if pNoisyTrap <= pQuietTrap {
		t.Errorf("noisy trap %.3f not above quiet %.3f", pNoisyTrap, pQuietTrap)
	}
}

// Зонд неустойчивого сектора активирует помеху: его цена растёт (R2).
func TestV9Destabilize_RaisesCost(t *testing.T) {
	if testing.Short() {
		t.Skip("тяжёлый прогон: не -short")
	}
	cfg := DefaultV9Config()
	var tested int
	for seed := int64(0); seed < 200 && tested < 5; seed++ {
		f, ok := GenerateFieldV9(seed, 20000, restlessPassport(), cfg)
		if !ok {
			continue
		}
		for i, s := range f.Sectors {
			if s.Content != ContentUnstableV9 {
				continue
			}
			g := f.destabilizedV9([]int{i})
			if g.Realized[s.Cells[0]] <= f.Realized[s.Cells[0]] {
				t.Errorf("seed %d: destabilize did not raise cost", seed)
			}
			tested++
			break
		}
	}
	if tested == 0 {
		t.Skip("не встретился unstable")
	}
}

func TestV9ComplexityFunctions(t *testing.T) {
	cfg := DefaultV9Config()
	if got := beaconCountV9(0, cfg); got != cfg.KMin {
		t.Errorf("beaconCount(0) = %d, want %d", got, cfg.KMin)
	}
	if got := hazardCountV9(1e6, 7, cfg); got != cfg.HMax {
		t.Errorf("hazardCount(large) = %d, want %d", got, cfg.HMax)
	}
}
