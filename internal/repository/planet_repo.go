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
	"zorion/internal/races"
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
	if err := r.attachFactionsAndBuildings(planets); err != nil {
		return nil, err
	}
	// Залежи — только здесь: GetPlanetsByWorldID кормит единственный путь,
	// сериализуемый игроку (GET /api/worlds/{id}/planets) и проходящий
	// applyPlanetVisibility → stripPlanetDetails (§5.1 спеки залежей).
	if err := r.attachDeposits(planets); err != nil {
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
	if err := r.attachFactionsAndBuildings(planets); err != nil {
		return nil, err
	}
	return &planets[0], nil
}

// GetPlanetsLightByWorldID — планеты системы БЕЗ поселений (спека 99.2.27
// §3.4): валидация цели внутрисистемного полёта + радиус орбиты. Лёгкий
// вариант GetPlanetsByWorldID — без attachSettlements (пересчёт населения
// для полёта не нужен и дорог).
func (r *PlanetRepository) GetPlanetsLightByWorldID(worldID string) ([]models.Planet, error) {
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
		if err := rows.Scan(
			&p.ID, &p.WorldID, &p.Name, &p.OrbitIndex,
			&dataJSON, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan planet: %w", err)
		}
		var data map[string]interface{}
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal planet data: %w", err)
		}
		populatePlanetFromJSON(&p, data)
		planets = append(planets, p)
	}
	return planets, rows.Err()
}

// GetBeltsByWorldID — пояса малых тел мира (спека
// 2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт §4.1): читающая ручка
// этапа 2 (в этапе 1 генератор писал через COPY). Возвращает все записи
// system_belts мира (фильтр visible — на стороне хендлера, §4.2); порядок —
// по radius_au (внешний к внутреннему нет — стабильный показ).
func (r *PlanetRepository) GetBeltsByWorldID(worldID string) ([]models.Belt, error) {
	query := `
		SELECT id, world_id, kind, name, orbit_index, radius_au, width_au, mass,
		       body_size_km, composition, visible, data, iron_remaining, created_at, updated_at
		FROM system_belts
		WHERE world_id = $1
		ORDER BY radius_au ASC
	`
	rows, err := r.db.Query(query, worldID)
	if err != nil {
		return nil, fmt.Errorf("failed to query belts: %w", err)
	}
	defer rows.Close()

	var belts []models.Belt
	for rows.Next() {
		var b models.Belt
		var orbitIndex sql.NullInt64
		var ironRemaining sql.NullFloat64
		var compJSON, dataJSON []byte
		if err := rows.Scan(
			&b.ID, &b.WorldID, &b.Kind, &b.Name, &orbitIndex, &b.RadiusAU,
			&b.WidthAU, &b.Mass, &b.BodySizeKm, &compJSON, &b.Visible, &dataJSON,
			&ironRemaining, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan belt: %w", err)
		}
		if orbitIndex.Valid {
			idx := int(orbitIndex.Int64)
			b.OrbitIndex = &idx
		}
		if ironRemaining.Valid {
			v := ironRemaining.Float64
			b.IronRemaining = &v
		}
		if len(compJSON) > 0 {
			if err := json.Unmarshal(compJSON, &b.Composition); err != nil {
				return nil, fmt.Errorf("failed to unmarshal belt composition: %w", err)
			}
		}
		if len(dataJSON) > 0 {
			if err := json.Unmarshal(dataJSON, &b.Data); err != nil {
				return nil, fmt.Errorf("failed to unmarshal belt data: %w", err)
			}
		}
		belts = append(belts, b)
	}
	return belts, rows.Err()
}

// GetBeltByID — один пояс по id (спека поясов этап 3 §7): источник состава и
// запаса для пакета захода/сбора. Не найден — (nil, nil).
func (r *PlanetRepository) GetBeltByID(beltID string) (*models.Belt, error) {
	query := `
		SELECT id, world_id, kind, name, orbit_index, radius_au, width_au, mass,
		       body_size_km, composition, visible, data, iron_remaining, created_at, updated_at
		FROM system_belts
		WHERE id = $1
	`
	var b models.Belt
	var orbitIndex sql.NullInt64
	var ironRemaining sql.NullFloat64
	var compJSON, dataJSON []byte
	err := r.db.QueryRow(query, beltID).Scan(
		&b.ID, &b.WorldID, &b.Kind, &b.Name, &orbitIndex, &b.RadiusAU,
		&b.WidthAU, &b.Mass, &b.BodySizeKm, &compJSON, &b.Visible, &dataJSON,
		&ironRemaining, &b.CreatedAt, &b.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query belt by id: %w", err)
	}
	if orbitIndex.Valid {
		idx := int(orbitIndex.Int64)
		b.OrbitIndex = &idx
	}
	if ironRemaining.Valid {
		v := ironRemaining.Float64
		b.IronRemaining = &v
	}
	if len(compJSON) > 0 {
		if err := json.Unmarshal(compJSON, &b.Composition); err != nil {
			return nil, fmt.Errorf("failed to unmarshal belt composition: %w", err)
		}
	}
	if len(dataJSON) > 0 {
		if err := json.Unmarshal(dataJSON, &b.Data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal belt data: %w", err)
		}
	}
	return &b, nil
}

// LockBeltForUpdate — строка пояса под блокировкой (спека поясов этап 3
// §5.3/§6.2): сериализация списания запаса и ленивой инициализации. Вызывается
// внутри транзакции вызывающего. Не найден — (nil, nil).
func (r *PlanetRepository) LockBeltForUpdate(tx *sql.Tx, beltID string) (*models.Belt, error) {
	query := `
		SELECT id, world_id, kind, name, composition, visible, iron_remaining
		FROM system_belts
		WHERE id = $1 FOR UPDATE
	`
	var b models.Belt
	var compJSON []byte
	var ironRemaining sql.NullFloat64
	err := tx.QueryRow(query, beltID).Scan(
		&b.ID, &b.WorldID, &b.Kind, &b.Name, &compJSON, &b.Visible, &ironRemaining,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock belt: %w", err)
	}
	if ironRemaining.Valid {
		v := ironRemaining.Float64
		b.IronRemaining = &v
	}
	if len(compJSON) > 0 {
		if err := json.Unmarshal(compJSON, &b.Composition); err != nil {
			return nil, fmt.Errorf("failed to unmarshal belt composition: %w", err)
		}
	}
	return &b, nil
}

// SetBeltIronRemaining — запись запаса пояса (спека поясов этап 3 §4): ленивая
// инициализация при первом обращении. Внутри транзакции вызывающего.
func (r *PlanetRepository) SetBeltIronRemaining(tx *sql.Tx, beltID string, value float64) error {
	_, err := tx.Exec(
		`UPDATE system_belts SET iron_remaining = $1, updated_at = NOW() WHERE id = $2`,
		value, beltID,
	)
	if err != nil {
		return fmt.Errorf("failed to set belt iron_remaining: %w", err)
	}
	return nil
}

// FindPlanetBySatellite — родительская планета спутника (спека 99.2.27 §3.6):
// спутник живёт в planets.data.satellites (UUID, models.PlanetSatellite.ID).
// Не найдена — (nil, nil).
func (r *PlanetRepository) FindPlanetBySatellite(worldID, satelliteID string) (*models.Planet, error) {
	query := `
		SELECT id, world_id, name, orbit_index, data, created_at, updated_at
		FROM planets
		WHERE world_id = $1
		  AND EXISTS (SELECT 1 FROM jsonb_array_elements(data->'satellites') sat WHERE sat->>'id' = $2)
	`
	var p models.Planet
	var dataJSON []byte
	err := r.db.QueryRow(query, worldID, satelliteID).Scan(
		&p.ID, &p.WorldID, &p.Name, &p.OrbitIndex,
		&dataJSON, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query planet by satellite: %w", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(dataJSON, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal planet data: %w", err)
	}
	populatePlanetFromJSON(&p, data)
	return &p, nil
}

// attachSettlements — подтягивает поселения планет и выполняет owner-проход
// «производство (ветки) → потребность → население» (спека 2026-09-22-эффекты-
// снабжения §4.1/§4.5; docs/gamedesign/18a_population_death.md). Открытие
// карточки триггерит ленивый проход: частые просмотры (Δt < MinPersistInterval
// у всех веток) считают в памяти, персистентный путь пишет одной транзакцией
// на поселение (advisory-лок, один now → load_at == processed_at == computed_at).
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

	now := time.Now()
	owners := make([]OwnerSettlement, 0, len(planets))
	settlementIDs := make([]string, 0, len(planets))
	for i := range planets {
		settlements := byPlanet[planets[i].ID]
		input := planetMortalityInput(planets[i])
		for j := range settlements {
			s := settlements[j]
			owners = append(owners, OwnerSettlement{
				ID:                s.ID,
				PlanetID:          planets[i].ID,
				Population:        s.Population,
				PopulationExact:   s.PopulationExact,
				ComputedAt:        s.ComputedAt,
				CreatedAt:         s.CreatedAt,
				RaceID:            s.RaceID,
				Planet:            input,
				EatByPosition:     s.EatByPosition,
				EffectsByPosition: s.EffectsByPosition,
			})
			settlementIDs = append(settlementIDs, s.ID)
		}
		planets[i].Settlements = settlements
		planets[i].Habitable = len(settlements) > 0
	}

	results, err := NewBranchRepository(r.db).SyncSettlements(now, owners)
	if err != nil {
		return fmt.Errorf("failed to run settlement owner pass: %w", err)
	}

	for i := range planets {
		planets[i].Population = 0
		for j := range planets[i].Settlements {
			s := &planets[i].Settlements[j]
			res, ok := results[s.ID]
			if !ok {
				continue
			}
			s.Population = res.Population
			s.PopulationExact = res.PopulationExact
			s.ComputedAt = res.ComputedAt
			s.RPerSec = res.RPerSec
			s.NDead = res.NDead
			s.Branches = res.Branches
			s.Effects = res.Effects
			s.RaceName = raceName(s.RaceID)
			planets[i].Population += int64(s.Population)
		}
	}

	// Лог поселения (записи «Вымерло»): один запрос на все поселения, последние
	// 3 записи на поселение (18b §«UI», settlements[].log).
	logBySettlement, err := NewEconomyRepository(r.db).GetSettlementLogBySettlementIDs(settlementIDs)
	if err != nil {
		return fmt.Errorf("failed to load settlement log: %w", err)
	}
	for i := range planets {
		for j := range planets[i].Settlements {
			planets[i].Settlements[j].Log = logBySettlement[planets[i].Settlements[j].ID]
		}
	}
	return nil
}

// attachFactionsAndBuildings — подтягивает фракции (по factions.homeworld_id)
// и строения (по buildings.planet_id) планет системы (спека
// 2026-09-21-фабрики-релиз-2-столицы-фракций §6). Два запроса на систему
// (`= ANY($1)`), без обхода галактики (инвариант 2). У планет без записей —
// массивы пустые (в JSON скрыты omitempty).
func (r *PlanetRepository) attachFactionsAndBuildings(planets []models.Planet) error {
	if len(planets) == 0 {
		return nil
	}

	ids := make([]string, 0, len(planets))
	index := make(map[string]int, len(planets))
	for i := range planets {
		ids = append(ids, planets[i].ID)
		index[planets[i].ID] = i
	}

	rows, err := r.db.Query(`
		SELECT id, name, type, color, description, homeworld_id
		FROM factions WHERE homeworld_id = ANY($1) ORDER BY name ASC
	`, pqStringArray(ids))
	if err != nil {
		return fmt.Errorf("failed to load factions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var f models.PlanetFaction
		var homeworldID string
		var color, description sql.NullString
		if err := rows.Scan(&f.ID, &f.Name, &f.Type, &color, &description, &homeworldID); err != nil {
			return fmt.Errorf("failed to scan faction: %w", err)
		}
		f.Color = color.String
		f.Description = description.String
		if i, ok := index[homeworldID]; ok {
			planets[i].Factions = append(planets[i].Factions, f)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("factions iteration error: %w", err)
	}

	brows, err := r.db.Query(`
		SELECT id, planet_id, building_type, owner_type, owner_id
		FROM buildings WHERE planet_id = ANY($1) ORDER BY building_type ASC, id ASC
	`, pqStringArray(ids))
	if err != nil {
		return fmt.Errorf("failed to load buildings: %w", err)
	}
	defer brows.Close()
	for brows.Next() {
		var b models.PlanetBuilding
		var planetID string
		if err := brows.Scan(&b.ID, &planetID, &b.BuildingType, &b.OwnerType, &b.OwnerID); err != nil {
			return fmt.Errorf("failed to scan building: %w", err)
		}
		if i, ok := index[planetID]; ok {
			planets[i].Buildings = append(planets[i].Buildings, b)
		}
	}
	return brows.Err()
}

// attachDeposits — подтягивает залежи планет системы (спека 2026-09-22-
// поселение-добыча-сырья-биома-ленивый-буфер §5.1). Один запрос на систему.
//
// ИНВАРИАНТ «залежи не утекают»: вызывать attachDeposits можно ровно в
// GetPlanetsByWorldID — только этот путь отдаётся игроку и фильтруется
// stripPlanetDetails. Light-пути (GetPlanetsLightByWorldID, GetPlanetByID)
// залежи НЕ несут: результат игроку не сериализуется.
func (r *PlanetRepository) attachDeposits(planets []models.Planet) error {
	if len(planets) == 0 {
		return nil
	}
	ids := make([]string, 0, len(planets))
	for i := range planets {
		ids = append(ids, planets[i].ID)
	}
	byPlanet, err := NewDepositRepository(r.db).GetDepositsByPlanetIDs(ids)
	if err != nil {
		return fmt.Errorf("failed to load deposits: %w", err)
	}
	for i := range planets {
		planets[i].Deposits = byPlanet[planets[i].ID]
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

// raceName — человекочитаемое имя расы поселения: из каталога рас
// (internal/races); пустой RaceID (NULL = легаси/люди, спека 99.2.21 §2.3) —
// «Люди». Неизвестный ключ — показываем как есть (защита от битого каталога).
func raceName(raceID string) string {
	if raceID == "" {
		return "Люди"
	}
	if r := races.ByID(raceID); r != nil {
		return r.Name
	}
	return raceID
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

	// Атмосфера-объект и новые поля каскада (99.2.20 §4.1): старые планеты
	// без ключей — нули/false (фолбэки §7).
	p.AtmosphereData = getMap(data, "atmosphere_data")
	p.LiquidWaterPossible = getBool(data, "liquid_water_possible")
	p.OrbitalPeriod = getFloat(data, "orbital_period")
	p.Eccentricity = getFloat(data, "eccentricity")
	p.EscapeVelocity = getFloat(data, "escape_velocity")
	p.TidalLock = getBool(data, "tidal_lock")

	// Жизнь
	p.Life = getBool(data, "life")

	// Орбитальный контекст (35b §2.2): P/S-тип. Фолбэки §2.4: нет ключа —
	// orbit_center="main", circumbinary=false; радиус не трогаем (фронт
	// считает из orbit_index, если поле отсутствует).
	p.OrbitCenter = getStr(data, "orbit_center")
	if p.OrbitCenter == "" {
		p.OrbitCenter = "main"
	}
	p.OrbitRadiusAU = getFloat(data, "orbit_radius_au")
	p.Circumbinary = getBool(data, "circumbinary")

	// Композиции
	p.SurfaceComposition = getFloatMap(data, "surface_composition")
	p.SubterrainComposition = getFloatMap(data, "subterrain_composition")

	// Биомы и зоны недр объектами (99.2.28 §9.3): отсутствие ключа → nil
	// (старый мир), не ошибка.
	p.Biomes = parseBiomes(data)
	p.Subterrain = parseSubterrain(data)

	// История формирования (спека 2026-09-22-облако-этап-2-... §6.5):
	// отсутствие ключа → nil (старый мир), не ошибка.
	p.FormationHistory = parseFormationHistory(data)

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

// parseBiomes — читает биомы поверхности из JSON (99.2.28 §9.3).
// Отсутствие ключа → nil (старый мир), не ошибка.
func parseBiomes(data map[string]interface{}) []models.Biome {
	raw, ok := data["biomes"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]models.Biome, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, models.Biome{
			Form:  getStr(m, "form"),
			Share: getFloat(m, "share"),
		})
	}
	return result
}

// parseSubterrain — читает зоны недр из JSON (99.2.28 §9.3).
// Отсутствие ключа → nil (старый мир), не ошибка.
func parseSubterrain(data map[string]interface{}) []models.SubterrainZone {
	raw, ok := data["subterrain"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]models.SubterrainZone, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, models.SubterrainZone{
			Type:  getStr(m, "type"),
			Share: getFloat(m, "share"),
		})
	}
	return result
}

// parseFormationHistory — читает историю формирования из JSON (спека
// 2026-09-22-облако-этап-2-... §6.5). Отсутствие ключа → nil (старый мир),
// не ошибка; записи без типа пропускаются.
func parseFormationHistory(data map[string]interface{}) []models.PlanetFormationEvent {
	raw, ok := data["formation_history"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]models.PlanetFormationEvent, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		typ := getStr(m, "type")
		if typ == "" {
			continue
		}
		result = append(result, models.PlanetFormationEvent{
			Type:       typ,
			TFormMyr:   getFloat(m, "t_form_myr"),
			XIce:       getFloat(m, "x_ice"),
			XForm:      getFloat(m, "x_form"),
			XNow:       getFloat(m, "x_now"),
			Direction:  getStr(m, "direction"),
			Factor:     getFloat(m, "factor"),
			Fraction:   getFloat(m, "fraction"),
			Residue:    getStr(m, "residue"),
			GiantOrbit: int(getFloat(m, "giant_orbit")),
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

// getMap — вложенный объект JSON (nil, если ключа нет или это не объект).
func getMap(data map[string]interface{}, key string) map[string]interface{} {
	raw, ok := data[key].(map[string]interface{})
	if !ok {
		return nil
	}
	return raw
}
