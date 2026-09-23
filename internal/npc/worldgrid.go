package npc

import (
	"math"
	"math/rand"

	"zorion/internal/mapcache"
)

// routeRadius — радиус выбора маршрута агента (спека §3.2, решение §10.2).
const routeRadius = 1000.0

// maxRing — предел расширения колец клеток в резервном поиске «ближайшего»
// (клетка = 1000 ед., покрывает галактику любого пресета; полный обход
// галактики не выполняется — И2).
const maxRing = 64

type gridWorld struct {
	id   string
	name string
	x, y float64
}

type cellKey struct{ cx, cy int }

// worldGrid — пространственная сетка миров по клеткам routeRadius.
// Строится из снапшота mapcache один раз на генерацию вселенной (O(n)),
// выбор маршрута — по 3×3 клеткам вокруг позиции (O(кандидатов)), не
// зависит от размера галактики (спека §9 И2).
type worldGrid struct {
	cellSize float64
	cells    map[cellKey][]gridWorld
	byID     map[string]gridWorld // координаты мира по id (позиции, §2.2.B)
}

func buildGrid(worlds []mapcache.World) *worldGrid {
	g := &worldGrid{
		cellSize: routeRadius,
		cells:    make(map[cellKey][]gridWorld, len(worlds)/8),
		byID:     make(map[string]gridWorld, len(worlds)),
	}
	for _, w := range worlds {
		gw := gridWorld{id: w.ID, name: w.Name, x: w.X, y: w.Y}
		key := cellKey{
			int(math.Floor(w.X / g.cellSize)),
			int(math.Floor(w.Y / g.cellSize)),
		}
		g.cells[key] = append(g.cells[key], gw)
		g.byID[w.ID] = gw
	}
	return g
}

// coordsOf — координаты мира по id (ok=false — мира нет в сетке).
func (g *worldGrid) coordsOf(id string) (float64, float64, bool) {
	if g == nil {
		return 0, 0, false
	}
	w, ok := g.byID[id]
	return w.x, w.y, ok
}

// nameOf — имя мира по id (ok=false — мира нет в сетке). Для тултипа агента:
// названия миров вместо айди (идея 2026-09-18).
func (g *worldGrid) nameOf(id string) (string, bool) {
	if g == nil {
		return "", false
	}
	w, ok := g.byID[id]
	return w.name, ok
}

// pickTarget — целевой мир для полёта из позиции (x, y) (спека §3.2):
//  1. случайный мир в радиусе routeRadius (клетка + 8 соседей, dist ≤ 1000);
//     все миры радиуса гарантированно в 3×3 окне клеток (клетка = радиус);
//  2. если в радиусе пусто — ближайший: расширение колец клеток;
//  3. на совсем пустое окно — случайный мир галактики.
//
// Текущий мир (avoidID) исключается на всех путях — полёт «на месте» не
// запускается. rnd — локальный *rand.Rand вызывающего (не потокобезопасны).
func (g *worldGrid) pickTarget(x, y float64, avoidID string, rnd *rand.Rand) (string, bool) {
	key := cellKey{
		int(math.Floor(x / g.cellSize)),
		int(math.Floor(y / g.cellSize)),
	}

	// 1. Случайный мир в радиусе.
	var candidates []gridWorld
	radius2 := routeRadius * routeRadius
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for _, w := range g.cells[cellKey{key.cx + dx, key.cy + dy}] {
				if w.id == avoidID {
					continue
				}
				dxw, dyw := w.x-x, w.y-y
				if dxw*dxw+dyw*dyw <= radius2 {
					candidates = append(candidates, w)
				}
			}
		}
	}
	if len(candidates) > 0 {
		return candidates[rnd.Intn(len(candidates))].id, true
	}

	// 2. Резерв: ближайший — кольца клеток (без полного обхода галактики).
	for r := 2; r <= maxRing; r++ {
		var nearest gridWorld
		best := math.Inf(1)
		found := false
		// Клетки кольца max(|dx|,|dy|) = r: две горизонтальные и две
		// вертикальные стороны (углы дублируются — повторная проверка
		// безвредна).
		for dx := -r; dx <= r; dx++ {
			for _, c := range [4]cellKey{
				{key.cx + dx, key.cy - r}, {key.cx + dx, key.cy + r},
				{key.cx - r, key.cy + dx}, {key.cx + r, key.cy + dx},
			} {
				for _, w := range g.cells[c] {
					if w.id == avoidID {
						continue
					}
					dxw, dyw := w.x-x, w.y-y
					d2 := dxw*dxw + dyw*dyw
					if d2 < best {
						best = d2
						nearest = w
						found = true
					}
				}
			}
		}
		if found {
			return nearest.id, true
		}
	}

	// 3. Совсем пусто: случайный мир галактики (исключая текущий).
	ids := make([]string, 0, len(g.byID))
	for id := range g.byID {
		if id != avoidID {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		return ids[rnd.Intn(len(ids))], true
	}
	return "", false
}