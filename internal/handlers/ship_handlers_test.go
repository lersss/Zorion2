// internal/handlers/ship_handlers_test.go
// Тесты GET /api/ship-parts (спека 99.2.15 §5.1): каталог отдаётся из памяти
// (O(1), И7), формат — id/category/name/svg + палитра + порядок слоёв;
// пустой каталог — пустой список (фолбэк И4, без падений).
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// testPalette — палитра тестового каталога (содержимое не важно для ручки).
var testPalette = []string{"#cbd5e1", "#3b82f6", "#f472b6"}

// testLayerOrder — порядок слоёв (как в конфиге, спека §2).
var testLayerOrder = []string{"hull", "wings", "nose", "engine", "tail"}

// newShipCatalogHarness — каталог в памяти с заданными деталями.
func newShipCatalogHarness(parts []models.ShipPart) *repository.ShipCatalog {
	c := repository.NewShipCatalog(testPalette, testLayerOrder)
	c.Replace(parts, testPalette, testLayerOrder)
	return c
}

func TestGetCatalogFromMemory(t *testing.T) {
	parts := []models.ShipPart{
		{ID: "hull_a", Category: "hull", Name: "Стрела", SVG: `<path d="M0 0"/>`},
		{ID: "nose_b", Category: "nose", Name: "Клин", SVG: `<path d="M1 1"/>`},
	}
	h := NewShipHandlers(newShipCatalogHarness(parts))

	req := httptest.NewRequest(http.MethodGet, "/api/ship-parts", nil)
	rec := execJSON(h.GetCatalog, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Parts      []struct {
			ID       string `json:"id"`
			Category string `json:"category"`
			Name     string `json:"name"`
			SVG      string `json:"svg"`
			Params   *int   `json:"params"`
		} `json:"parts"`
		Palette    []string `json:"palette"`
		LayerOrder []string `json:"layerOrder"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Parts, 2)
	require.Equal(t, "hull_a", resp.Parts[0].ID)
	require.Equal(t, "Стрела", resp.Parts[0].Name)
	// Параметры генерации клиенту не отдаются (спека §5.1: id, category, name, svg).
	require.Nil(t, resp.Parts[0].Params)
	require.Equal(t, testPalette, resp.Palette)
	require.Equal(t, testLayerOrder, resp.LayerOrder)
}

func TestGetCatalogEmpty(t *testing.T) {
	h := NewShipHandlers(newShipCatalogHarness(nil))

	req := httptest.NewRequest(http.MethodGet, "/api/ship-parts", nil)
	rec := execJSON(h.GetCatalog, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Parts []json.RawMessage `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Empty(t, resp.Parts, "пустой каталог — пустой список (И4, фолбэк на клиенте)")
}