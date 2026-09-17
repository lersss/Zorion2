package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertWindow — проверка окна по оси свойств в DietWindow.
func assertWindow(t *testing.T, dw DietWindow, axis string, lo, hi float64) {
	t.Helper()
	for _, aw := range dw.Windows {
		if aw.Axis == axis {
			assert.InDelta(t, lo, aw.Lo, 1e-9, "окно %s.Lo", axis)
			assert.InDelta(t, hi, aw.Hi, 1e-9, "окно %s.Hi", axis)
			return
		}
	}
	t.Fatalf("окно по оси %s не найдено", axis)
}

// §3.2: окно расы по consumption-оси = шаблон × resilience (дефолт 1.0).
// Люди: ВОД 60 → окно ВОД-шаблона (биос [60,100], токс [0,30], T 250–280/350–400).
func TestDeriveDietHumansWater(t *testing.T) {
	diet := DeriveDiet(map[string]float64{AxisWater: 60}, 50, LayerTemplates())
	require.Len(t, diet.PerAxis, 1)
	dw := diet.PerAxis[0]
	assert.Equal(t, AxisWater, dw.Axis)
	assert.Equal(t, 60.0, dw.Weight)
	require.NotNil(t, dw.TMelt)
	assert.Equal(t, 250.0, dw.TMelt.Lo)
	assert.Equal(t, 280.0, dw.TMelt.Hi)
	require.NotNil(t, dw.TBoil)
	assert.Equal(t, 350.0, dw.TBoil.Lo)
	assert.Equal(t, 400.0, dw.TBoil.Hi)
	assertWindow(t, dw, AxisBiocompatibility, 60, 100)
	assertWindow(t, dw, AxisToxicity, 0, 30)
}

// §3.2 п.2: окно расы по оси свойств = объединение окон шаблонов потребляемых
// осей. CO2 [20,50] ∪ ГРД [60,100] по хим.активности (детерминированный порядок).
func TestDeriveDietUnion(t *testing.T) {
	diet := DeriveDiet(map[string]float64{AxisCO2: 5, AxisRedox: 5}, 50, LayerTemplates())
	intervals := diet.Union[AxisChemicalActivity]
	require.Len(t, intervals, 2, "хим: два непересекающихся интервала")
	assert.Equal(t, 20.0, intervals[0].Lo)
	assert.Equal(t, 50.0, intervals[0].Hi)
	assert.Equal(t, 60.0, intervals[1].Lo)
	assert.Equal(t, 100.0, intervals[1].Hi)
}

// §3.2 п.3: вес = доля диеты, не ширина; resilience — множитель ширины.
func TestResilienceMultiplier(t *testing.T) {
	assert.InDelta(t, 1.0, ResilienceMultiplier(50), 1e-9, "дефолт ×1.0 (человек)")
	assert.InDelta(t, 0.9, ResilienceMultiplier(0), 1e-9)
	assert.InDelta(t, 1.15, ResilienceMultiplier(100), 1e-9)
	assert.InDelta(t, 0.95, ResilienceMultiplier(25), 1e-9)
	assert.InDelta(t, 1.075, ResilienceMultiplier(75), 1e-9)
}

// Resilience сужает/расширяет окно вокруг центра (множитель ширины):
// resilience 0 → ×0.9: [60,100] → центр 80, ширина 36 → [62,98].
func TestDeriveDietResilienceScalesWidth(t *testing.T) {
	diet := DeriveDiet(map[string]float64{AxisRedox: 100}, 0, LayerTemplates())
	require.Len(t, diet.PerAxis, 1)
	assertWindow(t, diet.PerAxis[0], AxisChemicalActivity, 62, 98)
}

// Инвариант 8: окна рас не хранятся — выводятся из шаблонов + consumption
// (чистая функция: одинаковый вход → одинаковый выход, детерминированный).
func TestDeriveDietPure(t *testing.T) {
	consumption := map[string]float64{AxisWater: 60, AxisOrganic: 30, AxisCO2: 5, AxisRedox: 5}
	a := DeriveDiet(consumption, 50, LayerTemplates())
	b := DeriveDiet(consumption, 50, LayerTemplates())
	assert.Equal(t, a, b, "детерминированный вывод окон")
}

// Оси с весом 0 не участвуют в диете.
func TestDeriveDietSkipsZeroWeight(t *testing.T) {
	diet := DeriveDiet(map[string]float64{AxisWater: 0, AxisOrganic: 100}, 50, LayerTemplates())
	require.Len(t, diet.PerAxis, 1)
	assert.Equal(t, AxisOrganic, diet.PerAxis[0].Axis)
}