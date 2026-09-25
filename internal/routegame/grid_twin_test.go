package routegame

import "testing"

// grid_twin_test.go — подтверждение «двойника»: производственная модель
// (grid_model.go/grid_search.go/grid_place.go) и замерный харнесс v9
// (gridv9_*_test.go) — один алгоритм. Тест доказывает, что ПУБЛИЧНАЯ геометрия
// поля (маяки, русла, стена/проход, топь, спецобъекты, тупик, режим, карта цен)
// совпадает при одном seed/dist/паспорте: RNG-поток до секторов и функции
// размещения — копия.
//
// Скрытый слой расходится НАМЕРЕННО (см. отчёт подэтапа A): прод берёт
// раскладку секторов из публичного seed, а содержимое — из серверного secret;
// харнесс сеял и то, и другое из seed. Поэтому побайтовое совпадение якорей
// L_naive/L_safe/L_risk и секторов не воспроизводится — это не дефект, а
// требуемое разделение слоёв (public layout vs hidden content).
func TestGridTwin_PublicGeometryMatchesV9(t *testing.T) {
	cfg := defaultGridConfig()
	v9 := DefaultV9Config()
	const mixed int64 = 0x123456789ABC
	for _, p := range []Passport{calmPassport(), restlessPassport()} {
		for _, dist := range []float64{10, 20000, 120000} {
			prod := buildGridField(mixed, 424242, dist, p, cfg)
			har := buildFieldV9(mixed, dist, p, v9)
			if prod.N != har.N || prod.Start != har.Start || prod.Finish != har.Finish {
				t.Fatalf("dist %v: базовая геометрия разошлась", dist)
			}
			if !gridEqualInts(prod.Beacons, har.Beacons) {
				t.Fatalf("dist %v: маяки разошлись: %v vs %v", dist, prod.Beacons, har.Beacons)
			}
			if !gridEqualBoolMaps(prod.Lane, har.Lane) || !gridEqualBoolMaps(prod.Wall, har.Wall) ||
				!gridEqualBoolMaps(prod.Gate, har.Gate) || !gridEqualBoolMaps(prod.Bridge, har.Bridge) ||
				!gridEqualBoolMaps(prod.DeadEnd, har.DeadEnd) || !gridEqualBoolMaps(prod.Bottleneck, har.Bottleneck) {
				t.Fatalf("dist %v: объекты поля разошлись", dist)
			}
			if !gridEqualIntMaps(prod.CurrentDir, har.CurrentDir) {
				t.Fatalf("dist %v: течения разошлись", dist)
			}
			if !gridEqualFloatMaps(prod.Mud, har.Mud) {
				t.Fatalf("dist %v: топь разошлась", dist)
			}
			if !gridEqualFloats(prod.Visible, har.Visible) {
				t.Fatalf("dist %v: карта публичных цен разошлась", dist)
			}
			if prod.Mode != har.Mode {
				t.Fatalf("dist %v: режим разошёлся: %q vs %q", dist, prod.Mode, har.Mode)
			}
		}
	}
}

func gridEqualInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func gridEqualFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func gridEqualBoolMaps(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func gridEqualIntMaps(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func gridEqualFloatMaps(a, b map[int]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Скрытый слой сеется по-разному (public layout из seed vs content из secret),
// но ЗАКОН распределения — копия замера: таблицы содержимого σ→content и
// поправки на видимое окружение совпадают с харнессом §14.3.
func TestGridTwin_ContentTablesMatchV9(t *testing.T) {
	for sig := range gridContentTable {
		if gridContentTable[sig] != V9ContentTable[SectorSigV9(sig)] {
			t.Fatalf("σ=%v: таблица содержимого разошлась", sig)
		}
	}
	for sig := GridSectorSig(0); sig <= GridSigNoisy; sig++ {
		for sur := GridSectorSurround(0); sur <= GridSurroundCurrent; sur++ {
			prod := gridSurroundShift(sig, sur)
			har := V9SurroundShift(SectorSigV9(sig), SectorSurroundV9(sur))
			if prod != har {
				t.Fatalf("σ=%v окружение=%v: поправка разошлась", sig, sur)
			}
		}
	}
}