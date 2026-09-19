package ai

import (
	"encoding/json"
	"strings"
)

// Component — одна составляющая из ответа ИИ (спека 99a.1 §7.3).
type Component struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

// FillResponse — ответ ИИ: ровно столько товаров, сколько пустых слотов
// (спека 99a.1 §7.3, уточнение создателя 2026-09-18, дважды).
type FillResponse struct {
	Components []Component `json:"components"`
}

// ParseFillResponse разбирает сырой текст ответа ИИ. Допускает обёртку
// в ```json ... ``` (модели часто оборачивают JSON в код-фенсы).
// Пустые имена отбрасываются.
func ParseFillResponse(raw string) ([]Component, error) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimSpace(s)
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	var resp FillResponse
	if err := json.Unmarshal([]byte(s), &resp); err != nil {
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