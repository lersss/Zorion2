// internal/repository/galaxy_population.go
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"math"
	"time"

	"zorion/internal/economy/settlement"
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

// galaxySettlementRow — поселение галактической сводки с привязками эффектов
// (params.effects: позиция → имя типа эффекта).
type galaxySettlementRow struct {
	id        string
	worldID   string
	data      map[string]interface{}
	raceID    string
	exact     float64
	computed  time.Time
	createdAt time.Time
	effects   map[string]string
}

// GalaxyPopulation — один запрос: поселения → планеты (физика) → миры.
// Каждое поселение пересчитывается на now в памяти (Recompute), скорость
// изменения — ChangeComponents (18a/99.2.12/99.2.13).
//
// Эффекты (спека 2026-09-22-эффекты-снабжения §5.5, читатель без прохода
// веток, С3): берётся ХРАНИМЫЙ `load` + R(load) без догона до now; нет данных
// (нет строки active_effects при привязке) → R = 0 + лог, не угадывается.
func (r *EconomyRepository) GalaxyPopulation(now time.Time) (*GalaxyPopulationReport, error) {
	rows, err := r.db.Query(`
		SELECT w.id, p.data, s.id, s.population_exact, s.computed_at, s.created_at, s.race_id,
		       pt.params->'effects'
		FROM settlements s
		JOIN planets p ON p.id = s.planet_id
		JOIN worlds w ON w.id = p.world_id
		LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id`)
	if err != nil {
		return nil, err
	}
	var list []galaxySettlementRow
	var ids []string
	for rows.Next() {
		var row galaxySettlementRow
		var dataJSON []byte
		var raceID sql.NullString
		var effectsRaw []byte
		if err := rows.Scan(&row.worldID, &dataJSON, &row.id, &row.exact, &row.computed, &row.createdAt, &raceID, &effectsRaw); err != nil {
			rows.Close()
			return nil, err
		}
		row.raceID = raceID.String
		if err := json.Unmarshal(dataJSON, &row.data); err != nil {
			rows.Close()
			return nil, err
		}
		if len(effectsRaw) > 0 {
			var effects map[string]string
			if err := json.Unmarshal(effectsRaw, &effects); err == nil {
				row.effects = effects
			}
		}
		list = append(list, row)
		ids = append(ids, row.id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stored, err := r.activeEffectsByOwner(ids)
	if err != nil {
		return nil, err
	}

	// Каталог типов эффектов нужен только для гварда по типу (§5.4, S5):
	// грузим его, лишь когда у кого-то из поселений есть привязки.
	var catalog map[string]effectTypeMeta
	for _, row := range list {
		if len(row.effects) > 0 {
			catalog, err = queryEffectTypeCatalog(context.Background(), r.db)
			if err != nil {
				return nil, err
			}
			break
		}
	}

	report := &GalaxyPopulationReport{ByWorld: map[string]int64{}, byWorldChange: map[string]float64{}}
	for _, row := range list {
		radioactivity := 0.0
		if core := parseCore(row.data); core != nil {
			radioactivity = core.Radioactivity
		}
		input := settlement.PlanetInput{
			TemperatureK:      getFloat(row.data, "temperature"),
			GravityG:          getFloat(row.data, "gravity"),
			CoreRadioactivity: radioactivity,
			// Раса поселения → расовая R-модель (99.2.23 §2.2): все пути
			// пересчёта используют active-кривые расы.
			RaceID: row.raceID,
		}
		// Минимальный путь (С3): хранимый load + R(load), без догона.
		input.Effects, input.AsOf = galaxyEffects(row, stored[row.id], catalog, now)

		rPerSec := settlement.ChangeComponents(input)
		live := settlement.Recompute(input, row.exact, row.computed, now, row.createdAt)
		pop := int64(math.Round(live))

		report.ByWorld[row.worldID] += pop
		report.Total += pop
		report.byWorldChange[row.worldID] -= live * rPerSec // убыль отрицательна
		report.changePerSec -= live * rPerSec               // убыль отрицательна
	}
	report.Trend = trendOf(report.Total, report.changePerSec)
	return report, nil
}

// galaxyEffects — сила эффектов для читателя без прохода веток (§5.5): по
// хранимому базису `load_at`, без догона до now. Нет данных (привязка есть, а
// строки active_effects нет) → R = 0 + лог, не угадывается.
//
// Гвард сирот — по ТИПУ ЭФФЕКТА, не по source_position (§5.4, S5): хранимая
// строка учитывается, только если её effect_type_id входит в текущие привязки
// владельца (params.effects → name_norm → id каталога). source_position — лишь
// первая позиция группы, сверка по нему отбросила бы живую строку.
func galaxyEffects(row galaxySettlementRow, stored []storedEffect, catalog map[string]effectTypeMeta, now time.Time) ([]settlement.EffectForcePoint, time.Time) {
	if len(row.effects) == 0 {
		return nil, now
	}
	allowed := make(map[int64]bool, len(row.effects))
	for _, typeName := range row.effects {
		if meta, ok := catalog[typeName]; ok {
			allowed[meta.ID] = true
		}
	}
	if len(stored) == 0 {
		log.Printf("⚠️ effect: у поселения %s нет строки active_effects при привязке позиций — R=0 (нет данных)", row.id)
		return nil, now
	}
	var out []settlement.EffectForcePoint
	asOf := now
	for _, se := range stored {
		if !allowed[se.effectTypeID] {
			continue // сирота: тип эффекта больше не привязан к позиции владельца
		}
		rate := settlement.EffectRate(se.impact, se.curve, se.load, settlement.BalancerCurveLookup)
		if se.loadAt.Before(asOf) {
			asOf = se.loadAt
		}
		out = append(out, settlement.EffectForcePoint{Rate: rate, Since: se.loadAt, Until: now})
	}
	return out, asOf
}

// activeEffectsByOwner — хранимые базисы нагрузки по settlement_id.
func (r *EconomyRepository) activeEffectsByOwner(ids []string) (map[string][]storedEffect, error) {
	out := map[string][]storedEffect{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(activeEffectsSelectSQL, pqStringArray(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ownerID string
		var e storedEffect
		if err := rows.Scan(&e.effectTypeID, &e.sourcePosition, &e.load, &e.loadAt, &e.impact, &e.curve, &ownerID); err != nil {
			return nil, err
		}
		out[ownerID] = append(out[ownerID], e)
	}
	return out, rows.Err()
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
