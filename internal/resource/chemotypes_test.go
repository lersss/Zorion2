package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 13 шаблонов хемотипов — по одному на consumption-ось (спека 94a §3.1).
func TestTemplatesCount(t *testing.T) {
	require.Len(t, LayerTemplates(), 13, "шаблонов хемотипов")
	for _, axis := range consumptionAxes {
		require.Contains(t, LayerTemplates(), axis, "шаблон оси %s", axis)
	}
}

// ГРД/ПЫЛ/ИЗЛ — без T-окон (пища в любой фазе, §3.1 примечания).
func TestTemplatesNoTWindows(t *testing.T) {
	for _, axis := range []string{AxisRedox, AxisDust, AxisRadiation} {
		tpl := LayerTemplates()[axis]
		require.NotNil(t, tpl, "шаблон %s", axis)
		assert.Nil(t, tpl.TMelt, "%s: нет окна T_melt", axis)
		assert.Nil(t, tpl.TBoil, "%s: нет окна T_boil", axis)
		assert.Equal(t, PhaseAny, tpl.Phase, "%s: любая фаза", axis)
	}
}

// Остальные 10 — с T-окнами, окнами по осям свойств и категориями.
func TestTemplatesWithTWindows(t *testing.T) {
	for axis, tpl := range LayerTemplates() {
		if axis == AxisRedox || axis == AxisDust || axis == AxisRadiation {
			continue
		}
		require.NotNil(t, tpl.TMelt, "%s: окно T_melt", axis)
		require.NotNil(t, tpl.TBoil, "%s: окно T_boil", axis)
		assert.NotEmpty(t, tpl.Windows, "%s: окна по осям свойств", axis)
		assert.NotEmpty(t, tpl.Categories, "%s: категории", axis)
	}
}

// CO2 — сублимирующий шаблон (T_boil-окно ниже T_melt-окна, §3.1), твёрдая
// фаза; СКФ — сверхкритическая фаза.
func TestTemplatesSpecialPhases(t *testing.T) {
	co2 := LayerTemplates()[AxisCO2]
	require.NotNil(t, co2)
	assert.True(t, co2.Sublimating, "CO2: сублимирующий шаблон")
	require.NotNil(t, co2.TMelt)
	require.NotNil(t, co2.TBoil)
	assert.Equal(t, 190.0, co2.TMelt.Lo)
	assert.Equal(t, 200.0, co2.TMelt.Hi)
	assert.Equal(t, 185.0, co2.TBoil.Lo)
	assert.Equal(t, 195.0, co2.TBoil.Hi)
	assert.Less(t, co2.TBoil.Lo, co2.TMelt.Lo, "CO2: T_boil-окно начинается ниже T_melt-окна (сублимация)")
	assert.Equal(t, PhaseSolid, co2.Phase, "CO2: твёрдая фаза (лёд)")

	scf := LayerTemplates()[AxisSupercritical]
	require.NotNil(t, scf)
	assert.Equal(t, PhaseSupercritical, scf.Phase, "СКФ: сверхкритическая фаза")
}

// Конвенция §3.1: T-окна шаблонов не соприкасаются вплотную.
func TestCheckTemplateRanges(t *testing.T) {
	assert.Empty(t, CheckTemplateRanges(LayerTemplates()))
}