// internal/handlers/admin_mortality.go
package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"zorion/internal/economy/settlement"
	"zorion/internal/repository"
)

// MortalityPreview — предпросмотр изменения населения планеты от среды
// (температура/гравитация/радиоактивность), без изменения БД
// (docs/gamedesign/18a_population_death.md). Население для расчёта — сумма
// населения уже существующих поселений планеты, либо query-параметр p0
// (например, для планеты без поселений).
//
// GET /admin/mortality-preview?planet_id=...&p0=1000000
func (h *AdminHandlers) MortalityPreview(w http.ResponseWriter, r *http.Request) {
	planetID := r.URL.Query().Get("planet_id")
	if planetID == "" {
		http.Error(w, "planet_id required", http.StatusBadRequest)
		return
	}

	planet, err := repository.NewPlanetRepository(h.db).GetPlanetByID(planetID)
	if err != nil {
		http.Error(w, "Failed to fetch planet: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if planet == nil {
		http.Error(w, "Planet not found", http.StatusNotFound)
		return
	}

	p0 := float64(planet.Population)
	if raw := r.URL.Query().Get("p0"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			http.Error(w, "Bad p0", http.StatusBadRequest)
			return
		}
		p0 = parsed
	}

	radioactivity := 0.0
	if planet.Core != nil {
		radioactivity = planet.Core.Radioactivity
	}

	input := settlement.PlanetInput{
		TemperatureK:      planet.Temperature,
		GravityG:          planet.Gravity,
		CoreRadioactivity: radioactivity,
	}
	rPerSec, lambdaPerHour := settlement.ChangeComponents(input, settlement.DefaultScale)
	tDeath := settlement.DeathMomentSeconds(p0, rPerSec)
	var tDeathHours *float64
	if !math.IsInf(tDeath, 1) {
		h := tDeath / 3600
		tDeathHours = &h
	}

	response := struct {
		PlanetID        string             `json:"planet_id"`
		TemperatureK    float64            `json:"temperature_k"`
		GravityG        float64            `json:"gravity_g"`
		Radioactivity   float64            `json:"core_radioactivity"`
		Severity        map[string]float64 `json:"severity"`
		RPerSec         float64            `json:"r_per_sec"`
		TDeathHours     *float64           `json:"t_death_hours,omitempty"`
		LambdaPerHour   float64            `json:"lambda_per_hour"`
		Uninhabitable   bool               `json:"uninhabitable"`
		P0              float64            `json:"p0"`
		Projection      map[string]float64 `json:"projection"`
	}{
		PlanetID:      planet.ID,
		TemperatureK:  planet.Temperature,
		GravityG:      planet.Gravity,
		Radioactivity: radioactivity,
		// Изменение населения = сумма компонент (99.2.12): рекурсивная —
		// жара (r_per_sec, t_смерти), λ-компоненты — прочие факторы
		// (lambda_per_hour); severity температуры не применяется; +Inf
		// (холод ≤ 100 K, временный полюс) клампится на границе.
		Severity: map[string]float64{
			"gravity":       settlement.TwoSidedSeverity(settlement.HumanGravityProfile, planet.Gravity),
			"radioactivity": settlement.OneSidedSeverity(settlement.HumanRadioactivityProfile, radioactivity),
		},
		RPerSec:       rPerSec,
		TDeathHours:   tDeathHours,
		LambdaPerHour: settlement.ClampLambda(lambdaPerHour),
		Uninhabitable: settlement.Uninhabitable(input, p0),
		P0:            p0,
		Projection:    settlement.Projection(p0, rPerSec, lambdaPerHour),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
