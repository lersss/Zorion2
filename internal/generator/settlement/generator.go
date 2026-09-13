// internal/generator/settlement/generator.go
package settlement

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"zorion/internal/resource"
)

// Generator — генератор поселений. Проходит по таблице planets и создаёт
// поселения на планетах, подходящих под модель генерации.
type Generator struct {
	db  *sql.DB
	rng *rand.Rand
}

// NewGenerator — создаёт генератор. Если seed = 0 — берётся time.Now().
func NewGenerator(db *sql.DB, seed int64) *Generator {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &Generator{
		db:  db,
		rng: rand.New(rand.NewSource(seed)),
	}
}

// GenerateSettlements — создаёт поселения (и заводы с товарами) на планетах,
// проходящих модель генерации. Возвращает число поселений.
//
// progressFn вызывается после каждой планеты (для статус-бара). Может быть nil.
func (g *Generator) GenerateSettlements(ctx context.Context, model *Model, progressFn func(processed int)) (int, error) {
	if err := model.Validate(); err != nil {
		return 0, err
	}

	rows, err := g.db.QueryContext(ctx, `SELECT id, data FROM planets`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var settlementRows, factoryRows, goodsRows []interface{}
	settled := 0
	processed := 0

	for rows.Next() {
		var id string
		var dataJSON []byte
		if err := rows.Scan(&id, &dataJSON); err != nil {
			return 0, err
		}
		processed++
		if progressFn != nil {
			progressFn(processed)
		}

		var data map[string]interface{}
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			continue
		}

		if !model.Matches(data) {
			continue
		}
		if g.rng.Float64() >= model.Chance {
			continue
		}

		// 1. Поселение
		settlementRows = append(settlementRows,
			buildSettlement(id, model.Population.value(g.rng), g.rng.Intn(41)+40))

		// 2. Заводы (по ресурсам)
		for category, value := range extractResources(data) {
			if value <= 0.3 {
				continue
			}
			for _, f := range buildFactories(id, category, value, g.rng) {
				factoryRows = append(factoryRows, f)
			}
		}

		// 3. Товары
		for _, gd := range buildGoods(id, g.rng) {
			goodsRows = append(goodsRows, gd)
		}
		settled++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if settled == 0 {
		return 0, nil
	}

	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if err := copyInRows(tx, "settlements",
		[]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at"},
		flatten(settlementRows), 6); err != nil {
		return 0, fmt.Errorf("copy settlements: %w", err)
	}
	if err := copyInRows(tx, "factories",
		[]string{"id", "planet_id", "name", "type", "input_resource", "output_product", "quality", "status"},
		flatten(factoryRows), 8); err != nil {
		return 0, fmt.Errorf("copy factories: %w", err)
	}
	if err := copyInRows(tx, "goods_batches",
		[]string{"id", "planet_id", "product_name", "quantity", "quality", "producer_id", "produced_at"},
		flatten(goodsRows), 7); err != nil {
		return 0, fmt.Errorf("copy goods: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return settled, nil
}

// ==================== ПОСЕЛЕНИЯ ====================

// buildSettlement — строка поселения для вставки в БД.
// Тир (level) — расчётная величина (13_tiers.md), при генерации не задаётся.
// population_exact и computed_at — точное состояние для пересчёта смерти от
// среды (18a_population_death.md), стартует равным population на момент
// генерации.
func buildSettlement(planetID string, population, stability int) []interface{} {
	return []interface{}{
		uuid.New().String(),
		planetID,
		population,
		float64(population),
		stability,
		time.Now(),
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

// ==================== ХЕЛПЕРЫ JSON ====================

// isGasGiant — является ли планета газовым гигантом.
func isGasGiant(data map[string]interface{}) bool {
	if v, ok := data["is_gas_giant"].(bool); ok && v {
		return true
	}
	return getString(data, "surface_dominant") == "газовый_гигант"
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

func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

func getFloat(data map[string]interface{}, key string) float64 {
	if v, ok := data[key].(float64); ok {
		return v
	}
	return 0
}

func getBool(data map[string]interface{}, key string) bool {
	if v, ok := data[key].(bool); ok {
		return v
	}
	return false
}

// ==================== БАТЧ-ВСТАВКА ====================

// flatten — превращает [][]interface{} в плоский []interface{}.
func flatten(rows []interface{}) []interface{} {
	total := 0
	for _, r := range rows {
		if slice, ok := r.([]interface{}); ok {
			total += len(slice)
		}
	}
	out := make([]interface{}, 0, total)
	for _, r := range rows {
		if slice, ok := r.([]interface{}); ok {
			out = append(out, slice...)
		}
	}
	return out
}

// copyInRows — батч-вставка через pq.CopyIn (COPY FROM STDIN).
func copyInRows(tx *sql.Tx, table string, cols []string, rows []interface{}, rowWidth int) error {
	if len(rows) == 0 {
		return nil
	}
	if len(rows)%rowWidth != 0 {
		return fmt.Errorf("copyInRows: len(rows)=%d не кратно rowWidth=%d", len(rows), rowWidth)
	}

	stmt, err := tx.Prepare(pq.CopyIn(table, cols...))
	if err != nil {
		return fmt.Errorf("prepare copy %s: %w", table, err)
	}

	for i := 0; i < len(rows); i += rowWidth {
		chunk := make([]interface{}, rowWidth)
		copy(chunk, rows[i:i+rowWidth])
		if _, err := stmt.Exec(chunk...); err != nil {
			stmt.Close()
			return fmt.Errorf("copy %s row %d: %w", table, i/rowWidth, err)
		}
	}

	// Финальный Exec без аргументов — flush.
	if _, err := stmt.Exec(); err != nil {
		stmt.Close()
		return fmt.Errorf("copy %s flush: %w", table, err)
	}

	return stmt.Close()
}