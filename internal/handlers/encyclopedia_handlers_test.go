// internal/handlers/encyclopedia_handlers_test.go
// Тесты GET /api/encyclopedia/races (спека 86a §8.1): публичный срез каталога
// рас (whitelist §5.1.1) + лор; без токена — 401; запрещённые поля не
// отдаются (И5).
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/races"
)

// loadEncyclopediaFixtures — каталог рас + лор из реальных файлов (паттерн
// catalog_test.go): ручка читает их из памяти (И11).
func loadEncyclopediaFixtures(t *testing.T) {
	t.Helper()
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	require.NoError(t, races.LoadLore("../../config/race_lore.json"))
}

// Без токена — 401 (как /me).
func TestGetRacesUnauthorized(t *testing.T) {
	h := NewEncyclopediaHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/encyclopedia/races", nil)
	rec := execJSON(h.GetRaces, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["error"])
}

// Ответ: 60 рас, whitelist-поля на месте, запрещённые отсутствуют (И5).
func TestGetRacesWhitelist(t *testing.T) {
	loadEncyclopediaFixtures(t)
	h := NewEncyclopediaHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/encyclopedia/races", nil)
	rec := execJSON(h.GetRaces, withUserID(req, "user-1"))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Races []map[string]interface{} `json:"races"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Races, 60, "все 60 рас в ответе")

	humans := findRace(resp.Races, "humans")
	require.NotNil(t, humans)
	assert.Equal(t, "Люди", humans["name"])
	assert.Equal(t, "F1", humans["family"], "family из лора на верхнем уровне")
	assert.Equal(t, "cold", humans["bulge"])

	// Запрещённые поля отсутствуют (И5).
	for _, forbidden := range []string{"consumption", "territory", "dormancy", "famine_aggression", "off_cascade", "tuning"} {
		_, ok := humans[forbidden]
		assert.False(t, ok, "поле %s не отдаётся", forbidden)
	}
	forage, ok := humans["forage"].(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, forage["source"])
	_, hasBasic := forage["basic_resources"]
	assert.False(t, hasBasic, "forage.basic_resources не отдаётся")
	_, hasWorks := forage["works_where"]
	assert.False(t, hasWorks, "forage.works_where не отдаётся")

	// Опциональные поля — null, если не заданы (heat_flux у людей нет).
	conds, ok := humans["conditions"].(map[string]interface{})
	require.True(t, ok)
	assert.Nil(t, conds["heat_flux"], "heat_flux = null, если не задан")
	assert.NotNil(t, conds["gravity"])
	assert.Equal(t, true, conds["liquid_water"])

	// Лор на месте.
	lore, ok := humans["lore"].(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, lore["character"])
	assert.NotEmpty(t, lore["how_live"])
	assert.NotEmpty(t, lore["why"])
	assert.NotEmpty(t, lore["coexistence"])
	assert.Nil(t, lore["origin"], "у био-рас origin = null")

	// Роботы: robotic-блок без heat_twist и basic_resources.
	robots := findRace(resp.Races, "archivists")
	require.NotNil(t, robots)
	assert.Equal(t, "robotic", robots["family"])
	rob, ok := robots["robotic"].(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, rob["power_source"])
	assert.NotEmpty(t, rob["heat"])
	mats, ok := rob["materials"].(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, mats["categories"])
	assert.NotEmpty(t, mats["axes"])
	_, hasTwist := rob["heat_twist"]
	assert.False(t, hasTwist, "robotic.heat_twist не отдаётся (флаг механики-кандидата)")
	_, hasRobBasic := mats["basic_resources"]
	assert.False(t, hasRobBasic, "robotic.materials.basic_resources не отдаётся")
	loreR, ok := robots["lore"].(map[string]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, loreR["origin"], "у роботов origin задан")
}

// findRace — раса по id в ответе ручки.
func findRace(races []map[string]interface{}, id string) map[string]interface{} {
	for _, r := range races {
		if r["id"] == id {
			return r
		}
	}
	return nil
}

// Новые поля карточки «игровое восприятие» (спека 99.2.26 §3.2): kind/niche/
// size_individual/size_group/home_words/lore/attributes_words в объекте lore;
// size_group = null у людей, строка у коллективной (sulfur_swarms); у робота
// (archivists) kind + origin на месте.
func TestGetRacesNewLoreFields(t *testing.T) {
	loadEncyclopediaFixtures(t)
	h := NewEncyclopediaHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/encyclopedia/races", nil)
	rec := execJSON(h.GetRaces, withUserID(req, "user-1"))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Races []map[string]interface{} `json:"races"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Races, 60)

	// Люди (неколлективная): новые поля в lore, size_group = null.
	humans := findRace(resp.Races, "humans")
	require.NotNil(t, humans)
	lore, ok := humans["lore"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "гуманоид", lore["kind"])
	assert.Equal(t, "строитель", lore["niche"])
	assert.Equal(t, "с человека", lore["size_individual"])
	assert.Nil(t, lore["size_group"], "у людей size_group = null")
	assert.NotEmpty(t, lore["home_words"])
	assert.NotEmpty(t, lore["lore"])
	aw, ok := lore["attributes_words"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, aw, 6)
	assert.NotEmpty(t, aw["aggression"])
	assert.NotEmpty(t, aw["reproduction"])

	// Коллективная раса: size_group — строка.
	swarms := findRace(resp.Races, "sulfur_swarms")
	require.NotNil(t, swarms)
	loreS, ok := swarms["lore"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "рой", loreS["kind"])
	assert.Equal(t, "рой-облако", loreS["size_group"])

	// Робот: kind + origin на месте.
	robots := findRace(resp.Races, "archivists")
	require.NotNil(t, robots)
	loreR, ok := robots["lore"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "машина", loreR["kind"])
	assert.NotEmpty(t, loreR["origin"])
}