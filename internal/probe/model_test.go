package probe

import (
	"math"
	"math/rand"
	"testing"
)

func almostEq(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}

func TestHabitabilityComfortZone(t *testing.T) {
	cp := DefaultCurveParams()
	// Землеподобная планета: комфортная температура, азотно-кислородная, много воды.
	hT, hA, hW, hP := cp.Habitability(280, 60, AtmoNitrogenOxygen)
	if hT != 1 || hA != 1 || hW != 1 || hP != 1 {
		t.Fatalf("ожидал все 1, получил %v %v %v %v", hT, hA, hW, hP)
	}
}

func TestHabitabilityTempDecay(t *testing.T) {
	cp := DefaultCurveParams() // зона 200..350, ширина 150
	if h, _, _, _ := cp.Habitability(200, 60, AtmoNitrogenOxygen); h != 1 {
		t.Fatalf("на границе T_min должен быть 1, получил %v", h)
	}
	h, _, _, _ := cp.Habitability(150, 60, AtmoNitrogenOxygen)
	if !almostEq(h, 1-0.9*(50.0/150.0), 1e-9) {
		t.Fatalf("ожидал 0.7, получил %v", h)
	}
	h, _, _, _ = cp.Habitability(50, 60, AtmoNitrogenOxygen)
	if !almostEq(h, 0.1, 1e-9) {
		t.Fatalf("далеко за границей должно быть 0.1, получил %v", h)
	}
	h, _, _, _ = cp.Habitability(500, 60, AtmoNitrogenOxygen)
	if !almostEq(h, 0.1, 1e-9) {
		t.Fatalf("выше T_max должно быть 0.1, получил %v", h)
	}
}

func TestHabitabilityAtmoPenalties(t *testing.T) {
	cp := DefaultCurveParams()
	if _, h, _, _ := cp.Habitability(280, 60, AtmoCO2); h != 0.5 {
		t.Fatalf("углекислая: ожидал 0.5, получил %v", h)
	}
	if _, h, _, _ := cp.Habitability(280, 60, AtmoToxic); h != 0.2 {
		t.Fatalf("ядовитая: ожидал 0.2, получил %v", h)
	}
	if _, h, _, _ := cp.Habitability(280, 60, "плотная"); h != 1.0 {
		t.Fatalf("неизвестная атмосфера без штрафа, получил %v", h)
	}
}

func TestHabitabilityWaterThreshold(t *testing.T) {
	cp := DefaultCurveParams() // порог 10, штраф 0.5
	if _, _, h, _ := cp.Habitability(280, 11, AtmoNitrogenOxygen); h != 1 {
		t.Fatalf("вода > 10%%: ожидал 1, получил %v", h)
	}
	if _, _, h, _ := cp.Habitability(280, 5, AtmoNitrogenOxygen); h != 0.5 {
		t.Fatalf("вода <= 10%%: ожидал 0.5, получил %v", h)
	}
}

func TestHabitabilityFloor(t *testing.T) {
	cp := DefaultCurveParams()
	// Ядовитая + сухая + холодно: произведение меньше 0.1, должен быть пол.
	_, _, _, hP := cp.Habitability(20, 0, AtmoToxic)
	if hP != 0.1 {
		t.Fatalf("пол 0.1, получил %v", hP)
	}
}

func TestGSeparate(t *testing.T) {
	cp := DefaultCurveParams()
	cp.HgMode = HgSeparate
	// Темп от температуры: при h_temp=0.5, g=2.
	if g := cp.G(1.0, 0.5); !almostEq(g, 2.0, 1e-9) {
		t.Fatalf("separate: ожидал g=2, получил %v", g)
	}
	// linked: g от h_planet.
	cp.HgMode = HgLinked
	if g := cp.G(0.5, 1.0); !almostEq(g, 2.0, 1e-9) {
		t.Fatalf("linked: ожидал g=2, получил %v", g)
	}
}

func TestNCritModes(t *testing.T) {
	cp := DefaultCurveParams()
	if v := cp.NCritValue(1000, 0.5); v != 10000 {
		t.Fatalf("constant: ожидал 10000, получил %v", v)
	}
	cp.NCritMode = NCritProportional
	if v := cp.NCritValue(1000, 0.5); v != 10000*1000 {
		t.Fatalf("proportional: ожидал coeff*p0, получил %v", v)
	}
	cp.NCritMode = NCritPlanet
	if v := cp.NCritValue(1000, 0.5); v != 10000*0.5 {
		t.Fatalf("planet: ожидал base*h, получил %v", v)
	}
}

func TestLifetimeLinear(t *testing.T) {
	cp := DefaultCurveParams() // α=1, k=0.05
	// p0=100000, ncrit=10000 → T=100000/10000/0.05=200
	tv := 200.0
	lt := cp.Lifetime(100000, 0.05, tv)
	want := tv * (1 - 100.0/100000.0)
	if !almostEq(lt, want, 1e-9) {
		t.Fatalf("ожидал %v, получил %v", want, lt)
	}
}

func TestLifetimeExponential(t *testing.T) {
	cp := DefaultCurveParams()
	cp.Alpha = 0
	lt := cp.Lifetime(100000, 0.05, 0)
	want := math.Log(100000.0/100.0) / 0.05
	if !almostEq(lt, want, 1e-9) {
		t.Fatalf("ожидал %v, получил %v", want, lt)
	}
}

func TestLifetimeWithDelay(t *testing.T) {
	cp := DefaultCurveParams()
	cp.T0 = 30
	lt := cp.Lifetime(100000, 0.05, 200)
	if !almostEq(lt, 30+200*(1-100.0/100000.0), 1e-9) {
		t.Fatalf("задержка должна добавляться к времени жизни")
	}
}

func TestLifetimeDeadAtBirth(t *testing.T) {
	cp := DefaultCurveParams()
	if lt := cp.Lifetime(50, 0.05, 200); lt != 0 {
		t.Fatalf("p0 <= N_dead: ожидал 0, получил %v", lt)
	}
}

func TestPopulationAt(t *testing.T) {
	cp := DefaultCurveParams() // α=1, T0=0
	c := Curve{P0: 100000, K: 0.05, T: 200, Alpha: 1, NDead: 100, T0: 0}
	if p := cp.PopulationAt(c, 0); p != 100000 {
		t.Fatalf("t=0: ожидал p0, получил %v", p)
	}
	if p := cp.PopulationAt(c, 100); !almostEq(p, 50000, 1) {
		t.Fatalf("линейная: на середине ~50000, получил %v", p)
	}
	if p := cp.PopulationAt(c, 200); p != 0 {
		t.Fatalf("в конце T население 0, получил %v", p)
	}
}

func TestPopulationAtDelay(t *testing.T) {
	cp := DefaultCurveParams()
	c := Curve{P0: 100000, K: 0.05, T: 200, Alpha: 1, NDead: 100, T0: 30}
	if p := cp.PopulationAt(c, 29); p != 100000 {
		t.Fatalf("до T0 население равно p0, получил %v", p)
	}
	if p := cp.PopulationAt(c, 31); p >= 100000 {
		t.Fatalf("после T0 должно падать, получил %v", p)
	}
}

func TestComputeCurveDeterministic(t *testing.T) {
	cp := DefaultCurveParams()
	r1 := rand.New(rand.NewSource(42))
	r2 := rand.New(rand.NewSource(42))
	c1 := cp.ComputeCurve("p1", "w1", "P1", ClassEarthlike, 280, 60, AtmoNitrogenOxygen, r1)
	c2 := cp.ComputeCurve("p1", "w1", "P1", ClassEarthlike, 280, 60, AtmoNitrogenOxygen, r2)
	if c1.P0 != c2.P0 || c1.Lifetime != c2.Lifetime {
		t.Fatalf("один seed — одна кривая: %v vs %v", c1, c2)
	}
}

func TestClassForType(t *testing.T) {
	cases := map[string]string{
		"землеподобная":  ClassEarthlike,
		"ледяная":        ClassIce,
		"вулканическая":  ClassVolcanic,
		"газовый гигант": ClassGas,
		"пустынная":      ClassOther,
	}
	for typ, want := range cases {
		if got := ClassForType(typ); got != want {
			t.Fatalf("%s: ожидал %s, получил %s", typ, want, got)
		}
	}
}