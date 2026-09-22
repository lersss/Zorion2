// internal/goodsstudio/settlement_type_consistency_test.go
// Критерии T1/T18 спеки 2026-09-22-поселение-потребление-населением-итерация-4
// (§12): миграция создаёт ровно один дефолтный подтип «Обычное поселение»
// идемпотентно; числа норм `params.eat` в миграции, в Go-сиде и в константе
// `DefaultEatK` — ОДНО утверждённое число (расхождение = красный тест).
package goodsstudio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
)

// findSettlementTypeMigration — текст миграции `*_settlement_type.sql`
// (номер не фиксируем: файл ищется по суффиксу).
func findSettlementTypeMigration(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), "_settlement_type.sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		return string(b)
	}
	t.Fatal("миграция *_settlement_type.sql не найдена")
	return ""
}

// settlementSeedSubtype — подтип «Обычное поселение» из сида.
func settlementSeedSubtype(t *testing.T) seedProducer {
	t.Helper()
	var found *seedProducer
	for i := range seedProducers {
		if seedProducers[i].Name == "Обычное поселение" {
			found = &seedProducers[i]
		}
	}
	require.NotNil(t, found, "в сиде обязан быть ровно один подтип «Обычное поселение»")
	return *found
}

// T1: миграция несёт связь «поселение → тип» (FK RESTRICT + индекс), снятие
// residual у родителя, идемпотентный INSERT подтипа и бэкфилл; сид — ровно
// один подтип под «Поселением» без категории.
func TestSettlementTypeMigrationShape(t *testing.T) {
	mig := findSettlementTypeMigration(t)
	require.Contains(t, mig, "ALTER TABLE settlements ADD COLUMN settlement_type_id")
	require.Contains(t, mig, "REFERENCES producer_types (id) ON DELETE RESTRICT")
	require.Contains(t, mig, "CREATE INDEX idx_settlements_type")
	require.Contains(t, mig, "ON CONFLICT (name_norm) DO NOTHING", "повторный прогон идемпотентен (T1/T9)")
	require.Contains(t, mig, "WHERE settlement_type_id IS NULL", "бэкфилл существующих поселений (T2)")

	sub := settlementSeedSubtype(t)
	require.Equal(t, "Поселение", sub.Parent)
	require.Equal(t, "goods", sub.Kind)
	require.Equal(t, "", sub.Category, "подтип типа без слотов — без категории (п.37)")
}

// T18: значение нормы каждого товара («вода», «пища») совпадает во всех трёх
// местах — миграция, сид, DefaultEatK.
func TestSettlementTypeEatKConsistency(t *testing.T) {
	mig := findSettlementTypeMigration(t)

	// Числа из SQL-структуры params.eat миграции.
	re := regexp.MustCompile(`"(вода|пища)":\s*([0-9][0-9.eE+-]*)`)
	migK := map[string]float64{}
	for _, m := range re.FindAllStringSubmatch(mig, -1) {
		v, err := strconv.ParseFloat(m[2], 64)
		require.NoError(t, err)
		migK[m[1]] = v
	}
	require.Contains(t, migK, "вода")
	require.Contains(t, migK, "пища")

	// Числа из сида.
	sub := settlementSeedSubtype(t)
	var seedParams struct {
		Eat map[string]float64 `json:"eat"`
	}
	require.NoError(t, json.Unmarshal([]byte(sub.Params), &seedParams))
	require.Contains(t, seedParams.Eat, "вода")
	require.Contains(t, seedParams.Eat, "пища")

	for _, good := range []string{"вода", "пища"} {
		require.InDelta(t, settlement.DefaultEatK, migK[good], 1e-20,
			"миграция: норма %q должна равняться DefaultEatK", good)
		require.InDelta(t, settlement.DefaultEatK, seedParams.Eat[good], 1e-20,
			"сид: норма %q должна равняться DefaultEatK", good)
	}
}
