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

// ==================== МОДЕЛЬ: МАТЧИНГ ПРАВИЛ ====================

func TestModelMatchesNumberRange(t *testing.T) {
	m := &Model{Mode: ModeComplex, Chance: 1,
		Population: Population{Kind: "fixed", Fixed: 100},
		Rules: []FieldRule{
			{Field: "temperature", Min: floatPtr(200), Max: floatPtr(300)},
		}}

	assert.True(t, m.Matches(map[string]interface{}{"temperature": 250.0}))
	assert.True(t, m.Matches(map[string]interface{}{"temperature": 300.0}))
	assert.False(t, m.Matches(map[string]interface{}{"temperature": 199.0}))
	assert.False(t, m.Matches(map[string]interface{}{"temperature": 301.0}))
	assert.False(t, m.Matches(map[string]interface{}{"temperature": "не число"}), "битое значение не проходит")
}

func TestModelMatchesStringInNotIn(t *testing.T) {
	m := &Model{Mode: ModeComplex, Chance: 1,
		Population: Population{Kind: "fixed", Fixed: 100},
		Rules: []FieldRule{
			{Field: "atmosphere", NotIn: []string{"ядовитая"}},
		}}

	assert.True(t, m.Matches(map[string]interface{}{"atmosphere": "азотно-кислородная"}))
	assert.False(t, m.Matches(map[string]interface{}{"atmosphere": "ядовитая"}))
	assert.True(t, m.Matches(map[string]interface{}{"atmosphere": ""}), "пустое значение не ядовитая")
}

func TestModelMatchesBool(t *testing.T) {
	m := &Model{Mode: ModeComplex, Chance: 1,
		Population: Population{Kind: "fixed", Fixed: 100},
		Rules: []FieldRule{
			{Field: "life", Is: boolPtr(true)},
		}}

	assert.True(t, m.Matches(map[string]interface{}{"life": true}))
	assert.False(t, m.Matches(map[string]interface{}{"life": false}))
	assert.False(t, m.Matches(map[string]interface{}{}), "отсутствие ключа — false")
}

func TestModelMatchesIsGasGiantFromSurface(t *testing.T) {
	m := &Model{Mode: ModeComplex, Chance: 1,
		Population: Population{Kind: "fixed", Fixed: 100},
		Rules: []FieldRule{
			{Field: "is_gas_giant", Is: boolPtr(false)},
		}}

	// is_gas_giant отсутствует, но surface_dominant указывает на гиганта.
	assert.False(t, m.Matches(map[string]interface{}{"surface_dominant": "газовый_гигант"}))
	assert.True(t, m.Matches(map[string]interface{}{"surface_dominant": "скалы"}))
}

func TestModelSimpleIgnoresRules(t *testing.T) {
	// В simple-режиме правила не применяются — все планеты подходят
	// (отбор только по шансу).
	m := &Model{Mode: ModeSimple, Chance: 1,
		Population: Population{Kind: "fixed", Fixed: 100},
		Rules: []FieldRule{
			{Field: "temperature", Min: floatPtr(9999)},
		}}
	assert.True(t, m.Matches(map[string]interface{}{"temperature": 100.0}))
}

// ==================== МОДЕЛЬ: НАСЕЛЕНИЕ ====================

func TestPopulationFixed(t *testing.T) {
	g := NewGenerator(nil, 3)
	assert.Equal(t, 42, Population{Kind: "fixed", Fixed: 42}.value(g.rng))
}

func TestPopulationRandomWithinBounds(t *testing.T) {
	g := NewGenerator(nil, 3)
	p := Population{Kind: "random", Min: 100, Max: 1000}
	for i := 0; i < 100; i++ {
		v := p.value(g.rng)
		assert.GreaterOrEqual(t, v, 100)
		assert.LessOrEqual(t, v, 1000)
	}
}

// ==================== МОДЕЛЬ: ВАЛИДАЦИЯ ====================

func TestModelValidate(t *testing.T) {
	valid := []*Model{
		DefaultModel(),
		{Mode: ModeSimple, Chance: 0.5, Population: Population{Kind: "random", Min: 1, Max: 10}},
		{Mode: ModeSimple, Chance: 1, Population: Population{Kind: "fixed", Fixed: 1}},
	}
	for _, m := range valid {
		require.NoError(t, m.Validate(), "%+v должен быть валидным", m)
	}

	invalid := []*Model{
		{Mode: "unknown", Chance: 1, Population: Population{Kind: "fixed", Fixed: 1}},
		{Mode: ModeSimple, Chance: 2, Population: Population{Kind: "fixed", Fixed: 1}},
		{Mode: ModeSimple, Chance: 1, Population: Population{Kind: "fixed", Fixed: 0}},
		{Mode: ModeSimple, Chance: 1, Population: Population{Kind: "random", Min: 10, Max: 1}},
		{Mode: ModeComplex, Chance: 1, Population: Population{Kind: "fixed", Fixed: 1},
			Rules: []FieldRule{{Field: "несуществующее_поле"}}},
		{Mode: ModeComplex, Chance: 1, Population: Population{Kind: "fixed", Fixed: 1},
			Rules: []FieldRule{{Field: "atmosphere", In: []string{"ядовитая"}, NotIn: []string{"плотная"}}}},
		{Mode: ModeComplex, Chance: 1, Population: Population{Kind: "fixed", Fixed: 1},
			Rules: []FieldRule{{Field: "atmosphere", In: []string{"не из списка"}}}},
	}
	for _, m := range invalid {
		require.Error(t, m.Validate(), "%+v должен быть невалидным", m)
	}
}

// ==================== ПОСТРОЕНИЕ ПОСЕЛЕНИЯ ====================

func TestBuildSettlement(t *testing.T) {
	row := buildSettlement("p1", 500000, 60)
	assert.Equal(t, 4, len(row), "id, planet_id, population, stability — тир не хранится")
	assert.Equal(t, "p1", row[1])
	assert.Equal(t, 500000, row[2])
	assert.Equal(t, 60, row[3])
}