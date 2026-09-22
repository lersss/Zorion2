// internal/generator/planet/composition_history_test.go
//
// Тесты этапа 2 протопылевого облака (спека
// 2026-09-22-облако-этап-2-состав-и-история-формирования.md §9, H1–H20):
// снеговая линия внутрь, конденсация/миграция/потеря/эпоха, маркер
// formation_history и его условия, поток RNG (+2 ролла/планету), рамки
// реестра. Модельные замеры — по функции/потоку (не по дампу planet.data,
// PITFALLS «Дизайн и числа»: порядок итерации map недетерминирован).
package planet

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator/settlement"
	"zorion/internal/models"
	"zorion/internal/races"
)

// ==================== СЭМПЛЕРЫ МОДЕЛИ ====================

// compSample — модельный замер одной планеты: x_now, доли и история.
type compSample struct {
	xNow  float64
	rock  float64
	iron  float64
	ice   float64
	hist  []models.PlanetFormationEvent
}

// sampleMode — режим миграции мира по одному роллу (та же дистрибуция, что
// гейт §4.4: 2% сильная, 15% умеренная, 83% нет).
func sampleMode(rng *rand.Rand) int {
	u := rng.Float64()
	switch {
	case u < migGateStrongHi:
		return migrationStrong
	case u < migGateModerateHi:
		return migrationModerate
	default:
		return migrationNone
	}
}

// sampleComp — модельный замер состава: 5 роллов состава (§5).
func sampleComp(rng *rand.Rand, xNow float64, mode int) compSample {
	r, ir, ic, h := compositionFromHistory(
		xNow,
		rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64(),
		mode, 1, 0, defaultFormationParams(),
	)
	return compSample{xNow: xNow, rock: r, iron: ir, ice: ic, hist: h}
}

// sampleOrbits — выборка тел: орбита 1..8 G-звезды (x_now = r_i), режим
// миграции из гейта мира.
func sampleOrbits(n int, seed int64) []compSample {
	rng := rand.New(rand.NewSource(seed))
	out := make([]compSample, 0, n)
	for i := 0; i < n; i++ {
		orbit := 1 + rng.Intn(8)
		mode := sampleMode(rng)
		out = append(out, sampleComp(rng, orbitRadiusScaled(orbit, 1), mode))
	}
	return out
}

// zoneMedianIce — медиана w_ice по набору орбит (зона).
func zoneMedianIce(seed int64, orbits []int, per int) float64 {
	rng := rand.New(rand.NewSource(seed))
	var vals []float64
	for _, o := range orbits {
		x := orbitRadiusScaled(o, 1)
		for i := 0; i < per; i++ {
			mode := sampleMode(rng)
			vals = append(vals, sampleComp(rng, x, mode).ice)
		}
	}
	sort.Float64s(vals)
	return medianOf(vals)
}

// hasType — есть ли в истории запись данного типа.
func hasType(h []models.PlanetFormationEvent, typ string) bool {
	for _, ev := range h {
		if ev.Type == typ {
			return true
		}
	}
	return false
}

// ==================== H1: СНЕГОВАЯ ЛИНИЯ ====================

// H1 — snowLineAt монотонно не возрастает: x_ice(0) ∈ [5, 6],
// x_ice(3) = 2.70 ± 0.01, x_ice(10) = 2.70.
func TestSnowLineEvolvesInward(t *testing.T) {
	assert.GreaterOrEqual(t, snowLineAt(0), 5.0)
	assert.LessOrEqual(t, snowLineAt(0), 6.0)
	prev := snowLineAt(0)
	for tt := 0.0; tt <= 12.0; tt += 0.1 {
		cur := snowLineAt(tt)
		assert.LessOrEqual(t, cur, prev+1e-12, "монотонно не возрастает при t=%.1f", tt)
		prev = cur
	}
	assert.InDelta(t, 2.70, snowLineAt(3), 0.01)
	assert.InDelta(t, 2.70, snowLineAt(10), 0.001)
	assert.InDelta(t, 5.5, snowLineAt(0), 1e-9)
}

// ==================== H2–H5: «НЕПРАВИЛЬНЫЕ» СОСТАВОМ ====================

// H2 — доля тел с x_now > 2.7 и w_ice < 0.10 (крупные — суперземли) ∈ [1%, 5%].
func TestRockyBeyondPresentLine(t *testing.T) {
	require.NotNil(t, sampleOrbits(1, 1))
	all := sampleOrbits(40000, 20260922)
	cnt := 0
	for _, s := range all {
		if s.xNow > 2.7 && s.ice < 0.10 {
			cnt++
		}
	}
	frac := float64(cnt) / float64(len(all))
	t.Logf("H2: каменистые за линией (x_now>2.7, w_ice<0.10): %.2f%%", frac*100)
	assert.GreaterOrEqual(t, frac, 0.01)
	assert.LessOrEqual(t, frac, 0.05)
}

// H3 — доля тел с x_now < 2.7 и w_ice ≥ 0.30 (лёд внутри линии) ∈ [0.5%, 5%].
func TestIcyInsidePresentLine(t *testing.T) {
	all := sampleOrbits(40000, 20260923)
	cnt := 0
	for _, s := range all {
		if s.xNow < 2.7 && s.ice >= 0.30 {
			cnt++
		}
	}
	frac := float64(cnt) / float64(len(all))
	t.Logf("H3: ледяные внутри линии (x_now<2.7, w_ice≥0.30): %.2f%%", frac*100)
	assert.GreaterOrEqual(t, frac, 0.005)
	assert.LessOrEqual(t, frac, 0.05)
}

// H4 — при x_now > 4.5 доля тел с w_ice ≥ 0.30 ≥ 85% (дальняя зона держит лёд).
func TestFarZoneKeepsIce(t *testing.T) {
	all := sampleOrbits(40000, 20260924)
	total, icy := 0, 0
	for _, s := range all {
		if s.xNow > 4.5 {
			total++
			if s.ice >= 0.30 {
				icy++
			}
		}
	}
	require.Greater(t, total, 100)
	frac := float64(icy) / float64(total)
	t.Logf("H4: лёд в дальней зоне (x_now>4.5, w_ice≥0.30): %.2f%% (%d/%d)", frac*100, icy, total)
	assert.GreaterOrEqual(t, frac, 0.85)
}

// H5 — суммарная доля «не по зоне» ∈ [3%, 12%].
func TestWrongCompositionShare(t *testing.T) {
	all := sampleOrbits(40000, 20260925)
	cnt := 0
	for _, s := range all {
		if (s.xNow > 2.7 && s.ice < 0.10) || (s.xNow < 2.7 && s.ice >= 0.30) {
			cnt++
		}
	}
	frac := float64(cnt) / float64(len(all))
	t.Logf("H5: «не по зоне»: %.2f%%", frac*100)
	assert.GreaterOrEqual(t, frac, 0.03)
	assert.LessOrEqual(t, frac, 0.12)
}

// ==================== H6: ПОЛОСЫ СОХРАНЕНЫ ====================

// H6 — медианы w_ice строго возрастают по зонам; внутренние (орб. 2–3) и
// дальняя (орб. 5+) — в пределах ±0.10 текущих полос §4.6; переходная
// (орб. 4) может превышать полосу, но остаётся ниже дальней.
func TestCompositionBandsPreserved(t *testing.T) {
	medInner := zoneMedianIce(6001, []int{2, 3}, 20000)
	medTransition := zoneMedianIce(6002, []int{4}, 20000)
	medOuter := zoneMedianIce(6003, []int{5, 6, 7}, 20000)
	t.Logf("H6: медианы w_ice: внутренняя=%.3f, переходная=%.3f, дальняя=%.3f",
		medInner, medTransition, medOuter)

	assert.Less(t, medInner, medTransition, "медиана внутренней < переходной")
	assert.Less(t, medTransition, medOuter, "медиана переходной < дальней")
	// Внутренняя зона: полоса §4.6 ≈ 0.04–0.17 → допуск ±0.10.
	assert.LessOrEqual(t, medInner, 0.27, "внутренняя в пределах ±0.10 полосы")
	// Дальняя зона: полоса §4.6 ≈ 0.40–0.64 → допуск ±0.10.
	assert.GreaterOrEqual(t, medOuter, 0.30, "дальняя в пределах ±0.10 полосы")
	// Переходная законно выше полосы, но ниже дальней (цель этапа 2).
	assert.Less(t, medTransition, medOuter)
}

// ==================== H7: СХЕМА МАРКЕРА ====================

// H7 — formation_history — массив объектов с type из §6.2; порядок записей
// фиксирован; неизвестный type отсутствует.
func TestFormationHistorySchema(t *testing.T) {
	order := map[string]int{
		"formed_early": 0, "formed_late": 0, "migrated": 1,
		"ice_lost": 2, "stripped_embryo": 3,
	}
	rng := rand.New(rand.NewSource(7001))
	for i := 0; i < 20000; i++ {
		orbit := 1 + rng.Intn(8)
		giant := 0
		if rng.Float64() < 0.3 {
			giant = 1 + rng.Intn(8)
		}
		mode := sampleMode(rng)
		_, _, _, h := compositionFromHistory(
			orbitRadiusScaled(orbit, 1),
			rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64(),
			mode, orbit, giant, defaultFormationParams(),
		)
		last := -1
		for _, ev := range h {
			idx, ok := order[ev.Type]
			require.True(t, ok, "неизвестный тип %q", ev.Type)
			require.GreaterOrEqual(t, idx, last, "порядок записей нарушен: %v", h)
			last = idx
		}
	}

	// Данные планеты: ключ — массив объектов с type.
	g := NewGenerator(nil, 7002)
	sp := stellarParamsFromClass("G", 5772, g.rng)
	pd := g.generateStandardPlanet("w", "W", 4, sp, false, nil, nil)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(pd.Data, &data))
	arr, ok := data["formation_history"].([]interface{})
	require.True(t, ok, "formation_history — массив объектов")
	for _, it := range arr {
		m, ok := it.(map[string]interface{})
		require.True(t, ok)
		typ, _ := m["type"].(string)
		_, known := order[typ]
		assert.True(t, known, "тип из §6.2: %q", typ)
	}
}

// ==================== H8: УСЛОВИЯ МАРКЕРА ====================

// H8 — formed_late ⟹ w_ice ≤ 0.10; migrated ⟹ f ≥ 1.3; ice_lost ⟹ L ≥ 0.20;
// stripped_embryo ⟹ |i−g| ≤ 2.
func TestFormationHistoryConditions(t *testing.T) {
	rng := rand.New(rand.NewSource(8001))
	badLate, badMig, badLoss, badEmbryo := 0, 0, 0, 0
	for i := 0; i < 30000; i++ {
		orbit := 1 + rng.Intn(8)
		giant := 0
		if rng.Float64() < 0.25 {
			giant = 1 + rng.Intn(8)
		}
		mode := sampleMode(rng)
		_, _, ice, h := compositionFromHistory(
			orbitRadiusScaled(orbit, 1),
			rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64(),
			mode, orbit, giant, defaultFormationParams(),
		)
		for _, ev := range h {
			switch ev.Type {
			case "formed_late":
				if ice > 0.10 {
					badLate++
				}
			case "migrated":
				if ev.Factor < 1.3 {
					badMig++
				}
			case "ice_lost":
				if ev.Fraction < 0.20 {
					badLoss++
				}
			case "stripped_embryo":
				d := orbit - ev.GiantOrbit
				if d < 0 {
					d = -d
				}
				if d > 2 {
					badEmbryo++
				}
			}
		}
	}
	assert.Zero(t, badLate, "formed_late ⟹ сухое тело")
	assert.Zero(t, badMig, "migrated ⟹ f ≥ 1.3")
	assert.Zero(t, badLoss, "ice_lost ⟹ L ≥ 0.20")
	assert.Zero(t, badEmbryo, "stripped_embryo ⟹ |i−g| ≤ 2")
}

// ==================== H9: ПЕРЕРАСПРЕДЕЛЕНИЕ ПРИ ПОТЕРЕ ====================

// compLossCase — фиксированные роллы (x_now=1.5, основная эпоха, inward
// f=2.0 → swept-условие), меняется только u_loss.
func compLossCase(uLoss float64) (float64, float64, float64, []models.PlanetFormationEvent) {
	return compositionFromHistory(
		1.5, 0.5, 0.0, uLoss, 0.5, 0.5,
		migrationStrong, 2, 0, defaultFormationParams(),
	)
}

// H9 — при ice_lost residue=iron доля железа растёт; residue=bare_ice доля
// льда растёт; сумма = 1 ± 1e−9; полы w ≥ 0.02.
func TestIceLostRedistributes(t *testing.T) {
	_, ironBase, iceBase, hBase := compLossCase(0.30) // u_loss ≥ p_loss — потери нет
	require.False(t, hasType(hBase, "ice_lost"))

	rIron, ironIron, iceIron, hIron := compLossCase(0.15) // u_l = 0.6 → iron, L = 0.40
	require.True(t, hasType(hIron, "ice_lost"))
	assert.Greater(t, ironIron, ironBase, "residue=iron: доля железа выше эталонной")
	assert.Less(t, iceIron, iceBase, "residue=iron: лёд снят")
	assert.InDelta(t, 1.0, rIron+ironIron+iceIron, 1e-9, "сумма долей = 1")

	rBare, ironBare, iceBare, hBare := compLossCase(0.05) // u_l = 0.2 → bare_ice, L = 0.20
	require.True(t, hasType(hBare, "ice_lost"))
	assert.Greater(t, iceBare, iceBase, "residue=bare_ice: доля льда выше эталонной")
	assert.InDelta(t, 1.0, rBare+ironBare+iceBare, 1e-9, "сумма долей = 1")

	for _, v := range []float64{rIron, ironIron, iceIron, rBare, ironBare, iceBare} {
		assert.GreaterOrEqual(t, v, shareFloor-1e-9, "пол w ≥ 0.02")
	}
	for _, h := range [][]models.PlanetFormationEvent{hIron, hBare} {
		for _, ev := range h {
			if ev.Type == "ice_lost" {
				assert.GreaterOrEqual(t, ev.Fraction, 0.20)
				assert.Contains(t, []string{"iron", "bare_ice"}, ev.Residue)
			}
		}
	}
}

// ==================== H10: ЧАСТОТА ГЕЙТА МИГРАЦИИ ====================

// H10 — доля миров: сильная ∈ [1%, 3%], умеренная ∈ [10%, 30%].
func TestMigrationGateFrequency(t *testing.T) {
	g := NewGenerator(nil, 10001)
	const n = 200000
	strong, moderate := 0, 0
	for i := 0; i < n; i++ {
		switch g.rollMigrationMode() {
		case migrationStrong:
			strong++
		case migrationModerate:
			moderate++
		}
	}
	fs := float64(strong) / n
	fm := float64(moderate) / n
	t.Logf("H10: гейт миграции — сильная %.2f%%, умеренная %.2f%%", fs*100, fm*100)
	assert.GreaterOrEqual(t, fs, 0.01)
	assert.LessOrEqual(t, fs, 0.03)
	assert.GreaterOrEqual(t, fm, 0.10)
	assert.LessOrEqual(t, fm, 0.30)
}

// ==================== H11: x_form = x_now БЕЗ МИГРАЦИИ ====================

// H11 — без миграции x_form = x_now (маркера migrated нет; доминирующий
// случай ≥ 83% тел).
func TestXFormEqualsXNowWhenNoMigration(t *testing.T) {
	rng := rand.New(rand.NewSource(11001))
	for i := 0; i < 5000; i++ {
		_, _, _, h := compositionFromHistory(
			orbitRadiusScaled(1+rng.Intn(8), 1),
			rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64(),
			migrationNone, 2, 0, defaultFormationParams(),
		)
		assert.False(t, hasType(h, "migrated"), "без миграции маркера migrated нет")
	}

	all := sampleOrbits(40000, 11002)
	migrated := 0
	for _, s := range all {
		if hasType(s.hist, "migrated") {
			migrated++
		}
	}
	share := float64(migrated) / float64(len(all))
	t.Logf("H11: доля тел с миграцией: %.2f%%", share*100)
	assert.GreaterOrEqual(t, 1-share, 0.83, "доминирующий случай ≥ 83%")
}

// ==================== H12: ГИГАНТЫ/ЭКЗОТИКА НЕ ТРОНУТЫ ====================

// H12 — гиганты, экзотика, MassOverride — без formation_history.
func TestGiantsExoticUntouched(t *testing.T) {
	g := NewGenerator(nil, 12001)
	res := g.runCascadeGiant(cascadeInput{
		Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, TEff: 5772,
		OrbitRadiusAU: 5.2, OrbitIndex: 5, MassOverride: 317.8,
	})
	assert.Empty(t, res.FormationHistory, "гигант без маркера")
	// Кривая M→R не тронута.
	assert.Equal(t, GasGiantRadiusMax, GasGiantRadius(GasGiantMassMax), "M→R гигантов")

	g2 := NewGenerator(nil, 12002)
	p := g2.generateExoticPlanet(WorldInfo{ID: "w", Name: "W", StarType: "white_dwarf"}, 5)
	require.NotNil(t, p)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(p.Data, &data))
	_, has := data["formation_history"]
	assert.False(t, has, "экзотика без маркера")

	// MassOverride — прежняя зонная функция, без маркера.
	g3 := NewGenerator(nil, 12003)
	res3 := g3.runCascade(cascadeInput{
		Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, TEff: 5772,
		OrbitRadiusAU: 1.156, OrbitIndex: 2, MassOverride: 1,
	})
	assert.Empty(t, res3.FormationHistory, "MassOverride без маркера")
}

// ==================== H13: СОСТАВ ПОЯСОВ НЕ МЕНЯЕТСЯ ====================

// H13 — beltComposition делегирует compositionByZone (не compositionFromHistory).
func TestBeltsCompositionUnchanged(t *testing.T) {
	a := NewGenerator(nil, 13001)
	comp := beltComposition(5.0, 1.0, a.rng)
	b := NewGenerator(nil, 13001)
	r, ir, ic := compositionByZone(5.0, 1.0, b.rng)
	assert.InDelta(t, r, comp[beltRockKey], 1e-12)
	assert.InDelta(t, ir, comp[beltIronKey], 1e-12)
	assert.InDelta(t, ic, comp[beltIceKey], 1e-12)
	// Пояс остаётся зонным: за линией лёд высокий.
	assert.Greater(t, comp[beltIceKey], 0.4)
}

// ==================== H15: ДЕТЕРМИНИЗМ ПО ПОТОКУ ====================

// H15 — один seed → одна последовательность состава/истории (функция от
// потока, не дамп planet.data).
func TestCompositionDeterministicByStream(t *testing.T) {
	seq := func() []string {
		rng := rand.New(rand.NewSource(4242))
		out := make([]string, 0, 300)
		for i := 0; i < 300; i++ {
			orbit := 1 + rng.Intn(8)
			mode := sampleMode(rng)
			s := sampleComp(rng, orbitRadiusScaled(orbit, 1), mode)
			b, _ := json.Marshal(s.hist)
			out = append(out, fmt.Sprintf("%.12f|%.12f|%.12f|%s", s.rock, s.iron, s.ice, b))
		}
		return out
	}
	assert.Equal(t, seq(), seq(), "один seed → одна последовательность")
}

// ==================== H16: СДВИГ ПОТОКА RNG ====================

// H16 — runCascade: 5 роллов состава (было 3 у compositionByZone → +2) до
// последующих потреблений; rollMigrationMode — ровно один ролл на мир.
func TestRngRollsNamedShift(t *testing.T) {
	const seed = 16001
	in := cascadeInput{
		Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, TEff: 5772,
		OrbitRadiusAU: orbitRadiusByIndex(4), OrbitIndex: 4,
	}
	g1 := NewGenerator(nil, seed)
	res1 := g1.runCascade(in)

	// Следующий после состава ролл — эксцентриситет: ζ (NormFloat64) + 5
	// Float64 состава (было 3 у compositionByZone → +2) → эксцентриситет.
	g2 := NewGenerator(nil, seed)
	_ = g2.rng.NormFloat64() // ζ — масса
	for i := 0; i < 5; i++ {
		_ = g2.rng.Float64() // u_t, u_mig, u_loss, J_ice, u_iron
	}
	expectedEcc := 0.03 + g2.rng.Float64()*0.27
	assert.Equal(t, expectedEcc, res1.Eccentricity,
		"состав — 5 роллов (+2 к compositionByZone) до эксцентриситета")

	// Гейт миграции — ровно один Float64.
	m1 := NewGenerator(nil, seed)
	_ = m1.rollMigrationMode()
	ma := m1.rng.Float64()
	m2 := NewGenerator(nil, seed)
	_ = m2.rng.Float64()
	mb := m2.rng.Float64()
	assert.Equal(t, mb, ma, "rollMigrationMode — ровно один ролл")
}

// ==================== H17: РАМКИ ПОКРЫТЫ СОСТАВОМ ====================

// H17 — фактические минимумы size/gravity внутри расширенных рамок fields.go.
func TestRegistryFramesCoveredByComposition(t *testing.T) {
	g := NewGenerator(nil, 17001)
	minSize, minGrav := math.Inf(1), math.Inf(1)
	buf := newBatchBuffers(64)
	for i := 0; i < 400; i++ {
		cls := []string{"G", "K", "M", "F"}[i%4]
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: cls, StarType: "star"},
			g.determinePlanetCount(cls), buf)
		for _, row := range buf.planetRows {
			fields := row.([]interface{})
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
			if data["is_gas_giant"] == true {
				continue
			}
			if s, ok := data["size"].(float64); ok && s > 0 && s < minSize {
				minSize = s
			}
			if gr, ok := data["gravity"].(float64); ok && gr > 0 && gr < minGrav {
				minGrav = gr
			}
		}
	}
	t.Logf("H17: факт. min size=%.4f (рамка 0.23), min gravity=%.4f (рамка 0.12)", minSize, minGrav)
	assert.GreaterOrEqual(t, minSize, 0.23, "size не выходит ниже рамки")
	assert.GreaterOrEqual(t, minGrav, 0.12, "gravity не выходит ниже рамки")
}

// ==================== H18: ЗАМЕР ОКНА ПРИГОДНОСТИ (информационный) ====================

// H18 — информационный замер доли Settleable и долей вне окна [0.5, 2]
// (без порога — наблюдаемая величина, решение создателя 2026-09-21).
func TestSettleableWindowMeasured(t *testing.T) {
	g := NewGenerator(nil, 18001)
	total, settle, low, high := 0, 0, 0, 0
	buf := newBatchBuffers(64)
	for i := 0; i < 3000; i++ {
		cls := []string{"G", "K", "M", "F", "A"}[i%5]
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: cls, StarType: "star"},
			g.determinePlanetCount(cls), buf)
		for _, row := range buf.planetRows {
			fields := row.([]interface{})
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
			if data["is_gas_giant"] == true {
				continue
			}
			total++
			if gr, ok := data["gravity"].(float64); ok {
				if gr < 0.5 {
					low++
				}
				if gr > 2 {
					high++
				}
			}
			if races.HumansSuitable(data) {
				settle++
			}
		}
	}
	require.Greater(t, total, 100)
	t.Logf("H18: Settleable %.2f%%; g<0.5 %.2f%%; g>2 %.2f%% (всего %d)",
		float64(settle)/float64(total)*100, float64(low)/float64(total)*100,
		float64(high)/float64(total)*100, total)
}

// ==================== H20: РАМКИ РЕЕСТРА РАСШИРЕНЫ ====================

// H20 — fields.go: size.Min == 0.23, gravity.Min == 0.12; density не тронута
// (0.077 / 2.95); mass не тронута (0.02 / ≥ GasGiantMassMax).
func TestRegistryFramesExtended(t *testing.T) {
	reg := map[string]settlement.FieldSpec{}
	for _, s := range settlement.FieldRegistry() {
		reg[s.Key] = s
	}
	require.Contains(t, reg, "size")
	require.Contains(t, reg, "gravity")
	require.Contains(t, reg, "density")
	require.Contains(t, reg, "mass")

	assert.Equal(t, 0.23, *reg["size"].Min, "size.Min расширена (§7.2)")
	assert.Equal(t, 0.12, *reg["gravity"].Min, "gravity.Min расширена (§7.2)")
	assert.Equal(t, 0.077, *reg["density"].Min, "density не тронута")
	assert.Equal(t, 2.95, *reg["density"].Max, "density не тронута")
	assert.Equal(t, 0.02, *reg["mass"].Min, "mass не тронута")
}
