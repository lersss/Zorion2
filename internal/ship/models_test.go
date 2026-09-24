// internal/ship/models_test.go
// Модели кораблей (спека 91a §7.3): ShipModelByID из дефолтов — стартовая
// модель 'starter' с именем «Стартовый разведчик» и слотами радар/сканер/двигатель.
package ship

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

func TestShipModelByIDDefaults(t *testing.T) {
	// Дефолты-страховка (пустая БД): starter (PITFALLS.md:185 — тест
	// справочника обязан сначала вызвать LoadDefaults).
	LoadModelDefaults()

	m := ShipModelByID(models.StarterShipModelID)
	require.NotNil(t, m, "стартовая модель из дефолтов")
	require.Equal(t, "Стартовый разведчик", m.Name)
	require.Equal(t, float64(1), m.Slots["radar"])
	require.Equal(t, float64(1), m.Slots["scanner"])
	require.Equal(t, float64(1), m.Slots["engine"])
	require.Equal(t, float64(3), m.Slots["universal"], "три универсальных слота (спека трюма §20)")
	require.Equal(t, float64(20), m.BaseCapacity, "врождённая ёмкость 20 т (спека трюма §5)")

	require.Nil(t, ShipModelByID("nope"), "неизвестная модель → nil")
}

// Свежая регистрация (спека трюма §14 п.9): стартовая комплектация несёт
// грузовой модуль в универсальном слоте.
func TestStarterEquipmentHasCargoModule(t *testing.T) {
	require.Equal(t, "cargo_1", models.StarterCargoModuleID)
	require.Equal(t, models.StarterCargoModuleID, models.StarterEquipment["universal"])
}
