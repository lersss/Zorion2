// Тест видимости владельца/типа структуры (спека
// 2026-09-24-постройка-структур-на-планете §10.1/§10.4, Г2/T16): новые поля
// (settlements[].owner_name, buildings[].type_name/owner_name) уходят ровно
// тогда, когда уходят сами блоки — видны в presence/snapshot, не отдаются в
// scan/none.
package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ownerVisibilityPlanet — планета с поселением (владелец) и строением (тип).
func ownerVisibilityPlanet() models.Planet {
	return models.Planet{
		ID: "p1",
		Settlements: []models.Settlement{{
			ID: "s1", Population: 1000, OwnerType: "faction", OwnerID: "f1", OwnerName: "Люди",
		}},
		Factions: []models.PlanetFaction{{ID: "f1", Name: "Люди"}},
		Buildings: []models.PlanetBuilding{{
			ID: "b1", BuildingType: "producer", OwnerType: "faction", OwnerID: "f1",
			ProducerTypeID: int64Ptr(152), TypeName: "Аутпост", OwnerName: "Люди",
		}},
	}
}

func int64Ptr(v int64) *int64 { return &v }

// T16: none → блоки пусты (владелец/тип не текут).
func TestOwnerVisibilityNone(t *testing.T) {
	p := stripPlanetDetails(ownerVisibilityPlanet(), nil, true)
	assert.Empty(t, p.Settlements)
	assert.Empty(t, p.Buildings)
	assert.Empty(t, p.Factions)
}

// T16: scan → поселений нет; строения остаются с type_name/owner_name
// (канон 2026-09-23 §2.2 — не менять).
func TestOwnerVisibilityScan(t *testing.T) {
	view := &models.PlanetKnowledgeView{Mode: knowledgeModeScan}
	p := stripPlanetDetails(ownerVisibilityPlanet(), view, true)
	assert.Empty(t, p.Settlements, "scan: поселений нет — владелец поселения не течёт")
	require.Len(t, p.Buildings, 1)
	assert.Equal(t, "Аутпост", p.Buildings[0].TypeName)
	assert.Equal(t, "Люди", p.Buildings[0].OwnerName)
}

// T16: presence → владелец поселения виден игроку (Г2), строения тоже.
func TestOwnerVisibilityPresence(t *testing.T) {
	view := &models.PlanetKnowledgeView{Mode: knowledgeModePresence}
	p := stripPlanetDetails(ownerVisibilityPlanet(), view, true)
	require.Len(t, p.Settlements, 1)
	assert.Equal(t, "faction", p.Settlements[0].OwnerType)
	assert.Equal(t, "Люди", p.Settlements[0].OwnerName, "владелец виден при присутствии (Г2)")
	require.Len(t, p.Buildings, 1)
	assert.Equal(t, "Аутпост", p.Buildings[0].TypeName)
	assert.Equal(t, "Люди", p.Buildings[0].OwnerName)
}

// T16/T20: snapshot → владелец поселения из снимка (заморожен), не чистится.
func TestOwnerVisibilitySnapshot(t *testing.T) {
	view := &models.PlanetKnowledgeView{Mode: knowledgeModeSnapshot}
	p := stripPlanetDetails(ownerVisibilityPlanet(), view, true)
	require.Len(t, p.Settlements, 1)
	assert.Equal(t, "Люди", p.Settlements[0].OwnerName, "владелец снимка заморожен (Г2)")
	require.Len(t, p.Buildings, 1)
	assert.Equal(t, "Люди", p.Buildings[0].OwnerName)
}
