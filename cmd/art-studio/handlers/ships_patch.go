package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"zorion/cmd/art-studio/config"
)

// updateShipsRace обновляет запись расы в ships.json на диске (кнопка
// «Пересобрать промт», спека §6.1 п.3). Минимальный дифф: меняются только
// поля расы, остальной файл сохраняется дословно (байтовая замена значений
// полей). Если записи нет — вставляется новая перед закрывающей } корня.
// Перед записью результат валидируется (json.Valid + ParseShips) — невалидное
// не записывается. racesPath — путь к config/races.json (валидация ключей).
func updateShipsRace(path, slug string, entry config.ShipEntry, racesPath string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, _, ok := findKeyObject(data, slug); !ok {
		data = insertShipEntry(data, slug, entry)
	} else {
		fields := []struct {
			name  string
			value string
		}{
			{"race_name", jsonString(entry.RaceName)},
			{"family", jsonString(entry.Family)},
			{"texture", jsonString(entry.Texture)},
			{"silhouette", jsonString(entry.Silhouette)},
			{"blocked", "[" + quotedTokens(entry.Blocked) + "]"},
		}
		for _, f := range fields {
			if data, err = patchShipField(data, slug, f.name, f.value); err != nil {
				return err
			}
		}
		if len(entry.Types) > 0 {
			tv, err := json.Marshal(entry.Types)
			if err != nil {
				return err
			}
			if data, err = patchShipField(data, slug, "types", string(tv)); err != nil {
				return err
			}
		}
	}
	if !json.Valid(data) {
		return errors.New("результат записи невалиден (json.Valid) — файл не изменён")
	}
	if _, err := config.ParseShips(data, path, racesPath); err != nil {
		return fmt.Errorf("результат записи невалиден: %v — файл не изменён", err)
	}
	return os.WriteFile(path, data, 0644)
}

// findKeyObject возвращает [start, end) диапазон JSON-объекта по ключу
// («"<key>": {»; ships.json — карта slug → объект, id-поля в записи нет).
func findKeyObject(data []byte, key string) (int, int, bool) {
	needle := []byte(`"` + key + `": {`)
	i := bytes.Index(data, needle)
	if i < 0 {
		return 0, 0, false
	}
	start := i + len(`"`+key+`"`)
	for start < len(data) && (data[start] == ' ' || data[start] == '\t' || data[start] == '\n' || data[start] == '\r' || data[start] == ':') {
		start++
	}
	depth := 0
	end := -1
	for j := start; j < len(data); j++ {
		switch data[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = j + 1
				j = len(data)
			}
		}
	}
	if end < 0 {
		return 0, 0, false
	}
	return start, end, true
}

// patchShipField заменяет (или вставляет перед закрывающей } объекта) поле
// field в объекте расы slug (ships.json).
func patchShipField(data []byte, slug, field, value string) ([]byte, error) {
	start, end, ok := findKeyObject(data, slug)
	if !ok {
		return nil, fmt.Errorf("раса %s не найдена в ships.json", slug)
	}
	if _, _, ok := findFieldValue(data, start, end, field); ok {
		out, _ := replaceFieldValue(data, start, end, field, value)
		return out, nil
	}
	return insertField(data, end-1, field, value), nil
}

// insertShipEntry вставляет «"slug": {...}» перед закрывающей } корня
// (новой расы в ships.json ещё нет).
func insertShipEntry(data []byte, slug string, entry config.ShipEntry) []byte {
	closeBrace := bytes.LastIndexByte(data, '}')
	ins := ",\n  \"" + slug + "\": " + shipEntryJSON(entry)
	out := make([]byte, 0, len(data)+len(ins))
	out = append(out, data[:closeBrace]...)
	out = append(out, ins...)
	out = append(out, data[closeBrace:]...)
	return out
}

// shipEntryJSON — JSON-объект записи корабля (стиль ships.json: 2 пробела).
// Типы корабля (types) добавляются, если заданы.
func shipEntryJSON(entry config.ShipEntry) string {
	out := fmt.Sprintf(`{"race_name": %s, "family": %s, "texture": %s, "silhouette": %s, "blocked": [%s]`,
		jsonString(entry.RaceName), jsonString(entry.Family), jsonString(entry.Texture),
		jsonString(entry.Silhouette), quotedTokens(entry.Blocked))
	if len(entry.Types) > 0 {
		if tv, err := json.Marshal(entry.Types); err == nil {
			out += fmt.Sprintf(`, "types": %s`, tv)
		}
	}
	return out + "}"
}