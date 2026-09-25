package routegame

import (
	"encoding/json"
	"strings"
	"testing"
)

func calmPassport() Passport {
	return Passport{
		Dist: 10000,
		From: PassportStar{SpectralClass: "K", StarType: "star", SystemType: "single", Temperature: 4000},
		To:   PassportStar{SpectralClass: "M", StarType: "star", SystemType: "single", Temperature: 3200},
	}
}

func restlessPassport() Passport {
	return Passport{
		Dist: 10000,
		From: PassportStar{StarType: "black_hole", SystemType: "multiple", Temperature: 20000},
		To:   PassportStar{StarType: "neutron", SystemType: "binary", Temperature: 15000},
		DestinationBelts: []PassportBelt{
			{Kind: "asteroid", Name: "Пояс", RadiusAU: 3, WidthAU: 0.5},
		},
	}
}

func countType(nodes []FieldNode, typ string) int {
	n := 0
	for _, node := range nodes {
		if node.Type == typ {
			n++
		}
	}
	return n
}

func TestHashSeed_StableAndDistinct(t *testing.T) {
	a := HashSeed("w1", "w2")
	if a != HashSeed("w1", "w2") {
		t.Fatalf("HashSeed not stable: %d != %d", a, HashSeed("w1", "w2"))
	}
	if a == HashSeed("w1", "w3") {
		t.Errorf("different target world produced same seed")
	}
	if a == HashSeed("w2", "w1") {
		t.Errorf("direction ignored: A→B seed equals B→A")
	}
	// Разделитель: ("ab","c") — не то же, что ("a","bc").
	if HashSeed("ab", "c") == HashSeed("a", "bc") {
		t.Errorf("separator missing: (ab,c) collides with (a,bc)")
	}
}

func TestGenerateField_DeterministicJSON(t *testing.T) {
	first, err := json.Marshal(GenerateField(42, 12345, calmPassport()))
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	second, err := json.Marshal(GenerateField(42, 12345, calmPassport()))
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("field not deterministic:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestGenerateField_DifferentSeedDifferentField(t *testing.T) {
	a, _ := json.Marshal(GenerateField(1, 10000, calmPassport()))
	b, _ := json.Marshal(GenerateField(2, 10000, calmPassport()))
	if string(a) == string(b) {
		t.Errorf("different seeds produced identical field")
	}
}

// Поле — чистая функция от (seed, dist, passport): живого остатка/прогресса/времени
// во входе нет, повторный вызов даёт то же поле.
func TestGenerateField_LiveStateNotAnInput(t *testing.T) {
	seed, dist := int64(777), 15000.5
	passport := calmPassport()
	var prev []byte
	for i := 0; i < 3; i++ {
		raw, err := json.Marshal(GenerateField(seed, dist, passport))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if prev != nil && string(prev) != string(raw) {
			t.Fatalf("call %d differs: %s vs %s", i, prev, raw)
		}
		prev = raw
	}
}

func TestGenerateField_CoordinatesInUnitSquare(t *testing.T) {
	field := GenerateField(99, 30000, restlessPassport())
	points := []Point{field.Start, field.Finish}
	for _, n := range field.Nodes {
		points = append(points, Point{X: n.X, Y: n.Y})
	}
	for _, z := range field.Zones {
		points = append(points, Point{X: z.X, Y: z.Y})
	}
	for i, p := range points {
		if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
			t.Errorf("point %d out of [0,1]: %+v", i, p)
		}
	}
}

func TestGenerateField_NodesAndZonesNonNil(t *testing.T) {
	field := GenerateField(5, 1000, Passport{})
	if field.Nodes == nil {
		t.Errorf("Nodes is nil, want non-nil")
	}
	if field.Zones == nil {
		t.Errorf("Zones is nil, want non-nil")
	}
	raw, err := json.Marshal(field)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"nodes":[`, `"zones":[`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("JSON missing %s: %s", key, raw)
		}
	}
}

func TestGenerateField_NoOverlapWithEndpoints(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		field := GenerateField(seed, 30000, restlessPassport())
		for _, n := range field.Nodes {
			if d := dist(Point{X: n.X, Y: n.Y}, field.Start); d < n.R-1e-9 {
				t.Errorf("seed %d: node overlaps START: d=%v r=%v", seed, d, n.R)
			}
			if d := dist(Point{X: n.X, Y: n.Y}, field.Finish); d < n.R-1e-9 {
				t.Errorf("seed %d: node overlaps FINISH: d=%v r=%v", seed, d, n.R)
			}
		}
		for _, z := range field.Zones {
			if d := dist(Point{X: z.X, Y: z.Y}, field.Start); d < z.R-1e-9 {
				t.Errorf("seed %d: zone overlaps START: d=%v r=%v", seed, d, z.R)
			}
			if d := dist(Point{X: z.X, Y: z.Y}, field.Finish); d < z.R-1e-9 {
				t.Errorf("seed %d: zone overlaps FINISH: d=%v r=%v", seed, d, z.R)
			}
		}
	}
}

func TestGenerateField_EndpointsSeparated(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		field := GenerateField(seed, 10000, calmPassport())
		if d := dist(field.Start, field.Finish); d < 0.5 {
			t.Errorf("seed %d: endpoints too close: %v", seed, d)
		}
	}
}

// «Беспокойный» паспорт даёт не меньше зон и ложных сигналов, чем «спокойный»
// при том же seed/dist; рост монотонен по «беспокойству».
func TestGenerateField_DifficultyMonotonic(t *testing.T) {
	const seed, d = int64(123), 20000.0

	calm := GenerateField(seed, d, calmPassport())
	restless := GenerateField(seed, d, restlessPassport())
	if len(restless.Zones) < len(calm.Zones) {
		t.Errorf("restless zones %d < calm zones %d", len(restless.Zones), len(calm.Zones))
	}
	if countType(restless.Nodes, falseSignalType) < countType(calm.Nodes, falseSignalType) {
		t.Errorf("restless false signals %d < calm %d",
			countType(restless.Nodes, falseSignalType), countType(calm.Nodes, falseSignalType))
	}

	// Монотонность по набору «беспокойства»: добавляем по одному фактору.
	passports := []Passport{
		calmPassport(),
		passportWith(func(p *Passport) { p.To.SystemType = "binary" }),
		passportWith(func(p *Passport) { p.To.SystemType = "binary"; p.To.Temperature = 9000 }),
		restlessPassport(),
	}
	prevZones, prevFalse := 0, 0
	for i, p := range passports {
		field := GenerateField(seed, d, p)
		z, f := len(field.Zones), countType(field.Nodes, falseSignalType)
		if i > 0 && (z < prevZones || f < prevFalse) {
			t.Errorf("unrest step %d not monotonic: zones %d<%d or false %d<%d", i, z, prevZones, f, prevFalse)
		}
		prevZones, prevFalse = z, f
	}
}

// Число обязательных маяков растёт с дальностью (3..6).
func TestGenerateField_BeaconsGrowWithDistance(t *testing.T) {
	prev := 0
	for _, d := range []float64{0, 8000, 16000, 24000, 40000} {
		field := GenerateField(7, d, calmPassport())
		n := countType(field.Nodes, beaconType)
		if n < beaconMin || n > beaconMax {
			t.Errorf("dist %v: beacons %d out of [%d,%d]", d, n, beaconMin, beaconMax)
		}
		if n < prev {
			t.Errorf("dist %v: beacons %d < previous %d", d, n, prev)
		}
		prev = n
	}
	if prev != beaconMax {
		t.Errorf("large distance: beacons %d, want %d", prev, beaconMax)
	}
}

func passportWith(apply func(*Passport)) Passport {
	p := calmPassport()
	apply(&p)
	return p
}
