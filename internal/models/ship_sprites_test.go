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

// Легаси-24 (21 базовый + 3 расовых «люди» — записи, существовавшие на момент
// правки, ShipSprites[:24]): пара показа (A, F) нулевая — поля в JSON
// отсутствуют, отрисовка не меняется (спека 2026-09-21-угол-корабля-в-метаданных
// §6.3, §9 п.11). Импортированные позже записи (в конец, И8) несут свою пару и
// эту проверку не ломают.
func TestShipSpritesLegacyZeroOrient(t *testing.T) {
	require.GreaterOrEqual(t, len(ShipSprites), 24)
	for _, s := range ShipSprites[:24] {
		require.Zero(t, s.Angle, "%s: Angle должен быть 0 (легаси)", s.ID)
		require.False(t, s.Flip, "%s: Flip должен быть false (легаси)", s.ID)
	}
}

// Диапазон угла пары у ВСЕХ записей реестра: −180 < Angle ≤ 180 (строго —
// значение −180 вне конвенции §3.1). Flip на диапазон не влияет: проверяется
// Angle независимо от него (спека §9 п.12).
func TestShipSpritesOrientInRange(t *testing.T) {
	for _, s := range ShipSprites {
		require.Greater(t, s.Angle, -180.0, "%s: Angle должен быть > -180", s.ID)
		require.LessOrEqual(t, s.Angle, 180.0, "%s: Angle должен быть <= 180", s.ID)
	}
}

// DefaultHumanShip ∈ расовый реестр (спека 2026-09-23 §4.1/§6.2: дефолт
// users.ship_icon — член реестра).
func TestDefaultHumanShipInRegistry(t *testing.T) {
	require.True(t, IsValidShipIcon(DefaultHumanShip))
}

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
// реестр ShipSprites не трогаем — порядок фиксирован, И8).
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
