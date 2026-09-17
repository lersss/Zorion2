// internal/handlers/admin_resources.go
// Read-only просмотр универсального слоя (спека 94a): каталог 20 ресурсов,
// 13 шаблонов хемотипов, покрытие рас (для каждой оси с весом ≥ 10 — ресурсы
// каталога, чей профиль попадает в окно расы). Данные — из internal/resource
// (LayerCatalog/LayerTemplates/DeriveDiet/CoveringResources/CheckAllFed),
// хендлер только сериализует — логика не дублируется.
//
//	GET /admin/resources — auth.AdminAuth (admin + skycomposer).
package handlers

import (
	"net/http"

	"zorion/internal/races"
	"zorion/internal/resource"
)

// adminResourceDTO — ресурс каталога для админки (проекция Resource).
type adminResourceDTO struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Icon     string   `json:"category_icon"`
	Bridge   bool     `json:"bridge"`
	Closes   []string `json:"closes"`

	// 10 осей свойств 0–100.
	Hardness         float64 `json:"hardness"`
	Elasticity       float64 `json:"elasticity"`
	Conductivity     float64 `json:"conductivity"`
	Density          float64 `json:"density"`
	EnergyDensity    float64 `json:"energy_density"`
	Biocompatibility float64 `json:"biocompatibility"`
	Radioactivity    float64 `json:"radioactivity"`
	Toxicity         float64 `json:"toxicity"`
	Flammability     float64 `json:"flammability"`
	ChemicalActivity float64 `json:"chemical_activity"`

	TMelt         float64 `json:"t_melt"`
	TBoil         float64 `json:"t_boil"`
	Sublimating   bool    `json:"sublimating"`
	Supercritical bool    `json:"supercritical"`
}

// adminTemplateDTO — шаблон хемотипа (кратко: ось + фаза).
type adminTemplateDTO struct {
	Axis  string `json:"axis"`
	Phase string `json:"phase"`
}

// adminRaceCoverageDTO — покрытие расы: consumption-вектор + по каждой оси
// с весом ≥ 10 список id ресурсов каталога, попадающих в окно расы.
type adminRaceCoverageDTO struct {
	RaceID      string              `json:"race_id"`
	Name        string              `json:"name"`
	Consumption map[string]float64  `json:"consumption"`
	Coverage    map[string][]string `json:"coverage"`
}

// adminResourcesResponse — ответ GET /admin/resources.
type adminResourcesResponse struct {
	Resources []adminResourceDTO     `json:"resources"`
	Templates []adminTemplateDTO     `json:"templates"`
	Races     []adminRaceCoverageDTO `json:"races"`
	Gaps      []resource.FedGap      `json:"gaps"`
}

// GetAdminResources — GET /admin/resources: каталог слоя + шаблоны + покрытие.
func (h *AdminHandlers) GetAdminResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	if len(races.Catalog()) == 0 {
		writeJSONError(w, "Каталог рас не загружен (config/races.json)", http.StatusInternalServerError)
		return
	}

	catalog := resource.LayerCatalog()
	templates := resource.LayerTemplates()
	allRaces := races.Catalog()

	resp := adminResourcesResponse{
		Resources: make([]adminResourceDTO, 0, len(catalog)),
		Templates: make([]adminTemplateDTO, 0, len(templates)),
		Races:     make([]adminRaceCoverageDTO, 0, len(allRaces)),
	}

	for _, res := range catalog {
		resp.Resources = append(resp.Resources, adminResourceDTO{
			ID: res.ID, Name: res.Name, Category: res.Category,
			Icon:   resource.GetCategory(res.Category).Icon,
			Bridge: res.Bridge, Closes: res.Closes,
			Hardness: res.Hardness, Elasticity: res.Elasticity,
			Conductivity: res.Conductivity, Density: res.Density,
			EnergyDensity: res.EnergyDensity, Biocompatibility: res.Biocompatibility,
			Radioactivity: res.Radioactivity, Toxicity: res.Toxicity,
			Flammability: res.Flammability, ChemicalActivity: res.ChemicalActivity,
			TMelt: res.TMelt, TBoil: res.TBoil,
			Sublimating: res.Sublimating(), Supercritical: res.Supercritical,
		})
	}

	// Шаблоны — в каноническом порядке 13 consumption-осей.
	for _, axis := range resource.ConsumptionAxes() {
		tpl, ok := templates[axis]
		if !ok {
			continue
		}
		resp.Templates = append(resp.Templates, adminTemplateDTO{Axis: tpl.Axis, Phase: string(tpl.Phase)})
	}

	// Покрытие рас: роботы (Robotic != nil) — отдельный слой, не в этой итерации.
	for _, race := range allRaces {
		if race.Robotic != nil {
			continue
		}
		rc := adminRaceCoverageDTO{
			RaceID:      race.ID,
			Name:        race.Name,
			Consumption: race.Consumption,
			Coverage:    map[string][]string{},
		}
		for axis, weight := range race.Consumption {
			if weight < 10 {
				continue // доли < 10 необязательны (§8.1)
			}
			rc.Coverage[axis] = resource.CoveringResources(catalog, templates, race, axis)
		}
		resp.Races = append(resp.Races, rc)
	}

	resp.Gaps = resource.CheckAllFed(catalog, templates, allRaces)
	if resp.Gaps == nil {
		resp.Gaps = []resource.FedGap{} // пустой список, не null
	}
	writeJSONStatus(w, http.StatusOK, resp)
}