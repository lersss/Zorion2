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
// len(Bends) = len(Nodes) − 1, |k| ≤ 10.
type ComponentCurve struct {
	Nodes []SegmentNode
	Bends []float64
}

// balancerStore — хранилище кривых по компонентам ("heat" | "cold" |
// "gravity" | "radiation"). Чтение (мини-R на каждом вызове): RLock + копия.
// Запись (админка, редко): Lock + атомарная замена всей кривой.
type balancerStore struct {
	mu     sync.RWMutex
	curves map[string]*ComponentCurve
}

// componentRanges — диапазоны X компонент (спека 99.2.17 §3, валидация §6).
// Единицы: жара и холод — °C (решение создателя 2026-09-15), гравитация — g,
// радиация — rad.
var componentRanges = map[string][2]float64{
	"heat":      {30, 4000},       // °C
	"cold":      {-273.15, 14.85}, // °C (0..288 K)
	"gravity":   {0, 10},          // g
	"radiation": {0, 100},         // rad
}

// balancerCurveStore — синглтон store (имя переменной отличается от типа:
// в Go тип и переменная пакета не могут называться одинаково; спека §5 —
// иллюстрация паттерна).
var balancerCurveStore = &balancerStore{
	curves: map[string]*ComponentCurve{
		"heat":      defaultHeatCurve(),
		"cold":      defaultColdCurve(),
		"gravity":   defaultGravityCurve(),
		"radiation": defaultRadiationCurve(),
	},
}

// ValidComponent — компонента из 4 допустимых (спека §3).
func ValidComponent(component string) bool {
	_, ok := componentRanges[component]
	return ok
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
		return fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation)", component)
	}
	if err := validateCurve(component, curve); err != nil {
		return err
	}
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()
	balancerCurveStore.curves[component] = cloneCurve(&curve)
	return nil
}

// ResetCurve — замена кривой компоненты дефолтом §3. Неизвестная
// компонента — ошибка.
func ResetCurve(component string) error {
	if !ValidComponent(component) {
		return fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation)", component)
	}
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()
	balancerCurveStore.curves[component] = defaultCurve(component)
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
