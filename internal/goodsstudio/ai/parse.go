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
// снимаем ```json-фенсы (если строка с них начинается), затем берём подстроку
// от первого «{» до последнего «}». Возвращает исходную (сжатую) строку, если
// JSON не найден, — Unmarshal вернёт ошибку сам.
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
	start := strings.IndexByte(stripped, '{')
	end := strings.LastIndexByte(stripped, '}')
	if start >= 0 && end > start {
		return stripped[start : end+1]
	}
	return stripped
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
