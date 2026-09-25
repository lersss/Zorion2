// internal/ship/accelerator_test.go
// Тесты модуля-ускорителя (спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md §3.1–§3.2, §9):
// монотонность и границы bonus(q), безопасный разбор params (М3), поиск модуля
// по детерминированному порядку слотов, стартовая комплектация и слот.
package ship

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

func TestAcceleratorBonusBounds(t *testing.T) {
	// q=0 → пол «участие без штрафа»; q=1 → bonus_max.
	b0, ok := AcceleratorBonus(0, 0.50)
	require.True(t, ok)
	require.InDelta(t, 0.10, b0, 1e-9)

	b1, ok := AcceleratorBonus(1, 0.50)
	require.True(t, ok)
	require.InDelta(t, 0.50, b1, 1e-9)

	// Верхняя граница bonus_max (М3): 1.0 → newRem = remaining/2 ровно (И-6).
	bmax, ok := AcceleratorBonus(1, 1.0)
	require.True(t, ok)
	require.InDelta(t, 1.0, bmax, 1e-9)
	require.InDelta(t, 0.5, 1.0/(1+bmax), 1e-9, "newRem = remaining/(1+bonus) не ниже половины")

	// Вне [0.10, 1.0] — нет выигрыша: ниже — немонотонность, выше — «телепорт».
	_, ok = AcceleratorBonus(0.5, 0.05)
	require.False(t, ok)
	_, ok = AcceleratorBonus(0.5, 1.5)
	require.False(t, ok)
}

func TestAcceleratorBonusMonotonic(t *testing.T) {
	prev := -1.0
	for i := 0; i <= 100; i++ {
		q := float64(i) / 100
		b, ok := AcceleratorBonus(q, 0.50)
		require.True(t, ok)
		require.GreaterOrEqual(t, b, prev, "bonus(q) не убывает при bonus_max ≥ bonus_min")
		prev = b
	}

	// Кламп q в [0,1].
	lo, _ := AcceleratorBonus(-1, 0.50)
	hi, _ := AcceleratorBonus(2, 0.50)
	require.InDelta(t, 0.10, lo, 1e-9)
	require.InDelta(t, 0.50, hi, 1e-9)

	// Пол: даже при bonus_max = bonus_min бонус не ниже 10 %.
	floor, ok := AcceleratorBonus(0, AcceleratorBonusMin)
	require.True(t, ok)
	require.InDelta(t, 0.10, floor, 1e-9)
}

func TestParseAcceleratorParams(t *testing.T) {
	good := map[string]interface{}{
		"game": "route", "cooldown_min": float64(25), "bonus_max": float64(0.5),
		"min_remaining_offer_s": float64(180), "min_remaining_boost_s": float64(90),
	}
	cfg, ok := parseAcceleratorParams(good)
	require.True(t, ok)
	require.Equal(t, AcceleratorGameRoute, cfg.Game)
	require.Equal(t, 25, cfg.CooldownMin)
	require.InDelta(t, 0.5, cfg.BonusMax, 1e-9)
	require.Equal(t, 180, cfg.MinRemainingOfferS)
	require.Equal(t, 90, cfg.MinRemainingBoostS)

	// JSON-целые (как в миграции) — тоже валидны (numParam через int).
	ints := map[string]interface{}{
		"game": "route", "cooldown_min": 25, "bonus_max": 0.5,
		"min_remaining_offer_s": 180, "min_remaining_boost_s": 90,
	}
	_, ok = parseAcceleratorParams(ints)
	require.True(t, ok)

	for name, bad := range map[string]map[string]interface{}{
		"nil":     nil,
		"no_game": {"cooldown_min": 25.0, "bonus_max": 0.5, "min_remaining_offer_s": 180.0, "min_remaining_boost_s": 90.0},
		"bonus_low": {"game": "route", "cooldown_min": 25.0, "bonus_max": 0.05,
			"min_remaining_offer_s": 180.0, "min_remaining_boost_s": 90.0},
		"bonus_teleport": {"game": "route", "cooldown_min": 25.0, "bonus_max": 1.5,
			"min_remaining_offer_s": 180.0, "min_remaining_boost_s": 90.0},
		"bonus_string": {"game": "route", "cooldown_min": 25.0, "bonus_max": "0.5",
			"min_remaining_offer_s": 180.0, "min_remaining_boost_s": 90.0},
		"no_ports": {"game": "route", "cooldown_min": 25.0, "bonus_max": 0.5},
	} {
		_, ok := parseAcceleratorParams(bad)
		require.Falsef(t, ok, "params %q должны быть невалидны", name)
	}
}

func TestAcceleratorModule(t *testing.T) {
	LoadDefaults()

	// Не установлен.
	_, _, found, _ := AcceleratorModule(map[string]interface{}{"radar": "radar_1"})
	require.False(t, found)

	// Выделенный слот (решение 10).
	id, cfg, found, valid := AcceleratorModule(map[string]interface{}{"accelerator": "accel_1"})
	require.True(t, found)
	require.True(t, valid)
	require.Equal(t, "accel_1", id)
	require.Equal(t, AcceleratorGameRoute, cfg.Game)
	require.InDelta(t, 0.5, cfg.BonusMax, 1e-9)

	// Универсальный слот: эффект работает по типу из любого слота (§3.2).
	id, _, found, valid = AcceleratorModule(map[string]interface{}{"universal": "accel_1"})
	require.True(t, found)
	require.True(t, valid)
	require.Equal(t, "accel_1", id)

	// Чужой тип / неизвестный id — модуля нет.
	_, _, found, _ = AcceleratorModule(map[string]interface{}{"accelerator": "radar_1"})
	require.False(t, found)
	_, _, found, _ = AcceleratorModule(map[string]interface{}{"accelerator": "nope"})
	require.False(t, found)
}

func TestStarterEquipmentHasAccelerator(t *testing.T) {
	require.Equal(t, "accel_1", models.StarterAcceleratorID)
	require.Equal(t, models.StarterAcceleratorID, models.StarterEquipment["accelerator"])
}

func TestShipModelDefaultAcceleratorSlot(t *testing.T) {
	LoadModelDefaults()
	m := ShipModelByID(models.StarterShipModelID)
	require.NotNil(t, m)
	require.Equal(t, float64(1), m.Slots["accelerator"], "выделенный слот accelerator (решение 10)")
}
