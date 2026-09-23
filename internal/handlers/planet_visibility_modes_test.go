// internal/handlers/planet_visibility_modes_test.go
// Режимы показа планеты для player (спека
// 2026-09-23-орбита-планеты-присутствие-и-снимок §5.1/§5.2/§5.4, тесты T7/T8):
// presence → полные данные; snapshot → замороженная картина без эффектов и
// лога; scan → скан-уровень; none → заглушка. Player-safe DTO эффектов без
// нагрузки/порога/силы; чек-точки населения не утекают.
package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// snapshotData — data.snapshot записи знания (форма §3.1).
func snapshotData(at time.Time) map[string]interface{} {
	return map[string]interface{}{
		"at":                  at.UTC().Format(time.RFC3339),
		"source":              "presence",
		"surface_dominant":    "горы",
		"surface_composition": map[string]interface{}{"горы": 62.0, "пыль": 38.0},
		"settlements": []interface{}{
			map[string]interface{}{
				"id": "s1", "race_id": "spark", "race_name": "Искры",
				"population": 876000000, "stability": 69,
				"branches": []interface{}{
					map[string]interface{}{
						"recipe_id": 73, "recipe_name": "Вода", "complexity": 3,
						"output":   []interface{}{map[string]interface{}{"good_id": 381, "good_name": "Питьевая вода", "amount": 12.5}},
						"produced": 12.5, "eaten": 4.0, "eaten_rate": 0.02,
					},
				},
			},
		},
		"factions":  []interface{}{map[string]interface{}{"id": "f1", "name": "Синдикат", "type": "t", "color": "#fff", "description": "d"}},
		"buildings": []interface{}{map[string]interface{}{"id": "bld1", "building_type": "capital", "owner_type": "faction", "owner_id": "f1"}},
	}
}

// knowledgeWithSnapshot — запись знания со снимком.
func knowledgeWithSnapshot(at time.Time) *models.PlanetKnowledge {
	return &models.PlanetKnowledge{
		UserID: "u1", PlanetID: "p1",
		Data: map[string]interface{}{
			"surface_dominant":    "горы",
			"surface_composition": map[string]interface{}{"горы": 62.0},
			"settlements_count":   float64(1),
			"snapshot":            snapshotData(at),
		},
		ScannedAt: at,
		Source:    "presence",
	}
}

// ==================== T7: РЕЖИМЫ §5.1 ====================

// presence: полные данные — поверхность, поселения (раса/население/
// стабильность/ветки/лог), фракции, строения, активные залежи; недра/
// атмосфера/ядро/описание скрыты (§1.4).
func TestStripPresenceModeFullDetails(t *testing.T) {
	p := models.Planet{
		ID:                    "p1",
		SurfaceDominant:       "горы",
		SurfaceComposition:    map[string]float64{"горы": 62},
		SubterrainComposition: map[string]float64{"порода": 100},
		Atmosphere:            "азотная",
		Core:                  &models.PlanetCore{Type: "железное"},
		Description:           "описание",
		Biomes:                []models.Biome{{Form: "горы", Share: 62}},
		Settlements: []models.Settlement{{
			ID: "s1", RaceID: "spark", RaceName: "Искры", Population: 876000000,
			PopulationExact: 876000000.4, Stability: 69, ComputedAt: time.Now(),
			RPerSec: 0.5, LambdaPerHour: 0.02, NDead: 1,
			Log: []models.SettlementLogEntry{{ID: "l1", Type: "extinct"}},
			Branches: []models.SettlementBranch{{
				ID: "b1", RecipeID: 73, RecipeName: "Вода",
				Output: []models.BranchBufferEntry{{GoodID: 381, GoodName: "Вода", Amount: 12.5}},
				Input:  []models.BranchBufferEntry{{GoodID: 1, GoodName: "лёд", Amount: 5}},
			}},
			Effects: []models.ActiveEffect{{
				EffectTypeID: 1, Name: "Голод", Impact: "population_rate",
				Load: 30, Threshold: 24, Rate: 0.5, W: 1, Enabled: true,
				SourcePosition: strPtr("продовольствие"), Curve: "hunger",
			}},
		}},
		Factions:  []models.PlanetFaction{{ID: "f1", Name: "Синдикат"}},
		Buildings: []models.PlanetBuilding{{ID: "bld1", BuildingType: "capital"}},
		Deposits: []models.SurfaceDeposit{
			{GoodID: 359, Amount: 1000},
			{GoodID: 1, Amount: 0},
		},
	}

	out := stripPlanetDetails(p, &models.PlanetKnowledgeView{Mode: knowledgeModePresence})

	require.Equal(t, "горы", out.SurfaceDominant, "поверхность видна при присутствии")
	require.Len(t, out.Settlements, 1, "поселения видны при присутствии")
	s := out.Settlements[0]
	assert.Equal(t, "spark", s.RaceID)
	assert.Equal(t, 876000000, s.Population)
	assert.Equal(t, 69, s.Stability)
	require.Len(t, s.Branches, 1, "ветки видны")
	assert.Nil(t, s.Branches[0].Input, "вход ветки скрыт (§5.4/§15)")
	require.Len(t, s.Log, 1, "лог вымирания виден при присутствии")
	require.Len(t, out.Factions, 1)
	require.Len(t, out.Buildings, 1)
	require.Len(t, out.Deposits, 1, "только активные залежи")
	assert.Equal(t, int64(359), out.Deposits[0].GoodID)

	// Физика — скрыта и при присутствии (§1.4).
	assert.Nil(t, out.SubterrainComposition)
	assert.Nil(t, out.Biomes)
	assert.Empty(t, out.Atmosphere)
	assert.Nil(t, out.Core)
	assert.Empty(t, out.Description)
}

// snapshot: картина из снимка — поверхность, поселения (без эффектов и лога),
// фракции, строения; население — сумма поселений; mode/snapshot_at/
// snapshot_fresh в знании.
func TestStripSnapshotModeFrozenPicture(t *testing.T) {
	at := time.Now().Add(-time.Hour)
	k := knowledgeWithSnapshot(at)
	view := buildKnowledgeView(k, time.Now())
	require.NotNil(t, view)
	require.NotNil(t, view.SnapshotAt)
	require.True(t, view.SnapshotFresh, "снимок часовой давности — свежий")

	snap, ok := parseSnapshot(k)
	require.True(t, ok)
	view.Mode = knowledgeModeSnapshot
	p := applySnapshotToPlanet(models.Planet{ID: "p1"}, snap)
	out := stripPlanetDetails(p, view)

	assert.Equal(t, "горы", out.SurfaceDominant)
	assert.Equal(t, map[string]float64{"горы": 62.0, "пыль": 38.0}, out.SurfaceComposition)
	require.Len(t, out.Settlements, 1)
	assert.Equal(t, "Искры", out.Settlements[0].RaceName)
	assert.Equal(t, 876000000, out.Settlements[0].Population)
	assert.Equal(t, 69, out.Settlements[0].Stability)
	assert.Nil(t, out.Settlements[0].Effects, "эффектов в снимке нет (§3.1)")
	assert.Nil(t, out.Settlements[0].Log, "лога в снимке нет (§3.1)")
	require.Len(t, out.Settlements[0].Branches, 1, "ветки заморожены")
	assert.Nil(t, out.Settlements[0].Branches[0].Input)
	assert.Equal(t, int64(876000000), out.Population, "население — сумма поселений снимка")
	require.Len(t, out.Factions, 1)
	require.Len(t, out.Buildings, 1)
}

// Граница 7 дней (T10): snapshot_fresh=false ровно при at < now − KnowledgeTTL.
func TestSnapshotFreshBoundary(t *testing.T) {
	now := time.Now()
	fresh := buildKnowledgeView(knowledgeWithSnapshot(now.Add(-6*24*time.Hour)), now)
	require.True(t, fresh.SnapshotFresh, "6 дней — свежий")

	stale := buildKnowledgeView(knowledgeWithSnapshot(now.Add(-8*24*time.Hour)), now)
	require.False(t, stale.SnapshotFresh, "8 дней — устарел")
}

// scan: скан-уровень — поверхность и счётчик в знании, детали поселений скрыты.
func TestStripScanMode(t *testing.T) {
	p := models.Planet{
		ID:              "p1",
		SurfaceDominant: "вода",
		Settlements:     []models.Settlement{{ID: "s1", Population: 100}},
		Factions:        []models.PlanetFaction{{ID: "f1"}},
		Buildings:       []models.PlanetBuilding{{ID: "b1"}},
	}
	view := &models.PlanetKnowledgeView{
		Mode: knowledgeModeScan, SurfaceDominant: "вода", SettlementsCount: 1,
	}
	out := stripPlanetDetails(p, view)

	assert.Empty(t, out.SurfaceDominant, "поверхность — только в knowledge")
	assert.Nil(t, out.Settlements, "детали поселений скрыты")
	assert.Equal(t, 0, int(out.Population))
	require.Len(t, out.Factions, 1, "фракции со знанием остаются (скан-уровень)")
	require.Len(t, out.Buildings, 1)
	require.NotNil(t, out.Knowledge)
	assert.Equal(t, knowledgeModeScan, out.Knowledge.Mode)
}

// none: знания нет — заглушка, knowledge == nil, детали скрыты.
func TestStripNoneMode(t *testing.T) {
	p := models.Planet{
		ID:              "p1",
		SurfaceDominant: "вода",
		Settlements:     []models.Settlement{{ID: "s1"}},
		Factions:        []models.PlanetFaction{{ID: "f1"}},
		Buildings:       []models.PlanetBuilding{{ID: "b1"}},
		Deposits:        []models.SurfaceDeposit{{GoodID: 1, Amount: 10}},
	}
	out := stripPlanetDetails(p, nil)

	assert.Nil(t, out.Knowledge, "нет знания — knowledge nil")
	assert.Empty(t, out.SurfaceDominant)
	assert.Nil(t, out.Settlements)
	assert.Nil(t, out.Factions)
	assert.Nil(t, out.Buildings)
	assert.Nil(t, out.Deposits)
}

// ==================== T8: PLAYER-SAFE DTO ЭФФЕКТОВ (§5.4) ====================

// Эффекты игроку — только name/impact/state; нагрузки/порога/кривой/владельца/
// силы в JSON нет.
func TestPlayerEffectsDTO(t *testing.T) {
	p := models.Planet{
		ID: "p1",
		Settlements: []models.Settlement{{
			ID: "s1", Population: 100,
			Effects: []models.ActiveEffect{{
				EffectTypeID: 1, Name: "Голод", Impact: "population_rate",
				Load: 30, Threshold: 24, Rate: 0.5, W: 1, Enabled: true,
				Curve: "hunger", SourcePosition: strPtr("продовольствие"),
				OwnerType: "settlement", OwnerID: "s1",
			}},
		}},
	}
	out := stripPlanetDetails(p, &models.PlanetKnowledgeView{Mode: knowledgeModePresence})
	require.Len(t, out.Settlements, 1)

	views, ok := out.Settlements[0].Effects.([]models.EffectView)
	require.True(t, ok, "эффекты игрока — player-safe DTO")
	require.Len(t, views, 1)
	assert.Equal(t, "Голод", views[0].Name)
	assert.Equal(t, "population_rate", views[0].Impact)
	assert.Equal(t, "active", views[0].State)

	raw, err := json.Marshal(out.Settlements[0])
	require.NoError(t, err)
	s := string(raw)
	for _, forbidden := range []string{
		`"load"`, `"load_at"`, `"threshold"`, `"curve"`, `"rate"`, `"w"`,
		`"source_position"`, `"owner_type"`, `"owner_id"`, `"effect_type_id"`,
		`"population_exact"`, `"computed_at"`, `"r_per_sec"`, `"lambda_per_hour"`, `"n_dead"`,
	} {
		assert.NotContains(t, s, forbidden, "внутреннее поле не утекает игроку")
	}
	assert.Contains(t, s, `"name":"Голод"`)
	assert.Contains(t, s, `"state":"active"`)
}

// При snapshot/scan/none эффектов нет (nil).
func TestEffectsOnlyInPresence(t *testing.T) {
	base := models.Planet{
		ID: "p1",
		Settlements: []models.Settlement{{
			ID: "s1", Population: 100,
			Effects: []models.ActiveEffect{{EffectTypeID: 1, Name: "Голод", Enabled: true}},
		}},
	}

	snap := stripPlanetDetails(base, &models.PlanetKnowledgeView{Mode: knowledgeModeSnapshot})
	require.Len(t, snap.Settlements, 1)
	assert.Nil(t, snap.Settlements[0].Effects, "в снимке эффектов нет")

	scan := stripPlanetDetails(base, &models.PlanetKnowledgeView{Mode: knowledgeModeScan})
	assert.Nil(t, scan.Settlements, "в скане деталей поселений нет")

	none := stripPlanetDetails(base, nil)
	assert.Nil(t, none.Settlements)
}

// ==================== T8: ЧЕК-ТОЧКИ НЕ УТЕКАЮТ ====================

// JSON поселения присутствия не несёт population_exact/computed_at/R-компонент
// (иначе клиент достроит население по формуле — §3.1/§15).
func TestPresenceSettlementNoCheckpoints(t *testing.T) {
	p := models.Planet{
		ID: "p1",
		Settlements: []models.Settlement{{
			ID: "s1", Population: 876000000, PopulationExact: 876000000.4,
			Stability: 69, ComputedAt: time.Now(), RPerSec: 0.5,
			LambdaPerHour: 0.02, NDead: 1,
			Branches: []models.SettlementBranch{{
				ID: "b1", RecipeID: 73, RecipeName: "Вода",
				Output: []models.BranchBufferEntry{{GoodID: 381, Amount: 12.5}},
			}},
		}},
	}
	out := stripPlanetDetails(p, &models.PlanetKnowledgeView{Mode: knowledgeModePresence})
	raw, err := json.Marshal(out.Settlements[0])
	require.NoError(t, err)
	s := string(raw)
	for _, forbidden := range []string{
		`"population_exact"`, `"computed_at"`, `"r_per_sec"`, `"lambda_per_hour"`,
		`"n_dead"`, `"processed_at"`, `"input"`,
	} {
		assert.NotContains(t, s, forbidden, "чек-точка не утекает игроку")
	}
	assert.Contains(t, s, `"population":876000000`)
	assert.Contains(t, s, `"recipe_name":"Вода"`)
}
