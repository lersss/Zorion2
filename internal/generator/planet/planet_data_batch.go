// internal/generator/planet/planet_data_batch.go
package planet

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// ==================== БАТЧ-ВСТАВКА ЧЕРЕЗ pq.CopyIn ====================
//
// Раньше был INSERT INTO ... VALUES (...), (...), ... — это медленно:
//   - Строка SQL длиной 100+ КБ, Postgres парсит её каждый раз.
//   - Лимит параметров Postgres — 65535, что ограничивает размер батча.
//   - buildPlaceholders + shiftPlaceholders жгут CPU на каждый батч.
//
// Теперь pq.CopyIn — это настоящий COPY FROM STDIN. В 5–10 раз быстрее,
// без лимита параметров, без парсинга гигантской строки.
//
// API: tx.Prepare(pq.CopyIn("table", "col1", ...)) → stmt.Exec(row...) (на каждую строку)
// → stmt.Exec() (flush) → stmt.Close(). Всё в одной транзакции.

// copyInPlanets — вставляет пачку планет через COPY.
// rowWidth = 7. Длина rows должна делиться на 7.
func (g *Generator) copyInPlanets(tx *sql.Tx, rows []interface{}) error {
	return copyInRows(tx, "planets",
		[]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"},
		rows, 7)
}

// copyInSettlements — вставляет пачку поселений через COPY. rowWidth = 6.
func (g *Generator) copyInSettlements(tx *sql.Tx, rows []interface{}) error {
	return copyInRows(tx, "settlements",
		[]string{"id", "planet_id", "level", "population", "capacity", "stability"},
		rows, 6)
}

// copyInFactories — вставляет пачку заводов через COPY. rowWidth = 8.
func (g *Generator) copyInFactories(tx *sql.Tx, rows []interface{}) error {
	return copyInRows(tx, "factories",
		[]string{"id", "planet_id", "name", "type", "input_resource", "output_product", "quality", "status"},
		rows, 8)
}

// copyInGoods — вставляет пачку партий товаров через COPY. rowWidth = 7.
func (g *Generator) copyInGoods(tx *sql.Tx, rows []interface{}) error {
	return copyInRows(tx, "goods_batches",
		[]string{"id", "planet_id", "product_name", "quantity", "quality", "producer_id", "produced_at"},
		rows, 7)
}

// ==================== ОБЩАЯ ФУНКЦИЯ ====================

// copyInRows — общая реализация для всех таблиц.
// rows — плоский срез значений, длина кратна rowWidth.
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