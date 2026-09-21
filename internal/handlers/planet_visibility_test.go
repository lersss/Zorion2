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

// TestStripFactionsAndBuildings — фракции/строения планеты (спека
// 2026-09-21-фабрики-релиз-2-столицы-фракций §5/§7 п.6): player без знания о
// планете получает nil (защита в глубину, как у поселений); со знанием —
// остаются; admin идёт мимо фильтра (stripPlanetDetails к нему не применяется).
func TestStripFactionsAndBuildings(t *testing.T) {
	p := models.Planet{
		ID:        "p1",
		Factions:  []models.PlanetFaction{{ID: "f1", Name: "Аквилонский Синдикат"}},
		Buildings: []models.PlanetBuilding{{ID: "b1", BuildingType: "capital", OwnerType: "faction", OwnerID: "f1"}},
	}

	// Player без знания — деталей о фракциях/строениях нет.
	stripped := stripPlanetDetails(p, nil)
	require.Nil(t, stripped.Factions, "без знания фракции скрыты")
	require.Nil(t, stripped.Buildings, "без знания строения скрыты")

	// Player со знанием — фракции/строения остаются.
	withKnowledge := stripPlanetDetails(p, &models.PlanetKnowledgeView{})
	require.Len(t, withKnowledge.Factions, 1)
	require.Len(t, withKnowledge.Buildings, 1)

	// admin/skycomposer фильтр не применяют (И7): планета не проходит
	// stripPlanetDetails — поля остаются как есть.
	require.Len(t, p.Factions, 1)
	require.Len(t, p.Buildings, 1)
}