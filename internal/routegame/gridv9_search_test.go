// internal/routegame/gridv9_search.go
// Поиск путей модели v9: точный минимум прогулки Start→Finish с посещением всех
// маяков на поднятом графе (клетка, направление, маска маяков) с ценой поворота
// τ и механиками направления/toll/моста, плюс вспомогательные маршруты.
// Только чистые функции.
package routegame

import (
	"container/heap"
	"math"
)

// V9Walk — результат поиска: стоимость и (опционально) цепочка клеток.
type V9Walk struct {
	cost float64
	path []int
}

type V9Item struct {
	d  float64
	st int
}

type V9PQ []V9Item

func (p V9PQ) Len() int           { return len(p) }
func (p V9PQ) Less(i, j int) bool { return p[i].d < p[j].d }
func (p V9PQ) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p *V9PQ) Push(x any)        { *p = append(*p, x.(V9Item)) }
func (p *V9PQ) Pop() any          { old := *p; n := len(old); it := old[n-1]; *p = old[:n-1]; return it }

// visitAllV9 — точный минимум стоимости прогулки Start→Finish, посещающей все
// маяки, на поднятом графе (клетка, направление, маска маяков).
func (f V9Field) visitAllV9(cm []float64, wantPath bool) V9Walk {
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
	pq := make(V9PQ, 0, 64)
	push := func(st int, d float64) {
		if d < dist[st]-1e-12 {
			dist[st] = d
			heap.Push(&pq, V9Item{d, st})
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
		push(idx(st.cell, st.dir, mask), f.edgeCostV9(cm, f.Start, st.cell))
	}
	for pq.Len() > 0 {
		it := heap.Pop(&pq).(V9Item)
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
			nd := it.d + f.edgeCostV9(cm, cell, st.cell)
			if st.dir != dir {
				nd += f.TurnCost
			}
			nst := idx(st.cell, st.dir, nm)
			if nd < dist[nst]-1e-12 {
				dist[nst] = nd
				if wantPath {
					prev[nst] = int32(it.st)
				}
				heap.Push(&pq, V9Item{nd, nst})
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
		return V9Walk{cost: math.Inf(1)}
	}
	w := V9Walk{cost: best}
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

// costOnlyLegV9 — кратчайший путь a→b по базовой цене клеток (без τ).
func (f V9Field) costOnlyLegV9(cm []float64, a, b int) (float64, []int) {
	if a == b {
		return 0, []int{a}
	}
	n2 := f.N * f.N
	dist := make([]float64, n2)
	prev := make([]int32, n2)
	for i := range dist {
		dist[i] = math.Inf(1)
		prev[i] = -1
	}
	dist[a] = 0
	pq := make(V9PQ, 0, 64)
	heap.Push(&pq, V9Item{0, a})
	for pq.Len() > 0 {
		it := heap.Pop(&pq).(V9Item)
		if it.d > dist[it.st]+1e-12 {
			continue
		}
		for _, st := range f.neighbors4(it.st) {
			nd := it.d + cm[st.cell]
			if nd < dist[st.cell]-1e-12 {
				dist[st.cell] = nd
				prev[st.cell] = int32(it.st)
				heap.Push(&pq, V9Item{nd, st.cell})
			}
		}
	}
	if math.IsInf(dist[b], 1) {
		return math.Inf(1), nil
	}
	rev := []int{}
	for st := b; st >= 0; st = int(prev[st]) {
		rev = append(rev, st)
		if int(prev[st]) < 0 {
			break
		}
	}
	path := make([]int, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		path = append(path, rev[i])
	}
	return dist[b], path
}

func (f V9Field) buildRouteStairV9(order []int) []int {
	path := []int{f.Start}
	prev := f.Start
	for _, b := range order {
		p := f.staircaseV9(prev, b)
		path = append(path, p[1:]...)
		prev = b
	}
	p := f.staircaseV9(prev, f.Finish)
	return append(path, p[1:]...)
}

// buildRouteStairAvoidMudV9 — «ленивый» лестничный маршрут естественного порядка,
// где только явная топь обходится локально (небрежный C, §14.9).
func (f V9Field) buildRouteStairAvoidMudV9(order []int) []int {
	path := []int{f.Start}
	prev := f.Start
	for _, b := range order {
		p := f.staircaseV9(prev, b)
		path = append(path, p[1:]...)
		prev = b
	}
	p := f.staircaseV9(prev, f.Finish)
	path = append(path, p[1:]...)
	return f.detourMudV9(path)
}

// mapAvoidMudV9 — видимая карта с обходом явной топи (небрежный C, §14.9).
func (f V9Field) mapAvoidMudV9() []float64 {
	cm := append([]float64(nil), f.Visible...)
	for c := range f.Mud {
		cm[c] = 9.0 // «явная топь» обходится, а не срезается
	}
	return cm
}

// detourMudV9 — локальный обход клеток топи на уже построенном пути.
func (f V9Field) detourMudV9(path []int) []int {
	out := append([]int(nil), path...)
	for i := 1; i < len(out)-1; i++ {
		if f.Mud[out[i]] <= 0 {
			continue
		}
		l, r := i-1, i+1
		for l > 0 && f.Mud[out[l]] > 0 {
			l--
		}
		for r < len(out)-1 && f.Mud[out[r]] > 0 {
			r++
		}
		cm := f.mapAvoidMudV9()
		_, seg := f.costOnlyLegV9(cm, out[l], out[r])
		if len(seg) >= 2 {
			rebuilt := append([]int{}, out[:l]...)
			rebuilt = append(rebuilt, seg...)
			rebuilt = append(rebuilt, out[r+1:]...)
			out = rebuilt
			i = l + len(seg) - 1
		}
	}
	return out
}
