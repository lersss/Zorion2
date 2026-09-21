// internal/models/ship_sprites_test.go
// Тесты реестра статичных спрайтов кораблей (спека 61b §3.2, §4, §5.5):
// 24 записи (21 базовый + 3 расовых «люди»), файлы существуют, расовая проба
// только в хвосте, маппинг legacy → PNG — биекция, неизвестное имя → дефолт,
// палитра — 9 хроматических цветов.
package models

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Реестр содержит ровно 24 записи: 21 базовый (спека §3.2, И1) + 3 расовых
// корабля «люди» (проба 2026-09-21).
func TestShipSpritesRegistrySize(t *testing.T) {
	require.Len(t, ShipSprites, 24)
}

// Расовая проба добавлена В КОНЕЦ реестра (И8): индексы первых 21 не
// сдвигаются — spriteForAgent(id) = ShipSprites[FNV-1a(id) % len(reestr)].
func TestShipSpritesRacialAppended(t *testing.T) {
	tail := []ShipSprite{
		{ID: "race_humans_starship", Name: "Звёздный корабль (люди)", File: "race_humans_starship.png"},
		{ID: "race_humans_cruiser", Name: "Крейсер (люди)", File: "race_humans_cruiser.png"},
		{ID: "race_humans_carrier", Name: "Носитель (люди)", File: "race_humans_carrier.png"},
	}
	require.Len(t, ShipSprites, 21+len(tail))
	require.Equal(t, tail, ShipSprites[21:])
	for _, s := range tail {
		require.True(t, IsValidShipIcon(s.File), "%s должен быть в реестре", s.File)
	}
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