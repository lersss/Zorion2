// internal/handlers/planet_visibility.go
// Скрытие деталей планет для role=player (спека 77a §6.2/§11.2): поверхность
// и наличие поселений — только при знании в player_planet_knowledge (с датой,
// И8); недра/атмосфера/детали поселений — всегда скрыты (сканер их не
// вскрывает, §6.2). Без знания — Knowledge=nil, клиент показывает
// «нет данных — купить отчёт». admin/skycomposer — без фильтра (И7).
package handlers

import (
	"time"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// applyPlanetVisibility — применяет знание к планетам системы для player.
func applyPlanetVisibility(userID string, planets []models.Planet, knowledge *repository.KnowledgeRepository) []models.Planet {
	out := make([]models.Planet, 0, len(planets))
	for _, p := range planets {
		k, err := knowledge.GetKnowledge(userID, p.ID)
		if err != nil || k == nil {
			out = append(out, stripPlanetDetails(p, nil))
			continue
		}
		view := &models.PlanetKnowledgeView{
			ScannedAt: k.ScannedAt,
			Fresh:     k.IsFresh(time.Now()),
		}
		if sd, ok := k.Data["surface_dominant"].(string); ok {
			view.SurfaceDominant = sd
		}
		if sc, ok := k.Data["surface_composition"].(map[string]interface{}); ok {
			view.SurfaceComposition = toFloatMap(sc)
		}
		if n, ok := k.Data["settlements_count"].(float64); ok {
			view.SettlementsCount = int(n)
		}
		out = append(out, stripPlanetDetails(p, view))
	}
	return out
}

// stripPlanetDetails — убирает детали планеты из ответа для player:
// поверхность/недра/атмосфера/ядро/поселения/описание. Базовые поля объекта
// (имя, тип, орбита, физика) остаются (спека 77a §5.2). Знание (если есть) —
// в p.Knowledge.
func stripPlanetDetails(p models.Planet, view *models.PlanetKnowledgeView) models.Planet {
	p.Knowledge = view
	p.SurfaceDominant = ""
	p.SurfaceComposition = nil
	p.SubterrainComposition = nil
	// Биомы/недры объектами (99.2.28 §9.3) — тоже скрыты для player: утечка
	// И1 с релиза 99.2.28 (находка 2026-09-20 §10.5). Картинка планеты из
	// биомов при этом остаётся допустимой — вид разрешён создателем (С1/гейт 2).
	p.Biomes = nil
	p.Subterrain = nil
	p.Atmosphere = ""
	p.Hydrosphere = ""
	p.Biosphere = ""
	p.Archetype = ""
	p.AtmosphereData = nil
	p.LiquidWaterPossible = false
	p.OrbitalPeriod = 0
	p.Eccentricity = 0
	p.EscapeVelocity = 0
	p.TidalLock = false
	p.Population = 0
	p.Core = nil
	p.Settlements = nil
	// Фракции/строения планеты (спека 2026-09-21-фабрики-релиз-2-столицы-фракций
	// §5/§7 п.6): player видит их только со знанием о планете (сканер вскрывает
	// фракции — осознанная дельта 77a §6.2). Без знания — nil (защита в
	// глубину, как у поселений); со знанием — остаются.
	if view == nil {
		p.Factions = nil
		p.Buildings = nil
		// Залежи поверхности (спека 2026-09-22-поселение-... §5.1): без знания
		// о планете игрок залежей не видит — защита в глубину (как фракции/
		// строения); со знанием остаются. attachDeposits зовётся только в
		// GetPlanetsByWorldID — других путей к игроку нет.
		p.Deposits = nil
	}
	p.Description = ""
	p.SystemAge = 0
	p.Moons = 0
	p.Radioactive = false
	// Спутники — объекты системы; их детали (поверхность/атмосфера) тоже
	// скрыты для player (консистентность с §5.2).
	for i := range p.Satellites {
		s := &p.Satellites[i]
		s.SurfaceDominant = ""
		s.SurfaceComposition = nil
		s.SubterrainComposition = nil
		s.Atmosphere = ""
		s.Biosphere = ""
		s.Description = ""
	}
	return p
}

// applyBeltVisibility — выдача поясов игроку (спека
// 2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт §4.2/§4.3): только
// visible=true (фильтр ДО маппинга); состав (composition) раскрывается при
// знании — система в радиусе радара (inRadar) ИЛИ присутствие игрока в поясе
// (myPosition: orbit на belt с этим id). Без знания состав не отдаётся.
func applyBeltVisibility(belts []models.Belt, inRadar bool, myPosition *models.CurrentPosition) []models.BeltView {
	out := make([]models.BeltView, 0, len(belts))
	for _, b := range belts {
		if !b.Visible {
			continue
		}
		revealed := inRadar || beltPresence(myPosition, b.ID)
		out = append(out, stripBeltDetails(b, revealed))
	}
	return out
}

// visibleBelts — только видимые игроку пояса (visible=true, §4.2): пояс
// visible=false игроку не отдаётся и не является валидной целью полёта.
func visibleBelts(belts []models.Belt) []models.Belt {
	out := make([]models.Belt, 0, len(belts))
	for _, b := range belts {
		if b.Visible {
			out = append(out, b)
		}
	}
	return out
}

// beltPresence — игрок физически в этом поясе (позиция orbit на belt, §4.3
// условие 2): присутствие даёт состав без сканера.
func beltPresence(pos *models.CurrentPosition, beltID string) bool {
	return pos != nil && pos.Status == "orbit" &&
		pos.ObjectType == "belt" && pos.ObjectID == beltID
}

// stripBeltDetails — маппит запись system_belts в BeltView для игрока (спека
// 2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт §4.2): базовые поля
// (тип/имя/геометрия/типичное тело/масса) открыты; composition — только при
// знании (compositionRevealed, §4.3); data/visible/created_at/updated_at не
// отдаются. models.Belt напрямую игроку не сериализуется.
func stripBeltDetails(b models.Belt, compositionRevealed bool) models.BeltView {
	v := models.BeltView{
		ID:         b.ID,
		WorldID:    b.WorldID,
		Kind:       b.Kind,
		Name:       b.Name,
		OrbitIndex: b.OrbitIndex,
		RadiusAU:   b.RadiusAU,
		WidthAU:    b.WidthAU,
		Mass:       b.Mass,
		BodySizeKm: b.BodySizeKm,
	}
	if compositionRevealed {
		v.Composition = b.Composition
	}
	return v
}

// toFloatMap — map[string]interface{} → map[string]float64 (значения-числа).
func toFloatMap(m map[string]interface{}) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		if f, ok := v.(float64); ok {
			out[k] = f
		}
	}
	return out
}
