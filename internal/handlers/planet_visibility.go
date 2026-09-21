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