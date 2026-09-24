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
	planets, err := r.planetsByWorldID(worldID)
	if err != nil {
		return nil, err
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

// GetPlanetsByWorldIDForPlayer — планеты системы для карточки игрока (спека
// 2026-09-23-орбита-планеты-присутствие-и-снимок §5.6, И-С3): поселения
// читаются (SELECT), но ленивый owner-проход — запись чек-точки населения и
// веток — НЕ выполняется. Чтение планеты в режиме snapshot не двигает
// population_exact; живой путь (планета присутствия) синхронизируется отдельно
// через SyncPresenceSettlements. Фракции/строения/залежи — как в
// GetPlanetsByWorldID.
func (r *PlanetRepository) GetPlanetsByWorldIDForPlayer(worldID string) ([]models.Planet, error) {
	planets, err := r.planetsByWorldID(worldID)
	if err != nil {
		return nil, err
	}
	if err := r.loadSettlements(planets); err != nil {
		return nil, err
	}
	if err := r.attachFactionsAndBuildings(planets); err != nil {
		return nil, err
	}
	if err := r.attachDeposits(planets); err != nil {
		return nil, err
	}
	return planets, nil
}

// SyncPresenceSettlements — ленивый owner-проход для планет присутствия (живой
// путь, спека 2026-09-23 §5.6): пересчитывает население/ветки/эффекты и при
// устаревших чек-точках пишет их в БД. Карточка игрока зовёт это ровно для
// планеты, на которой он стоит; планеты в режиме snapshot/scan/none проходят
// через GetPlanetsByWorldIDForPlayer без записи (И-С3).
func (r *PlanetRepository) SyncPresenceSettlements(planets []models.Planet) error {
	return r.syncSettlements(planets)
}

// planetsByWorldID — планеты системы из planets (композиции/ядро/спутники), без
// поселений/фракций/строений/залежей.
func (r *PlanetRepository) planetsByWorldID(worldID string) ([]models.Planet, error) {
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
		       body_size_km, composition, visible, data, iron_remaining, ice_remaining, created_at, updated_at
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
		var ironRemaining, iceRemaining sql.NullFloat64
		var compJSON, dataJSON []byte
		if err := rows.Scan(
			&b.ID, &b.WorldID, &b.Kind, &b.Name, &orbitIndex, &b.RadiusAU,
			&b.WidthAU, &b.Mass, &b.BodySizeKm, &compJSON, &b.Visible, &dataJSON,
			&ironRemaining, &iceRemaining, &b.CreatedAt, &b.UpdatedAt,
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
		if iceRemaining.Valid {
			v := iceRemaining.Float64
			b.IceRemaining = &v
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
		       body_size_km, composition, visible, data, iron_remaining, ice_remaining, created_at, updated_at
		FROM system_belts
		WHERE id = $1
	`
	var b models.Belt
	var orbitIndex sql.NullInt64
	var ironRemaining, iceRemaining sql.NullFloat64
	var compJSON, dataJSON []byte
	err := r.db.QueryRow(query, beltID).Scan(
		&b.ID, &b.WorldID, &b.Kind, &b.Name, &orbitIndex, &b.RadiusAU,
		&b.WidthAU, &b.Mass, &b.BodySizeKm, &compJSON, &b.Visible, &dataJSON,
		&ironRemaining, &iceRemaining, &b.CreatedAt, &b.UpdatedAt,
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
	if iceRemaining.Valid {
		v := iceRemaining.Float64
		b.IceRemaining = &v
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
		SELECT id, world_id, kind, name, composition, visible, iron_remaining, ice_remaining
		FROM system_belts
		WHERE id = $1 FOR UPDATE
	`
	var b models.Belt
	var compJSON []byte
	var ironRemaining, iceRemaining sql.NullFloat64
	err := tx.QueryRow(query, beltID).Scan(
		&b.ID, &b.WorldID, &b.Kind, &b.Name, &compJSON, &b.Visible, &ironRemaining, &iceRemaining,
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
	if iceRemaining.Valid {
		v := iceRemaining.Float64
		b.IceRemaining = &v
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

// SetBeltIceRemaining — запись запаса льда пояса (спека 2026-09-24 §5.7):
// зеркало SetBeltIronRemaining, ленивая инициализация при первом обращении.
// Внутри транзакции вызывающего.
func (r *PlanetRepository) SetBeltIceRemaining(tx *sql.Tx, beltID string, value float64) error {
	_, err := tx.Exec(
		`UPDATE system_belts SET ice_remaining = $1, updated_at = NOW() WHERE id = $2`,
		value, beltID,
	)
	if err != nil {
		return fmt.Errorf("failed to set belt ice_remaining: %w", err)
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
	if err := r.loadSettlements(planets); err != nil {
		return err
	}
	return r.syncSettlements(planets)
}

// loadSettlements — только чтение поселений планет (SELECT + заполнение), без
// owner-прохода и без записи. Нужен пути игрока (GetPlanetsByWorldIDForPlayer),
// где запись чек-точек на чтении снимка запрещена (спека 2026-09-23 И-С3).
func (r *PlanetRepository) loadSettlements(planets []models.Planet) error {
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
	// Пакетный резолв имён владельцев (спека 2026-09-24-постройка-структур
	// §10.3): одна выборка на пачку, не по строке на запись (инвариант 2).
	refs := make([]ownerRef, 0)
	for _, settlements := range byPlanet {
		for j := range settlements {
			if settlements[j].OwnerID != "" {
				refs = append(refs, ownerRef{ownerType: settlements[j].OwnerType, ownerID: settlements[j].OwnerID})
			}
		}
	}
	names, err := resolveOwnerNames(r.db, refs)
	if err != nil {
		return err
	}
	for i := range planets {
		settlements := byPlanet[planets[i].ID]
		for j := range settlements {
			if settlements[j].OwnerID != "" {
				settlements[j].OwnerName = names[settlements[j].OwnerType+"|"+settlements[j].OwnerID]
			}
		}
		planets[i].Settlements = settlements
		planets[i].Habitable = len(settlements) > 0
	}
	return nil
}

// ownerRef — полиморфный владелец (type player/faction/agent + id) для
// пакетного резолва имени.
type ownerRef struct {
	ownerType string
	ownerID   string
}

// ownerNamesSelectSQL — пакетный резолв имён владельцев (users.username /
// factions.name / npc_agents.name) одним запросом UNION ALL (инвариант 2: не по
// строке на запись). `id::text = ANY(...)` — сравнение без каста uuid:
// устойчиво к не-UUID значениям и не требует валидного UUID-литерала.
const ownerNamesSelectSQL = `
	SELECT 'player', id::text, username FROM users WHERE id::text = ANY($1)
	UNION ALL
	SELECT 'faction', id::text, name FROM factions WHERE id::text = ANY($2)
	UNION ALL
	SELECT 'agent', id::text, name FROM npc_agents WHERE id::text = ANY($3)`

// resolveOwnerNames — имена владельцев по ссылкам (пакетно, один запрос).
// Пустой набор ссылок — пустая карта без обращения к БД. Ключ — "type|id".
func resolveOwnerNames(db *sql.DB, refs []ownerRef) (map[string]string, error) {
	players := []string{}
	factions := []string{}
	agents := []string{}
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.ownerID == "" || seen[ref.ownerType+"|"+ref.ownerID] {
			continue
		}
		seen[ref.ownerType+"|"+ref.ownerID] = true
		switch ref.ownerType {
		case "player":
			players = append(players, ref.ownerID)
		case "faction":
			factions = append(factions, ref.ownerID)
		case "agent":
			agents = append(agents, ref.ownerID)
		}
	}
	out := map[string]string{}
	if len(players)+len(factions)+len(agents) == 0 {
		return out, nil
	}
	rows, err := db.Query(ownerNamesSelectSQL, pqStringArray(players), pqStringArray(factions), pqStringArray(agents))
	if err != nil {
		return nil, fmt.Errorf("failed to load owner names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ownerType, ownerID, name string
		if err := rows.Scan(&ownerType, &ownerID, &name); err != nil {
			return nil, fmt.Errorf("failed to scan owner name: %w", err)
		}
		out[ownerType+"|"+ownerID] = name
	}
	return out, rows.Err()
}

// syncSettlements — owner-проход «производство → потребность → население» по
// уже загруженным поселениям планет: считает в памяти или пишет одной
// транзакцией на поселение (см. attachSettlements). Двигает чек-точку
// population_exact и ветки — поэтому вызывается только на живом пути.
func (r *PlanetRepository) syncSettlements(planets []models.Planet) error {
	if len(planets) == 0 {
		return nil
	}

	now := time.Now()
	owners := make([]OwnerSettlement, 0, len(planets))
	settlementIDs := make([]string, 0, len(planets))
	for i := range planets {
		settlements := planets[i].Settlements
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
				SettlementTypeID:  s.SettlementTypeID,
				Planet:            input,
				EatByPosition:     s.EatByPosition,
				EffectsByPosition: s.EffectsByPosition,
			})
			settlementIDs = append(settlementIDs, s.ID)
		}
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
			s.Arithmetic = res.Arithmetic
			s.Stage = res.Stage
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

	// Строения: имя типа — LEFT JOIN producer_types (у столиц producer_type_id
	// NULL → type_name пусто, метка «не производит»); владелец резолвится
	// пакетно ниже (спека 2026-09-24-постройка-структур §10.3).
	brows, err := r.db.Query(`
		SELECT b.id, b.planet_id, b.building_type, b.owner_type, b.owner_id, b.producer_type_id, pt.name
		FROM buildings b
		LEFT JOIN producer_types pt ON pt.id = b.producer_type_id
		WHERE b.planet_id = ANY($1) ORDER BY b.building_type ASC, b.id ASC
	`, pqStringArray(ids))
	if err != nil {
		return fmt.Errorf("failed to load buildings: %w", err)
	}
	defer brows.Close()
	for brows.Next() {
		var b models.PlanetBuilding
		var planetID string
		var producerTypeID sql.NullInt64
		var typeName sql.NullString
		if err := brows.Scan(&b.ID, &planetID, &b.BuildingType, &b.OwnerType, &b.OwnerID,
			&producerTypeID, &typeName); err != nil {
			return fmt.Errorf("failed to scan building: %w", err)
		}
		if producerTypeID.Valid {
			id := producerTypeID.Int64
			b.ProducerTypeID = &id
		}
		b.TypeName = typeName.String
		if i, ok := index[planetID]; ok {
			planets[i].Buildings = append(planets[i].Buildings, b)
		}
	}
	if err := brows.Err(); err != nil {
		return err
	}

	// Пакетный резолв имён владельцев строений (одна выборка на пачку).
	refs := make([]ownerRef, 0)
	for i := range planets {
		for j := range planets[i].Buildings {
			if planets[i].Buildings[j].OwnerID != "" {
				refs = append(refs, ownerRef{
					ownerType: planets[i].Buildings[j].OwnerType,
					ownerID:   planets[i].Buildings[j].OwnerID,
				})
			}
		}
	}
	names, err := resolveOwnerNames(r.db, refs)
	if err != nil {
		return err
	}
	for i := range planets {
		for j := range planets[i].Buildings {
			b := &planets[i].Buildings[j]
			if b.OwnerID != "" {
				b.OwnerName = names[b.OwnerType+"|"+b.OwnerID]
			}
		}
	}
	return nil
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

	// Газовый гигант, мини-нептун и спутники
	p.IsGasGiant = getBool(data, "is_gas_giant")
	p.IsMiniNeptune = getBool(data, "is_mini_neptune")
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
