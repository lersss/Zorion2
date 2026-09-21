package ai

import (
	"encoding/json"
	"strings"
)

// MaxDescriptionRunes — предел длины описания (И9): ручной ввод длиннее — 400,
// ИИ-текст обрезается по рунам (предел защищает хранение/UI, не смысл).
const MaxDescriptionRunes = 2000

// DescTarget — запись каталога, для которой просим описание.
type DescTarget struct {
	Name     string
	Category string // имя категории (для ресурса — системная ресурсная категория)
	Kind     string // "good" | "resource"
}

// DescItem — разобранное предложение ИИ.
type DescItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DescResponse — ответ ИИ на запрос описаний.
type DescResponse struct {
	Items []DescItem `json:"items"`
}

// ParseDescriptionResponse — разбор ответа ИИ (как ParseFillResponse:
// допускает прозу и ```json-обёртку вокруг JSON; пустые name/description
// отбрасываются).
func ParseDescriptionResponse(raw string) ([]DescItem, error) {
	var resp DescResponse
	if err := json.Unmarshal([]byte(extractJSON(raw)), &resp); err != nil {
		return nil, err
	}
	items := make([]DescItem, 0, len(resp.Items))
	for _, it := range resp.Items {
		if strings.TrimSpace(it.Name) == "" || strings.TrimSpace(it.Description) == "" {
			continue
		}
		items = append(items, it)
	}
	return items, nil
}

// NormalizeDescription — trim + обрезка по рунам до MaxDescriptionRunes.
func NormalizeDescription(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > MaxDescriptionRunes {
		r = r[:MaxDescriptionRunes]
	}
	return string(r)
}
