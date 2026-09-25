// internal/routegame/gridv9_strategy.go
// Кривая bonus(C) v9 (§14.2) и стратегии честного замера без оракула (§14.9):
// L0/C/S/Safe/X_σ/I_σ/B/F плюс справка I_oracle. Планы, доступные игроку,
// строятся ТОЛЬКО по публичным признакам (σ и геометрия); истинное содержимое
// используется лишь для оценки реализованной цены. Только чистые функции.
package routegame

import (
	"math"
	"math/rand"
)

// bonusCostV9 — кусочно-линейная кривая §14.2 на ЕДИНОЙ (реализованной) шкале:
//
//	C >= L_naive          → 0.10 − 0.30·min(1, (C−L_naive)/m0), m0 = 0.10·L_naive
//	L_safe <= C < L_naive → 0.10 + 0.20·u, u = (L_naive−C)/(L_naive−L_safe)
//	C < L_safe            → 0.30 + 0.20·w, w = (L_safe−C)/(L_safe−L_risk)
func (f V9Field) bonusCostV9(C float64) float64 {
	ln, ls, lr := f.LNaive, f.LSafe, f.LRisk
	if C >= ln {
		m0 := f.M0Frac * ln
		if m0 <= 0 {
			return 0.10
		}
		d := (C - ln) / m0
		if d > 1 {
			d = 1
		}
		return 0.10 - 0.30*d
	}
	if C >= ls {
		denom := ln - ls
		if denom <= 1e-12 {
			return 0.10
		}
		u := (ln - C) / denom
		u = math.Max(0, math.Min(1, u))
		return 0.10 + 0.20*u
	}
	denom := ls - lr
	if denom <= 1e-12 {
		return 0.30
	}
	w := (ls - C) / denom
	w = math.Max(0, math.Min(1, w))
	return 0.30 + 0.20*w
}

// bonusPathV9 — бонус пути, посчитанного на реализованном поле (§14.2).
func (f V9Field) bonusPathV9(path []int) float64 {
	if len(path) < 2 {
		return 0.10
	}
	return f.bonusCostV9(f.pathCostV9(f.Realized, path))
}

// planSectorV9 — план «ставка на сектор j»: сектор j считается дешёвым срезом,
// прочие секторы обходятся (тёмное вне ставки). Планирование — по публичной карте.
func (f V9Field) planSectorV9(j int) []int {
	if j < 0 || j >= len(f.Sectors) {
		return nil
	}
	cm := append([]float64(nil), f.Visible...)
	blocked := map[int]bool{}
	for i, s := range f.Sectors {
		for _, c := range s.Cells {
			if i == j {
				if f.LureCost < cm[c] {
					cm[c] = f.LureCost
				}
			} else {
				blocked[c] = true
			}
		}
	}
	g := f.withBlockedV9(blocked)
	if w := g.visitAllV9(cm, true); len(w.path) >= 2 {
		return w.path
	}
	return nil
}

// safePathV9 — обход всех тёмных секторов (линия Safe).
func (f V9Field) safePathV9() []int {
	if w := f.withBlockedV9(f.sectorCellSetV9(nil)).visitAllV9(f.Visible, true); len(w.path) >= 2 {
		return w.path
	}
	return nil
}

// planCostVisibleV9 — цена плана ставки на сектор j по публичной карте.
func (f V9Field) planCostVisibleV9(j int) float64 {
	p := f.planSectorV9(j)
	if len(p) < 2 {
		return math.Inf(1)
	}
	return f.pathCostV9(f.Visible, p)
}

// pickRiskSectorV9 — рисковая ставка X_σ: среди секторов с самой громкой σ
// выбирается лучший по геометрии (публичная цена плана).
func (f V9Field) pickRiskSectorV9() int {
	if len(f.Sectors) == 0 {
		return -1
	}
	top := f.Sectors[0].Sig
	for _, s := range f.Sectors {
		if s.Sig > top {
			top = s.Sig
		}
	}
	bestJ, bestC := -1, math.Inf(1)
	for i, s := range f.Sectors {
		if s.Sig != top {
			continue
		}
		if c := f.planCostVisibleV9(i); c < bestC-1e-12 {
			bestC, bestJ = c, i
		}
	}
	if bestJ < 0 {
		for i := range f.Sectors {
			return i
		}
	}
	return bestJ
}

// informedPathV9 — линия I_σ (§14.4): зонды выбираются по σ и геометрии; вскрытые
// капкан/обманка/неустойчивость обходятся (блокируются), вскрытый благоприятный
// срез используется как дешёвый. Невскрытые секторы остаются обычными (риск).
// Возвращает путь, индексы вскрытых unstable (активированная помеха) и вскрытия.
func (f V9Field) informedPathV9(pings int) ([]int, []int, map[int]SectorContentV9) {
	order := make([]int, len(f.Sectors))
	for i := range order {
		order[i] = i
	}
	// Ранг: σ по убыванию, затем видимое окружение (шлюз/течение — «читать»,
	// тупик/топь — «не зондировать»), затем публичная цена плана по возрастанию.
	for i := 1; i < len(order); i++ {
		for j := i; j > 0; j-- {
			a, b := order[j], order[j-1]
			sa, sb := f.Sectors[a].Sig, f.Sectors[b].Sig
			swap := false
			if sa != sb {
				swap = sa > sb
			} else {
				pa, pb := V9SurroundPriority(f.Sectors[a].Surround), V9SurroundPriority(f.Sectors[b].Surround)
				if pa != pb {
					swap = pa > pb
				} else {
					ca, cb := f.planCostVisibleV9(a), f.planCostVisibleV9(b)
					swap = ca < cb-1e-12
				}
			}
			if !swap {
				break
			}
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	n := min(pings, len(order))
	revealed := map[int]SectorContentV9{}
	var destab []int
	cm := append([]float64(nil), f.Visible...)
	// По умолчанию все тёмные секторы обходятся; вскрытый благоприятный
	// открывается как дешёвый срез, вскрытые капкан/обманка/неустойчивость
	// остаются обойдёнными.
	blocked := f.sectorCellSetV9(nil)
	probed := 0
	for k := 0; k < len(order) && probed < n; k++ {
		j := order[k]
		// Условная политика: не тратим зонд на громкое рядом с тупиком/топью
		// (скорее trap) — это и отличает I_σ от слепого правила Rule.
		if V9SurroundPriority(f.Sectors[j].Surround) < 1 {
			continue
		}
		probed++
		content := f.Sectors[j].Content
		revealed[j] = content
		switch content {
		case ContentUnstableV9:
			destab = append(destab, j) // зонд активировал помеху — обходим
		case ContentJackpotV9, ContentLureV9:
			for _, c := range f.Sectors[j].Cells {
				delete(blocked, c)
				if f.LureCost < cm[c] {
					cm[c] = f.LureCost
				}
			}
		}
	}
	g := f.withBlockedV9(blocked)
	if w := g.visitAllV9(cm, true); len(w.path) >= 2 {
		return w.path, destab, revealed
	}
	return f.safePathV9(), destab, revealed
}

// rulePathV9 — линия Rule (§14.9): слепое одно-признаковое правило «Safe +
// всегда зондировать громкое». Зонд тратится на самый громкий сектор независимо
// от геометрии; решение по вскрытию — как в informedPathV9. Возвращает путь,
// индексы вскрытых unstable и вскрытия.
func (f V9Field) rulePathV9(pings int) ([]int, []int, map[int]SectorContentV9) {
	if len(f.Sectors) == 0 {
		return f.safePathV9(), nil, nil
	}
	// Порядок: только σ по убыванию (без геометрии) — «громкое → зондируй».
	order := make([]int, len(f.Sectors))
	for i := range order {
		order[i] = i
	}
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && f.Sectors[order[j]].Sig > f.Sectors[order[j-1]].Sig; j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	n := min(pings, len(order))
	var destab []int
	revealed := map[int]SectorContentV9{}
	cm := append([]float64(nil), f.Visible...)
	blocked := f.sectorCellSetV9(nil)
	for k := 0; k < n; k++ {
		j := order[k]
		revealed[j] = f.Sectors[j].Content
		switch f.Sectors[j].Content {
		case ContentUnstableV9:
			destab = append(destab, j)
		case ContentJackpotV9, ContentLureV9:
			for _, c := range f.Sectors[j].Cells {
				delete(blocked, c)
				if f.LureCost < cm[c] {
					cm[c] = f.LureCost
				}
			}
		}
	}
	g := f.withBlockedV9(blocked)
	if w := g.visitAllV9(cm, true); len(w.path) >= 2 {
		return w.path, destab, revealed
	}
	return f.safePathV9(), destab, revealed
}

// oraclePathV9 — справка I_oracle: лучший срез по ИСТИННОМУ содержимому (в
// критерии §14.10 не входит).
func (f V9Field) oraclePathV9() []int {
	bestJ, bestC := -1, f.LSafe
	for i, s := range f.Sectors {
		if s.Content != ContentJackpotV9 && s.Content != ContentLureV9 {
			continue
		}
		p := f.planSectorV9(i)
		if len(p) < 2 {
			continue
		}
		if c := f.pathCostV9(f.Realized, p); c < bestC-1e-9 {
			bestC, bestJ = c, i
		}
	}
	if bestJ >= 0 {
		return f.planSectorV9(bestJ)
	}
	return f.safePathV9()
}

// blindPathV9 — линия B: слепая случайная ставка на сектор (секретный RNG).
func (f V9Field) blindPathV9() []int {
	if len(f.Sectors) == 0 {
		return f.safePathV9()
	}
	j := f.blindSectorV9()
	if p := f.planSectorV9(j); len(p) >= 2 {
		return p
	}
	return f.safePathV9()
}

// blindSectorV9 — индекс сектора, на который ставит слепая линия B.
func (f V9Field) blindSectorV9() int {
	if len(f.Sectors) == 0 {
		return -1
	}
	rng := rand.New(rand.NewSource(mixSeedV9(f.Seed, V9BlindSalt)))
	return rng.Intn(len(f.Sectors))
}

// destabilizedV9 — копия поля, где зонд активировал помеху в секторах с
// неустойчивостью (R2): их клетки и подход (соседние клетки) становятся дорогими.
func (f V9Field) destabilizedV9(indices []int) V9Field {
	if len(indices) == 0 {
		return f
	}
	g := f
	r := append([]float64(nil), f.Realized...)
	for _, idx := range indices {
		if idx < 0 || idx >= len(f.Sectors) {
			continue
		}
		for _, c := range f.Sectors[idx].Cells {
			r[c] = f.TrapCost
			// помеха на подходе: соседние клетки тоже дорогие.
			for _, st := range f.neighbors4(c) {
				n := st.cell
				if n == f.Start || n == f.Finish || isBeaconV9(&f, n) || f.Gate[n] || f.Bridge[n] {
					continue
				}
				if r[n] < f.TrapCost {
					r[n] = f.TrapCost
				}
			}
		}
	}
	g.Realized = r
	return g
}

// riskOutcomeV9 — исход рисковой ставки на сектор j (§14.2): премия за риск
// задаётся СОДЕРЖИМОМУ сектора, а не только цене трассы. jackpot/lure → верх
// (+0.50), empty/unstable → около Safe (+0.30), trap → ниже пола (−0.05…−0.15),
// decoy → штраф. Значение детерминировано полем (seed + индекс).
func (f V9Field) riskOutcomeV9(j int) float64 {
	if j < 0 || j >= len(f.Sectors) {
		return f.bonusCostV9(f.LSafe)
	}
	rng := rand.New(rand.NewSource(mixSeedV9(f.Seed, int64(0x7150+j))))
	switch f.Sectors[j].Content {
	case ContentJackpotV9, ContentLureV9:
		if rng.Float64() < 0.5 {
			return 0.50
		}
		return 0.45 + 0.05*rng.Float64()
	case ContentTrapV9:
		return -0.05
	case ContentDecoyV9:
		return 0.10 - 0.20*rng.Float64()
	default: // empty, unstable
		return 0.30
	}
}

// V9Metrics — стоимости и бонусы стратегий по одному полю (единая шкала §14.2).
type V9Metrics struct {
	LNaive, LSafe, LRisk                                                                   float64
	CostL0, CostC, CostS, CostSafe, CostX, CostI, CostB, CostF, CostRule, CostOracle       float64
	BonusL0, BonusC, BonusS, BonusSafe, BonusX, BonusI, BonusB, BonusF, BonusRule, BonusOracle float64
	K, H, Sectors, Pings                                                                   int
	XSig                                                                                   SectorSigV9
	XContent                                                                               SectorContentV9
	XPlanCells                                                                             int
}

// AnalyzeV9 — стратегии и бонусы по одному полю. ВСЕ стратегии — на реализованной
// шкале (§14.2); решение — по публичным признакам (σ и геометрия).
func AnalyzeV9(f V9Field, cfg V9Config) V9Metrics {
	m := V9Metrics{LNaive: f.LNaive, LSafe: f.LSafe, LRisk: f.LRisk,
		K: len(f.Beacons), Sectors: len(f.Sectors), Pings: cfg.Pings}
	for _, c := range f.Visible {
		if c >= 2 {
			m.H++
		}
	}

	// L0 — ленивый план (естественный порядок, без обхода): ровно +0.10.
	m.CostL0 = f.LNaive
	m.BonusL0 = f.bonusCostV9(m.CostL0)

	// C — небрежный: естественный порядок + обход только явной топи.
	cpath := f.buildRouteStairAvoidMudV9(naturalOrderV9(f))
	m.CostC = f.pathCostV9(f.Realized, cpath)
	m.BonusC = f.bonusPathV9(cpath)

	// S — сильный видимый: точный оптимум на видимом поле, тёмное избегает
	// (без зондов, §14.9). Тёмные секторы блокируются, как в линии Safe.
	spath := f.withBlockedV9(f.sectorCellSetV9(nil)).visitAllV9(f.Visible, true).path
	if len(spath) < 2 {
		spath = f.buildRouteStairV9(naturalOrderV9(f))
	}
	m.CostS = f.pathCostV9(f.Realized, spath)
	m.BonusS = f.bonusPathV9(spath)

	// Safe — обход тёмного без импульсов (§14.2): +0.30 на L_safe.
	safepath := f.safePathV9()
	if len(safepath) < 2 {
		safepath = spath
	}
	m.CostSafe = f.pathCostV9(f.Realized, safepath)
	m.BonusSafe = f.bonusPathV9(safepath)

	// X_σ — рисковая линия по подписям без импульсов.
	xj := f.pickRiskSectorV9()
	xpath := safepath
	if xj >= 0 {
		if p := f.planSectorV9(xj); len(p) >= 2 {
			xpath = p
			m.XSig = f.Sectors[xj].Sig
			m.XContent = f.Sectors[xj].Content
			onPath := map[int]bool{}
			for _, c := range p {
				onPath[c] = true
			}
			for _, c := range f.Sectors[xj].Cells {
				if onPath[c] {
					m.XPlanCells++
				}
			}
		}
	}
	m.CostX = f.pathCostV9(f.Realized, xpath)
	// Исход рисковой ставки — по содержимому выбранного сектора (§14.2):
	// премия за риск (jackpot +0.45…+0.50, trap −0.05…−0.15, empty ≈ Safe).
	if xj >= 0 {
		m.BonusX = f.riskOutcomeV9(xj)
	} else {
		m.BonusX = f.bonusPathV9(xpath)
	}

	// I_σ — осведомлённый: зонды по σ+геометрии, решение по вскрытию.
	ipath, destab, revealed := f.informedPathV9(cfg.Pings)
	evaluated := f.destabilizedV9(destab)
	if len(ipath) < 2 {
		ipath = safepath
	}
	m.CostI = evaluated.pathCostV9(evaluated.Realized, ipath)
	// Исход I_σ — по вскрытому содержимому: использованный jackpot/lure даёт
	// верх, иначе — безопасная линия (вскрытые trap/decoy обойдены).
	m.BonusI = f.bonusPathV9(ipath)
	for j, ct := range revealed {
		if ct == ContentJackpotV9 || ct == ContentLureV9 {
			if b := f.riskOutcomeV9(j); b > m.BonusI {
				m.BonusI = b
			}
		}
	}

	// B — слепой: случайная ставка (исход — по содержимому случайного сектора).
	bpath := f.blindPathV9()
	if len(bpath) < 2 {
		bpath = spath
	}
	m.CostB = f.pathCostV9(f.Realized, bpath)
	if bj := f.blindSectorV9(); bj >= 0 {
		m.BonusB = f.riskOutcomeV9(bj)
	} else {
		m.BonusB = f.bonusPathV9(bpath)
	}

	// F — плохой: неверный порядок + жадный срез топью + тупик.
	forder := append([]int(nil), naturalOrderV9(f)...)
	if len(forder) >= 2 {
		forder[0], forder[len(forder)-1] = forder[len(forder)-1], forder[0]
	}
	fpath := f.buildRouteStairV9(forder)
	fpath = f.spliceMudDouble(fpath)
	fpath = f.errDeadEnd(fpath)
	m.CostF = f.pathCostV9(f.Realized, fpath)
	m.BonusF = f.bonusCostV9(m.CostF)

	// I_oracle — справка (по истинному содержимому), в критерий не входит.
	opath := f.oraclePathV9()
	if len(opath) < 2 {
		opath = safepath
	}
	m.CostOracle = f.pathCostV9(f.Realized, opath)
	m.BonusOracle = f.bonusPathV9(opath)

	// Rule — слепое одно-признаковое правило «Safe + всегда зондировать громкое».
	rpath, rdestab, rrevealed := f.rulePathV9(cfg.Pings)
	reval := f.destabilizedV9(rdestab)
	if len(rpath) < 2 {
		rpath = safepath
	}
	m.CostRule = reval.pathCostV9(reval.Realized, rpath)
	m.BonusRule = reval.bonusPathV9(rpath)
	for j, ct := range rrevealed {
		if ct == ContentJackpotV9 || ct == ContentLureV9 {
			if b := f.riskOutcomeV9(j); b > m.BonusRule {
				m.BonusRule = b
			}
		}
	}

	return m
}

// ──────────────────────────── помощники плохого пути F ────────────────────────────

// V9Manhattan — манхэттенское расстояние между клетками.
func (f V9Field) V9Manhattan(a, b int) int {
	return V9Abs(f.iOf(a)-f.iOf(b)) + V9Abs(f.jOf(a)-f.jOf(b))
}

// findAdjV9 — индекс t и сосед cell клетки path[t-1], удовлетворяющий pred.
func (f V9Field) findAdjV9(path []int, pred func(int) bool) (int, int, bool) {
	for t := 1; t < len(path); t++ {
		for _, st := range f.neighbors4(path[t-1]) {
			if pred(st.cell) {
				return t, st.cell, true
			}
		}
	}
	return 0, 0, false
}

// insertAfter — вставить блок ins после индекса t.
func insertAfter(path []int, t int, ins ...int) []int {
	if t < 0 || t >= len(path) {
		return path
	}
	out := make([]int, 0, len(path)+len(ins))
	out = append(out, path[:t+1]...)
	out = append(out, ins...)
	out = append(out, path[t+1:]...)
	return out
}

// spliceMudDouble — заход в топь и возврат дважды (ошибка «сквозь топь»).
func (f V9Field) spliceMudDouble(path []int) []int {
	t, m, ok := f.findAdjV9(path, func(c int) bool { return f.Mud[c] > 0 })
	if !ok {
		return path
	}
	a := path[t-1]
	return insertAfter(path, t-1, m, a, m, a)
}

// deadEndChain — цепочка клеток тупикового русла от entry до конца.
func (f V9Field) deadEndChain() []int {
	if f.DeadEndEntry < 0 {
		return nil
	}
	run := []int{f.DeadEndEntry}
	cur := f.DeadEndEntry
	for len(run) < f.N*f.N {
		nxt := -1
		for _, st := range f.neighbors4(cur) {
			if f.DeadEnd[st.cell] && (len(run) < 2 || st.cell != run[len(run)-2]) {
				nxt = st.cell
				break
			}
		}
		if nxt < 0 {
			break
		}
		run = append(run, nxt)
		cur = nxt
	}
	return run
}

// errDeadEnd — заход в тупиковое русло и возврат по всей длине.
func (f V9Field) errDeadEnd(path []int) []int {
	if f.DeadEndEntry < 0 || f.DeadEndAttach < 0 || len(path) < 2 {
		return path
	}
	run := f.deadEndChain()
	if len(run) == 0 {
		return path
	}
	bestT, bestD := 1, 1<<30
	for t := 1; t < len(path); t++ {
		if d := f.V9Manhattan(f.DeadEndAttach, path[t]); d < bestD {
			bestD, bestT = d, t
		}
	}
	seg := f.staircaseV9(path[bestT], f.DeadEndAttach)
	out := append([]int(nil), path[:bestT+1]...)
	out = append(out, seg[1:]...)
	out = append(out, run...)
	for i := len(run) - 2; i >= 0; i-- {
		out = append(out, run[i])
	}
	for i := len(seg) - 1; i >= 0; i-- {
		out = append(out, seg[i])
	}
	out = append(out, path[bestT+1:]...)
	return out
}
