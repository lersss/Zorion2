// internal/generator/galaxy/performance_test.go
package galaxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGenerateGalaxyPerformance — регрессионный тест на медленную генерацию.
//
// История: при боевых параметрах (100k миров, MapSize 60000, MinDist 300)
// генерация «зависала» на десятки минут. Причина — isPointValid делает
// линейный проход по всем уже расставленным точкам (O(n²)), а
// galaxyEdgeAccept в «добивке» и выбросах отсеивает часть кандидатов,
// заставляя каждый отбор снова сканировать весь список.
//
// Ключ регрессии — «добивка» в состоянии высокой плотности: когда цель
// близка к физическому пределу упаковки (для 100k это ~62% от максимума
// при MinDist 300), почти все попытки неудачны, и каждая неудача — полный
// проход по всем точкам. Здесь масштаб уменьшен, но плотность та же:
// 12k миров в круге радиуса 6000 при MinDist 150 (максимум ~6400/точки
// с учётом разрежения краёв) — добивка упирается в стену и молотит.
//
// После оптимизации (сеточный индекс вместо линейного прохода) тест
// должен проходить за доли секунды.
func TestGenerateGalaxyPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("производительность не проверяется в -short")
	}
	if isRace() {
		t.Skip("производительность не меряется под -race: детектор замедляет прогон в 3–10x, бюджет 5 с писан для обычного прогона")
	}

	g := NewGenerator(&Config{
		Seed: 42, WorldCount: 12000, MapSize: 6000, MinDist: 150,
		ClusterCount: 40, ClusterSpacing: 2000, ClusterRadius: 1000,
		OutlierPercent: 0.2, WorldSpread: 20, Shape: "blob",
	})

	const budget = 5 * time.Second
	start := time.Now()
	result := g.GenerateGalaxyWithRegions()
	elapsed := time.Since(start)

	require.NotNil(t, result)
	require.Less(t, elapsed, budget,
		"генерация 12k миров заняла %s — вероятно линейный проход isPointValid", elapsed)
}
