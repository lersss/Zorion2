package planet

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gravity сохраняется в data для всех типов планет и спутников
// и согласован с массой и размером (g = M / R²).
func TestDataContainsGravity(t *testing.T) {
	g := NewGenerator(nil, 13)
	classes := []string{"O", "B", "A", "F", "G", "K", "M"}

	for i := 0; i < 5000; i++ {
		spec := classes[g.rng.Intn(len(classes))]
		orbit := g.rng.Intn(10)
		age := determineSystemAge(spec, g.rng)
		pd := g.generatePlanet("w", "World", orbit, spec, age)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(pd.Data, &data))

		mass, okM := data["mass"].(float64)
		siz, okS := data["size"].(float64)
		grav, okG := data["gravity"].(float64)
		require.True(t, okM && okS && okG, "планета %s без mass/size/gravity", pd.Name)
		assert.InDelta(t, mass/(siz*siz), grav, 1e-9,
			"планета %s: gravity != mass/size²", pd.Name)
	}
}

// Спутники газовых гигантов тоже получают gravity в свои данные.
func TestSatelliteDataContainsGravity(t *testing.T) {
	g := NewGenerator(nil, 17)
	for i := 0; i < 200; i++ {
		giantTemp := 50 + g.rng.Float64()*100
		sats := g.generateSatellites(3+g.rng.Intn(8), "Star", 10, giantTemp, "M")
		for _, s := range sats {
			m := satelliteToMap(s)
			sm, okM := m["mass"].(float64)
			ss, okS := m["size"].(float64)
			sg, okG := m["gravity"].(float64)
			require.True(t, okM && okS && okG, "спутник %s без gravity", s.Name)
			assert.InDelta(t, sm/(ss*ss), sg, 1e-9, "спутник %s: gravity != m/s²", s.Name)
		}
	}
}