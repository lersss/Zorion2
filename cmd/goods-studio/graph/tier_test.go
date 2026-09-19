package graph

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/cmd/goods-studio/model"
)

// --- Эффективный тир (99a.3 §4.2): override ?? вычисленный; ресурс = 0 ---

func intPtr(v int) *int { return &v }

// TestEffectiveTierNoOverride — без оверрайда эффективный = вычисленный.
func TestEffectiveTierNoOverride(t *testing.T) {
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

// TestEffectiveTierOverride — оверрайд задан → эффективный = оверрайд
// (в т.ч. ниже вычисленного — полная свобода, решение создателя 2026-09-19).
func TestEffectiveTierOverride(t *testing.T) {
	goods := mkGoods(
		res("r1", "Железо"),
		good("g1", "Сталь", model.Slot{GoodID: "r1"}),
	)
	goods[1].TierOverride = intPtr(7)
	byID := ByID(goods)
	require.Equal(t, 7, EffectiveTier(&goods[1], byID))
	// ниже вычисленного — тоже оверрайд
	goods[1].TierOverride = intPtr(0)
	require.Equal(t, 0, EffectiveTier(&goods[1], byID))
}

// TestEffectiveTierResourceZero — ресурс всегда 0, даже если оверрайд
// проставлен в данных (API запрещает, но функция не должна падать).
func TestEffectiveTierResourceZero(t *testing.T) {
	goods := mkGoods(res("r1", "Железо"))
	goods[0].TierOverride = intPtr(5)
	require.Equal(t, 0, EffectiveTier(&goods[0], ByID(goods)))
}

// TestTierUnaffectedByOverride — вычисленный Tier не зависит от оверрайда
// (промпт ИИ, валидаторы, глубина — по вычисленному, 99a.3 §4.2).
func TestTierUnaffectedByOverride(t *testing.T) {
	goods := mkGoods(
		res("r1", "Железо"),
		good("g1", "Сталь", model.Slot{GoodID: "r1"}),
	)
	goods[1].TierOverride = intPtr(9)
	byID := ByID(goods)
	require.Equal(t, 1, Tier(&goods[1], byID))
}