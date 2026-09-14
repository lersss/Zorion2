// internal/npc/worldgrid_test.go
// Тесты выбора маршрута (спека 20a.1 §3.2): рандомный мир в радиусе 1000,
// резерв — ближайший, исключение текущего мира, стоимость не зависит от
// размера галактики (И2 — поиск по 3×3 клеткам, не полным обходом).
package npc

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/mapcache"
)

func testRnd() *rand.Rand { return rand.New(rand.NewSource(42)) }

func TestPickTargetWithinRadius(t *testing.T) {
	// w2 в радиусе (500), w3 вне (2000) — возвращается только w2.
	grid := buildGrid([]mapcache.World{
		{ID: "w1", X: 0, Y: 0},
		{ID: "w2", X: 500, Y: 0},
		{ID: "w3", X: 2000, Y: 0},
	})
	id, ok := grid.pickTarget(0, 0, "w1", testRnd())
	require.True(t, ok)
	require.Equal(t, "w2", id)
}

func TestPickTargetBoundaryRadius(t *testing.T) {
	// dist = 1000 ровно — на границе радиуса, входит (≤).
	grid := buildGrid([]mapcache.World{
		{ID: "w1", X: 0, Y: 0},
		{ID: "w2", X: 1000, Y: 0},
	})
	id, ok := grid.pickTarget(0, 0, "w1", testRnd())
	require.True(t, ok)
	require.Equal(t, "w2", id)
}

func TestPickTargetRandomAmongCandidates(t *testing.T) {
	// 10 миров в радиусе — любой результат в радиусе и не текущий мир.
	grid := buildGrid([]mapcache.World{
		{ID: "w1", X: 0, Y: 0},
		{ID: "a", X: 100, Y: 0}, {ID: "b", X: -100, Y: 0},
		{ID: "c", X: 0, Y: 100}, {ID: "d", X: 0, Y: -100},
		{ID: "e", X: 700, Y: 700}, {ID: "f", X: -700, Y: 700},
		{ID: "g", X: 700, Y: -700}, {ID: "h", X: -700, Y: -700},
		{ID: "i", X: 999, Y: 0},
	})
	for i := 0; i < 20; i++ {
		id, ok := grid.pickTarget(0, 0, "w1", testRnd())
		require.True(t, ok)
		require.NotEqual(t, "w1", id, "текущий мир исключается")
		w := grid.byID[id]
		require.LessOrEqual(t, math.Hypot(w.x, w.y), routeRadius, "цель в радиусе 1000")
	}
}

func TestPickTargetNearestFallback(t *testing.T) {
	// В радиусе пусто — ближайший из колец клеток (w2 ближе w3).
	grid := buildGrid([]mapcache.World{
		{ID: "w1", X: 0, Y: 0},
		{ID: "w2", X: 3000, Y: 0},
		{ID: "w3", X: 5000, Y: 100},
	})
	id, ok := grid.pickTarget(0, 0, "w1", testRnd())
	require.True(t, ok)
	require.Equal(t, "w2", id)
}

func TestPickTargetOnlyCurrentWorld(t *testing.T) {
	// В галактике один мир (текущий) — лететь некуда.
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}})
	_, ok := grid.pickTarget(0, 0, "w1", testRnd())
	require.False(t, ok)
}

func TestPickTargetEmptyGalaxy(t *testing.T) {
	grid := buildGrid(nil)
	_, ok := grid.pickTarget(0, 0, "w1", testRnd())
	require.False(t, ok)
}

func TestPickTargetFallbackRandom(t *testing.T) {
	// Радиус пуст, кольца до maxRing пусты (мир дальше maxRing×1000),
	// но галактика непуста — случайный мир галактики.
	grid := buildGrid([]mapcache.World{
		{ID: "w1", X: 0, Y: 0},
		{ID: "far", X: (maxRing + 5) * 1000, Y: 0},
	})
	id, ok := grid.pickTarget(0, 0, "w1", testRnd())
	require.True(t, ok)
	require.Equal(t, "far", id)
}

func TestCoordsOf(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 3, Y: -4}})
	x, y, ok := grid.coordsOf("w1")
	require.True(t, ok)
	require.Equal(t, 3.0, x)
	require.Equal(t, -4.0, y)

	_, _, ok = grid.coordsOf("nope")
	require.False(t, ok)
}

// ==================== МАССОВАЯ ГЕНЕРАЦИЯ (спека 26a.1 §4.4) ====================

// allIDs — предвычисленный список id всех миров галактики (без N запросов
// к БД и без построения слайса на каждый вызов).
func TestGridAllIDs(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}, {ID: "w3", X: 200, Y: 0}})
	require.Len(t, grid.allIDs, 3)
	require.ElementsMatch(t, []string{"w1", "w2", "w3"}, grid.allIDs)

	empty := buildGrid(nil)
	require.Empty(t, empty.allIDs)
}

// randomWorlds — n случайных миров галактики (повторы допустимы, спека
// §4.4: «случайные по всей галактике» ≠ «разные миры»); false — пусто.
func TestGridRandomWorlds(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 0, Y: 0}, {ID: "w2", X: 100, Y: 0}})
	rnd := testRnd()

	out, ok := grid.randomWorlds(rnd, 5)
	require.True(t, ok)
	require.Len(t, out, 5)
	for _, id := range out {
		require.Contains(t, []string{"w1", "w2"}, id)
	}

	_, ok = grid.randomWorlds(rnd, 0)
	require.True(t, ok, "пустая пачка — не ошибка, мир не нужен")

	empty := buildGrid(nil)
	_, ok = empty.randomWorlds(rnd, 3)
	require.False(t, ok, "галактика пуста — миров нет")

	var nilGrid *worldGrid
	_, ok = nilGrid.randomWorlds(rnd, 3)
	require.False(t, ok, "сетка не построена — миров нет")
}