// internal/handlers/region_handler_test.go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/races"
)

func TestGetRegionsHandler(t *testing.T) {
	require.NoError(t, races.LoadCatalog("../../config/races.json"))
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "name", "center_x", "center_y", "radius", "color", "world_count", "profile", "race_id"}).
		AddRow("r1", "Астерия", 100.0, 200.0, 1200.0, "#7c6cff", 45, "young", "sulfur_nests").
		AddRow("r2", "Сектор Ксирон", -50.0, -80.0, 1200.0, "#ff6b9d", 60, "", "")

	mock.ExpectQuery(`(?i)SELECT id, name, center_x, center_y, radius, color, world_count, COALESCE\(profile, ''\), COALESCE\(race_id, ''\) FROM regions ORDER BY name`).
		WillReturnRows(rows)

	h := &AdminHandlers{db: db}
	req := httptest.NewRequest(http.MethodGet, "/api/regions", nil)
	rec := httptest.NewRecorder()

	h.GetRegionsHandler(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NoError(t, mock.ExpectationsWereMet())

	var regions []regionDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regions))
	require.Len(t, regions, 2)
	assert.Equal(t, "Астерия", regions[0].Name)
	assert.Equal(t, 1200.0, regions[0].Radius)
	assert.Equal(t, 45, regions[0].WorldCount)
	assert.Equal(t, "young", regions[0].Profile, "профиль региона приходит в DTO (отладочно, 59a)")
	assert.Equal(t, "Серные гнёзда", regions[0].RaceName, "имя расы из каталога (отладочно, расы)")
	assert.Equal(t, "Сектор Ксирон", regions[1].Name)
	assert.Equal(t, "", regions[1].Profile, "NULL-профиль → пустая строка")
	assert.Equal(t, "", regions[1].RaceName, "NULL race_id → пустая строка")
}