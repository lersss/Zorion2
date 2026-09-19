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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"zorion/internal/races"
	"zorion/internal/repository"
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

	// Каталог реальных веществ (оценка ёмкости, идея 2026-09-18 §4б):
	// 111 веществ + сводка различимости (distance.go) + семейства.
	Real        []adminRealResourceDTO `json:"real"`
	RealSummary adminRealSummaryDTO    `json:"real_summary"`
	Families    []string               `json:"families"`

	// Шаблоны хемотипов (спека 94a §3.1) — реальные окна «расы» для витрины
	// форматов подвкладки «Реальные вещества» (окна товаров — демо в JS,
	// реестр §5.2.2 не зафиксирован).
	Chemotypes []adminChemotypeDTO `json:"chemotypes"`
}

// adminChemotypeDTO — шаблон хемотипа для витрины окон (спека 94a §3.1):
// окна T_melt/T_boil, окна по осям свойств, уместные категории, фаза.
type adminChemotypeDTO struct {
	Axis        string               `json:"axis"`
	Phase       string               `json:"phase"`
	Sublimating bool                 `json:"sublimating"`
	TMelt       *adminIntervalDTO    `json:"t_melt,omitempty"`
	TBoil       *adminIntervalDTO    `json:"t_boil,omitempty"`
	Windows     []adminAxisWindowDTO `json:"windows"`
	Categories  []string             `json:"categories"`
}

// adminIntervalDTO — включительный интервал [Lo, Hi] (границы входят).
type adminIntervalDTO struct {
	Lo float64 `json:"lo"`
	Hi float64 `json:"hi"`
}

// adminAxisWindowDTO — окно по оси свойств (имя оси — русское, как в Go).
type adminAxisWindowDTO struct {
	Axis string  `json:"axis"`
	Lo   float64 `json:"lo"`
	Hi   float64 `json:"hi"`
}

// adminRealResourceDTO — реальное вещество каталога-витрины (проекция
// resource.RealResource).
type adminRealResourceDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Family   string `json:"family"`
	Category string `json:"category"`
	Icon     string `json:"category_icon"`

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

// adminRealSummaryDTO — сводка различимости каталога реальных веществ
// (числа оценки §4а, считаются distance.go).
type adminRealSummaryDTO struct {
	Total      int `json:"total"`
	ByAxes     int `json:"by_axes"`
	WithT      int `json:"with_t"`
	Collisions int `json:"collisions"`
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

	// Каталог реальных веществ + сводка различимости (спека iterB §5.3):
	// real/real_summary/families — из БД (goods kind=resource с props.family),
	// не из real.go (С1); DistinctCount пересчитывается на собранных из БД
	// данных (развилка 6). Пользовательские ресурсы без props в витрину не
	// попадают (критерий приёмки §7.6 — ожидаемо, не баг).
	realList, err := h.realResourcesFromDB()
	if err != nil {
		writeJSONError(w, "ошибка чтения real-каталога: "+err.Error(), http.StatusInternalServerError)
		return
	}
	byAxes, withT, collisions := resource.DistinctCount(realList)
	resp.Real = make([]adminRealResourceDTO, 0, len(realList))
	for _, rr := range realList {
		resp.Real = append(resp.Real, adminRealResourceDTO{
			ID: rr.ID, Name: rr.Name, Family: rr.Family, Category: rr.Category,
			Icon:   resource.GetCategory(rr.Category).Icon,
			Hardness: rr.Hardness, Elasticity: rr.Elasticity,
			Conductivity: rr.Conductivity, Density: rr.Density,
			EnergyDensity: rr.EnergyDensity, Biocompatibility: rr.Biocompatibility,
			Radioactivity: rr.Radioactivity, Toxicity: rr.Toxicity,
			Flammability: rr.Flammability, ChemicalActivity: rr.ChemicalActivity,
			TMelt: rr.TMelt, TBoil: rr.TBoil,
			Sublimating: rr.Sublimating(), Supercritical: false,
		})
	}
	resp.RealSummary = adminRealSummaryDTO{
		Total:      len(realList),
		ByAxes:     byAxes,
		WithT:      withT,
		Collisions: len(collisions),
	}
	resp.Families = realFamilies(realList)

	// Шаблоны хемотипов — в каноническом порядке 13 consumption-осей
	// (окна «расы» для витрины форматов).
	resp.Chemotypes = make([]adminChemotypeDTO, 0, len(templates))
	for _, axis := range resource.ConsumptionAxes() {
		tpl, ok := templates[axis]
		if !ok {
			continue
		}
		ct := adminChemotypeDTO{
			Axis: tpl.Axis, Phase: string(tpl.Phase), Sublimating: tpl.Sublimating,
			Windows:    make([]adminAxisWindowDTO, 0, len(tpl.Windows)),
			Categories: tpl.Categories,
		}
		if tpl.TMelt != nil {
			ct.TMelt = &adminIntervalDTO{Lo: tpl.TMelt.Lo, Hi: tpl.TMelt.Hi}
		}
		if tpl.TBoil != nil {
			ct.TBoil = &adminIntervalDTO{Lo: tpl.TBoil.Lo, Hi: tpl.TBoil.Hi}
		}
		for _, w := range tpl.Windows {
			ct.Windows = append(ct.Windows, adminAxisWindowDTO{Axis: w.Axis, Lo: w.Lo, Hi: w.Hi})
		}
		resp.Chemotypes = append(resp.Chemotypes, ct)
	}

	writeJSONStatus(w, http.StatusOK, resp)
}

// realResourcesFromDB — real-ресурсы витрины из БД (спека iterB §5.3):
// props JSONB (русские ключи осей) → []*resource.RealResource.
func (h *AdminHandlers) realResourcesFromDB() ([]*resource.RealResource, error) {
	rows, err := repository.NewGoodsRepository(h.db).RealResources()
	if err != nil {
		return nil, err
	}
	out := make([]*resource.RealResource, 0, len(rows))
	for _, row := range rows {
		rr, err := realResourceFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rr)
	}
	return out, nil
}

// realResourceFromRow — строка БД (props JSONB) → resource.RealResource.
// Маппинг русских ключей осей → поля (источник — resource.Axis* константы,
// layer.go:8–17; в клиенте тот же маппинг — RUS_AXIS_TO_KEY, resources.js:531).
func realResourceFromRow(row repository.RealResourceRow) (*resource.RealResource, error) {
	var props map[string]interface{}
	if err := json.Unmarshal(row.Props, &props); err != nil {
		return nil, fmt.Errorf("props %s: %w", row.Name, err)
	}
	axis := func(key string) float64 {
		if v, ok := props[key].(float64); ok {
			return v
		}
		return 0
	}
	rr := &resource.RealResource{
		ID:       strconv.FormatInt(row.ID, 10), // id — строка (BIGSERIAL → string)
		Name:     row.Name,
		Category: row.Category, // code из categories.code
		Hardness: axis(resource.AxisHardness), Elasticity: axis(resource.AxisElasticity),
		Conductivity: axis(resource.AxisConductivity), Density: axis(resource.AxisDensity),
		EnergyDensity: axis(resource.AxisEnergyDensity), Biocompatibility: axis(resource.AxisBiocompatibility),
		Radioactivity: axis(resource.AxisRadioactivity), Toxicity: axis(resource.AxisToxicity),
		Flammability: axis(resource.AxisFlammability), ChemicalActivity: axis(resource.AxisChemicalActivity),
		TMelt: axis("t_melt_k"), TBoil: axis("t_boil_k"), // в K (клиент k2c)
	}
	if fam, ok := props["family"].(string); ok {
		rr.Family = fam
	}
	return rr, nil
}

// realFamilies — уникальные семейства real-каталога в порядке появления
// (как RealFamilies; клиент порядок не фиксирует).
func realFamilies(list []*resource.RealResource) []string {
	seen := make(map[string]bool)
	fams := make([]string, 0, len(list))
	for _, rr := range list {
		if seen[rr.Family] {
			continue
		}
		seen[rr.Family] = true
		fams = append(fams, rr.Family)
	}
	return fams
}