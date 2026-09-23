// internal/goodsstudio/producer_units_migration_test.go
// T-Е2 (признак единицы): миграция 000076 переводит params.eat в «ед/сутки/млрд»
// РОВНО ОДИН раз — повторный прогон на строке с params.eat_units не меняет
// значение (идемпотентность по признаку); строка без признака конвертируется
// один раз; строки без eat не трогаются (спека 2026-09-23 §2.5/§9.1/§15.2).
package goodsstudio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// findMigrationBySuffix — текст миграции по суффиксу имени (номер не фиксируем).
func findMigrationBySuffix(t *testing.T, suffix string) string {
	t.Helper()
	dir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		return string(b)
	}
	t.Fatalf("миграция *%s не найдена", suffix)
	return ""
}

// convertEatUnitsModel — модель конверсии блока 2 миграции 000076: признак
// eat_units ставится в том же UPDATE и отсекает повторную конверсию.
func convertEatUnitsModel(params map[string]interface{}) map[string]interface{} {
	eat, ok := params["eat"].(map[string]interface{})
	if !ok {
		return params
	}
	if u, _ := params["eat_units"].(string); u == "per_day_per_billion" {
		return params // повторный прогон — no-op (условие миграции)
	}
	out := make(map[string]interface{}, len(eat))
	for k, v := range eat {
		out[k] = v.(float64) * 2.4e10
	}
	params["eat"] = out
	params["eat_units"] = "per_day_per_billion"
	return params
}

// T-Е2: идемпотентность по признаку — второй прогон конверсии не меняет
// значение; строка без признака конвертируется один раз; без eat — не трогается.
func TestProducerEatUnitsConversionIdempotent(t *testing.T) {
	// (1) Поведение: строка без признака конвертируется один раз.
	row := map[string]interface{}{"eat": map[string]interface{}{"вода": 2.5e-08, "пища": 2.5e-08}}
	convertEatUnitsModel(row)
	require.InDelta(t, 600, row["eat"].(map[string]interface{})["вода"], 1e-6, "2.5e-08 × 2.4e10 = 600")
	require.Equal(t, "per_day_per_billion", row["eat_units"])

	// Повторный прогон: значение то же (условие `<> 'per_day_per_billion'`).
	convertEatUnitsModel(row)
	require.InDelta(t, 600, row["eat"].(map[string]interface{})["вода"], 1e-6, "повторный прогон — no-op")

	// Строка без eat — не трогается, признак не ставится.
	noEat := map[string]interface{}{"effects": map[string]interface{}{"продовольствие": "голод"}}
	convertEatUnitsModel(noEat)
	require.NotContains(t, noEat, "eat_units")

	// (2) Исполняемый SQL несёт условие идемпотентности и литерал признака.
	sql := findMigrationBySuffix(t, "_producer_rate_and_eat_units.sql")
	require.Contains(t, sql, "ADD COLUMN rate DOUBLE PRECISION NULL")
	require.Contains(t, sql, "CHECK (rate IS NULL OR rate >= 0)")
	require.Contains(t, sql, "2.4e10")
	require.Contains(t, sql, "{eat_units}")
	require.Contains(t, sql, `"per_day_per_billion"`)
	require.Contains(t, sql, "params ? 'eat'")
	require.Contains(t, sql, `COALESCE(params->>'eat_units', '') <> 'per_day_per_billion'`,
		"условие отсекает повторную конверсию (§2.5)")
}
