// internal/goodsstudio/supply_effects_consistency_test.go
// Критерии T1/T35/T36 спеки 2026-09-22-эффекты-снабжения-задержка-голод (§12) —
// в части этапа 1 (фундамент): миграция 000070 создаёт effect_types/
// active_effects и идемпотентно сеет тип «Голод» (params.curve='hunger', без
// recovery); пилотная привязка по ПОЗИЦИИ собирается слиянием `||` с
// объектом-обёрткой (`jsonb_build_object` + `COALESCE(params->…, '{}')`), НЕ
// jsonb_set — тот не создаёт отсутствующий промежуточный объект и молча
// теряет правку (finding 1); нормы итерации 4 («вода»/«пища») не затираются;
// Go-сид несёт ту же привязку для свежей БД (иначе пилот — no-op, §7.5).
package goodsstudio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
)

// findSupplyEffectsMigration — текст миграции `*_supply_effects.sql` (номер не
// фиксируем: файл ищется по суффиксу, T31).
func findSupplyEffectsMigration(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), "_supply_effects.sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		return string(b)
	}
	t.Fatal("миграция *_supply_effects.sql не найдена")
	return ""
}

// T1: миграция создаёт обе таблицы и идемпотентно сеет тип «Голод»;
// recovery в params НЕ хранится (скаляр Балансировки, §7.1/§7.4).
func TestSupplyEffectsMigrationShape(t *testing.T) {
	mig := findSupplyEffectsMigration(t)
	require.Contains(t, mig, "CREATE TABLE IF NOT EXISTS effect_types")
	require.Contains(t, mig, "CREATE TABLE IF NOT EXISTS active_effects")
	require.Contains(t, mig, "UNIQUE (owner_type, owner_id, effect_type_id)")
	require.Contains(t, mig, "REFERENCES effect_types (id) ON DELETE RESTRICT")
	require.Contains(t, mig, "REFERENCES settlements (id) ON DELETE CASCADE")
	// активные эффекты несут нагрузку и её базис, источник — позиция (не ветка/товар).
	require.Contains(t, mig, "load")
	require.Contains(t, mig, "load_at")
	require.Contains(t, mig, "source_position")

	// Сид «Голод»: params.curve='hunger', идемпотентно, без recovery в params.
	require.Contains(t, mig, `'Голод', 'голод', 'population_rate'`)
	require.Contains(t, mig, `{"curve": "hunger"}`)
	require.Contains(t, mig, "ON CONFLICT (name_norm) DO NOTHING")
	require.NotContains(t, mig, `"recovery":`, "recovery — скаляр компоненты Балансировки, НЕ в effect_types.params (§7.1)")
}

// jsonbSetNested — модель PostgreSQL `jsonb_set(doc, '{parent,key}', v, true)`:
// правится ТОЛЬКО уже существующий промежуточный объект `parent`; если его нет
// — молчаливый no-op (create_missing создаёт конечный ключ, но не его родителя).
func jsonbSetNested(doc map[string]interface{}, parent, key string, value interface{}) map[string]interface{} {
	mid, ok := doc[parent].(map[string]interface{})
	if !ok {
		return doc
	}
	out := make(map[string]interface{}, len(mid)+1)
	for k, v := range mid {
		out[k] = v
	}
	out[key] = value
	doc[parent] = out
	return doc
}

// mergeNestedKey — модель рабочей сборки миграции 000070:
// `doc || jsonb_build_object(parent, COALESCE(doc->parent, '{}') || jsonb_build_object(key, value))`.
// Отсутствующий промежуточный объект создаётся, чужие ключи внутри сохраняются.
func mergeNestedKey(doc map[string]interface{}, parent, key string, value interface{}) map[string]interface{} {
	mid, _ := doc[parent].(map[string]interface{})
	out := make(map[string]interface{}, len(mid)+1)
	for k, v := range mid {
		out[k] = v
	}
	out[key] = value
	doc[parent] = out
	return doc
}

// mergeWholeObject — модель запрещённого варианта `doc || '{"eat": {…}}'`:
// объект `parent` перезаписывается целиком, чужие ключи внутри теряются.
func mergeWholeObject(doc map[string]interface{}, parent string, obj map[string]interface{}) map[string]interface{} {
	doc[parent] = obj
	return doc
}

// stripSQLLineComments — SQL без строк-комментариев (`-- …`): проверяем
// исполняемый текст, а не упоминания jsonb_set в пояснениях к миграции.
func stripSQLLineComments(sqlText string) string {
	var b strings.Builder
	for _, line := range strings.Split(sqlText, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// T35: пилотная привязка — сборка вложенных ключей слиянием `||` с
// объектом-обёрткой (миграция НЕ использует jsonb_set: он не создаёт
// отсутствующий промежуточный объект — молчаливый no-op, finding 1);
// нормы итерации 4 («вода»/«пища») не перезаписываются объектом eat целиком.
func TestSupplyEffectsMigrationPilotBindingByKey(t *testing.T) {
	// (1) Суть ловушки, воспроизведённая на Go (без живого PostgreSQL):
	// строка в миграции — не доказательство, доказывает поведение.
	pilot := func() map[string]interface{} {
		return map[string]interface{}{
			"eat": map[string]interface{}{"вода": 2.5e-08, "пища": 2.5e-08},
		}
	}

	// jsonb_set по пути {effects,продовольствие}: объекта `effects` нет —
	// правка МОЛЧА теряется, голод не активируется (это и был баг 000070).
	broken := jsonbSetNested(pilot(), "effects", "продовольствие", "голод")
	require.NotContains(t, broken, "effects",
		"jsonb_set не создаёт отсутствующий промежуточный объект — правка молча теряется")

	// Слияние с объектом-обёрткой создаёт `effects.продовольствие`.
	ok := mergeNestedKey(pilot(), "effects", "продовольствие", "голод")
	require.Equal(t, "голод", ok["effects"].(map[string]interface{})["продовольствие"])

	// Поуровневое слияние второй привязки (eat.продовольствие) сохраняет нормы
	// итерации 4 — «вода»/«пища» не теряются.
	both := mergeNestedKey(ok, "eat", "продовольствие", 2.5e-08)
	eat := both["eat"].(map[string]interface{})
	require.Contains(t, eat, "вода")
	require.Contains(t, eat, "пища")

	// Запрещённый вариант `params || '{"eat": {…}}'` затирает объект eat целиком.
	wiped := mergeWholeObject(pilot(), "eat", map[string]interface{}{"продовольствие": 2.5e-08})
	require.NotContains(t, wiped["eat"].(map[string]interface{}), "вода",
		"слияние объекта целиком затирает нормы «вода»/«пища» — запрещено (finding 1)")

	// (2) Исполняемый SQL миграции применяет именно рабочую сборку.
	sql := stripSQLLineComments(findSupplyEffectsMigration(t))
	require.NotContains(t, sql, "jsonb_set",
		"пилотная привязка не может идти jsonb_set — он не создаёт промежуточный объект `effects`")
	require.Contains(t, sql, "jsonb_build_object",
		"вложенные привязки собираются объектом-обёрткой")
	require.Contains(t, sql, "COALESCE(params->'effects'",
		"чужие ключи внутри `effects` сохраняются слиянием с COALESCE(params->'effects')")
	require.Contains(t, sql, "COALESCE(params->'eat'",
		"нормы итерации 4 («вода»/«пища») сохраняются слиянием с COALESCE(params->'eat')")
	require.NotContains(t, sql, `'{"eat"`,
		"вариант params || '{\"eat\": {…}}' затирает весь объект eat — запрещён (finding 1)")
	require.Contains(t, sql, `WHERE name_norm = 'обычное поселение'`, "пилотная привязка — к дефолтному типу поселения, не глобально")
}

// T36 (предусловие пилота): Go-сид несёт ту же привязку для свежей БД —
// params.effects.продовольствие='голод' и params.eat.продовольствие=DefaultEatK;
// нормы «вода»/«пища» сохранены (согласовано с миграцией 000067).
func TestSettlementSeedCarriesPilotBinding(t *testing.T) {
	sub := settlementSeedSubtype(t)
	var p struct {
		Eat     map[string]float64 `json:"eat"`
		Effects map[string]string  `json:"effects"`
	}
	require.NoError(t, json.Unmarshal([]byte(sub.Params), &p))
	require.Equal(t, "голод", p.Effects["продовольствие"], "сид свежей БД несёт пилотную привязку по позиции (§7.5)")
	require.InDelta(t, settlement.DefaultEatK, p.Eat["продовольствие"], 1e-20)
	// Нормы итерации 4 не потеряны.
	require.Contains(t, p.Eat, "вода")
	require.Contains(t, p.Eat, "пища")
}
