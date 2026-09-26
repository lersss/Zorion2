// internal/routegame/grid_explain_util.go
// Служебные помощники разбора по факторам (§14.13): эталон манёвров, клетки,
// поднятые помехой, и порядок массива факторов. Вынесено из grid_explain.go
// без изменения поведения.
package routegame

import "sort"

// gridTurnCount — число смен направления в пути (для Tref).
func gridTurnCount(field GridField, path []int) int {
	n := 0
	for t := 2; t < len(path); t++ {
		if field.dirOf(path[t-2], path[t-1]) != field.dirOf(path[t-1], path[t]) {
			n++
		}
	}
	return n
}

// gridRaisedCells — клетки, поднятые помехой unstable-сектора: сам сектор и
// его 4-соседи, кроме старта/финиша/маяков/кордонов/тоннелей (как в
// DestabilizeGridField).
func gridRaisedCells(field GridField, s GridSector) []int {
	out := make([]int, 0, len(s.Cells))
	for _, c := range s.Cells {
		out = append(out, c)
		for _, st := range field.neighbors4(c) {
			n := st.cell
			if n == field.Start || n == field.Finish || isBeaconGrid(&field, n) ||
				field.Gate[n] || field.Bridge[n] {
				continue
			}
			out = append(out, n)
		}
	}
	return out
}

// gridSortFactors — порядок массива (§14.13.5): error → gain → neutral; внутри
// группы severity desc, count desc, code asc. Не контракт для клиента.
func gridSortFactors(fs []GridFactor) {
	rank := func(g string) int {
		switch g {
		case GridFactorGroupError:
			return 0
		case GridFactorGroupGain:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(fs, func(i, j int) bool {
		ri, rj := rank(fs[i].Group), rank(fs[j].Group)
		if ri != rj {
			return ri < rj
		}
		if fs[i].Severity != fs[j].Severity {
			return fs[i].Severity > fs[j].Severity
		}
		if fs[i].Count != fs[j].Count {
			return fs[i].Count > fs[j].Count
		}
		return fs[i].Code < fs[j].Code
	})
}
