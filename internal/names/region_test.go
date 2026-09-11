// internal/names/region_test.go
package names

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRegionName(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	used := make(map[string]bool)

	for i := 0; i < 200; i++ {
		n := GenerateRegionName(rng, used)
		require.NotEmpty(t, n)
		assert.True(t, used[n], "имя %s не в usedNames после генерации", n)
		assert.True(t, IsEuphoniousRegionName(n), "неблагозвучное имя: %s", n)
	}
}

func TestGenerateRegionNameUniqueFor500(t *testing.T) {
	// Регионов в галактике может быть 200–500 — все имена должны быть
	// уникальны без падения в fallback.
	rng := rand.New(rand.NewSource(2026))
	used := make(map[string]bool)
	for i := 0; i < 500; i++ {
		n := GenerateRegionName(rng, used)
		assert.NotEmpty(t, n)
		assert.True(t, IsEuphoniousRegionName(n), "неблагозвучное имя: %s", n)
	}
	assert.Len(t, used, 500, "все 500 имён регионов должны быть уникальны")

	// Ни одно имя не должно выпадать в fallback "Сектор-XXXX".
	for n := range used {
		assert.NotRegexp(t, `^Сектор-[A-Z0-9]{4}$`, n, "выпало в fallback: %s", n)
	}
}

func TestRegionNameCombinationSpace(t *testing.T) {
	// Пространство имён достаточно для сотен регионов.
	assert.GreaterOrEqual(t, len(regionRoots)*len(regionEndings), 500,
		"комбинаций корней и окончаний не хватает на 500 регионов")
}

func TestRegionNamePrefixAndBase(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	used := make(map[string]bool)
	withPrefix := false
	withoutPrefix := false
	for i := 0; i < 200 && (!withPrefix || !withoutPrefix); i++ {
		n := GenerateRegionName(rng, used)
		isPrefixed := false
		for _, p := range regionPrefixes {
			if strings.HasPrefix(n, p+" ") {
				isPrefixed = true
				break
			}
		}
		if isPrefixed {
			withPrefix = true
		} else {
			withoutPrefix = true
		}
	}
	assert.True(t, withPrefix, "ни одного имени с префиксом-типом за 200 попыток")
	assert.True(t, withoutPrefix, "ни одного имени без префикса за 200 попыток")
}