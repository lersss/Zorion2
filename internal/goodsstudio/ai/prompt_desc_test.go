package ai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDescriptionPromptContents — промпт несёт имя/категорию/вид каждой записи,
// требование «не более 240 символов» и ключи items/name/description.
func TestDescriptionPromptContents(t *testing.T) {
	p := BuildDescriptionPrompt([]DescTarget{
		{Name: "Крейсер", Category: "Корабли", Kind: "good"},
		{Name: "Железо Fe", Category: "Минералы", Kind: "resource"},
	})
	require.Contains(t, p, "Крейсер")
	require.Contains(t, p, "Корабли")
	require.Contains(t, p, "Железо Fe")
	require.Contains(t, p, "Минералы")
	require.Contains(t, p, "вид: товар")
	require.Contains(t, p, "вид: ресурс")
	require.Contains(t, p, "не более 240 символов")
	require.Contains(t, p, `"items"`)
	require.Contains(t, p, `"name"`)
	require.Contains(t, p, `"description"`)
}
