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

		// Флаг жидкой воды и температура (явные оверрайды §6.9).
		assert.Equal(t, true, data["liquid_water_possible"], "флаг жидкой воды true")
		temp, ok := data["temperature"].(float64)
		require.True(t, ok, "temperature должен быть числом")
		assert.InDelta(t, 288.0, temp, 0.5, "T ≈ 288 K")

		// Пригодность: political_system ≠ «нет» — признак settleable.
		assert.NotEqual(t, "нет", data["political_system"], "прототип пригоден под поселение")

		// Население в data не хранится — оно вычисляется из поселений.
		_, hasPopulation := data["population"]
		assert.False(t, hasPopulation, "population не должен лежать в data планеты")
	}
}