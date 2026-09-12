// internal/mapcache/query.go
package mapcache

import (
	"math"
	"sort"
	"strings"
)

// maxClusterReturn — страховка размера ответа (как прежний LIMIT в SQL).
const maxClusterReturn = 20000

// Filter — опциональные фильтры карты. Пустое поле — фильтр выключен.
type Filter struct {
	HasPlanets       bool
	HasLife          bool
	HasHabitable     bool
	PlanetType       string // тип планеты, без учёта регистра
	ResourceCategory string // код категории ресурсов, напр. "fuel"
}

// Cluster — одна запись ответа карты: кластер (Count >= 5) или отдельная
// звезда из разреженной ячейки (Count == 1 с заполненными Sample*).
type Cluster struct {
	CellX          int64
	CellY          int64
	Count          int
	X, Y           float64
	SampleID       string
	SampleName     string
	SampleSpectral string
	SampleTemp     float64
}

// spectralRank — «яркость» спектрального класса для выбора представителя
// ячейки: меньше — ярче (O=1 ... Y=10, неизвестные — 99).
var spectralRank = map[string]int{
	"O": 1, "B": 2, "A": 3, "F": 4, "G": 5,
	"K": 6, "M": 7, "L": 8, "T": 9, "Y": 10,
}

func rankOf(spectral string) int {
	if r, ok := spectralRank[spectral]; ok {
		return r
	}
	return 99
}

type cellKey struct{ x, y int64 }

type cellAgg struct {
	cnt      int
	bestRank int
	bestID   string
	bestX    float64
	bestY    float64
	bestSpec string
	bestTemp float64
	// Индексы миров для разреженных ячеек (cnt < 5): каждый мир ячейки
	// возвращается отдельной точкой в ответе.
	sparse []int
}

// Query — фильтрует миры по границам viewport'а и фильтрам и кластеризует
// по ячейкам ровно так же, как прежний SQL-запрос FilterWorldsHandler:
//
//	cell_x = FLOOR(coord_x / cell), cell_y = FLOOR(coord_y / cell)
//	ячейка 1–4 мира — каждая звезда отдельной точкой (cnt=1),
//	5+ миров — кластер; позиция и цвет кластера — от «самой яркой» звезды
//	(минимальный srank, при равенстве — меньший id);
//	результат отсортирован по (cell_x, cell_y), внутри разреженной ячейки —
//	по id для детерминизма.
func (s *Snapshot) Query(xMin, xMax, yMin, yMax, cell float64, f Filter) []Cluster {
	if s == nil || len(s.worlds) == 0 {
		return nil
	}

	planetType := strings.ToLower(f.PlanetType)
	planetTypeActive := f.PlanetType != ""
	resourceActive := f.ResourceCategory != ""
	resourceMask := resourceBit(f.ResourceCategory)

	cells := make(map[cellKey]*cellAgg)
	keys := make([]cellKey, 0, 512)

	for i := range s.worlds {
		w := &s.worlds[i]
		if w.X < xMin || w.X > xMax || w.Y < yMin || w.Y > yMax {
			continue
		}
		if f.HasPlanets && !w.HasPlanets {
			continue
		}
		if f.HasLife && !w.HasLife {
			continue
		}
		if f.HasHabitable && !w.HasHabitable {
			continue
		}
		if planetTypeActive && !w.PlanetTypes[planetType] {
			continue
		}
		if resourceActive && w.Resources&resourceMask == 0 {
			continue
		}

		key := cellKey{
			int64(math.Floor(w.X / cell)),
			int64(math.Floor(w.Y / cell)),
		}
		a := cells[key]
		if a == nil {
			a = &cellAgg{bestRank: math.MaxInt32}
			cells[key] = a
			keys = append(keys, key)
		}
		a.cnt++

		if r := rankOf(w.Spectral); r < a.bestRank || (r == a.bestRank && w.ID < a.bestID) {
			a.bestRank = r
			a.bestID = w.ID
			a.bestX = w.X
			a.bestY = w.Y
			a.bestSpec = w.Spectral
			a.bestTemp = w.Temp
		}
		if a.cnt < 5 {
			a.sparse = append(a.sparse, i)
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].x != keys[j].x {
			return keys[i].x < keys[j].x
		}
		return keys[i].y < keys[j].y
	})

	out := make([]Cluster, 0, len(keys)*2)
	for _, k := range keys {
		a := cells[k]
		if a.cnt >= 5 {
			out = append(out, Cluster{
				CellX:          k.x,
				CellY:          k.y,
				Count:          a.cnt,
				X:              a.bestX,
				Y:              a.bestY,
				SampleSpectral: a.bestSpec,
				SampleTemp:     a.bestTemp,
			})
			continue
		}
		sort.Slice(a.sparse, func(i, j int) bool {
			return s.worlds[a.sparse[i]].ID < s.worlds[a.sparse[j]].ID
		})
		for _, wi := range a.sparse {
			w := &s.worlds[wi]
			out = append(out, Cluster{
				CellX:          k.x,
				CellY:          k.y,
				Count:          1,
				X:              w.X,
				Y:              w.Y,
				SampleID:       w.ID,
				SampleName:     w.Name,
				SampleSpectral: w.Spectral,
				SampleTemp:     w.Temp,
			})
		}
	}
	if len(out) > maxClusterReturn {
		out = out[:maxClusterReturn]
	}
	return out
}