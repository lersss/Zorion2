// internal/goodsstudio/settlement_type_consistency_test.go
// Критерии T1/T18 спеки 2026-09-22-поселение-потребление-населением-итерация-4
// (§12): миграция создаёт ровно один дефолтный подтип (базовая ступень
// «Форпост») идемпотентно; числа норм `params.eat` в миграции, в Go-сиде и в
// константе `DefaultEatK` — ОДНО утверждённое число (расхождение = красный тест).
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

// thirstNormPerDayPerBillion — норма воды, «ед/сутки/млрд» (решение создателя
// О4/Р7: 20 000 000 = 20 т/сут на 1000 чел «экономного быта», спека
// 2026-09-24-потребление-по-товарам §6.2/§12.1).
const thirstNormPerDayPerBillion = 20000000

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

// settlementSeedSubtype — базовая ступень (пол ладдеры) из сида, по канон-
// константе settlementFloorStageName (имя ступени-пола — «Форпост»).
func settlementSeedSubtype(t *testing.T) seedProducer {
	t.Helper()
	var found *seedProducer
	for i := range seedProducers {
		if seedProducers[i].Name == settlementFloorStageName {
			found = &seedProducers[i]
		}
	}
	require.NotNil(t, found, "в сиде обязана быть ровно одна базовая ступень %q", settlementFloorStageName)
	return *found
}

// T1: миграция несёт связь «поселение → тип» (FK RESTRICT + индекс), снятие
// residual у родителя, идемпотентный INSERT подтипа и бэкфилл; сид — ровно
// один подтип под классом «Колония» без категории.
func TestSettlementTypeMigrationShape(t *testing.T) {
	mig := findSettlementTypeMigration(t)
	require.Contains(t, mig, "ALTER TABLE settlements ADD COLUMN settlement_type_id")
	require.Contains(t, mig, "REFERENCES producer_types (id) ON DELETE RESTRICT")
	require.Contains(t, mig, "CREATE INDEX idx_settlements_type")
	require.Contains(t, mig, "ON CONFLICT (name_norm) DO NOTHING", "повторный прогон идемпотентен (T1/T9)")
	require.Contains(t, mig, "WHERE settlement_type_id IS NULL", "бэкфилл существующих поселений (T2)")

	sub := settlementSeedSubtype(t)
	require.Equal(t, "Колония", sub.Parent)
	require.Equal(t, "goods", sub.Kind)
	require.Equal(t, "", sub.Category, "подтип типа без слотов — без категории (п.37)")
}

// T18: нормы согласованы между ИСТОРИЧЕСКИМИ литералами миграций (000067 —
// вода/пища, 000070 — продовольствие; их НЕ переписываем, §2.3), Go-константой
// DefaultEatK и сидом (сразу в новой единице + признак eat_units). Инвариант —
// «старый литерал × 2.4e10 = новое значение», спека 2026-09-23 §2.3/§15.1.
func TestSettlementTypeEatKConsistency(t *testing.T) {
	// 000067: `"вода": 2.5e-08` / `"пища": 2.5e-08` (JSON-структура params.eat).
	reJSON := regexp.MustCompile(`"(вода|пища)":\s*([0-9][0-9.eE+-]*)`)
	hist := map[string]float64{}
	for _, m := range reJSON.FindAllStringSubmatch(findSettlementTypeMigration(t), -1) {
		v, err := strconv.ParseFloat(m[2], 64)
		require.NoError(t, err)
		hist[m[1]] = v
	}
	// 000070: пилотная привязка `jsonb_build_object('продовольствие', 2.5e-08)`.
	reJSONB := regexp.MustCompile(`'продовольствие',\s*([0-9][0-9.eE+-]*)`)
	if m := reJSONB.FindStringSubmatch(findSupplyEffectsMigration(t)); m != nil {
		v, err := strconv.ParseFloat(m[1], 64)
		require.NoError(t, err)
		hist["продовольствие"] = v
	}
	for _, pos := range []string{"вода", "пища", "продовольствие"} {
		require.Contains(t, hist, pos, "исторический литерал нормы %q не найден в миграциях", pos)
		require.InDelta(t, settlement.DefaultEatK, hist[pos]*2.4e10, 1e-6,
			"исторический литерал %q × 2.4·10¹⁰ должен равняться DefaultEatK (=600)", pos)
	}

	// Сид: целевые params на ВСЕЙ ладдере (спека 2026-09-24-потребление-по-
	// товарам §6.2/§6.3): позиция — товар, единица — «ед/сутки/млрд» с
	// признаком eat_units (§2.3/§2.5). «пища» → норма DefaultEatK (600),
	// «очищенная вода» → норма жажды 20000000 (О4/Р7); мёртвые
	// ключи-категории «вода»/«продовольствие» вычищены (О6).
	sub := settlementSeedSubtype(t)
	var seedParams struct {
		Eat      map[string]float64 `json:"eat"`
		EatUnits string             `json:"eat_units"`
	}
	require.NoError(t, json.Unmarshal([]byte(sub.Params), &seedParams))
	require.Equal(t, "per_day_per_billion", seedParams.EatUnits, "сид несёт признак единицы (§2.5)")
	require.InDelta(t, settlement.DefaultEatK, seedParams.Eat["пища"], 1e-9,
		"сид: норма «пища» — сразу в новой единице (=600)")
	require.InDelta(t, thirstNormPerDayPerBillion, seedParams.Eat["очищенная вода"], 1e-9,
		"сид: норма воды — 20000000 ед/сутки/млрд (О4/Р7)")
	require.NotContains(t, seedParams.Eat, "вода", "мёртвый ключ-категория «вода» вычищен (О6)")
	require.NotContains(t, seedParams.Eat, "продовольствие", "категория «продовольствие» заменена товаром «пища» (§6.2)")
}

// T16/T18 (спека 2026-09-24-потребление-по-товарам §6.2/§6.3): нужда задаётся
// на КАЖДОЙ ступени ладдеры — иначе переход «Форпост» → «Поселение» снимает
// нужду воды (поселение «перестаёт пить»). Сид несёт одни и те же
// params.eat/effects/eat_units на всех семи ступенях; величина нормы от ступени
// не зависит (О4) — различается только наличие привязки.
func TestSettlementLadderSeedCarriesNeedParams(t *testing.T) {
	ladder := []string{"Форпост", "Поселение", "Городок", "Город", "Мегаполис", "Метрополия", "Экуменополис"}
	byName := map[string]seedProducer{}
	for _, p := range seedProducers {
		byName[p.Name] = p
	}
	for _, name := range ladder {
		p, ok := byName[name]
		require.True(t, ok, "в сиде нет ступени %q", name)
		var params struct {
			Eat      map[string]float64 `json:"eat"`
			Effects  map[string]string  `json:"effects"`
			EatUnits string             `json:"eat_units"`
		}
		require.NoError(t, json.Unmarshal([]byte(p.Params), &params), "ступень %q: params", name)
		require.Equal(t, "per_day_per_billion", params.EatUnits, "ступень %q: признак единицы", name)
		require.InDelta(t, settlement.DefaultEatK, params.Eat["пища"], 1e-9, "ступень %q: норма «пища»", name)
		require.InDelta(t, thirstNormPerDayPerBillion, params.Eat["очищенная вода"], 1e-9, "ступень %q: норма воды", name)
		require.Equal(t, "голод", params.Effects["пища"], "ступень %q: эффект «пища»", name)
		require.Equal(t, "жажда", params.Effects["очищенная вода"], "ступень %q: эффект воды", name)
	}
}
