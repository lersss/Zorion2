// internal/handlers/planet_visibility_test.go
//
// Тесты скрытия деталей планет для player (спека 77a §6.2): stripPlanetDetails
// чистит Biomes/Subterrain (находка 2026-09-20 §10.5 — утечка И1 с 99.2.28).
package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

func TestStripBiomes(t *testing.T) {
	p := models.Planet{
		ID:                    "p1",
		SurfaceComposition:    map[string]float64{"горы": 40},
		SubterrainComposition: map[string]float64{"пустая_порода": 100},
		Biomes:                []models.Biome{{Form: "горы", Share: 40}},
		Subterrain:            []models.SubterrainZone{{Type: "пустая_порода", Share: 100}},
		Atmosphere:            "азотно-кислородная",
		Core:                  &models.PlanetCore{Type: "железное"},
		Settlements:           []models.Settlement{{ID: "s1"}},
	}
	stripped := stripPlanetDetails(p, nil)

	require.Nil(t, stripped.Biomes, "Biomes скрыты для player (77a И1, находка §10.5)")
	require.Nil(t, stripped.Subterrain, "Subterrain скрыт для player (77a И1, находка §10.5)")
	require.Nil(t, stripped.SurfaceComposition)
	require.Nil(t, stripped.SubterrainComposition)
	require.Equal(t, "", stripped.Atmosphere)
	require.Nil(t, stripped.Core)
	require.Nil(t, stripped.Settlements)
	require.Equal(t, "p1", stripped.ID, "базовые поля объекта остаются (77a §5.2)")
}