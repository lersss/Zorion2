// internal/handlers/admin_probe.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"zorion/internal/generator"
	"zorion/internal/mapcache"
	"zorion/internal/probe"
)

// ProbeRun — запускает батчевый прогон пробы смертности.
// Тело: {preset_id?, overrides?, seed?, sample_size?, horizon_days?}.
// Асинхронный, прогресс через /admin/generate-status?job=probe_run.
func (h *AdminHandlers) ProbeRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PresetID    string                 `json:"preset_id"`
		Overrides   map[string]interface{} `json:"overrides"`
		Seed        *int64                 `json:"seed"`
		SampleSize  *int                   `json:"sample_size"`
		HorizonDays *float64               `json:"horizon_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	params := probe.DefaultCurveParams()
	if req.PresetID != "" {
		if p, ok := probe.PresetByID(req.PresetID); ok {
			params = p.Params
		}
	}
	if len(req.Overrides) > 0 {
		params = probe.ApplyOverrides(params, probe.ParseOverrides(req.Overrides))
	}

	runOpts := probe.DefaultRunOptions()
	if req.Seed != nil {
		runOpts.Seed = *req.Seed
	}
	if req.SampleSize != nil {
		runOpts.SampleSize = *req.SampleSize
	}
	if req.HorizonDays != nil {
		runOpts.HorizonDays = *req.HorizonDays
	}

	var planetCount int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM planets`).Scan(&planetCount); err != nil {
		http.Error(w, "Failed to count planets", http.StatusInternalServerError)
		return
	}
	total := planetCount
	if runOpts.SampleSize > 0 && runOpts.SampleSize < total {
		total = runOpts.SampleSize
	}

	ctx, cancel := context.WithCancel(context.Background())
	if !statusManager.TryStart(generator.JobProbeRun, total, cancel) {
		cancel()
		http.Error(w, "Probe run already in progress", http.StatusConflict)
		return
	}

	presetID := req.PresetID
	if presetID == "" {
		presetID = "custom"
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ ProbeRun panic: %v", rec)
				statusManager.Fail(generator.JobProbeRun, "panic: "+recoverErr(rec))
			}
		}()

		progress := func(processed, _ int) {
			statusManager.Progress(generator.JobProbeRun, processed)
		}
		res, err := h.probeRunner.Run(ctx, presetID, params, runOpts, progress)
		if err != nil {
			log.Printf("❌ ProbeRun: %v", err)
			statusManager.Fail(generator.JobProbeRun, err.Error())
			return
		}
		log.Printf("✅ ProbeRun: %s (поселений %d, медиана %v сут)",
			res.ID, res.Total, res.Stats.MedianDays)
		statusManager.Done(generator.JobProbeRun)
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"started"}`))
}

// ProbePresets — список пресетов (открытый, из конфига).
func (h *AdminHandlers) ProbePresets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(probe.Presets())
}

// ProbeResults — список прогонов, новые сверху. Компактный: без кривых.
func (h *AdminHandlers) ProbeResults(w http.ResponseWriter, r *http.Request) {
	type item struct {
		ID             string  `json:"id"`
		PresetID       string  `json:"preset_id"`
		CreatedAt      string  `json:"created_at"`
		Total          int     `json:"total"`
		MedianDays     float64 `json:"median_days"`
		ExtinctFraction float64 `json:"extinct_fraction"`
	}
	list := make([]item, 0)
	for _, res := range h.probeRunner.List() {
		list = append(list, item{
			ID:              res.ID,
			PresetID:        res.PresetID,
			CreatedAt:       res.CreatedAt,
			Total:           res.Total,
			MedianDays:      res.Stats.MedianDays,
			ExtinctFraction: res.Stats.ExtinctFraction,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// ProbeResult — полный результат прогона.
// ?stats=1 — только статистика; ?curves=N — только первые N кривых.
func (h *AdminHandlers) ProbeResult(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/admin/probe/results/")
	if id == "" {
		http.Error(w, "result id required", http.StatusBadRequest)
		return
	}
	res, ok := h.probeRunner.Get(id)
	if !ok {
		http.Error(w, "result not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	if q.Get("stats") == "1" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         res.ID,
			"preset_id":  res.PresetID,
			"created_at": res.CreatedAt,
			"total":      res.Total,
			"params":     res.Params,
			"run":        res.Run,
			"stats":      res.Stats,
		})
		return
	}
	if n := q.Get("curves"); n != "" {
		count := 0
		fmt.Sscanf(n, "%d", &count)
		if count <= 0 {
			count = 1
		}
		if count > len(res.Curves) {
			count = len(res.Curves)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           res.ID,
			"total":        res.Total,
			"horizon_days": res.Run.HorizonDays,
			"curves":       res.Curves[:count],
		})
		return
	}
	json.NewEncoder(w).Encode(res)
}

// ProbePlayback — per-мировые кривые для Maptest: результат прогона,
// соединённый с координатами миров из снапшота. По одному поселению
// на мир (первое в результате). Опционально ограничивается по границам.
func (h *AdminHandlers) ProbePlayback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	if file == "" {
		http.Error(w, "file parameter required", http.StatusBadRequest)
		return
	}
	res, ok := h.probeRunner.Get(file)
	if !ok {
		http.Error(w, "result not found", http.StatusNotFound)
		return
	}

	xMin := qf(q, "x_min", math.Inf(-1))
	xMax := qf(q, "x_max", math.Inf(1))
	yMin := qf(q, "y_min", math.Inf(-1))
	yMax := qf(q, "y_max", math.Inf(1))

	worldByID := map[string]mapcache.World{}
	if snap := h.mapCache.Snapshot(); snap != nil {
		for _, w := range snap.Worlds() {
			worldByID[w.ID] = w
		}
	}

	seen := map[string]bool{}
	out := []map[string]interface{}{}
	for _, c := range res.Curves {
		if seen[c.WorldID] {
			continue
		}
		seen[c.WorldID] = true
		w, ok := worldByID[c.WorldID]
		if !ok {
			continue
		}
		if w.X < xMin || w.X > xMax || w.Y < yMin || w.Y > yMax {
			continue
		}
		out = append(out, map[string]interface{}{
			"world_id":    c.WorldID,
			"name":        w.Name,
			"x":           w.X,
			"y":           w.Y,
			"spectral":    w.Spectral,
			"class":       c.Class,
			"planet_name": c.PlanetName,
			"p0":          c.P0,
			"lifetime":    c.Lifetime,
			"alpha":       c.Alpha,
			"n_dead":      c.NDead,
			"t0":          c.T0,
			"h_planet":    c.HPlanet,
			"k":           c.K,
			"t":           c.T,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"horizon_days": res.Run.HorizonDays,
		"rows":         out,
	})
}

func qf(q url.Values, key string, def float64) float64 {
	v := q.Get(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}