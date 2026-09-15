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
type GenerationConfigPayload struct {
	StarWeights       galaxy.Weights           `json:"star_weights"`
	PlanetMeans       planet.PlanetMeans       `json:"planet_means"`
	StellarMassRanges galaxy.StellarMassRanges `json:"stellar_mass_ranges"`
}

// DefaultGenerationConfig — дефолты конфига генерации
// (99.2.4 §4.1/§4.2/§5.2 + 29a §4м).
func DefaultGenerationConfig() GenerationConfigPayload {
	return GenerationConfigPayload{
		StarWeights:       galaxy.DefaultWeights(),
		PlanetMeans:       planet.DefaultPlanetMeans(),
		StellarMassRanges: galaxy.DefaultStellarMassRanges(),
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