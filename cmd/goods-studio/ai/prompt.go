package ai

import (
	"fmt"
	"strings"

	"zorion/internal/goodsstudio/model"
)

// BuildPrompt собирает промпт «заполнить комплектующие» (спека 99a.1 §7.2,
// уточнение создателя 2026-09-18): товар N + уже заполненные слоты
// исключить + список бана/исключённых + известные ресурсы + категории.
// allowResourceSlots — 1-базовые позиции пустых слотов (по порядку пустых),
// для которых разрешены ресурсы (99a Пакет 4, п.8): с галкой ИИ может
// предложить ресурс, без галки — только товары.
func BuildPrompt(g *model.Good, categoryName string, tier int, filled []string, banned []string, resources []string, categories []string, k int, allowResourceSlots []int) string {
	var sb strings.Builder
	sb.WriteString("Система: ты — генератор производственных цепочек космической экономической игры.\n")
	sb.WriteString("Товар производится из составляющих (товаров или ресурсов). Отвечай строго JSON.\n\n")
	sb.WriteString("Пользователь:\n")
	fmt.Fprintf(&sb, "Товар: %s (категория: %s, тир: %d).\n", g.Name, categoryName, tier)
	sb.WriteString("Уже заполненные слоты (не предлагай их): ")
	sb.WriteString(joinOrDash(filled))
	sb.WriteString(".\n")
	sb.WriteString("Забаненные и исключённые (не предлагай): ")
	sb.WriteString(joinOrDash(banned))
	sb.WriteString(".\n")
	sb.WriteString("Известные ресурсы (если составляющая — существующий ресурс, используй его точное имя):\n")
	sb.WriteString(joinLines(resources))
	sb.WriteString("Категории студии (id: имя): ")
	sb.WriteString(strings.Join(categories, ", "))
	sb.WriteString(".\n")
	sb.WriteString("Поле category каждой составляющей — строго один из id или точных имён из списка категорий выше; не выдумывай новые категории.\n")
	fmt.Fprintf(&sb, "Пустых слотов: %d.\n", k)
	if len(allowResourceSlots) > 0 {
		fmt.Fprintf(&sb, "Пустые слоты, допускающие ресурсы (по порядку пустых слотов): %s.\n", joinOrDash(intsToStrings(allowResourceSlots)))
		sb.WriteString("Для этих слотов можно предлагать ресурсы из списка известных. Для остальных пустых слотов — только товары, НЕ ресурсы.\n")
	} else {
		sb.WriteString("Ни один пустой слот не допускает ресурсы — предлагай только товары, не ресурсы.\n")
	}
	fmt.Fprintf(&sb, "Верни ровно %d составляющих — по одному на каждый пустой слот, максимально полно описывающих состав товара на уровень ниже в графе.\n", k)
	sb.WriteString("JSON: {\"components\":[{\"name\":\"...\",\"category\":\"...\",\"reason\":\"...\"}]}")
	return sb.String()
}

func intsToStrings(items []int) []string {
	out := make([]string, len(items))
	for i, v := range items {
		out[i] = fmt.Sprintf("%d", v)
	}
	return out
}

func joinOrDash(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	return strings.Join(items, ", ")
}

func joinLines(items []string) string {
	if len(items) == 0 {
		return "—\n"
	}
	return strings.Join(items, "\n") + "\n"
}