// internal/models/race_ship_sprites_test.go
// Тесты расового реестра кораблей (спека
// docs/specs/2026-09-23-корабли-рас-раса-агентов-и-игрока.md §6.1–§6.5, П1;
// спека 2026-09-25-арт-студия-удаление-кораблей-из-игры.md §3.5): состав и
// порядок людского блока, нейтральный корабль, индексы
// (shipSpriteByFile/shipFilesByRace), RandomShipForRace, файлы на диске.
// Тесты data-agnostic: длины/состав считаются из файла реестра, не хардкодятся.
package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/races"
)

const shipRegistryPath = "../../config/ships_registry.json"

// TestMain — реестр кораблей читается из файла данных (как в игре при старте).
func TestMain(m *testing.M) {
	if err := LoadShipRegistry(shipRegistryPath); err != nil {
		panic("LoadShipRegistry: " + err.Error())
	}
	os.Exit(m.Run())
}

// registryShips — записи файла реестра (включая нейтральную).
func registryShips(t *testing.T) []ShipSprite {
	t.Helper()
	ships, err := ReadShipRegistry(shipRegistryPath)
	require.NoError(t, err)
	return ships
}

// humanBlock — людской блок из данных: ведущие записи race==humans.
func humanBlock(ships []ShipSprite) []ShipSprite {
	var block []ShipSprite
	for _, s := range ships {
		if s.Race != RaceHumans {
			break
		}
		block = append(block, s)
	}
	return block
}

// Число расовых записей == числу в файле минус нейтральная.
func TestRaceShipSpritesSize(t *testing.T) {
	ships := registryShips(t)
	require.Len(t, RaceShipSprites, len(ships)-1)
}

// Порядок: людской блок первым (тип→вариант), после него людей нет;
// ID = имя файла без .png у всех записей.
func TestRaceShipSpritesOrder(t *testing.T) {
	for _, s := range RaceShipSprites {
		require.Equal(t, strings.TrimSuffix(s.File, ".png"), s.ID, "%s: ID = имя без .png", s.File)
	}
	block := humanBlock(RaceShipSprites)
	require.NotEmpty(t, block, "людской блок не пуст")
	typeRank := map[string]int{"starship": 0, "cruiser": 1, "carrier": 2, "fighter": 3}
	prevType, prevVariant := -1, -1
	for _, s := range block {
		rest := strings.TrimSuffix(strings.TrimPrefix(s.File, "race_humans_"), ".png")
		typ, variant := rest, 0
		if i := strings.LastIndex(rest, "_"); i >= 0 {
			if v, err := strconv.Atoi(rest[i+1:]); err == nil {
				typ, variant = rest[:i], v
			}
		}
		rank, ok := typeRank[typ]
		require.True(t, ok, "%s: неизвестный тип %q", s.File, typ)
		require.True(t, rank > prevType || (rank == prevType && variant > prevVariant),
			"%s: порядок тип→вариант нарушен", s.File)
		prevType, prevVariant = rank, variant
	}
	// После людского блока — только расовые записи (не humans), порядок меты.
	for i := len(block); i < len(RaceShipSprites); i++ {
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

// ship_options = расовые записи файла + neutral, нейтральный последним.
func TestShipOptionsSize(t *testing.T) {
	ships := registryShips(t)
	require.Len(t, ShipOptions, len(ships))
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

// shipFilesByRace: пул humans == людской блок (порядок реестра), ключ "" = neutral.
func TestShipFilesByRaceIndex(t *testing.T) {
	block := humanBlock(RaceShipSprites)
	require.NotEmpty(t, block)
	require.Len(t, shipFilesByRace[RaceHumans], len(block))
	for i, s := range block {
		require.Equal(t, s.File, shipFilesByRace[RaceHumans][i])
	}
	require.Equal(t, []string{NeutralShip.File}, shipFilesByRace[""])
}

// Все расы реестра (кроме "") есть в config/races.json. Список рас без корабля
// не хардкодится — он производный (после удалений меняется).
func TestRaceShipSpritesRacesExistInCatalog(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	racesInRegistry := map[string]bool{}
	for _, s := range RaceShipSprites {
		racesInRegistry[s.Race] = true
	}
	require.True(t, racesInRegistry[RaceHumans])
	for race := range racesInRegistry {
		require.NotNil(t, races.ByID(race), "раса %q реестра отсутствует в config/races.json", race)
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

// Файл реестра обязан содержать обе защищённые записи (страховка от дрейфа).
func TestShipRegistryFileHasProtectedShips(t *testing.T) {
	ships := registryShips(t)
	var hasDefault, hasNeutral bool
	for _, s := range ships {
		if s.File == DefaultHumanShip {
			hasDefault = true
		}
		if s.File == NeutralShip.File {
			hasNeutral = true
		}
	}
	require.True(t, hasDefault, "файл реестра обязан содержать %s", DefaultHumanShip)
	require.True(t, hasNeutral, "файл реестра обязан содержать %s", NeutralShip.File)
}

// Аварийный минимум: отсутствующий/битый файл → ошибка, паники нет; package-
// переменные остаются рабочими (RaceShipSprites=[DefaultHumanShip],
// ShipOptions без дубля нейтрального, shipFilesByRace[""]=[NeutralShip]).
func TestLoadShipRegistryFallback(t *testing.T) {
	prevRace, prevOpts := RaceShipSprites, ShipOptions
	prevByFile, prevByRace := shipSpriteByFile, shipFilesByRace
	t.Cleanup(func() {
		RaceShipSprites, ShipOptions = prevRace, prevOpts
		shipSpriteByFile, shipFilesByRace = prevByFile, prevByRace
	})

	require.Error(t, LoadShipRegistry(filepath.Join(t.TempDir(), "no_such_registry.json")))

	applyShipRegistryFallback()
	require.Len(t, RaceShipSprites, 1)
	require.Equal(t, DefaultHumanShip, RaceShipSprites[0].File)
	require.Equal(t, RaceHumans, RaceShipSprites[0].Race)
	require.Len(t, ShipOptions, 2)
	require.Equal(t, NeutralShip, ShipOptions[1])
	// нейтральный входит ровно один раз — дубля нет
	n := 0
	for _, s := range ShipOptions {
		if s.File == NeutralShip.File {
			n++
		}
	}
	require.Equal(t, 1, n)
	require.Equal(t, []string{NeutralShip.File}, shipFilesByRace[""])
	require.True(t, IsValidShipIcon(DefaultHumanShip))
	require.True(t, IsValidShipIcon(NeutralShip.File))
}

// Битый JSON → ошибка чтения (без паники).
func TestReadShipRegistryBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o644))
	_, err := ReadShipRegistry(path)
	require.Error(t, err)
}

// writeRegistry — temp-файл реестра из записей (для проверок валидации).
func writeRegistry(t *testing.T, ships []ShipSprite) string {
	t.Helper()
	raw, err := json.Marshal(shipRegistryFile{Version: shipRegistryVersion, Ships: ships})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "registry.json")
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	return path
}

// humanShip — людская запись реестра по имени файла.
func humanShip(file string) ShipSprite {
	return ShipSprite{ID: strings.TrimSuffix(file, ".png"), Name: file, File: file, Race: RaceHumans}
}

// Валидация загрузчика: корректный порядок людского блока (тип→вариант) → ок.
func TestReadShipRegistryHumanOrderOK(t *testing.T) {
	path := writeRegistry(t, []ShipSprite{
		humanShip("race_humans_starship.png"),
		humanShip("race_humans_starship_02.png"),
		humanShip("race_humans_cruiser.png"),
		humanShip("race_humans_carrier.png"),
		humanShip("race_humans_fighter.png"),
		NeutralShip,
	})
	_, err := ReadShipRegistry(path)
	require.NoError(t, err)
}

// Переставленный людской блок (тип→вариант) → ошибка валидации.
func TestReadShipRegistryHumanOrderReordered(t *testing.T) {
	path := writeRegistry(t, []ShipSprite{
		humanShip("race_humans_cruiser.png"),
		humanShip("race_humans_starship.png"),
		NeutralShip,
	})
	_, err := ReadShipRegistry(path)
	require.Error(t, err)
}

// Нарушен порядок вариантов внутри типа (base → _02 → _03) → ошибка.
func TestReadShipRegistryHumanVariantOrderReordered(t *testing.T) {
	path := writeRegistry(t, []ShipSprite{
		humanShip("race_humans_starship_03.png"),
		humanShip("race_humans_starship_02.png"),
		NeutralShip,
	})
	_, err := ReadShipRegistry(path)
	require.Error(t, err)
}

// Битый/непонятный суффикс людской записи → ошибка валидации.
func TestReadShipRegistryHumanBadSuffix(t *testing.T) {
	path := writeRegistry(t, []ShipSprite{
		humanShip("race_humans_starship_abc.png"),
		NeutralShip,
	})
	_, err := ReadShipRegistry(path)
	require.Error(t, err)
}

// BOM снимается защитно: файл с BOM читается как обычный.
func TestReadShipRegistryStripsBOM(t *testing.T) {
	raw, err := os.ReadFile(shipRegistryPath)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "bom.json")
	require.NoError(t, os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, raw...), 0o644))
	ships, err := ReadShipRegistry(path)
	require.NoError(t, err)
	require.NotEmpty(t, ships)
}
