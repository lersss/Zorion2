// internal/handlers/admin_generation_config.go
package handlers

import (
	"encoding/json"
	"net/http"

	"zorion/internal/generator/galaxy"
	"zorion/internal/generator/planet"
)

// GenerationConfigPayload — конфиг генерации (99.2.3 §3/§4): веса звёзд,
// средние числа планет и диапазоны масс (29a §4м). Дефолты — из 99.2.4
// (§4.1/§4.2/§5.2) и 29a §4м, источник один.
// RaceTuningSoftness/RaceClusterPlanetCountMult — подкрутка под расу-дома
// (99.2.22 §4.3): мягкость s и множитель числа планет в кластерах рас.
type GenerationConfigPayload struct {
	StarWeights       galaxy.Weights           `json:"star_weights"`
	PlanetMeans       planet.PlanetMeans       `json:"planet_means"`
	StellarMassRanges galaxy.StellarMassRanges `json:"stellar_mass_ranges"`

	// RaceTuningSoftness — «Подкрутка под расу-дома» (0–1, дефолт 0.5):
	// доля миров кластера, подстроенных под расу-дома (99.2.22 §4).
	RaceTuningSoftness float64 `json:"race_tuning_softness"`
	// RaceClusterPlanetCountMult — «Число планет в кластерах рас» (0.7–1.3,
	// дефолт 1.1): mean × mult перед потолком 8 (99.2.22 §3.3 ручка 6).
	RaceClusterPlanetCountMult float64 `json:"race_cluster_planet_count_mult"`
}

// DefaultGenerationConfig — дефолты конфига генерации
// (99.2.4 §4.1/§4.2/§5.2 + 29a §4м + 99.2.22 §4.3).
func DefaultGenerationConfig() GenerationConfigPayload {
	return GenerationConfigPayload{
		StarWeights:       galaxy.DefaultWeights(),
		PlanetMeans:       planet.DefaultPlanetMeans(),
		StellarMassRanges: galaxy.DefaultStellarMassRanges(),
		// 99.2.22 §4.3: «половина миров кластера подстроена под расу»;
		// «Больше шансов на кластер» (ручка 6).
		RaceTuningSoftness:         0.5,
		RaceClusterPlanetCountMult: 1.1,
	}
}

// HandleGenerationConfig — GET/PUT /admin/generation/config (99.2.3 §4.5).
func (h *AdminHandlers) HandleGenerationConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetGenerationConfig(w, r)
	case http.MethodPut:
		h.PutGenerationConfig(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GetGenerationConfig — GET /admin/generation/config.
// Отдаёт сохранённый конфиг; если в БД пусто — дефолты (99.2.3 §4.5).
func (h *AdminHandlers) GetGenerationConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadGenerationConfig()
	if err != nil {
		http.Error(w, "Failed to load generation config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

// PutGenerationConfig — PUT /admin/generation/config.
// Валидация (99.2.3 §4.5): веса ≥ 0 и сумма > 0 — иначе 409; mean в [0, 8] —
// иначе 422 (не клампится: mean > 8 молча обрезал бы распределение, E[n] ≠ mean).
func (h *AdminHandlers) PutGenerationConfig(w http.ResponseWriter, r *http.Request) {
	var req GenerationConfigPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	// Потолок max не редактируется фронтом (99.2.3 §4.3: PLANET_MEAN_KEYS без max).
	// PUT без max не должен затирать дефолт нулём (баг #3, прогон @tester):
	// Max ≤ 0 бессмыслен (meanPlanetCount трактует как 8), возвращаем дефолт.
	if req.PlanetMeans.Max <= 0 {
		req.PlanetMeans.Max = planet.DefaultPlanetMeans().Max
	}
	// Диапазоны масс не слал фронт (старые клиенты/пустая форма) — не затирать:
	// берём сохранённые, иначе дефолты (как с max, 29a §4м).
	if len(req.StellarMassRanges) == 0 {
		if cur, err := h.loadGenerationConfig(); err == nil && len(cur.StellarMassRanges) > 0 {
			req.StellarMassRanges = cur.StellarMassRanges
		} else {
			req.StellarMassRanges = galaxy.DefaultStellarMassRanges()
		}
	}
	// Множитель числа планет в кластерах рас не слал фронт (старые клиенты) —
	// не затирать нулём (0 вне диапазона 0.7–1.3): дефолт 1.1 (99.2.22 §4.3).
	if req.RaceClusterPlanetCountMult == 0 {
		req.RaceClusterPlanetCountMult = DefaultGenerationConfig().RaceClusterPlanetCountMult
	}
	if status, msg := validateGenerationConfig(&req); status != 0 {
		http.Error(w, msg, status)
		return
	}
	if err := h.saveGenerationConfig(req); err != nil {
		http.Error(w, "Failed to save generation config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}

// loadGenerationConfig — читает конфиг из generation_config и накладывает
// поверх дефолтов (99.2.3 §3): отсутствующий ключ = дефолт.
func (h *AdminHandlers) loadGenerationConfig() (GenerationConfigPayload, error) {
	cfg := DefaultGenerationConfig()

	rows, err := h.db.Query(`SELECT key, payload FROM generation_config`)
	if err != nil {
		return cfg, err
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var payload []byte
		if err := rows.Scan(&key, &payload); err != nil {
			return cfg, err
		}
		switch key {
		case "star_weights":
			json.Unmarshal(payload, &cfg.StarWeights)
		case "planet_means":
			json.Unmarshal(payload, &cfg.PlanetMeans)
		case "stellar_mass_ranges":
			// Map-мерж поверх дефолтов: json.Unmarshal в map заменил бы её целиком,
			// отсутствующие ключи потерялись бы — поэтому сливаем вручную.
			var stored galaxy.StellarMassRanges
			if err := json.Unmarshal(payload, &stored); err == nil {
				for k, v := range stored {
					cfg.StellarMassRanges[k] = v
				}
			}
		case "race_tuning_softness":
			json.Unmarshal(payload, &cfg.RaceTuningSoftness)
		case "race_cluster_planet_count_mult":
			json.Unmarshal(payload, &cfg.RaceClusterPlanetCountMult)
		}
	}
	return cfg, rows.Err()
}

// saveGenerationConfig — upsert двух ключей конфига одной транзакцией.
func (h *AdminHandlers) saveGenerationConfig(cfg GenerationConfigPayload) error {
	tx, err := h.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range []struct {
		key string
		v   interface{}
	}{
		{"star_weights", cfg.StarWeights},
		{"planet_means", cfg.PlanetMeans},
		{"stellar_mass_ranges", cfg.StellarMassRanges},
		{"race_tuning_softness", cfg.RaceTuningSoftness},
		{"race_cluster_planet_count_mult", cfg.RaceClusterPlanetCountMult},
	} {
		payload, err := json.Marshal(item.v)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO generation_config (key, payload, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`,
			item.key, string(payload),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// validateGenerationConfig — возвращает HTTP-статус (0 = ok) и сообщение.
func validateGenerationConfig(cfg *GenerationConfigPayload) (int, string) {
	if sum := sumWeights(cfg.StarWeights.Spectral); sum <= 0 {
		return http.StatusConflict, "сумма весов спектральных классов должна быть > 0"
	}
	for k, v := range cfg.StarWeights.Spectral {
		if v < 0 {
			return http.StatusConflict, "вес спектрального класса " + k + " не может быть отрицательным"
		}
	}
	if sum := sumWeights(cfg.StarWeights.SystemTypes); sum <= 0 {
		return http.StatusConflict, "сумма весов типов систем/объектов должна быть > 0"
	}
	for k, v := range cfg.StarWeights.SystemTypes {
		if v < 0 {
			return http.StatusConflict, "вес типа " + k + " не может быть отрицательным"
		}
	}
	if err := cfg.PlanetMeans.Validate(); err != nil {
		return http.StatusUnprocessableEntity, err.Error()
	}
	// Диапазоны масс (29a §4м): min > 0, max ≥ min, max ≤ 100 → 422.
	for k, r := range cfg.StellarMassRanges {
		if r.Min <= 0 {
			return http.StatusUnprocessableEntity, k + ": min должен быть > 0"
		}
		if r.Max < r.Min {
			return http.StatusUnprocessableEntity, k + ": max ≥ min"
		}
		if r.Max > 100 {
			return http.StatusUnprocessableEntity, k + ": max ≤ 100 M☉"
		}
	}
	// Подкрутка под расу-дома (99.2.22 §4.3): мягкость s ∈ [0, 1];
	// множитель числа планет в кластерах рас 0.7–1.3.
	if cfg.RaceTuningSoftness < 0 || cfg.RaceTuningSoftness > 1 {
		return http.StatusUnprocessableEntity, "race_tuning_softness: 0–1 (доля миров кластера, подстроенных под расу)"
	}
	if cfg.RaceClusterPlanetCountMult < 0.7 || cfg.RaceClusterPlanetCountMult > 1.3 {
		return http.StatusUnprocessableEntity, "race_cluster_planet_count_mult: 0.7–1.3"
	}
	return 0, ""
}

// sumWeights — сумма весов map.
func sumWeights(m map[string]float64) float64 {
	s := 0.0
	for _, v := range m {
		s += v
	}
	return s
}