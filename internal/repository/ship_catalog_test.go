// internal/repository/ship_catalog_test.go
// Тесты каталога деталей в памяти (спека §3.4): immutable snapshot +
// atomic.Pointer; Part/PartsByCategory/DefaultPart/AssemblyFromSeed;
// Replace после генерации; пустой каталог → фолбэк.
package repository

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/generator/ship"
	"zorion/internal/models"
)

// testCatalogParts — каталог из генератора (10 форм на категорию).
func testCatalogParts(t *testing.T) []models.ShipPart {
	t.Helper()
	_, err := ship.LoadConfig("../../config/ship_visual.json")
	require.NoError(t, err)
	var parts []models.ShipPart
	for _, cat := range ship.Categories {
		for seed := int64(0); seed < 10; seed++ {
			p, err := ship.GeneratePart(cat, seed)
			require.NoError(t, err)
			parts = append(parts, models.ShipPart{ID: p.ID, Category: cat, Name: p.Name, SVG: p.SVG})
		}
	}
	return parts
}

func TestShipCatalogReplaceAndLookup(t *testing.T) {
	parts := testCatalogParts(t)
	c := NewShipCatalog([]string{"#3b82f6"}, []string{"hull", "wings", "nose", "engine", "tail"})
	snap := c.Snapshot()
	require.True(t, snap.Empty(), "до загрузки каталог пуст")

	c.Replace(parts, snap.Palette(), snap.LayerOrder())
	snap = c.Snapshot()
	require.False(t, snap.Empty())
	require.Len(t, snap.Parts(), len(parts))

	for _, p := range parts {
		got, ok := snap.Part(p.ID)
		require.True(t, ok, "деталь %s в каталоге", p.ID)
		require.Equal(t, p.ID, got.ID)
	}
	for _, cat := range ship.Categories {
		list := snap.PartsByCategory(cat)
		require.Len(t, list, 10, "категория %s: 10 форм", cat)
		require.Equal(t, list[0].ID, func() string {
			d, ok := snap.DefaultPart(cat)
			require.True(t, ok)
			return d.ID
		}(), "дефолт = первая деталь категории")
	}
}

func TestShipCatalogAssembly(t *testing.T) {
	parts := testCatalogParts(t)
	c := NewShipCatalog([]string{"#3b82f6"}, []string{"hull", "wings", "nose", "engine", "tail"})
	c.Replace(parts, c.Snapshot().Palette(), c.Snapshot().LayerOrder())

	v1 := c.Snapshot().AssemblyFromSeed([]byte("agent-1"))
	v2 := c.Snapshot().AssemblyFromSeed([]byte("agent-1"))
	require.Equal(t, v1, v2, "сборка детерминирована")
	require.Len(t, v1.Parts, 5)
	for _, cat := range ship.Categories {
		_, ok := c.Snapshot().Part(v1.Parts[cat])
		require.True(t, ok, "деталь %s схемы ∈ каталог", cat)
	}
}

func TestShipCatalogEmptyFallback(t *testing.T) {
	c := NewShipCatalog([]string{"#3b82f6"}, []string{"hull", "wings", "nose", "engine", "tail"})
	_, ok := c.Snapshot().Part("anything")
	require.False(t, ok, "пустой каталог → детали нет (фолбэк И4)")
	require.Len(t, c.Snapshot().PartsByCategory("hull"), 0)
}