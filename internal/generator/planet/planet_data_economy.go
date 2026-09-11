// internal/generator/planet/planet_data_economy.go
package planet

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"zorion/internal/resource"
)

// collectEconomy — генерирует поселения, заводы и товары для планеты
// и добавляет их в буферы для батч-вставки.
//
// Пропускает:
//   - газовые гиганты (нет поверхности для жизни);
//   - радиоактивные планеты (нет населения);
//   - планеты без жизни и населения.
func (g *Generator) collectEconomy(
	planetID string,
	dataJSON []byte,
	spectralClass string,
	settlementRows *[]interface{},
	factoryRows *[]interface{},
	goodsRows *[]interface{},
) error {
	var data map[string]interface{}
	if err := json.Unmarshal(dataJSON, &data); err != nil {
		return err
	}

	// Пропуск: газовые гиганты и радиоактивные
	if isGasGiant(data) || isRadioactive(data) {
		return nil
	}

	life := getBool(data, "life")
	populationRaw := getFloat(data, "population")
	population := int(populationRaw)
	habitable := getBool(data, "habitable")

	if !life && population == 0 {
		return nil
	}

	// 1. Поселение
	settlement := buildSettlement(planetID, population, habitable, g.rng.Intn(41)+40)
	*settlementRows = append(*settlementRows, settlement)

	// 2. Заводы (по ресурсам)
	resources := extractResources(data)
	for category, value := range resources {
		if value <= 0.3 {
			continue
		}
		factories := buildFactories(planetID, category, value, g.rng)
		for _, f := range factories {
			*factoryRows = append(*factoryRows, f)
		}
	}

	// 3. Товары
	goods := buildGoods(planetID, g.rng)
	for _, gd := range goods {
		*goodsRows = append(*goodsRows, gd)
	}

	return nil
}

// ==================== ПОСЕЛЕНИЯ ====================

// buildSettlement — строка поселения для вставки в БД.
func buildSettlement(planetID string, population int, habitable bool, stability int) []interface{} {
	level := 1
	if population > 1000000 {
		level = 2
	}
	if population > 10000000 {
		level = 3
	}
	if !habitable {
		level = 1
	}

	capacity := population * 2
	if capacity < 1000 {
		capacity = 1000
	}

	return []interface{}{
		uuid.New().String(),
		planetID,
		level,
		population,
		capacity,
		stability,
	}
}

// ==================== ЗАВОДЫ ====================

// factoryRecipe — что производит завод по категории ресурса.
type factoryRecipe struct {
	factoryType string
	output      string
}

var factoryRecipes = map[string]factoryRecipe{
	resource.CategoryMineral: {"добывающий", "металл"},
	resource.CategoryFuel:    {"добывающий", "энергоноситель"},
	resource.CategoryOrganic: {"перерабатывающий", "еда"},
	resource.CategoryRare:    {"перерабатывающий", "компоненты"},
}

// buildFactories — строки заводов для категории ресурса.
// Количество заводов — 1 или 2 (случайно).
func buildFactories(planetID, category string, value float64, rng interface {
	Intn(int) int
}) [][]interface{} {
	recipe, ok := factoryRecipes[category]
	if !ok {
		return nil
	}

	count := 1 + rng.Intn(2)
	result := make([][]interface{}, 0, count)
	for i := 0; i < count; i++ {
		quality := 30 + rng.Intn(41)
		result = append(result, []interface{}{
			uuid.New().String(),
			planetID,
			fmt.Sprintf("%s завод %d", recipe.output, i+1),
			recipe.factoryType,
			category,
			recipe.output,
			quality,
			"active",
		})
	}
	return result
}

// ==================== ТОВАРЫ ====================

// buildGoods — строки партий товаров для вставки в БД.
// Всегда есть еда + 1–3 случайных товара.
func buildGoods(planetID string, rng interface {
	Intn(int) int
}) [][]interface{} {
	result := [][]interface{}{}

	// Еда всегда
	result = append(result, buildGoodsRow(
		planetID,
		"еда",
		100+rng.Intn(401),
		30+rng.Intn(41),
	))

	// Случайные товары
	possible := []string{"металл", "энергоноситель", "компоненты", "инструменты"}
	count := 1 + rng.Intn(3)
	for i := 0; i < count; i++ {
		product := possible[rng.Intn(len(possible))]
		result = append(result, buildGoodsRow(
			planetID,
			product,
			50+rng.Intn(201),
			30+rng.Intn(41),
		))
	}

	return result
}

// buildGoodsRow — одна строка партии товара.
func buildGoodsRow(planetID, product string, qty, quality int) []interface{} {
	return []interface{}{
		uuid.New().String(),
		planetID,
		product,
		qty,
		quality,
		nil, // producer_id = NULL (свободное производство)
		time.Now().Add(-24 * time.Hour),
	}
}

// ==================== ХЕЛПЕРЫ ====================

// isGasGiant — является ли планета газовым гигантом.
func isGasGiant(data map[string]interface{}) bool {
	if v, ok := data["is_gas_giant"].(bool); ok && v {
		return true
	}
	return getString(data, "surface_dominant") == "газовый_гигант"
}

// isRadioactive — помечена ли планета как радиоактивная.
func isRadioactive(data map[string]interface{}) bool {
	return getBool(data, "radioactive")
}

// extractResources — вытаскивает ресурсы из JSON планеты.
func extractResources(data map[string]interface{}) map[string]float64 {
	result := map[string]float64{}
	raw, ok := data["resources"].(map[string]interface{})
	if !ok {
		return result
	}
	for k, v := range raw {
		if f, ok := v.(float64); ok {
			result[k] = f
		}
	}
	return result
}

// interface гарантирует, что у rng есть метод Intn.
// Помогает не тащить *rand.Rand в сигнатуры, где нужен только Intn.
var _ = sql.ErrNoRows // защита от неиспользуемого импорта
