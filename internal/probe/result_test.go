package probe

import (
	"math"
	"testing"
)

func mkCurves(lifetimes []float64) []Curve {
	out := make([]Curve, len(lifetimes))
	for i, lt := range lifetimes {
		out[i] = Curve{Lifetime: lt, Class: ClassEarthlike}
	}
	return out
}

func TestComputeStatsEmpty(t *testing.T) {
	s := ComputeStats(nil, 1000)
	if s.Count != 0 || s.MedianDays != 0 || len(s.Histogram) != 101 || len(s.Survival) != 101 {
		t.Fatalf("пустой вход должен давать пустые метрики: %+v", s)
	}
}

func TestComputeStatsQuantiles(t *testing.T) {
	s := ComputeStats(mkCurves([]float64{100, 200, 1000}), 1000)
	if s.MedianDays != 200 {
		t.Fatalf("медиана 200, получил %v", s.MedianDays)
	}
	if !almostEq(s.Q25Days, 150, 1e-9) {
		t.Fatalf("q25 = 150, получил %v", s.Q25Days)
	}
	if !almostEq(s.Q75Days, 600, 1e-9) {
		t.Fatalf("q75 = 600, получил %v", s.Q75Days)
	}
	if s.MinDays != 100 || s.MaxDays != 1000 {
		t.Fatalf("min/max: %v/%v", s.MinDays, s.MaxDays)
	}
	if s.ExtinctFraction != 1.0 {
		t.Fatalf("все умирают до горизонта, получил %v", s.ExtinctFraction)
	}
}

func TestComputeStatsExtinctFraction(t *testing.T) {
	// Горизонт 500: живут 100 и 200, живёт 1000.
	s := ComputeStats(mkCurves([]float64{100, 200, 1000}), 500)
	if s.ExtinctFraction != 2.0/3.0 {
		t.Fatalf("ожидал 2/3, получил %v", s.ExtinctFraction)
	}
}

func TestComputeStatsSurvival(t *testing.T) {
	s := ComputeStats(mkCurves([]float64{100, 200}), 200)
	if len(s.Survival) == 0 || s.Survival[0].Alive != 1.0 {
		t.Fatalf("в t=0 все живы, получил %+v", s.Survival[:2])
	}
	// binWidth = 200/100 = 2. в t=50: живут 100 и 200 → 1.0
	at50 := s.Survival[25]
	if at50.Alive != 1.0 {
		t.Fatalf("в t=50 оба живы, получил %v", at50.Alive)
	}
	// в t=150: живёт только 200 → 0.5
	at150 := s.Survival[75]
	if at150.Alive != 0.5 {
		t.Fatalf("в t=150 жив один, получил %v", at150.Alive)
	}
	// в t=200: никто → 0
	at200 := s.Survival[100]
	if at200.Alive != 0 {
		t.Fatalf("в t=200 никто, получил %v", at200.Alive)
	}
}

func TestComputeStatsBornDead(t *testing.T) {
	s := ComputeStats(mkCurves([]float64{0, 0, 100}), 1000)
	if s.MedianDays != 0 {
		t.Fatalf("две мёртвые с рождения → медиана 0, получил %v", s.MedianDays)
	}
	if s.Survival[0].Alive != 1.0/3.0 {
		t.Fatalf("в t=0 жива треть, получил %v", s.Survival[0].Alive)
	}
}

func TestComputeStatsGroups(t *testing.T) {
	cs := []Curve{
		{Lifetime: 100, Class: ClassEarthlike},
		{Lifetime: 300, Class: ClassEarthlike},
		{Lifetime: 500, Class: ClassIce},
	}
	s := ComputeStats(cs, 1000)
	earth := s.Groups[ClassEarthlike]
	if earth.Count != 2 || earth.MedianDays != 200 {
		t.Fatalf("землеподобные: count 2, медиана 200; получил %+v", earth)
	}
	ice := s.Groups[ClassIce]
	if ice.Count != 1 || ice.MedianDays != 500 {
		t.Fatalf("ледяные: count 1, медиана 500; получил %+v", ice)
	}
}

func TestComputeStatsHistogram(t *testing.T) {
	s := ComputeStats(mkCurves([]float64{50, 150, 2000}), 1000)
	// binWidth = 10. 50 → бин 5, 150 → бин 15, 2000 → бин 100 (пережил горизонт).
	if s.Histogram[5].Count != 1 || s.Histogram[15].Count != 1 {
		t.Fatalf("бины по 10 суток: %+v", s.Histogram[:20])
	}
	overflow := s.Histogram[len(s.Histogram)-1]
	if overflow.Count != 1 || !almostEq(overflow.Lo, 1000, 1e-9) || math.IsInf(overflow.Hi, 0) {
		t.Fatalf("переживший горизонт — в последний бин, Hi не должен быть Inf: %+v", overflow)
	}
}