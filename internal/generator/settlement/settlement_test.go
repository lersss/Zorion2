package settlement

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== ПРИГОДНОСТЬ ====================

func TestDefaultPresetSuitable(t *testing.T) {
	p := DefaultPreset()

	assert.True(t, p.Suitability.suitable(70, 288, "азотно-кислородная", true, false, false))
	// Пригодная без жизни — тоже (жизнь не обязательна по умолчанию).
	assert.True(t, p.Suitability.suitable(70, 288, "азотно-кислородная", false, false, false))

	// Не пригодна: мало воды, холодно, жарко, ядовитая атмосфера.
	assert.False(t, p.Suitability.suitable(5, 288, "азотно-кислородная", true, false, false))
	assert.False(t, p.Suitability.suitable(70, 150, "азотно-кислородная", true, false, false))
	assert.False(t, p.Suitability.suitable(70, 400, "азотно-кислородная", true, false, false))
	assert.False(t, p.Suitability.suitable(70, 288, "ядовитая", true, false, false))

	// Исключения по флагам.
	assert.False(t, p.Suitability.suitable(70, 288, "водородная", true, true, false))
	assert.False(t, p.Suitability.suitable(70, 288, "плотная", true, false, true))
}

func TestOnlyWithLife(t *testing.T) {
	s := Suitability{MinWater: 10, MinTemperature: 200, MaxTemperature: 350, OnlyWithLife: true}
	assert.False(t, s.suitable(70, 288, "азотно-кислородная", false, false, false))
	assert.True(t, s.suitable(70, 288, "азотно-кислородная", true, false, false))
}

func TestDefaultPresetEqualsGenerateHabitableMinusLife(t *testing.T) {
	// Границы прежней формулы generateHabitable.
	p := DefaultPreset()
	assert.True(t, p.Suitability.suitable(10, 200, "кислородная", false, false, false))
	assert.True(t, p.Suitability.suitable(10, 349, "кислородная", false, false, false))
	assert.False(t, p.Suitability.suitable(9.9, 200, "кислородная", false, false, false))
}

// ==================== ПРЕСЕТ / ПЕРЕОПРЕДЕЛЕНИЯ ====================

func TestApplyOverrides(t *testing.T) {
	p, err := ApplyOverrides(map[string]interface{}{
		"минимальная_вода": float64(5),
		"шанс_заселения":   float64(0.5),
	})
	require.NoError(t, err)
	assert.Equal(t, 5.0, p.Suitability.MinWater)
	assert.Equal(t, 0.5, p.Suitability.Chance)
	assert.Equal(t, 200.0, p.Suitability.MinTemperature, "остальные поля не тронуты")
	assert.Equal(t, 350.0, p.Suitability.MaxTemperature)
}

func TestApplyOverridesUnknownKey(t *testing.T) {
	_, err := ApplyOverrides(map[string]interface{}{"чего": float64(1)})
	require.Error(t, err)
}

func TestApplyOverridesBadValue(t *testing.T) {
	_, err := ApplyOverrides(map[string]interface{}{"минимальная_вода": "не число"})
	require.Error(t, err)
}

// ==================== ПОСТРОЕНИЕ ПОСЕЛЕНИЯ ====================

func TestGenerateSettlementPopulationPositive(t *testing.T) {
	g := NewGenerator(nil, 3)
	for i := 0; i < 100; i++ {
		assert.Greater(t, generateSettlementPopulation(g.rng), 0)
	}
}

func TestBuildSettlementLevelByPopulation(t *testing.T) {
	assert.Equal(t, 1, buildSettlement("p1", 10, 50)[2].(int))
	assert.Equal(t, 2, buildSettlement("p1", 5_000_000, 50)[2].(int))
	assert.Equal(t, 3, buildSettlement("p1", 50_000_000, 50)[2].(int))
	assert.GreaterOrEqual(t, buildSettlement("p1", 10, 50)[4].(int), 1000, "ёмкость не меньше минимума")
}