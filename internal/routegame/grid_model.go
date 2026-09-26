// internal/routegame/grid_model.go
// Производственная модель мини-игры «Прокладка маршрута» v9 «Планшет» (спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md, §14): типы поля,
// детерминированный генератор, якоря единой шкалы, кривая bonus и серверная
// оценка/вскрытие пути.
//
// Объекты поля: mud, lane, gate (toll), current_oneway, dead_end_lane,
// one_shot_bridge, bottleneck. Секторы несут ПУБЛИЧНУЮ подпись σ ∈ {Тихий,
// Ровный, Гулкий} и скрытое содержимое (jackpot/lure/trap/decoy/unstable/empty).
//
// Разделение слоёв: видимый (маяки, объекты, клетки и подписи секторов)
// выводится из seed; скрытое содержимое секторов — из secret (см.
// RevealGridSector). Всё — чистые функции: без БД, HTTP, времени и глобального
// RNG (RNG локальный, §0). Старая модель (field.go/evaluate.go/passport.go) не
// затрагивается.
package routegame

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
)

// gridConfig — параметры модели. Числа — калиброванные гипотезы спеки §14.
type gridConfig struct {
	N              int
	KMin, KMax     int
	BeaconDistStep float64
	HBase          int
	HMin, HMax     int
	HazardDistStep float64
	UnrestHazards  int
	KChoices       []float64
	WallCost       float64
	WallGapRow     int // −1 — проход выбирается RNG
	LaneCount      int
	LaneLen        int
	LaneCost       float64
	TurnCost       float64 // τ
	GateToll       float64 // T
	CurrentAgainst float64 // цена «против течения»; «по течению» = 1
	BridgeFirst    float64 // разовый мост, первый проход
	BridgeSecond   float64 // разовый мост, повторный проход
	DeadEndLen     int
	SectorCount    int
	SectorCellMin  int
	SectorCellMax  int
	LureCost       float64 // lure: истинно дешёвый срез
	TrapCost       float64 // trap: дорогая клетка
	DecoyCost      float64 // decoy: обманка (дорого)
	M0Frac         float64 // m0 = M0Frac·L_naive (низ, §14.2)
	CorridorProb   float64
	MaxAttempts    int
}

// defaultGridConfig — базовые значения модели.
func defaultGridConfig() gridConfig {
	return gridConfig{
		N:              10,
		KMin:           4,
		KMax:           6,
		BeaconDistStep: 6000,
		HBase:          3,
		HMin:           3,
		HMax:           9,
		HazardDistStep: 8000,
		UnrestHazards:  1,
		KChoices:       []float64{2, 3},
		WallCost:       9.0,
		WallGapRow:     -1,
		LaneCount:      2,
		LaneLen:        6,
		LaneCost:       0.2,
		TurnCost:       1.9,
		GateToll:       5.5,
		CurrentAgainst: 4.0,
		BridgeFirst:    0.2,
		BridgeSecond:   6.5,
		DeadEndLen:     4,
		SectorCount:    4,
		SectorCellMin:  1,
		SectorCellMax:  3,
		LureCost:       0.2,
		TrapCost:       40.0,
		DecoyCost:      25.0,
		M0Frac:         0.10,
		CorridorProb:   0.6,
		MaxAttempts:    10,
	}
}

// GridSectorSig — публичная подпись сектора σ (§14.3).
type GridSectorSig int

const (
	GridSigQuiet GridSectorSig = iota
	GridSigMedium
	GridSigNoisy
)

// String — player-facing гулкость σ (§1 интерфейсной спеки / §14.12):
// Тихий / Ровный / Гулкий. Стабильные значения строки — для сторонних
// потребителей поля (клиент имеет свои метки).
func (s GridSectorSig) String() string {
	switch s {
	case GridSigNoisy:
		return "Гулкий"
	case GridSigMedium:
		return "Ровный"
	default:
		return "Тихий"
	}
}

// GridSectorContent — истинное содержимое сектора (§14.3).
type GridSectorContent int

const (
	GridContentEmpty GridSectorContent = iota
	GridContentLure
	GridContentTrap
	GridContentDecoy
	GridContentJackpot
	GridContentUnstable
)

func (c GridSectorContent) String() string {
	switch c {
	case GridContentLure:
		return "lure"
	case GridContentTrap:
		return "trap"
	case GridContentDecoy:
		return "decoy"
	case GridContentJackpot:
		return "jackpot"
	case GridContentUnstable:
		return "unstable"
	default:
		return "empty"
	}
}

// GridSectorSurround — видимое окружение сектора (§14.3): признак геометрии
// рядом с клетками сектора. Содержимое — шумная функция σ × окружения.
type GridSectorSurround int

const (
	GridSurroundPlain GridSectorSurround = iota
	GridSurroundGate
	GridSurroundDeadEnd
	GridSurroundMud
	GridSurroundCurrent
)

// String — player-facing окружение сектора (§1 интерфейсной спеки / §14.12):
// Ничего / Кордон / Обрыв / Мгла / Течение.
func (s GridSectorSurround) String() string {
	switch s {
	case GridSurroundGate:
		return "Кордон"
	case GridSurroundDeadEnd:
		return "Обрыв"
	case GridSurroundMud:
		return "Мгла"
	case GridSurroundCurrent:
		return "Течение"
	default:
		return "Ничего"
	}
}

// GridSector — сектор: публичные клетки, подпись σ и окружение; content —
// скрытое содержимое (в JSON не попадает, вскрывается RevealGridSector).
type GridSector struct {
	Cells    []int
	Sig      GridSectorSig
	Surround GridSectorSurround
	content  GridSectorContent
}

// GridLayout — публичная раскладка секторов (без содержимого). Моделирует
// хранимый layout: по нему RevealGridSector вскрывает сектор своим secret.
type GridLayout struct {
	Seed    int64
	N       int
	Sectors []GridSector
}

// GridField — детерминированное поле мини-игры. Публичный слой выводится из
// seed; скрытый (realized/content) — из secret.
type GridField struct {
	Seed          int64
	N             int
	Start, Finish int
	Beacons       []int

	Visible  []float64 // публичная цена клетки (без секторов)
	realized []float64 // истина: секторы по содержимому; клиенту НЕ отдаётся

	Mud        map[int]float64
	Lane       map[int]bool
	Wall       map[int]bool
	Gate       map[int]bool
	CurrentDir map[int]int
	Bridge     map[int]bool
	DeadEnd    map[int]bool
	Bottleneck map[int]bool

	DeadEndAttach int // клетка трассы, к которой пристроен тупик
	DeadEndEntry  int // первая клетка тупикового русла (сосед attach)

	Blocked map[int]bool // непроходимые клетки для конкретной карты планирования

	Sectors []GridSector

	LaneCost, TurnCost, GateToll, CurrentAgainst, BridgeFirst, BridgeSecond float64
	LureCost, TrapCost, DecoyCost                                           float64
	M0Frac                                                                  float64

	// Якоря единой (реализованной) шкалы §14.2.
	LNaive, LSafe, LRisk float64
	VisPath              []int // трасса видимого оптимума
	L0Cells              map[int]bool
	Mode                 string
	Attempt              int
}

// gridDirDI/gridDirDJ — приращения координат по направлению: 0=+i, 1=−i, 2=+j, 3=−j.
var (
	gridDirDI = [4]int{1, -1, 0, 0}
	gridDirDJ = [4]int{0, 0, 1, -1}
)

func gridAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func gridSign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

func gridClampInt(v, lo, hi int) int { return min(max(v, lo), hi) }

func (f GridField) cell(i, j int) int { return j*f.N + i }
func (f GridField) iOf(c int) int     { return c % f.N }
func (f GridField) jOf(c int) int     { return c / f.N }

// gridStep — сосед и направление перехода к нему.
type gridStep struct {
	cell int
	dir  int
}

func (f GridField) neighbors4(c int) []gridStep {
	i, j := f.iOf(c), f.jOf(c)
	out := make([]gridStep, 0, 4)
	if i > 0 {
		out = append(out, gridStep{f.cell(i-1, j), 1})
	}
	if i < f.N-1 {
		out = append(out, gridStep{f.cell(i+1, j), 0})
	}
	if j > 0 {
		out = append(out, gridStep{f.cell(i, j-1), 3})
	}
	if j < f.N-1 {
		out = append(out, gridStep{f.cell(i, j+1), 2})
	}
	return out
}

// adjacent4 — клетки 4-связны (манхэттенское расстояние 1).
func (f GridField) adjacent4(a, b int) bool {
	return gridAbs(f.iOf(a)-f.iOf(b))+gridAbs(f.jOf(a)-f.jOf(b)) == 1
}

func (f GridField) dirOf(a, b int) int {
	di, dj := f.iOf(b)-f.iOf(a), f.jOf(b)-f.jOf(a)
	for d := 0; d < 4; d++ {
		if gridDirDI[d] == di && gridDirDJ[d] == dj {
			return d
		}
	}
	return -1
}

// mixGridSeed — детерминированная перегенерация без времени.
func mixGridSeed(seed, attempt int64) int64 {
	x := uint64(seed) + 0x9E3779B97F4A7C15*uint64(attempt+1)
	x ^= x >> 30
	x *= 0xBF58476D1CE4E5B9
	x ^= x >> 27
	x *= 0x94D049BB133111EB
	x ^= x >> 31
	return int64(x)
}

// gridSecretSeed — стабильный seed скрытого слоя из secret.
func gridSecretSeed(secret []byte) int64 {
	h := fnv.New64a()
	_, _ = h.Write(secret)
	return int64(h.Sum64())
}

// Соли RNG: σ и раскладка секторов (публичные, из seed), содержимое (secret),
// режим задачи.
const (
	gridSigSalt     int64 = 0x51617A5EC0DE
	gridLayoutSalt  int64 = 0x1A40BEEF0DD5
	gridContentSalt int64 = 0x5EC5E7A15EED
	gridModeSalt    int64 = 0xA11CE000
)

// gridModes — режимы задачи §14.6.
var gridModes = []string{"русла", "течения", "шлюзы", "тупики/обманки", "топь"}

// edgeCostGrid — стоимость перехода from→to по базовой карте cm с механиками
// направления/toll/моста (первый проход).
func (f GridField) edgeCostGrid(cm []float64, from, to int) float64 {
	c := cm[to]
	if d, ok := f.CurrentDir[to]; ok {
		if f.dirOf(from, to) == d {
			if c > 1.0 {
				c = 1.0
			}
		} else if c < f.CurrentAgainst {
			c = f.CurrentAgainst
		}
	}
	if f.Gate[to] {
		c += f.GateToll
	}
	if f.Bridge[to] && c > f.BridgeFirst {
		c = f.BridgeFirst
	}
	return c
}

// pathCostGrid — стоимость пути по карте cm: цена клетки, τ за смену направления,
// toll за каждый вход в шлюз, разовый мост.
func (f GridField) pathCostGrid(cm []float64, path []int) float64 {
	if len(path) < 2 {
		return 0
	}
	total := 0.0
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
		if f.Bridge[cur] && c > f.BridgeFirst {
			c = f.BridgeFirst
		}
		total += c
		if t >= 2 && f.dirOf(path[t-2], path[t-1]) != f.dirOf(path[t-1], path[t]) {
			total += f.TurnCost
		}
	}
	return total
}

// staircaseGrid — канонический «лестничный» путь (без обхода).
func (f GridField) staircaseGrid(a, b int) []int {
	path := []int{a}
	ci, cj := f.iOf(a), f.jOf(a)
	bi, bj := f.iOf(b), f.jOf(b)
	for ci != bi || cj != bj {
		di, dj := bi-ci, bj-cj
		switch {
		case di == 0:
			cj += gridSign(dj)
		case dj == 0:
			ci += gridSign(di)
		case gridAbs(di) >= gridAbs(dj):
			ci += gridSign(di)
		default:
			cj += gridSign(dj)
		}
		path = append(path, f.cell(ci, cj))
	}
	return path
}

// gridNaturalOrder — естественный порядок обхода маяков (по i, затем j).
func gridNaturalOrder(f GridField) []int {
	order := append([]int(nil), f.Beacons...)
	gridSortIntsStable(order, func(a, b int) bool {
		ia, ja := f.iOf(a), f.jOf(a)
		ib, jb := f.iOf(b), f.jOf(b)
		if ia != ib {
			return ia < ib
		}
		return ja < jb
	})
	return order
}

func gridSortIntsStable(order []int, less func(a, b int) bool) {
	// вставками: k ≤ 6, зато без импорта sort и лишних аллокаций.
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && less(order[j], order[j-1]); j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
}

type gridBeaconPolicy int
type gridHazardBias int

const (
	gridBeaconZigzag gridBeaconPolicy = iota
	gridBeaconRing
	gridBeaconCross
	gridBeaconCluster
)

const (
	gridHazardCorridors gridHazardBias = iota
	gridHazardCluster
)

func gridPassportProfile(p Passport) (gridBeaconPolicy, gridHazardBias, float64) {
	policy := gridBeaconZigzag
	switch {
	case p.From.SystemType == "multiple" || p.To.SystemType == "multiple":
		policy = gridBeaconRing
	case p.From.SystemType == "binary" || p.To.SystemType == "binary":
		policy = gridBeaconCross
	}
	bias := gridHazardCorridors
	if len(p.DestinationBelts) > 0 {
		bias = gridHazardCluster
	}
	exotic := 0
	for _, s := range []PassportStar{p.From, p.To} {
		if s.StarType != "" && s.StarType != "star" {
			exotic++
		}
		if s.Temperature >= highTempK {
			exotic++
		}
	}
	p3 := 0.08 * float64(min(exotic, 4))
	return policy, bias, p3
}

func gridBeaconCount(dist float64, cfg gridConfig) int {
	if dist < 0 {
		dist = 0
	}
	k := cfg.KMin + int(dist/cfg.BeaconDistStep)
	return min(max(k, cfg.KMin), cfg.KMax)
}

func gridHazardCount(dist float64, unrest int, cfg gridConfig) int {
	if dist < 0 {
		dist = 0
	}
	h := cfg.HBase + int(dist/cfg.HazardDistStep) + cfg.UnrestHazards*unrest
	return min(max(h, cfg.HMin), cfg.HMax)
}

// GenerateGridField строит поле из seed (видимый слой), secret (скрытый слой),
// dist и паспорта. Гейт §14.2: L_naive > L_safe (иначе перегенерация до
// MaxAttempts).
func GenerateGridField(seed int64, secret []byte, dist float64, passport Passport) (GridField, bool) {
	cfg := defaultGridConfig()
	secretSeed := gridSecretSeed(secret)
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		f := buildGridField(mixGridSeed(seed, int64(attempt)), secretSeed, dist, passport, cfg)
		if f.LNaive-f.LSafe > 1e-9 {
			f.Attempt = attempt
			return f, true
		}
	}
	return GridField{}, false
}

func buildGridField(seed, secretSeed int64, dist float64, passport Passport, cfg gridConfig) GridField {
	rng := rand.New(rand.NewSource(seed))
	n := cfg.N
	f := GridField{Seed: seed, N: n, TurnCost: cfg.TurnCost, GateToll: cfg.GateToll,
		CurrentAgainst: cfg.CurrentAgainst, BridgeFirst: cfg.BridgeFirst, BridgeSecond: cfg.BridgeSecond,
		LaneCost: cfg.LaneCost, LureCost: cfg.LureCost, TrapCost: cfg.TrapCost, DecoyCost: cfg.DecoyCost,
		M0Frac: cfg.M0Frac,
		Mud:    map[int]float64{}, Lane: map[int]bool{}, Wall: map[int]bool{}, Gate: map[int]bool{},
		CurrentDir: map[int]int{}, Bridge: map[int]bool{}, DeadEnd: map[int]bool{},
		Bottleneck: map[int]bool{}}
	span := max(n-4, 1)
	f.Start = f.cell(0, 2+rng.Intn(span))
	f.Finish = f.cell(n-1, 2+rng.Intn(span))

	policy, bias, p3 := gridPassportProfile(passport)
	k := gridBeaconCount(dist, cfg)
	unrest := passportUnrest(passport)
	h := gridHazardCount(dist, unrest, cfg)

	lanes := gridPlaceLanes(rng, cfg)
	wall, gaps := gridPlaceWall(rng, cfg, f.Start, f.Finish)
	f.Beacons = gridPlaceBeacons(rng, cfg, policy, k, f.Start, f.Finish, lanes, wall)
	occ := map[int]bool{f.Start: true, f.Finish: true}
	for _, b := range f.Beacons {
		occ[b] = true
	}
	mud := gridPlaceMud(rng, cfg, bias, h, unrest, p3, occ, lanes, wall, f.jOf(f.Start), f.jOf(f.Finish))

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
	route := f.visitAllGrid(f.Visible, true)
	routePath := route.path
	if len(routePath) == 0 {
		routePath = f.buildRouteStairGrid(gridNaturalOrder(f))
	}
	f.VisPath = routePath
	f.L0Cells = map[int]bool{}
	for _, c := range f.buildRouteStairGrid(gridNaturalOrder(f)) {
		f.L0Cells[c] = true
	}
	gridAdjustWallGap(&f, routePath, cfg)

	gridPlaceSpecials(&f, rng, cfg, routePath)
	gridPlaceDeadEnd(&f, rng, cfg, routePath)
	f.Mode = gridPickMode(seed, &f, unrest)
	gridPlaceSectors(&f, seed, secretSeed, cfg, routePath, f.Mode)
	gridBuildMaps(&f)
	gridComputeAnchors(&f)
	return f
}

// gridPickMode — режим задачи §14.6 из seed + видимого поля.
func gridPickMode(seed int64, f *GridField, unrest int) string {
	rng := rand.New(rand.NewSource(mixGridSeed(seed, gridModeSalt)))
	weights := map[string]float64{
		"русла":          1.0 + 0.05*float64(len(f.Lane)),
		"течения":        1.0 + 0.30*float64(len(f.CurrentDir)),
		"шлюзы":          1.0 + 0.30*float64(len(f.Gate)),
		"тупики/обманки": 1.0 + 0.30*float64(min(len(f.DeadEnd), 4)),
		"топь":           1.0 + 0.10*float64(len(f.Mud)) + 0.05*float64(unrest),
	}
	best, bestW := "", -1.0
	for _, m := range gridModes {
		w := weights[m] * (0.60 + 0.80*rng.Float64())
		if w > bestW {
			bestW, best = w, m
		}
	}
	return best
}

// gridComputeAnchors — якоря единой (реализованной) шкалы §14.2.
func gridComputeAnchors(f *GridField) {
	f.LNaive = f.pathCostGrid(f.realized, f.buildRouteStairGrid(gridNaturalOrder(*f)))
	blocked := f.withBlockedGrid(f.sectorCellSetGrid(nil))
	if w := blocked.visitAllGrid(f.realized, false); !math.IsInf(w.cost, 1) {
		f.LSafe = w.cost
	} else {
		f.LSafe = f.LNaive
	}
	f.LRisk = f.LSafe
	for j, s := range f.Sectors {
		if s.content != GridContentJackpot && s.content != GridContentLure {
			continue // «срез» даёт только благоприятное содержимое
		}
		if p := f.planGridSector(j); len(p) >= 2 {
			if c := f.pathCostGrid(f.realized, p); c < f.LRisk {
				f.LRisk = c
			}
		}
	}
	if f.LRisk > f.LSafe {
		f.LRisk = f.LSafe
	}
}

// planGridSector — план «ставка на сектор j»: сектор j считается дешёвым срезом,
// прочие секторы обходятся (тёмное вне ставки). Планирование — по публичной карте.
func (f GridField) planGridSector(j int) []int {
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
	g := f.withBlockedGrid(blocked)
	if w := g.visitAllGrid(cm, true); len(w.path) >= 2 {
		return w.path
	}
	return nil
}

// withBlockedGrid — копия поля с непроходимыми клетками (карта планирования).
func (f GridField) withBlockedGrid(cells map[int]bool) GridField {
	g := f
	g.Blocked = cells
	return g
}

// sectorCellSetGrid — множество клеток секторов, для которых pred даёт true
// (pred == nil — все секторы).
func (f GridField) sectorCellSetGrid(pred func(GridSectorContent) bool) map[int]bool {
	out := map[int]bool{}
	for _, s := range f.Sectors {
		if pred != nil && !pred(s.content) {
			continue
		}
		for _, c := range s.Cells {
			out[c] = true
		}
	}
	return out
}

// bonusCostGrid — кусочно-линейная кривая §14.2 на единой (реализованной) шкале:
//
//	C >= L_naive          → 0.10 − 0.30·min(1, (C−L_naive)/m0), m0 = 0.10·L_naive
//	L_safe <= C < L_naive → 0.10 + 0.20·u, u = (L_naive−C)/(L_naive−L_safe)
//	C < L_safe            → 0.30 + 0.20·w, w = (L_safe−C)/(L_safe−L_risk)
//
// Бонус может быть отрицательным (низ до −0.20).
func (f GridField) bonusCostGrid(C float64) float64 {
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

// bonusPathGrid — бонус пути, посчитанного на реализованном поле (§14.2).
func (f GridField) bonusPathGrid(path []int) float64 {
	if len(path) < 2 {
		return 0.10
	}
	return f.bonusCostGrid(f.pathCostGrid(f.realized, path))
}

// Причины невалидности пути. Стабильные коды для хендлера.
const (
	GridReasonTooFewCells     = "too_few_cells"
	GridReasonCellOutOfBounds = "cell_out_of_bounds"
	GridReasonStepTooSmall    = "step_too_small"
	GridReasonNotAdjacent     = "not_adjacent"
	GridReasonStartMismatch   = "start_mismatch"
	GridReasonFinishMismatch  = "finish_mismatch"
	GridReasonBeaconMissing   = "beacon_missing"
)

// EvaluateGridPath — серверная оценка пути (§14.2/§14.7). Валидный путь:
// 4-связная цепочка клеток, начинается в Start, заканчивается в Finish,
// проходит через все маяки. Бонус — по кривой bonusCostGrid; может быть
// отрицательным. Невалидный путь → bonus=0, valid=false, reason≠"".
func EvaluateGridPath(field GridField, cells []int) (bonus float64, valid bool, reason string) {
	if len(cells) < 2 {
		return 0, false, GridReasonTooFewCells
	}
	n2 := field.N * field.N
	for _, c := range cells {
		if c < 0 || c >= n2 {
			return 0, false, GridReasonCellOutOfBounds
		}
	}
	if cells[0] != field.Start {
		return 0, false, GridReasonStartMismatch
	}
	if cells[len(cells)-1] != field.Finish {
		return 0, false, GridReasonFinishMismatch
	}
	// Минимальный шаг пути (анти-бот, §6.6): соседние клетки обязаны быть
	// различны — «дребезг» в одной клетке (шаг < 1 клетки) отклоняется своим
	// кодом до проверки 4-связности. Честное рисование по клеткам такого пути
	// не даёт (клиент дедуплицирует точки).
	for t := 1; t < len(cells); t++ {
		if cells[t] == cells[t-1] {
			return 0, false, GridReasonStepTooSmall
		}
	}
	for t := 1; t < len(cells); t++ {
		if !field.adjacent4(cells[t-1], cells[t]) {
			return 0, false, GridReasonNotAdjacent
		}
	}
	onPath := make(map[int]bool, len(cells))
	for _, c := range cells {
		onPath[c] = true
	}
	for _, b := range field.Beacons {
		if !onPath[b] {
			return 0, false, GridReasonBeaconMissing
		}
	}
	return field.bonusPathGrid(cells), true, ""
}

// Layout — публичная раскладка секторов (без содержимого) для хранения и вскрытия.
func (f GridField) Layout() GridLayout {
	return GridLayout{Seed: f.Seed, N: f.N, Sectors: f.Sectors}
}

// RevealGridSector вскрывает содержимое сектора idx по secret. Содержимое —
// детерминированная функция secret + подпись σ + видимое окружение, поэтому
// сектор вскрывается только своим secret (чужой даёт другой результат).
func RevealGridSector(secret []byte, layout GridLayout, idx int) (string, error) {
	if idx < 0 || idx >= len(layout.Sectors) {
		return "", fmt.Errorf("routegame: sector %d out of range [0,%d)", idx, len(layout.Sectors))
	}
	s := layout.Sectors[idx]
	c := pickGridSectorContent(gridSecretSeed(secret), idx, s.Sig, s.Surround)
	return c.String(), nil
}
