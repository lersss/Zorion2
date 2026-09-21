package ai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/model"
)

// TestFillPromptUsesComputedTier — промпт «заполнить комплектующие» получает
// ВЫЧИСЛЕННЫЙ тир: сложность рецепта не влияет на генерацию (99a.3 §4.2,
// критик №10; спека 2026-09-21-рецепт-сущность §4.4).
func TestFillPromptUsesComputedTier(t *testing.T) {
	st := &model.State{
		SchemaVersion: 1,
		Categories:    []model.Category{{ID: "1", Name: "Корабли", Kind: model.KindGood}},
		Goods: []model.Good{
			{ID: "g1", Name: "A", Category: "1", Kind: model.KindGood, Recipe: []model.Slot{}},
			{ID: "g2", Name: "B", Category: "1", Kind: model.KindGood, Recipe: []model.Slot{{GoodID: "g1"}}},
		},
	}
	// сложность 9 на B — вычисленный тир B = 1 (A с пустым рецептом = 0)
	tier := 9
	st.Goods[1].Complexity = &tier
	prompt := BuildFillPrompt(st, "g2")
	require.Contains(t, prompt, "тир: 1")
	require.NotContains(t, prompt, "тир: 9")
}

// TestFillPromptContents — промпт несёт заполненные слоты, ресурсы,
// категории ("id: имя"), число пустых слотов и слоты с галкой ресурса.
// Строки про бан/исключённых нет (скрытия у товаров нет, 2026-09-21).
func TestFillPromptContents(t *testing.T) {
	st := &model.State{
		SchemaVersion: 1,
		Categories: []model.Category{
			{ID: "1", Name: "Корабли", Kind: model.KindGood},
			{ID: "7", Name: "Минералы", Kind: model.KindResource},
		},
		Goods: []model.Good{
			{ID: "1", Name: "Корабль", Category: "1", Kind: model.KindGood,
				Recipe: []model.Slot{{GoodID: "2"}, {}, {AllowResource: true}}},
			{ID: "2", Name: "Сталь", Category: "1", Kind: model.KindGood},
			{ID: "4", Name: "Железо Fe", Category: "7", Kind: model.KindResource},
		},
	}
	prompt := BuildFillPrompt(st, "1")
	require.Contains(t, prompt, "Сталь")      // заполненный слот
	require.Contains(t, prompt, "Железо Fe")  // ресурсы
	require.Contains(t, prompt, "1: Корабли") // категории "id: имя"
	require.Contains(t, prompt, "7: Минералы")
	require.Contains(t, prompt, "Пустых слотов: 2")
	require.Contains(t, prompt, "допускающие ресурсы")
	require.Contains(t, prompt, "2") // 1-базовая позиция пустого слота с галкой
	require.NotContains(t, prompt, "Забаненные")
}