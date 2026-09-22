// internal/generator/settlement/generator.go
package settlement

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"zorion/internal/repository"
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

// GenerateSettlements — создаёт поселения на планетах, проходящих модель
// генерации. Возвращает число поселений.
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

	var settlementRows []interface{}
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

		// Поселение
		settlementRows = append(settlementRows,
			buildSettlement(id, model.Population.value(g.rng), g.rng.Intn(41)+40))
		settled++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if settled == 0 {
		return 0, nil
	}

	// Тип поселения — настоящая связь (спека итерации 4 §3.4): дефолтный подтип
	// «Обычное поселение» резолвится один раз на джоб и дописывается в каждую
	// строку (типа нет → NULL, чтение применит фолбэк DefaultEatK).
	typeID, err := repository.ResolveDefaultSettlementTypeID(g.db)
	if err != nil {
		return 0, fmt.Errorf("resolve settlement type: %w", err)
	}
	for i := range settlementRows {
		settlementRows[i] = append(settlementRows[i].([]interface{}), nullableTypeID(typeID))
	}

	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if err := copyInRows(tx, "settlements",
		[]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "settlement_type_id"},
		flatten(settlementRows), 7); err != nil {
		return 0, fmt.Errorf("copy settlements: %w", err)
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
// генерации (w-сброса нет — R-модель, 99.2.12).
// settlement_type_id дописывается вызывающим один раз на джоб (спека итерации 4
// §3.4) — здесь строка из 6 полей.
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

// nullableTypeID — id типа поселения в параметр вставки: 0 (типа нет) → NULL
// (колонка nullable, спека итерации 4 §3.1), иначе число.
func nullableTypeID(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}

// ==================== ХЕЛПЕРЫ JSON ====================

// isGasGiant — является ли планета газовым гигантом.
func isGasGiant(data map[string]interface{}) bool {
	if v, ok := data["is_gas_giant"].(bool); ok && v {
		return true
	}
	return getString(data, "surface_dominant") == "газовый_гигант"
}

func getString(data map[string]interface{}, key string) string {
	v, _ := valueAtPath(data, key)
	s, _ := v.(string)
	return s
}

func getFloat(data map[string]interface{}, key string) float64 {
	v, _ := valueAtPath(data, key)
	f, _ := v.(float64)
	return f
}

func getBool(data map[string]interface{}, key string) bool {
	v, _ := valueAtPath(data, key)
	b, _ := v.(bool)
	return b
}

// valueAtPath — значение по dot-ключу ("core.radioactivity" → data["core"]
// ["radioactivity"]). Плоские ключи читаются как раньше; отсутствующий
// промежуточный map даёт отсутствующее значение.
func valueAtPath(data map[string]interface{}, key string) (interface{}, bool) {
	i := strings.IndexByte(key, '.')
	if i < 0 {
		v, ok := data[key]
		return v, ok
	}
	head, tail := key[:i], key[i+1:]
	if child, ok := data[head].(map[string]interface{}); ok {
		return valueAtPath(child, tail)
	}
	return nil, false
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