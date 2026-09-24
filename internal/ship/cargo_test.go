// internal/ship/cargo_test.go
// Ёмкость трюма (спека трюма §4/§14): врождённая ёмкость модели корабля +
// сумма params.capacity установленных грузовых модулей (type=cargo).
// Безопасное чтение при битом params — только врождённая (не бесконечность).
package ship

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// loadTestCatalogCargo — каталог с грузовым модулем cargo_1 (30 т) и модулем
// с битым params (null) для проверки безопасного чтения.
func loadTestCatalogCargo(t *testing.T) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, type, name, params FROM equipment`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "type", "name", "params"}).
			AddRow("radar_1", "radar", "Радар-1", `{"radius":800}`).
			AddRow("scanner_1", "scanner", "Сканер-1", `{"depth":"surface","settlements":true}`).
			AddRow("engine_1", "engine", "Двигатель-1", `{"speed_factor":0.3}`).
			AddRow("cargo_1", "cargo", "Грузовой модуль-1", `{"capacity":30}`).
			AddRow("cargo_bad", "cargo", "Грузовой модуль-битый", `null`))

	require.NoError(t, LoadCatalog(db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCargoCapacityMass(t *testing.T) {
	loadTestCatalogCargo(t)
	LoadModelDefaults() // starter: base_capacity 20

	tests := []struct {
		name      string
		modelID   string
		equipment map[string]interface{}
		want      float64
	}{
		{
			name:      "без модуля → только врождённая 20 т",
			modelID:   models.StarterShipModelID,
			equipment: nil,
			want:      20,
		},
		{
			name:      "пустое оборудование → 20 т",
			modelID:   models.StarterShipModelID,
			equipment: map[string]interface{}{},
			want:      20,
		},
		{
			name:      "стартовый модуль cargo_1 → 20 + 30 = 50 т",
			modelID:   models.StarterShipModelID,
			equipment: map[string]interface{}{"universal": "cargo_1"},
			want:      50,
		},
		{
			name:      "universal2/universal3 = null (пустой слот) → 50 т",
			modelID:   models.StarterShipModelID,
			equipment: map[string]interface{}{"universal": "cargo_1", "universal2": nil, "universal3": nil},
			want:      50,
		},
		{
			name:      "второй модуль в universal2 → 20 + 30 + 30 = 80 т",
			modelID:   models.StarterShipModelID,
			equipment: map[string]interface{}{"universal": "cargo_1", "universal2": "cargo_1"},
			want:      80,
		},
		{
			name:      "третий модуль в universal3 → 20 + 30 + 30 + 30 = 110 т",
			modelID:   models.StarterShipModelID,
			equipment: map[string]interface{}{"universal": "cargo_1", "universal2": "cargo_1", "universal3": "cargo_1"},
			want:      110,
		},
		{
			name:      "не-cargo модуль в universal2 → не учитывается, 50 т",
			modelID:   models.StarterShipModelID,
			equipment: map[string]interface{}{"universal": "cargo_1", "universal2": "radar_1"},
			want:      50,
		},
		{
			name:      "битый params модуля → только врождённая 20 т",
			modelID:   models.StarterShipModelID,
			equipment: map[string]interface{}{"universal": "cargo_bad"},
			want:      20,
		},
		{
			name:      "неизвестный модуль → только врождённая 20 т",
			modelID:   models.StarterShipModelID,
			equipment: map[string]interface{}{"universal": "cargo_99"},
			want:      20,
		},
		{
			name:      "не-cargo модуль в слоте → не учитывается, 20 т",
			modelID:   models.StarterShipModelID,
			equipment: map[string]interface{}{"universal": "radar_1"},
			want:      20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CargoCapacityMass(tt.modelID, tt.equipment))
		})
	}
}

func TestCargoCapacityMassDefaults(t *testing.T) {
	// Дефолты-страховка (пустая БД): starter 20 + cargo_1 30 = 50 т.
	LoadDefaults()
	LoadModelDefaults()

	require.Equal(t, float64(50), CargoCapacityMass(models.StarterShipModelID,
		map[string]interface{}{"universal": models.StarterCargoModuleID}),
		"стартовый модуль из дефолтов → 50 т")
	require.Equal(t, float64(20), CargoCapacityMass(models.StarterShipModelID, nil),
		"без модуля → врождённая 20 т")
}
