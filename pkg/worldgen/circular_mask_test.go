// pkg/worldgen/circular_mask_test.go
package worldgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// newGrid — сетка rows×cols со значением fill.
func newGrid(rows, cols int, fill float64) [][]float64 {
	g := make([][]float64, rows)
	for y := range g {
		g[y] = make([]float64, cols)
		for x := range g[y] {
			g[y][x] = fill
		}
	}
	return g
}

func TestApplyCircularMaskEmpty(t *testing.T) {
	// Пустая карта — не должна паниковать.
	ApplyCircularMask([][]float64{}, 2, 2, 2, 0)
	ApplyCircularMask(nil, 2, 2, 2, 0)

	// Строка нулевой длины — не должна паниковать.
	ApplyCircularMask([][]float64{{}}, 2, 2, 2, 0)
}

func TestApplyCircularMaskClearsOutside(t *testing.T) {
	// Сетка 5×5, центр (2,2), радиус 2.
	// Круг: |dx|²+|dy|² ≤ 4.
	grid := newGrid(5, 5, 1.0)
	ApplyCircularMask(grid, 2, 2, 2, 0)

	inside := [][2]int{{2, 2}, {2, 1}, {2, 3}, {1, 2}, {3, 2}, {1, 1}}
	// Клетки ровно на границе (distSq == radiusSq) НЕ очищаются.
	edge := [][2]int{{0, 2}, {2, 0}}
	outside := [][2]int{{0, 0}, {0, 4}, {4, 0}, {4, 4}}

	for _, p := range inside {
		assert.Equal(t, 1.0, grid[p[1]][p[0]], "внутри круга (%d,%d) должно остаться исходное", p[0], p[1])
	}
	for _, p := range edge {
		assert.Equal(t, 1.0, grid[p[1]][p[0]], "на границе (%d,%d) должно остаться исходное", p[0], p[1])
	}
	for _, p := range outside {
		assert.Equal(t, 0.0, grid[p[1]][p[0]], "вне круга (%d,%d) должно стать outsideValue", p[0], p[1])
	}
}

func TestApplyCircularMaskCustomOutsideValue(t *testing.T) {
	grid := newGrid(3, 3, 5.0)
	ApplyCircularMask(grid, 1, 1, 1, -1.0)

	assert.Equal(t, 5.0, grid[1][1], "центр не трогаем")
	assert.Equal(t, -1.0, grid[0][0], "угол вне круга")
	assert.Equal(t, 5.0, grid[1][0], "клетка внутри радиуса 1 от центра")
}

func TestApplySmoothCircularMaskNoSmoothWidth(t *testing.T) {
	// smoothWidth = 0 → ведёт себя как жёсткая маска (граница совпадает с радиусом).
	grid := newGrid(3, 3, 1.0)
	ApplySmoothCircularMask(grid, 1, 1, 1, 0, 0)

	assert.Equal(t, 0.0, grid[0][0])
	assert.Equal(t, 1.0, grid[1][1])
	assert.Equal(t, 1.0, grid[1][0])
}

func TestApplySmoothCircularMaskValues(t *testing.T) {
	// Сетка 5×5, центр (2,2), radius=2, smoothWidth=1.
	grid := newGrid(5, 5, 1.0)
	ApplySmoothCircularMask(grid, 2, 2, 2, 1, 0)

	// Центр и клетки внутри radius-smoothWidth — без изменений.
	assert.Equal(t, 1.0, grid[2][2], "центр без изменений")
	assert.Equal(t, 1.0, grid[1][2], "(dist=1) на границе зоны сглаживания")

	// Клетка на dist == radius должна стать ровно outsideValue.
	assert.Equal(t, 0.0, grid[0][2], "(dist=2) должно слиться в outsideValue")

	// Клетка вне круга — outsideValue.
	for _, p := range [][2]int{{0, 0}, {4, 4}, {0, 4}} {
		assert.Equal(t, 0.0, grid[p[1]][p[0]])
	}

	// В зоне сглаживания значение строго между исходным и outsideValue.
	v := grid[1][1] // dist ≈ 1.414
	assert.Greater(t, v, 0.0, "в зоне сглаживания значение должно быть > outsideValue")
	assert.Less(t, v, 1.0, "в зоне сглаживания значение должно быть < исходного")
}

func TestApplySmoothCircularMaskMonotonic(t *testing.T) {
	// Чем дальше от центра — тем ближе значение к outsideValue.
	// (чем дальше от центра — тем ближе значение к outsideValue)
	grid := newGrid(9, 9, 1.0)
	ApplySmoothCircularMask(grid, 4, 4, 3, 2, 0)

	// dist=√2 (ближе к центру) против dist=√8 (ближе к краю).
	near := grid[3][3] // (dx=-1, dy=-1) → dist≈1.414
	far := grid[2][4]  // (dx=-2, dy=0)  → dist=2
	assert.Greater(t, near, far,
		"ближе к центру значение должно быть выше (монотонность)")

	// Крайние точки зоны сглаживания.
	assert.Equal(t, 1.0, grid[4][3], "на границе, где t=0, значение исходное")       // dist=1
	assert.Equal(t, 0.0, grid[1][4], "за радиусом 3 — уже outsideValue")             // dist=3
	assert.Equal(t, 0.0, grid[4][1], "за радиусом 3 — уже outsideValue")             // dist=3
}