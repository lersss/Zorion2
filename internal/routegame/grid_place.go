// internal/routegame/grid_place.go
// Генерация геометрии и скрытого слоя поля v9 «Планшет» (§14): стена с проходом,
// русла, топь, спецобъекты (шлюз, мост, течение), тупиковое русло; секторы
// (публичная σ из seed + скрытое содержимое из secret). Только чистые функции.
package routegame

import (
	"math"
	"math/rand"
)

// gridPlaceWall — дорогая стена по средней колонке с проходом (bottleneck).
func gridPlaceWall(rng *rand.Rand, cfg gridConfig, start, finish int) (map[int]bool, []int) {
	n := cfg.N
	out := map[int]bool{}
	if cfg.WallCost <= 0 {
		return out, nil
	}
	w := n / 2
	gap := cfg.WallGapRow
	if gap < 0 {
		gap = 1 + rng.Intn(max(n-3, 1))
	}
	gap = gridClampInt(gap, 1, n-3)
	var gaps []int
	for j := 1; j <= n-2; j++ {
		c := j*n + w
		if j == gap || j == gap+1 {
			gaps = append(gaps, c)
			continue
		}
		if c == start || c == finish {
			continue
		}
		out[c] = true
	}
	return out, gaps
}

// gridAdjustWallGap — переносит проход (bottleneck) в строку, где трасса
// публичного оптимума пересекает колонку стены.
func gridAdjustWallGap(f *GridField, route []int, cfg gridConfig) {
	if len(f.Bottleneck) == 0 {
		return
	}
	w := f.N / 2
	newRow := -1
	bestD := 1 << 30
	for _, c := range route {
		if f.iOf(c) != w {
			continue
		}
		j := f.jOf(c)
		if j < 1 || j > f.N-3 {
			continue
		}
		if d := gridAbs(j - f.N/2); d < bestD {
			bestD, newRow = d, j
		}
	}
	if newRow < 0 {
		return
	}
	want := map[int]bool{newRow*f.N + w: true, (newRow+1)*f.N + w: true}
	same := len(f.Bottleneck) == 2
	for c := range want {
		if !f.Bottleneck[c] {
			same = false
		}
	}
	if same {
		return
	}
	old := make([]int, 0, len(f.Bottleneck))
	for c := range f.Bottleneck {
		old = append(old, c)
	}
	for _, c := range old {
		delete(f.Bottleneck, c)
		if c != f.Start && c != f.Finish {
			f.Wall[c] = true
			f.Visible[c] = cfg.WallCost
		}
	}
	for c := range want {
		if f.Wall[c] {
			delete(f.Wall, c)
			if f.Lane[c] {
				f.Visible[c] = cfg.LaneCost
			} else {
				f.Visible[c] = 1.0
			}
		}
		f.Bottleneck[c] = true
	}
}

// gridPlaceLanes — дешёвые горизонтальные «русла» на случайных строках.
func gridPlaceLanes(rng *rand.Rand, cfg gridConfig) map[int]bool {
	n := cfg.N
	out := map[int]bool{}
	for t := 0; t < cfg.LaneCount; t++ {
		r := 1 + rng.Intn(max(n-2, 1))
		s := rng.Intn(max(n-cfg.LaneLen, 1))
		for i := s; i < s+cfg.LaneLen && i < n; i++ {
			out[r*n+i] = true
		}
	}
	return out
}

func gridPlaceBeacons(rng *rand.Rand, cfg gridConfig, policy gridBeaconPolicy, k, start, finish int, lanes, wall map[int]bool) []int {
	n := cfg.N
	var raw []int
	switch policy {
	case gridBeaconRing:
		raw = gridRingBeacons(rng, n, k)
	case gridBeaconCross:
		raw = gridCrossBeacons(rng, n, k)
	case gridBeaconCluster:
		raw = gridClusterBeacons(rng, n, k)
	default:
		raw = gridZigzagBeacons(rng, n, k)
	}
	seen := map[int]bool{start: true, finish: true}
	out := make([]int, 0, k)
	for _, c := range raw {
		cell := gridSnapToLane(n, c, lanes)
		for seen[cell] || wall[cell] {
			cell = gridAdvanceCell(n, cell)
		}
		seen[cell] = true
		out = append(out, cell)
	}
	return out
}

func gridSnapToLane(n, c int, lanes map[int]bool) int {
	if len(lanes) == 0 {
		return c
	}
	ci, cj := c%n, c/n
	best, bestD := c, 3
	for rc := range lanes {
		d := gridAbs(rc%n-ci) + gridAbs(rc/n-cj)
		if d < bestD || (d == bestD && rc < best) {
			best, bestD = rc, d
		}
	}
	if bestD <= 2 {
		return best
	}
	return c
}

func gridAdvanceCell(n, c int) int {
	i, j := c%n, c/n
	i++
	if i >= n-1 {
		i = 1
		j++
		if j >= n-1 {
			j = 1
		}
	}
	return gridClampInt(j, 1, n-2)*n + gridClampInt(i, 1, n-2)
}

func gridZigzagBeacons(rng *rand.Rand, n, k int) []int {
	loMin, loMax := 2, n/2-1
	hiMin, hiMax := n/2+1, n-3
	if loMax < loMin {
		loMax = loMin
	}
	if hiMax < hiMin {
		hiMax = hiMin
	}
	out := make([]int, 0, k)
	for t := 0; t < k; t++ {
		ci := gridClampInt(int(math.Round(float64(t+1)*float64(n-1)/float64(k+1))), 1, n-2)
		var cj int
		if t%2 == 0 {
			cj = loMin + rng.Intn(loMax-loMin+1)
		} else {
			cj = hiMin + rng.Intn(hiMax-hiMin+1)
		}
		out = append(out, cj*n+ci)
	}
	return out
}

func gridRingBeacons(rng *rand.Rand, n, k int) []int {
	c := float64(n-1) / 2
	r := float64(n)/2 - 1.5
	base := rng.Float64() * 2 * math.Pi
	out := make([]int, 0, k)
	for t := 0; t < k; t++ {
		ang := base + 2*math.Pi*float64(t)/float64(k)
		i := gridClampInt(int(math.Round(c+r*math.Cos(ang))), 1, n-2)
		j := gridClampInt(int(math.Round(c+r*math.Sin(ang))), 1, n-2)
		out = append(out, j*n+i)
	}
	return out
}

func gridCrossBeacons(rng *rand.Rand, n, k int) []int {
	mid := n / 2
	out := make([]int, 0, k)
	for t := 0; t < k; t++ {
		if t%2 == 0 {
			out = append(out, mid*n+gridClampInt(1+rng.Intn(n-2), 1, n-2))
		} else {
			out = append(out, gridClampInt(1+rng.Intn(n-2), 1, n-2)*n+mid)
		}
	}
	return out
}

func gridClusterBeacons(rng *rand.Rand, n, k int) []int {
	c1i, c1j := 1+rng.Intn(max(n/3, 1)), 1+rng.Intn(max(n/3, 1))
	c2i, c2j := n-2-rng.Intn(max(n/3, 1)), n-2-rng.Intn(max(n/3, 1))
	out := make([]int, 0, k)
	for t := 0; t < k; t++ {
		ci, cj := c1i, c1j
		if t%2 == 1 {
			ci, cj = c2i, c2j
		}
		ci = gridClampInt(ci+rng.Intn(3)-1, 1, n-2)
		cj = gridClampInt(cj+rng.Intn(3)-1, 1, n-2)
		out = append(out, cj*n+ci)
	}
	return out
}

func gridPlaceMud(rng *rand.Rand, cfg gridConfig, bias gridHazardBias, h, unrest int, p3 float64, occupied, lanes, wall map[int]bool, js, jf int) map[int]float64 {
	n := cfg.N
	res := map[int]float64{}
	if h <= 0 {
		return res
	}
	rows := []int{js, jf, n/2 - 1, n / 2, n/2 + 1}
	var seeds []int
	pickCell := func(cluster bool) int {
		var i, j int
		switch {
		case cluster && len(seeds) > 0 && rng.Float64() < 0.7:
			s := seeds[rng.Intn(len(seeds))]
			i = gridClampInt(s%n+rng.Intn(3)-1, 0, n-1)
			j = gridClampInt(s/n+rng.Intn(3)-1, 0, n-1)
		case bias == gridHazardCorridors && rng.Float64() < cfg.CorridorProb:
			j = gridClampInt(rows[rng.Intn(len(rows))], 0, n-1)
			i = rng.Intn(n)
		default:
			i, j = rng.Intn(n), rng.Intn(n)
		}
		return j*n + i
	}
	ok := func(cell int) bool { return !occupied[cell] && !lanes[cell] && !wall[cell] }
	try := func(cluster bool) {
		cell := pickCell(cluster)
		if !ok(cell) {
			return
		}
		if _, dup := res[cell]; dup {
			return
		}
		res[cell] = gridPickK(rng, cfg.KChoices, unrest, p3)
		if bias == gridHazardCluster && len(seeds) < max(h/3, 2) && rng.Float64() < 0.5 {
			seeds = append(seeds, cell)
		}
	}
	for len(res) < h {
		before := len(res)
		for t := 0; t < 300 && len(res) < h; t++ {
			try(bias == gridHazardCluster)
		}
		if len(res) == before {
			for cell := 0; cell < n*n && len(res) < h; cell++ {
				if !ok(cell) {
					continue
				}
				if _, dup := res[cell]; dup {
					continue
				}
				res[cell] = gridPickK(rng, cfg.KChoices, unrest, p3)
			}
		}
	}
	return res
}

func gridPickK(rng *rand.Rand, choices []float64, unrest int, extra float64) float64 {
	if len(choices) == 0 {
		return 2
	}
	if len(choices) == 1 {
		return choices[0]
	}
	p3 := 0.3 + 0.2*float64(gridClampInt(unrest, 0, 7))/7.0 + extra
	if p3 > 0.95 {
		p3 = 0.95
	}
	if rng.Float64() < p3 {
		return choices[1]
	}
	return choices[0]
}

// gridPlaceSpecials размещает на трассе публичного оптимума шлюз, мост и течение.
func gridPlaceSpecials(f *GridField, rng *rand.Rand, cfg gridConfig, route []int) {
	free := func(c int) bool {
		return c != f.Start && c != f.Finish && f.Mud[c] == 0 && !f.Wall[c] && !f.Gate[c] &&
			!f.Bridge[c] && !f.Bottleneck[c] && !isBeaconGrid(f, c) && !f.L0Cells[c]
	}
	pick := func(frac float64) int {
		if len(route) < 3 {
			return -1
		}
		t := gridClampInt(int(frac*float64(len(route)-1)), 1, len(route)-2)
		for step := 0; step < len(route); step++ {
			idx := (t + step) % len(route)
			c := route[idx]
			if free(c) {
				return idx
			}
		}
		return -1
	}
	if idx := pick(1.0 / 3.0); idx >= 0 {
		f.Gate[route[idx]] = true
	}
	if idx := pick(0.5); idx >= 0 {
		c := route[idx]
		f.Bridge[c] = true
	}
	if idx := pick(0.66); idx >= 0 && idx > 0 {
		c := route[idx]
		f.CurrentDir[c] = f.dirOf(route[idx-1], c)
	}
}

// gridPlaceDeadEnd — короткое тупиковое русло, пристроенное к трассе.
func gridPlaceDeadEnd(f *GridField, rng *rand.Rand, cfg gridConfig, route []int) {
	n := f.N
	f.DeadEndAttach, f.DeadEndEntry = -1, -1
	if len(route) < 4 {
		return
	}
	cands := []int{len(route) / 2, len(route) / 3, 2 * len(route) / 3, len(route) / 4, 3 * len(route) / 4}
	for _, ci := range cands {
		if ci < 0 || ci >= len(route) {
			continue
		}
		att := route[ci]
		bi, bj := f.iOf(att), f.jOf(att)
		dirs := [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
		rng.Shuffle(4, func(a, b int) { dirs[a], dirs[b] = dirs[b], dirs[a] })
		var best []int
		for _, d := range dirs {
			var cells []int
			x, y := bi, bj
			for t := 0; t < cfg.DeadEndLen; t++ {
				x += d[0]
				y += d[1]
				if x < 0 || x >= n || y < 0 || y >= n {
					break
				}
				c := f.cell(x, y)
				if f.Wall[c] || f.Mud[c] > 0 || isBeaconGrid(f, c) || c == f.Start || c == f.Finish ||
					f.Gate[c] || f.Bridge[c] || f.Lane[c] || f.DeadEnd[c] {
					break
				}
				cells = append(cells, c)
			}
			if len(cells) > len(best) {
				best = cells
			}
		}
		if len(best) >= 2 {
			for _, c := range best {
				f.DeadEnd[c] = true
				f.Lane[c] = true
				f.Visible[c] = cfg.LaneCost
			}
			f.DeadEndAttach, f.DeadEndEntry = att, best[0]
			return
		}
	}
}

func isBeaconGrid(f *GridField, c int) bool {
	for _, b := range f.Beacons {
		if b == c {
			return true
		}
	}
	return false
}

// gridContentTable — шумная связь σ → content (§14.3) как база, до поправки на
// видимое окружение. Порядок — как GridSectorContent:
// empty, lure, trap, decoy, jackpot, unstable.
var gridContentTable = map[GridSectorSig][6]float64{
	GridSigNoisy:  {0.02, 0.003, 0.26, 0.001, 0.715, 0.001},
	GridSigMedium: {0.45, 0.05, 0.05, 0.32, 0.03, 0.10},
	GridSigQuiet:  {0.60, 0.02, 0.01, 0.30, 0.00, 0.07},
}

// gridSurroundShift — поправка содержимого на видимое окружение (§14.3): шумная
// рядом со шлюзом → скорее jackpot; шумная рядом с тупиком → скорее trap.
// Возвращает множители по индексам GridSectorContent. Единица — без поправки.
func gridSurroundShift(sig GridSectorSig, sur GridSectorSurround) [6]float64 {
	one := [6]float64{1, 1, 1, 1, 1, 1}
	if sig != GridSigNoisy {
		return one
	}
	switch sur {
	case GridSurroundGate:
		return [6]float64{0.5, 1.2, 0.5, 0.6, 2.2, 0.8}
	case GridSurroundDeadEnd:
		return [6]float64{0.6, 0.8, 2.5, 0.8, 0.3, 1.0}
	case GridSurroundMud:
		return [6]float64{0.7, 0.8, 1.3, 1.2, 0.8, 1.0}
	case GridSurroundCurrent:
		return [6]float64{0.6, 1.0, 0.7, 0.7, 1.6, 0.9}
	default:
		return one
	}
}

func gridPickContent(rng *rand.Rand, sig GridSectorSig, sur GridSectorSurround) GridSectorContent {
	table := gridContentTable[sig]
	shift := gridSurroundShift(sig, sur)
	var w [6]float64
	total := 0.0
	for i := range w {
		w[i] = table[i] * shift[i]
		total += w[i]
	}
	if total <= 0 {
		return GridContentEmpty
	}
	r := rng.Float64() * total
	acc := 0.0
	for i, wi := range w {
		acc += wi
		if r < acc {
			return GridSectorContent(i)
		}
	}
	return GridContentEmpty
}

// pickGridSectorContent — детерминированное содержимое сектора из secret + σ +
// окружения. Одна и та же функция используется генератором и RevealGridSector.
func pickGridSectorContent(secretSeed int64, idx int, sig GridSectorSig, sur GridSectorSurround) GridSectorContent {
	rng := rand.New(rand.NewSource(mixGridSeed(secretSeed^gridContentSalt, int64(idx)+1)))
	return gridPickContent(rng, sig, sur)
}

// sectorSurroundGrid — видимое окружение сектора (§14.3).
func (f *GridField) sectorSurroundGrid(cells []int) GridSectorSurround {
	hasGate, hasDead, hasCur, hasMud := false, false, false, false
	for _, c := range cells {
		for _, st := range f.neighbors4(c) {
			n := st.cell
			if f.Gate[n] {
				hasGate = true
			}
			if f.DeadEnd[n] {
				hasDead = true
			}
			if _, ok := f.CurrentDir[n]; ok {
				hasCur = true
			}
			if f.Mud[n] > 0 {
				hasMud = true
			}
		}
	}
	switch {
	case hasGate:
		return GridSurroundGate
	case hasDead:
		return GridSurroundDeadEnd
	case hasCur:
		return GridSurroundCurrent
	case hasMud:
		return GridSurroundMud
	default:
		return GridSurroundPlain
	}
}

// gridPlaceSectors — секторы: публичные клетки, σ и окружение из seed; истинное
// содержимое — из secret. Режим задачи смещает «громкость» (§14.6).
func gridPlaceSectors(f *GridField, seed, secretSeed int64, cfg gridConfig, route []int, mode string) {
	if rp := f.visitAllGrid(f.Visible, true).path; len(rp) > 0 {
		route = rp
	}
	if len(route) == 0 {
		return
	}
	var free []int
	for _, c := range route {
		if c == f.Start || c == f.Finish || isBeaconGrid(f, c) {
			continue
		}
		if f.Gate[c] || f.Bridge[c] || f.Mud[c] > 0 || f.Wall[c] || f.Bottleneck[c] {
			continue
		}
		free = append(free, c)
	}
	if len(free) < cfg.SectorCount {
		return
	}

	srand := rand.New(rand.NewSource(mixGridSeed(seed, gridSigSalt)))
	lrand := rand.New(rand.NewSource(mixGridSeed(seed, gridLayoutSalt)))

	sigs := make([]GridSectorSig, cfg.SectorCount)
	noisyP, mediumP := 0.40, 0.35
	switch mode {
	case "тупики/обманки":
		noisyP, mediumP = 0.60, 0.25
	case "топь":
		noisyP, mediumP = 0.20, 0.30
	}
	noisy := 0
	for i := range sigs {
		r := srand.Float64()
		switch {
		case r < noisyP:
			sigs[i] = GridSigNoisy
			noisy++
		case r < noisyP+mediumP:
			sigs[i] = GridSigMedium
		default:
			sigs[i] = GridSigQuiet
		}
	}
	if noisy == 0 {
		sigs[srand.Intn(len(sigs))] = GridSigNoisy
	}

	used := map[int]bool{}
	takeBlob := func(anchor, size int, pool []int) []int {
		si := 0
		for i, c := range pool {
			if c == anchor {
				si = i
				break
			}
		}
		cells := []int{pool[si]}
		used[pool[si]] = true
		for k := 1; k < size; k++ {
			c := pool[(si+k)%len(pool)]
			if used[c] {
				break
			}
			used[c] = true
			cells = append(cells, c)
		}
		return cells
	}
	cellCount := func() int {
		s := cfg.SectorCellMin
		if cfg.SectorCellMax > cfg.SectorCellMin {
			s += lrand.Intn(cfg.SectorCellMax - cfg.SectorCellMin + 1)
		}
		return s
	}
	anchorFrom := func() int {
		for tries := 0; tries < len(free)*2; tries++ {
			if c := free[lrand.Intn(len(free))]; !used[c] {
				return c
			}
		}
		for _, c := range free {
			if !used[c] {
				return c
			}
		}
		return -1
	}
	for i := 0; i < cfg.SectorCount; i++ {
		anchor := anchorFrom()
		if anchor < 0 {
			break
		}
		cells := takeBlob(anchor, cellCount(), free)
		sur := f.sectorSurroundGrid(cells)
		f.Sectors = append(f.Sectors, GridSector{
			Cells:    cells,
			Sig:      sigs[i],
			Surround: sur,
			content:  pickGridSectorContent(secretSeed, i, sigs[i], sur),
		})
	}
}

// gridBuildMaps — Visible и realized (истина) по секторам.
func gridBuildMaps(f *GridField) {
	f.realized = append([]float64(nil), f.Visible...)
	for _, s := range f.Sectors {
		for _, c := range s.Cells {
			switch s.content {
			case GridContentJackpot:
				f.realized[c] = 0.0
			case GridContentLure:
				f.realized[c] = f.LureCost
			case GridContentTrap:
				f.realized[c] = f.TrapCost
			case GridContentDecoy:
				f.realized[c] = f.DecoyCost
			case GridContentUnstable, GridContentEmpty:
				// слепо — обычная клетка; зонд unstable активирует помеху на
				// стороне вызывающего (вне этой модели).
				f.realized[c] = 1.0
			}
		}
	}
}
