// internal/models/ship_registry.go
// Реестр кораблей игры как данные (спека
// docs/specs/2026-09-25-арт-студия-удаление-кораблей-из-игры.md §3): файл
// config/ships_registry.json читается при старте игры в package-переменные
// RaceShipSprites/ShipOptions и индексы. После старта реестр только читается
// (игра файл не перечитывает) — гонок нет, мьютекс не нужен.
package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// shipRegistryVersion — поддерживаемая версия формата файла реестра.
const shipRegistryVersion = 1

// shipRegistryFile — формат config/ships_registry.json.
type shipRegistryFile struct {
	Version int          `json:"version"`
	Ships   []ShipSprite `json:"ships"`
}

// ReadShipRegistry — чистое чтение + валидация файла реестра (без побочных
// эффектов; нужна студии и тестам). BOM снимается защитно.
func ReadShipRegistry(path string) ([]ShipSprite, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var f shipRegistryFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("разбор JSON: %w", err)
	}
	if f.Version != 0 && f.Version != shipRegistryVersion {
		return nil, fmt.Errorf("версия реестра %d не поддерживается", f.Version)
	}
	if err := validateShipRegistry(f.Ships); err != nil {
		return nil, err
	}
	return f.Ships, nil
}

// humanShipTypes — фиксированный порядок типов людского блока (спека §3.1,
// как tools/import_ship_sprites.ps1:72).
var humanShipTypes = []string{"starship", "cruiser", "carrier", "fighter"}

// parseHumanShipFile — тип (ранг) и вариант людской записи из имени файла
// race_humans_<тип>[_NN].png; нет суффикса → вариант 1. Непонятный суффикс или
// неизвестный тип — ошибка валидации.
func parseHumanShipFile(file string) (typeRank, variant int, err error) {
	rest := strings.TrimSuffix(strings.TrimPrefix(file, "race_humans_"), ".png")
	typ := rest
	if i := strings.LastIndex(rest, "_"); i >= 0 {
		v, convErr := strconv.Atoi(rest[i+1:])
		if convErr != nil {
			return 0, 0, fmt.Errorf("%s: непонятный вариант людской записи", file)
		}
		typ, variant = rest[:i], v
	} else {
		variant = 1
	}
	for i, t := range humanShipTypes {
		if t == typ {
			return i, variant, nil
		}
	}
	return 0, 0, fmt.Errorf("%s: неизвестный тип людской записи %q", file, typ)
}

// validateShipRegistry — проверки §3.2: непустой массив, обязательные поля,
// ID == File без .png, диапазон угла, уникальность файлов, людской блок первым
// (порядок тип→вариант), нейтральный ровно один и последний, DefaultHumanShip
// присутствует.
func validateShipRegistry(ships []ShipSprite) error {
	if len(ships) == 0 {
		return fmt.Errorf("реестр пуст")
	}
	seen := make(map[string]bool, len(ships))
	humanBlockEnded := false
	prevHumanRank, prevHumanVariant := -1, -1
	neutralCount := 0
	hasDefault := false
	for i, s := range ships {
		if s.ID == "" || s.Name == "" || s.File == "" {
			return fmt.Errorf("запись %d: id/name/file обязательны", i)
		}
		if !strings.HasSuffix(s.File, ".png") {
			return fmt.Errorf("%s: file обязан заканчиваться на .png", s.File)
		}
		if s.ID != strings.TrimSuffix(s.File, ".png") {
			return fmt.Errorf("%s: ID != имя файла без .png", s.File)
		}
		if s.Angle <= -180 || s.Angle > 180 {
			return fmt.Errorf("%s: угол %v вне (−180,180]", s.File, s.Angle)
		}
		if seen[s.File] {
			return fmt.Errorf("%s: файл повторяется", s.File)
		}
		seen[s.File] = true

		if s.File == NeutralShip.File {
			if s.Race != "" {
				return fmt.Errorf("%s: у нейтрального race обязана быть пустой", s.File)
			}
			neutralCount++
			if i != len(ships)-1 {
				return fmt.Errorf("%s: нейтральный обязан быть последним", s.File)
			}
			continue
		}
		if s.Race == "" {
			return fmt.Errorf("%s: race обязана быть непустой", s.File)
		}
		if s.Race == RaceHumans {
			if humanBlockEnded {
				return fmt.Errorf("%s: людские записи обязаны идти подряд в начале", s.File)
			}
			rank, variant, err := parseHumanShipFile(s.File)
			if err != nil {
				return err
			}
			if rank < prevHumanRank || (rank == prevHumanRank && variant <= prevHumanVariant) {
				return fmt.Errorf("%s: порядок людского блока тип→вариант нарушен", s.File)
			}
			prevHumanRank, prevHumanVariant = rank, variant
		} else {
			humanBlockEnded = true
		}
		if s.File == DefaultHumanShip {
			hasDefault = true
		}
	}
	if neutralCount != 1 {
		return fmt.Errorf("нейтральная запись: найдено %d, ожидалась ровно одна", neutralCount)
	}
	if !hasDefault {
		return fmt.Errorf("запись %s отсутствует", DefaultHumanShip)
	}
	return nil
}

// LoadShipRegistry — читает файл и заполняет package-переменные реестра:
// RaceShipSprites (записи без нейтральной), ShipOptions (= RaceShipSprites +
// NeutralShip), индексы shipSpriteByFile/shipFilesByRace. Вызывается один раз
// при старте игры (cmd/server/main.go) до bootstrapSkycomposer. При ошибке
// переменные не меняются — остаётся аварийный минимум
// (applyShipRegistryFallback).
func LoadShipRegistry(path string) error {
	ships, err := ReadShipRegistry(path)
	if err != nil {
		return err
	}
	raceShips := make([]ShipSprite, 0, len(ships)-1)
	for _, s := range ships {
		if s.File == NeutralShip.File {
			continue
		}
		raceShips = append(raceShips, s)
	}
	RaceShipSprites = raceShips
	ShipOptions = append(append([]ShipSprite{}, raceShips...), NeutralShip)
	rebuildShipIndexes()
	return nil
}

// rebuildShipIndexes — строит индексы shipSpriteByFile/shipFilesByRace по
// текущим RaceShipSprites + NeutralShip. Вызывается и для аварийного
// минимума, и после загрузки файла.
func rebuildShipIndexes() {
	byFile := make(map[string]ShipSprite, len(RaceShipSprites)+1)
	for _, s := range RaceShipSprites {
		byFile[s.File] = s
	}
	byFile[NeutralShip.File] = NeutralShip
	shipSpriteByFile = byFile

	byRace := map[string][]string{}
	for _, s := range RaceShipSprites {
		byRace[s.Race] = append(byRace[s.Race], s.File)
	}
	byRace[""] = []string{NeutralShip.File}
	shipFilesByRace = byRace
}

// applyShipRegistryFallback — аварийный минимум реестра (начальное значение
// package-переменных): единственная людская запись DefaultHumanShip (пара
// ориентации Angle: 0 — допустимая деградация) + нейтральный. Нейтральный в
// RaceShipSprites НЕ кладём — в ShipOptions он входит ровно один раз.
func applyShipRegistryFallback() {
	RaceShipSprites = []ShipSprite{
		{ID: "race_humans_starship", Name: "Звёздный корабль (люди)", File: DefaultHumanShip, Race: RaceHumans},
	}
	ShipOptions = append(append([]ShipSprite{}, RaceShipSprites...), NeutralShip)
	rebuildShipIndexes()
}

var (
	// RaceShipSprites — расовый реестр кораблей (без нейтрального). Заполняется
	// LoadShipRegistry; до загрузки — аварийный минимум.
	RaceShipSprites []ShipSprite
	// ShipOptions — транспорт реестра для /me.ship_options: RaceShipSprites +
	// NeutralShip (нейтральный последним).
	ShipOptions []ShipSprite
	// shipSpriteByFile — индекс реестра по имени файла (включая NeutralShip).
	shipSpriteByFile map[string]ShipSprite
	// shipFilesByRace — индекс «раса → файлы»; ключ "" → нейтральный пул.
	shipFilesByRace map[string][]string
)

func init() {
	applyShipRegistryFallback()
}
