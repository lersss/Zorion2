// internal/repository/planet_repo.go
package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

type PlanetRepository struct {
	db *sql.DB
}

func NewPlanetRepository(db *sql.DB) *PlanetRepository {
	return &PlanetRepository{db: db}
}

// GetPlanetsByWorldID — возвращает планеты мира с полной структурой,
// включая композиции, ядро, спутники.
func (r *PlanetRepository) GetPlanetsByWorldID(worldID string) ([]models.Planet, error) {
	query := `
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE world_id = $1
		ORDER BY orbit_index ASC
	`
	rows, err := r.db.Query(query, worldID)
	if err != nil {
		return nil, fmt.Errorf("failed to query planets: %w", err)
	}
	defer rows.Close()

	var planets []models.Planet
	for rows.Next() {
		var p models.Planet
		var dataJSON []byte
		err := rows.Scan(
			&p.ID, &p.WorldID, &p.Name, &p.OrbitIndex,
			&dataJSON, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan planet: %w", err)
		}

		var data map[string]interface{}
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			log.Printf("❌ JSON unmarshal error for planet %s: %v", p.ID, err)
			return nil, fmt.Errorf("failed to unmarshal planet data: %w", err)
		}

		populatePlanetFromJSON(&p, data)
		planets = append(planets, p)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	if err := r.attachSettlements(planets); err != nil {
		return nil, err
	}
	return planets, nil
}

// GetPlanetByID — возвращает одну планету с полной структурой. Планета не
// найдена — (nil, nil), а не ошибка (соглашение проекта, см. WorldRepository.GetByID).
func (r *PlanetRepository) GetPlanetByID(id string) (*models.Planet, error) {
	query := `
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE id = $1
	`
	var p models.Planet
	var dataJSON []byte
	err := r.db.QueryRow(query, id).Scan(
		&p.ID, &p.WorldID, &p.Name, &p.OrbitIndex,
		&dataJSON, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query planet: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(dataJSON, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal planet data: %w", err)
	}
	populatePlanetFromJSON(&p, data)

	planets := []models.Planet{p}
	if err := r.attachSettlements(planets); err != nil {
		return nil, err
	}
	return &planets[0], nil
}

// attachSettlements — подтягивает поселения планет, пересчитывает их
// население от среды на текущий момент (docs/gamedesign/18a_population_death.md
// — открытие карточки планеты игроком триггерит ленивый пересчёт,
// 13_tiers_impl.md §13.13.3), вычисляет население планеты как сумму
// пересчитанного и обитаемость как наличие поселения. Частые просмотры
// (Δt < MinPersistInterval) пересчитывают только в памяти; запись в БД
// происходит лишь по «событию» — при содержательно прошедшем времени.
// Планеты без поселений: население 0, необитаемы.
func (r *PlanetRepository) attachSettlements(planets []models.Planet) error {
	if len(planets) == 0 {
		return nil
	}

	ids := make([]string, 0, len(planets))
	for _, p := range planets {
		ids = append(ids, p.ID)
	}

	byPlanet, err := NewEconomyRepository(r.db).GetSettlementsByPlanetIDs(ids)
	if err != nil {
		return fmt.Errorf("failed to load settlements: %w", err)
	}

	econRepo := NewEconomyRepository(r.db)
	now := time.Now()
	for i := range planets {
		settlements := byPlanet[planets[i].ID]
		input := planetMortalityInput(planets[i])
		for j := range settlements {
			updated, err := econRepo.RecomputeSettlementPopulation(&settlements[j], input, now)
			if err != nil {
				return fmt.Errorf("failed to recompute settlement %s: %w", settlements[j].ID, err)
			}
			settlements[j] = updated
		}
		planets[i].Settlements = settlements
		planets[i].Habitable = len(settlements) > 0
		for _, s := range settlements {
			planets[i].Population += int64(s.Population)
		}
	}
	return nil
}

// planetMortalityInput — физика планеты для пересчёта смерти населения от
// среды. Радиоактивность — 0, если у планеты нет ядра (например, спутник
// газового гиганта пока не учитывается на этом срезе).
func planetMortalityInput(p models.Planet) settlement.PlanetInput {
	radioactivity := 0.0
	if p.Core != nil {
		radioactivity = p.Core.Radioactivity
	}
	return settlement.PlanetInput{
		TemperatureK:      p.Temperature,
		GravityG:          p.Gravity,
		CoreRadioactivity: radioactivity,
	}
}

// ==================== ПАРСИНГ ====================

// populatePlanetFromJSON — заполняет модель Planet из map JSON.
func populatePlanetFromJSON(p *models.Planet, data map[string]interface{}) {
	// Идентификация и типы
	p.Type = getStr(data, "type")
	p.SurfaceDominant = getStr(data, "surface_dominant")
	p.Archetype = getStr(data, "archetype")

	// Физика
	p.Size = getFloat(data, "size")
	p.Mass = getFloat(data, "mass")
	p.Density = getFloat(data, "density")
	p.Temperature = getFloat(data, "temperature")
	p.Gravity = getFloat(data, "gravity")
	p.WaterPercent = getFloat(data, "water_percent")

	// Атмосфера и биосфера
	p.Atmosphere = getStr(data, "atmosphere")
	p.Hydrosphere = getStr(data, "hydrosphere")
	p.Biosphere = getStr(data, "biosphere")

	// Жизнь
	p.Life = getBool(data, "life")

	// Композиции
	p.SurfaceComposition = getFloatMap(data, "surface_composition")
	p.SubterrainComposition = getFloatMap(data, "subterrain_composition")

	// Ядро
	p.Core = parseCore(data)

	// Газовый гигант и спутники
	p.IsGasGiant = getBool(data, "is_gas_giant")
	p.Satellites = parseSatellites(data)

	// Прочее
	p.Description = getStr(data, "description")
	p.SystemAge = getFloat(data, "system_age")
	p.Moons = int(getFloat(data, "moons"))
	p.Radioactive = getBool(data, "radioactive")
}

// parseCore — читает ядро из JSON.
func parseCore(data map[string]interface{}) *models.PlanetCore {
	raw, ok := data["core"].(map[string]interface{})
	if !ok {
		return nil
	}
	return &models.PlanetCore{
		Type:          getStr(raw, "type"),
		MassPercent:   getFloat(raw, "mass_percent"),
		Activity:      getFloat(raw, "activity"),
		Radioactivity: getFloat(raw, "radioactivity"),
		Age:           getFloat(raw, "age"),
		IsActive:      getBool(raw, "is_active"),
		IsMetallic:    getBool(raw, "is_metallic"),
	}
}

// parseSatellites — читает спутники газового гиганта из JSON.
func parseSatellites(data map[string]interface{}) []models.PlanetSatellite {
	raw, ok := data["satellites"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]models.PlanetSatellite, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, models.PlanetSatellite{
			ID:                    getStr(m, "id"),
			Name:                  getStr(m, "name"),
			OrbitIndex:            int(getFloat(m, "orbit_index")),
			Size:                  getFloat(m, "size"),
			Mass:                  getFloat(m, "mass"),
			Temperature:           getFloat(m, "temperature"),
			WaterPercent:          getFloat(m, "water_percent"),
			Habitable:             getBool(m, "habitable"),
			Life:                  getBool(m, "life"),
			Atmosphere:            getStr(m, "atmosphere"),
			Biosphere:             getStr(m, "biosphere"),
			SurfaceDominant:       getStr(m, "surface_dominant"),
			SurfaceComposition:    getFloatMap(m, "surface_composition"),
			SubterrainComposition: getFloatMap(m, "subterrain_composition"),
			Description:           getStr(m, "description"),
		})
	}
	return result
}

// ==================== ХЕЛПЕРЫ ====================

func getStr(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

func getFloat(data map[string]interface{}, key string) float64 {
	if v, ok := data[key].(float64); ok {
		return v
	}
	return 0
}

func getBool(data map[string]interface{}, key string) bool {
	if v, ok := data[key].(bool); ok {
		return v
	}
	return false
}

func getFloatMap(data map[string]interface{}, key string) map[string]float64 {
	raw, ok := data[key].(map[string]interface{})
	if !ok {
		return map[string]float64{}
	}
	result := make(map[string]float64, len(raw))
	for k, v := range raw {
		if f, ok := v.(float64); ok {
			result[k] = f
		}
	}
	return result
}
