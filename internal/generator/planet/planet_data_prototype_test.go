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

		// Землеподобная: жизнь, обитаемость, вода, умеренный климат.
		assert.Equal(t, true, data["life"], "на прототипе должна быть жизнь")
		assert.Equal(t, true, data["habitable"], "прототип должен быть обитаем")
		assert.Equal(t, "temperate", data["climate"], "климат — умеренный")

		water, ok := data["water_percent"].(float64)
		require.True(t, ok, "water_percent должен быть числом")
		assert.Greater(t, water, float64(0), "вода обязательна")

		// Население ровно 10 человек.
		assert.Equal(t, float64(10), data["population"], "население прототипа — 10")
	}
}