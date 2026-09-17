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

	require.Nil(t, ShipModelByID("nope"), "неизвестная модель → nil")
}