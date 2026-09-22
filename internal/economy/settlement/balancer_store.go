// internal/economy/settlement/balancer_store.go
// In-memory store сегментных кривых компонент изменения населения (спека
// 99.2.17 §5/§6): RWMutex, атомарная замена кривой, дефолты при рестарте.
// Мини-R читают store на КАЖДОМ вызове (change_components.go) — сохранение
// из админки применяется к следующему расчёту без рестарта. Не БД (решение
// создателя 2026-09-15). Прецедент — settings.go / internal/npc/settings.go.
package settlement

import (
	"fmt"
	"sync"
)

// SegmentNode — узел кривой: X (°C / K / g / rad, строго возрастает),
// Y — значение R ∈ [0, 0.999). JSON-теги — формат API (спека 99.2.17 §6).
type SegmentNode struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ComponentCurve — сегментная кривая компоненты: узлы + изгибы сегментов.
// len(Bends) = len(Nodes) − 1, |k| ≤ 10. JSON-теги — формат файла расовых
// кривых (99.2.23 §3.1: {"nodes": [...], "bends": [...]}).
type ComponentCurve struct {
	Nodes []SegmentNode `json:"nodes"`
	Bends []float64     `json:"bends"`
}

// HungerCurveKey — ключ кривой компоненты-эффекта «голод» (ось X — нагрузка,
// сило-часы; §7.1). На этапе 2 компонента живёт в store для расчёта R(load);
// полноценный UI/пресеты/валидация — этап 3.
const HungerCurveKey = "hunger"

// balancerStore — хранилище кривых по компонентам ("heat" | "cold" |
// "gravity" | "radiation" + эффект-компонента "hunger"). Чтение (мини-R на
// каждом вызове): RLock + копия. Запись (админка, редко): Lock + атомарная
// замена всей кривой. scalars — скаляры эффект-компонент (recovery, §7.1):
// не внутри ComponentCurve (иначе поле протекло бы в файл расовых кривых).
type balancerStore struct {
	mu      sync.RWMutex
	curves  map[string]*ComponentCurve
	scalars map[string]float64
}

// effectComponents — компоненты-эффекты: только они несут скаляр (`recovery`)
// и обязаны иметь нулевой префикс кривой (порог, §4.4/§7.1).
var effectComponents = map[string]bool{HungerCurveKey: true}

// componentRanges — диапазоны X компонент (спека 99.2.17 §3, валидация §6).
// Единицы: жара и холод — °C (решение создателя 2026-09-15), гравитация — g,
// радиация — rad.
var componentRanges = map[string][2]float64{
	"heat":         {30, 4000},       // °C
	"cold":         {-273.15, 14.85}, // °C (0..288 K)
	"gravity":      {0, 10},          // g
	"radiation":    {0, 100},         // rad
	HungerCurveKey: {0, 8760},        // нагрузка, сило-часы (§7.1: год при w=1)
}

// balancerCurveStore — синглтон store (имя переменной отличается от типа:
// в Go тип и переменная пакета не могут называться одинаково; спека §5 —
// иллюстрация паттерна).
var balancerCurveStore = &balancerStore{
	curves: map[string]*ComponentCurve{
		"heat":         defaultHeatCurve(),
		"cold":         defaultColdCurve(),
		"gravity":      defaultGravityCurve(),
		"radiation":    defaultRadiationCurve(),
		HungerCurveKey: defaultHungerCurve(),
	},
	scalars: map[string]float64{
		HungerCurveKey: defaultComponentScalar(HungerCurveKey),
	},
}

// ValidComponent — компонента из набора админ-ручек/пресетов/глобальных
// кривых: 4 среды (heat/cold/gravity/radiation) + эффект-компонента `hunger`
// (этап 3, §7.1/§7.2). Расовая допустимость — отдельно, см. IsRaceComponent.
func ValidComponent(component string) bool {
	_, ok := componentRanges[component]
	return ok
}

// IsRaceComponent — компонента допустима для РАСОВОГО балансировщика (только
// heat/cold/gravity/radiation, §7.2). `hunger` — глобальный эффект (норма еды
// типа поселения, не расы): расовый store/UI его НЕ принимает (иначе
// Active.Curves["hunger"] == nil → nil-кривая → 500). Гейты расовых
// эндпоинтов и SetRaceCurve — по этой функции (защита в глубину).
// Источник правды — `raceComponents` (race_balancer.go): список не дублируется,
// иначе расовый набор расчерпывался бы от него (мелочь ревью этапа 3).
func IsRaceComponent(component string) bool {
	for _, c := range raceComponents {
		if c == component {
			return true
		}
	}
	return false
}

// defaultComponentScalar — заводское значение скаляра компоненты (§7.1):
// `hunger` → 0.25 сило-ч/ч (решение создателя 2026-09-23, В2 @balancetester).
// Не-эффект-компоненты скаляра не несут → 0.
func defaultComponentScalar(component string) float64 {
	if component == HungerCurveKey {
		return 0.25
	}
	return 0
}

// ComponentScalar — скаляр компоненты (`recovery`, сило-ч/ч, §7.1). ok=false
// для не-эффект-компонент (скаляр у них отсутствует). Чтение под RLock.
func ComponentScalar(component string) (float64, bool) {
	if !effectComponents[component] {
		return 0, false
	}
	balancerCurveStore.mu.RLock()
	defer balancerCurveStore.mu.RUnlock()
	v, ok := balancerCurveStore.scalars[component]
	return v, ok
}

// SetComponentScalar — задать скаляр компоненты (валидация ≥ 0, §7.1).
// Не-эффект-компонента → ошибка (422 на уровне хендлера). Значение — in-memory
// (заводское при рестарте, переносится пресетами — этап 3).
func SetComponentScalar(component string, value float64) error {
	if !effectComponents[component] {
		return fmt.Errorf("компонента %q не несёт скаляр (только эффект-компоненты)", component)
	}
	if value < 0 {
		return fmt.Errorf("скаляр компоненты %q должен быть ≥ 0, получили %v", component, value)
	}
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()
	balancerCurveStore.scalars[component] = value
	return nil
}

// ResetComponentScalar — вернуть заводское значение скаляра (§7.1).
func ResetComponentScalar(component string) error {
	if !effectComponents[component] {
		return fmt.Errorf("компонента %q не несёт скаляр (только эффект-компоненты)", component)
	}
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()
	balancerCurveStore.scalars[component] = defaultComponentScalar(component)
	return nil
}

// GetCurve — копия узлов и изгибов кривой компоненты. ok = false, если
// компонента неизвестна. RLock на время чтения, слайсы копируются — caller
// может мутировать копии, store не трогается.
func GetCurve(component string) ([]SegmentNode, []float64, bool) {
	balancerCurveStore.mu.RLock()
	defer balancerCurveStore.mu.RUnlock()
	c, ok := balancerCurveStore.curves[component]
	if !ok {
		return nil, nil, false
	}
	nodes := make([]SegmentNode, len(c.Nodes))
	copy(nodes, c.Nodes)
	bends := make([]float64, len(c.Bends))
	copy(bends, c.Bends)
	return nodes, bends, true
}

// getCurveRef — ссылка на хранимую кривую БЕЗ копирования (внутренний
// хот-пат, change_components.go): RLock на время чтения, caller НЕ должен
// мутировать слайсы (только evaluateCurve). SetCurve/ResetCurve заменяют
// указатель атомарно под Lock, старые слайсы не мутируются — читать их
// после отпускания RLock безопасно (неизменная кривая, concurrent reads).
// Публичный GetCurve (копия для внешних caller'ов) не тронут.
func getCurveRef(component string) (*ComponentCurve, bool) {
	balancerCurveStore.mu.RLock()
	defer balancerCurveStore.mu.RUnlock()
	c, ok := balancerCurveStore.curves[component]
	return c, ok
}

// SetCurve — атомарная замена кривой компоненты с валидацией §6:
// 1) узлов ∈ [3, 16], изгибов = узлов − 1; 2) X строго возрастают;
// 3) y ∈ [0, 0.999); 4) |k| ≤ 10; 5) X в диапазоне компоненты.
// При ошибке текущее значение не меняется. Хранится копия (caller может
// мутировать свою кривую после сохранения).
func SetCurve(component string, curve ComponentCurve) error {
	if !ValidComponent(component) {
		return fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation/hunger)", component)
	}
	if err := validateCurve(component, curve); err != nil {
		return err
	}
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()
	balancerCurveStore.curves[component] = cloneCurve(&curve)
	return nil
}

// SetCurveWithScalar — атомарная замена кривой компоненты И её скаляра
// (`recovery`, §7.1): PUT «сохранил = применил» пишет оба под одним Lock.
// scalar == nil — скаляр не трогается (не-эффект-компоненты); scalar != nil
// принимается ТОЛЬКО эффект-компонентой (иначе ошибка → 422) и должен быть
// ≥ 0. При ошибке валидации не меняется ни кривая, ни скаляр.
func SetCurveWithScalar(component string, curve ComponentCurve, scalar *float64) error {
	if !ValidComponent(component) {
		return fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation/hunger)", component)
	}
	if scalar != nil {
		if !effectComponents[component] {
			return fmt.Errorf("компонента %q не несёт скаляр recovery (только эффект-компоненты)", component)
		}
		if *scalar < 0 {
			return fmt.Errorf("recovery компоненты %q должен быть ≥ 0, получили %v", component, *scalar)
		}
	}
	if err := validateCurve(component, curve); err != nil {
		return err
	}
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()
	balancerCurveStore.curves[component] = cloneCurve(&curve)
	if scalar != nil {
		balancerCurveStore.scalars[component] = *scalar
	}
	return nil
}

// ResetCurve — замена кривой компоненты дефолтом §3. Неизвестная
// компонента — ошибка.
func ResetCurve(component string) error {
	if !ValidComponent(component) {
		return fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation/hunger)", component)
	}
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()
	balancerCurveStore.curves[component] = defaultCurve(component)
	return nil
}

// ResetCurveWithScalar — сброс кривой компоненты к заводскому дефолту И
// (для эффект-компонент) её скаляра `recovery` к заводскому значению под
// одним Lock (§7.1: `…/curve/reset` сбрасывает и кривую, и скаляр).
func ResetCurveWithScalar(component string) error {
	if !ValidComponent(component) {
		return fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation/hunger)", component)
	}
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()
	balancerCurveStore.curves[component] = defaultCurve(component)
	if effectComponents[component] {
		balancerCurveStore.scalars[component] = defaultComponentScalar(component)
	}
	return nil
}

// SampleCurve — оцифровка кривой компоненты в точках xs (спека 99.2.17 §6,
// POST /admin/balancer/curve/sample): единый источник математики — сервер
// считает evaluateCurve, клиент математику не дублирует. RLock на всё время
// оцифровки (до 500 точек, копейки). Точки вне диапазона компоненты —
// экстраполяцией (горизонтальной, §2) — это и есть цель sample.
func SampleCurve(component string, xs []float64) ([]float64, bool) {
	nodes, bends, ok := GetCurve(component)
	if !ok {
		return nil, false
	}
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = evaluateCurve(nodes, bends, x)
	}
	return out, true
}

// validateCurve — проверки §6 (без блокировок; вызывается из SetCurve).
func validateCurve(component string, curve ComponentCurve) error {
	n := len(curve.Nodes)
	if n < 3 || n > 16 {
		return fmt.Errorf("число узлов должно быть в диапазоне 3..16, получили %d", n)
	}
	if len(curve.Bends) != n-1 {
		return fmt.Errorf("число изгибов должно быть равно числу узлов − 1 (%d), получили %d", n-1, len(curve.Bends))
	}
	rnge := componentRanges[component]
	for i, node := range curve.Nodes {
		if i > 0 && !(node.X > curve.Nodes[i-1].X) {
			return fmt.Errorf("X узлов должны строго возрастать: узел %d (x=%v) не больше предыдущего (x=%v)", i, node.X, curve.Nodes[i-1].X)
		}
		if node.Y < 0 || node.Y >= 0.999 {
			return fmt.Errorf("Y узла %d должен быть в диапазоне [0, 0.999), получили %v", i, node.Y)
		}
		if node.X < rnge[0] || node.X > rnge[1] {
			return fmt.Errorf("X узла %d (%v) вне диапазона компоненты %q [%v, %v]", i, node.X, component, rnge[0], rnge[1])
		}
	}
	for i, k := range curve.Bends {
		if k < -10 || k > 10 {
			return fmt.Errorf("изгиб сегмента %d должен быть в диапазоне [-10, +10], получили %v", i, k)
		}
	}
	// Порог включения — нулевой префикс кривой (§4.4/§7.1): обязателен ТОЛЬКО
	// для эффект-компонент (`hunger`). cold/gravity/radiation первый узел
	// ненулевой — требование к ним ломало бы их валидацию и пресеты.
	if effectComponents[component] && curve.Nodes[0].Y != 0 {
		return fmt.Errorf("для эффект-компоненты %q первый узел кривой должен иметь Y = 0 (порог, нулевой префикс), получили %v", component, curve.Nodes[0].Y)
	}
	return nil
}

// defaultCurve — свежий дефолт компоненты (новые слайсы, без алиасинга).
func defaultCurve(component string) *ComponentCurve {
	switch component {
	case "heat":
		return defaultHeatCurve()
	case "cold":
		return defaultColdCurve()
	case "gravity":
		return defaultGravityCurve()
	case "radiation":
		return defaultRadiationCurve()
	case HungerCurveKey:
		return defaultHungerCurve()
	}
	return nil
}

// cloneCurve — глубокая копия кривой.
func cloneCurve(c *ComponentCurve) *ComponentCurve {
	nodes := make([]SegmentNode, len(c.Nodes))
	copy(nodes, c.Nodes)
	bends := make([]float64, len(c.Bends))
	copy(bends, c.Bends)
	return &ComponentCurve{Nodes: nodes, Bends: bends}
}
