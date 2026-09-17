// internal/handlers/encyclopedia_handlers.go — энциклопедия (спека 86a §8.1):
// GET /api/encyclopedia/races — публичный срез каталога рас (whitelist
// §5.1.1) + лор. Только общий справочник, без персональных и чужих данных
// (И5, И6); каталог и лор читаются из памяти (read-only, И11).
package handlers

import (
	"encoding/json"
	"net/http"

	"zorion/internal/auth"
	"zorion/internal/races"
)

// EncyclopediaHandlers — ручки энциклопедии. Без зависимостей: читает каталог
// рас и лор из памяти (паттерн catalog, И11).
type EncyclopediaHandlers struct{}

// NewEncyclopediaHandlers — конструктор.
func NewEncyclopediaHandlers() *EncyclopediaHandlers {
	return &EncyclopediaHandlers{}
}

// GetRaces — GET /api/encyclopedia/races: публичный срез каталога рас
// (whitelist §5.1.1) + лор из config/race_lore.json. Без токена — 401 (как
// /me). Ответ: {"races": [...]}, эскиз §8.1.
func (h *EncyclopediaHandlers) GetRaces(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	loreByID := make(map[string]*races.RaceLore, len(races.LoreCatalog()))
	for _, l := range races.LoreCatalog() {
		loreByID[l.ID] = l
	}

	out := make([]encyclopediaRace, 0, len(races.Catalog()))
	for _, rc := range races.Catalog() {
		out = append(out, publicRace(rc, loreByID[rc.ID]))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"races": out})
}

// encyclopediaRace — публичный срез карточки расы (спека 86a §5.1.1 whitelist):
// без consumption/territory/флагов механик-кандидатов/off_cascade/tuning;
// forage только source; robotic без heat_twist и basic_resources.
type encyclopediaRace struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Family     string                 `json:"family"`
	Basis      string                 `json:"basis"`
	Conditions encyclopediaConditions `json:"conditions"`
	Attributes races.Attributes       `json:"attributes"`
	Home       races.Home             `json:"home"`
	Bulge      string                 `json:"bulge"`
	Forage     encyclopediaForage     `json:"forage"`
	Robotic    *encyclopediaRobotic   `json:"robotic"`
	Lore       encyclopediaLore       `json:"lore"`
}

// encyclopediaConditions — окна приёмлемости без omitempty: опциональные
// heat_flux/gravity/liquid_water отдаются null, если не заданы (§8.1).
type encyclopediaConditions struct {
	Temperature races.Window       `json:"temperature"`
	Pressure    races.Window       `json:"pressure"`
	Atmosphere  races.Atmosphere   `json:"atmosphere"`
	Radiation   races.Window       `json:"radiation"`
	HeatFlux    *races.Window      `json:"heat_flux"`
	Gravity     *races.Window      `json:"gravity"`
	LiquidWater *bool              `json:"liquid_water"`
}

// encyclopediaForage — корм: только source (basic_resources пусты до 99.2.5).
type encyclopediaForage struct {
	Source string `json:"source"`
}

// encyclopediaRobotic — слой роботов: энергия + сырьё; без heat_twist (флаг
// механики-кандидата) и materials.basic_resources (пусты до 99.2.5).
type encyclopediaRobotic struct {
	PowerSource []string                     `json:"power_source"`
	Heat        string                       `json:"heat"`
	Materials   encyclopediaRoboticMaterials `json:"materials"`
}

type encyclopediaRoboticMaterials struct {
	Categories []string          `json:"categories"`
	Axes       map[string]string `json:"axes"`
}

// encyclopediaLore — лор из config/race_lore.json; origin — только у роботов
// (null у био-рас).
type encyclopediaLore struct {
	Character   string  `json:"character"`
	HowLive     string  `json:"how_live"`
	Why         string  `json:"why"`
	Coexistence string  `json:"coexistence"`
	Origin      *string `json:"origin"`
}

// publicRace — маппинг карточки каталога в публичный срез (whitelist §5.1.1).
// Лор (family + блок lore) — из race_lore.json; нет записи — пустые поля
// (деградация при незагруженном лоре, сервер живёт).
func publicRace(rc *races.Race, lore *races.RaceLore) encyclopediaRace {
	out := encyclopediaRace{
		ID:    rc.ID,
		Name:  rc.Name,
		Basis: rc.Basis,
		Conditions: encyclopediaConditions{
			Temperature: rc.Conditions.Temperature,
			Pressure:    rc.Conditions.Pressure,
			Atmosphere:  rc.Conditions.Atmosphere,
			Radiation:   rc.Conditions.Radiation,
			HeatFlux:    rc.Conditions.HeatFlux,
			Gravity:     rc.Conditions.Gravity,
			LiquidWater: rc.Conditions.LiquidWater,
		},
		Attributes: rc.Attributes,
		Home:       rc.Home,
		Bulge:      rc.Bulge,
		Forage:     encyclopediaForage{Source: rc.Forage.Source},
	}
	if rc.Robotic != nil {
		out.Robotic = &encyclopediaRobotic{
			PowerSource: rc.Robotic.PowerSource,
			Heat:        rc.Robotic.Heat,
			Materials: encyclopediaRoboticMaterials{
				Categories: rc.Robotic.Materials.Categories,
				Axes:       rc.Robotic.Materials.Axes,
			},
		}
	}
	if lore != nil {
		out.Family = lore.Family
		out.Lore = encyclopediaLore{
			Character:   lore.Character,
			HowLive:     lore.HowLive,
			Why:         lore.Why,
			Coexistence: lore.Coexistence,
		}
		if lore.Origin != "" {
			origin := lore.Origin
			out.Lore.Origin = &origin
		}
	}
	return out
}