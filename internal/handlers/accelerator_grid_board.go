package handlers

import (
	"encoding/json"
	"sort"

	"zorion/internal/routegame"
)

// accelerator_grid_board.go — сборка публичной доски v9 (§14.8): геометрия,
// объекты, подписи секторов и вскрытые секторы. Вынесено из
// accelerator_grid_handlers.go без изменения поведения.

// acceleratorGridBoard — публичный слой поля. Клетки объектов — срезами в
// детерминированном порядке (map'ы RNG-нестабильны, JSON должен быть стабилен);
// цены — в Visible.
func acceleratorGridBoard(field routegame.GridField) acceleratorBoard {
	b := acceleratorBoard{
		N:          field.N,
		Start:      field.Start,
		Finish:     field.Finish,
		Beacons:    append([]int{}, field.Beacons...),
		Visible:    append([]float64{}, field.Visible...),
		Lane:       gridCellList(field.Lane),
		Wall:       gridCellList(field.Wall),
		Mud:        gridMudCellList(field.Mud),
		Gate:       gridCellList(field.Gate),
		Bridge:     gridCellList(field.Bridge),
		DeadEnd:    gridCellList(field.DeadEnd),
		Bottleneck: gridCellList(field.Bottleneck),
		Sectors:    make([]acceleratorSector, 0, len(field.Sectors)),
		Mode:       field.Mode,
	}
	for c, d := range field.CurrentDir {
		b.Current = append(b.Current, acceleratorCurrent{Cell: c, Dir: d})
	}
	sort.Slice(b.Current, func(i, j int) bool { return b.Current[i].Cell < b.Current[j].Cell })
	for _, s := range field.Sectors {
		b.Sectors = append(b.Sectors, acceleratorSector{
			Cells:        append([]int{}, s.Cells...),
			Sig:          int(s.Sig),
			SigName:      s.Sig.String(),
			Surround:     int(s.Surround),
			SurroundName: s.Surround.String(),
		})
	}
	return b
}

// gridCellList — ключи map[int]bool отсортированным срезом (стабильный JSON).
func gridCellList(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	sort.Ints(out)
	return out
}

// gridMudCellList — клетки топи (цена — в Visible) отсортированным срезом.
func gridMudCellList(m map[int]float64) []int {
	out := make([]int, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	sort.Ints(out)
	return out
}

// acceleratorRevealedList — revealed из БД в список вскрытых; пусто/битый JSON
// → пустой не-nil список (клиент получает []).
func acceleratorRevealedList(raw []byte) []acceleratorRevealed {
	out := make([]acceleratorRevealed, 0)
	if len(raw) == 0 {
		return out
	}
	var parsed []acceleratorRevealed
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed == nil {
		return out
	}
	return parsed
}
