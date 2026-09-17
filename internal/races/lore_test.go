// internal/races/lore_test.go
// Тесты лора рас (спека 86a §5.1.1): config/race_lore.json — машиночитаемая
// проекция 22_races.md §3/§4; валидация формата как у каталога рас.
package races

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Лор загружается: 60 записей, все проходят валидацию (id в каталоге,
// family из набора, текстовые поля непусты).
func TestLoadLoreValidatesAllRaces(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	require.Len(t, LoreCatalog(), 60, "лор — 60 записей (по одной на расу)")
	for _, l := range LoreCatalog() {
		require.NoError(t, l.Validate(), "лор %s", l.ID)
	}
}

// Каждая раса каталога имеет запись лора (и наоборот — ровно 60).
func TestLoreCoversEveryRace(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	byID := map[string]*RaceLore{}
	for _, l := range LoreCatalog() {
		byID[l.ID] = l
	}
	for _, r := range Catalog() {
		assert.NotNil(t, byID[r.ID], "раса %s имеет запись лора", r.ID)
	}
}

// Family — из набора F1–F9/robotic (22_races.md §2.2/§4).
func TestLoreFamiliesValid(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	for _, l := range LoreCatalog() {
		assert.True(t, loreFamilies[l.Family], "лор %s: family %q из набора", l.ID, l.Family)
	}
}

// Роботы (family = robotic) имеют origin; био-расы — нет (22_races.md §3/§4).
func TestLoreRobotsHaveOriginBioDont(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	for _, l := range LoreCatalog() {
		if l.Family == "robotic" {
			assert.NotEmpty(t, l.Origin, "робот %s: origin задан", l.ID)
		} else {
			assert.Empty(t, l.Origin, "био-раса %s: origin пуст", l.ID)
		}
	}
}

// Валидация ловит id вне каталога.
func TestLoreRejectsUnknownID(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "nope", Family: "F1", Character: "x", HowLive: "x", Why: "x", Coexistence: "x"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "нет в каталоге")
}

// Валидация ловит family вне набора.
func TestLoreRejectsBadFamily(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "humans", Family: "F10", Character: "x", HowLive: "x", Why: "x", Coexistence: "x"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "family")
}

// Валидация ловит пустое текстовое поле.
func TestLoreRejectsEmptyField(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "humans", Family: "F1", Character: "", HowLive: "x", Why: "x", Coexistence: "x"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "character")
}

// Валидация ловит робота без origin.
func TestLoreRejectsRobotWithoutOrigin(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "archivists", Family: "robotic", Character: "x", HowLive: "x", Why: "x", Coexistence: "x"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin")
}

// Валидация ловит био-расу с origin.
func TestLoreRejectsBioWithOrigin(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "humans", Family: "F1", Character: "x", HowLive: "x", Why: "x", Coexistence: "x", Origin: "люди"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin")
}