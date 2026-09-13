package planet

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Прототип землеподобной планеты (для поселения 1 уровня).
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
		assert.Greater(t, water, float64(0), "вода обязательна")

		// Население в data не хранится — оно вычисляется из поселений.
		_, hasPopulation := data["population"]
		assert.False(t, hasPopulation, "population не должен лежать в data планеты")
	}
}