// internal/routegame/gridv9_errors_test.go
// Офлайн-замер ЭТАПА 2 модели v9 (§14 спеки): таблица 16 ошибок §14.7, цена
// разведки R1–R5 (§14.4), распределение режимов и развилка решений (§14.6),
// наказание линии «игнорировать тёмное» (§14.2). Тяжёлый прогон — вне -short.
package routegame

import (
	"math"
	"sort"
	"testing"
)

// ───────────────────── стоимость с учётом повторного прохода ─────────────────────

// V9ReuseCost — как pathCostV9, но мост на ВТОРОМ входе стоит BridgeSecond
// (§14.7 п.6 «мост дважды»); шлюз, как и в pathCostV9, платит toll на каждый вход.
func (f V9Field) V9ReuseCost(cm []float64, path []int) float64 {
	if len(path) < 2 {
		return 0
	}
	total := 0.0
	usedBridge := map[int]bool{}
	for t := 1; t < len(path); t++ {
		prev, cur := path[t-1], path[t]
		c := cm[cur]
		if d, ok := f.CurrentDir[cur]; ok {
			if f.dirOf(prev, cur) == d {
				if c > 1.0 {
					c = 1.0
				}
			} else if c < f.CurrentAgainst {
				c = f.CurrentAgainst
			}
		}
		if f.Gate[cur] {
			c += f.GateToll
		}
		if f.Bridge[cur] {
			if usedBridge[cur] {
				c = f.BridgeSecond
			} else if c > f.BridgeFirst {
				c = f.BridgeFirst
			}
			usedBridge[cur] = true
		}
		total += c
		if t >= 2 && f.dirOf(path[t-2], path[t-1]) != f.dirOf(path[t-1], path[t]) {
			total += f.TurnCost
		}
	}
	return total
}

func (f V9Field) V9ReuseBonus(path []int) float64 {
	return f.bonusCostV9(f.V9ReuseCost(f.Realized, path))
}

// ───────────────────────────── деформации пути ─────────────────────────────

// detourViaV9 — вставить в трассу заход к target и обратно (twice — target
// проходится дважды, для «мост/шлюз дважды»). Возврат замкнут: трасса
// продолжается тем же соседом, что и до врезки.
func (f V9Field) detourViaV9(path []int, target int, twice bool) ([]int, bool) {
	if target < 0 {
		return path, false
	}
	bestT, bestD := -1, 1<<30
	for t, c := range path {
		if d := f.V9Manhattan(c, target); d < bestD {
			bestT, bestD = t, d
		}
	}
	if bestT < 0 {
		return path, false
	}
	seg := f.staircaseV9(path[bestT], target)
	if len(seg) < 2 {
		return path, false
	}
	out := append([]int(nil), path[:bestT+1]...)
	out = append(out, seg[1:]...)
	if twice {
		for i := len(seg) - 1; i >= 0; i-- {
			out = append(out, seg[i])
		}
	} else {
		for i := len(seg) - 2; i >= 0; i-- {
			out = append(out, seg[i])
		}
	}
	out = append(out, path[bestT+1:]...)
	return out, true
}

// errRevisitV9 — петля: a→b→a на середине трассы.
func errRevisitV9(f V9Field, path []int) ([]int, bool) {
	if len(path) < 4 {
		return path, false
	}
	t := len(path) / 2
	a, b := path[t], path[t+1]
	out := append([]int(nil), path[:t+1]...)
	out = append(out, b, a)
	out = append(out, path[t+1:]...)
	return out, true
}

// errOvershootV9 — перелёт цели: за Finish к соседу и обратно.
func errOvershootV9(f V9Field, path []int) ([]int, bool) {
	if len(path) < 2 {
		return path, false
	}
	fin := path[len(path)-1]
	z := -1
	for _, st := range f.neighbors4(fin) {
		if st.cell != path[len(path)-2] {
			z = st.cell
			break
		}
	}
	if z < 0 {
		return path, false
	}
	out := append([]int(nil), path...)
	out = append(out, z, fin)
	return out, true
}

// errCurrentAgainstV9 — вход в клетку течения ПРОТИВ направления течения.
func (f V9Field) errCurrentAgainstV9(path []int) ([]int, bool) {
	for c, d := range f.CurrentDir {
		bestT, bestD := -1, 1<<30
		for t, pc := range path {
			if pc == c {
				continue
			}
			if dd := f.V9Manhattan(pc, c); dd < bestD {
				bestT, bestD = t, dd
			}
		}
		if bestT < 0 {
			continue
		}
		A := path[bestT]
		z := -1
		for _, st := range f.neighbors4(c) {
			if st.cell == A {
				continue
			}
			if f.dirOf(st.cell, c) != d {
				z = st.cell
				break
			}
		}
		if z < 0 {
			continue
		}
		toZ := f.staircaseV9(A, z)
		backA := f.staircaseV9(c, A)
		if len(toZ) < 1 || len(backA) < 2 {
			continue
		}
		out := append([]int(nil), path[:bestT+1]...)
		out = append(out, toZ[1:]...)
		out = append(out, c)
		out = append(out, backA[1:]...)
		out = append(out, path[bestT+1:]...)
		return out, true
	}
	return path, false
}

// ───────────────────────── выбор клеток-целей ─────────────────────────

func (f V9Field) nearestCellFromV9(cands []int, path []int) (int, bool) {
	best, bestD := -1, 1<<30
	for _, c := range cands {
		for _, p := range path {
			if d := f.V9Manhattan(c, p); d < bestD {
				bestD, best = d, c
			}
		}
	}
	return best, best >= 0
}

func (f V9Field) keysOfV9(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	return out
}

func (f V9Field) mudCellsV9() []int {
	out := make([]int, 0, len(f.Mud))
	for c := range f.Mud {
		out = append(out, c)
	}
	return out
}

// normalAdjV9 — обычная клетка (Visible==1, без механик), соседняя с трассой.
func (f V9Field) normalAdjV9(path []int) int {
	for _, p := range path {
		for _, st := range f.neighbors4(p) {
			c := st.cell
			if c == f.Start || c == f.Finish || f.Visible[c] != 1.0 {
				continue
			}
			if f.Mud[c] > 0 || f.Wall[c] || f.Gate[c] || f.Bridge[c] || f.Lane[c] ||
				f.DeadEnd[c] || f.Bottleneck[c] || isBeaconV9(&f, c) {
				continue
			}
			return c
		}
	}
	return -1
}

func (f V9Field) sectorCellV9(ct SectorContentV9) int {
	for _, s := range f.Sectors {
		if s.Content == ct && len(s.Cells) > 0 {
			return s.Cells[0]
		}
	}
	return -1
}

// V9ErrSpec — одна ошибка §14.7: номер, ветка (reason), источник, номинальные
// рамки цены из спеки и инъекция в хороший путь (Safe). У ошибок 14–16
// (решения над разведкой) Inject == nil — они меряются отдельно.
type V9ErrSpec struct {
	Num    int
	Reason string
	Source string
	Min    float64
	Max    float64
	Inject func(f V9Field, base []int) ([]int, bool)
}

// V9ErrorTable — 16 ошибок §14.7.
func V9ErrorTable() []V9ErrSpec {
	return []V9ErrSpec{
		{1, "mud_cost", "грубый", -0.05, -0.02, func(f V9Field, base []int) ([]int, bool) {
			c, ok := f.nearestCellFromV9(f.mudCellsV9(), base)
			if !ok {
				return base, false
			}
			return f.detourViaV9(base, c, false)
		}},
		{2, "mud_entry_repeat", "грубый", -0.08, -0.04, func(f V9Field, base []int) ([]int, bool) {
			c, ok := f.nearestCellFromV9(f.mudCellsV9(), base)
			if !ok {
				return base, false
			}
			return f.detourViaV9(base, c, true)
		}},
		{3, "turn_cost", "грубый", -0.04, -0.02, func(f V9Field, base []int) ([]int, bool) {
			c := f.normalAdjV9(base)
			if c < 0 {
				return base, false
			}
			return f.detourViaV9(base, c, false)
		}},
		{4, "revisit", "грубый", -0.04, -0.02, errRevisitV9},
		{5, "overshoot", "грубый", -0.03, -0.03, errOvershootV9},
		{6, "bridge_reuse", "грубый", -0.07, -0.05, func(f V9Field, base []int) ([]int, bool) {
			c, ok := f.nearestCellFromV9(f.keysOfV9(f.Bridge), base)
			if !ok {
				return base, false
			}
			return f.detourViaV9(base, c, true)
		}},
		{7, "gate_useless", "решение", -0.06, -0.06, func(f V9Field, base []int) ([]int, bool) {
			c, ok := f.nearestCellFromV9(f.keysOfV9(f.Gate), base)
			if !ok {
				return base, false
			}
			return f.detourViaV9(base, c, false)
		}},
		{8, "gate_reentry", "грубый", -0.05, -0.05, func(f V9Field, base []int) ([]int, bool) {
			c, ok := f.nearestCellFromV9(f.keysOfV9(f.Gate), base)
			if !ok {
				return base, false
			}
			return f.detourViaV9(base, c, true)
		}},
		{9, "current_against", "решение", -0.12, -0.08, func(f V9Field, base []int) ([]int, bool) {
			return f.errCurrentAgainstV9(base)
		}},
		{10, "dead_end", "решение", -0.15, -0.10, func(f V9Field, base []int) ([]int, bool) {
			out := f.errDeadEnd(base)
			if len(out) == len(base) {
				return base, false
			}
			return out, true
		}},
		{11, "decoy_penalty", "решение под неопр.", -0.12, -0.12, func(f V9Field, base []int) ([]int, bool) {
			c := f.sectorCellV9(ContentDecoyV9)
			if c < 0 {
				return base, false
			}
			return f.detourViaV9(base, c, false)
		}},
		{12, "hidden_trap", "решение под неопр.", -0.25, -0.15, func(f V9Field, base []int) ([]int, bool) {
			c := f.sectorCellV9(ContentTrapV9)
			if c < 0 {
				return base, false
			}
			return f.detourViaV9(base, c, false)
		}},
		{13, "trap_entered_known", "решение под неопр.", -0.18, -0.18, func(f V9Field, base []int) ([]int, bool) {
			c := f.sectorCellV9(ContentTrapV9)
			if c < 0 {
				return base, false
			}
			return f.detourViaV9(base, c, true)
		}},
		{14, "ping_wasted", "решение под неопр.", -0.05, 0.0, nil},
		{15, "ping_destabilize", "решение под неопр.", -0.08, -0.08, nil},
		{16, "lure_missed", "решение под неопр.", -0.10, -0.10, nil},
	}
}

// ───────────────────── разведка: настраиваемый порядок зондов ─────────────────────

// V9SectorOrderV9 — порядок зондирования: !sigAsc — σ по убыванию (лучший
// сначала, как I_σ), sigAsc — σ по возрастанию (впустую: тихие сначала).
func V9SectorOrderV9(f V9Field, sigAsc bool) []int {
	order := make([]int, len(f.Sectors))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		sa := f.Sectors[order[a]].Sig
		sb := f.Sectors[order[b]].Sig
		if sa != sb {
			if sigAsc {
				return sa < sb
			}
			return sa > sb
		}
		ca := f.planCostVisibleV9(order[a])
		cb := f.planCostVisibleV9(order[b])
		if sigAsc {
			return ca > cb
		}
		return ca < cb
	})
	return order
}

// informedCustomBonusV9 — бонус линии I_σ, где первые cfg.Pings секторов
// вскрываются в заданном порядке (решение по вскрытию как в informedPathV9).
func (f V9Field) informedCustomBonusV9(pings int, order []int) float64 {
	n := min(pings, len(order))
	cm := append([]float64(nil), f.Visible...)
	blocked := f.sectorCellSetV9(nil)
	var destab []int
	for k := 0; k < n; k++ {
		j := order[k]
		if j < 0 || j >= len(f.Sectors) {
			continue
		}
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
	p := g.visitAllV9(cm, true).path
	if len(p) < 2 {
		p = f.safePathV9()
	}
	ev := f.destabilizedV9(destab)
	return ev.bonusPathV9(p)
}

// ───────────────────────────── агрегация и отчёт ─────────────────────────────

type V9ErrStat struct {
	num        int
	reason     string
	source     string
	min, max   float64
	applicable int
	ds         []float64
}

type V9ErrAgg struct {
	stats       map[string]*V9ErrStat
	order       []string
	modeCount   map[string]int
	modeProbe   map[string]int
	modeBet     map[string]int
	fields      int
	sAll, sSafe []float64
	pingWasted  []float64
	pingDestab  []float64
	lureMissed  []float64
	pwAppl      int
	pdAppl      int
	lmAppl      int
}

func runV9Errors(t *testing.T, pairs []V9PairRecord, cfg V9Config) V9ErrAgg {
	a := V9ErrAgg{
		stats:     map[string]*V9ErrStat{},
		modeCount: map[string]int{},
		modeProbe: map[string]int{},
		modeBet:   map[string]int{},
	}
	specs := V9ErrorTable()
	for i := range specs {
		a.stats[specs[i].Reason] = &V9ErrStat{num: specs[i].Num, reason: specs[i].Reason, source: specs[i].Source, min: specs[i].Min, max: specs[i].Max}
		a.order = append(a.order, specs[i].Reason)
	}
	for _, r := range pairs {
		f, ok := GenerateFieldV9(HashSeed(r.FromID, r.ToID), r.Dist, V9PairPassport(r), cfg)
		if !ok {
			continue
		}
		base := f.safePathV9()
		if len(base) < 2 {
			continue
		}
		a.fields++
		m := AnalyzeV9(f, cfg)
		bSafe := f.bonusPathV9(base)
		a.sSafe = append(a.sSafe, bSafe)
		a.sAll = append(a.sAll, m.BonusS)

		// Режимы §14.6: доля полей и развилка решений (зонд/ставка).
		a.modeCount[f.Mode]++
		hasNoisy := false
		for _, s := range f.Sectors {
			if s.Sig == SigNoisyV9 {
				hasNoisy = true
			}
		}
		if hasNoisy {
			a.modeProbe[f.Mode]++
		}
		if m.XPlanCells > 0 {
			a.modeBet[f.Mode]++
		}

		// Ошибки 1–13: инъекция в хороший путь Safe.
		for i := range specs {
			if specs[i].Inject == nil {
				continue
			}
			p, ok2 := specs[i].Inject(f, base)
			if !ok2 {
				continue
			}
			st := a.stats[specs[i].Reason]
			st.applicable++
			d := f.V9ReuseBonus(p) - bSafe
			st.ds = append(st.ds, d)
		}

		// 14: зонд впустую — тихие секторы вместо лучших.
		if len(f.Sectors) >= 2 {
			bI := f.informedCustomBonusV9(cfg.Pings, V9SectorOrderV9(f, false))
			bW := f.informedCustomBonusV9(cfg.Pings, V9SectorOrderV9(f, true))
			a.pingWasted = append(a.pingWasted, bW-bI)
			a.pwAppl++
		}
		// 15: зонд в громкий — цена против «не зондировать» (Safe).
		if hasNoisy {
			bLoud := f.informedCustomBonusV9(cfg.Pings, V9SectorOrderV9(f, false))
			a.pingDestab = append(a.pingDestab, bLoud-bSafe)
			a.pdAppl++
		}
		// 16: приманку вскрыл — не использовал (ушёл линией Safe).
		if f.sectorCellV9(ContentLureV9) >= 0 {
			bI := f.informedCustomBonusV9(cfg.Pings, V9SectorOrderV9(f, false))
			a.lureMissed = append(a.lureMissed, bSafe-bI)
			a.lmAppl++
		}
	}
	return a
}

func V9ShareNeg(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	n := 0
	for _, v := range xs {
		if v < -1e-9 {
			n++
		}
	}
	return 100 * float64(n) / float64(len(xs))
}

func V9ErrBucket(m float64) string {
	switch {
	case m >= -1e-9:
		return "0"
	case m >= -0.05:
		return "[0,−0.05)"
	case m >= -0.12:
		return "[−0.05,−0.12)"
	case m >= -0.20:
		return "[−0.12,−0.20)"
	default:
		return "≤−0.20"
	}
}

func (a V9ErrAgg) report(t *testing.T, cfg V9Config) {
	t.Logf("========== §14.7: таблица 16 ошибок (n полей=%d, база = Safe %+.3f)", a.fields, V9Median(a.sSafe))
	t.Logf("%-3s %-22s %-20s %8s %8s %8s %6s %6s %5s", "№", "ветка (reason)", "источник", "мед.Δ", "доля<0", "номина.", "n", "p25", "p75")
	buckets := map[string]int{}
	srcCount := map[string]int{}
	medians := map[string]float64{}
	for _, reason := range a.order {
		st := a.stats[reason]
		var med, p25, p75, share float64
		if len(st.ds) > 0 {
			med = V9Median(st.ds)
			p25 = V9Pctile(st.ds, 25)
			p75 = V9Pctile(st.ds, 75)
			share = V9ShareNeg(st.ds)
		}
		medians[reason] = med
		buckets[V9ErrBucket(med)]++
		srcCount[st.source]++
		t.Logf("%-3d %-22s %-20s %+8.4f %7.1f%% %+5.2f..%+5.2f %6d %+6.3f %+6.3f",
			st.num, reason, st.source, med, share, st.max, st.min, len(st.ds), p25, p75)
	}
	t.Logf("── источники: %v", srcCount)
	t.Logf("── корзины цены (медиана): %v", buckets)
	// 14–16 отдельно (свои базовые линии).
	row := func(name, base string, xs []float64, appl int) {
		if len(xs) == 0 {
			t.Logf("   %-18s n=0", name)
			return
		}
		t.Logf("   %-18s база=%-16s n=%3d мед.Δ=%+.4f доляΔ<0=%5.1f%% (применимо=%d)",
			name, base, len(xs), V9Median(xs), V9ShareNeg(xs), appl)
	}
	t.Logf("── решения над разведкой (14–16) ──")
	row("ping_wasted", "I_σ (лучшие)", a.pingWasted, a.pwAppl)
	row("ping_destabilize", "Safe (не зондир.)", a.pingDestab, a.pdAppl)
	row("lure_missed", "I_σ (использовал)", a.lureMissed, a.lmAppl)
	t.Logf("── различимость: близкие медианы (|Δмед|<0.003) ──")
	dup := 0
	for i := 0; i < len(a.order); i++ {
		for j := i + 1; j < len(a.order); j++ {
			mi, mj := medians[a.order[i]], medians[a.order[j]]
			if math.Abs(mi-mj) < 0.003 {
				dup++
				t.Logf("   ~ %s (%+.4f) ≈ %s (%+.4f)", a.order[i], mi, a.order[j], mj)
			}
		}
	}
	if dup == 0 {
		t.Logf("   нет: все 16 медиан различимы")
	}
}

func (a V9ErrAgg) reportModes(t *testing.T) {
	t.Logf("========== §14.6: режимы — доля полей и развилка решений (n=%d)", a.fields)
	modes := make([]string, 0, len(a.modeCount))
	for m := range a.modeCount {
		modes = append(modes, m)
	}
	sort.Strings(modes)
	t.Logf("%-16s %6s %8s %10s %10s", "режим", "полей", "доля", "зонд(доля)", "ставка(доля)")
	probe := map[string]float64{}
	bet := map[string]float64{}
	for _, m := range modes {
		pc := 100 * float64(a.modeProbe[m]) / float64(max(a.modeCount[m], 1))
		bc := 100 * float64(a.modeBet[m]) / float64(max(a.modeCount[m], 1))
		probe[m], bet[m] = pc, bc
		t.Logf("%-16s %6d %7.1f%% %9.1f%% %9.1f%%", m, a.modeCount[m],
			100*float64(a.modeCount[m])/float64(max(a.fields, 1)), pc, bc)
	}
	pMin, pMax := 101.0, -1.0
	for _, m := range modes {
		pMin, pMax = math.Min(pMin, probe[m]), math.Max(pMax, probe[m])
	}
	bMin, bMax := 101.0, -1.0
	for _, m := range modes {
		bMin, bMax = math.Min(bMin, bet[m]), math.Max(bMax, bet[m])
	}
	t.Logf("── развилка: макс−мин по зонду = %.1f п.п.; по ставке = %.1f п.п. (нужно ≥15)", pMax-pMin, bMax-bMin)
}

func (a V9ErrAgg) reportIgnoreDark(t *testing.T) {
	t.Logf("========== §14.2: линия «игнорировать тёмное» (S) наказывается")
	t.Logf("   Safe (безопасная линия): мед=%+.3f мед.Δ=%+.3f..%+.3f → ровно +0.30 на %.0f%%",
		V9Median(a.sSafe), V9Pctile(a.sSafe, 0), V9Pctile(a.sSafe, 100), V9ShareEq(a.sSafe, 0.30))
	t.Logf("   S (сильный видимый, тёмное игнорит): мед=%+.3f p10=%+.3f p25=%+.3f mean=%+.3f",
		V9Median(a.sAll), V9Pctile(a.sAll, 10), V9Pctile(a.sAll, 25), V9Mean(a.sAll))
	worse := 0
	for _, v := range a.sAll {
		if v < 0.30-1e-9 {
			worse++
		}
	}
	t.Logf("   S хуже Safe (<+0.30): %.1f%%; S на полу ≤−0.08: %.1f%%; S < +0.10: %.1f%%",
		100*float64(worse)/float64(max(len(a.sAll), 1)), V9ShareLE(a.sAll, -0.08), V9ShareLE(a.sAll, 0.10))
}

// TestV9ErrorsHarness — офлайн-прогон ЭТАПА 2 (16 ошибок, разведка, режимы).
func TestV9ErrorsHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("V9 errors harness: только полный прогон, не -short")
	}
	pairs := loadPairsV9(t)
	t.Logf("pairs=%d", len(pairs))
	cfg := DefaultV9Config()
	a := runV9Errors(t, pairs, cfg)
	a.report(t, cfg)
	a.reportModes(t)
	a.reportIgnoreDark(t)
}
