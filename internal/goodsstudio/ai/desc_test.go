package ai

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseDescriptionResponseReasoningQuotedTemplate — реальный проблемный
// RAW живого прогона 2026-09-24: модель рассуждает, цитирует шаблон промпта
// (тоже валидный JSON) и лишь затем отдаёт настоящий ответ. «Первый { … последний }»
// склеивал шаблон-цитату + прозу + ответ → мусор. Разбор обязан взять валидный
// объект ответа (фикстура — точная копия %TEMP%\opencode\raw-text.txt).
func TestParseDescriptionResponseReasoningQuotedTemplate(t *testing.T) {
	raw, err := os.ReadFile("testdata/problem_desc_raw.txt")
	require.NoError(t, err)

	items, err := ParseDescriptionResponse(string(raw))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "метан-ресурс", items[0].Name)
	require.Contains(t, items[0].Description, "Ледяные месторождения метана")
}

// TestParseDescriptionResponseValid — валидный JSON разбирается.
func TestParseDescriptionResponseValid(t *testing.T) {
	items, err := ParseDescriptionResponse(`{"items":[{"name":"Сталь","description":"Прочный сплав."}]}`)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Сталь", items[0].Name)
	require.Equal(t, "Прочный сплав.", items[0].Description)
}

// TestParseDescriptionResponseCodeFence — обёртка ```json ... ``` снимается.
func TestParseDescriptionResponseCodeFence(t *testing.T) {
	items, err := ParseDescriptionResponse("```json\n{\"items\":[{\"name\":\"Сталь\",\"description\":\"Сплав.\"}]}\n```")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Сталь", items[0].Name)
}

// TestParseDescriptionResponseGarbage — мусор → ошибка.
func TestParseDescriptionResponseGarbage(t *testing.T) {
	_, err := ParseDescriptionResponse("это не json вообще")
	require.Error(t, err)
}

// TestParseDescriptionResponseProse — проза вокруг ```json-блока (баг
// 2026-09-21: пустой agent → default_agent менеджера) → JSON извлекается.
func TestParseDescriptionResponseProse(t *testing.T) {
	raw := "Здравствуйте! Как менеджер, предлагаю описания:\n```json\n" +
		`{"items":[{"name":"Сталь","description":"Прочный сплав."}]}` +
		"\n```\nГотов уточнить."
	items, err := ParseDescriptionResponse(raw)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Сталь", items[0].Name)
	require.Equal(t, "Прочный сплав.", items[0].Description)
}

// TestParseDescriptionResponseProseBare — проза вокруг «голого» JSON → извлекается.
func TestParseDescriptionResponseProseBare(t *testing.T) {
	raw := `Вот: {"items":[{"name":"Железо Fe","description":"Металл."}]} — проверьте.`
	items, err := ParseDescriptionResponse(raw)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Железо Fe", items[0].Name)
}

// TestParseDescriptionResponseNoJSON — проза без JSON → ошибка.
func TestParseDescriptionResponseNoJSON(t *testing.T) {
	_, err := ParseDescriptionResponse("никакого JSON здесь нет")
	require.Error(t, err)
}

// TestParseDescriptionResponseEmptyDropped — пустые name/description отбрасываются.
func TestParseDescriptionResponseEmptyDropped(t *testing.T) {
	items, err := ParseDescriptionResponse(`{"items":[
		{"name":"  ","description":"x"},
		{"name":"Сталь","description":"  "},
		{"name":"Топливо","description":"Горит."}
	]}`)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Топливо", items[0].Name)
}

// TestNormalizeDescriptionTrim — trim по краям.
func TestNormalizeDescriptionTrim(t *testing.T) {
	require.Equal(t, "текст", NormalizeDescription("  текст\n"))
}

// TestNormalizeDescriptionRuneCap — обрезка по рунам до MaxDescriptionRunes
// на кириллице (2001 руна → 2000).
func TestNormalizeDescriptionRuneCap(t *testing.T) {
	long := strings.Repeat("я", MaxDescriptionRunes+1)
	got := NormalizeDescription(long)
	require.Equal(t, MaxDescriptionRunes, len([]rune(got)))
}

// TestNormalizeDescriptionShortUnchanged — короткий текст не меняется.
func TestNormalizeDescriptionShortUnchanged(t *testing.T) {
	require.Equal(t, "короткий", NormalizeDescription("короткий"))
}
