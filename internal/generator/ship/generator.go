// internal/generator/ship/generator.go
// Генератор форм кораблей (спека 99.2.15 §3.3): GeneratePart(category, seed)
// эмитит параметрический SVG-фрагмент детали. RNG — локальный rand.New от
// seed (правило AGENTS.md §0); детерминизм: тот же seed → тот же фрагмент
// (включая акценты — они в параметрах генерации, инвариант И3).
package ship

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	"zorion/internal/names"
)

// Part — результат генерации детали: геометрия (для проверок и тестов) +
// svg-фрагмент + id/имя.
type Part struct {
	ID       string
	Category string
	Name     string
	Params   map[string]interface{}
	SVG      string
	Geometry Geometry
}

// Geometry — основная масса (currentColor) + акценты (явный fill).
type Geometry struct {
	Components []Component
	Accents    []Accent
}

// Component — полигон основной массы детали (fill="currentColor").
// Base — прямая базовая кромка (стык с корпусом, стиль §3.2 п.5);
// nil у корпуса (он — якорь).
type Component struct {
	Poly Polygon
	Base *Segment
}

// Accent — фиксированный акцентный элемент (кабина, огни, свечение...).
// Kind: "circle" | "rect".
type Accent struct {
	Kind   string
	Color  string
	Center Pt
	Radius float64
	Rect   Rect
}

// maxShapeAttempts — попыток сгенерировать валидную форму (пере-ролл при
// нарушении инвариантов checkPart). Детерминизм не страдает: пере-роллы
// потребляют локальный rng предсказуемо.
const maxShapeAttempts = 40

// GeneratePart — деталь категории от seed (спека §3.3). id = {category}_{slug},
// name — из словаря имён категории (internal/names). Ошибка — только при
// незагруженном конфиге или исчерпании попыток (на практике не бывает:
// диапазоны и валидация согласованы, спека §11 Этап 2).
func GeneratePart(category string, seed int64) (Part, error) {
	cfg, err := defaultConfig()
	if err != nil {
		return Part{}, err
	}
	if !categoryValid(category) {
		return Part{}, fmt.Errorf("ship: неизвестная категория %q", category)
	}
	rng := rand.New(rand.NewSource(seed))

	gen, ok := generators[category]
	if !ok {
		return Part{}, fmt.Errorf("ship: нет шаблона для категории %q", category)
	}
	geometry, params, err := retryShape(cfg, category, rng, gen)
	if err != nil {
		return Part{}, err
	}
	id := partID(category, params)
	name := names.GenerateShipPartName(category, rng)
	return Part{
		ID:       id,
		Category: category,
		Name:     name,
		Params:   params,
		SVG:      geometry.SVG(cfg),
		Geometry: geometry,
	}, nil
}

// generators — шаблоны генерации по категориям (спека §3.3: контур/полигон
// с параметрами; точные формы — зона разработчика).
var generators = map[string]func(*Config, *rand.Rand) (Geometry, map[string]interface{}){
	"hull":   genHull,
	"nose":   genNose,
	"wings":  genWings,
	"engine": genEngine,
	"tail":   genTail,
}

// retryShape — генерирует форму и проверяет её checkPart (И6/И9);
// невалидная — пере-ролл с новыми случайными параметрами.
func retryShape(cfg *Config, cat string, rng *rand.Rand,
	gen func(*Config, *rand.Rand) (Geometry, map[string]interface{})) (Geometry, map[string]interface{}, error) {
	var lastErr error
	for i := 0; i < maxShapeAttempts; i++ {
		g, params := gen(cfg, rng)
		if err := checkPart(cfg, cat, g); err == nil {
			return g, params, nil
		} else {
			lastErr = err
		}
	}
	return Geometry{}, nil, fmt.Errorf("ship: %s: валидная форма не получена за %d попыток: %v",
		cat, maxShapeAttempts, lastErr)
}

// partID — id детали: {category}_{k1=v1_k2=v2...} от параметров (детерминизм:
// те же параметры → тот же id; INSERT ... ON CONFLICT DO UPDATE при
// перегенерации, спека §2/§6).
func partID(category string, params map[string]interface{}) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(category)
	for _, k := range keys {
		sb.WriteString("_" + k + "=" + formatParam(params[k]))
	}
	return sb.String()
}

// formatParam — значение параметра как строка (float64 и int — без хвостов).
func formatParam(v interface{}) string {
	switch n := v.(type) {
	case float64:
		return num(n)
	case int:
		return strconv.Itoa(n)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ==================== ОБЩИЕ ХЕЛПЕРЫ ====================

// randInRange — случайное значение в диапазоне (включительно).
func randInRange(rng *rand.Rand, r Range) float64 {
	if r.Max <= r.Min {
		return r.Min
	}
	return r.Min + rng.Float64()*(r.Max-r.Min)
}

// randInt — случайное целое в [lo, hi].
func randInt(rng *rand.Rand, lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + rng.Intn(hi-lo+1)
}

// clamp — ограничение значения диапазоном.
func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

// mirrorY — зеркало полигона относительно оси y=100 (нижняя плоскость
// крыла / зеркальная пара, спека §3.1).
func mirrorY(p Polygon) Polygon {
	out := make(Polygon, len(p))
	for i, pt := range p {
		out[i] = Pt{X: pt.X, Y: 200 - pt.Y}
	}
	return out
}

// ==================== КОРПУС ====================

// genHull — корпус (спека §3.1): зона x 45–155, y 70–130, ширина 60–110
// (эффективно ≥ 65: min x ≤ 65 и max x ≥ 130), высота 42–60, центрирован на
// y=100; полная высота до x ≥ 130 — гарантированная область [65,130]×[79,121]
// лежит в полновысотной части при любой форме. Акценты — иллюминаторы 0–3.
func genHull(cfg *Config, rng *rand.Rand) (Geometry, map[string]interface{}) {
	st := cfg.Style
	cc := cfg.Categories["hull"]
	// Эффективная ширина ≥ 65: иначе min x ≤ 65 и max x ≥ 130 несовместимы
	// с зоной [45,155] (спека §3.1, «почему эти числа»).
	w := math.Round(randInRange(rng, Range{Min: math.Max(cc.Width.Min, 65), Max: cc.Width.Max}))
	h := math.Round(randInRange(rng, *cc.Height))
	minX := math.Round(randInRange(rng, Range{Min: math.Max(45, 130-w), Max: math.Min(65, 155-w)}))
	maxX := minX + w

	// Скос кормы («клин»): 0 — вертикальная корма, >0 — задний верх смещён
	// вперёд (к носу). Ограничен так, чтобы гарантированная область оставалась
	// внутри корпуса: rTop ≤ 65 - minX (при y=79 корма не правее 65).
	rTop := 0.0
	if rng.Float64() < 0.5 {
		rTop = math.Round(randInRange(rng, Range{Min: 4, Max: math.Min(18, 65-minX)}))
	}

	// Начало сужения носа: не раньше 130 — корпус полной высоты покрывает
	// гарантированную область (спека §3.1).
	noseStart := math.Round(randInRange(rng, Range{Min: 130, Max: maxX}))
	top := 100 - h/2
	bottom := 100 + h/2

	poly := Polygon{
		{X: minX + rTop, Y: top},
		{X: noseStart, Y: top},
		{X: maxX, Y: 100},
		{X: noseStart, Y: bottom},
		{X: minX, Y: bottom},
	}
	// Фаски острых углов (нос-остриё, задние углы) — без «игл» (стиль п.1–3);
	// у корпуса базы нет (он — якорь), Segment{} не совпадает ни с одной точкой.
	poly = chamferSharp(poly, st.CornerRadius, 100*math.Pi/180, st.SegmentsMax, Segment{})
	poly = poly.Round()

	// Акценты: иллюминаторы 0–3 (стекло) на корпусной части (спека §3.3/§9).
	accents := genPortholes(cfg, rng, poly, noseStart, minX)

	params := map[string]interface{}{
		"w": w, "h": h, "mn": minX, "rt": rTop, "ns": noseStart, "t": 0,
	}
	for i, a := range accents {
		if a.Kind == "circle" {
			params["pw"+num(float64(i))] = a.Center.X
			params["py"+num(float64(i))] = a.Center.Y
			params["pr"+num(float64(i))] = a.Radius
		}
	}
	params["pc"] = float64(len(accents))

	return Geometry{Components: []Component{{Poly: poly}}, Accents: accents}, params
}

// genPortholes — 0–3 иллюминатора (стекло) на корпусной части корпуса
// (спека §3.3: корпус — иллюминаторы 0–3 случайно). Позиции — с шагом по x,
// без наложений; отступы проверяет checkPart (пере-ролл при промахе).
func genPortholes(cfg *Config, rng *rand.Rand, hull Polygon, noseStart, minX float64) []Accent {
	count := randInt(rng, 0, 3)
	if count == 0 {
		return nil
	}
	glass := cfg.Accents.Glass
	r := randInRange(rng, Range{Min: 2, Max: 3.5})
	_, y0, _, y1 := hull.BBox()
	accents := make([]Accent, 0, count)
	step := (noseStart - minX - 24) / float64(count)
	for i := 0; i < count; i++ {
		cx := minX + 12 + step*float64(i) + randInRange(rng, Range{Min: -2, Max: 2})
		cy := randInRange(rng, Range{Min: y0 + 10, Max: y1 - 10})
		accents = append(accents, Accent{
			Kind:   "circle",
			Color:  glass,
			Center: Pt{X: math.Round(cx), Y: math.Round(cy)},
			Radius: math.Round(r*10) / 10,
		})
	}
	return accents
}