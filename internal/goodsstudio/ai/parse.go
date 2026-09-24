package ai

import (
	"encoding/json"
	"strings"
)

// Component — одна составляющая из ответа ИИ (спека 99a.1 §7.3).
// Description — короткое игровое описание составляющей (спека
// 2026-09-21-каталог-описание §8.1), применяется только к kind=new (И5).
type Component struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Reason      string `json:"reason"`
	Description string `json:"description"`
}

// FillResponse — ответ ИИ: ровно столько товаров, сколько пустых слотов
// (спека 99a.1 §7.3, уточнение создателя 2026-09-18, дважды).
type FillResponse struct {
	Components []Component `json:"components"`
}

// extractJSON — выделение JSON из ответа ИИ: модель может добавить прозу
// вокруг JSON (приветствие, пояснения). Если прямой разбор не проходит:
// снимаем ```json-фенсы (если строка с них начинается), затем ищем последний
// сбалансированный JSON-объект, проходящий json.Valid.
//
// Прежний способ «от первого { до последнего }» ломался, когда модель сначала
// рассуждает и цитирует шаблон промпта (тоже валидный JSON): склейка
// «шаблон-цитата + проза + реальный ответ» не парсилась (баг 2026-09-24).
// Возвращает исходную (сжатую) строку, если JSON не найден, — Unmarshal вернёт
// ошибку сам.
func extractJSON(s string) string {
	trimmed := strings.TrimSpace(s)
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}
	stripped := trimmed
	if strings.HasPrefix(stripped, "```") {
		stripped = strings.TrimPrefix(stripped, "```")
		stripped = strings.TrimPrefix(stripped, "json")
		stripped = strings.TrimSpace(stripped)
		stripped = strings.TrimSuffix(stripped, "```")
		stripped = strings.TrimSpace(stripped)
	}
	if json.Valid([]byte(stripped)) {
		return stripped
	}
	if obj, ok := lastValidJSONObject(stripped); ok {
		return obj
	}
	// JSON не найден: отдаём исходную строку, Unmarshal вернёт ошибку сам.
	return stripped
}

// lastValidJSONObject возвращает последний сбалансированный JSON-объект,
// который проходит json.Valid. Скан по байтам ведёт стек открытых «{» и
// учитывает строки (кавычки/экранирование), поэтому фигурные скобки и кавычки
// внутри прозы не сбивают баланс. «Последний» — потому что настоящий ответ
// идёт после рассуждения и цитаты шаблона (идея 2026-09-24).
func lastValidJSONObject(s string) (string, bool) {
	var starts []int
	inString := false
	escaped := false
	found := ""
	ok := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			starts = append(starts, i)
		case '}':
			if len(starts) == 0 {
				continue
			}
			start := starts[len(starts)-1]
			starts = starts[:len(starts)-1]
			candidate := s[start : i+1]
			if json.Valid([]byte(candidate)) {
				found = candidate
				ok = true
			}
		}
	}
	return found, ok
}

// ParseFillResponse разбирает сырой текст ответа ИИ. Допускает прозу и обёртку
// в ```json ... ``` вокруг JSON (модели добавляют и то, и другое).
// Пустые имена отбрасываются.
func ParseFillResponse(raw string) ([]Component, error) {
	var resp FillResponse
	if err := json.Unmarshal([]byte(extractJSON(raw)), &resp); err != nil {
		return nil, err
	}
	comps := make([]Component, 0, len(resp.Components))
	for _, c := range resp.Components {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		comps = append(comps, c)
	}
	return comps, nil
}
