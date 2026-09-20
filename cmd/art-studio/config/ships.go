package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ShipsConfig — проекция races/ships/*.md (config/art/ships.json, спека
// 2026-09-20-ships-races-generator §4.1): карта slug → запись корабля расы.
// kind=duplicates: источник истины — лор-файлы, пересборка кнопкой
// «Пересобрать промт» (как families.json ↔ races/*.md, 98a §3).
type ShipsConfig map[string]ShipEntry

// ShipEntry — запись корабля расы (поля — дословно из маркеров
// «**Для генератора (...):**» раздела «## Корабль (внешний вид)»).
type ShipEntry struct {
	RaceName   string   `json:"race_name"`
	Family     string   `json:"family"`
	Texture    string   `json:"texture"`
	Silhouette string   `json:"silhouette"`
	Blocked    []string `json:"blocked"`
}

// LoadShips читает и валидирует config/art/ships.json.
// Валидация (спека §4.1): ключ ∈ id рас config/races.json; texture непустая,
// без \n; silhouette непустая; blocked — токены непустые, нормализация
// lowercase/trim/дедуп (как 98a §4.3). Отсутствие записи расы — валидно
// (пачка ещё не пересобрана), не ошибка.
func LoadShips(path string) (ShipsConfig, error) {
	return LoadShipsRaces(path, "config/races.json")
}

// LoadShipsRaces — LoadShips с явным путём к config/races.json (тесты,
// пересборка промпта: путь может быть относительным от CWD вызывающего).
func LoadShipsRaces(path, racesPath string) (ShipsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseShips(data, path, racesPath)
}

// ParseShips — валидация ships.json из байтов (общая для LoadShips и
// пересборки промпта: результат патча проверяется до записи на диск).
// racesPath — путь к config/races.json (валидация ключей).
func ParseShips(data []byte, path, racesPath string) (ShipsConfig, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	delete(raw, "comment") // служебное поле файла
	ids := loadRaceIDs(racesPath)
	ships := ShipsConfig{}
	for k, v := range raw {
		var e ShipEntry
		if err := json.Unmarshal(v, &e); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", path, k, err)
		}
		if !ids[k] {
			return nil, fmt.Errorf("%s: %s: нет в config/races.json", path, k)
		}
		if strings.TrimSpace(e.Texture) == "" {
			return nil, fmt.Errorf("%s: %s: texture пуст", path, k)
		}
		if strings.Contains(e.Texture, "\n") {
			return nil, fmt.Errorf("%s: %s: texture содержит перенос строки", path, k)
		}
		if strings.TrimSpace(e.Silhouette) == "" {
			return nil, fmt.Errorf("%s: %s: silhouette пуст", path, k)
		}
		// blocked: нормализация lowercase+trim, дедуп (тихо, порядок первого
		// вхождения); токены непустые.
		if len(e.Blocked) > 0 {
			seen := make(map[string]bool, len(e.Blocked))
			norm := make([]string, 0, len(e.Blocked))
			for _, tok := range e.Blocked {
				t := strings.ToLower(strings.TrimSpace(tok))
				if t == "" {
					return nil, fmt.Errorf("%s: %s: blocked содержит пустой токен", path, k)
				}
				if !seen[t] {
					seen[t] = true
					norm = append(norm, t)
				}
			}
			e.Blocked = norm
		}
		ships[k] = e
	}
	return ships, nil
}

// loadRaceIDs — множество id рас из config/races.json (валидация ключей
// ships.json). Если файл не найден/невалиден — пустая map (фолбек, как
// loadRaceSlug в handlers).
func loadRaceIDs(path string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var doc struct {
		Races []struct {
			ID string `json:"id"`
		} `json:"races"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return out
	}
	for _, rc := range doc.Races {
		out[rc.ID] = true
	}
	return out
}