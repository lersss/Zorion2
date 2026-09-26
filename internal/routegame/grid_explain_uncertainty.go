// internal/routegame/grid_explain_uncertainty.go
// Класс «решения под неопределённостью» разбора (§14.13.3, факторы №11–16) и
// положительные/нейтральные факторы (§14.13.4). Вынесено из grid_explain.go
// без изменения поведения.
package routegame

// explainUncertainty — детекторы №11 hidden_trap … №16 ping_destabilize, затем
// find_used/current_along/wall_cost (§14.13.4).
func explainUncertainty(x *explainCtx) {
	field, cells, onPath, dirAt := x.field, x.cells, x.onPath, x.dirAt
	revealed := x.revealed

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
	x.add("hidden_trap", GridFactorGroupError, hiddenTrap, hiddenCells)
	x.add("trap_entered_known", GridFactorGroupError, trapKnown, knownCells)

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
	x.add("decoy_penalty", GridFactorGroupError, decoy, decoyCells)

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
	x.add("lure_missed", GridFactorGroupError, lureMissed, lureMissedCells)

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
	x.add("ping_wasted", GridFactorGroupError, wasted, wastedCells)

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
			x.add("ping_destabilize", GridFactorGroupError, destab, destabErrCells)
		} else {
			x.add("ping_destabilize", GridFactorGroupNeutral, destab, destabNeutralCells)
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
	x.add("find_used", GridFactorGroupGain, findUsed, findUsedCells)

	// current_along: вход по направлению течения.
	along := make([]int, 0)
	for t := 1; t < len(cells); t++ {
		c := cells[t]
		if d, ok := field.CurrentDir[c]; ok && dirAt[t] == d {
			along = append(along, c)
		}
	}
	x.add("current_along", GridFactorGroupGain, len(along), along)

	// wall_cost: шаги в Помехи.
	wallSteps := make([]int, 0)
	for t := 1; t < len(cells); t++ {
		if field.Wall[cells[t]] {
			wallSteps = append(wallSteps, cells[t])
		}
	}
	x.add("wall_cost", GridFactorGroupNeutral, len(wallSteps), wallSteps)
}
