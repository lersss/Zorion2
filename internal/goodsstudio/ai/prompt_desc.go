package ai

import (
	"fmt"
	"strings"
)

// BuildDescriptionPrompt — промпт запроса описаний пачкой (спека
// 2026-09-21-каталог-описание §8.2): список записей с категорией и видом,
// требование короткого игрового описания без цен/чисел/статов.
func BuildDescriptionPrompt(items []DescTarget) string {
	var sb strings.Builder
	sb.WriteString("Система: ты — автор коротких игровых описаний предметов космической экономической игры. Отвечай строго JSON.\n\n")
	sb.WriteString("Пользователь:\n")
	sb.WriteString("Создай описание для каждой записи каталога.\n")
	for i, it := range items {
		fmt.Fprintf(&sb, "%d. %s (категория: %s, вид: %s)\n", i+1, it.Name, it.Category, descKindLabel(it.Kind))
	}
	sb.WriteString("Для каждой записи — 1–2 предложения (не более 240 символов): что это и зачем в игре, игровым языком, без цен, чисел, характеристик и названий брендов.\n")
	sb.WriteString(`JSON: {"items":[{"name":"…","description":"…"}]}`)
	return sb.String()
}

// descKindLabel — человекочитаемый вид записи для промпта описаний.
func descKindLabel(kind string) string {
	if kind == "resource" {
		return "ресурс"
	}
	return "товар"
}
