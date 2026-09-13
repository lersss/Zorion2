package faction

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"zorion/internal/names"
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

func (g *Generator) GenerateFactions() (int, error) {
	rows, err := g.db.Query(`
		SELECT p.id, p.name, p.data FROM planets p
		WHERE EXISTS (
			SELECT 1 FROM settlements s
			WHERE s.planet_id = p.id AND s.population > 0
		)
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var planets []struct {
		ID   string
		Name string
		Data map[string]interface{}
	}
	for rows.Next() {
		var id, name string
		var dataJSON []byte
		if err := rows.Scan(&id, &name, &dataJSON); err != nil {
			return 0, err
		}
		var data map[string]interface{}
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			return 0, err
		}
		planets = append(planets, struct {
			ID   string
			Name string
			Data map[string]interface{}
		}{ID: id, Name: name, Data: data})
	}

	if len(planets) == 0 {
		return 0, nil
	}

	total := 0
	usedNames := make(map[string]bool)
	for _, p := range planets {
		count := 1 + g.rng.Intn(3)
		for i := 0; i < count; i++ {
			faction := g.generateFaction(p.ID, p.Name, p.Data, usedNames)
			if err := g.saveFaction(faction); err != nil {
				return total, err
			}
			total++
		}
	}
	return total, nil
}

func (g *Generator) generateFaction(planetID, planetName string, planetData map[string]interface{}, usedNames map[string]bool) *Faction {
	name := names.GenerateFactionName(g.rng, usedNames)
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
		INSERT INTO factions (id, name, type, homeworld_id, strength, resources, color, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
	`
	_, err := g.db.Exec(query, f.ID, f.Name, f.Type, f.HomeworldID, f.Strength, resourcesJSON, f.Color, f.Description)
	return err
}

type Faction struct {
	ID          string
	Name        string
	Type        string
	HomeworldID string
	Strength    int
	Resources   map[string]float64
	Color       string
	Description string
}
