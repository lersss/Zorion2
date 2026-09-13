// internal/handlers/admin_hypothesis.go
//
// Подсистема «Проверка гипотез» (specs/hypothesis_testing.md §4): очистка
// галактики, генерация планет-«близнецов» по TwinSpec (канонический шаблон +
// оверрайды группы), поселения по стратегии группы. Универсум транзиентный —
// следующий прогон перетирает предыдущий (решение игрока: инструмент анализа,
// история экспериментов в БД не сохраняется).
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"zorion/internal/audit"
	auditplanet "zorion/internal/audit/planet"
	"zorion/internal/generator"
	"zorion/internal/generator/planet"
)

// RunHypothesis — запуск «Проверки гипотез».
//
//	POST /admin/hypothesis/run
//	{"id":"young_worlds","base":{...},"groups":[{...},{...}]}  → 202
//	409, если другой джоб уже крутится.
//
// Статус читается через /admin/generate-status?job=hypothesis.
func (h *AdminHandlers) RunHypothesis(w http.ResponseWriter, r *http.Request) {
	var spec planet.TwinSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, "Bad JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := spec.Validate(); err != nil {
		http.Error(w, "Bad twin spec: "+err.Error(), http.StatusBadRequest)
		return
	}

	total := 0
	for _, g := range spec.Groups {
		total += g.PlanetsPerWorld
	}

	// Контекст от фоновой задачи, НЕ от запроса: контекст запроса отменяется,
	// когда handler возвращает ответ.
	ctx, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobHypothesis, total, cancel) {
		cancel()
		http.Error(w, "Generation already running", http.StatusConflict)
		return
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ RunHypothesis panic: %v", rec)
				statusManager.Fail(generator.JobHypothesis, "panic: "+recoverErr(rec))
			}
		}()

		log.Printf("🧪 RunHypothesis: %q, групп %d, планет %d", spec.ID, len(spec.Groups), total)

		settled, report, err := h.runHypothesisJob(ctx, spec)
		if err != nil {
			log.Printf("❌ RunHypothesis: %v", err)
			statusManager.Fail(generator.JobHypothesis, err.Error())
			return
		}
		log.Printf("✅ RunHypothesis: %q готово, поселений %d", spec.ID, settled)
		statusManager.SetReport(generator.JobHypothesis, report)
		statusManager.Done(generator.JobHypothesis)
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}

// runHypothesisJob — тело фоновой задачи: очистка → генерация миров/планет →
// поселения → кэши. Возвращает число поселений и отчёт (включая число
// «невозможных» планет).
func (h *AdminHandlers) runHypothesisJob(ctx context.Context, spec planet.TwinSpec) (settled int, report string, err error) {
	planetGen := planet.NewGenerator(h.db, 0)
	worldsByGroup, planetsByGroup, err := planetGen.GenerateTwins(spec, func(processed int) {
		statusManager.Progress(generator.JobHypothesis, processed)
	})
	if err != nil {
		return 0, "", err
	}

	// Аудит сгенерированных планет: сколько «невозможных» (глобальные
	// противоречия, high). Оазисы и прочие объяснимые аномалии (medium/low)
	// в счёт не идут.
	totalPlanets := 0
	var allPlanets []*planet.PlanetData
	for _, group := range spec.Groups {
		ps := planetsByGroup[group.ID]
		totalPlanets += len(ps)
		allPlanets = append(allPlanets, ps...)
	}
	impossible := 0
	var highCodes map[string]int
	if totalPlanets > 0 {
		_, impossible, highCodes = countImpossiblePlanets(allPlanets)
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()

	if err := clearUniverseTx(ctx, tx); err != nil {
		return 0, "", err
	}

	settled = 0
	for _, group := range spec.Groups {
		worlds := worldsByGroup[group.ID]
		planets := planetsByGroup[group.ID]

		for _, w := range worlds {
			now := time.Now()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO worlds (id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				w.ID, w.Name, w.CoordX, w.CoordY, w.SpectralClass, w.Temperature, now, now,
			); err != nil {
				return 0, "", err
			}
		}

		for _, p := range planets {
			now := time.Now()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO planets (id, world_id, name, orbit_index, data, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				p.ID, p.WorldID, p.Name, p.OrbitIndex, string(p.Data), now, now,
			); err != nil {
				return 0, "", err
			}
		}

		// Поселения группы: шанс + стратегия населения, напрямую в settlements.
		// Заводы и товары не создаются (спека §2 «Не-цели»).
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		for _, p := range planets {
			if rng.Float64() > group.Settlement.Chance {
				continue
			}
			now := time.Now()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO settlements (id, planet_id, population, stability, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				uuid.New().String(), p.ID,
				group.Settlement.Population.Value(rng),
				rng.Intn(41)+40, now, now,
			); err != nil {
				return 0, "", err
			}
			settled++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, "", err
	}

	h.recomputePlanetStats()
	h.mapCache.LoadAsync(h.db)
	return settled, buildHypothesisReport(totalPlanets, settled, impossible, highCodes), nil
}

// countImpossiblePlanets — аудит сгенерированных планет. Возвращает всего
// планет, число «невозможных» (с хотя бы одной high-проблемой) и разбивку
// high-проблем по кодам.
func countImpossiblePlanets(planets []*planet.PlanetData) (total, impossible int, highCodes map[string]int) {
	rows := make([]auditplanet.Row, 0, len(planets))
	for _, p := range planets {
		rows = append(rows, auditplanet.Row{ID: p.ID, WorldID: p.WorldID, Name: p.Name, Data: p.Data})
	}
	res := audit.Run("planet", auditplanet.ParseAll(rows), auditplanet.AllRules())
	return res.TotalEntities, res.EntitiesWithHighIssue, res.HighIssuesByCode
}

// buildHypothesisReport — текст отчёта о прогоне гипотезы.
func buildHypothesisReport(total, settled, impossible int, highCodes map[string]int) string {
	if impossible == 0 {
		return fmt.Sprintf("✅ Создано планет: %d, поселений: %d, невозможных: 0", total, settled)
	}
	return fmt.Sprintf("⚠️ Создано планет: %d, поселений: %d, невозможных: %d (%s)",
		total, settled, impossible, formatHighCodes(highCodes))
}

// formatHighCodes — «oceans_in_heat ×2, life_without_water ×1» по убыванию.
func formatHighCodes(codes map[string]int) string {
	type kv struct {
		code string
		n    int
	}
	var list []kv
	for c, n := range codes {
		list = append(list, kv{c, n})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	parts := make([]string, 0, len(list))
	for _, k := range list {
		parts = append(parts, fmt.Sprintf("%s ×%d", k.code, k.n))
	}
	return strings.Join(parts, ", ")
}