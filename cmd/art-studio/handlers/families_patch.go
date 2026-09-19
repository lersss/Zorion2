package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"zorion/cmd/art-studio/config"
)

// updateFamiliesRace обновляет appearance/blocked расы в families.json на диске
// (кнопка «Пересобрать промт»). Минимальный дифф: меняются только поля расы,
// остальной файл сохраняется дословно (байтовая замена значений полей, формат
// файла — JSON с отступами 2 пробела, как сейчас). Перед записью результат
// валидируется (json.Valid + ParseFamilies) — невалидное не записывается.
func updateFamiliesRace(path, raceID, appearance string, blocked []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	appValue := jsonString(appearance)
	blValue := "[" + quotedTokens(blocked) + "]"
	// appearance и blocked заменяются по очереди; после первой правки объект
	// расы пересчитывается заново (data изменилась)
	if data, err = patchRaceField(data, raceID, "appearance", appValue); err != nil {
		return err
	}
	if data, err = patchRaceField(data, raceID, "blocked", blValue); err != nil {
		return err
	}
	if !json.Valid(data) {
		return errors.New("результат записи невалиден (json.Valid) — файл не изменён")
	}
	if _, err := config.ParseFamilies(data, path); err != nil {
		return fmt.Errorf("результат записи невалиден: %v — файл не изменён", err)
	}
	return os.WriteFile(path, data, 0644)
}

// patchRaceField заменяет (или вставляет перед закрывающей } объекта) поле
// field в объекте расы raceID. Возвращает новые байты файла.
func patchRaceField(data []byte, raceID, field, value string) ([]byte, error) {
	start, end, ok := findRaceObject(data, raceID)
	if !ok {
		return nil, fmt.Errorf("раса %s не найдена в families.json", raceID)
	}
	if _, _, ok := findFieldValue(data, start, end, field); ok {
		out, _ := replaceFieldValue(data, start, end, field, value)
		return out, nil
	}
	return insertField(data, end-1, field, value), nil
}

// findRaceObject возвращает [start, end) диапазон JSON-объекта расы с данным
// id (по ключу «"id": "<raceID>"»; id в families.json уникальны).
func findRaceObject(data []byte, raceID string) (int, int, bool) {
	key := []byte(`"id": "` + raceID + `"`)
	i := bytes.Index(data, key)
	if i < 0 {
		return 0, 0, false
	}
	// открывающая { объекта расы: назад от id, баланс скобок
	depth := 0
	start := -1
	for j := i - 1; j >= 0; j-- {
		switch data[j] {
		case '}':
			depth++ // вложенный объект закрылся до id (в прямом порядке)
		case '{':
			if depth == 0 {
				start = j
			} else {
				depth--
			}
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return 0, 0, false
	}
	// закрывающая } объекта расы: вперёд от id
	depth = 0
	end := -1
	for j := i; j < len(data); j++ {
		switch data[j] {
		case '{':
			depth++
		case '}':
			if depth == 0 {
				end = j + 1
			} else {
				depth--
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return 0, 0, false
	}
	return start, end, true
}

// findFieldValue возвращает [start, end) диапазон значения поля field
// (после «"field":») внутри объекта [objStart, objEnd). Значение — строка
// или плоский массив (в families.json appearance — строка, blocked — массив).
func findFieldValue(data []byte, objStart, objEnd int, field string) (int, int, bool) {
	key := []byte(`"` + field + `":`)
	i := bytes.Index(data[objStart:objEnd], key)
	if i < 0 {
		return 0, 0, false
	}
	i += objStart + len(key)
	for i < objEnd && (data[i] == ' ' || data[i] == '\t' || data[i] == '\n' || data[i] == '\r') {
		i++
	}
	if i >= objEnd {
		return 0, 0, false
	}
	switch data[i] {
	case '"':
		// строка: до закрывающей кавычки с учётом экранирования
		j := i + 1
		for j < objEnd {
			if data[j] == '\\' {
				j += 2
				continue
			}
			if data[j] == '"' {
				j++
				break
			}
			j++
		}
		return i, j, true
	case '[':
		// плоский массив: до закрывающей ]
		depth := 1
		j := i + 1
		for j < objEnd && depth > 0 {
			switch data[j] {
			case '[':
				depth++
			case ']':
				depth--
			}
			j++
		}
		return i, j, true
	}
	return 0, 0, false
}

// replaceFieldValue заменяет значение поля field на newValue.
func replaceFieldValue(data []byte, objStart, objEnd int, field, newValue string) ([]byte, bool) {
	start, end, ok := findFieldValue(data, objStart, objEnd, field)
	if !ok {
		return data, false
	}
	out := make([]byte, 0, len(data)-(end-start)+len(newValue))
	out = append(out, data[:start]...)
	out = append(out, newValue...)
	out = append(out, data[end:]...)
	return out, true
}

// insertField вставляет «"field": value» перед закрывающей } объекта расы
// (поля расы в families.json — с отступом 8 пробелов, как в файле).
func insertField(data []byte, closeBrace int, field, value string) []byte {
	ins := ",\n        \"" + field + "\": " + value
	out := make([]byte, 0, len(data)+len(ins))
	out = append(out, data[:closeBrace]...)
	out = append(out, ins...)
	out = append(out, data[closeBrace:]...)
	return out
}

// quotedTokens — JSON-строки токенов через ", " (стиль families.json:
// "blocked": ["pyramid", "obelisk", ...]).
func quotedTokens(tokens []string) string {
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = jsonString(t)
	}
	return strings.Join(parts, ", ")
}

// jsonString — JSON-кодированная строка (с кавычками и экранированием).
func jsonString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}