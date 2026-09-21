package graph

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/model"
)

// --- Эффективный тир (99a.3 §4.2 + спека 2026-09-21-рецепт-сущность §2.3):
// complexity ?? вычисленный; ресурс = 0 ---

func intPtr(v int) *int { return &v }

// TestEffectiveTierNoComplexity — без сложности эффективный = вычисленный.
func TestEffectiveTierNoComplexity(t *testing.T) {
	goods := mkGoods(
		res("r1", "Железо"),
		good("g1", "Сталь", model.Slot{GoodID: "r1"}),
		good("g2", "Каркас", model.Slot{GoodID: "g1"}),
	)
	byID := ByID(goods)
	require.Equal(t, 0, EffectiveTier(&goods[0], byID))
	require.Equal(t, 1, EffectiveTier(&goods[1], byID))
	require.Equal(t, 2, EffectiveTier(&goods[2], byID))
}

// TestEffectiveTierComplexity — сложность рецепта задана → эффективный =
// сложность (в т.ч. ниже вычисленного — сложность не гейт, спека фабрик §8.2).
func TestEffectiveTierComplexity(t *testing.T) {
	goods := mkGoods(
		res("r1", "Железо"),
		good("g1", "Сталь", model.Slot{GoodID: "r1"}),
	)
	goods[1].Complexity = intPtr(7)
	byID := ByID(goods)
	require.Equal(t, 7, EffectiveTier(&goods[1], byID))
	// ниже вычисленного — тоже сложность
	goods[1].Complexity = intPtr(0)
	require.Equal(t, 0, EffectiveTier(&goods[1], byID))
}

// TestEffectiveTierResource — у ресурса рецепта нет (и сложности нет):
// эффективный = 0, даже если поле задано (защита от неверного состояния).
func TestEffectiveTierResource(t *testing.T) {
	goods := mkGoods(res("r1", "Железо"))
	byID := ByID(goods)
	require.Equal(t, 0, EffectiveTier(&goods[0], byID))
	require.Equal(t, 0, Tier(&goods[0], byID))
	goods[0].Complexity = intPtr(3)
	require.Equal(t, 0, EffectiveTier(&goods[0], byID), "ресурс = 0 — рецепта нет")
}

// TestTierUnaffectedByComplexity — вычисленный Tier не зависит от сложности
// рецепта (промпт ИИ, валидаторы, глубина — по вычисленному, §4.4/§5).
func TestTierUnaffectedByComplexity(t *testing.T) {
	goods := mkGoods(
		res("r1", "Железо"),
		good("g1", "Сталь", model.Slot{GoodID: "r1"}),
	)
	goods[1].Complexity = intPtr(9)
	byID := ByID(goods)
	require.Equal(t, 1, Tier(&goods[1], byID))
}
