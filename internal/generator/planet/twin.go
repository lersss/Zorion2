// internal/generator/planet/twin.go
//
// «Близнецы» — детерминированная генерация для «Проверки гипотез»
// (specs/hypothesis_testing.md). Канонический шаблон планеты клонируется N раз;
// в группе переопределяется ровно то, что проверяет гипотеза (например,
// system_age). Никакого каскада: поля копируются из шаблона побайтово,
// оверрайды перезаписывают ось. Дельту между группами можно атрибутировать
// одной оси.
//
// На каждую группу — одна звезда; звезды двух групп стоят визуально рядом
// (реализм не нужен, никаких «квадратов»): группа i на координате (i*S, 0).
package planet

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/google/uuid"
	"zorion/internal/generator/galaxy"
	"zorion/internal/generator/settlement"
	"zorion/internal/names"
)

// twinStarOffset — расстояние между звездами двух групп на карте.
const twinStarOffset = 600.0

// ControlledWorld — мир (звезда) группы для генерации близнецов (минимальный
// набор для вставки в worlds; физика планеты — в клонах data).
type ControlledWorld struct {
	ID            string
	Name          string
	SpectralClass string
	SystemAge     float64
	CoordX        float64
	CoordY        float64
	// Temperature — температура звезды (для карты), согласована со спектром.
	Temperature int
}

// SettlementSpec — стратегия населения поселений группы (без правил:
// планета уже клон шаблона, фильтровать нечего).
type SettlementSpec struct {
	// Chance — шанс заселения планеты (0..1).
	Chance float64 `json:"chance"`
	// Population — стратегия населения (fixed/random), как в модели генерации.
	Population settlement.Population `json:"population"`
}

// TwinGroup — одна группа «близнецов»: эксперимент или контроль. Ровно одна
// звезда; все планеты группы — клоны шаблона с оверрайдами.
type TwinGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Overrides — поля planet.data, которые в этой группе заменяются
	// относительно шаблона (например, {"system_age": 1.0}). Всё остальное
	// наследуется из Base побайтово.
	Overrides map[string]interface{} `json:"overrides,omitempty"`
	// PlanetsPerWorld — сколько планет-близнецов у звезды группы.
	PlanetsPerWorld int `json:"planets_per_world"`
	// Settlement — поселения группы; пустая Population (Kind="") — не заселять.
	Settlement SettlementSpec `json:"settlement"`
}

// TwinSpec — запрос на генерацию близнецов.
type TwinSpec struct {
	// ID — идентификатор эксперимента (тег _experiment.id).
	ID   string `json:"id"`
	Base map[string]interface{} `json:"base"` // канонический шаблон planet.data
	// Groups — «эксперимент» и «контроль» (две и более).
	Groups []TwinGroup `json:"groups"`
}

// Validate — проверяет запрос перед генерацией.
func (s *TwinSpec) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("id: пустой идентификатор эксперимента")
	}
	if len(s.Base) == 0 {
		return fmt.Errorf("base: пустой шаблон планеты")
	}
	if len(s.Groups) == 0 {
		return fmt.Errorf("groups: нужна хотя бы одна группа")
	}
	for i, g := range s.Groups {
		if g.ID == "" {
			return fmt.Errorf("groups[%d]: id пустой", i)
		}
		if g.PlanetsPerWorld <= 0 {
			return fmt.Errorf("groups[%d].planets_per_world: должно быть > 0, получил %d", i, g.PlanetsPerWorld)
		}
		if g.Settlement.Chance < 0 || g.Settlement.Chance > 1 {
			return fmt.Errorf("groups[%d].settlement.chance: должно быть от 0 до 1, получил %v", i, g.Settlement.Chance)
		}
		p := g.Settlement.Population
		switch p.Kind {
		case "":
			// поселения группы не создаются
		case "fixed":
			if p.Fixed <= 0 {
				return fmt.Errorf("groups[%d].population.fixed: должно быть > 0, получил %d", i, p.Fixed)
			}
			if p.Fixed > math.MaxInt32 {
				return fmt.Errorf("groups[%d].population.fixed: %d больше допустимого для integer-колонки settlements.population (2^31-1)", i, p.Fixed)
			}
		case "random":
			if p.Min <= 0 || p.Max < p.Min {
				return fmt.Errorf("groups[%d].population.random: нужно 0 < min <= max, получил %d..%d", i, p.Min, p.Max)
			}
			if p.Max > math.MaxInt32 {
				return fmt.Errorf("groups[%d].population.random: максимум %d больше допустимого для integer-колонки settlements.population (2^31-1)", i, p.Max)
			}
		default:
			return fmt.Errorf("groups[%d].population.kind: ожидается \"fixed\" или \"random\", получил %q", i, p.Kind)
		}
	}
	return nil
}

// PopulationKind — строка стратегии населения (для хендлера).

// GenerateTwins — звезды и планеты-близнецы по TwinSpec.
// На каждую группу — одна звезда (координаты (i*S, 0)); планеты группы —
// клоны шаблона. Возвращает звезды и планеты по группам; вставкой в БД
// занимается хендлер. progressFn вызывается после планет каждой группы.
func (g *Generator) GenerateTwins(
	spec TwinSpec,
	progressFn func(processed int),
) (map[string][]*ControlledWorld, map[string][]*PlanetData, error) {
	if err := spec.Validate(); err != nil {
		return nil, nil, err
	}

	worldsByGroup := make(map[string][]*ControlledWorld, len(spec.Groups))
	planetsByGroup := make(map[string][]*PlanetData, len(spec.Groups))
	processed := 0

	for gi, group := range spec.Groups {
		spectral := galaxy.RandomSpectralClass(g.rng)
		worldName := names.GeneratePlanetName(g.rng, g.usedNames)
		if worldName == "" {
			worldName = "Мир-" + uuidShort()
		}
		world := &ControlledWorld{
			ID:            uuid.New().String(),
			Name:          worldName,
			SpectralClass: spectral,
			CoordX:        float64(gi) * twinStarOffset,
			CoordY:        0,
			Temperature:   galaxy.RandomTemperature(spectral, g.rng),
		}
		worldsByGroup[group.ID] = []*ControlledWorld{world}

		planets := make([]*PlanetData, 0, group.PlanetsPerWorld)
		for orbit := 1; orbit <= group.PlanetsPerWorld; orbit++ {
			planets = append(planets, g.buildTwinPlanet(spec, group, world, orbit))
		}
		processed += group.PlanetsPerWorld
		if progressFn != nil {
			progressFn(processed)
		}
		planetsByGroup[group.ID] = planets
	}

	return worldsByGroup, planetsByGroup, nil
}

// buildTwinPlanet — один клон шаблона с оверрайдами группы и тегом _experiment.
func (g *Generator) buildTwinPlanet(spec TwinSpec, group TwinGroup, w *ControlledWorld, orbit int) *PlanetData {
	data := cloneTwinData(spec.Base, group.Overrides)

	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Планета-" + uuidShort()
	}

	dataJSON, _ := json.Marshal(data)
	p := &PlanetData{
		ID:         uuid.New().String(),
		WorldID:    w.ID,
		Name:       name,
		OrbitIndex: orbit,
		Data:       dataJSON,
	}
	return tagExperiment(p, spec.ID, group.ID)
}

// cloneTwinData — поверхностная копия шаблона + оверрайды. Вложенные map
// разделяются (данные неизменяемы после клонирования).
func cloneTwinData(base, overrides map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+len(overrides))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

// tagExperiment — добавляет в data тег _experiment.{id,group} — единственную
// «память» об эксперименте в БД (для SQL-анализа дельты).
func tagExperiment(p *PlanetData, experimentID, groupID string) *PlanetData {
	var data map[string]interface{}
	if err := json.Unmarshal(p.Data, &data); err != nil {
		return p
	}
	data["_experiment"] = map[string]interface{}{
		"id":    experimentID,
		"group": groupID,
	}
	dataJSON, _ := json.Marshal(data)
	p.Data = dataJSON
	return p
}