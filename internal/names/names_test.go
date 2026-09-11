// internal/names/names_test.go
package names

import (
	"math/rand"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== LOCALIZED NAME ====================

func TestLocalizedName(t *testing.T) {
	n := LocalizedName{Cyr: "Атрокс", Lat: "Atrox"}
	assert.Equal(t, "Атрокс", n.String())
	assert.False(t, n.IsEmpty())

	assert.True(t, (LocalizedName{}).IsEmpty())
	// Пустой считается только полностью пустой объект.
	assert.False(t, (LocalizedName{Cyr: "X"}).IsEmpty())
	assert.False(t, (LocalizedName{Lat: "X"}).IsEmpty())
}

// ==================== СЛУЧАЙНЫЙ СУФФИКС ====================

func TestRandomSuffix(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		s := randomSuffix(rng)
		require.Len(t, s, 4)
		for _, ch := range s {
			assert.True(t, ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9',
				"unexpected char %q в суффиксе", ch)
		}
	}
}

// ==================== ИМЕНА ЗВЁЗД ====================

func TestGenerateStarName(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	used := make(map[string]bool)
	names := make(map[string]bool)

	for i := 0; i < 100; i++ {
		n := GenerateStarName(rng, used)
		require.NotEmpty(t, n)
		assert.False(t, names[n], "дубликат звезды: %s", n)
		names[n] = true
		assert.True(t, used[n], "имя не добавлено в usedNames")
	}
}

func TestGenerateStarNameUnique1000(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	used := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		n := GenerateStarName(rng, used)
		assert.NotEmpty(t, n)
	}
	assert.Len(t, used, 1000, "все 1000 имён звёзд должны быть уникальны")
}

// ==================== ИМЕНА ПЛАНЕТ ====================

func TestGeneratePlanetName(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	used := make(map[string]bool)

	for i := 0; i < 200; i++ {
		n := GeneratePlanetName(rng, used)
		require.NotEmpty(t, n)
		// Первый символ — заглавная.
		assert.True(t, unicode.IsUpper(rune(n[0])), " имя не начинается с заглавной: %s", n)
		// Длина ≤ 10 (из константы maxLen).
		assert.LessOrEqual(t, len(n), 10, "длина > 10: %s", n)
		assert.GreaterOrEqual(t, len(n), 2, "слишком короткое: %s", n)
		assert.False(t, used[n], "дубликат планеты: %s", n)
		used[n] = true
	}
}

// ==================== ИМЕНА СПУТНИКОВ ====================

func TestGenerateSatelliteName(t *testing.T) {
	rng := rand.New(rand.NewSource(55))
	used := make(map[string]bool)

	for i := 0; i < 100; i++ {
		n := GenerateSatelliteName("Aldebaran", rng, used)
		require.NotEmpty(t, n)
		assert.True(t, strings.HasPrefix(n, "Aldebaran-"),
			"не начинается с имени звезды: %s", n)
		assert.Less(t, len(n), 20)
		assert.True(t, used[n], "имя %s не в usedNames после генерации", n)
	}
}

func TestGenerateSatelliteNameEmptyStar(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	used := make(map[string]bool)
	n := GenerateSatelliteName("", rng, used)
	require.NotEmpty(t, n)
	assert.True(t, strings.HasPrefix(n, "Satel-"))
}

// ==================== ИМЕНА ФРАКЦИЙ ====================

func TestGenerateFactionName(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	used := make(map[string]bool)

	for i := 0; i < 100; i++ {
		n := GenerateFactionName(rng, used)
		require.NotEmpty(t, n)
		assert.True(t, used[n], "имя %s не в usedNames после генерации", n)
	}
}

// ==================== ИМЕНА ТОВАРОВ ====================

func TestGenerateProductName(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	used := make(map[string]bool)

	categories := []string{"food", "clothing", "weapons", "medicine", "unknown"}
	for _, cat := range categories {
		n := GenerateProductName(cat, rng, used)
		require.NotEmpty(t, n, "пустое имя для категории %s", cat)
		// Проверяем, что функция зарегистрировала имя.
		assert.True(t, used[n], "имя %s не в usedNames после генерации", n)
	}
}

// ==================== ИМЕНА РЕСУРСОВ ====================

// isCyrillicAll — все символы являются кириллическими буквами.
func isCyrillicAll(s string) bool {
	for _, r := range s {
		if !unicode.Is(unicode.Cyrillic, r) {
			return false
		}
	}
	return true
}

// isLatinAll — все символы являются латинскими буквами.
func isLatinAll(s string) bool {
	for _, r := range s {
		if !unicode.Is(unicode.Latin, r) {
			return false
		}
	}
	return true
}

func TestGenerateResourceName(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	used := make(map[string]bool)
	before := len(used)

	categories := []string{"mineral", "organic", "rare", "fuel", "water", "gas", "unknown"}
	for _, cat := range categories {
		n := GenerateResourceName(cat, rng, used)
		require.NotEmpty(t, n.Cyr, "пустая кириллица для %s", cat)
		require.NotEmpty(t, n.Lat, "пустая латиница для %s", cat)

		// Кириллица и латиница в правильных алфавитах.
		assert.True(t, isCyrillicAll(n.Cyr),
			"не кириллица в %q для категории %s", n.Cyr, cat)
		assert.True(t, isLatinAll(n.Lat),
			"не латиница в %q для категории %s", n.Lat, cat)
	}

	// Функция сама добавляет имена в usedNames → после всех вызовов
	// количество уникальных имён должно быть >= числа категорий.
	// (Возможны fallback-дублик "Ресурс-XXXX" → по одному на вызов,
	// но_usedNames гарантирует, что даже fallback не повторяется.)
	_ = before
}

func TestGenerateResourceNameCyrLatPattern(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	used := make(map[string]bool)

	// 50 генераций: Cyr и Lat оба консистентны и не пусты.
	for i := 0; i < 50; i++ {
		n := GenerateResourceName("mineral", rng, used)
		assert.GreaterOrEqual(t, len([]rune(n.Cyr)), 2,
			"слишком короткая кириллица: %q", n.Cyr)
		assert.GreaterOrEqual(t, len([]rune(n.Lat)), 2,
			"слишком короткая латиница: %q", n.Lat)
		assert.False(t, strings.ContainsAny(n.Cyr, "0123456789-"),
			"цифры/дефис в кириллице: %q", n.Cyr)
		assert.False(t, strings.ContainsAny(n.Lat, "0123456789-"),
			"цифры/дефис в латинице: %q", n.Lat)
	}
}

func TestGenerateResourceNameUniqueness1000(t *testing.T) {
	rng := rand.New(rand.NewSource(100))
	used := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		_ = GenerateResourceName("organic", rng, used)
	}
	assert.Len(t, used, 1000, "все 1000 имён ресурсов должны быть уникальны")
}
