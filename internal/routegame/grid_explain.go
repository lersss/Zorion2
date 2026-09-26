// internal/routegame/grid_explain.go
// «Разбор по факторам» после необратимого boost (спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md, §14.13):
// именованный список того, что случилось на курсе. Чистая функция: без БД,
// HTTP, времени и глобального RNG; на bonus/применение не влияет. Детекция —
// 16 ошибок §14.13.3 + положительные/нейтральные факторы §14.13.4; полосы
// значимости — гипотеза §14.13.7 (при gain/neutral — 0).
package routegame

import "sort"

// Группы факторов разбора (§14.13.5).
const (
	GridFactorGroupError   = "error"
	GridFactorGroupGain    = "gain"
	GridFactorGroupNeutral = "neutral"
)

// gridFactorMaxCells — предел подсветки клеток на фактор (§14.13.5).
const gridFactorMaxCells = 16

// GridFactor — сработавший фактор разбора (§14.13.5). Code — стабильный
// машинный код (не локализуется); Severity — 1..4 у error и 0 у gain/neutral.
type GridFactor struct {
	Code     string `json:"code"`
	Group    string `json:"group"`
	Severity int    `json:"severity"`
	Count    int    `json:"count"`
	Cells    []int  `json:"cells"`
}

// gridFactorSeverity — полоса значимости по коду (§14.13.7): гипотеза
// калибровки, только для group=error. У gain/neutral полосы нет (severity 0).
var gridFactorSeverity = map[string]int{
	"revisit": 1, "overshoot": 1, "turn_cost": 1, "mud_cost": 1,
	"gate_useless": 2, "mud_entry_repeat": 2,
	"gate_reentry": 3, "bridge_reuse": 3, "dead_end": 3,
	"current_against": 3, "decoy_penalty": 3, "ping_destabilize": 3,
	"lure_missed": 4, "hidden_trap": 4, "ping_wasted": 4, "trap_entered_known": 4,
}

// GridSectorContentFromString — обратный к GridSectorContent.String: разбор
// содержимого вскрытого сектора из player_route_puzzle.revealed.
func GridSectorContentFromString(s string) (GridSectorContent, bool) {
	switch s {
	case "empty":
		return GridContentEmpty, true
	case "lure":
		return GridContentLure, true
	case "trap":
		return GridContentTrap, true
	case "decoy":
		return GridContentDecoy, true
	case "jackpot":
		return GridContentJackpot, true
	case "unstable":
		return GridContentUnstable, true
	}
	return GridContentEmpty, false
}

// ExplainGridPath — разбор провалидированного пути по факторам (§14.13.3/
// §14.13.4). field — поле УЖЕ с применённой помехой (как в boost); cells —
// присланный путь; revealed — множество вскрытых секторов (sector → content,
// пустой/повтор допустим — set-семантика). Возвращает только сработавшие
// факторы, детерминированно, порядок: error → gain → neutral (внутри —
// severity desc, count desc, code asc). Пустой результат — не-nil `[]`.
func ExplainGridPath(field GridField, cells []int, revealed map[int]GridSectorContent) []GridFactor {
	out := make([]GridFactor, 0)
	if len(cells) < 2 {
		return out
	}

	visit := make(map[int]int, len(cells))
	enter := make(map[int]int, len(cells))
	onPath := make(map[int]bool, len(cells))
	for _, c := range cells {
		visit[c]++
		onPath[c] = true
	}
	for t := 1; t < len(cells); t++ {
		enter[cells[t]]++
	}
	dirAt := make([]int, len(cells))
	for t := 1; t < len(cells); t++ {
		dirAt[t] = field.dirOf(cells[t-1], cells[t])
	}

	// uniquePath — клетки пути в порядке первого появления (для фактор-агрегатов).
	uniquePath := make([]int, 0, len(cells))
	seen := make(map[int]bool, len(cells))
	for _, c := range cells {
		if !seen[c] {
			seen[c] = true
			uniquePath = append(uniquePath, c)
		}
	}

	add := func(code, group string, count int, cs []int) {
		if count <= 0 {
			return
		}
		sev := 0
		if group == GridFactorGroupError {
			sev = gridFactorSeverity[code]
		}
		if cs == nil {
			cs = []int{}
		}
		if len(cs) > gridFactorMaxCells {
			cs = cs[:gridFactorMaxCells]
		}
		out = append(out, GridFactor{Code: code, Group: group, Severity: sev, Count: count, Cells: cs})
	}

	// ---- Класс «путь-геометрия» (10) ----

	// №1 turn_cost: манёвры пути против канонической лестницы через маяки в
	// порядке игрока (Tref — только эталон числа манёвров).
	turns := 0
	for t := 2; t < len(cells); t++ {
		if dirAt[t] != dirAt[t-1] {
			turns++
		}
	}
	beaconOrder := make([]int, 0, len(field.Beacons))
	seenBeacon := make(map[int]bool, len(field.Beacons))
	for _, c := range cells {
		if isBeaconGrid(&field, c) && !seenBeacon[c] {
			seenBeacon[c] = true
			beaconOrder = append(beaconOrder, c)
		}
	}
	tref := gridTurnCount(field, field.buildRouteStairGrid(beaconOrder))
	if turns > tref {
		corners := make([]int, 0, turns)
		for t := 2; t < len(cells); t++ {
			if dirAt[t] != dirAt[t-1] {
				corners = append(corners, cells[t-1])
			}
		}
		add("turn_cost", GridFactorGroupError, turns-tref, corners)
	}

	// №2 revisit: повторные клетки без собственного кода повтора.
	revisit := make([]int, 0)
	for _, c := range uniquePath {
		if c == field.Finish || visit[c] < 2 {
			continue
		}
		if field.Gate[c] || field.Bridge[c] || field.DeadEnd[c] {
			continue
		}
		if _, ok := field.Mud[c]; ok {
			continue
		}
		revisit = append(revisit, c)
	}
	add("revisit", GridFactorGroupError, len(revisit), revisit)

	// №3 overshoot: первое вхождение Finish — не последнее.
	firstFinish, finishVisits := -1, 0
	for idx, c := range cells {
		if c == field.Finish {
			if firstFinish < 0 {
				firstFinish = idx
			}
			finishVisits++
		}
	}
	if firstFinish >= 0 && firstFinish < len(cells)-1 {
		add("overshoot", GridFactorGroupError, finishVisits-1, []int{field.Finish})
	}

	// №4 mud_cost: шаги в Мглу.
	mudSteps := make([]int, 0)
	for t := 1; t < len(cells); t++ {
		if _, ok := field.Mud[cells[t]]; ok {
			mudSteps = append(mudSteps, cells[t])
		}
	}
	add("mud_cost", GridFactorGroupError, len(mudSteps), mudSteps)

	// №5 mud_entry_repeat: повторные входы в Мглу.
	mudRepeat, mudRepeatCells := 0, make([]int, 0)
	for _, c := range uniquePath {
		if _, ok := field.Mud[c]; !ok {
			continue
		}
		if e := enter[c]; e >= 2 {
			mudRepeat += e - 1
			mudRepeatCells = append(mudRepeatCells, c)
		}
	}
	add("mud_entry_repeat", GridFactorGroupError, mudRepeat, mudRepeatCells)

	// №6 gate_reentry: повторные входы в Кордон.
	gateRepeat, gateRepeatCells := 0, make([]int, 0)
	for _, c := range uniquePath {
		if !field.Gate[c] {
			continue
		}
		if e := enter[c]; e >= 2 {
			gateRepeat += e - 1
			gateRepeatCells = append(gateRepeatCells, c)
		}
	}
	add("gate_reentry", GridFactorGroupError, gateRepeat, gateRepeatCells)

	// №7 gate_useless: вход и выход тем же ходом (enter == 1, path[t−1] == path[t+1]).
	gateUseless := make([]int, 0)
	for t := 1; t <= len(cells)-2; t++ {
		c := cells[t]
		if !field.Gate[c] || enter[c] != 1 {
			continue
		}
		if cells[t-1] == cells[t+1] {
			gateUseless = append(gateUseless, c)
		}
	}
	add("gate_useless", GridFactorGroupError, len(gateUseless), gateUseless)

	// №8 bridge_reuse: повторные входы в Тоннель.
	bridgeRepeat, bridgeRepeatCells := 0, make([]int, 0)
	for _, c := range uniquePath {
		if !field.Bridge[c] {
			continue
		}
		if e := enter[c]; e >= 2 {
			bridgeRepeat += e - 1
			bridgeRepeatCells = append(bridgeRepeatCells, c)
		}
	}
	add("bridge_reuse", GridFactorGroupError, bridgeRepeat, bridgeRepeatCells)

	// №9 dead_end: клетки Обрыва на пути.
	deadEnd := make([]int, 0)
	for _, c := range uniquePath {
		if field.DeadEnd[c] {
			deadEnd = append(deadEnd, c)
		}
	}
	add("dead_end", GridFactorGroupError, len(deadEnd), deadEnd)

	// №10 current_against: вход против направления течения.
	against := make([]int, 0)
	for t := 1; t < len(cells); t++ {
		c := cells[t]
		if d, ok := field.CurrentDir[c]; ok && dirAt[t] != d {
			against = append(against, c)
		}
	}
	add("current_against", GridFactorGroupError, len(against), against)

	// ---- Класс «решения под неопределённостью» (6) ----

	contentOf := func(i int) GridSectorContent {
		if c, ok := revealed[i]; ok {
			return c
		}
		return field.Sectors[i].content
	}
	isRevealed := func(i int) bool {
		_, ok := revealed[i]
		return ok
	}
	cellsOnPath := func(s GridSector) []int {
		on := make([]int, 0, len(s.Cells))
		for _, c := range s.Cells {
			if onPath[c] {
				on = append(on, c)
			}
		}
		return on
	}

	// №11/12 hidden_trap / trap_entered_known: Воронка на пути (по вскрытию).
	hiddenTrap, trapKnown := 0, 0
	hiddenCells, knownCells := make([]int, 0), make([]int, 0)
	for i, s := range field.Sectors {
		if contentOf(i) != GridContentTrap {
			continue
		}
		on := cellsOnPath(s)
		if len(on) == 0 {
			continue
		}
		if isRevealed(i) {
			trapKnown++
			knownCells = append(knownCells, on...)
		} else {
			hiddenTrap++
			hiddenCells = append(hiddenCells, on...)
		}
	}
	add("hidden_trap", GridFactorGroupError, hiddenTrap, hiddenCells)
	add("trap_entered_known", GridFactorGroupError, trapKnown, knownCells)

	// №13 decoy_penalty: Мираж на пути (вскрытие не меняет код, только текст).
	decoy, decoyCells := 0, make([]int, 0)
	for i, s := range field.Sectors {
		if contentOf(i) != GridContentDecoy {
			continue
		}
		on := cellsOnPath(s)
		if len(on) == 0 {
			continue
		}
		decoy++
		decoyCells = append(decoyCells, on...)
	}
	add("decoy_penalty", GridFactorGroupError, decoy, decoyCells)

	// №14 lure_missed: вскрытый Зов, ни одна клетка не на пути.
	lureMissed, lureMissedCells := 0, make([]int, 0)
	for i, s := range field.Sectors {
		if contentOf(i) != GridContentLure || !isRevealed(i) {
			continue
		}
		if len(cellsOnPath(s)) > 0 {
			continue
		}
		lureMissed++
		lureMissedCells = append(lureMissedCells, s.Cells...)
	}
	add("lure_missed", GridFactorGroupError, lureMissed, lureMissedCells)

	// №15 ping_wasted: вскрытый Вакуум; вскрытый Просвет мимо пути.
	wasted, wastedCells := 0, make([]int, 0)
	for i, s := range field.Sectors {
		if !isRevealed(i) {
			continue
		}
		c := contentOf(i)
		switch {
		case c == GridContentEmpty:
		case c == GridContentJackpot && len(cellsOnPath(s)) == 0:
		default:
			continue
		}
		wasted++
		wastedCells = append(wastedCells, s.Cells...)
	}
	add("ping_wasted", GridFactorGroupError, wasted, wastedCells)

	// №16 ping_destabilize: вскрытый Сбой; error, если помеха задела путь.
	destab, destabErrCells, destabNeutralCells := 0, make([]int, 0), make([]int, 0)
	for i, s := range field.Sectors {
		if contentOf(i) != GridContentUnstable || !isRevealed(i) {
			continue
		}
		destab++
		raised := gridRaisedCells(field, s)
		hit := false
		for _, c := range raised {
			if onPath[c] {
				hit = true
				break
			}
		}
		if hit {
			for _, c := range raised {
				if onPath[c] {
					destabErrCells = append(destabErrCells, c)
				}
			}
		} else {
			destabNeutralCells = append(destabNeutralCells, raised...)
		}
	}
	if destab > 0 {
		if len(destabErrCells) > 0 {
			add("ping_destabilize", GridFactorGroupError, destab, destabErrCells)
		} else {
			add("ping_destabilize", GridFactorGroupNeutral, destab, destabNeutralCells)
		}
	}

	// ---- Положительные и нейтральные (§14.13.4) ----

	// find_used: вскрытый Просвет/Зов с клеткой на пути.
	findUsed, findUsedCells := 0, make([]int, 0)
	for i, s := range field.Sectors {
		if !isRevealed(i) {
			continue
		}
		c := contentOf(i)
		if c != GridContentJackpot && c != GridContentLure {
			continue
		}
		on := cellsOnPath(s)
		if len(on) == 0 {
			continue
		}
		findUsed++
		findUsedCells = append(findUsedCells, on...)
	}
	add("find_used", GridFactorGroupGain, findUsed, findUsedCells)

	// current_along: вход по направлению течения.
	along := make([]int, 0)
	for t := 1; t < len(cells); t++ {
		c := cells[t]
		if d, ok := field.CurrentDir[c]; ok && dirAt[t] == d {
			along = append(along, c)
		}
	}
	add("current_along", GridFactorGroupGain, len(along), along)

	// wall_cost: шаги в Помехи.
	wallSteps := make([]int, 0)
	for t := 1; t < len(cells); t++ {
		if field.Wall[cells[t]] {
			wallSteps = append(wallSteps, cells[t])
		}
	}
	add("wall_cost", GridFactorGroupNeutral, len(wallSteps), wallSteps)

	gridSortFactors(out)
	return out
}

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
