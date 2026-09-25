// internal/routegame/gridv9_place.go
// Генерация поля v9: маяки, стена с проходом, русла, топь, спецобъекты (шлюз,
// мост, течение), тупиковое русло; секторы v9 (публичная σ + шумное содержимое);
// карты Visible/Realized и якоря L_naive/L_safe/L_risk. Только чистые функции.
package routegame

import (
	"math"
	"math/rand"
)

// GenerateFieldV9 строит поле из seed, dist и паспорта. Гейт §14.2:
// L_naive > L_safe (иначе перегенерация).
func GenerateFieldV9(seed int64, dist float64, passport Passport, cfg V9Config) (V9Field, bool) {
	cfg = cfg.withDefaults()
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		f := buildFieldV9(mixSeedV9(seed, int64(attempt)), dist, passport, cfg)
		if f.LNaive-f.LSafe > 1e-9 {
			f.Attempt = attempt
			return f, true
		}
	}
	return V9Field{}, false
}

func buildFieldV9(seed int64, dist float64, passport Passport, cfg V9Config) V9Field {
	rng := rand.New(rand.NewSource(seed))
	n := cfg.N
	f := V9Field{Seed: seed, N: n, TurnCost: cfg.TurnCost, GateToll: cfg.GateToll,
		CurrentAgainst: cfg.CurrentAgainst, BridgeFirst: cfg.BridgeFirst, BridgeSecond: cfg.BridgeSecond,
		LaneCost: cfg.LaneCost, LureCost: cfg.LureCost, TrapCost: cfg.TrapCost, DecoyCost: cfg.DecoyCost,
		M0Frac: cfg.M0Frac, BMin: cfg.BMin, BMax: cfg.BMax,
		Mud: map[int]float64{}, Lane: map[int]bool{}, Wall: map[int]bool{}, Gate: map[int]bool{},
		CurrentDir: map[int]int{}, Bridge: map[int]bool{}, DeadEnd: map[int]bool{},
		Bottleneck: map[int]bool{}}
	span := max(n-4, 1)
	f.Start = f.cell(0, 2+rng.Intn(span))
	f.Finish = f.cell(n-1, 2+rng.Intn(span))

	policy, bias, p3 := V9PassportProfile(passport)
	k := beaconCountV9(dist, cfg)
	unrest := passportUnrest(passport)
	h := hazardCountV9(dist, unrest, cfg)

	lanes := placeLanesV9(rng, cfg)
	wall, gaps := placeWallV9(rng, cfg, f.Start, f.Finish)
	f.Beacons = placeBeaconsV9(rng, cfg, policy, k, f.Start, f.Finish, lanes, wall)
	occ := map[int]bool{f.Start: true, f.Finish: true}
	for _, b := range f.Beacons {
		occ[b] = true
	}
	mud := placeMudV9(rng, cfg, bias, h, unrest, p3, occ, lanes, wall, f.jOf(f.Start), f.jOf(f.Finish))

	f.Visible = make([]float64, n*n)
	for i := range f.Visible {
		f.Visible[i] = 1.0
	}
	for c := range lanes {
		f.Visible[c] = cfg.LaneCost
		f.Lane[c] = true
	}
	for c, kk := range mud {
		f.Visible[c] = kk
		f.Mud[c] = kk
	}
	for c := range wall {
		f.Visible[c] = cfg.WallCost
		f.Wall[c] = true
	}
	for _, g := range gaps {
		f.Bottleneck[g] = true
	}

	// Публичный оптимум — трасса для размещения объектов и секторов.
	route := f.visitAllV9(f.Visible, true)
	routePath := route.path
	if len(routePath) == 0 {
		routePath = f.buildRouteStairV9(naturalOrderV9(f))
	}
	f.VisPath = routePath
	f.L0Cells = map[int]bool{}
	for _, c := range f.buildRouteStairV9(naturalOrderV9(f)) {
		f.L0Cells[c] = true
	}
	adjustWallGapV9(&f, routePath, cfg)

	placeSpecialsV9(&f, rng, cfg, routePath)
	placeDeadEndV9(&f, rng, cfg, routePath)
	f.Mode = pickModeV9(seed, &f, unrest)
	placeSectorsV9(&f, seed, cfg, routePath, f.Mode)
	f.buildMapsV9()
	f.computeAnchorsV9()
	return f
}

// pickModeV9 — режим задачи §14.6 из secret + паспорта: «русла» (Steiner),
// «течения» (односторонние), «шлюзы» (toll), «тупики/обманки», «топь». Влияние
// признаков поля — умеренное (иначе распределение схлопывается на 1–2 режима).
func pickModeV9(seed int64, f *V9Field, unrest int) string {
	rng := rand.New(rand.NewSource(mixSeedV9(seed, V9Modesalt)))
	pick := func(name string) string { return name }
	weights := map[string]float64{
		"русла":          1.0 + 0.05*float64(len(f.Lane)),
		"течения":        1.0 + 0.30*float64(len(f.CurrentDir)),
		"шлюзы":          1.0 + 0.30*float64(len(f.Gate)),
		"тупики/обманки": 1.0 + 0.30*float64(min(len(f.DeadEnd), 4)),
		"топь":           1.0 + 0.10*float64(len(f.Mud)) + 0.05*float64(unrest),
	}
	best, bestW := "", -1.0
	for _, m := range V9Modes {
		w := weights[m] * (0.60 + 0.80*rng.Float64())
		if w > bestW {
			bestW, best = w, m
		}
	}
	return pick(best)
}

// computeAnchorsV9 — якоря единой (реализованной) шкалы §14.2.
func (f *V9Field) computeAnchorsV9() {
	f.LNaive = f.pathCostV9(f.Realized, f.buildRouteStairV9(naturalOrderV9(*f)))
	blocked := f.withBlockedV9(f.sectorCellSetV9(nil))
	if w := blocked.visitAllV9(f.Realized, false); !math.IsInf(w.cost, 1) {
		f.LSafe = w.cost
	} else {
		f.LSafe = f.LNaive
	}
	f.LRisk = f.LSafe
	for j, s := range f.Sectors {
		if s.Content != ContentJackpotV9 && s.Content != ContentLureV9 {
			continue // «срез» даёт только благоприятное содержимое
		}
		if p := f.planSectorV9(j); len(p) >= 2 {
			if c := f.pathCostV9(f.Realized, p); c < f.LRisk {
				f.LRisk = c
			}
		}
	}
	if f.LRisk > f.LSafe {
		f.LRisk = f.LSafe
	}
}

// placeWallV9 — дорогая стена по средней колонке с проходом (bottleneck).
func placeWallV9(rng *rand.Rand, cfg V9Config, start, finish int) (map[int]bool, []int) {
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
	gap = V9ClampInt(gap, 1, n-3)
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

// adjustWallGapV9 — переносит проход (bottleneck) в строку, где трасса публичного
// оптимума пересекает колонку стены.
func adjustWallGapV9(f *V9Field, route []int, cfg V9Config) {
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
		if d := V9Abs(j - f.N/2); d < bestD {
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

// placeLanesV9 — дешёвые горизонтальные «русла» на случайных строках.
func placeLanesV9(rng *rand.Rand, cfg V9Config) map[int]bool {
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

func placeBeaconsV9(rng *rand.Rand, cfg V9Config, policy beaconPolicy, k, start, finish int, lanes, wall map[int]bool) []int {
	n := cfg.N
	var raw []int
	switch policy {
	case beaconRing:
		raw = ringBeaconsV9(rng, n, k)
	case beaconCross:
		raw = crossBeaconsV9(rng, n, k)
	case beaconCluster:
		raw = clusterBeaconsV9(rng, n, k)
	default:
		raw = zigzagBeaconsV9(rng, n, k)
	}
	seen := map[int]bool{start: true, finish: true}
	out := make([]int, 0, k)
	for _, c := range raw {
		cell := V9SnapToLane(n, c, lanes)
		for seen[cell] || wall[cell] {
			cell = V9AdvanceCell(n, cell)
		}
		seen[cell] = true
		out = append(out, cell)
	}
	return out
}

func V9SnapToLane(n, c int, lanes map[int]bool) int {
	if len(lanes) == 0 {
		return c
	}
	ci, cj := c%n, c/n
	best, bestD := c, 3
	for rc := range lanes {
		d := V9Abs(rc%n-ci) + V9Abs(rc/n-cj)
		if d < bestD || (d == bestD && rc < best) {
			best, bestD = rc, d
		}
	}
	if bestD <= 2 {
		return best
	}
	return c
}

func V9AdvanceCell(n, c int) int {
	i, j := c%n, c/n
	i++
	if i >= n-1 {
		i = 1
		j++
		if j >= n-1 {
			j = 1
		}
	}
	return V9ClampInt(j, 1, n-2)*n + V9ClampInt(i, 1, n-2)
}

func zigzagBeaconsV9(rng *rand.Rand, n, k int) []int {
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
		ci := V9ClampInt(int(math.Round(float64(t+1)*float64(n-1)/float64(k+1))), 1, n-2)
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

func ringBeaconsV9(rng *rand.Rand, n, k int) []int {
	c := float64(n-1) / 2
	r := float64(n)/2 - 1.5
	base := rng.Float64() * 2 * math.Pi
	out := make([]int, 0, k)
	for t := 0; t < k; t++ {
		ang := base + 2*math.Pi*float64(t)/float64(k)
		i := V9ClampInt(int(math.Round(c+r*math.Cos(ang))), 1, n-2)
		j := V9ClampInt(int(math.Round(c+r*math.Sin(ang))), 1, n-2)
		out = append(out, j*n+i)
	}
	return out
}

func crossBeaconsV9(rng *rand.Rand, n, k int) []int {
	mid := n / 2
	out := make([]int, 0, k)
	for t := 0; t < k; t++ {
		if t%2 == 0 {
			out = append(out, mid*n+V9ClampInt(1+rng.Intn(n-2), 1, n-2))
		} else {
			out = append(out, V9ClampInt(1+rng.Intn(n-2), 1, n-2)*n+mid)
		}
	}
	return out
}

func clusterBeaconsV9(rng *rand.Rand, n, k int) []int {
	c1i, c1j := 1+rng.Intn(max(n/3, 1)), 1+rng.Intn(max(n/3, 1))
	c2i, c2j := n-2-rng.Intn(max(n/3, 1)), n-2-rng.Intn(max(n/3, 1))
	out := make([]int, 0, k)
	for t := 0; t < k; t++ {
		ci, cj := c1i, c1j
		if t%2 == 1 {
			ci, cj = c2i, c2j
		}
		ci = V9ClampInt(ci+rng.Intn(3)-1, 1, n-2)
		cj = V9ClampInt(cj+rng.Intn(3)-1, 1, n-2)
		out = append(out, cj*n+ci)
	}
	return out
}

func placeMudV9(rng *rand.Rand, cfg V9Config, bias hazardBias, h, unrest int, p3 float64, occupied, lanes, wall map[int]bool, js, jf int) map[int]float64 {
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
			i = V9ClampInt(s%n+rng.Intn(3)-1, 0, n-1)
			j = V9ClampInt(s/n+rng.Intn(3)-1, 0, n-1)
		case bias == hazardCorridors && rng.Float64() < cfg.CorridorProb:
			j = V9ClampInt(rows[rng.Intn(len(rows))], 0, n-1)
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
		res[cell] = pickKV9(rng, cfg.KChoices, unrest, p3)
		if bias == hazardCluster && len(seeds) < max(h/3, 2) && rng.Float64() < 0.5 {
			seeds = append(seeds, cell)
		}
	}
	for len(res) < h {
		before := len(res)
		for t := 0; t < 300 && len(res) < h; t++ {
			try(bias == hazardCluster)
		}
		if len(res) == before {
			for cell := 0; cell < n*n && len(res) < h; cell++ {
				if !ok(cell) {
					continue
				}
				if _, dup := res[cell]; dup {
					continue
				}
				res[cell] = pickKV9(rng, cfg.KChoices, unrest, p3)
			}
		}
	}
	return res
}

func pickKV9(rng *rand.Rand, choices []float64, unrest int, extra float64) float64 {
	if len(choices) == 0 {
		return 2
	}
	if len(choices) == 1 {
		return choices[0]
	}
	p3 := 0.3 + 0.2*float64(V9ClampInt(unrest, 0, 7))/7.0 + extra
	if p3 > 0.95 {
		p3 = 0.95
	}
	if rng.Float64() < p3 {
		return choices[1]
	}
	return choices[0]
}

// placeSpecialsV9 размещает на трассе публичного оптимума шлюз, мост и течение.
func placeSpecialsV9(f *V9Field, rng *rand.Rand, cfg V9Config, route []int) {
	free := func(c int) bool {
		return c != f.Start && c != f.Finish && f.Mud[c] == 0 && !f.Wall[c] && !f.Gate[c] &&
			!f.Bridge[c] && !f.Bottleneck[c] && !isBeaconV9(f, c) && !f.L0Cells[c]
	}
	pick := func(frac float64) int {
		if len(route) < 3 {
			return -1
		}
		t := V9ClampInt(int(frac*float64(len(route)-1)), 1, len(route)-2)
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

// placeDeadEndV9 — короткое тупиковое русло, пристроенное к трассе.
func placeDeadEndV9(f *V9Field, rng *rand.Rand, cfg V9Config, route []int) {
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
				if f.Wall[c] || f.Mud[c] > 0 || isBeaconV9(f, c) || c == f.Start || c == f.Finish ||
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

func isBeaconV9(f *V9Field, c int) bool {
	for _, b := range f.Beacons {
		if b == c {
			return true
		}
	}
	return false
}

// V9ContentTable — шумная связь σ → content (§14.3) как БАЗА, до поправки на
// видимое окружение. Порядок — как SectorContentV9:
// empty, lure, trap, decoy, jackpot, unstable.
// Шумная база близка к премии за риск §14.2: jackpot ≈ 0.5, empty ≈ 0.2,
// trap ≈ 0.3 (остаток — lure/decoy/unstable).
var V9ContentTable = map[SectorSigV9][6]float64{
	SigNoisyV9:  {0.02, 0.003, 0.26, 0.001, 0.715, 0.001},
	SigMediumV9: {0.45, 0.05, 0.05, 0.32, 0.03, 0.10},
	SigQuietV9:  {0.60, 0.02, 0.01, 0.30, 0.00, 0.07},
}

// V9SurroundShift — поправка содержимого на видимое окружение (§14.3): шумная
// рядом со шлюзом → скорее jackpot; шумная рядом с тупиком → скорее trap.
// Возвращает множители по индексам SectorContentV9 (empty, lure, trap, decoy,
// jackpot, unstable). Единица — без поправки.
func V9SurroundShift(sig SectorSigV9, sur SectorSurroundV9) [6]float64 {
	one := [6]float64{1, 1, 1, 1, 1, 1}
	if sig != SigNoisyV9 {
		return one
	}
	switch sur {
	case SurroundGateV9:
		// громкая + шлюз → скорее jackpot (срез через шлюз окупается).
		return [6]float64{0.5, 1.2, 0.5, 0.6, 2.2, 0.8}
	case SurroundDeadEndV9:
		// громкая + тупик → скорее trap (громкое рядом с тупиком — приманка).
		return [6]float64{0.6, 0.8, 2.5, 0.8, 0.3, 1.0}
	case SurroundMudV9:
		// громкая + топь → скорее trap/decoy.
		return [6]float64{0.7, 0.8, 1.3, 1.2, 0.8, 1.0}
	case SurroundCurrentV9:
		// громкая + течение → скорее jackpot (срез по течению).
		return [6]float64{0.6, 1.0, 0.7, 0.7, 1.6, 0.9}
	default:
		return one
	}
}

func pickContentV9(rng *rand.Rand, sig SectorSigV9, sur SectorSurroundV9) SectorContentV9 {
	table := V9ContentTable[sig]
	shift := V9SurroundShift(sig, sur)
	var w [6]float64
	total := 0.0
	for i := range w {
		w[i] = table[i] * shift[i]
		total += w[i]
	}
	if total <= 0 {
		return ContentEmptyV9
	}
	r := rng.Float64() * total
	acc := 0.0
	for i, wi := range w {
		acc += wi
		if r < acc {
			return SectorContentV9(i)
		}
	}
	return ContentEmptyV9
}

// V9SurroundPriority — приоритет зондирования по видимому окружению (§14.3/§14.4):
// шлюз/течение — «читать» (скорее jackpot), тупик/топь — «не зондировать»
// (скорее trap). Больше — раньше.
func V9SurroundPriority(sur SectorSurroundV9) int {
	switch sur {
	case SurroundGateV9:
		return 3
	case SurroundCurrentV9:
		return 2
	case SurroundPlainV9:
		return 1
	case SurroundMudV9:
		return 0
	case SurroundDeadEndV9:
		return -1
	default:
		return 1
	}
}

// sectorSurroundV9 — видимое окружение сектора (§14.3): что из публичных
// объектов поля лежит рядом с его клетками. Приоритет: шлюз > тупик > течение >
// топь > пусто (шлюз и тупик — ключевые признаки противохода боту).
func (f *V9Field) sectorSurroundV9(cells []int) SectorSurroundV9 {
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
		return SurroundGateV9
	case hasDead:
		return SurroundDeadEndV9
	case hasCur:
		return SurroundCurrentV9
	case hasMud:
		return SurroundMudV9
	default:
		return SurroundPlainV9
	}
}

// placeSectorsV9 — секторы V9: клетки на трассе публичного оптимума; публичная
// подпись σ из публичного RNG (видна игроку), истинный content — из secret RNG,
// шумная функция от σ. Режим задачи смещает «громкость» (приоритет линии §14.6).
func placeSectorsV9(f *V9Field, seed int64, cfg V9Config, route []int, mode string) {
	// Секторы садятся на актуальную трассу публичного оптимума (после спецобъектов).
	if rp := f.visitAllV9(f.Visible, true).path; len(rp) > 0 {
		route = rp
	}
	if len(route) == 0 {
		return
	}
	var free []int
	for _, c := range route {
		if c == f.Start || c == f.Finish || isBeaconV9(f, c) {
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

	srand := rand.New(rand.NewSource(mixSeedV9(seed, V9SigSalt)))
	crand := rand.New(rand.NewSource(mixSeedV9(seed, V9SecretSalt)))

	sigs := make([]SectorSigV9, cfg.SectorCount)
	noisyP, mediumP := 0.40, 0.35
	switch mode {
	case "тупики/обманки":
		noisyP, mediumP = 0.60, 0.25 // громче: чаще есть что читать/зондировать
	case "топь":
		noisyP, mediumP = 0.20, 0.30 // тише: подписи мало что дают
	}
	noisy := 0
	for i := range sigs {
		r := srand.Float64()
		switch {
		case r < noisyP:
			sigs[i] = SigNoisyV9
			noisy++
		case r < noisyP+mediumP:
			sigs[i] = SigMediumV9
		default:
			sigs[i] = SigQuietV9
		}
	}
	if noisy == 0 {
		sigs[srand.Intn(len(sigs))] = SigNoisyV9
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
			s += crand.Intn(cfg.SectorCellMax - cfg.SectorCellMin + 1)
		}
		return s
	}
	anchorFrom := func() int {
		for tries := 0; tries < len(free)*2; tries++ {
			if c := free[crand.Intn(len(free))]; !used[c] {
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
		sur := f.sectorSurroundV9(cells)
		f.Sectors = append(f.Sectors, SectorV9{
			Cells:    cells,
			Sig:      sigs[i],
			Surround: sur,
			Content:  pickContentV9(crand, sigs[i], sur),
		})
	}
}

// buildMapsV9 — Visible и Realized (истина) по секторам.
func (f *V9Field) buildMapsV9() {
	f.Realized = append([]float64(nil), f.Visible...)
	for _, s := range f.Sectors {
		for _, c := range s.Cells {
			switch s.Content {
			case ContentJackpotV9:
				f.Realized[c] = 0.0
			case ContentLureV9:
				f.Realized[c] = f.LureCost
			case ContentTrapV9:
				f.Realized[c] = f.TrapCost
			case ContentDecoyV9:
				f.Realized[c] = f.DecoyCost
			case ContentUnstableV9:
				// слепо unstable — обычная клетка; зонд активирует помеху —
				// см. destabilizedV9.
				f.Realized[c] = 1.0
			case ContentEmptyV9:
				// пусто — обычная клетка (ни среза, ни помехи).
				f.Realized[c] = 1.0
			}
		}
	}
}

// withBlockedV9 — копия поля с непроходимыми клетками (карта планирования).
func (f V9Field) withBlockedV9(cells map[int]bool) V9Field {
	g := f
	g.Blocked = cells
	return g
}

// sectorCellSetV9 — множество клеток секторов, для которых pred даёт true
// (pred == nil — все секторы).
func (f V9Field) sectorCellSetV9(pred func(SectorContentV9) bool) map[int]bool {
	out := map[int]bool{}
	for _, s := range f.Sectors {
		if pred != nil && !pred(s.Content) {
			continue
		}
		for _, c := range s.Cells {
			out[c] = true
		}
	}
	return out
}
