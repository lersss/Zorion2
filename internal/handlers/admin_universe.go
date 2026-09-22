// internal/handlers/admin_universe.go
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"zorion/internal/generator"
	"zorion/internal/generator/faction"
	"zorion/internal/generator/galaxy"
	"zorion/internal/generator/planet"
	"zorion/internal/models"
	"zorion/internal/regionprofile"
	"zorion/internal/repository"
)

var statusManager = generator.NewStatusManager()

// universeMutationMu — разделяемый мьютекс между ClearUniverse (TRUNCATE) и
// пакманом (порционный DELETE, спека 2026-09-20 §2.2): операции не
// пересекаются; окно гонки «клик Clear в момент старта пакмана» закрыто.
// ClearUniverse держит lock на время операции, StartPacman — до конца джоба.
var universeMutationMu sync.Mutex

func recoverErr(r interface{}) string {
	if r == nil {
		return ""
	}
	if s, ok := r.(string); ok {
		return s
	}
	if err, ok := r.(error); ok {
		return err.Error()
	}
	return fmt.Sprintf("%v", r)
}

// ==================== ОБЩАЯ ОЧИСТКА ====================
//
// ВАЖНО: TRUNCATE ... CASCADE снёс бы users (у неё FK на worlds).
// Поэтому используем TRUNCATE без CASCADE + явный список таблиц,
// а FK у users на время операции снимаем и возвращаем назад.
//
// Список таблиц — все, что прямо или косвенно ссылаются на worlds
// (кроме users):
//   worlds    ← locations, planets, npc_agents, system_belts
//   planets   ← contracts (→ contract_requirements), factions, settlements, buildings, deposits
//
// Если появится новая таблица с FK на любую из этих — TRUNCATE упадёт
// с ошибкой "cannot truncate a table referenced in a foreign key
// constraint". Тогда добавь её в этот список.

const truncateTables = `worlds, locations, planets, contracts, contract_requirements, contract_log, factions, settlements, settlement_branches, settlement_branch_buffers, settlement_log, regions, npc_agents, player_planet_knowledge, buildings, deposits, system_belts, money_operations`

// clearUniverseTx — очистка внутри уже начатой транзакции.
// Вызывающий делает Begin/Commit/Rollback.
func clearUniverseTx(ctx context.Context, tx *sql.Tx) error {
	// 0. Возврат залога живых контрактов автору ДО удаления контрактов (§6.5):
	// деньги не исчезают без следа. Одна транзакция с TRUNCATE. Пустая область
	// ContractScope{} = вся таблица (очистка вселенной).
	if _, err := repository.ReturnEscrowForContractsTx(tx,
		repository.ContractScope{}, models.EscrowReasonWorldDeleted); err != nil {
		return fmt.Errorf("return escrow: %w", err)
	}

	// 1. Обнуляем current_world_id — чтобы после возврата FK не было висячих ссылок.
	if _, err := tx.ExecContext(ctx, "UPDATE users SET current_world_id = NULL WHERE current_world_id IS NOT NULL"); err != nil {
		return fmt.Errorf("update users: %w", err)
	}

	// 2. Снимаем FK — временно.
	if _, err := tx.ExecContext(ctx, "ALTER TABLE users DROP CONSTRAINT IF EXISTS users_current_world_id_fkey"); err != nil {
		return fmt.Errorf("drop fk: %w", err)
	}

	// 3. TRUNCATE без CASCADE. Все зависимые таблицы перечислены.
	if _, err := tx.ExecContext(ctx, "TRUNCATE TABLE "+truncateTables); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}

	// 4. Счета фракций и агентов удаляются (спека денег §3.5): фракции и агенты
	// перегенерируются с новыми id, старые счета стали бы сиротами. Кошелёк
	// игрока переживает очистку.
	if _, err := tx.ExecContext(ctx, "DELETE FROM accounts WHERE owner_type IN ('faction', 'agent')"); err != nil {
		return fmt.Errorf("delete faction/agent accounts: %w", err)
	}

	// 5. Возвращаем FK на место.
	if _, err := tx.ExecContext(ctx, "ALTER TABLE users ADD CONSTRAINT users_current_world_id_fkey FOREIGN KEY (current_world_id) REFERENCES worlds(id) ON DELETE SET NULL"); err != nil {
		return fmt.Errorf("re-add fk: %w", err)
	}

	return nil
}

// assignCurrentWorldsTx — назначает skycomposer-ам без текущего мира
// ближайший к центру галактики мир. Вызывается внутри транзакции
// генерации вселенной/близнецов после вставки миров, до commit.
// Миров нет — no-op; у остальных ролей и уже заданных миров не трогаем.
func assignCurrentWorldsTx(ctx context.Context, tx *sql.Tx) error {
	var worldID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM worlds ORDER BY (coord_x * coord_x + coord_y * coord_y) LIMIT 1`).Scan(&worldID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("select closest world: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET current_world_id = $1 WHERE role = 'skycomposer' AND current_world_id IS NULL`, worldID); err != nil {
		return fmt.Errorf("assign current world: %w", err)
	}
	return nil
}

// worldInsertValues — значения INSERT мира для spectral_class, stellar_mods,
// stellar_mass и age (41a §3.3).
//
// spectral_class: экзотика пишет NULL (99.2.4 §3), а не пустую строку.
// stellar_mods: ВСЕГДА валидный JSONB — для пустых модификаторов "{}",
// а не nil: lib/pq передаёт []byte(nil) как "" → "invalid input syntax for
// type json" (баг #1, прогон @tester). Другие пути вставки worlds
// (admin_hypothesis.go, world_repository.go, admin_worlds.go) колонку не
// пишут — там дефолт NULL, не затронуты.
// stellar_mass: NULL, если масса не сгенерирована (29a §4м).
// age: NULL, если возраст не сгенерирован (41a: обычные звёзды, старые миры).
func worldInsertValues(w *models.World) (spectralClass interface{}, modsJSON []byte, stellarMass interface{}, age interface{}) {
	if w.SpectralClass == "" {
		spectralClass = nil
	} else {
		spectralClass = w.SpectralClass
	}
	if w.StellarMods == nil {
		modsJSON = []byte("{}")
	} else {
		modsJSON, _ = json.Marshal(w.StellarMods)
		if len(modsJSON) == 0 {
			modsJSON = []byte("{}")
		}
	}
	if w.StellarMass != nil {
		stellarMass = *w.StellarMass
	} else {
		stellarMass = nil
	}
	if w.Age != nil {
		age = *w.Age
	} else {
		age = nil
	}
	return spectralClass, modsJSON, stellarMass, age
}

// insertRegionsTx — сохраняет регионы в уже начатой транзакции.
func insertRegionsTx(ctx context.Context, tx *sql.Tx, regions []*models.Region) error {
	if len(regions) == 0 {
		return nil
	}
	for _, r := range regions {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO regions (id, name, center_x, center_y, radius, color, world_count, profile, profile_intensity, race_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			r.ID, r.Name, r.CenterX, r.CenterY, r.Radius, r.Color, r.WorldCount,
			nullIfEmpty(r.Profile), r.ProfileIntensity, nullIfEmpty(r.RaceID), r.CreatedAt, r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("insert region %s: %w", r.Name, err)
		}
	}
	return nil
}

// nullIfEmpty — пустая строка → NULL (для nullable-колонок regions.profile).
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// loadRegionsWithProfiles — все регионы с профилями (59a §10): привязка
// планет к региону по ближайшему центру. ОТЛАДОЧНО (59a) profile также
// выводится в /api/regions (region_handler.go); в финале — убрать (не ярлык, §11.7).
func (h *AdminHandlers) loadRegionsWithProfiles() ([]*models.Region, error) {
	rows, err := h.db.Query(`
		SELECT id, name, center_x, center_y, radius, color, world_count, profile, profile_intensity, race_id
		FROM regions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	regions := make([]*models.Region, 0, 256)
	for rows.Next() {
		var r models.Region
		var profile sql.NullString
		var raceID sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.CenterX, &r.CenterY, &r.Radius, &r.Color,
			&r.WorldCount, &profile, &r.ProfileIntensity, &raceID); err != nil {
			return nil, err
		}
		r.Profile = profile.String
		r.RaceID = raceID.String
		regions = append(regions, &r)
	}
	return regions, rows.Err()
}

// ==================== GENERATE UNIVERSE ====================

func (h *AdminHandlers) GenerateUniverse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorldCount     int     `json:"world_count"`
		ClusterCount   int     `json:"cluster_count"`
		MapSize        float64 `json:"map_size"`
		MinDist        float64 `json:"min_dist"`
		ClusterRadius  float64 `json:"cluster_radius"`
		ClusterSpacing float64 `json:"cluster_spacing"`
		OutlierPercent int     `json:"outlier_percent"`
		Shape          string  `json:"shape"` // "blob" (по умолчанию) | "circle" | "ring" | "bar" | "spiral" | "dumbbell" | "stream" | "core_halo" | "random"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.WorldCount <= 0 {
		req.WorldCount = 1000
	}
	if req.ClusterCount <= 0 {
		req.ClusterCount = 20
	}
	if req.MapSize <= 0 {
		req.MapSize = 8000
	}
	if req.MinDist <= 0 {
		req.MinDist = 150
	}
	if req.ClusterRadius <= 0 {
		req.ClusterRadius = 1200
	}
	if req.ClusterSpacing <= 0 {
		req.ClusterSpacing = 200
	}
	if req.OutlierPercent < 0 {
		req.OutlierPercent = 30
	}
	if req.OutlierPercent > 50 {
		req.OutlierPercent = 50
	}
	if req.Shape == "" {
		req.Shape = "blob"
	}

	log.Printf("🌌 GenerateUniverse: mapSize=%.1f, minDist=%.1f, clusterRadius=%.1f, clusterSpacing=%.1f, outlierPercent=%d%%, shape=%q",
		req.MapSize, req.MinDist, req.ClusterRadius, req.ClusterSpacing, req.OutlierPercent, req.Shape)

	// Взаимная блокировка: не стартуем, пока крутится пересчёт планет
	// (джобы пишут в одни таблицы и не знают о соседе, AGENTS.md §23).
	// Пакман ест миры (спека 2026-09-20 §2.2) — генерация поверх него не
	// стартует (иначе новые миры переживут вайп).
	if statusManager.IsRunning(generator.JobRegeneratePlanets) ||
		statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	if !statusManager.TryStart(generator.JobGenerateUniverse, req.WorldCount, cancel) {
		cancel()
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ GenerateUniverse panic: %v", rec)
				statusManager.Fail(generator.JobGenerateUniverse, "panic: "+recoverErr(rec))
			}
		}()
		log.Printf("🌌 GenerateUniverse: start with %d worlds, %d clusters", req.WorldCount, req.ClusterCount)

		cfg := galaxy.Config{
			Seed:           time.Now().UnixNano(),
			WorldCount:     req.WorldCount,
			ClusterCount:   req.ClusterCount,
			MapSize:        req.MapSize,
			MinDist:        req.MinDist,
			ClusterRadius:  req.ClusterRadius,
			ClusterSpacing: req.ClusterSpacing,
			OutlierPercent: float64(req.OutlierPercent) / 100.0,
			WorldSpread:    20.0,
			Shape:          req.Shape,
		}
		// Веса из generation_config (99.2.3 §4.4): дефолты, если в БД пусто.
		weights := galaxy.DefaultWeights()
		massRanges := galaxy.DefaultStellarMassRanges()
		softness := 0.5 // дефолт подкрутки под расу-дома (99.2.22 §4.3)
		if loaded, err := h.loadGenerationConfig(); err == nil {
			weights = loaded.StarWeights
			if len(loaded.StellarMassRanges) > 0 {
				massRanges = loaded.StellarMassRanges
			}
			softness = loaded.RaceTuningSoftness
		}
		gen := galaxy.NewGeneratorWithWeights(&cfg, weights)
		gen.SetMassRanges(massRanges)
		// Мягкость подкрутки под расу-дома (99.2.22 §4.3): слой 1 непрерывный —
		// спектральные веса eff = 1 + (config−1)·s.
		gen.SetRaceSoftness(softness)
		result := gen.GenerateGalaxyWithRegions()
		worlds := result.Worlds
		log.Printf("✅ Generated %d worlds, %d regions", len(worlds), len(result.Regions))

		tx, err := h.db.BeginTx(ctx, nil)
		if err != nil {
			log.Printf("❌ GenerateUniverse: failed to start transaction: %v", err)
			statusManager.Fail(generator.JobGenerateUniverse, err.Error())
			return
		}
		defer tx.Rollback()

		if err := clearUniverseTx(ctx, tx); err != nil {
			log.Printf("❌ GenerateUniverse: clear failed: %v", err)
			statusManager.Fail(generator.JobGenerateUniverse, err.Error())
			return
		}

		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO worlds (id, name, coord_x, coord_y, spectral_class, temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		`)
		if err != nil {
			log.Printf("❌ GenerateUniverse: failed to prepare statement: %v", err)
			statusManager.Fail(generator.JobGenerateUniverse, err.Error())
			return
		}
		defer stmt.Close()

		for i, world := range worlds {
			select {
			case <-ctx.Done():
				log.Printf("⚠️ GenerateUniverse: canceled")
				statusManager.Cancel(generator.JobGenerateUniverse)
				return
			default:
			}
			spectralClass, modsJSON, stellarMass, age := worldInsertValues(world)
			if _, err := stmt.ExecContext(ctx,
				world.ID, world.Name, world.CoordX, world.CoordY,
				spectralClass, world.Temperature,
				world.StarType, world.SystemType,
				modsJSON, stellarMass, age,
				world.CreatedAt, world.UpdatedAt,
			); err != nil {
				log.Printf("❌ GenerateUniverse: failed to insert world %s: %v", world.ID, err)
				statusManager.Fail(generator.JobGenerateUniverse, err.Error())
				return
			}
			statusManager.Progress(generator.JobGenerateUniverse, i+1)
		}

		if err := insertRegionsTx(ctx, tx, result.Regions); err != nil {
			log.Printf("❌ GenerateUniverse: failed to insert regions: %v", err)
			statusManager.Fail(generator.JobGenerateUniverse, err.Error())
			return
		}

		if err := assignCurrentWorldsTx(ctx, tx); err != nil {
			log.Printf("❌ GenerateUniverse: failed to assign current worlds: %v", err)
			statusManager.Fail(generator.JobGenerateUniverse, err.Error())
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("❌ GenerateUniverse: failed to commit: %v", err)
			statusManager.Fail(generator.JobGenerateUniverse, err.Error())
			return
		}
		log.Printf("✅ GenerateUniverse: completed, %d worlds saved", len(worlds))
		statusManager.Done(generator.JobGenerateUniverse)
		h.recomputePlanetStats()
		h.mapCache.LoadAsync(h.db)
		// TRUNCATE npc_agents (clearUniverseTx) — агентов больше нет: позиции
		// и кэш агентов сбросить сразу (идея 26c A2, как ClearAllAgents).
		if h.npcManager != nil {
			h.npcManager.OnAgentsDeleted()
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}

// ==================== GENERATE PLANETS ====================

func (h *AdminHandlers) GeneratePlanets(w http.ResponseWriter, r *http.Request) {
	tFetchStart := time.Now()
	worlds, err := h.worldRepo.GetAll()
	if err != nil {
		log.Printf("❌ GeneratePlanets: failed to fetch worlds: %v", err)
		http.Error(w, "Failed to fetch worlds: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("📋 GeneratePlanets: fetched %d worlds за %v", len(worlds), time.Since(tFetchStart).Round(time.Millisecond))

	if len(worlds) == 0 {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"planets_generated","total":0}`))
		return
	}

	// Взаимная блокировка: не стартуем, пока крутится пересчёт планет
	// (джобы пишут в одни таблицы и не знают о соседе, AGENTS.md §23).
	// Пакман ест миры (спека 2026-09-20 §2.2) — генерация планет поверх
	// него не стартует.
	if statusManager.IsRunning(generator.JobRegeneratePlanets) ||
		statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	_, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobGeneratePlanets, len(worlds), cancel) {
		cancel()
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	// Кэш статистики к этому моменту почти наверняка устарел (например,
	// «0 планет» после GenerateUniverse). Сбрасываем до генерации, чтобы
	// вкладка статистики не показывала прошлое состояние в окне пересчёта.
	h.invalidatePlanetStats()

	// Конвертируем []*models.World в []planet.WorldInfo — лёгкий тип,
	// чтобы генератор не зависел от models. Экзотические типы и модификаторы
	// несутся в WorldInfo для ветки generateExoticPlanet (99.2.4 §5.3).
	// Профиль региона (59a §10) и раса-дома (99.2.22 §2.1): для каждого мира —
	// ближайший регион по координатам (та же NearestRegionIndex, что в
	// генерации звёзд; раса — из regions.race_id, консистентно с profile).
	regions, err := h.loadRegionsWithProfiles()
	if err != nil {
		log.Printf("❌ GeneratePlanets: failed to load regions: %v", err)
		statusManager.Fail(generator.JobGeneratePlanets, err.Error())
		return
	}
	worldInfos := make([]planet.WorldInfo, 0, len(worlds))
	for _, w := range worlds {
		wi := planet.WorldInfo{
			ID:            w.ID,
			Name:          w.Name,
			SpectralClass: w.SpectralClass,
			Temperature:   w.Temperature,
			StarType:      w.StarType,
			SystemType:    w.SystemType,
			Mods:          w.StellarMods,
			Age:           w.Age,
			StellarMass:   w.StellarMass,
		}
		if idx := regionprofile.NearestRegionIndex(w.CoordX, w.CoordY, regions); idx >= 0 {
			if r := regions[idx]; r.Profile != "" {
				wi.Profile = regionprofile.ByID(r.Profile)
				wi.ProfileIntensity = regionprofile.Intensity(r.ProfileIntensity)
			}
			wi.RaceID = regions[idx].RaceID
		}
		worldInfos = append(worldInfos, wi)
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ GeneratePlanets panic: %v", rec)
				statusManager.Fail(generator.JobGeneratePlanets, "panic: "+recoverErr(rec))
			}
		}()

		tStart := time.Now()
		log.Printf("🌍 GeneratePlanets: starting for %d worlds (batch=500, COPY)", len(worldInfos))

		// B11: старые планеты удаляются до генерации, иначе повторный запуск
		// дублирует данные (было 638k вместо 319k планет). Дочерние записи
		// (поселения, ресурсы) удаляются каскадно (ON DELETE CASCADE).
		// Пояса малых тел мира тоже чистятся здесь (система поясов §4.6):
		// иначе дубли поясов при повторном прогоне без GenerateUniverse.
		oldCount, err := h.clearPlanets()
		if err != nil {
			log.Printf("❌ GeneratePlanets: delete old planets: %v", err)
			statusManager.Fail(generator.JobGeneratePlanets, err.Error())
			return
		}
		log.Printf("🗑️ GeneratePlanets: удалено старых планет: %d", oldCount)

		planetGen := planet.NewGenerator(h.db, 0)

		// Средние числа планет и подкрутка под расу-дома из generation_config
		// (99.2.3 §4.3, 99.2.22 §4.3): дефолты, если пусто.
		if loaded, err := h.loadGenerationConfig(); err == nil {
			planetGen.SetMeans(loaded.PlanetMeans)
			planetGen.SetRaceTuning(loaded.RaceTuningSoftness, loaded.RaceClusterPlanetCountMult)
		}

		// Карта ресурсов каталога для залежей (спека залежей §3.1): генератор
		// сеттер, БД сама не ходит. Пустая карта — залежей не будет.
		planetGen.SetGoodsIndex(loadResourceGoodsIndex(h.db))

		progressFn := func(processed int) {
			statusManager.Progress(generator.JobGeneratePlanets, processed)
		}

		totalPlanets, err := planetGen.GeneratePlanetsForWorlds(worldInfos, 500, progressFn)
		if err != nil {
			log.Printf("❌ GeneratePlanets: %v", err)
			statusManager.Fail(generator.JobGeneratePlanets, err.Error())
			return
		}

		elapsed := time.Since(tStart)
		log.Printf("✅ GeneratePlanets: total = %d планет за %v (%.0f планет/сек)",
			totalPlanets, elapsed.Round(time.Millisecond),
			float64(totalPlanets)/elapsed.Seconds(),
		)
		statusManager.Done(generator.JobGeneratePlanets)
		h.recomputePlanetStats()
		h.mapCache.LoadAsync(h.db)
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}

// clearPlanets — удаляет все планеты и возвращает число удалённых.
// Безопасно благодаря ON DELETE CASCADE на дочерних таблицах.
//
// Пояса малых тел (спека поясов §4.6/§4.7): перед пересозданием планет
// чистятся ЗДЕСЬ ЖЕ — GeneratePlanets перегенерирует все миры, поэтому
// пояса удаляются глобально (как и планеты; форма `world_id = ANY($1)` из
// clearPlanetsOf здесь не применима — список миров тут не отбирается).
// Иначе повторный прогон без GenerateUniverse даёт дубли поясов: генератор
// кладёт пояса заново для каждого мира.
//
// Возврат залога живых контрактов и DELETE — ОДНА транзакция (§6.5): иначе
// залог контрактов удалённых планет пропал бы без следа.
func (h *AdminHandlers) clearPlanets() (int, error) {
	tx, err := h.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := repository.ReturnEscrowForContractsTx(tx,
		repository.ContractScope{}, models.EscrowReasonWorldDeleted); err != nil {
		return 0, fmt.Errorf("return escrow: %w", err)
	}
	var oldCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM planets`).Scan(&oldCount); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM system_belts`); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM planets`); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return oldCount, nil
}

// ==================== GENERATE PROTOTYPE PLANET ====================

// GeneratePrototypePlanet — тестовая галактика для прототипа поселения:
// землеподобная планета (население 10) с поселением 1 уровня для первого мира.
// Ничего не очищает и не удаляет — состояние вселенной в руках оператора.
func (h *AdminHandlers) GeneratePrototypePlanet(w http.ResponseWriter, r *http.Request) {
	// Пакман ест миры (спека 2026-09-20 §2.2, правка Н1): синхронный писатель
	// в planets+settlements — иначе прото-планета вставится в выживший мир
	// посреди пакмана и переживёт вайп.
	if statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}
	worlds, err := h.worldRepo.GetAll()
	if err != nil {
		http.Error(w, "Failed to fetch worlds: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(worlds) == 0 {
		http.Error(w, "Вселенная пуста — сначала сгенерируй миры", http.StatusBadRequest)
		return
	}
	world := worlds[0]

	planetGen := planet.NewGenerator(h.db, 0)
	planetGen.SetGoodsIndex(loadResourceGoodsIndex(h.db))
	pd := planetGen.GeneratePrototypePlanet(world.ID, world.Name, world.SpectralClass)

	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, "Failed to start transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	now := time.Now()
	if _, err := tx.Exec(`
		INSERT INTO planets (id, world_id, name, orbit_index, data, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		pd.ID, pd.WorldID, pd.Name, pd.OrbitIndex, string(pd.Data), now, now,
	); err != nil {
		http.Error(w, "Failed to insert planet: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Залежи прототипа — в той же транзакции, после планеты (FK §3.4).
	if err := insertDepositsTx(r.Context(), tx, pd.Deposits, now); err != nil {
		http.Error(w, "Failed to insert deposits: "+err.Error(), http.StatusInternalServerError)
		return
	}

	const population = 10
	settlementID := uuid.New().String()
	// Тип поселения — настоящая связь (спека итерации 4 §3.4): дефолтный подтип
	// резолвится один раз; типа нет → NULL (чтение применит DefaultEatK).
	settlementTypeID, err := repository.ResolveDefaultSettlementTypeID(h.db)
	if err != nil {
		http.Error(w, "Failed to resolve settlement type: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var settlementTypeArg interface{}
	if settlementTypeID != 0 {
		settlementTypeArg = settlementTypeID
	}
	if _, err := tx.Exec(`
		INSERT INTO settlements (id, planet_id, population, population_exact, stability, computed_at, settlement_type_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		settlementID, pd.ID, population, float64(population), 85, now, settlementTypeArg, now, now,
	); err != nil {
		http.Error(w, "Failed to insert settlement: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.mapCache.LoadAsync(h.db)

	log.Printf("🪐 GeneratePrototypePlanet: мир %s, планета %s, поселение %d чел.",
		world.Name, pd.Name, population)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "ok",
		"world_id":      world.ID,
		"world_name":    world.Name,
		"planet_id":     pd.ID,
		"planet_name":   pd.Name,
		"settlement_id": settlementID,
		"population":    population,
	})
}

// ==================== GENERATE FACTIONS ====================

func (h *AdminHandlers) GenerateFactions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT COUNT(DISTINCT s.race_id) FROM settlements s
		WHERE s.race_id IS NOT NULL AND s.race_id <> '' AND s.population > 0
	`)
	if err != nil {
		log.Printf("❌ GenerateFactions: count error: %v", err)
		http.Error(w, "Failed to count settled races", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var total int
	if rows.Next() {
		rows.Scan(&total)
	}

	// Гейт мутаций вселенной — ДО ветки total == 0 (C3, спека
	// 2026-09-21-фабрики-релиз-2-столицы-фракций §3/§7 п.3): фракции и столицы
	// пишут в planets-каскад, синхронный догон столиц в ветке total == 0 —
	// тот же писатель. Пакман ест миры (спека 2026-09-20 §2.2), ClearUniverse
	// TRUNCATE-ит под universeMutationMu: гонка «проверка + действие» закрыта
	// (инвариант 1) — гейт раньше возврата, а не после.
	if statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}
	if !universeMutationMu.TryLock() {
		http.Error(w, "Universe mutation is running, wait for it", http.StatusConflict)
		return
	}

	factionGen := faction.NewGenerator(h.db, 0)

	if total == 0 {
		// Нет заселённых рас: фракции не создаются, но догон столиц
		// легаси-фракций выполняется синхронно под тем же гейтом (§3).
		defer universeMutationMu.Unlock()
		capitals, err := factionGen.EnsureCapitals()
		if err != nil {
			log.Printf("❌ GenerateFactions: capitals (sync): %v", err)
			http.Error(w, "Failed to create capitals", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "factions_generated",
			"total":    0,
			"capitals": capitals,
		})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobGenerateFactions, total, cancel) {
		cancel()
		universeMutationMu.Unlock()
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	go func() {
		// Мьютекс мутаций вселенной держится до конца джоба (как StartPacman):
		// ClearUniverse не стартует поверх генерации фракций (TryLock → 409).
		defer universeMutationMu.Unlock()
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ GenerateFactions panic: %v", rec)
				statusManager.Fail(generator.JobGenerateFactions, "panic: "+recoverErr(rec))
			}
		}()
		select {
		case <-ctx.Done():
			statusManager.Cancel(generator.JobGenerateFactions)
			return
		default:
		}
		log.Printf("🏛️ GenerateFactions: start")
		factions, capitals, err := factionGen.GenerateFactions()
		if err != nil {
			log.Printf("❌ GenerateFactions: %v", err)
			statusManager.Fail(generator.JobGenerateFactions, err.Error())
			return
		}
		log.Printf("✅ GenerateFactions: %d factions, %d capitals", factions, capitals)
		statusManager.Done(generator.JobGenerateFactions)
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}

// ==================== CANCEL / STATUS / CLEAR ====================

func (h *AdminHandlers) CancelGeneration(w http.ResponseWriter, r *http.Request) {
	job := r.URL.Query().Get("job")
	if job == "" {
		http.Error(w, "job parameter required", http.StatusBadRequest)
		return
	}
	jt := generator.JobType(job)
	if !statusManager.IsRunning(jt) {
		http.Error(w, "Job not running", http.StatusBadRequest)
		return
	}
	statusManager.Cancel(jt)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"canceled"}`))
}

func (h *AdminHandlers) GenerateStatus(w http.ResponseWriter, r *http.Request) {
	job := r.URL.Query().Get("job")
	if job == "" {
		http.Error(w, "job parameter required", http.StatusBadRequest)
		return
	}
	jt := generator.JobType(job)
	total, processed, status, errMsg, report := statusManager.GetStatus(jt)
	resp := map[string]interface{}{
		"total":     total,
		"processed": processed,
		"status":    status,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	if report != "" {
		resp["report"] = report
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ClearUniverse — удаляет все миры и связанные данные.
// Использует TRUNCATE без CASCADE + явный список таблиц.
// FK от users снимается на время операции и возвращается назад.
func (h *AdminHandlers) ClearUniverse(w http.ResponseWriter, r *http.Request) {
	if statusManager.IsRunning(generator.JobGenerateUniverse) ||
		statusManager.IsRunning(generator.JobGeneratePlanets) ||
		statusManager.IsRunning(generator.JobRegeneratePlanets) ||
		statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}

	// Разделяемый мьютекс с пакманом (спека 2026-09-20 §2.2): TRUNCATE и
	// порционный DELETE не пересекаются. Lock занят пакманом → 409.
	if !universeMutationMu.TryLock() {
		http.Error(w, "Pacman is eating, stop it first", http.StatusConflict)
		return
	}
	defer universeMutationMu.Unlock()

	tStart := time.Now()

	var worldsBefore, planetsBefore, usersBefore int
	h.db.QueryRow("SELECT COUNT(*) FROM worlds").Scan(&worldsBefore)
	h.db.QueryRow("SELECT COUNT(*) FROM planets").Scan(&planetsBefore)
	h.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&usersBefore)

	log.Printf("🗑️ ClearUniverse: начало (worlds=%d, planets=%d, users=%d)", worldsBefore, planetsBefore, usersBefore)

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		log.Printf("❌ ClearUniverse: begin tx: %v", err)
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if err := clearUniverseTx(r.Context(), tx); err != nil {
		log.Printf("❌ ClearUniverse: %v", err)
		http.Error(w, "Failed to clear universe: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("❌ ClearUniverse: commit: %v", err)
		http.Error(w, "Failed to commit", http.StatusInternalServerError)
		return
	}

	var usersAfter int
	h.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&usersAfter)

	log.Printf("✅ ClearUniverse: очищено за %v (users=%d)", time.Since(tStart).Round(time.Millisecond), usersAfter)
	h.invalidatePlanetStats()
	h.mapCache.LoadAsync(h.db)
	// TRUNCATE npc_agents — агентов больше нет: позиции и кэш агентов
	// сбросить сразу (идея 26c A2, как ClearAllAgents).
	if h.npcManager != nil {
		h.npcManager.OnAgentsDeleted()
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"cleared"}`))
}

func (h *AdminHandlers) GetStats(w http.ResponseWriter, r *http.Request) {
	var worldsCount, planetsCount int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM worlds").Scan(&worldsCount); err != nil {
		http.Error(w, "Failed to count worlds", http.StatusInternalServerError)
		return
	}
	if err := h.db.QueryRow("SELECT COUNT(*) FROM planets").Scan(&planetsCount); err != nil {
		http.Error(w, "Failed to count planets", http.StatusInternalServerError)
		return
	}
	resp := map[string]int{
		"worlds":  worldsCount,
		"planets": planetsCount,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
