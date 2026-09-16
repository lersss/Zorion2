// internal/generator/settlement/races.go — генерация поселений рас
// (спека 99.2.21 §7, идея 56a «Механика заселения»).
//
// Отдельный проход от человеческого GenerateSettlements: доминантная раса
// кластера + подселение соседней расы на выбросах. R-модель рас
// (reproduction/resilience) НЕ реализуется — будущая сессия (спека §14);
// стартовые population/stability — как у человеческих поселений.
package settlement

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"zorion/internal/models"
	"zorion/internal/races"
)

// RaceGenConfig — параметры генерации поселений рас.
type RaceGenConfig struct {
	// NeighborChance — шанс заселения соседней расы на выбросе (0–1).
	// Умножается на отношение расстояний d1/d2: 0 у центра кластера,
	// максимум (крутилка) у середины между кластерами.
	NeighborChance float64
	// Chance — шанс заселения доминантной расы кластера (0–1, дефолт 1.0):
	// доминанта селится, если пригодна И ролл < Chance (65a, перенос из
	// старого генератора поселений).
	Chance float64
	// Population — стратегия населения поселений рас (fixed/random, 65a,
	// перенос из старого генератора). Дефолт — как DefaultModel
	// (random 100k–1B), заполняется в GenerateRaceSettlements.
	Population Population
}

// Validate — проверяет конфиг перед генерацией (валидация как в
// Model.Validate, 65a: chance/neighbor_chance 0–1, population по стратегии).
func (c RaceGenConfig) Validate() error {
	if c.Chance < 0 || c.Chance > 1 {
		return fmt.Errorf("chance: должно быть от 0 до 1, получил %v", c.Chance)
	}
	if c.NeighborChance < 0 || c.NeighborChance > 1 {
		return fmt.Errorf("neighbor_chance: должно быть от 0 до 1, получил %v", c.NeighborChance)
	}
	return c.Population.Validate()
}

// clusterEdgeFactor — граница территории кластера: точки в пределах
// 1.25×радиус от центра — планеты кластера (clusterEdgeAccept в poisson.go),
// дальше — выбросы (межкластерные звёзды).
const clusterEdgeFactor = 1.25

// GenerateRaceSettlements — отдельный проход генерации поселений рас.
// Возвращает число поселений и список рас без поселений (0 из 50) —
// копилка для разбора причин (идея 56a: «потом будем по каждой выяснять»).
func (g *Generator) GenerateRaceSettlements(ctx context.Context, cfg RaceGenConfig, progressFn func(processed int)) (int, []string, error) {
	if cfg.Population.Kind == "" {
		// Дефолт населения — как DefaultModel (random 100k–1B, 65a).
		cfg.Population = Population{Kind: "random", Min: 100_000, Max: 1_000_000_000}
	}
	regions, err := g.loadRaceRegions(ctx)
	if err != nil {
		return 0, nil, err
	}
	if len(regions) == 0 {
		// Регионов с расой нет (легаси-вселенная) — все расы без поселений.
		return 0, allRaceIDs(), nil
	}

	rows, err := g.db.QueryContext(ctx, `SELECT p.id, p.data, w.coord_x, w.coord_y FROM planets p JOIN worlds w ON w.id = p.world_id`)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var settlementRows []interface{}
	settledRaces := map[string]bool{}
	settled := 0
	processed := 0

	for rows.Next() {
		var id string
		var dataJSON []byte
		var cx, cy float64
		if err := rows.Scan(&id, &dataJSON, &cx, &cy); err != nil {
			return 0, nil, err
		}
		processed++
		if progressFn != nil {
			progressFn(processed)
		}

		var data map[string]interface{}
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			continue
		}

		for _, raceID := range decideRaceSettlements(data, cx, cy, regions, cfg, g.rng) {
			settlementRows = append(settlementRows, buildRaceSettlement(id, raceID, g.rng, cfg.Population))
			settledRaces[raceID] = true
			settled++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	if settled == 0 {
		return 0, unsettledRaces(settledRaces), nil
	}

	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()

	if err := copyInRows(tx, "settlements",
		[]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "race_id"},
		flatten(settlementRows), 7); err != nil {
		return 0, nil, fmt.Errorf("copy race settlements: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return settled, unsettledRaces(settledRaces), nil
}

// decideRaceSettlements — какие расы поселяются на планете (0–2):
// доминанта кластера (если пригодна и ролл < Chance) + на выбросе сосед
// с шансом из крутилки. Приоритет всегда у доминантной расы кластера
// (идея 56a). Chance (65a) — шанс заселения доминанты: 0 → доминанта не
// селится даже на пригодной планете.
func decideRaceSettlements(data map[string]interface{}, cx, cy float64, regions []*models.Region, cfg RaceGenConfig, rng *rand.Rand) []string {
	idx1, idx2, d1, d2 := twoNearestRegions(cx, cy, regions)
	if idx1 < 0 {
		return nil
	}
	dominant := races.ByID(regions[idx1].RaceID)
	if dominant == nil {
		return nil
	}

	var out []string
	if dominant.Suitable(data) && rng.Float64() < cfg.Chance {
		out = append(out, dominant.ID)
	}

	// Выброс (межкластерная зона): доминанта + шанс расы ближайшего
	// соседнего кластера. Чем ближе к центру своего кластера, тем меньше
	// шанс (пропорционально отношению расстояний d1/d2; у середины между
	// кластерами d1 = d2 → шанс = крутилка).
	if d1 > clusterEdgeFactor*regions[idx1].Radius && idx2 >= 0 {
		neighbor := races.ByID(regions[idx2].RaceID)
		if neighbor != nil && neighbor.ID != dominant.ID {
			chance := cfg.NeighborChance * (d1 / d2)
			if chance > 1 {
				chance = 1
			}
			if rng.Float64() < chance && neighbor.Suitable(data) {
				out = append(out, neighbor.ID)
			}
		}
	}
	return out
}

// loadRaceRegions — регионы с назначенной расой (regions.race_id).
func (g *Generator) loadRaceRegions(ctx context.Context) ([]*models.Region, error) {
	rows, err := g.db.QueryContext(ctx, `SELECT id, center_x, center_y, radius, race_id FROM regions WHERE race_id IS NOT NULL AND race_id != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	regions := make([]*models.Region, 0, 256)
	for rows.Next() {
		var r models.Region
		var raceID sql.NullString
		if err := rows.Scan(&r.ID, &r.CenterX, &r.CenterY, &r.Radius, &raceID); err != nil {
			return nil, err
		}
		r.RaceID = raceID.String
		regions = append(regions, &r)
	}
	return regions, rows.Err()
}

// twoNearestRegions — индексы двух ближайших регионов к точке и расстояния
// до них (второй ближайший — «соседний кластер» для подселения на выбросе).
func twoNearestRegions(x, y float64, regions []*models.Region) (idx1, idx2 int, d1, d2 float64) {
	idx1, idx2 = -1, -1
	d1, d2 = math.Inf(1), math.Inf(1)
	for i, r := range regions {
		dx := r.CenterX - x
		dy := r.CenterY - y
		d := dx*dx + dy*dy
		if d < d1 {
			d2, idx2 = d1, idx1
			d1, idx1 = d, i
		} else if d < d2 {
			d2, idx2 = d, i
		}
	}
	if idx1 >= 0 {
		d1 = math.Sqrt(d1)
	}
	if idx2 >= 0 {
		d2 = math.Sqrt(d2)
	}
	return idx1, idx2, d1, d2
}

// buildRaceSettlement — строка поселения расы для вставки в БД.
// Стартовые population/stability — как у человеческих (buildSettlement);
// население — по стратегии конфига (fixed/random, 65a), дефолт
// random 100k–1B.
func buildRaceSettlement(planetID, raceID string, rng *rand.Rand, pop Population) []interface{} {
	population := pop.Value(rng)
	return []interface{}{
		uuid.New().String(),
		planetID,
		population,
		float64(population),
		rng.Intn(41) + 40,
		time.Now(),
		raceID,
	}
}

// unsettledRaces — расы каталога без поселений (отчёт).
func unsettledRaces(settled map[string]bool) []string {
	var out []string
	for _, r := range races.Catalog() {
		if !settled[r.ID] {
			out = append(out, r.ID)
		}
	}
	return out
}

// allRaceIDs — все расы каталога (для отчёта, когда регионов с расой нет).
func allRaceIDs() []string {
	var out []string
	for _, r := range races.Catalog() {
		out = append(out, r.ID)
	}
	return out
}