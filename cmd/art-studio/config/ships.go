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
// Types — необязательный список типов корабля расы (starship/cruiser/…): у
// обычных рас его нет (один безымянный корабль из плоских полей), у людей —
// 4 типа с разными texture. Плоские поля при наличии types — базовые
// (наследуются типами, у которых поле не задано).
type ShipEntry struct {
	RaceName   string     `json:"race_name"`
	Family     string     `json:"family"`
	Texture    string     `json:"texture"`
	Silhouette string     `json:"silhouette"`
	Blocked    []string   `json:"blocked"`
	Types      []ShipType `json:"types,omitempty"`
}

// ShipType — тип корабля расы (starship/cruiser/carrier/fighter и т.п.).
// Texture/Silhouette/Blocked наследуют базовые поля записи, если не заданы.
type ShipType struct {
	Type       string   `json:"type"`
	Texture    string   `json:"texture"`
	Silhouette string   `json:"silhouette,omitempty"`
	Blocked    []string `json:"blocked,omitempty"`
}

// ShipTypes — нормализованный список типов корабля расы: без types — один
// безымянный тип из плоских полей (легаси-совместимость); с types — каждый тип
// с наследованием незаполненных полей от базовой записи.
func (e ShipEntry) ShipTypes() []ShipType {
	if len(e.Types) == 0 {
		return []ShipType{{Texture: e.Texture, Silhouette: e.Silhouette, Blocked: e.Blocked}}
	}
	out := make([]ShipType, len(e.Types))
	for i, t := range e.Types {
		if t.Texture == "" {
			t.Texture = e.Texture
		}
		if t.Silhouette == "" {
			t.Silhouette = e.Silhouette
		}
		if len(t.Blocked) == 0 {
			t.Blocked = e.Blocked
		}
		out[i] = t
	}
	return out
}

// ShipTypeNames — имена типов расы (пусто у легаси-рас с одним безымянным
// типом) — для UI/списка.
func (e ShipEntry) ShipTypeNames() []string {
	if len(e.Types) == 0 {
		return nil
	}
	out := make([]string, 0, len(e.Types))
	for _, t := range e.Types {
		out = append(out, t.Type)
	}
	return out
}

// ResolveShipType — тип корабля по имени (пусто → первый тип) с наследованием
// базовых полей. ok=false — расы/типа нет.
func (e ShipEntry) ResolveShipType(name string) (ShipType, bool) {
	types := e.ShipTypes()
	if len(types) == 0 {
		return ShipType{}, false
	}
	if name == "" {
		return types[0], true
	}
	for _, t := range types {
		if t.Type == name {
			return t, true
		}
	}
	return ShipType{}, false
}

// ForType — плоская запись корабля с полями типа t (генератор/промпт):
// race_name/family сохраняются, texture/silhouette/blocked — из типа.
func (e ShipEntry) ForType(t ShipType) ShipEntry {
	return ShipEntry{
		RaceName: e.RaceName, Family: e.Family,
		Texture: t.Texture, Silhouette: t.Silhouette, Blocked: t.Blocked,
	}
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
			norm, err := normalizeBlocked(e.Blocked)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %v", path, k, err)
			}
			e.Blocked = norm
		}
		// types: имя типа непустое и уникальное, texture типа (или базовая)
		// непустая, blocked типа нормализуется.
		if len(e.Types) > 0 {
			seen := make(map[string]bool, len(e.Types))
			for i := range e.Types {
				t := &e.Types[i]
				name := strings.TrimSpace(t.Type)
				if name == "" {
					return nil, fmt.Errorf("%s: %s: types[%d]: пустое имя типа", path, k, i)
				}
				if seen[name] {
					return nil, fmt.Errorf("%s: %s: дубль типа %q", path, k, name)
				}
				seen[name] = true
				t.Type = name
				if strings.TrimSpace(t.Texture) == "" && strings.TrimSpace(e.Texture) == "" {
					return nil, fmt.Errorf("%s: %s: тип %s: texture пуст", path, k, name)
				}
				if strings.Contains(t.Texture, "\n") {
					return nil, fmt.Errorf("%s: %s: тип %s: texture содержит перенос строки", path, k, name)
				}
				if len(t.Blocked) > 0 {
					norm, err := normalizeBlocked(t.Blocked)
					if err != nil {
						return nil, fmt.Errorf("%s: %s: тип %s: %v", path, k, name, err)
					}
					t.Blocked = norm
				}
			}
		}
		ships[k] = e
	}
	return ships, nil
}

// normalizeBlocked — нормализация токенов blocked (lowercase/trim, дедуп по
// порядку первого вхождения); пустой токен — ошибка.
func normalizeBlocked(tokens []string) ([]string, error) {
	seen := make(map[string]bool, len(tokens))
	norm := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		t := strings.ToLower(strings.TrimSpace(tok))
		if t == "" {
			return nil, fmt.Errorf("blocked содержит пустой токен")
		}
		if !seen[t] {
			seen[t] = true
			norm = append(norm, t)
		}
	}
	return norm, nil
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