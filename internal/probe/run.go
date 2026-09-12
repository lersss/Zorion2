// internal/probe/run.go
package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/lib/pq"
)

// Runner — батчевый прогон пробы по планетам БД.
// Не демон, не расписание: один прогон = один вызов Run.
type Runner struct {
	db         *sql.DB
	resultsDir string

	resultsMu sync.RWMutex
	results   map[string]*Result
}

// ProgressFunc — прогресс прогона (processed из total).
type ProgressFunc func(processed, total int)

// NewRunner — runner с каталогом результатов probe_results.
func NewRunner(db *sql.DB, resultsDir string) *Runner {
	return &Runner{db: db, resultsDir: resultsDir, results: map[string]*Result{}}
}

// Run — считает кривые всех планет выборки и возвращает результат.
// Идемпотентно: стартует с фиксированного seed, БД не мутирует.
func (r *Runner) Run(ctx context.Context, presetID string, params CurveParams, runOpts RunOptions, progress ProgressFunc) (*Result, error) {
	ids, err := r.planetIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("список планет: %w", err)
	}
	total := len(ids)
	if total == 0 {
		return nil, fmt.Errorf("в базе нет планет — сгенерируй вселенную сначала")
	}

	// Семплирование: seeded-перестановка, чтобы прогон был идемпотентным.
	if runOpts.SampleSize > 0 && runOpts.SampleSize < total {
		rng := rand.New(rand.NewSource(runOpts.Seed))
		perm := rng.Perm(total)
		picked := make([]string, runOpts.SampleSize)
		for i := 0; i < runOpts.SampleSize; i++ {
			picked[i] = ids[perm[i]]
		}
		ids = picked
		total = len(ids)
	}

	curves, err := r.computeCurves(ctx, ids, params, runOpts.Seed, progress)
	if err != nil {
		return nil, fmt.Errorf("расчёт кривых: %w", err)
	}

	res := &Result{
		ID:        newResultID(presetID),
		PresetID:  presetID,
		CreatedAt: time.Now().Format(time.RFC3339),
		Params:    params,
		Run:       runOpts,
		Total:     total,
		Stats:     ComputeStats(curves, runOpts.HorizonDays),
		Curves:    curves,
	}

	if err := r.save(res); err != nil {
		log.Printf("❌ probe: сохранить результат: %v", err)
	}
	return res, nil
}

// planetIDs — все id планет (для выборки и полного прогона).
func (r *Runner) planetIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM planets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// computeCurves — стриминг данных планет и расчёт кривых.
func (r *Runner) computeCurves(ctx context.Context, ids []string, params CurveParams, seed int64, progress ProgressFunc) ([]Curve, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, world_id, name, data FROM planets WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rng := rand.New(rand.NewSource(seed))
	curves := make([]Curve, 0, len(ids))
	processed := 0
	for rows.Next() {
		var planetID, worldID, planetName string
		var dataJSON []byte
		if err := rows.Scan(&planetID, &worldID, &planetName, &dataJSON); err != nil {
			return nil, err
		}

		var data map[string]interface{}
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			log.Printf("⚠️ probe: битый data планеты %s: %v", planetID, err)
			continue
		}

		curves = append(curves, params.ComputeCurve(
			planetID,
			worldID,
			planetName,
			ClassForType(getStr(data, "type")),
			getFloat(data, "temperature"),
			getFloat(data, "water_percent"),
			getStr(data, "atmosphere"),
			rng,
		))

		processed++
		if progress != nil && processed%1000 == 0 {
			progress(processed, len(ids))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if progress != nil {
		progress(processed, len(ids))
	}
	return curves, nil
}

// save — держит результат в памяти и пишет файл. Память первична:
// ошибка диска не должна лишать сессию результата.
func (r *Runner) save(res *Result) error {
	r.resultsMu.Lock()
	r.results[res.ID] = res
	r.resultsMu.Unlock()

	if r.resultsDir == "" {
		return nil
	}
	if err := os.MkdirAll(r.resultsDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(r.resultsDir, res.ID+".json")
	data, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// List — результаты, новые сверху.
func (r *Runner) List() []*Result {
	r.resultsMu.RLock()
	defer r.resultsMu.RUnlock()
	out := make([]*Result, 0, len(r.results))
	for _, res := range r.results {
		out = append(out, res)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// Get — результат по id.
func (r *Runner) Get(id string) (*Result, bool) {
	r.resultsMu.RLock()
	defer r.resultsMu.RUnlock()
	res, ok := r.results[id]
	return res, ok
}

func newResultID(presetID string) string {
	return fmt.Sprintf("%s_%s", presetID, time.Now().Format("20060102_150405.000"))
}

func getFloat(data map[string]interface{}, key string) float64 {
	if v, ok := data[key].(float64); ok {
		return v
	}
	return 0
}

func getStr(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}