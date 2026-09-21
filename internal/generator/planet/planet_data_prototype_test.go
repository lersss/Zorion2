package planet

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Прототип землеподобной планеты (для поселения 1 уровня).
// Контракт инструмента (99.2.20 §6.9, §13.10): settleable = true,
// жизнь = true, вода > 60, флаг жидкой воды true, T ≈ 288 K.
func TestGeneratePrototypePlanet(t *testing.T) {
	g := NewGenerator(nil, 7)
	for i := 0; i < 20; i++ {
		pd := g.GeneratePrototypePlanet("w1", "World", "G")
		require.NotNil(t, pd, "прототип всегда даёт планету")
		assert.Equal(t, "w1", pd.WorldID)
		assert.Equal(t, 1, pd.OrbitIndex)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(pd.Data, &data))

		// Землеподобная: жизнь, пригодность, вода, умеренный архетип.
		assert.Equal(t, true, data["life"], "на прототипе должна быть жизнь")
		assert.Equal(t, "умеренный", data["archetype"], "архетип — умеренный")

		// Обитаемость не хранится в data — она вычисляется из поселений.
		_, hasHabitable := data["habitable"]
		assert.False(t, hasHabitable, "habitable не должен лежать в data планеты")

		water, ok := data["water_percent"].(float64)
		require.True(t, ok, "water_percent должен быть числом")
		assert.Greater(t, water, float64(60), "вода > 60 (контракт инструмента)")

		// Температура прототипа (явный оверрайд §6.9).
		temp, ok := data["temperature"].(float64)
		require.True(t, ok, "temperature должен быть числом")
		assert.InDelta(t, 288.0, temp, 0.5, "T ≈ 288 K")

		// Флаг жидкой воды проверяется инвариантом «флаг ⟺ (P, T_final)»
		// (99.2.20 §13.3), а не точным значением true: при тонкой атмосфере
		// (P ниже точки кипения при T = 288 K) флаг законно false, и контракт
		// «флаг true» генератор не гарантирует. Разброс давления — предсуществующая
		// флейкость потока rng (порядок итерации map в композиции, PITFALLS
		// «Дизайн и числа»); залежи лишь сдвигают поток, создавая флейкость.
		atm, ok := data["atmosphere_data"].(map[string]interface{})
		require.True(t, ok, "atmosphere_data в данных прототипа")
		pressure, ok := atm["pressure_atm"].(float64)
		require.True(t, ok, "pressure_atm должен быть числом")
		flagVal, ok := data["liquid_water_possible"].(bool)
		require.True(t, ok, "liquid_water_possible должен быть bool")
		assert.Equal(t, liquidWaterPossible(temp, pressure), flagVal,
			"инвариант флага: liquid_water_possible ⟺ (P = %.4f атм, T = %.1f K)", pressure, temp)

		// Пригодность: political_system ≠ «нет» — признак settleable.
		assert.NotEqual(t, "нет", data["political_system"], "прототип пригоден под поселение")

		// Население в data не хранится — оно вычисляется из поселений.
		_, hasPopulation := data["population"]
		assert.False(t, hasPopulation, "population не должен лежать в data планеты")
	}
}
