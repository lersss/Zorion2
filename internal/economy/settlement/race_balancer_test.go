// Тесты расовых R-кривых (спека 99.2.23 §14): генерация из карточки (§3.3),
// авто-инициализация при старте (§4.4), перегенерация/возврат заводских
// (§2.4.4), устаревание по card_hash, round-trip файла, спец-случай humans.
package settlement

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/races"
)

// resetRaceBalancerStore — чистое состояние store для теста (пакетный
// синглтон; тесты изолированы друг от друга).
func resetRaceBalancerStore() {
	raceBalancerCurveStore.mu.Lock()
	raceBalancerCurveStore.records = map[string]*RaceRecord{}
	raceBalancerCurveStore.path = ""
	raceBalancerCurveStore.mu.Unlock()
}

// raceCurveAt — значение кривой компоненты расы в точке X (для проверок).
func raceCurveAt(rc *RaceCurves, component string, x float64) float64 {
	c := rc.Curves[component]
	return evaluateCurve(c.Nodes, c.Bends, x)
}

// Тест 2 (§14): генерация из карточки — детерминизм и ожидания аммиачников.
// Аммиачники: opt [200, 235] K, surv [196, 255] K, reproduction 0.05,
// resilience 65 → HeatStretch = 20/60 = 0.333, Y × 50/65 = 0.769.
func TestDeriveRaceCurvesAmmonia(t *testing.T) {
	ammonia := races.ByID("ammonia")
	require.NotNil(t, ammonia)

	rc1, err := deriveRaceCurves(ammonia)
	require.NoError(t, err)
	rc2, err := deriveRaceCurves(ammonia)
	require.NoError(t, err)

	// Детерминизм: два вызова из одной карточки и одних человеческих кривых —
	// одинаковый результат.
	require.True(t, RaceCurvesEqual(&rc1, &rc2), "два вызова deriveRaceCurves должны давать одинаковый результат")

	require.Equal(t, 0.05, rc1.Reproduction, "reproduction из карточки")

	// Жара 0 при T ≤ opt_hi (235 K): первый узел → opt_hi − 273.15 °C.
	heat := rc1.Curves["heat"]
	require.InDelta(t, 235-273.15, heat.Nodes[0].X, 1e-9, "первый узел жары → opt_hi − 273.15 °C")
	require.Equal(t, 0.0, heat.Nodes[0].Y, "жара 0 при T ≤ opt_hi")
	require.InDelta(t, 0.0, raceCurveAt(&rc1, "heat", 235-273.15), 1e-12, "жара 0 при 235 K")
	require.InDelta(t, 0.0, raceCurveAt(&rc1, "heat", 215-273.15), 1e-12, "жара 0 при 215 K (центр opt)")

	// HeatStretch = (255−235)/60 = 0.333: узел 90 °C (сильная зона) →
	// −38.15 + 60·0.333 = −18.15 °C = 255 K (край surv).
	require.InDelta(t, 255-273.15, heat.Nodes[5].X, 1e-9, "узел 90 °C → край surv 255 K")

	// Холод 0 при T ≥ opt_lo (200 K): правый узел → opt_lo − 273.15 °C.
	cold := rc1.Curves["cold"]
	last := cold.Nodes[len(cold.Nodes)-1]
	require.InDelta(t, 200-273.15, last.X, 1e-9, "правый узел холода → opt_lo − 273.15 °C")
	require.Equal(t, 0.0, last.Y, "холод 0 при T ≥ opt_lo")
	require.InDelta(t, 0.0, raceCurveAt(&rc1, "cold", 200-273.15), 1e-12, "холод 0 при 200 K")

	// Масштаб Y: 50/65 = 0.769 (узел 90 °C жары: 3.9068e-5 × 0.769 ≈ 3.0e-5).
	require.InDelta(t, 3.9068e-5*50.0/65.0, heat.Nodes[5].Y, 1e-7, "Y × ResilienceScale(65) = 0.769")
}

// Тест 3 (§14): сдвиги — термо-рои, крио-небесные, радиотолерантные;
// пиннинг холодного левого узла на 0 K; узлы строго возрастают.
func TestDeriveRaceCurvesShifts(t *testing.T) {
	// Термо-рои: opt [500, 700] K, surv [480, 780] K.
	thermo := races.ByID("thermo_swarms")
	require.NotNil(t, thermo)
	rc, err := deriveRaceCurves(thermo)
	require.NoError(t, err)
	require.InDelta(t, 0.0, raceCurveAt(&rc, "heat", 700-273.15), 1e-12, "жара 0 при ≤ 700 K")
	require.InDelta(t, 0.0, raceCurveAt(&rc, "cold", 500-273.15), 1e-12, "холод 0 при ≥ 500 K")
	require.InDelta(t, 0.0, raceCurveAt(&rc, "heat", 600-273.15), 1e-12, "жара 0 в центре opt")

	// Крио-небесные (48): opt g [1, 5] → гравитация 0 в [1, 5], ветки вне.
	cryo := races.ByID("cryo_sky")
	require.NotNil(t, cryo)
	rc, err = deriveRaceCurves(cryo)
	require.NoError(t, err)
	require.InDelta(t, 0.0, raceCurveAt(&rc, "gravity", 1.0), 1e-12, "гравитация 0 при 1 g (opt_lo)")
	require.InDelta(t, 0.0, raceCurveAt(&rc, "gravity", 3.0), 1e-12, "гравитация 0 при 3 g (центр opt)")
	require.InDelta(t, 0.0, raceCurveAt(&rc, "gravity", 5.0), 1e-12, "гравитация 0 при 5 g (opt_hi)")
	require.Greater(t, raceCurveAt(&rc, "gravity", 0.5), 0.0, "ветка вне комфорта (ниже opt_lo) > 0")
	require.Greater(t, raceCurveAt(&rc, "gravity", 8.0), 0.0, "ветка вне комфорта (выше opt_hi) > 0")

	// Радиотолерантные (30): opt rad [0, 90] → радиация 0 при rad ≤ 90.
	radio := races.ByID("radiotolerant")
	require.NotNil(t, radio)
	rc, err = deriveRaceCurves(radio)
	require.NoError(t, err)
	require.InDelta(t, 0.0, raceCurveAt(&rc, "radiation", 0), 1e-12, "радиация 0 при 0 rad")
	require.InDelta(t, 0.0, raceCurveAt(&rc, "radiation", 90), 1e-12, "радиация 0 при 90 rad (opt_hi)")
	require.Greater(t, raceCurveAt(&rc, "radiation", 95), 0.0, "радиация > 0 за opt_hi")

	// Пиннинг холодного левого узла на −273.15 °C + строгое возрастание X
	// (валидация кривой не ломается) — для всех рас каталога.
	for _, race := range races.Catalog() {
		if race.ID == "humans" {
			continue
		}
		rc, err := deriveRaceCurves(race)
		require.NoError(t, err, "раса %s", race.ID)
		for _, comp := range raceComponents {
			c := rc.Curves[comp]
			require.NoError(t, validateRaceCurve(*c), "раса %s, кривая %s", race.ID, comp)
			if comp == "cold" {
				require.GreaterOrEqual(t, c.Nodes[0].X, -273.15, "раса %s: левый узел холода ≥ 0 K", race.ID)
			}
		}
	}
}

// Тест 4 (§14): масштаб ResilienceScale = 50/resilience; Y клампится в [0, 0.999).
func TestResilienceScale(t *testing.T) {
	require.InDelta(t, 50.0/65.0, 50.0/65.0, 1e-9, "ResilienceScale(65) = 0.769")
	require.InDelta(t, 1.0, 50.0/50.0, 1e-9, "ResilienceScale(50) = 1.0")
	require.InDelta(t, 50.0/35.0, 50.0/35.0, 1e-9, "ResilienceScale(35) = 1.429")

	// Кламп Y ≤ 0.999: человеческий максимум 0.98 × 1.43 = 1.40 → 0.999.
	// Термо-рои (resilience 35): узел 4000 °C жары (0.9799) × 1.429 = 1.40.
	thermo := races.ByID("thermo_swarms")
	rc, err := deriveRaceCurves(thermo)
	require.NoError(t, err)
	heat := rc.Curves["heat"]
	require.Less(t, heat.Nodes[len(heat.Nodes)-1].Y, 0.999, "Y клампится в [0, 0.999)")
	require.GreaterOrEqual(t, heat.Nodes[len(heat.Nodes)-1].Y, 0.0)
}

// Тест 5 (§14): round-trip — запись store → файл → загрузка → идентичные
// узлы/bends/reproduction (порядок полей канонический).
func TestRaceBalancerRoundTrip(t *testing.T) {
	resetRaceBalancerStore()
	defer resetRaceBalancerStore()

	dir := t.TempDir()
	path := filepath.Join(dir, "race_balancer.json")

	require.NoError(t, LoadRaceBalancer(path))
	before, ok := GetRaceRecordMeta("ammonia")
	require.True(t, ok, "аммиачники инициализированы при старте")

	// Имитация рестарта: новый LoadRaceBalancer на том же файле.
	resetRaceBalancerStore()
	require.NoError(t, LoadRaceBalancer(path))
	after, ok := GetRaceRecordMeta("ammonia")
	require.True(t, ok)

	require.Equal(t, before.CardHash, after.CardHash)
	require.True(t, RaceCurvesEqual(&before.Factory, &after.Factory), "factory идентичен после round-trip")
	require.True(t, RaceCurvesEqual(&before.Active, &after.Active), "active идентичен после round-trip")
	require.Equal(t, before.RaceID, after.RaceID)
}

// Тест 6 (§14): авто-инициализация при старте — раса без записи получает
// factory/active из карточки, card_hash = текущий, файл записан; раса с
// записью не трогается (даже при устаревшем card_hash); битый JSON → лог,
// store пуст, сервер не падает.
func TestRaceBalancerAutoInit(t *testing.T) {
	resetRaceBalancerStore()
	defer resetRaceBalancerStore()

	dir := t.TempDir()
	path := filepath.Join(dir, "race_balancer.json")

	// Файла нет → все расы инициализируются из карточек, файл создан.
	require.NoError(t, LoadRaceBalancer(path))
	_, err := os.Stat(path)
	require.NoError(t, err, "файл создан при авто-инициализации")

	rec, ok := GetRaceRecordMeta("ammonia")
	require.True(t, ok)
	require.Equal(t, CardHash(races.ByID("ammonia")), rec.CardHash, "card_hash = текущий хэш карточки")
	require.True(t, RaceCurvesEqual(&rec.Active, &rec.Factory), "active = factory сразу после генерации")

	// Раса с записью не трогается: правим active вручную, перезагружаем —
	// правка сохраняется (не пересчитывается при старте).
	require.NoError(t, SetRaceReproduction("ammonia", 0.123))
	require.NoError(t, LoadRaceBalancer(path))
	rec2, _ := GetRaceRecordMeta("ammonia")
	require.Equal(t, 0.123, rec2.Active.Reproduction, "запись есть → не пересчитывается при старте")

	// Битый JSON → лог, store пуст, сервер не падает; расы инициализируются
	// из карточек (файл пересоздаётся при первой записи).
	resetRaceBalancerStore()
	require.NoError(t, os.WriteFile(path, []byte("{не-json"), 0o644))
	require.NoError(t, LoadRaceBalancer(path), "битый JSON не роняет старт")
	_, ok = GetRaceRecordMeta("ammonia")
	require.True(t, ok, "расы инициализируются из карточек после битого JSON")
}

// Тест 7 (§14): перегенерация — factory обновлён из карточки, active НЕ
// тронут (приоритет ручной правки), card_hash обновлён; reset-factory →
// active = factory.
func TestRaceBalancerRegenerate(t *testing.T) {
	resetRaceBalancerStore()
	defer resetRaceBalancerStore()

	require.NoError(t, LoadRaceBalancer(filepath.Join(t.TempDir(), "race_balancer.json")))

	// Ручная правка active (кривая жары + reproduction).
	rec, _ := GetRaceRecordMeta("ammonia")
	heat := rec.Active.Curves["heat"]
	edited := *heat
	edited.Nodes = append([]SegmentNode(nil), heat.Nodes...)
	edited.Nodes[0].Y = 0.5 // правка узла
	require.NoError(t, SetRaceCurve("ammonia", "heat", edited))
	require.NoError(t, SetRaceReproduction("ammonia", 0.077))

	// Перегенерация factory из карточки.
	require.NoError(t, RegenerateRaceFactory("ammonia"))
	rec2, _ := GetRaceRecordMeta("ammonia")
	require.Equal(t, CardHash(races.ByID("ammonia")), rec2.CardHash, "card_hash обновлён")
	require.Equal(t, 0.05, rec2.Factory.Reproduction, "factory из карточки")
	require.Equal(t, 0.077, rec2.Active.Reproduction, "active НЕ тронут (приоритет ручной правки)")
	require.Equal(t, 0.5, rec2.Active.Curves["heat"].Nodes[0].Y, "active-кривая НЕ тронута")
	require.NotEqual(t, 0.5, rec2.Factory.Curves["heat"].Nodes[0].Y, "factory пересчитан из карточки")

	// Возврат заводских: active = factory.
	require.NoError(t, ResetRaceToFactory("ammonia"))
	rec3, _ := GetRaceRecordMeta("ammonia")
	require.True(t, RaceCurvesEqual(&rec3.Active, &rec3.Factory), "reset-factory → active = factory")
	require.Equal(t, 0.05, rec3.Active.Reproduction)
}

// Тест 8 (§14): устаревание — card_hash ≠ хэш текущей карточки → флаг
// «устарело» в status; после generate → флаг снят.
func TestRaceBalancerStale(t *testing.T) {
	resetRaceBalancerStore()
	defer resetRaceBalancerStore()

	require.NoError(t, LoadRaceBalancer(filepath.Join(t.TempDir(), "race_balancer.json")))

	// Имитация правки карточки: подменяем card_hash на заведомо другой.
	raceBalancerCurveStore.mu.Lock()
	rec := raceBalancerCurveStore.records["ammonia"]
	rec.CardHash = "sha256:stale"
	raceBalancerCurveStore.mu.Unlock()

	status := RaceStatusList()
	var found *RaceStatus
	for i := range status {
		if status[i].RaceID == "ammonia" {
			found = &status[i]
			break
		}
	}
	require.NotNil(t, found)
	require.False(t, found.CardHashOK, "card_hash ≠ текущий → «заводские устарели»")

	// После generate → флаг снят.
	require.NoError(t, RegenerateRaceFactory("ammonia"))
	status = RaceStatusList()
	for i := range status {
		if status[i].RaceID == "ammonia" {
			require.True(t, status[i].CardHashOK, "после generate флаг «устарело» снят")
		}
	}
}

// Спец-случай humans (§2.3): записи "humans" в расовом store не создаются.
func TestRaceBalancerHumansSkipped(t *testing.T) {
	resetRaceBalancerStore()
	defer resetRaceBalancerStore()

	require.NoError(t, LoadRaceBalancer(filepath.Join(t.TempDir(), "race_balancer.json")))
	_, ok := GetRaceRecordMeta("humans")
	require.False(t, ok, "запись humans не создаётся")

	// deriveRaceCurves для humans — ошибка (не вывод из карточки).
	_, err := deriveRaceCurves(races.ByID("humans"))
	require.Error(t, err)
}

// Валидация записи: невалидные записи при загрузке пропускаются с логом
// (не роняют старт) — reproduction ≤ 0 и битая кривая.
func TestRaceBalancerInvalidRecordSkipped(t *testing.T) {
	resetRaceBalancerStore()
	defer resetRaceBalancerStore()

	dir := t.TempDir()
	path := filepath.Join(dir, "race_balancer.json")

	// Запись с reproduction ≤ 0 и запись с битой кривой (Y ≥ 1).
	bad := map[string]interface{}{
		"races": []interface{}{
			map[string]interface{}{
				"race_id": "bad_repro", "card_hash": "sha256:x",
				"factory": map[string]interface{}{
					"reproduction": 0,
					"curves": map[string]interface{}{
						"heat":      map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"x": 0, "y": 0}, map[string]interface{}{"x": 1, "y": 0}, map[string]interface{}{"x": 2, "y": 0}}, "bends": []interface{}{0, 0}},
						"cold":      map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"x": 0, "y": 0}, map[string]interface{}{"x": 1, "y": 0}, map[string]interface{}{"x": 2, "y": 0}}, "bends": []interface{}{0, 0}},
						"gravity":   map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"x": 0, "y": 0}, map[string]interface{}{"x": 1, "y": 0}, map[string]interface{}{"x": 2, "y": 0}}, "bends": []interface{}{0, 0}},
						"radiation": map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"x": 0, "y": 0}, map[string]interface{}{"x": 1, "y": 0}, map[string]interface{}{"x": 2, "y": 0}}, "bends": []interface{}{0, 0}},
					},
				},
				"active": map[string]interface{}{
					"reproduction": 1,
					"curves": map[string]interface{}{
						"heat":      map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"x": 0, "y": 0}, map[string]interface{}{"x": 1, "y": 0}, map[string]interface{}{"x": 2, "y": 0}}, "bends": []interface{}{0, 0}},
						"cold":      map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"x": 0, "y": 0}, map[string]interface{}{"x": 1, "y": 0}, map[string]interface{}{"x": 2, "y": 0}}, "bends": []interface{}{0, 0}},
						"gravity":   map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"x": 0, "y": 0}, map[string]interface{}{"x": 1, "y": 0}, map[string]interface{}{"x": 2, "y": 0}}, "bends": []interface{}{0, 0}},
						"radiation": map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"x": 0, "y": 0}, map[string]interface{}{"x": 1, "y": 0}, map[string]interface{}{"x": 2, "y": 0}}, "bends": []interface{}{0, 0}},
					},
				},
				"updated_at": "2026-09-17T00:00:00Z",
			},
		},
	}
	data, err := json.Marshal(bad)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	require.NoError(t, LoadRaceBalancer(path), "невалидные записи пропускаются, старт не падает")
	_, ok := GetRaceRecordMeta("bad_repro")
	require.False(t, ok, "невалидная запись пропущена")
	// Остальные расы инициализированы из карточек.
	_, ok = GetRaceRecordMeta("ammonia")
	require.True(t, ok)
}