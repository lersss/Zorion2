// internal/routegame/grid_search.go
// Поиск путей производственной модели «Прокладка маршрута» v9 «Планшет»
// (спека 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md, §14):
// точный минимум прогулки Start→Finish с посещением всех маяков на поднятом
// графе (клетка, направление, маска маяков) с ценой поворота τ и механиками
// направления/toll/моста. Только чистые функции: без БД, HTTP, времени и
// глобального RNG.
package routegame

import (
	"container/heap"
	"math"
)

// gridWalk — результат поиска: стоимость и (опционально) цепочка клеток.
type gridWalk struct {
	cost float64
	path []int
}

type gridItem struct {
	d  float64
	st int
}

type gridPQ []gridItem

func (p gridPQ) Len() int           { return len(p) }
func (p gridPQ) Less(i, j int) bool { return p[i].d < p[j].d }
func (p gridPQ) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p *gridPQ) Push(x any)        { *p = append(*p, x.(gridItem)) }
func (p *gridPQ) Pop() any {
	old := *p
	n := len(old)
	it := old[n-1]
	*p = old[:n-1]
	return it
}

// visitAllGrid — точный минимум стоимости прогулки Start→Finish, посещающей все
// маяки, на поднятом графе (клетка, направление, маска маяков).
func (f GridField) visitAllGrid(cm []float64, wantPath bool) gridWalk {
	n, k := f.N, len(f.Beacons)
	masks := 1 << k
	full := masks - 1
	bit := make([]int, n*n)
	for i := range bit {
		bit[i] = -1
	}
	for idx, b := range f.Beacons {
		bit[b] = idx
	}
	states := n * n * 4 * masks
	dist := make([]float64, states)
	for i := range dist {
		dist[i] = math.Inf(1)
	}
	var prev []int32
	if wantPath {
		prev = make([]int32, states)
		for i := range prev {
			prev[i] = -1
		}
	}
	idx := func(c, d, mask int) int { return ((c*4)+d)*masks + mask }
	pq := make(gridPQ, 0, 64)
	push := func(st int, d float64) {
		if d < dist[st]-1e-12 {
			dist[st] = d
			heap.Push(&pq, gridItem{d, st})
		}
	}
	for _, st := range f.neighbors4(f.Start) {
		if f.Blocked[st.cell] {
			continue
		}
		mask := 0
		if bit[st.cell] >= 0 {
			mask = 1 << bit[st.cell]
		}
		push(idx(st.cell, st.dir, mask), f.edgeCostGrid(cm, f.Start, st.cell))
	}
	for pq.Len() > 0 {
		it := heap.Pop(&pq).(gridItem)
		if it.d > dist[it.st]+1e-12 {
			continue
		}
		rem := it.st % (4 * masks)
		cell := it.st / (4 * masks)
		dir := rem / masks
		mask := rem % masks
		for _, st := range f.neighbors4(cell) {
			if f.Blocked[st.cell] {
				continue
			}
			nm := mask
			if bit[st.cell] >= 0 {
				nm |= 1 << bit[st.cell]
			}
			nd := it.d + f.edgeCostGrid(cm, cell, st.cell)
			if st.dir != dir {
				nd += f.TurnCost
			}
			nst := idx(st.cell, st.dir, nm)
			if nd < dist[nst]-1e-12 {
				dist[nst] = nd
				if wantPath {
					prev[nst] = int32(it.st)
				}
				heap.Push(&pq, gridItem{nd, nst})
			}
		}
	}
	best, bestSt := math.Inf(1), -1
	for d := 0; d < 4; d++ {
		st := idx(f.Finish, d, full)
		if dist[st] < best {
			best, bestSt = dist[st], st
		}
	}
	if bestSt < 0 || math.IsInf(best, 1) {
		return gridWalk{cost: math.Inf(1)}
	}
	w := gridWalk{cost: best}
	if wantPath {
		rev := []int{}
		for st := bestSt; st >= 0; st = int(prev[st]) {
			rev = append(rev, st/(4*masks))
			if int(prev[st]) < 0 {
				break
			}
		}
		path := make([]int, 0, len(rev)+1)
		path = append(path, f.Start)
		for i := len(rev) - 1; i >= 0; i-- {
			if rev[i] != f.Start {
				path = append(path, rev[i])
			}
		}
		w.path = path
	}
	return w
}

// buildRouteStairGrid — «лестничный» маршрут: Start → маяки в заданном порядке
// → Finish, каждый пролёт по канонической лестнице (без обхода).
func (f GridField) buildRouteStairGrid(order []int) []int {
	path := []int{f.Start}
	prev := f.Start
	for _, b := range order {
		p := f.staircaseGrid(prev, b)
		path = append(path, p[1:]...)
		prev = b
	}
	p := f.staircaseGrid(prev, f.Finish)
	return append(path, p[1:]...)
}
