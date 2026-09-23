// internal/models/race_ship_sprites_test.go
// Тесты расового реестра кораблей (спека
// docs/specs/2026-09-23-корабли-рас-раса-агентов-и-игрока.md §6.1–§6.5, П1):
// состав и порядок людского блока, нейтральный корабль, индексы
// (shipSpriteByFile/shipFilesByRace), RandomShipForRace, файлы на диске.
package models

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// П3: 117 расовых записей (12 людских типизированных + 105 расовых).
func TestRaceShipSpritesSize(t *testing.T) {
	require.Len(t, RaceShipSprites, 117)
}

// Порядок фиксирован (спека §6.1 п.6): людские записи первыми — starship →
// cruiser → carrier → fighter, внутри типа base → _02 → _03 (12); затем 105
// расовых дописаны В КОНЕЦ (И7). ID = имя файла без .png — у всех записей.
func TestRaceShipSpritesOrder(t *testing.T) {
	humanBlock := []string{
		"race_humans_starship.png",
		"race_humans_starship_02.png",
		"race_humans_starship_03.png",
		"race_humans_cruiser.png",
		"race_humans_cruiser_02.png",
		"race_humans_cruiser_03.png",
		"race_humans_carrier.png",
		"race_humans_carrier_02.png",
		"race_humans_carrier_03.png",
		"race_humans_fighter.png",
		"race_humans_fighter_02.png",
		"race_humans_fighter_03.png",
	}
	for _, s := range RaceShipSprites {
		require.Equal(t, s.File[:len(s.File)-len(".png")], s.ID, "%s: ID = имя без .png", s.File)
	}
	require.Len(t, RaceShipSprites, len(humanBlock)+105)
	for i, f := range humanBlock {
		require.Equal(t, f, RaceShipSprites[i].File, "людской блок, позиция %d", i)
		require.Equal(t, RaceHumans, RaceShipSprites[i].Race, "%s: раса humans", f)
	}
	// После людского блока — только расовые записи (не humans), порядок меты.
	for i := len(humanBlock); i < len(RaceShipSprites); i++ {
		require.NotEqual(t, RaceHumans, RaceShipSprites[i].Race,
			"%s: расовые записи идут после людского блока (И7)", RaceShipSprites[i].File)
	}
}

// Диапазон угла пары у всех записей реестра: −180 < Angle ≤ 180 (конвенция §3.1).
func TestRaceShipSpritesOrientInRange(t *testing.T) {
	for _, s := range RaceShipSprites {
		require.Greater(t, s.Angle, -180.0, "%s: Angle > -180", s.ID)
		require.LessOrEqual(t, s.Angle, 180.0, "%s: Angle <= 180", s.ID)
	}
}

// Каждый файл реестра (и neutral) существует в web/static/sprites/.
func TestRaceShipSpritesFilesExist(t *testing.T) {
	for _, s := range RaceShipSprites {
		_, err := os.Stat("../../web/static/sprites/" + s.File)
		require.NoError(t, err, "спрайт %s должен существовать", s.File)
	}
	_, err := os.Stat("../../web/static/sprites/" + NeutralShip.File)
	require.NoError(t, err, "нейтральный спрайт должен существовать")
}

// Нейтральный корабль — член реестра (N2): в ship_options и валидации.
func TestNeutralShipIsRegistryMember(t *testing.T) {
	require.Equal(t, "neutral.png", NeutralShip.File)
	require.Empty(t, NeutralShip.Race)
	require.True(t, IsValidShipIcon(NeutralShip.File))
}

// ship_options (П3) = 117 расовых + neutral = 118 строк, нейтральный последним.
func TestShipOptionsSize(t *testing.T) {
	require.Len(t, ShipOptions, 118)
	require.Equal(t, NeutralShip, ShipOptions[len(ShipOptions)-1])
}

// D1: поле race присутствует в JSON у КАЖДОЙ строки ship_options, включая
// нейтральную (пустая строка, но не отсутствие) — клиент строит индекс
// «раса → файлы» по ключу "" (спека §6.2, N12).
func TestShipOptionsRaceFieldAlwaysPresent(t *testing.T) {
	for _, s := range ShipOptions {
		raw, err := json.Marshal(s)
		require.NoError(t, err)
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &m))
		_, ok := m["race"]
		require.True(t, ok, "%s: поле race обязано присутствовать", s.File)
	}
}

// shipFilesByRace: пул humans = 12 файлов (порядок реестра), ключ "" = neutral.
func TestShipFilesByRaceIndex(t *testing.T) {
	require.Len(t, shipFilesByRace[RaceHumans], 12)
	require.Equal(t, []string{NeutralShip.File}, shipFilesByRace[""])
	require.Equal(t, RaceShipSprites[0].File, shipFilesByRace[RaceHumans][0])
}

// 57 рас с кораблями (humans + 56 расовых), спека §10. 3 расы без корабля
// (cryo_sky, geysers, ice_plankton) пула не имеют — клиент берёт нейтральный
// (§6.4), не уходя в него при неполном наборе суффиксов.
func TestRaceShipSpritesRaceCount(t *testing.T) {
	races := map[string]bool{}
	for _, s := range RaceShipSprites {
		races[s.Race] = true
	}
	require.Len(t, races, 57)
	require.True(t, races[RaceHumans])
	for _, race := range []string{"cryo_sky", "geysers", "ice_plankton"} {
		require.Empty(t, shipFilesByRace[race], "%s: расы нет в реестре кораблей", race)
		require.False(t, races[race])
	}
}

// RandomShipForRace: humans → файл людского пула; неизвестная раса/пусто →
// нейтральный (страховка §6.5).
func TestRandomShipForRace(t *testing.T) {
	pool := map[string]bool{}
	for _, f := range shipFilesByRace[RaceHumans] {
		pool[f] = true
	}
	for i := 0; i < 50; i++ {
		require.True(t, pool[RandomShipForRace(RaceHumans)], "humans → людской файл")
	}
	require.Equal(t, NeutralShip.File, RandomShipForRace("unknown_race"))
	require.Equal(t, NeutralShip.File, RandomShipForRace(""))
}

// DefaultHumanShip — член реестра (миграция 000074 ставит его DEFAULT).
func TestDefaultHumanShipIsRegistryMember(t *testing.T) {
	require.True(t, IsValidShipIcon(DefaultHumanShip))
	require.Equal(t, RaceShipSprites[0].File, DefaultHumanShip)
}
