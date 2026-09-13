// internal/handlers/admin_universe.go
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"zorion/internal/generator"
	"zorion/internal/generator/faction"
	"zorion/internal/generator/galaxy"
	"zorion/internal/generator/planet"
	"zorion/internal/models"
)

var statusManager = generator.NewStatusManager()

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
//   worlds    ← locations, assignments, planets
//   locations ← production_units
//   planets   ← factions, settlements, factories, goods_batches
//
// Если появится новая таблица с FK на любую из этих — TRUNCATE упадёт
// с ошибкой "cannot truncate a table referenced in a foreign key
// constraint". Тогда добавь её в этот список.

const truncateTables = `worlds, locations, planets, assignments, production_units, factions, settlements, factories, goods_batches, regions`

// clearUniverseTx — очистка внутри уже начатой транзакции.
// Вызывающий делает Begin/Commit/Rollback.
func clearUniverseTx(ctx context.Context, tx *sql.Tx) error {
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

	// 4. Возвращаем FK на место.
	if _, err := tx.ExecContext(ctx, "ALTER TABLE users ADD CONSTRAINT users_current_world_id_fkey FOREIGN KEY (current_world_id) REFERENCES worlds(id) ON DELETE SET NULL"); err != nil {
		return fmt.Errorf("re-add fk: %w", err)
	}

	return nil
}

// insertRegionsTx — сохраняет регионы в уже начатой транзакции.
func insertRegionsTx(ctx context.Context, tx *sql.Tx, regions []*models.Region) error {
	if len(regions) == 0 {
		return nil
	}
	for _, r := range regions {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO regions (id, name, center_x, center_y, radius, color, world_count, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			r.ID, r.Name, r.CenterX, r.CenterY, r.Radius, r.Color, r.WorldCount, r.CreatedAt, r.UpdatedAt,
		); err != nil {
			return fmt.Errorf("insert region %s: %w", r.Name, err)
		}
	}
	return nil
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
		Shape          string  `json:"shape"` // "blob" (по умолчанию) | "circle"
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
		gen := galaxy.NewGenerator(&cfg)
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
			INSERT INTO worlds (id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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
			if _, err := stmt.ExecContext(ctx,
				world.ID, world.Name, world.CoordX, world.CoordY,
				world.SpectralClass, world.Temperature,
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

		if err := tx.Commit(); err != nil {
			log.Printf("❌ GenerateUniverse: failed to commit: %v", err)
			statusManager.Fail(generator.JobGenerateUniverse, err.Error())
			return
		}
		log.Printf("✅ GenerateUniverse: completed, %d worlds saved", len(worlds))
		statusManager.Done(generator.JobGenerateUniverse)
		h.recomputePlanetStats()
		h.mapCache.LoadAsync(h.db)
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
	// чтобы генератор не зависел от models.
	worldInfos := make([]planet.WorldInfo, 0, len(worlds))
	for _, w := range worlds {
		worldInfos = append(worldInfos, planet.WorldInfo{
			ID:            w.ID,
			Name:          w.Name,
			SpectralClass: w.SpectralClass,
			Temperature:   w.Temperature,
		})
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
		// (поселения, заводы, ресурсы) удаляются каскадно (ON DELETE CASCADE).
		oldCount, err := h.clearPlanets()
		if err != nil {
			log.Printf("❌ GeneratePlanets: delete old planets: %v", err)
			statusManager.Fail(generator.JobGeneratePlanets, err.Error())
			return
		}
		log.Printf("🗑️ GeneratePlanets: удалено старых планет: %d", oldCount)

		planetGen := planet.NewGenerator(h.db, 0)

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
func (h *AdminHandlers) clearPlanets() (int, error) {
	var oldCount int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM planets`).Scan(&oldCount); err != nil {
		return 0, err
	}
	if _, err := h.db.Exec(`DELETE FROM planets`); err != nil {
		return 0, err
	}
	return oldCount, nil
}

// ==================== GENERATE PROTOTYPE PLANET ====================

// GeneratePrototypePlanet — тестовая галактика для прототипа поселения:
// землеподобная планета (население 10) с поселением 1 уровня для первого мира.
// Ничего не очищает и не удаляет — состояние вселенной в руках оператора.
func (h *AdminHandlers) GeneratePrototypePlanet(w http.ResponseWriter, r *http.Request) {
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

	const population = 10
	settlementID := uuid.New().String()
	if _, err := tx.Exec(`
		INSERT INTO settlements (id, planet_id, population, stability, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		settlementID, pd.ID, population, 85, now, now,
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
		SELECT COUNT(*) FROM planets p
		WHERE EXISTS (
			SELECT 1 FROM settlements s
			WHERE s.planet_id = p.id AND s.population > 0
		)
	`)
	if err != nil {
		log.Printf("❌ GenerateFactions: count error: %v", err)
		http.Error(w, "Failed to count settled planets", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var total int
	if rows.Next() {
		rows.Scan(&total)
	}
	if total == 0 {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"factions_generated","total":0}`))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobGenerateFactions, total, cancel) {
		cancel()
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	go func() {
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
		factionGen := faction.NewGenerator(h.db, 0)
		count, err := factionGen.GenerateFactions()
		if err != nil {
			log.Printf("❌ GenerateFactions: %v", err)
			statusManager.Fail(generator.JobGenerateFactions, err.Error())
			return
		}
		log.Printf("✅ GenerateFactions: %d factions", count)
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
	total, processed, status, errMsg := statusManager.GetStatus(jt)
	resp := map[string]interface{}{
		"total":     total,
		"processed": processed,
		"status":    status,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ClearUniverse — удаляет все миры и связанные данные.
// Использует TRUNCATE без CASCADE + явный список таблиц.
// FK от users снимается на время операции и возвращается назад.
func (h *AdminHandlers) ClearUniverse(w http.ResponseWriter, r *http.Request) {
	if statusManager.IsRunning(generator.JobGenerateUniverse) ||
		statusManager.IsRunning(generator.JobGeneratePlanets) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}

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
