// internal/models/ship_sprites_test.go
// Тесты реестра статичных спрайтов кораблей (спека 61b §3.2, §4, §5.5):
// ровно 21 запись, файлы существуют, маппинг legacy → PNG — биекция,
// неизвестное имя → дефолт, палитра — 9 хроматических цветов.
package models

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Реестр содержит ровно 21 запись (спека §3.2, И1).
func TestShipSpritesRegistrySize(t *testing.T) {
	require.Len(t, ShipSprites, 21)
}

// Каждый file существует в web/static/sprites/ (этап 1, §3.1).
func TestShipSpritesFilesExist(t *testing.T) {
	for _, s := range ShipSprites {
		_, err := os.Stat("../../web/static/sprites/" + s.File)
		require.NoError(t, err, "спрайт %s должен существовать в web/static/sprites/", s.File)
	}
}

// DefaultShipIcon ∈ реестр (§4.1).
func TestDefaultShipIconInRegistry(t *testing.T) {
	require.True(t, IsValidShipIcon(DefaultShipIcon))
}

// Маппинг — биекция (И3): ровно 21 legacy-имя, все значения — имена из
// реестра, дублей значений нет.
func TestLegacyShipIconMapBijection(t *testing.T) {
	require.Len(t, LegacyShipIconMap, 21)
	seen := map[string]bool{}
	for legacy, png := range LegacyShipIconMap {
		require.True(t, IsValidShipIcon(png), "%s → %s: значение не из реестра", legacy, png)
		require.False(t, seen[png], "дубль значения %s", png)
		seen[png] = true
	}
}

// Неизвестное имя / пустое / битое → дефолт (§4).
func TestResolveShipIconUnknownToDefault(t *testing.T) {
	require.Equal(t, DefaultShipIcon, ResolveShipIcon("x.png"))
	require.Equal(t, DefaultShipIcon, ResolveShipIcon(""))
	require.Equal(t, DefaultShipIcon, ResolveShipIcon("ship_unknown"))
}

// Legacy-имя → PNG-имя; уже PNG-имя из реестра → как есть (§4).
func TestResolveShipIconMapping(t *testing.T) {
	require.Equal(t, "boomerang.png", ResolveShipIcon("ship_strela.svg"))
	require.Equal(t, "shark.png", ResolveShipIcon("ship_akula.svg"))
	require.Equal(t, "crescent.png", ResolveShipIcon("crescent.png"))
	require.Equal(t, "volcano.png", ResolveShipIcon("volcano.png"))
}

// Палитра — ровно 9 хроматических цветов (§5.5); вне палитры — невалидно.
func TestShipColorPalette(t *testing.T) {
	require.Len(t, ShipColorPalette, 9)
	for _, c := range ShipColorPalette {
		require.True(t, IsValidShipColor(c), "цвет %s должен быть в палитре", c)
	}
	require.False(t, IsValidShipColor("#000000"))
	require.False(t, IsValidShipColor("red"))
}