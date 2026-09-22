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
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"zorion/internal/audit"
	auditplanet "zorion/internal/audit/planet"
	"zorion/internal/economy/settlement"
	"zorion/internal/generator"
	"zorion/internal/generator/planet"
	"zorion/internal/races"
	"zorion/internal/repository"
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

	// Пакман ест миры (спека 2026-09-20 §2.2): гипотезы пишут в
	// worlds/planets/settlements — прогон поверх пакмана не стартует.
	if statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
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
	// Тип поселения — настоящая связь (спека итерации 4 §3.4): один резолв на
	// джоб; типа нет → NULL (чтение применит DefaultEatK).
	settlementTypeID, err := repository.ResolveDefaultSettlementTypeID(h.db)
	if err != nil {
		return 0, "", err
	}
	var settlementTypeArg interface{}
	if settlementTypeID != 0 {
		settlementTypeArg = settlementTypeID
	}
	// Карта ресурсов каталога для залежей (спека залежей §3.1): генератор
	// сеттер, БД сама не ходит. Пустая карта — залежей не будет.
	planetGen.SetGoodsIndex(loadResourceGoodsIndex(h.db))
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
	groupReports := make([]groupReport, 0, len(spec.Groups))
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
			// Залежи близнеца — в той же транзакции, после планеты (FK §3.4).
			if err := insertDepositsTx(ctx, tx, p.Deposits, now); err != nil {
				return 0, "", err
			}
		}

		// Поселения группы: шанс + стратегия населения + число поселений на
		// планету, напрямую в settlements. Фабрики и товары не создаются
		// (спека §2 «Не-цели»). Гейт пригодности расы (99.2.23 §5.2):
		// раса, непригодная на клоне, не селится — планета идёт в счётчик
		// «непригодных» группы (в отчёте).
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		perPlanet := group.Settlement.SettlementsPerPlanet
		if perPlanet <= 0 {
			perPlanet = 1
		}
		gr := groupReport{Name: group.Name, Planets: len(planets)}
		if group.RaceID != "" {
			if r := races.ByID(group.RaceID); r != nil {
				gr.RaceName = r.Name
			}
		}
		for _, p := range planets {
			if rng.Float64() > group.Settlement.Chance {
				continue
			}
			// Гейт пригодности: race_id задан (включая "humans" — единый
			// источник пригодности 65a) → Suitable; пусто — как сейчас.
			if group.RaceID != "" {
				var data map[string]interface{}
				if err := json.Unmarshal(p.Data, &data); err == nil {
					if race := races.ByID(group.RaceID); race != nil && !race.Suitable(data) {
						gr.Unsuitable++
						continue
					}
				}
			}
			for n := 0; n < perPlanet; n++ {
				now := time.Now()
				population := group.Settlement.Population.Value(rng)
				var raceIDArg interface{}
				if group.RaceID != "" {
					raceIDArg = group.RaceID
				}
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO settlements (id, planet_id, population, population_exact, stability, computed_at, race_id, settlement_type_id, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
					uuid.New().String(), p.ID,
					population, float64(population),
					rng.Intn(41)+40, now, raceIDArg, settlementTypeArg, now, now,
				); err != nil {
					return 0, "", err
				}
				settled++
				gr.Settled++
			}
		}
		// Числа R для отчёта (99.2.23 §5.3): расовый ChangeComponents в точке
		// оси (по active-кривой расы), t50, проекция на год.
		gr.RPerSec, gr.T50, gr.Projection = groupReportNumbers(planets, group)
		gr.AxisValue = axisValueLabel(spec, group)
		groupReports = append(groupReports, gr)
	}

	if err := assignCurrentWorldsTx(ctx, tx); err != nil {
		return 0, "", err
	}

	if err := tx.Commit(); err != nil {
		return 0, "", err
	}

	h.recomputePlanetStats()
	h.mapCache.LoadAsync(h.db)
	report = buildHypothesisReport(totalPlanets, settled, impossible, highCodes)
	if len(groupReports) > 0 {
		report += "\n\n" + buildGroupReports(groupReports)
	}
	return settled, report, nil
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

// ==================== Отчёт групп с числами R (99.2.23 §5.3) ====================

// groupReport — строка отчёта группы: числа R в точке оси (по active-кривой
// расы), счётчики планет/поселений/непригодных.
type groupReport struct {
	Name       string
	RaceName   string
	AxisValue  string
	RPerSec    float64
	T50        string
	Projection string
	Planets    int
	Settled    int
	Unsuitable int
}

// groupReportNumbers — расовый ChangeComponents в точке оси группы (эффективные
// данные = клоны шаблона с оверрайдами; первая планета группы), t50 и проекция
// на 1 год (p0 — стратегия населения группы).
func groupReportNumbers(planets []*planet.PlanetData, group planet.TwinGroup) (rPerSec float64, t50 string, projection string) {
	if len(planets) == 0 {
		return 0, "", ""
	}
	var data map[string]interface{}
	if err := json.Unmarshal(planets[0].Data, &data); err != nil {
		return 0, "", ""
	}
	input := settlement.PlanetInput{RaceID: group.RaceID}
	if v, ok := dataFloat(data, "temperature"); ok {
		input.TemperatureK = v
	}
	if v, ok := dataFloat(data, "gravity"); ok {
		input.GravityG = v
	}
	if v, ok := dataFloat(data, "core.radioactivity"); ok {
		input.CoreRadioactivity = v
	}
	rPerSec = settlement.ChangeComponents(input)

	t := settlement.DeathMomentSeconds(2, rPerSec)
	if math.IsInf(t, 1) {
		t50 = "∞ (рост)"
	} else {
		t50 = fmt.Sprintf("%.1f ч", t/3600)
	}

	if p0 := groupPopulation(group); p0 > 0 {
		proj := settlement.Projection(p0, rPerSec)
		projection = fmt.Sprintf("%.0f", proj["1 год"])
	}
	return rPerSec, t50, projection
}

// groupPopulation — стартовое население группы для проекции (fixed — точное,
// random — середина диапазона; пустая стратегия — 0, проекция не считается).
func groupPopulation(group planet.TwinGroup) float64 {
	p := group.Settlement.Population
	switch p.Kind {
	case "fixed":
		return float64(p.Fixed)
	case "random":
		return (float64(p.Min) + float64(p.Max)) / 2
	}
	return 0
}

// axisValueLabel — значение варьируемой оси группы для отчёта: температура
// в °C (в данных — K), остальные оси — как есть.
func axisValueLabel(spec planet.TwinSpec, group planet.TwinGroup) string {
	if spec.Axis == "" {
		return ""
	}
	data := effectiveData(spec.Base, group.Overrides)
	v, ok := data[spec.Axis]
	if !ok {
		return ""
	}
	if spec.Axis == "temperature" {
		if f, ok := v.(float64); ok {
			return fmt.Sprintf("T = %.1f °C", f-273)
		}
	}
	return fmt.Sprintf("%s = %v", spec.Axis, v)
}

// effectiveData — шаблон + оверрайды группы (плоские ключи; для отчёта оси
// достаточно — вложенные оси не варьируются в близнецах).
func effectiveData(base, overrides map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+len(overrides))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

// dataFloat — число по ключу planet.data (dot-ключи раскрываются).
func dataFloat(data map[string]interface{}, key string) (float64, bool) {
	if i := strings.IndexByte(key, '.'); i >= 0 {
		child, ok := data[key[:i]].(map[string]interface{})
		if !ok {
			return 0, false
		}
		return dataFloat(child, key[i+1:])
	}
	v, ok := data[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// buildGroupReports — текст отчёта групп (99.2.23 §5.3): для каждой группы —
// раса, значение оси, счётчики, r_per_sec (по active-кривой), t50, проекция
// на год. Проценты роста — экспоненциальные ((1−r)^год − 1, поправки В1+В2).
func buildGroupReports(groups []groupReport) string {
	var b strings.Builder
	for _, g := range groups {
		b.WriteString(fmt.Sprintf("Группа «%s»", g.Name))
		meta := []string{}
		if g.RaceName != "" {
			meta = append(meta, "раса: "+g.RaceName)
		}
		if g.AxisValue != "" {
			meta = append(meta, g.AxisValue)
		}
		if len(meta) > 0 {
			b.WriteString(" (" + strings.Join(meta, ", ") + ")")
		}
		b.WriteString(":\n")
		b.WriteString(fmt.Sprintf("  планет: %d, поселений: %d, непригодных: %d\n", g.Planets, g.Settled, g.Unsuitable))
		b.WriteString(fmt.Sprintf("  r_per_sec: %.1e (%s)\n", g.RPerSec, growthLabel(g.RPerSec)))
		b.WriteString(fmt.Sprintf("  t50: %s\n", g.T50))
		if g.Projection != "" {
			b.WriteString(fmt.Sprintf("  население через 1 год: %s\n", g.Projection))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// growthLabel — экспоненциальный процент роста за год: (1−r)^год − 1
// (99.2.23 §5.3, поправки В1+В2: для термо-роёв «×54.9/год», не «+400%/год»).
// При r ≥ 1 (мгновенная гибель, гвард Population) Pow(1−r, год) дал бы
// отрицательное основание = NaN → метка «мгновенная гибель» без Pow (ревью).
func growthLabel(r float64) string {
	if r >= 1 {
		return "мгновенная гибель"
	}
	year := 365.0 * 24 * 3600
	growth := math.Pow(1-r, year) - 1
	switch {
	case growth > 1e-6:
		return fmt.Sprintf("рост %+.1f%%/год (×%.2f)", growth*100, 1+growth)
	case growth < -1e-6:
		return fmt.Sprintf("убыль %+.1f%%/год", growth*100)
	default:
		return "статика"
	}
}