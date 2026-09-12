// internal/mapcache/mapcache_test.go
package mapcache

import (
	"context"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSnapshot(worlds ...World) *Snapshot {
	return &Snapshot{worlds: worlds}
}

// ==================== КЛАСТЕРИЗАЦИЯ ====================

// TestQueryClustersAndSingles — ячейка 5+ миров даёт кластер с позицией
// и цветом «самой яркой» звезды; ячейка 1–4 мира отдаёт каждую звезду
// отдельной точкой.
func TestQueryClustersAndSingles(t *testing.T) {
	worlds := []World{
		{ID: "a", Name: "A", X: 1, Y: 1, Spectral: "M", Temp: 3000},
		{ID: "b", Name: "B", X: 2, Y: 2, Spectral: "G", Temp: 5600},
		{ID: "c", Name: "C", X: 3, Y: 3, Spectral: "O", Temp: 42000},
		{ID: "d", Name: "D", X: 4, Y: 4, Spectral: "M", Temp: 2500},
		{ID: "e", Name: "E", X: 5, Y: 5, Spectral: "M", Temp: 3400},
		{ID: "f", Name: "F", X: 6, Y: 6, Spectral: "M", Temp: 2900},
		// ячейка (1,0): три разреженные звезды
		{ID: "g", Name: "G1", X: 11, Y: 1, Spectral: "K", Temp: 4000},
		{ID: "h", Name: "H", X: 12, Y: 2, Spectral: "K", Temp: 4500},
		{ID: "i", Name: "I", X: 13, Y: 3, Spectral: "K", Temp: 3900},
	}
	res := newSnapshot(worlds...).Query(-100, 100, -100, 100, 10, Filter{})
	require.Len(t, res, 4)

	// Кластер первым: ячейка (0,0), позиция — самая яркая звезда (O).
	c0 := res[0]
	assert.Equal(t, int64(0), c0.CellX)
	assert.Equal(t, int64(0), c0.CellY)
	assert.Equal(t, 6, c0.Count)
	assert.Equal(t, 3.0, c0.X, "позиция кластера — координаты звезды O")
	assert.Equal(t, 3.0, c0.Y)
	assert.Equal(t, "O", c0.SampleSpectral)
	assert.Equal(t, 42000.0, c0.SampleTemp, "температура кластера — температура самой яркой звезды")
	assert.Empty(t, c0.SampleID, "у кластера нет данных отдельной звезды")

	// Три одиночки ячейки (1,0), отсортированы по id.
	require.Len(t, res[1:], 3)
	for _, c := range res[1:] {
		assert.Equal(t, int64(1), c.CellX)
		assert.Equal(t, int64(0), c.CellY)
		assert.Equal(t, 1, c.Count)
		assert.Equal(t, "K", c.SampleSpectral)
		assert.NotZero(t, c.SampleTemp, "у одиночки есть температура звезды")
		assert.NotEmpty(t, c.SampleID, "у одиночки есть данные звезды")
		assert.NotEmpty(t, c.SampleName)
	}
	assert.Equal(t, "g", res[1].SampleID)
	assert.Equal(t, 4000.0, res[1].SampleTemp)
	assert.Equal(t, "h", res[2].SampleID)
	assert.Equal(t, "i", res[3].SampleID)
}

// TestQueryNegativeCellBoundaries — деление с отрицательными координатами:
// граница ячейки должна совпадать с SQL FLOOR: FLOOR(-0.5) = -1, а -10/10 -> -1.
func TestQueryNegativeCellBoundaries(t *testing.T) {
	worlds := []World{
		{ID: "w1", X: -0.5, Y: 0},
		{ID: "w2", X: 0.5, Y: 0},
		{ID: "w3", X: -10, Y: 0}, // FLOOR(-1.0) = -1
		{ID: "w4", X: 9, Y: 0},   // FLOOR(0.9) = 0
		{ID: "w5", X: -15, Y: 0}, // FLOOR(-1.5) = -2
	}
	res := newSnapshot(worlds...).Query(-100, 100, -100, 100, 10, Filter{})
	require.Len(t, res, 5)

	byCell := map[int64][]string{}
	for _, c := range res {
		byCell[c.CellX] = append(byCell[c.CellX], c.SampleID)
	}
	assert.Equal(t, []string{"w5"}, byCell[-2])
	assert.Equal(t, []string{"w1", "w3"}, byCell[-1], "w1 (-0.5) и w3 (-10) в одной ячейке -1")
	assert.Equal(t, []string{"w2", "w4"}, byCell[0])
}

// TestQueryBounds — миры вне viewport не попадают в ответ.
func TestQueryBounds(t *testing.T) {
	worlds := []World{
		{ID: "in1", X: 5, Y: 5},
		{ID: "in2", X: 6, Y: 6},
		{ID: "outX", X: -100, Y: 5},
		{ID: "outY", X: 5, Y: -100},
	}
	res := newSnapshot(worlds...).Query(0, 10, 0, 10, 5, Filter{})
	require.Len(t, res, 2)
	assert.Equal(t, "in1", res[0].SampleID)
	assert.Equal(t, "in2", res[1].SampleID)
}

// ==================== ФИЛЬТРЫ ====================

func TestQueryFilters(t *testing.T) {
	worlds := []World{
		{
			ID: "life", X: 1, Y: 1,
			HasPlanets: true, HasLife: true, HasHabitable: true,
			PlanetTypes: map[string]bool{"океан": true},
			Resources:   resourceBit("fuel") | resourceBit("water"),
		},
		{
			ID: "noLife", X: 2, Y: 2,
			HasPlanets: true, HasLife: false, HasHabitable: false,
			PlanetTypes: map[string]bool{"пустыня": true},
			Resources:   resourceBit("mineral"),
		},
		{
			ID: "noPlanets", X: 3, Y: 3, // без планет вообще
		},
	}
	s := newSnapshot(worlds...)
	bounds := []float64{-100, 100, -100, 100}

	ids := func(res []Cluster) []string {
		var out []string
		for _, c := range res {
			out = append(out, c.SampleID)
		}
		return out
	}

	assert.Equal(t, []string{"life"}, ids(s.Query(bounds[0], bounds[1], bounds[2], bounds[3], 10, Filter{HasLife: true})))
	assert.Equal(t, []string{"life"}, ids(s.Query(bounds[0], bounds[1], bounds[2], bounds[3], 10, Filter{HasHabitable: true})))
	assert.Equal(t, []string{"life", "noLife"}, ids(s.Query(bounds[0], bounds[1], bounds[2], bounds[3], 10, Filter{HasPlanets: true})))
	assert.Equal(t, []string{"life"}, ids(s.Query(bounds[0], bounds[1], bounds[2], bounds[3], 10, Filter{PlanetType: "Океан"})))
	assert.Equal(t, []string{"life"}, ids(s.Query(bounds[0], bounds[1], bounds[2], bounds[3], 10, Filter{ResourceCategory: "water"})))
	assert.Equal(t, []string{"noLife"}, ids(s.Query(bounds[0], bounds[1], bounds[2], bounds[3], 10, Filter{ResourceCategory: "mineral"})))
	// Неизвестная категория не даёт совпадений (как и в SQL).
	assert.Empty(t, ids(s.Query(bounds[0], bounds[1], bounds[2], bounds[3], 10, Filter{ResourceCategory: "nonexistent"})))
}

// ==================== ДЕТЕРМИНИЗМ И КРАЙНИЕ СЛУЧАИ ====================

// TestQueryDeterministic — порядок входных миров не влияет на результат.
func TestQueryDeterministic(t *testing.T) {
	a := []World{
		{ID: "z", X: 1, Y: 1}, {ID: "q", X: 1, Y: 1}, {ID: "b", X: 1, Y: 1},
		{ID: "m", X: 21, Y: 1}, {ID: "p", X: 22, Y: 1}, {ID: "a", X: 23, Y: 1},
	}
	b := []World{a[5], a[3], a[0], a[4], a[2], a[1]}
	assert.Equal(t,
		newSnapshot(a...).Query(-100, 100, -100, 100, 10, Filter{}),
		newSnapshot(b...).Query(-100, 100, -100, 100, 10, Filter{}),
	)
}

func TestQueryNilAndEmpty(t *testing.T) {
	var s *Snapshot
	assert.Nil(t, s.Query(0, 10, 0, 10, 5, Filter{}))

	assert.Nil(t, newSnapshot().Query(0, 10, 0, 10, 5, Filter{}))
}

// TestMaxClusterReturnCapped — ответ ограничен maxClusterReturn даже при
// микроскопических ячейках.
func TestMaxClusterReturnCapped(t *testing.T) {
	worlds := make([]World, 0, 30000)
	for i := 0; i < 30000; i++ {
		worlds = append(worlds, World{ID: string(rune('a' + i%26)) + "x", X: float64(i), Y: 1})
	}
	res := newSnapshot(worlds...).Query(-1, 30001, 0, 2, 1, Filter{})
	require.Len(t, res, maxClusterReturn)
}

// ==================== MANAGER / ГОНКИ ====================

// TestManagerConcurrentReplace — читатели видят либо старый, либо новый
// снапшот, никогда — битое состояние. Гонки ловит -race.
func TestManagerConcurrentReplace(t *testing.T) {
	m := NewManager()
	m.Replace(newSnapshot(World{ID: "old", X: 1, Y: 1, Spectral: "G"}))

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, c := range m.Snapshot().Query(-1e6, 1e6, -1e6, 1e6, 100, Filter{}) {
					if c.Count == 0 {
						t.Error("cluster with zero count")
					}
				}
			}
		}()
	}
	go func() {
		for i := 0; i < 200; i++ {
			m.Replace(newSnapshot(World{ID: "w", X: 1, Y: 1, Spectral: "M"}, World{ID: "u", X: -5, Y: 3}))
		}
		close(stop)
	}()
	wg.Wait()
}

// ==================== ЗАГРУЗКА ИЗ БД ====================

// TestLoadAndSwap — миры + сводка планет превращаются в снапшот с флагами.
func TestLoadAndSwap(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`
		SELECT id, name, coord_x, coord_y, spectral_class, temperature FROM worlds
	`).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "coord_x", "coord_y", "spectral_class", "temperature"}).
		AddRow("w1", "Alpha", 1.0, 2.0, "G", 5600).
		AddRow("w2", "Beta", 10.0, 20.0, "O", 42000))

	mock.ExpectQuery(`
		SELECT world_id, data->>'life', data->>'habitable', data->>'type', data->'resources'
		FROM planets
	`).WillReturnRows(sqlmock.NewRows([]string{"world_id", "life", "habitable", "type", "resources"}).
		AddRow("w1", "true", "true", "Газовый гигант", []byte(`{"fuel":0.55,"water":0.4}`)).
		AddRow("w1", "true", "true", "океан", []byte(`{}`)).
		AddRow("w2", nil, nil, nil, nil).
		AddRow("unknown", "true", "true", "пустыня", []byte(`{"mineral":0.4}`)))

	m := NewManager()
	require.NoError(t, m.LoadAndSwap(context.Background(), db))
	require.NoError(t, mock.ExpectationsWereMet())

	require.NotNil(t, m.Snapshot())
	assert.Equal(t, 2, m.Snapshot().Len())

	var w1, w2 *World
	for i := range m.Snapshot().worlds {
		switch m.Snapshot().worlds[i].ID {
		case "w1":
			w1 = &m.Snapshot().worlds[i]
		case "w2":
			w2 = &m.Snapshot().worlds[i]
		}
	}
	require.NotNil(t, w1)
	require.NotNil(t, w2)

	assert.True(t, w1.HasPlanets)
	assert.True(t, w1.HasLife)
	assert.True(t, w1.HasHabitable)
	assert.Equal(t, 5600.0, w1.Temp)
	assert.True(t, w1.PlanetTypes["газовый гигант"], "тип планеты хранится в нижнем регистре")
	assert.True(t, w1.PlanetTypes["океан"])
	assert.NotZero(t, w1.Resources&resourceBit("fuel"))
	assert.NotZero(t, w1.Resources&resourceBit("water"))
	assert.Zero(t, w1.Resources&resourceBit("mineral"))

	// w2 имеет планету (строка присутствует), но все поля пусты.
	assert.True(t, w2.HasPlanets)
	assert.False(t, w2.HasLife)
	assert.False(t, w2.HasHabitable)
	assert.Nil(t, w2.PlanetTypes)
	assert.Zero(t, w2.Resources)
}

// TestLoadAndSwapError — при ошибке загрузки старый снапшот остаётся в силе.
func TestLoadAndSwapError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`
		SELECT id, name, coord_x, coord_y, spectral_class, temperature FROM worlds
	`).WillReturnError(assert.AnError)

	m := NewManager()
	old := newSnapshot(World{ID: "old", X: 0, Y: 0})
	m.Replace(old)

	require.Error(t, m.LoadAndSwap(context.Background(), db))
	assert.Same(t, old, m.Snapshot(), "провал загрузки не трогает текущий снапшот")
}