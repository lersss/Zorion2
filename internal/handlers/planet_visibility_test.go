// internal/handlers/planet_visibility_test.go
//
// Тесты скрытия деталей планет для player (спека 77a §6.2): stripPlanetDetails
// чистит Biomes/Subterrain (находка 2026-09-20 §10.5 — утечка И1 с 99.2.28).
package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

// TestStripDeposits — залежи поверхности (спека 2026-09-22-поселение-... §5.1 и
// спека итерации 3 §5.2/п.26): player без знания их не видит; со знанием —
// только активные (amount > 0), выработанная (amount = 0) скрыта; admin идёт
// мимо stripPlanetDetails и видит обе.
func TestStripDeposits(t *testing.T) {
	p := models.Planet{
		ID: "p1",
		Deposits: []models.SurfaceDeposit{
			{GoodID: 359, GoodName: "Мясо", Stratum: "surface", Wealth: 0.5, Amount: 1000},
			{GoodID: 1, GoodName: "вода-ресурс", Stratum: "surface", Wealth: 0.3, Amount: 0},
		},
	}

	stripped := stripPlanetDetails(p, nil)
	require.Nil(t, stripped.Deposits, "без знания залежи скрыты (§5.1)")

	withKnowledge := stripPlanetDetails(p, &models.PlanetKnowledgeView{})
	require.Len(t, withKnowledge.Deposits, 1, "со знанием остаются только активные залежи")
	require.Equal(t, int64(359), withKnowledge.Deposits[0].GoodID, "выработанная (amount = 0) скрыта")

	// admin/skycomposer фильтр не применяют (И7): планета не проходит
	// stripPlanetDetails — обе залежи остаются как есть.
	require.Len(t, p.Deposits, 2, "админу видны и выработанные залежи")
}

// TestStripFormationHistory — история формирования (спека
// 2026-09-22-облако-этап-2 §6.5): без знания о планете маркер скрыт (защита в
// глубину, как фракции/строения/залежи); со знанием — остаётся (значок в
// карточке виден только известной планеты).
func TestStripFormationHistory(t *testing.T) {
	p := models.Planet{
		ID:               "p1",
		FormationHistory: []models.PlanetFormationEvent{{Type: "migrated"}, {Type: "ice_lost"}},
	}

	stripped := stripPlanetDetails(p, nil)
	require.Nil(t, stripped.FormationHistory, "без знания маркер скрыт (§6.5)")

	withKnowledge := stripPlanetDetails(p, &models.PlanetKnowledgeView{})
	require.Len(t, withKnowledge.FormationHistory, 2, "со знанием маркер остаётся")
}

// ==================== ГЕЙТ ЗНАНИЯ ПОЯСА (спека поясов этап 3 §6.4/§8.4) ====================

// M21: без знания пояса BeltView не отдаёт belt_class/remaining_level; при
// знании (радар ИЛИ присутствие mining) — отдаёт обе шкалы.
func TestBeltKnowledgeGate(t *testing.T) {
	rem := 300.0
	b := models.Belt{
		ID: "b1", Visible: true,
		Composition:   map[string]float64{"rock": 0.6, "iron": 0.3, "ice": 0.1},
		IronRemaining: &rem,
	}

	// Без знания — обе шкалы не отдаются.
	out := applyBeltVisibility([]models.Belt{b}, false, nil)
	require.Len(t, out, 1)
	assert.Nil(t, out[0].Composition)
	assert.Empty(t, out[0].BeltClass)
	assert.Empty(t, out[0].RemainingLevel)

	// Знание по радару — обе шкалы есть.
	out2 := applyBeltVisibility([]models.Belt{b}, true, nil)
	require.Len(t, out2, 1)
	assert.Equal(t, "богатый", out2[0].BeltClass)
	assert.Equal(t, "истощается", out2[0].RemainingLevel, "300/600 = 0.5")

	// Присутствие во время захода (mining) — тоже знание (не только orbit).
	pos := &models.CurrentPosition{Status: "mining", ObjectType: "belt", ObjectID: "b1", Level: "mining"}
	out3 := applyBeltVisibility([]models.Belt{b}, false, pos)
	require.Len(t, out3, 1)
	assert.Equal(t, "богатый", out3[0].BeltClass)
	assert.Equal(t, "истощается", out3[0].RemainingLevel)
}
