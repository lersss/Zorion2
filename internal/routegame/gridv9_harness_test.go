package routegame

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// V9PairRecord — одна упорядоченная пара миров из dev-БД (read-only выборка).
type V9PairRecord struct {
	FromID       string  `json:"from_id"`
	ToID         string  `json:"to_id"`
	Dist         float64 `json:"dist"`
	Band         string  `json:"band"`
	FromSpectral string  `json:"from_spectral"`
	FromStar     string  `json:"from_star"`
	FromSystem   string  `json:"from_system"`
	FromTemp     int     `json:"from_temp"`
	ToSpectral   string  `json:"to_spectral"`
	ToStar       string  `json:"to_star"`
	ToSystem     string  `json:"to_system"`
	ToTemp       int     `json:"to_temp"`
	Belt         bool    `json:"belt"`
}

func V9PairPassport(r V9PairRecord) Passport {
	p := Passport{
		Dist: r.Dist,
		From: PassportStar{SpectralClass: r.FromSpectral, StarType: r.FromStar, SystemType: r.FromSystem, Temperature: r.FromTemp},
		To:   PassportStar{SpectralClass: r.ToSpectral, StarType: r.ToStar, SystemType: r.ToSystem, Temperature: r.ToTemp},
	}
	if r.Belt {
		p.DestinationBelts = []PassportBelt{{Kind: "asteroid"}}
	}
	return p
}

func loadPairsV9(t *testing.T) []V9PairRecord {
	t.Helper()
	candidates := []string{}
	if p := os.Getenv("ROUTE_PAIRS"); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates,
		filepath.Join(os.TempDir(), "opencode", "route_pairs.jsonl"),
		filepath.Join("testdata", "route_pairs.jsonl"),
	)
	var path string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			path = c
			break
		}
	}
	if path == "" {
		t.Skipf("выборка пар недоступна: %v", candidates)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("выборка пар недоступна (%s): %v", path, err)
	}
	defer f.Close()
	limit := 0
	if v := os.Getenv("ROUTE_PAIRS_LIMIT"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	var out []V9PairRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" {
			continue
		}
		var r V9PairRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("bad pair line: %v", err)
		}
		out = append(out, r)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan pairs: %v", err)
	}
	return out
}

type V9Row struct {
	band                            string
	unrest                          int
	k, h, nsect                     int
	jackpot, unstable               bool
	bL0, bC, bS, bSafe              float64
	bX, bI, bB, bF, bRule, bOracle  float64
	xSig                            SectorSigV9
	xContent                        SectorContentV9
	xUsed                           int
	ln, ls                          float64
}

type V9Agg struct {
	rows             []V9Row
	total, noGame    int
	distMin, distMax float64
	kSet             map[int]bool
	sigCounts        map[SectorSigV9]int
	contentCounts    map[SectorContentV9]int
	surCounts        map[SectorSurroundV9]int
	noisyBySur       map[SectorSurroundV9]map[SectorContentV9]int
	jackpotFields    int
}

func runV9Agg(t *testing.T, pairs []V9PairRecord, cfg V9Config) V9Agg {
	a := V9Agg{kSet: map[int]bool{}, sigCounts: map[SectorSigV9]int{}, contentCounts: map[SectorContentV9]int{},
		surCounts: map[SectorSurroundV9]int{}, noisyBySur: map[SectorSurroundV9]map[SectorContentV9]int{}, distMin: math.Inf(1)}
	for _, r := range pairs {
		a.total++
		a.distMax = math.Max(a.distMax, r.Dist)
		a.distMin = math.Min(a.distMin, r.Dist)
		f, ok := GenerateFieldV9(HashSeed(r.FromID, r.ToID), r.Dist, V9PairPassport(r), cfg)
		if !ok {
			a.noGame++
			continue
		}
		m := AnalyzeV9(f, cfg)
		row := V9Row{band: r.Band, unrest: passportUnrest(V9PairPassport(r)),
			k: m.K, h: m.H, nsect: len(f.Sectors), xSig: m.XSig, xContent: m.XContent, xUsed: m.XPlanCells,
			ln: f.LNaive, ls: f.LSafe,
			bL0: m.BonusL0, bC: m.BonusC, bS: m.BonusS, bSafe: m.BonusSafe,
			bX: m.BonusX, bI: m.BonusI, bB: m.BonusB, bF: m.BonusF, bRule: m.BonusRule, bOracle: m.BonusOracle}
		for _, s := range f.Sectors {
			a.sigCounts[s.Sig]++
			a.contentCounts[s.Content]++
			a.surCounts[s.Surround]++
			if s.Sig == SigNoisyV9 {
				if a.noisyBySur[s.Surround] == nil {
					a.noisyBySur[s.Surround] = map[SectorContentV9]int{}
				}
				a.noisyBySur[s.Surround][s.Content]++
			}
			if s.Content == ContentJackpotV9 {
				row.jackpot = true
			}
			if s.Content == ContentUnstableV9 {
				row.unstable = true
			}
		}
		a.rows = append(a.rows, row)
		if row.jackpot {
			a.jackpotFields++
		}
		a.kSet[m.K] = true
	}
	return a
}

func (a V9Agg) col(f func(V9Row) float64) []float64 {
	out := make([]float64, 0, len(a.rows))
	for _, r := range a.rows {
		out = append(out, f(r))
	}
	return out
}

func V9Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range xs {
		s += v
	}
	return s / float64(len(xs))
}

func V9Var(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := V9Mean(xs)
	s := 0.0
	for _, v := range xs {
		s += (v - m) * (v - m)
	}
	return s / float64(len(xs))
}

func V9Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func V9Pctile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if p <= 0 {
		return s[0]
	}
	if p >= 100 {
		return s[len(s)-1]
	}
	return s[int(math.Round(p/100*float64(len(s)-1)))]
}

func V9ShareLE(xs []float64, thr float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	n := 0
	for _, v := range xs {
		if v <= thr+1e-9 {
			n++
		}
	}
	return 100 * float64(n) / float64(len(xs))
}

func V9ShareIn(xs []float64, lo, hi float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	n := 0
	for _, v := range xs {
		if v >= lo-1e-9 && v <= hi+1e-9 {
			n++
		}
	}
	return 100 * float64(n) / float64(len(xs))
}

func V9ShareEq(xs []float64, v float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	n := 0
	for _, x := range xs {
		if math.Abs(x-v) <= 1e-6 {
			n++
		}
	}
	return 100 * float64(n) / float64(len(xs))
}

func V9KeySorted(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func (a V9Agg) report(t *testing.T, cfg V9Config) {
	n := len(a.rows)
	t.Logf("========== модель V9 «Планшет» (единая реализованная шкала): %s", cfg.Name())
	t.Logf("выборка: total=%d accepted=%d (%.1f%%) noGame=%d dist=%.0f..%.0f jackpot-полей=%d (%.1f%%)",
		a.total, n, 100*float64(n)/float64(max(a.total, 1)), a.noGame, a.distMin, a.distMax,
		a.jackpotFields, 100*float64(a.jackpotFields)/float64(max(n, 1)))
	if n == 0 {
		t.Logf("нет принятых полей")
		return
	}
	l0 := a.col(func(r V9Row) float64 { return r.bL0 })
	c := a.col(func(r V9Row) float64 { return r.bC })
	s := a.col(func(r V9Row) float64 { return r.bS })
	safe := a.col(func(r V9Row) float64 { return r.bSafe })
	x := a.col(func(r V9Row) float64 { return r.bX })
	i := a.col(func(r V9Row) float64 { return r.bI })
	b := a.col(func(r V9Row) float64 { return r.bB })
	ff := a.col(func(r V9Row) float64 { return r.bF })
	or := a.col(func(r V9Row) float64 { return r.bOracle })
	rule := a.col(func(r V9Row) float64 { return r.bRule })

	t.Logf("── Критерии §14.10 (n=%d) ──", n)

	t.Logf("[пол] bonus_L0=+0.10±0.02: %.1f%% (≥95%%); bonus_F≤−0.08: %.1f%% (≥50%%) мед.F=%+.3f",
		V9ShareIn(l0, 0.08, 0.12), V9ShareLE(ff, -0.08), V9Median(ff))

	eX, eSafe := V9Mean(x), V9Mean(safe)
	// P(X_σ < Safe) — доля полей, где рисковая линия хуже безопасной.
	ltSafe := 0
	for k := range x {
		if x[k] < safe[k]-1e-9 {
			ltSafe++
		}
	}
	pLtSafe := 100 * float64(ltSafe) / float64(max(n, 1))
	t.Logf("[премия за риск] E[X_σ]=%+.4f E[Safe]=%+.4f Δ=%+.4f (нужно [+0.04;+0.08]); Var[X]=%.4f > Var[Safe]=%.4f: %v; P(X_σ<Safe)=%.1f%% (≥30%%)",
		eX, eSafe, eX-eSafe, V9Var(x), V9Var(safe), V9Var(x) > V9Var(safe), pLtSafe)

	eI := V9Mean(i)
	t.Logf("[разведка выгодна] E[I_σ]−E[Safe]=%+.4f (≥+0.05); E[I_σ]−E[X_σ]=%+.4f",
		eI-eSafe, eI-eX)

	eRule := V9Mean(rule)
	t.Logf("[нет единого скрипта] E[I_σ]−E[Rule]=%+.4f (≥+0.05); E[Rule]=%+.4f E[I_σ]=%+.4f",
		eI-eRule, eRule, eI)

	p50x, p50b, p50c := V9ShareEq(x, 0.50), V9ShareEq(b, 0.50), V9ShareEq(c, 0.50)
	t.Logf("[верх решением] P(+0.50|X_σ)=%.1f%% ≥ 1.5×P(+0.50|B)=%.1f%%: %v; P(+0.50|C)=%.1f%% (≤10%%); P(+0.50|I_σ)=%.1f%%",
		p50x, p50b, p50x >= 1.5*p50b, p50c, V9ShareEq(i, 0.50))

	t.Logf("[сильная видимая не наказана] bonus(S)≥+0.24: %.1f%% (≥95%%); p10(S)=%+.3f (≥+0.05); Safe≥+0.24: %.1f%%",
		V9ShareIn(s, 0.24, 10), V9Pctile(s, 10), V9ShareIn(safe, 0.24, 10))

	pubMean := math.Max(math.Max(V9Mean(c), V9Mean(s)), math.Max(eSafe, math.Max(eX, V9Mean(b))))
	pubP50 := math.Max(math.Max(p50c, V9ShareEq(s, 0.50)), math.Max(V9ShareEq(safe, 0.50), math.Max(p50x, p50b)))
	t.Logf("[анти-автоматизация] E ≤ +0.40: max(E[C],E[S],E[Safe],E[X],E[B])=%+.4f → %v; P(+0.50) ≤ 55%%: max=%.1f%% → %v",
		pubMean, pubMean <= 0.40+1e-9, pubP50, pubP50 <= 55.0+1e-9)
	t.Logf("[анти-автоматизация, справка] E[L0]=%+.4f E[F]=%+.4f E[I_oracle]=%+.4f",
		V9Mean(l0), V9Mean(ff), V9Mean(or))

	t.Logf("── Распределения bonus (единая шкала) ──")
	for _, sc := range []struct {
		name string
		xs   []float64
	}{{"L0", l0}, {"C", c}, {"S", s}, {"Safe", safe}, {"X", x}, {"I", i}, {"B", b}, {"F", ff}, {"Rule", rule}, {"Orcl", or}} {
		t.Logf("   %-4s min=%+.3f p10=%+.3f p25=%+.3f med=%+.3f p75=%+.3f p90=%+.3f max=%+.3f mean=%+.3f",
			sc.name, V9Pctile(sc.xs, 0), V9Pctile(sc.xs, 10), V9Pctile(sc.xs, 25), V9Median(sc.xs),
			V9Pctile(sc.xs, 75), V9Pctile(sc.xs, 90), V9Pctile(sc.xs, 100), V9Mean(sc.xs))
	}
	t.Logf("── выборка ── k=%v", V9KeySorted(a.kSet))
	t.Logf("── σ: тихая=%d средняя=%d шумная=%d; content: empty=%d lure=%d trap=%d decoy=%d jackpot=%d unstable=%d ──",
		a.sigCounts[SigQuietV9], a.sigCounts[SigMediumV9], a.sigCounts[SigNoisyV9],
		a.contentCounts[ContentEmptyV9], a.contentCounts[ContentLureV9], a.contentCounts[ContentTrapV9],
		a.contentCounts[ContentDecoyV9], a.contentCounts[ContentJackpotV9], a.contentCounts[ContentUnstableV9])
	t.Logf("── окружение секторов: пусто=%d шлюз=%d тупик=%d топь=%d течение=%d ──",
		a.surCounts[SurroundPlainV9], a.surCounts[SurroundGateV9], a.surCounts[SurroundDeadEndV9],
		a.surCounts[SurroundMudV9], a.surCounts[SurroundCurrentV9])
	t.Logf("── σ×окружение → content (шумная) ──")
	for _, sur := range []SectorSurroundV9{SurroundGateV9, SurroundDeadEndV9, SurroundMudV9, SurroundCurrentV9, SurroundPlainV9} {
		cc := a.noisyBySur[sur]
		tot := cc[ContentEmptyV9] + cc[ContentLureV9] + cc[ContentTrapV9] + cc[ContentDecoyV9] + cc[ContentJackpotV9] + cc[ContentUnstableV9]
		if tot == 0 {
			continue
		}
		t.Logf("   шумная+%-7s n=%3d jackpot=%4.1f%% trap=%4.1f%% empty=%4.1f%% decoy=%4.1f%% unstable=%4.1f%%",
			sur, tot, 100*float64(cc[ContentJackpotV9])/float64(tot), 100*float64(cc[ContentTrapV9])/float64(tot),
			100*float64(cc[ContentEmptyV9])/float64(tot), 100*float64(cc[ContentDecoyV9])/float64(tot),
			100*float64(cc[ContentUnstableV9])/float64(tot))
	}

	// Диагностика ставки X_σ по истинному содержимому выбранного сектора.
	t.Logf("── X_σ: исход по content выбранного сектора ──")
	byXC := map[SectorContentV9][]float64{}
	usedXC := map[SectorContentV9]int{}
	for _, r := range a.rows {
		byXC[r.xContent] = append(byXC[r.xContent], r.bX)
		if r.xUsed > 0 {
			usedXC[r.xContent]++
		}
	}
	for _, ct := range []SectorContentV9{ContentJackpotV9, ContentLureV9, ContentUnstableV9, ContentEmptyV9, ContentDecoyV9, ContentTrapV9} {
		xs := byXC[ct]
		if len(xs) == 0 {
			continue
		}
		t.Logf("   %-9s n=%3d mean(X)=%+.3f med=%+.3f p10=%+.3f p90=%+.3f P(+0.50)=%5.1f%% использован=%5.1f%%",
			ct, len(xs), V9Mean(xs), V9Median(xs), V9Pctile(xs, 10), V9Pctile(xs, 90), V9ShareEq(xs, 0.50),
			100*float64(usedXC[ct])/float64(len(xs)))
	}
	// Диагностика: сколько клеток выбранного сектора реально на трассе X.
	t.Logf("── X_σ: клеток сектора на трассе (mean) ──")
	cellSum := map[SectorContentV9]int{}
	cellN := map[SectorContentV9]int{}
	for _, r := range a.rows {
		cellSum[r.xContent] += r.xUsed
		cellN[r.xContent]++
	}
	for _, ct := range []SectorContentV9{ContentJackpotV9, ContentTrapV9, ContentEmptyV9, ContentDecoyV9} {
		if cellN[ct] == 0 {
			continue
		}
		t.Logf("   %-9s n=%3d среднее клеток на трассе=%.2f", ct, cellN[ct], float64(cellSum[ct])/float64(cellN[ct]))
	}
	// Отладка: значения trap-X.
	{
		for _, r := range a.rows {
			if r.xContent == ContentTrapV9 {
				t.Logf("   [debug] trap-X bonus=%+.3f cells=%d Ln=%.1f Ls=%.1f", r.bX, r.xUsed, r.ln, r.ls)
			}
		}
	}

	// Расслоение по band.
	t.Logf("── Расслоение dist(band): mean(X)/mean(Safe)/mean(I)/P(+0.50|X) ──")
	byBand := map[string][]V9Row{}
	for _, r := range a.rows {
		byBand[r.band] = append(byBand[r.band], r)
	}
	bands := make([]string, 0, len(byBand))
	for k := range byBand {
		bands = append(bands, k)
	}
	sort.Strings(bands)
	for _, bd := range bands {
		rs := byBand[bd]
		xs, ss, is := make([]float64, 0, len(rs)), make([]float64, 0, len(rs)), make([]float64, 0, len(rs))
		for _, r := range rs {
			xs = append(xs, r.bX)
			ss = append(ss, r.bSafe)
			is = append(is, r.bI)
		}
		t.Logf("   %-7s n=%3d mean(X)=%+.3f mean(Safe)=%+.3f mean(I)=%+.3f P(+0.50|X)=%5.1f%%",
			bd, len(rs), V9Mean(xs), V9Mean(ss), V9Mean(is), V9ShareEq(xs, 0.50))
	}
}

func reportExamplesV9(t *testing.T, pairs []V9PairRecord, cfg V9Config) {
	t.Logf("== живые примеры")
	shown := 0
	seen := map[string]bool{}
	emit := func(r V9PairRecord) bool {
		key := r.FromID + "→" + r.ToID
		if seen[key] || shown >= 5 {
			return false
		}
		f, ok := GenerateFieldV9(HashSeed(r.FromID, r.ToID), r.Dist, V9PairPassport(r), cfg)
		if !ok {
			return false
		}
		m := AnalyzeV9(f, cfg)
		nh := 0
		for _, cc := range f.Visible {
			if cc >= 2 {
				nh++
			}
		}
		sectDesc := ""
		for _, s := range f.Sectors {
			sectDesc += fmt.Sprintf("%s/%s/%d ", s.Sig, s.Content, len(s.Cells))
		}
		t.Logf("   %s→%s dist=%.0f band=%s k=%d h=%d секторы=[%s]", shortID(r.FromID), shortID(r.ToID), r.Dist, r.Band, m.K, nh, strings.TrimSpace(sectDesc))
		t.Logf("      Ln=%.1f Lsafe=%.1f Lrisk=%.1f | L0=%+.2f C=%+.2f S=%+.2f Safe=%+.2f X(%s/%s)=%+.2f I=%+.2f B=%+.2f F=%+.2f | I−Safe=%+.2f I−X=%+.2f Orcl=%+.2f",
			f.LNaive, f.LSafe, f.LRisk,
			m.BonusL0, m.BonusC, m.BonusS, m.BonusSafe, m.XSig, m.XContent, m.BonusX, m.BonusI, m.BonusB, m.BonusF,
			m.BonusI-m.BonusSafe, m.BonusI-m.BonusX, m.BonusOracle)
		seen[key] = true
		shown++
		return true
	}
	for _, band := range []string{"short", "medium", "long"} {
		for _, r := range pairs {
			if r.Band == band && emit(r) {
				break
			}
		}
	}
	for _, r := range pairs {
		if shown >= 5 {
			break
		}
		emit(r)
	}
}

func shortID(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}

// TestV9Harness — офлайн-прогон модели v9 по dev-выборке (§14). Тяжёлый, вне
// быстрого DoD-цикла: пропускается в -short. В БД не пишет.
func TestV9Harness(t *testing.T) {
	if testing.Short() {
		t.Skip("V9 harness: только полный прогон, не -short")
	}
	pairs := loadPairsV9(t)
	t.Logf("pairs=%d", len(pairs))
	cfg := DefaultV9Config()
	runV9Agg(t, pairs, cfg).report(t, cfg)
	reportExamplesV9(t, pairs, cfg)
}
