package faction

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/google/uuid"
	"zorion/internal/races"
)

type Generator struct {
	db  *sql.DB
	rng *rand.Rand
}

func NewGenerator(db *sql.DB, seed int64) *Generator {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &Generator{
		db:  db,
		rng: rand.New(rand.NewSource(seed)),
	}
}

// GenerateFactions создаёт по одной фракции на заселённую расу и идемпотентно
// добивает их столицы (идея 2026-09-22 «Фракции: одна на расу со своей
// столицей»). Кандидат — settlements с непустым race_id и population > 0;
// родная планета расы — её поселение с наибольшим населением (при равенстве —
// меньший planet_id, детерминированно). Фракция с уже существующим race_id
// пропускается. Возвращает число созданных фракций и число фактически
// созданных столиц.
func (g *Generator) GenerateFactions() (int, int, error) {
	rows, err := g.db.Query(`
		SELECT s.race_id, s.planet_id, s.population, p.name, p.data
		FROM settlements s
		JOIN planets p ON p.id = s.planet_id
		WHERE s.race_id IS NOT NULL AND s.race_id <> '' AND s.population > 0
	`)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	// Родная планета каждой расы — лучший кандидат по населению (tiebreak —
	// меньший planet_id).
	homeworlds := make(map[string]raceHomeworld)
	for rows.Next() {
		var raceID, planetID, planetName string
		var population int
		var dataJSON []byte
		if err := rows.Scan(&raceID, &planetID, &population, &planetName, &dataJSON); err != nil {
			return 0, 0, err
		}
		if prev, ok := homeworlds[raceID]; ok && !betterHomeworld(population, planetID, prev.population, prev.planetID) {
			continue
		}
		var data map[string]interface{}
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			return 0, 0, err
		}
		homeworlds[raceID] = raceHomeworld{
			planetID:   planetID,
			planetName: planetName,
			planetData: data,
			population: population,
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	existing, err := g.existingRaceIDs()
	if err != nil {
		return 0, 0, err
	}

	raceIDs := make([]string, 0, len(homeworlds))
	for raceID := range homeworlds {
		raceIDs = append(raceIDs, raceID)
	}
	sort.Strings(raceIDs)

	created := 0
	for _, raceID := range raceIDs {
		if existing[raceID] {
			continue
		}
		hw := homeworlds[raceID]
		faction := g.generateFaction(raceID, hw.planetID, hw.planetName, hw.planetData)
		if err := g.saveFaction(faction); err != nil {
			return created, 0, err
		}
		created++
	}

	// Столицы — всегда, и когда фракций 0 (§3: не ошибка, догон легаси-БД).
	capitals, err := g.EnsureCapitals()
	if err != nil {
		return created, 0, err
	}
	return created, capitals, nil
}

// raceHomeworld — родная планета расы (лучший кандидат из её поселений).
type raceHomeworld struct {
	planetID   string
	planetName string
	planetData map[string]interface{}
	population int
}

// betterHomeworld — кандидат лучше текущего, если население больше; при
// равенстве — planet_id лексикографически меньше (детерминированный tiebreak).
func betterHomeworld(population int, planetID string, curPopulation int, curPlanetID string) bool {
	if population != curPopulation {
		return population > curPopulation
	}
	return planetID < curPlanetID
}

// existingRaceIDs — множество рас, у которых фракция уже есть (идемпотентность:
// повторный прогон только добивает недостающие).
func (g *Generator) existingRaceIDs() (map[string]bool, error) {
	rows, err := g.db.Query(`SELECT race_id FROM factions WHERE race_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var raceID string
		if err := rows.Scan(&raceID); err != nil {
			return nil, err
		}
		existing[raceID] = true
	}
	return existing, rows.Err()
}

// EnsureCapitals — идемпотентный проход «столица на фракцию» (спека
// 2026-09-21-фабрики-релиз-2-столицы-фракций §3): одна столица на фракцию, на
// её родной планете (factions.homeworld_id). Повторный прогон не дублирует —
// NOT EXISTS + частичный UNIQUE uq_buildings_capital_owner (ON CONFLICT DO
// NOTHING). Возвращает число фактически созданных столиц.
func (g *Generator) EnsureCapitals() (int, error) {
	res, err := g.db.Exec(`
		INSERT INTO buildings (planet_id, building_type, owner_type, owner_id)
		SELECT f.homeworld_id, 'capital', 'faction', f.id
		FROM factions f
		WHERE NOT EXISTS (
			SELECT 1 FROM buildings b
			WHERE b.building_type = 'capital' AND b.owner_type = 'faction' AND b.owner_id = f.id
		)
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (g *Generator) generateFaction(raceID, planetID, planetName string, planetData map[string]interface{}) *Faction {
	// Имя фракции — название расы из каталога; fallback — race_id (каталог не
	// загружен / расы нет в нём).
	name := raceID
	if race := races.ByID(raceID); race != nil && race.Name != "" {
		name = race.Name
	}
	factionType := g.randomFactionType()

	resources := map[string]float64{
		"minerals": g.randomResource(planetData, "mineral"),
		"energy":   g.randomResource(planetData, "fuel"),
		"organics": g.randomResource(planetData, "organic"),
		"rare":     g.randomResource(planetData, "rare"),
	}

	color := g.randomColor()
	description := g.generateDescription(name, factionType, planetName)
	strength := 1

	return &Faction{
		ID:          uuid.New().String(),
		Name:        name,
		Type:        factionType,
		RaceID:      raceID,
		HomeworldID: planetID,
		Strength:    strength,
		Resources:   resources,
		Color:       color,
		Description: description,
	}
}

var factionTypes = []string{
	"Правительство", "Корпорация", "Альянс", "Культ", "Военный блок", "Торговая гильдия", "Научный совет",
	"Технократия", "Теократия", "Плутократия", "Аристократия", "Монархия", "Республика", "Диктатура",
	"Анархия", "ИИ-управление", "Клика", "Картель", "Синдикат", "Федерация", "Конфедерация", "Империя",
	"Княжество", "Герцогство", "Маркграфство", "Город-государство", "Колония", "Экспансия", "Конгломерат",
	"Трест", "Кооператив", "Содружество", "Лига", "Коалиция", "Пакт", "Союз", "Братство", "Орден",
	"Гильдия", "Дом", "Клан", "Племя", "Совет", "Круг", "Ассамблея", "Коллегия", "Бюро", "Агентство",
	"Исследовательский центр", "Фермерский коллектив", "Ремесленный цех", "Космопорт", "Станция",
	"Астероидная база", "Колония на газовом гиганте", "Подводная цивилизация", "Подземная цивилизация",
	"Кочевой флот", "Реликтовая цивилизация", "Кибернетический коллектив", "Генная империя",
	"Энергетический картель", "Кристаллический союз", "Виртуальное государство", "Пост-человеческий коллектив",
	"Симбиотический улей", "Космическая корпорация", "Торговый синдикат", "Военная хунта", "Техно-монастырь",
	"Экологическая лига", "Пиратский клан", "Наёмная гильдия", "Космическая строительная компания",
}

func (g *Generator) randomFactionType() string {
	return factionTypes[g.rng.Intn(len(factionTypes))]
}

func (g *Generator) randomResource(planetData map[string]interface{}, key string) float64 {
	if resources, ok := planetData["resources"].(map[string]interface{}); ok {
		if val, ok := resources[key].(float64); ok && val > 0 {
			return val
		}
	}
	return g.rng.Float64()
}

func (g *Generator) randomColor() string {
	colors := []string{"#ff6b6b", "#ffd93d", "#6bcb77", "#4d96ff", "#ff6bff", "#ff9f43", "#00d2d3", "#54a0ff", "#5f27cd", "#ff6348"}
	return colors[g.rng.Intn(len(colors))]
}

func (g *Generator) generateDescription(name, ftype, planet string) string {
	templates := []string{
		"%s — %s с центром на планете %s.",
		"%s представляет собой %s, доминирующую в регионе.",
		"%s — это %s, известная своей %s.",
		"%s — %s, контролирующая %s.",
		"%s — %s, которая славится своими %s.",
		"%s — %s, ведущая активную экспансию в %s.",
		"%s — %s, сохраняющая древние традиции на %s.",
	}
	template := templates[g.rng.Intn(len(templates))]
	adjectives := []string{"мощью", "ресурсами", "технологиями", "флотом", "дипломатией", "тайными агентами"}
	areas := []string{"космос", "торговлю", "науку", "военное дело", "культуру", "религию"}

	switch g.rng.Intn(3) {
	case 0:
		return fmt.Sprintf(template, name, ftype, planet)
	case 1:
		return fmt.Sprintf(template, name, ftype, adjectives[g.rng.Intn(len(adjectives))])
	default:
		return fmt.Sprintf(template, name, ftype, areas[g.rng.Intn(len(areas))])
	}
}

func (g *Generator) saveFaction(f *Faction) error {
	resourcesJSON, _ := json.Marshal(f.Resources)
	query := `
		INSERT INTO factions (id, name, type, race_id, homeworld_id, strength, resources, color, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
	`
	_, err := g.db.Exec(query, f.ID, f.Name, f.Type, f.RaceID, f.HomeworldID, f.Strength, resourcesJSON, f.Color, f.Description)
	return err
}

type Faction struct {
	ID          string
	Name        string
	Type        string
	RaceID      string
	HomeworldID string
	Strength    int
	Resources   map[string]float64
	Color       string
	Description string
}
