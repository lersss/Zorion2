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

// copyInBelts — вставляет пачку поясов малых тел через COPY (спека поясов
// §4.7). rowWidth = 14. system_belts.world_id — FK на worlds; composition/
// data — JSONB, поэтому в addBelt они уже приведены []byte → string
// (ловушка lib/pq). Длина rows должна делиться на 14.
func (g *Generator) copyInBelts(tx *sql.Tx, rows []interface{}) error {
	return copyInRows(tx, "system_belts",
		[]string{
			"id", "world_id", "kind", "name", "orbit_index",
			"radius_au", "width_au", "mass", "body_size_km",
			"composition", "visible", "data", "created_at", "updated_at",
		},
		rows, 14)
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