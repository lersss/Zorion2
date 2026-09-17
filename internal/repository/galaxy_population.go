// internal/repository/galaxy_population.go
package repository

import (
	"database/sql"
	"encoding/json"
	"math"
	"time"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

// GalaxyPopulationReport — живая сводка населения галактики для вкладки
// «Миры» (admin_worlds.go): по каждому поселению население пересчитывается
// на текущий момент (чистая функция от чек-точки — БД не пишется, как в
// attachSettlements), агрегируется по миру и по галактике; тренд — направление
// суммарного мгновенного изменения (people/sec).
type GalaxyPopulationReport struct {
	ByWorld map[string]int64 // живое население мира (сумма по поселениям)
	Total   int64            // живое население галактики
	Trend   string           // см. константы тренда ниже

	// byWorldChange — суммарное мгновенное изменение по миру, люди/сек
	// (убыль < 0). Не сериализуется — для тренда мира в админке «Миры».
	byWorldChange map[string]float64

	// changePerSec — суммарное мгновенное изменение, люди/сек (убыль < 0).
	// Не сериализуется — внутри отчёта для тренда.
	changePerSec float64
}

// Направления тренда (вкладка «Миры»). Рост даёт рождаемость (99.2.16:
// r < 0 при k > 1 в комфорте) — тренд считается честно от мгновенного
// изменения, а не от истории (истории нет).
const (
	TrendDecline = "decline" // ↓ население убывает
	TrendGrowth  = "growth"  // ↑ население растёт
	TrendStable  = "stable"  // — без изменений
)

// GalaxyPopulation — один запрос: поселения → планеты (физика) → миры.
// Каждое поселение пересчитывается на now в памяти (Recompute), скорость
// изменения — ChangeComponents (18a/99.2.12/99.2.13).
func (r *EconomyRepository) GalaxyPopulation(now time.Time) (*GalaxyPopulationReport, error) {
	rows, err := r.db.Query(`
		SELECT w.id, p.data, s.population_exact, s.computed_at, s.created_at, s.race_id
		FROM settlements s
		JOIN planets p ON p.id = s.planet_id
		JOIN worlds w ON w.id = p.world_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	report := &GalaxyPopulationReport{ByWorld: map[string]int64{}, byWorldChange: map[string]float64{}}
	for rows.Next() {
		var worldID string
		var dataJSON []byte
		var s models.Settlement
		var raceID sql.NullString
		if err := rows.Scan(&worldID, &dataJSON, &s.PopulationExact, &s.ComputedAt, &s.CreatedAt, &raceID); err != nil {
			return nil, err
		}
		var data map[string]interface{}
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			return nil, err
		}

		radioactivity := 0.0
		if core := parseCore(data); core != nil {
			radioactivity = core.Radioactivity
		}
		input := settlement.PlanetInput{
			TemperatureK:      getFloat(data, "temperature"),
			GravityG:          getFloat(data, "gravity"),
			CoreRadioactivity: radioactivity,
			// Раса поселения → расовая R-модель (99.2.23 §2.2): все пути
			// пересчёта используют active-кривые расы.
			RaceID: raceID.String,
		}

		rPerSec := settlement.ChangeComponents(input)
		live := settlement.Recompute(input, s.PopulationExact, s.ComputedAt, now, s.CreatedAt)
		pop := int64(math.Round(live))

		report.ByWorld[worldID] += pop
		report.Total += pop
		report.byWorldChange[worldID] -= live * rPerSec // убыль отрицательна
		report.changePerSec -= live * rPerSec // убыль отрицательна
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	report.Trend = trendOf(report.Total, report.changePerSec)
	return report, nil
}

// trendOf — направление тренда по суммарному мгновенному изменению.
// Пустая галактика (поселений нет) — «stable», а не «decline».
func trendOf(total int64, changePerSec float64) string {
	if total == 0 {
		return TrendStable
	}
	const eps = 1e-9 // гвард от шумов плавающей точки
	switch {
	case changePerSec < -eps:
		return TrendDecline
	case changePerSec > eps:
		return TrendGrowth
	default:
		return TrendStable
	}
}

// WorldTrend — тренд населения конкретного мира (админка «Миры», колонка
// «Население»): по живому населению мира и его мгновенному изменению.
// Мир без поселений — «stable» (пусто, а не убыль).
func (r *GalaxyPopulationReport) WorldTrend(worldID string) string {
	return trendOf(r.ByWorld[worldID], r.byWorldChange[worldID])
}