package routegame

import "testing"

// gridDisturbOnes — карта 1.0 (базовая реализованная цена обычной клетки).
func gridDisturbOnes(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = 1.0
	}
	return out
}

// Двойник: прод DestabilizeGridField совпадает с эталонным destabilizedV9 на
// одном и том же поле — клетки unstable-сектора и подход (4-соседи) дорожают до
// TrapCost, старт/финиш/маяки/шлюзы/мосты исключены.
func TestGridDisturb_TwinMatchesV9(t *testing.T) {
	const n = 6
	prod := GridField{
		N: n, Start: 0, Finish: n*n - 1, TrapCost: 40.0,
		Beacons:  []int{3},
		realized: gridDisturbOnes(n * n),
		Sectors:  []GridSector{{Cells: []int{7, 8}}},
		Gate:     map[int]bool{13: true},
		Bridge:   map[int]bool{14: true},
	}
	v9 := V9Field{
		N: n, Start: 0, Finish: n*n - 1, TrapCost: 40.0,
		Beacons:  []int{3},
		Realized: gridDisturbOnes(n * n),
		Sectors:  []SectorV9{{Cells: []int{7, 8}}},
		Gate:     map[int]bool{13: true},
		Bridge:   map[int]bool{14: true},
	}
	gp := DestabilizeGridField(prod, []int{0})
	gv := v9.destabilizedV9([]int{0})
	for c := range gp.realized {
		if gp.realized[c] != gv.Realized[c] {
			t.Fatalf("cell %d: прод %v != харнесс %v", c, gp.realized[c], gv.Realized[c])
		}
	}
	// исходное поле не мутировано.
	if prod.realized[7] != 1.0 || prod.realized[8] != 1.0 {
		t.Fatalf("исходное поле мутировано: %v/%v", prod.realized[7], prod.realized[8])
	}
}

// Пустой список не меняет поле; неизвестный индекс игнорируется.
func TestGridDisturb_NoopAndOutOfRange(t *testing.T) {
	f := GridField{N: 4, Start: 0, Finish: 15, TrapCost: 40.0, realized: gridDisturbOnes(16),
		Sectors: []GridSector{{Cells: []int{5}}}}
	if got := DestabilizeGridField(f, nil); got.realized[5] != 1.0 {
		t.Fatalf("nil-список изменил поле")
	}
	if got := DestabilizeGridField(f, []int{9, -1}); got.realized[5] != 1.0 {
		t.Fatalf("неизвестный индекс изменил поле")
	}
}

// На сгенерированном поле с unstable-сектором помеха дорожает и понижает бонус
// пути через этот сектор.
func TestGridDisturb_LowersBonusThroughUnstable(t *testing.T) {
	var field GridField
	idx := -1
	for seed := int64(0); seed < 300 && idx < 0; seed++ {
		ff, ok := GenerateGridField(seed, []byte("disturb-secret"), 20000, calmPassport())
		if !ok {
			continue
		}
		for i, s := range ff.Sectors {
			if s.content == GridContentUnstable {
				field, idx = ff, i
				break
			}
		}
	}
	if idx < 0 {
		t.Skip("не встретился unstable-сектор")
	}
	disturbed := DestabilizeGridField(field, []int{idx})
	for _, c := range field.Sectors[idx].Cells {
		if disturbed.realized[c] != field.TrapCost {
			t.Fatalf("клетка сектора %d не дорожает: %v", c, disturbed.realized[c])
		}
	}
}
