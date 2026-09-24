// internal/models/ship_sprites_test.go
// Тесты общих резолверов визуала кораблей (спека 61b §4/§5.5, спека
// 2026-09-23 §8.1): неизвестное имя → дефолт, имя реестра → как есть, раса
// файла, палитра 9 цветов, поза по имени. Реестр и его состав/порядок —
// race_ship_sprites_test.go.
package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Неизвестное имя / пустое / битое / легаси → людской корабль (спека
// 2026-09-23 §8.1: правило упрощено, легаси-маппинг при чтении не применяется).
func TestResolveShipIconUnknownToDefaultHuman(t *testing.T) {
	require.Equal(t, DefaultHumanShip, ResolveShipIcon("x.png"))
	require.Equal(t, DefaultHumanShip, ResolveShipIcon(""))
	require.Equal(t, DefaultHumanShip, ResolveShipIcon("ship_unknown"))
	require.Equal(t, DefaultHumanShip, ResolveShipIcon("ship_strela.svg"))
	require.Equal(t, DefaultHumanShip, ResolveShipIcon("boomerang.png"))
}

// Имя из расового реестра (включая нейтральный) → как есть (спека §8.1, N2).
func TestResolveShipIconRegistryPassthrough(t *testing.T) {
	require.Equal(t, "race_humans_starship.png", ResolveShipIcon("race_humans_starship.png"))
	require.Equal(t, "race_humans_cruiser_03.png", ResolveShipIcon("race_humans_cruiser_03.png"))
	require.Equal(t, "neutral.png", ResolveShipIcon("neutral.png"))
}

// ShipRaceByFile — раса файла для валидации PUT /me/ship-icon (спека §6.5):
// файл реестра → слаг расы; нейтральный → ""; неизвестный/пусто → ok=false.
func TestShipRaceByFile(t *testing.T) {
	race, ok := ShipRaceByFile("race_humans_cruiser.png")
	require.True(t, ok)
	require.Equal(t, RaceHumans, race)

	race, ok = ShipRaceByFile("race_ammonia_02.png")
	require.True(t, ok)
	require.Equal(t, "ammonia", race)

	race, ok = ShipRaceByFile("neutral.png")
	require.True(t, ok)
	require.Empty(t, race, "нейтральный корабль — раса пустая")

	_, ok = ShipRaceByFile("crescent.png")
	require.False(t, ok)
	_, ok = ShipRaceByFile("")
	require.False(t, ok)
}

// ShipScaleHuman — размер корабля в ростах человека (решение создателя
// 2026-09-25): запись без значения → дефолт 12, значение > 0 → как задано,
// неизвестный/пустой файл → 12 (тот же фолбэк, что у ResolveShipIcon).
func TestShipScaleHuman(t *testing.T) {
	require.Equal(t, 12.0, DefaultShipScaleHuman)
	require.Equal(t, DefaultShipScaleHuman, ShipScaleHuman("race_humans_starship.png"))
	require.Equal(t, DefaultShipScaleHuman, ShipScaleHuman(""))
	require.Equal(t, DefaultShipScaleHuman, ShipScaleHuman("nope.png"))

	// Переопределение и явный ноль: временная запись в индексе (реестр не трогаем).
	const tmp = "test_scale_ship.png"
	defer delete(shipSpriteByFile, tmp)
	shipSpriteByFile[tmp] = ShipSprite{ID: "test_scale", File: tmp, ScaleHuman: 30}
	require.Equal(t, 30.0, ShipScaleHuman(tmp))
	shipSpriteByFile[tmp] = ShipSprite{ID: "test_scale", File: tmp, ScaleHuman: 0}
	require.Equal(t, DefaultShipScaleHuman, ShipScaleHuman(tmp), "ноль → дефолт (условие > 0)")
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

// ShipOrientByFile — поза по имени файла (ЧК-ship, идея 2026-09-23 §5): реюз
// индекса реестра, неизвестный/пустой файл → (0, false) — как фолбэк клиента.
// Плумбинг ненулевого угла проверяется временной записью в индексе (сам
// реестр RaceShipSprites не трогаем — порядок фиксирован, И7).
func TestShipOrientByFile(t *testing.T) {
	angle, flip := ShipOrientByFile("nope.png")
	require.Zero(t, angle)
	require.False(t, flip)
	angle, flip = ShipOrientByFile("")
	require.Zero(t, angle)
	require.False(t, flip)

	// Каждая запись расового реестра отдаёт свою пару (спека 2026-09-23:
	// источник — RaceShipSprites, не легаси-21).
	for _, s := range RaceShipSprites {
		a, f := ShipOrientByFile(s.File)
		require.Equal(t, s.Angle, a, s.File)
		require.Equal(t, s.Flip, f, s.File)
	}

	// Ненулевой угол/зеркало: временная запись в индексе.
	const tmp = "test_race_angled.png"
	shipSpriteByFile[tmp] = ShipSprite{ID: "test_race_angled", File: tmp, Angle: -90, Flip: true}
	defer delete(shipSpriteByFile, tmp)
	a, f := ShipOrientByFile(tmp)
	require.Equal(t, -90.0, a)
	require.True(t, f)
}
