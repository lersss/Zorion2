package planet

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ресурсы генерируются при построении планеты: summary (английские коды
// категорий) попадает в JSON, сами ресурсы — в PlanetData.Resources.
func TestGeneratePlanet_ResourcesAttached(t *testing.T) {
	g := NewGenerator(nil, 11)
	classes := []string{"G", "K", "M", "O", "A"}

	planets := 0
	for i := 0; i < 500; i++ {
		spec := classes[g.rng.Intn(len(classes))]
		orbit := g.rng.Intn(9)
		pd := g.generatePlanet("w", "World", orbit, stellarParamsFromClass(spec, 0, g.rng))
		if pd == nil {
			continue
		}
		planets++

		raw := map[string]interface{}{}
		require.NoError(t, json.Unmarshal(pd.Data, &raw))

		resourcesRaw, ok := raw["resources"].(map[string]interface{})
		require.True(t, ok, "в JSON планеты должен быть ключ resources")

		// Summary непустой и использует английские коды категорий.
		require.NotEmpty(t, resourcesRaw, "summary ресурсов не должен быть пустым")
		for cat := range resourcesRaw {
			assert.Contains(t,
				[]string{"mineral", "organic", "rare", "fuel", "water", "gas"},
				cat, "код категории %q должен быть английским", cat)
		}

		// У каждой планеты с summary есть детальные ресурсы.
		if pd.Resources == nil {
			t.Fatalf("планета %s без детальных ресурсов при наличии summary", pd.Name)
		}
	}
	assert.Greater(t, planets, 0)
}