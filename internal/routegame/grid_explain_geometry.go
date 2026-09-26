// internal/routegame/grid_explain_geometry.go
// Класс «путь-геометрия» разбора (§14.13.3, факторы №1–10): подсчёт по самому
// пути и объектам поля. Вынесено из grid_explain.go без изменения поведения.
package routegame

// explainGeometry — детекторы №1 turn_cost … №10 current_against в порядке
// §14.13.3.
func explainGeometry(x *explainCtx) {
	field, cells, dirAt := x.field, x.cells, x.dirAt
	visit, enter, uniquePath := x.visit, x.enter, x.uniquePath

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
		x.add("turn_cost", GridFactorGroupError, turns-tref, corners)
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
	x.add("revisit", GridFactorGroupError, len(revisit), revisit)

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
		x.add("overshoot", GridFactorGroupError, finishVisits-1, []int{field.Finish})
	}

	// №4 mud_cost: шаги в Мглу.
	mudSteps := make([]int, 0)
	for t := 1; t < len(cells); t++ {
		if _, ok := field.Mud[cells[t]]; ok {
			mudSteps = append(mudSteps, cells[t])
		}
	}
	x.add("mud_cost", GridFactorGroupError, len(mudSteps), mudSteps)

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
	x.add("mud_entry_repeat", GridFactorGroupError, mudRepeat, mudRepeatCells)

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
	x.add("gate_reentry", GridFactorGroupError, gateRepeat, gateRepeatCells)

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
	x.add("gate_useless", GridFactorGroupError, len(gateUseless), gateUseless)

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
	x.add("bridge_reuse", GridFactorGroupError, bridgeRepeat, bridgeRepeatCells)

	// №9 dead_end: клетки Обрыва на пути.
	deadEnd := make([]int, 0)
	for _, c := range uniquePath {
		if field.DeadEnd[c] {
			deadEnd = append(deadEnd, c)
		}
	}
	x.add("dead_end", GridFactorGroupError, len(deadEnd), deadEnd)

	// №10 current_against: вход против направления течения.
	against := make([]int, 0)
	for t := 1; t < len(cells); t++ {
		c := cells[t]
		if d, ok := field.CurrentDir[c]; ok && dirAt[t] != d {
			against = append(against, c)
		}
	}
	x.add("current_against", GridFactorGroupError, len(against), against)
}
