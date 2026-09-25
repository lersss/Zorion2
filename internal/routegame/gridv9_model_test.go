// internal/routegame/gridv9.go
// Модель v9 «Планшет» мини-игры «Прокладка маршрута» — офлайн-замер спеки
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md, раздел §14
// (черновик v9, §14.1–§14.11).
//
// Объекты поля и τ — из v7: mud, lane, gate (toll), current_oneway,
// dead_end_lane, one_shot_bridge, bottleneck. Скрытый слой — секторы с
// ПУБЛИЧНОЙ подписью σ ∈ {тихая, средняя, шумная} (из публичного seed, видна
// игроку) и истинным content ∈ {jackpot, lure, trap, decoy, unstable, empty} —
// шумная функция от σ (§14.3).
//
// Отличие от v7: три якоря единой (реализованной) шкалы L_naive → L_safe →
// L_risk (§14.2); низ через фиксированный запас m0 = 0.10·L_naive (снятие
// honeypot'а); guard L_naive > L_safe; разведка с ценой (R1–R5), тип unstable.
//
// Всё — чистые функции: БД/HTTP/времени/глобального RNG нет (RNG локальный, §0).
// Прод-поведение (field.go/evaluate.go/хендлеры/UI) не затрагивается.
package routegame

import "fmt"

// V9Config — параметры модели v9. Все числа — ГИПОТЕЗЫ, калибруются замером §14.10.
type V9Config struct {
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
	CurrentAgainst float64 // K (цена «против течения»); «по течению» = 1
	BridgeFirst    float64 // разовый мост, первый проход
	BridgeSecond   float64 // разовый мост, повторный проход (полная K)
	DeadEndLen     int     // длина тупикового русла
	SectorCount    int
	SectorCellMax  int
	SectorCellMin  int
	LureCost       float64 // lure: истинно дешёвый срез
	TrapCost       float64 // trap: дорогая клетка
	DecoyCost      float64 // decoy: обманка (дорого)
	Pings          int     // импульсов (R3: меньше секторов)
	M0Frac         float64 // m0 = M0Frac·L_naive (низ, §14.2)
	BMin, BMax     float64
	CorridorProb   float64
	MaxAttempts    int
}

// DefaultV9Config — базовые значения модели v9.
func DefaultV9Config() V9Config {
	return V9Config{
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
		Pings:          2,
		M0Frac:         0.10,
		BMin:           -0.20,
		BMax:           0.50,
		CorridorProb:   0.6,
		MaxAttempts:    10,
	}
}

// Name — короткое имя конфигурации для отчёта.
func (c V9Config) Name() string {
	c = c.withDefaults()
	return fmt.Sprintf("N=%d k=%d..%d h=%d..%d τ=%.2f русло=%d×%d@%.2f шлюз=%.1f течение=%.1f мост=%.1f/%.1f тупик=%d сектор=%d×%d..%d lure=%.1f trap=%.1f decoy=%.1f pings=%d m0=%.2f",
		c.N, c.KMin, c.KMax, c.HMin, c.HMax, c.TurnCost, c.LaneCount, c.LaneLen, c.LaneCost,
		c.GateToll, c.CurrentAgainst, c.BridgeFirst, c.BridgeSecond, c.DeadEndLen,
		c.SectorCount, c.SectorCellMin, c.SectorCellMax, c.LureCost, c.TrapCost, c.DecoyCost, c.Pings, c.M0Frac)
}

func (c V9Config) withDefaults() V9Config {
	d := DefaultV9Config()
	if c.N < 5 {
		c.N = d.N
	}
	if c.KMin <= 0 {
		c.KMin = d.KMin
	}
	if c.KMax < c.KMin {
		c.KMax = c.KMin
	}
	if c.BeaconDistStep <= 0 {
		c.BeaconDistStep = d.BeaconDistStep
	}
	if c.HBase <= 0 {
		c.HBase = d.HBase
	}
	if c.HMin <= 0 {
		c.HMin = d.HMin
	}
	if c.HMax < c.HMin {
		c.HMax = c.HMin
	}
	if c.HazardDistStep <= 0 {
		c.HazardDistStep = d.HazardDistStep
	}
	if c.UnrestHazards < 0 {
		c.UnrestHazards = d.UnrestHazards
	}
	if len(c.KChoices) == 0 {
		c.KChoices = d.KChoices
	}
	if c.WallCost < 0 {
		c.WallCost = d.WallCost
	}
	if c.LaneCount < 0 {
		c.LaneCount = d.LaneCount
	}
	if c.LaneLen <= 0 {
		c.LaneLen = d.LaneLen
	}
	if c.LaneCost < 0 || c.LaneCost >= 1 {
		c.LaneCost = d.LaneCost
	}
	if c.TurnCost < 0 {
		c.TurnCost = d.TurnCost
	}
	if c.GateToll <= 0 {
		c.GateToll = d.GateToll
	}
	if c.CurrentAgainst <= 1 {
		c.CurrentAgainst = d.CurrentAgainst
	}
	if c.BridgeFirst <= 0 {
		c.BridgeFirst = d.BridgeFirst
	}
	if c.BridgeSecond <= c.BridgeFirst {
		c.BridgeSecond = d.BridgeSecond
	}
	if c.DeadEndLen <= 0 {
		c.DeadEndLen = d.DeadEndLen
	}
	if c.SectorCount <= 0 {
		c.SectorCount = d.SectorCount
	}
	if c.SectorCellMin < 1 {
		c.SectorCellMin = d.SectorCellMin
	}
	if c.SectorCellMax < c.SectorCellMin {
		c.SectorCellMax = c.SectorCellMin
	}
	if c.LureCost < 0 || c.LureCost >= 1 {
		c.LureCost = d.LureCost
	}
	if c.TrapCost <= 1 {
		c.TrapCost = d.TrapCost
	}
	if c.DecoyCost <= 1 {
		c.DecoyCost = d.DecoyCost
	}
	if c.Pings < 0 {
		c.Pings = d.Pings
	}
	if c.M0Frac <= 0 || c.M0Frac > 1 {
		c.M0Frac = d.M0Frac
	}
	if c.BMin >= 0 {
		c.BMin = d.BMin
	}
	if c.BMax <= 0 {
		c.BMax = d.BMax
	}
	if c.CorridorProb <= 0 || c.CorridorProb > 1 {
		c.CorridorProb = d.CorridorProb
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = d.MaxAttempts
	}
	return c
}

// SectorSigV9 — публичная подпись сектора σ (§14.3).
type SectorSigV9 int

const (
	SigQuietV9 SectorSigV9 = iota
	SigMediumV9
	SigNoisyV9
)

func (s SectorSigV9) String() string {
	switch s {
	case SigNoisyV9:
		return "шумная"
	case SigMediumV9:
		return "средняя"
	default:
		return "тихая"
	}
}

// SectorContentV9 — истинное содержимое сектора (§14.3).
type SectorContentV9 int

const (
	ContentEmptyV9 SectorContentV9 = iota
	ContentLureV9
	ContentTrapV9
	ContentDecoyV9
	ContentJackpotV9
	ContentUnstableV9
)

func (c SectorContentV9) String() string {
	switch c {
	case ContentLureV9:
		return "lure"
	case ContentTrapV9:
		return "trap"
	case ContentDecoyV9:
		return "decoy"
	case ContentJackpotV9:
		return "jackpot"
	case ContentUnstableV9:
		return "unstable"
	default:
		return "empty"
	}
}

// SectorSurroundV9 — видимое окружение сектора (§14.3): признак геометрии рядом
// с клетками сектора. Содержимое — шумная функция σ × окружения, поэтому одного
// признака «громкое → зондируй» мало.
type SectorSurroundV9 int

const (
	SurroundPlainV9 SectorSurroundV9 = iota // ничего примечательного
	SurroundGateV9                          // рядом шлюз (toll)
	SurroundDeadEndV9                       // рядом тупиковое русло
	SurroundMudV9                           // рядом топь
	SurroundCurrentV9                       // рядом течение
)

func (s SectorSurroundV9) String() string {
	switch s {
	case SurroundGateV9:
		return "шлюз"
	case SurroundDeadEndV9:
		return "тупик"
	case SurroundMudV9:
		return "топь"
	case SurroundCurrentV9:
		return "течение"
	default:
		return "пусто"
	}
}

// SectorV9 — скрытый сектор: клетки, публичная подпись σ, видимое окружение и
// истинный content.
type SectorV9 struct {
	Cells    []int
	Sig      SectorSigV9
	Surround SectorSurroundV9
	Content  SectorContentV9
}

// V9Field — поле модели v9.
type V9Field struct {
	Seed          int64
	N             int
	Start, Finish int
	Beacons       []int

	Visible  []float64 // публичная цена клетки (без секторов, без механик направления/toll)
	Realized []float64 // истина: секторы по content (jackpot ≈ 0, trap/decoy дорого)

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

	Sectors []SectorV9

	LaneCost, TurnCost, GateToll, CurrentAgainst, BridgeFirst, BridgeSecond float64
	LureCost, TrapCost, DecoyCost                                           float64
	M0Frac                                                                  float64
	BMin, BMax                                                              float64

	// Якоря единой (реализованной) шкалы §14.2.
	LNaive, LSafe, LRisk float64
	VisPath              []int // трасса видимого оптимума (якорь размещения)
	L0Cells              map[int]bool
	Mode                 string // режим задачи §14.6
	Attempt              int
}

// V9DirDI/V9DirDJ — приращения координат по направлению: 0=+i, 1=−i, 2=+j, 3=−j.
var (
	V9DirDI = [4]int{1, -1, 0, 0}
	V9DirDJ = [4]int{0, 0, 1, -1}
)

func V9Abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func V9Sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

func V9ClampInt(v, lo, hi int) int { return min(max(v, lo), hi) }

func (f V9Field) cell(i, j int) int { return j*f.N + i }
func (f V9Field) iOf(c int) int     { return c % f.N }
func (f V9Field) jOf(c int) int     { return c / f.N }

// V9Step — сосед и направление перехода к нему.
type V9Step struct {
	cell int
	dir  int
}

func (f V9Field) neighbors4(c int) []V9Step {
	i, j := f.iOf(c), f.jOf(c)
	out := make([]V9Step, 0, 4)
	if i > 0 {
		out = append(out, V9Step{f.cell(i-1, j), 1})
	}
	if i < f.N-1 {
		out = append(out, V9Step{f.cell(i+1, j), 0})
	}
	if j > 0 {
		out = append(out, V9Step{f.cell(i, j-1), 3})
	}
	if j < f.N-1 {
		out = append(out, V9Step{f.cell(i, j+1), 2})
	}
	return out
}

func (f V9Field) dirOf(a, b int) int {
	di, dj := f.iOf(b)-f.iOf(a), f.jOf(b)-f.jOf(a)
	for d := 0; d < 4; d++ {
		if V9DirDI[d] == di && V9DirDJ[d] == dj {
			return d
		}
	}
	return -1
}

// mixSeedV9 — детерминированная перегенерация без времени.
func mixSeedV9(seed, attempt int64) int64 {
	x := uint64(seed) + 0x9E3779B97F4A7C15*uint64(attempt+1)
	x ^= x >> 30
	x *= 0xBF58476D1CE4E5B9
	x ^= x >> 27
	x *= 0x94D049BB133111EB
	x ^= x >> 31
	return int64(x)
}

// Соли RNG: σ (публичная), содержимое (secret), импульсы, слепая ставка B.
const (
	V9SigSalt    int64 = 0x51617A5EC0DE
	V9SecretSalt int64 = 0x5EC5E7A15EED
	V9PingSalt   int64 = 0x9E3779B1
	V9BlindSalt  int64 = 0xB11D0000
	V9Modesalt   int64 = 0xA11CE000
)

// V9Modes — режимы задачи §14.6 (≥3 с долей ≥15%).
var V9Modes = []string{"русла", "течения", "шлюзы", "тупики/обманки", "топь"}

// edgeCostV9 — стоимость перехода from→to по базовой карте cm с механиками
// направления/toll/моста (первый проход).
func (f V9Field) edgeCostV9(cm []float64, from, to int) float64 {
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

// pathCostV9 — стоимость пути по карте cm: цена клетки, τ за смену направления,
// toll за каждый вход в шлюз.
func (f V9Field) pathCostV9(cm []float64, path []int) float64 {
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

// staircaseV9 — канонический «лестничный» путь (без обхода).
func (f V9Field) staircaseV9(a, b int) []int {
	path := []int{a}
	ci, cj := f.iOf(a), f.jOf(a)
	bi, bj := f.iOf(b), f.jOf(b)
	for ci != bi || cj != bj {
		di, dj := bi-ci, bj-cj
		switch {
		case di == 0:
			cj += V9Sign(dj)
		case dj == 0:
			ci += V9Sign(di)
		case V9Abs(di) >= V9Abs(dj):
			ci += V9Sign(di)
		default:
			cj += V9Sign(dj)
		}
		path = append(path, f.cell(ci, cj))
	}
	return path
}

// naturalOrderV9 — естественный порядок обхода маяков (по i, затем j).
func naturalOrderV9(f V9Field) []int {
	order := append([]int(nil), f.Beacons...)
	sortIntsStable(order, func(a, b int) bool {
		ia, ja := f.iOf(a), f.jOf(a)
		ib, jb := f.iOf(b), f.jOf(b)
		if ia != ib {
			return ia < ib
		}
		return ja < jb
	})
	return order
}

func sortIntsStable(order []int, less func(a, b int) bool) {
	// вставками: k ≤ 6, зато без импорта sort и без лишних аллокаций.
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && less(order[j], order[j-1]); j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
}

type beaconPolicy int
type hazardBias int

const (
	beaconZigzag beaconPolicy = iota
	beaconRing
	beaconCross
	beaconCluster
)

const (
	hazardCorridors hazardBias = iota
	hazardUniform
	hazardCluster
)

func V9PassportProfile(p Passport) (beaconPolicy, hazardBias, float64) {
	policy := beaconZigzag
	switch {
	case p.From.SystemType == "multiple" || p.To.SystemType == "multiple":
		policy = beaconRing
	case p.From.SystemType == "binary" || p.To.SystemType == "binary":
		policy = beaconCross
	}
	bias := hazardCorridors
	if len(p.DestinationBelts) > 0 {
		bias = hazardCluster
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

func beaconCountV9(dist float64, cfg V9Config) int {
	if dist < 0 {
		dist = 0
	}
	k := cfg.KMin + int(dist/cfg.BeaconDistStep)
	return min(max(k, cfg.KMin), cfg.KMax)
}

func hazardCountV9(dist float64, unrest int, cfg V9Config) int {
	if dist < 0 {
		dist = 0
	}
	h := cfg.HBase + int(dist/cfg.HazardDistStep) + cfg.UnrestHazards*unrest
	return min(max(h, cfg.HMin), cfg.HMax)
}
