// internal/ship/catalog_test.go
// Каталог оборудования и радиус радара (спека 77a §3/§4.2): радиус
// определяется ТОЛЬКО установленным радаром (И4); без радара — минимум 200 px.
package ship

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// loadTestCatalog — загружает каталог из sqlmock-БД (radar_1 + scanner_1).
func loadTestCatalog(t *testing.T) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, type, name, params FROM equipment`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "type", "name", "params"}).
			AddRow("radar_1", "radar", "Радар-1", `{"radius":800}`).
			AddRow("scanner_1", "scanner", "Сканер-1", `{"depth":"surface","settlements":true}`))

	require.NoError(t, LoadCatalog(db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRadarRadius(t *testing.T) {
	loadTestCatalog(t)

	tests := []struct {
		name      string
		equipment map[string]interface{}
		want      float64
	}{
		{
			name:      "стартовый радар radar_1 → 800 px",
			equipment: map[string]interface{}{"radar": "radar_1", "scanner": "scanner_1", "engine": nil},
			want:      models.RadarRadiusDefault,
		},
		{
			name:      "без радара → минимум 200 px",
			equipment: map[string]interface{}{"radar": nil, "scanner": "scanner_1", "engine": nil},
			want:      models.RadarRadiusMin,
		},
		{
			name:      "пустое оборудование → минимум 200 px",
			equipment: map[string]interface{}{},
			want:      models.RadarRadiusMin,
		},
		{
			name:      "nil оборудование → минимум 200 px",
			equipment: nil,
			want:      models.RadarRadiusMin,
		},
		{
			name:      "неизвестный радар → минимум 200 px (безопасный фолбэк)",
			equipment: map[string]interface{}{"radar": "radar_99"},
			want:      models.RadarRadiusMin,
		},
		{
			name:      "в слоте радара сканер → минимум 200 px (тип не совпадает)",
			equipment: map[string]interface{}{"radar": "scanner_1"},
			want:      models.RadarRadiusMin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, RadarRadius(tt.equipment))
		})
	}
}

func TestRadarRadiusDefaults(t *testing.T) {
	// Дефолты-страховка (пустая БД): radar_1 → 800 px.
	LoadDefaults()

	require.Equal(t, models.RadarRadiusDefault,
		RadarRadius(map[string]interface{}{"radar": "radar_1", "scanner": "scanner_1", "engine": nil}),
		"radar_1 из дефолтов → 800 px")
	require.Equal(t, models.RadarRadiusMin,
		RadarRadius(map[string]interface{}{"radar": nil}),
		"без радара → минимум 200 px")
}

func TestHasScanner(t *testing.T) {
	loadTestCatalog(t)

	require.True(t, HasScanner(map[string]interface{}{"scanner": "scanner_1"}),
		"установленный сканер → true")
	require.False(t, HasScanner(map[string]interface{}{"scanner": nil}),
		"пустой слот сканера → false")
	require.False(t, HasScanner(map[string]interface{}{}),
		"нет слота сканера → false")
	require.False(t, HasScanner(map[string]interface{}{"scanner": "radar_1"}),
		"в слоте сканера радар → false (тип не совпадает)")
	require.False(t, HasScanner(nil), "nil оборудование → false")
}

func TestEquipmentByID(t *testing.T) {
	loadTestCatalog(t)

	it := EquipmentByID("radar_1")
	require.NotNil(t, it)
	require.Equal(t, models.EquipmentTypeRadar, it.Type)
	require.Equal(t, float64(800), it.Params["radius"])

	require.Nil(t, EquipmentByID("nope"), "неизвестный id → nil")
}