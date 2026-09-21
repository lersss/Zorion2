// internal/generator/planet/accretion_mass_test.go
//
// Тесты массы каменистых/ледяных планет (спека
// 2026-09-21-масса-каменистых-и-ледяных-планет.md, §8 T1–T16;
// дополнение — протопылевое облако, §8.2 T17–T22 и C1–C5):
// ядро V1 от нормированного расстояния, бюджет облака M_диск, границы
// 0.02–8, реестр полей и единый источник номинала подкрутки рас.
package planet

import (
	"encoding/json"
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator/galaxy"
	"zorion/internal/generator/settlement"
	"zorion/internal/models"
)

// ==================== ПЛОЩАДКА ЗАМЕРА ====================

// sampleRockyMasses — массы каменистых/ледяных планет мирового потока
// (с per-системным бюджетом B): веса классов как у галактики
// (galaxy.DefaultWeights), гиганты исключены.
func sampleRockyMasses(t *testing.T, g *Generator, target int) []float64 {
	t.Helper()
	classes := []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}
	weights := galaxy.DefaultWeights().Spectral
	total := 0.0
	for _, c := range classes {
		total += weights[c]
	}
	pickClass := func() string {
		x := g.rng.Float64() * total
		for _, c := range classes {
			x -= weights[c]
			if x <= 0 {
				return c
			}
		}
		return classes[len(classes)-1]
	}

	var masses []float64
	buf := newBatchBuffers(64)
	for i := 0; len(masses) < target && i < target*8; i++ {
		cls := pickClass()
		count := g.determinePlanetCount(cls)
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: cls, StarType: "star"}, count, buf)
		for _, row := range buf.planetRows {
			fields := row.([]interface{})
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
			if data["is_gas_giant"] == true {
				continue
			}
			m, ok := data["mass"].(float64)
			require.True(t, ok, "mass обязателен")
			masses = append(masses, m)
		}
	}
	require.GreaterOrEqual(t, len(masses), target, "набрали выборку")
	return masses[:target]
}

// cascadeMasses — n масс каскада для орбиты (без гигантов, бюджет B = дефолт
// генератора; runCascade напрямую — per-системный ролл здесь не участвует).
func cascadeMasses(g *Generator, sp StellarParams, orbit, n int) []float64 {
	out := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		res := g.runCascade(cascadeInput{
			Luminosity:    sp.Luminosity,
			StellarMass:   sp.StellarMass,
			AgeGyr:        sp.AgeGyr,
			Metallicity:   sp.Metallicity,
			TEff:          sp.TEff,
			OrbitRadiusAU: orbitRadiusScaled(orbit, sp.Luminosity),
			OrbitIndex:    orbit,
		})
		out = append(out, res.Mass)
	}
	return out
}

// medianOf — медиана выборки (выборка не мутируется).
func medianOf(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

// percentile — p-й процентиль отсортированной выборки (линейная интерполяция).
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return sorted[lo]
	}
	w := idx - float64(lo)
	return sorted[lo]*(1-w) + sorted[hi]*w
}

// pearson — коэффициент корреляции Пирсона.
func pearson(xs, ys []float64) float64 {
	n := float64(len(xs))
	if n == 0 {
		return 0
	}
	mx, my := 0.0, 0.0
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	mx /= n
	my /= n
	var sxy, sxx, syy float64
	for i := range xs {
		dx := xs[i] - mx
		dy := ys[i] - my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}

// ==================== T1/T2: ХВОСТЫ РАСПРЕДЕЛЕНИЯ ====================

// T1 — отсутствие сваливания в пол 0.02 (пол — страховка реестра, не рычаг)
// и нижний хвост < 0.5 не доминирует. Пороги пересчитаны под этап 1
// (σ_ζ 0.6 + f_обр, спека протопылевого облака §8.1): факт пола 0.066%
// (порог < 0.5%), факт < 0.5 — 18.3% (порог ≤ 26%).
func TestMassNoFloorPileup(t *testing.T) {
	g := NewGenerator(nil, 20260921)
	masses := sampleRockyMasses(t, g, 5000)
	require.Len(t, masses, 5000)

	floor, low := 0, 0
	for _, m := range masses {
		if m == 0.02 {
			floor++
		}
		if m < 0.5 {
			low++
		}
	}
	floorFrac := float64(floor) / float64(len(masses))
	lowFrac := float64(low) / float64(len(masses))
	t.Logf("пол 0.02: %.3f%%; < 0.5 M⊕: %.2f%%", floorFrac*100, lowFrac*100)

	assert.Less(t, floorFrac, 0.005, "доля на полу 0.02 < 0.5%% (пол не срабатывает штатно; факт 0.066%%)")
	assert.LessOrEqual(t, lowFrac, 0.26, "доля < 0.5 M⊕ ≤ 26%% (факт 18.3%%)")
}

// T2 — отсутствие сваливания в потолок 8.00. Порог пересчитан под этап 1
// (σ_ζ 0.6): факт 3.31%, порог < 5% (спека протопылевого облака §8.1).
// Полка ровно 8.00 — ВХОД задачи перелива массы в гигантов (не дефект этой
// задачи, §4.5 спеки).
func TestMassNoCeilingPileup(t *testing.T) {
	g := NewGenerator(nil, 20260922)
	masses := sampleRockyMasses(t, g, 5000)
	require.Len(t, masses, 5000)

	ceil := 0
	for _, m := range masses {
		if m == 8.0 {
			ceil++
		}
	}
	frac := float64(ceil) / float64(len(masses))
	t.Logf("ровно 8.00: %.2f%% (вход задачи перелива, факт 3.31%%)", frac*100)
	assert.Less(t, frac, 0.05, "доля ровно 8.0 < 5%%")
}

// ==================== T3/T4/T5: СТРУКТУРА ====================

// T3 — внутрисистемная структура: у G медианы орбит 1/3/5/7 строго
// возрастают, медиана(7)/медиана(1) > 2.5. Проверяет ПОТЕНЦИАЛ системы
// (giantOrbit = 0 → f_обр = 1, без обрезки); M_диск — общий множитель
// системы, структуру не сдвигает (спека протопылевого облака §8.1).
func TestMassSpreadAcrossOrbits(t *testing.T) {
	g := NewGenerator(nil, 20260923)
	sp := stellarParamsFromClass("G", 5772, g.rng)
	med := func(orbit int) float64 { return medianOf(cascadeMasses(g, sp, orbit, 1500)) }

	m1, m3, m5, m7 := med(1), med(3), med(5), med(7)
	t.Logf("медианы орбит 1/3/5/7: %.3f / %.3f / %.3f / %.3f", m1, m3, m5, m7)
	assert.Less(t, m1, m3, "медиана орбиты 1 < 3")
	assert.Less(t, m3, m5, "медиана орбиты 3 < 5")
	assert.Less(t, m5, m7, "медиана орбиты 5 < 7")
	assert.Greater(t, m7/m1, 2.5, "медиана(7)/медиана(1) > 2.5")
}

// T4 — M-карлик не сваливается в пол: доля на 0.02 < 1%%, медиана > 0.5 M⊕.
func TestMassMDwarfNotOnFloor(t *testing.T) {
	g := NewGenerator(nil, 20260924)
	sp := stellarParamsFromClass("M", 0, g.rng)
	vals := cascadeMasses(g, sp, 2, 5000)

	floor := 0
	for _, m := range vals {
		if m == 0.02 {
			floor++
		}
	}
	median := medianOf(vals)
	t.Logf("M (орбита 2): пол %.2f%%, медиана %.3f", float64(floor)/float64(len(vals))*100, median)
	assert.Less(t, float64(floor)/float64(len(vals)), 0.01, "M: доля на полу < 1%%")
	assert.Greater(t, median, 0.5, "M: медиана > 0.5 M⊕ (получено %.2f)", median)
}

// T5 — светимость не входит в массу дважды (выбор V1): медиана массы на
// одной орбите у G и M отличается < 20%%.
// f_обр — функция номеров орбит (не физического r), поэтому самоподобие
// по √L сохранено: обрезка у G и M на одной орбите одинакова (см. T18).
func TestMassNoLuminosityDoubleCount(t *testing.T) {
	g := NewGenerator(nil, 20260925)
	gMed := medianOf(cascadeMasses(g, stellarParamsFromClass("G", 5772, g.rng), 2, 3000))
	mMed := medianOf(cascadeMasses(g, stellarParamsFromClass("M", 0, g.rng), 2, 3000))
	diff := math.Abs(gMed-mMed) / gMed
	t.Logf("медианы орбиты 2: G=%.3f, M=%.3f (отличие %.1f%%)", gMed, mMed, diff*100)
	assert.Less(t, diff, 0.20, "медианы G и M на орбите 2 отличаются < 20%%")
}

// ==================== T9: ГИГАНТЫ НЕ ЗАТРОНУТЫ (регресс) ====================

// T9 — кривая M→R и распределение масс гигантов не изменены.
func TestGiantMassUntouched(t *testing.T) {
	assert.Equal(t, GasGiantRadiusMax, GasGiantRadius(GasGiantMassRef), "кривая M→R")
	assert.Equal(t, GasGiantRadiusMax, GasGiantRadius(GasGiantMassMax), "плато насыщения")
	g := NewGenerator(nil, 20260929)
	for i := 0; i < 2000; i++ {
		m := g.gasGiantMass()
		require.GreaterOrEqual(t, m, GasGiantMassMin)
		require.LessOrEqual(t, m, GasGiantMassMax)
	}
}

// ==================== T10: АУДИТ M–R–ρ (регресс) ====================

// T10 — сгенерированная планета проходит правило checkMassSizeDensity:
// R = (M/ρ)^(1/3) (допуск 0.05 из internal/audit/planet).
func TestMassSizeDensityAudit(t *testing.T) {
	g := NewGenerator(nil, 20260930)
	buf := newBatchBuffers(64)
	checked := 0
	for i := 0; i < 300; i++ {
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
			mass, _ := data["mass"].(float64)
			size, _ := data["size"].(float64)
			density, _ := data["density"].(float64)
			expected := math.Cbrt(mass / density)
			assert.InDelta(t, expected, size, 0.05, "R = cbrt(M/ρ)")
			checked++
		}
	}
	require.Greater(t, checked, 100, "выборка не пустая")
}

// ==================== T11: РЕЕСТР ПОЛЕЙ ====================

// T11 — рамки реестра (fields.go) совпадают со спекой: mass.Min 0.02,
// size.Min 0.25, gravity.Min 0.13 (страховка от пола); mass.Max ≥ гигантов.
func TestFieldRegistryMassRange(t *testing.T) {
	reg := map[string]settlement.FieldSpec{}
	for _, s := range settlement.FieldRegistry() {
		reg[s.Key] = s
	}
	requireField := func(key string) settlement.FieldSpec {
		s, ok := reg[key]
		require.True(t, ok, "нет поля %q", key)
		require.NotNil(t, s.Min, "%s: нет min", key)
		require.NotNil(t, s.Max, "%s: нет max", key)
		return s
	}

	s := requireField("mass")
	assert.Equal(t, 0.02, *s.Min, "mass.Min — страховка каскада (§5.3)")
	assert.GreaterOrEqual(t, *s.Max, GasGiantMassMax, "mass.Max — ветка гигантов")

	s = requireField("size")
	assert.Equal(t, 0.25, *s.Min, "size.Min — страховка от M_min (§5.3)")

	s = requireField("gravity")
	assert.Equal(t, 0.13, *s.Min, "gravity.Min — страховка от M_min (§5.3)")
}

// ==================== T14: МЕЖСИСТЕМНЫЙ РАЗБРОС (БЮДЖЕТ M_диск) ====================

// T14 — системы с одинаковыми звёздными условиями (G, фиксированный набор
// орбит) различаются: p95/p5 системных медиан ≥ 2. Порог структурный
// (нижняя граница); якорь этапа 1 — факт ×19.6 (σ_ζ 0.6, спека
// протопылевого облака §8.1); рост перекрывает T22 (порог ≥ 10).
func TestSystemBudgetInterSystemSpread(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла, гоняется отдельно")
	}
	g := NewGenerator(nil, 20260914)
	const systems = 2000
	medians := make([]float64, 0, systems)
	buf := newBatchBuffers(16)
	for i := 0; i < systems; i++ {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 8, buf)
		var ms []float64
		for _, row := range buf.planetRows {
			fields := row.([]interface{})
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
			if data["is_gas_giant"] == true {
				continue
			}
			ms = append(ms, data["mass"].(float64))
		}
		if len(ms) == 0 {
			continue
		}
		medians = append(medians, medianOf(ms))
	}
	sort.Float64s(medians)
	p5 := percentile(medians, 0.05)
	p95 := percentile(medians, 0.95)
	t.Logf("системные медианы: p5=%.3f, p95=%.3f (×%.1f)", p5, p95, p95/p5)
	assert.GreaterOrEqual(t, p95/p5, 2.0, "межсистемный разброс p95/p5 ≥ 2")
}

// ==================== T15: ОДИН РОЛЛ M_диск НА СИСТЕМУ ====================

// T15 — бюджет облака M_диск ролится ровно один раз на систему (не в цикле
// орбит) и планеты системы имеют общий множитель.
func TestSystemBudgetSingleRollPerSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла, гоняется отдельно")
	}
	// (а) структурно: rollCloudBudget — ровно один нормальный ролл (поток
	// после него идентичен потоку после одного rng.NormFloat64()).
	a := NewGenerator(nil, 20260915)
	_ = a.rollCloudBudget()
	va := a.rng.Float64()
	b := NewGenerator(nil, 20260915)
	_ = b.rng.NormFloat64()
	vb := b.rng.Float64()
	assert.Equal(t, vb, va, "M_диск = exp(0.5·NormFloat64()) — ровно один нормальный ролл")

	// 0 планет — B не ролится.
	w := WorldInfo{
		ID: "w", Name: "W", SpectralClass: "O", StarType: "star",
		Mods: &models.StellarMods{Phase: "I", Subtype: "lbv"},
	}
	gz := NewGenerator(nil, 20260915)
	require.Zero(t, gz.generateWorldWithCountIntoBuffer(w, 0, newBatchBuffers(1)))
	vgz := gz.rng.Float64()
	hz := NewGenerator(nil, 20260915)
	assert.Equal(t, hz.rng.Float64(), vgz, "0 планет — ролл B не срабатывает")

	// (б) поведенчески: массы орбит 1 и 2 системы растут вместе (общий
	// M_диск). Корреляция — в лог-пространстве (при σ_ζ 0.6 линейная
	// занижена); пары с обрезкой гигантом-соседом исключаются (f_обр —
	// ступень, ломает лог-линию). Теоретическое ожидание (σ_M 0.5, σ_ζ 0.6)
	// = 0.25/(0.25+0.36) ≈ 0.41, а не 0.50 спеки §8.1. Замер (сид 20260916,
	// 15 прогонов; поток гуляет из-за порядка итерации map, PITFALLS):
	// 0.367–0.417, ср. 0.399; другие сиды — 0.38–0.44 (ревьюер — 0.384).
	// Порог 0.30 оставляет запас ≥ 0.067 к худшему замеру и дискриминирует:
	// без общего множителя (свой бюджет на орбиту) r ≈ 0 (n ≈ 2350 → > 10σ).
	g2 := NewGenerator(nil, 20260916)
	var a1, a2 []float64
	buf := newBatchBuffers(8)
	for i := 0; i < 2500; i++ {
		buf.reset()
		g2.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 6, buf)
		m := map[int]float64{}
		for _, row := range buf.planetRows {
			fields := row.([]interface{})
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
			if data["is_gas_giant"] == true {
				continue
			}
			orbit, ok := fields[3].(int)
			require.True(t, ok)
			m[orbit] = data["mass"].(float64)
		}
		x, ok1 := m[1]
		y, ok2 := m[2]
		if !ok1 || !ok2 {
			continue
		}
		if x == 8.0 || y == 8.0 || x == 0.02 || y == 0.02 {
			continue // кламп ломает пропорциональность — вне корреляции
		}
		if retentionFactor(1, g2.giantOrbit) != 1 || retentionFactor(2, g2.giantOrbit) != 1 {
			continue // пары с обрезкой — ступень f_обр вне корреляции
		}
		a1 = append(a1, math.Log(x))
		a2 = append(a2, math.Log(y))
	}
	require.Greater(t, len(a1), 500, "выборка орбит 1–2 достаточна")
	r := pearson(a1, a2)
	t.Logf("корреляция ln масс орбит 1 и 2: r=%.3f (ожидание ~0.41)", r)
	assert.Greater(t, r, 0.30, "планеты системы имеют общий множитель M_диск")
}

// ==================== T16: B НЕ ПРИМЕНЯЕТСЯ К ГИГАНТАМ/ЭКЗОТИКЕ ====================

// T16 — масса газовых гигантов (15.9–4131, эталон 99.2.15) и экзотики
// (0.05–0.45, своё происхождение) не зависит от M_диск (cloudBudget).
func TestSystemBudgetNotAppliedToGiantsAndExotic(t *testing.T) {
	giantMass := func(budget float64) float64 {
		g := NewGenerator(nil, 777)
		g.cloudBudget = budget
		res := g.runCascadeGiant(cascadeInput{
			Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, Metallicity: 0, TEff: 5772,
			OrbitRadiusAU: 5.2, OrbitIndex: 5,
		})
		return res.Mass
	}
	assert.Equal(t, giantMass(1), giantMass(100), "масса гиганта не зависит от B")

	exoticMass := func(budget float64) float64 {
		g := NewGenerator(nil, 778)
		g.cloudBudget = budget
		p := g.generateExoticPlanet(WorldInfo{ID: "w", Name: "W", StarType: "white_dwarf"}, 5)
		require.NotNil(t, p)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(p.Data, &data))
		return data["mass"].(float64)
	}
	m1, m2 := exoticMass(1), exoticMass(100)
	assert.Equal(t, m1, m2, "масса экзотики не зависит от B")
	assert.GreaterOrEqual(t, m1, 0.05)
	assert.Less(t, m1, 0.45)
}

// ==================== C1–C5: ПРОТОПЫЛЕВОЕ ОБЛАКО (§8.2) ====================
//
// C1 — TestProtoDiskSingleRollPerSystem — покрыт T15(а): поток после
// rollCloudBudget идентичен потоку после одного rng.NormFloat64() (ровно
// один нормальный ролл на систему); отдельный счётчик не дублируется.
// C3 — TestProtoDiskNotAppliedToGiantsAndExotic — покрыт T16
// (TestSystemBudgetNotAppliedToGiantsAndExotic): масса гигантов (15.9–4131)
// и экзотики (0.05–0.45) не зависит от M_диск (cloudBudget).

// C2 — медиана M_диск = 1: калибровка орбиты 2 (1 M⊕) не сдвигается
// (M_диск ~ logN(0, 0.5), медиана exp(0) = 1; спека §8.2).
func TestProtoDiskBudgetMedianNeutral(t *testing.T) {
	g := NewGenerator(nil, 20260934)
	const n = 20000
	vals := make([]float64, n)
	for i := range vals {
		vals[i] = g.rollCloudBudget()
	}
	med := medianOf(vals)
	t.Logf("медиана M_диск = %.4f (n=%d, ожидание 1.0)", med, n)
	assert.InDelta(t, 1.0, med, 0.03, "медиана M_диск = 1 — калибровка орбиты 2 не сдвигается")
}

// C4 — один M_диск на мир: P-планета тесной двойной — та же ветка того же
// мира (planet_data.go), своего бюджета не роллит. Мир роллит M_диск ровно
// один раз — первым роллом потока (isCircumbinary → giantOrbit без ролла);
// кросс-мировой шаринг не проверяется (открытый вопрос §12 п.8).
func TestProtoDiskSingleBudgetPerWorld(t *testing.T) {
	const seed = 20260933
	w := WorldInfo{
		ID: "w", Name: "W", SpectralClass: "G", StarType: "star",
		SystemType: "binary",
		Mods: &models.StellarMods{
			BinaryType: "close", Companion: "K", CompanionSepAU: f64(0.2),
		},
	}
	// (а) мир роллит M_диск ровно один раз — первым роллом потока.
	expected := NewGenerator(nil, seed).rollCloudBudget()
	g := NewGenerator(nil, seed)
	require.Equal(t, 1, g.generateWorldWithCountIntoBuffer(w, 1, newBatchBuffers(1)))
	assert.Equal(t, expected, g.cloudBudget, "P-мир: M_диск — первый и единственный ролл")
	require.NotNil(t, g.generateCircumbinaryPlanet(w))
	assert.Equal(t, expected, g.cloudBudget, "P-планета не перероллила M_диск (бюджет мира)")

	// (б) P-планета читает бюджет мира: при том же seed масса
	// масштабируется им (f_обр = 1, giantOrbit = 0), своего ролла нет.
	pMass := func(budget float64) float64 {
		pg := NewGenerator(nil, 1)
		pg.cloudBudget = budget
		p := pg.generateCircumbinaryPlanet(w)
		require.NotNil(t, p)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(p.Data, &data))
		require.NotEqual(t, true, data["is_gas_giant"], "сид 1 — каменистая P-планета")
		return data["mass"].(float64)
	}
	m1, m2 := pMass(0.5), pMass(1.0)
	t.Logf("P-масса: M_диск=0.5 → %.4f, M_диск=1.0 → %.4f (×%.2f)", m1, m2, m2/m1)
	assert.InDelta(t, 2.0, m2/m1, 0.02, "масса P-планеты пропорциональна M_диск мира")
}

// C5 — детерминизм: один seed → одна последовательность M_диск; значение —
// чистая функция потока. Значения planet.data целиком не детерминированы
// между прогонами одного seed (порядок итерации map) — здесь проверяется
// поток RNG, а не data (PITFALLS «Дизайн и числа»).
func TestProtoDiskDeterminism(t *testing.T) {
	seq := func() []float64 {
		g := NewGenerator(nil, 4242)
		out := make([]float64, 50)
		for i := range out {
			out[i] = g.rollCloudBudget()
		}
		return out
	}
	assert.Equal(t, seq(), seq(), "один seed → одна последовательность M_диск")

	// Чистая функция потока: значение не зависит от того, что роллится после
	// него (тот же seed → тот же бюджет в своей позиции).
	first := func(consumeAfter bool) float64 {
		g := NewGenerator(nil, 777)
		v := g.rollCloudBudget()
		if consumeAfter {
			_ = g.rng.NormFloat64()
			_ = g.rng.Float64()
		}
		return v
	}
	assert.Equal(t, first(false), first(true), "M_диск — функция своей позиции потока")
}

// ==================== T6/T7/T8: ЧИСТОЕ ЯДРО (без RNG) ====================

// T6 — якорь Земли: accretionMass(1.156, 0, 1) = 1.00 ± 1% (чистое ядро без
// RNG и без B; a_⊕ = a₂ = 1.156 а.е.).
func TestAccretionMassEarthAnchor(t *testing.T) {
	assert.InDelta(t, 1.0, accretionMass(1.156, 0, 1), 0.01, "орбита-якорь = 1 M⊕")
}

// T7 — границы: кламп §4 применяется ОДИН раз — к произведению B·M_ядро
// (ревью 2026-09-21, правка 1: внутренний кламп ядра убран). Ядро возвращает
// сырое значение, результат каскада клампится в [0.02, 8].
func TestAccretionMassBounds(t *testing.T) {
	// Ядро сырое: i=8 при ζ=+3σ превышает потолок — кламп стоит снаружи.
	raw := accretionMass(orbitRadiusByIndex(8), 0, math.Exp(1.2))
	assert.Greater(t, raw, massMax, "ядро не клампится внутри")

	// Результат каскада: при достаточных входах (большой B) масса упирается
	// в потолок — кламп на произведении, а не на ядре.
	hi := NewGenerator(nil, 20260927)
	hi.cloudBudget = 100
	resHi := hi.runCascade(cascadeInput{
		Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, Metallicity: 0, TEff: 5772,
		OrbitRadiusAU: orbitRadiusByIndex(8), OrbitIndex: 8,
	})
	assert.Equal(t, massMax, resHi.Mass, "i=8, B большой → потолок на произведении")

	// Штатный сторож не задевается: i=1 при ζ=−3σ (B = 1) выше пола.
	lo := clamp(accretionMass(orbitRadiusByIndex(1), 0, math.Exp(-1.2)), massMin, massMax)
	assert.Greater(t, lo, massMin, "i=1, ζ=−3σ > пол")

	// 5000 роллов произведений — в границах, без NaN/Inf (бюджет как в игре).
	g := NewGenerator(nil, 20260927)
	for i := 0; i < 5000; i++ {
		aNorm := 0.1 + g.rng.Float64()*40
		met := -0.8 + g.rng.Float64()*1.3
		zeta := math.Exp(0.6 * g.rng.NormFloat64())
		budget := g.rollCloudBudget()
		m := clamp(budget*accretionMass(aNorm, met, zeta), massMin, massMax)
		require.False(t, math.IsNaN(m) || math.IsInf(m, 0), "NaN/Inf")
		require.GreaterOrEqual(t, m, massMin)
		require.LessOrEqual(t, m, massMax)
	}
}

// T8 — детерминизм: ядро — чистая функция; один seed → одна последовательность.
func TestAccretionMassDeterminism(t *testing.T) {
	const aNorm, met, zeta = 2.5, -0.3, 1.7
	for i := 0; i < 100; i++ {
		assert.Equal(t, accretionMass(aNorm, met, zeta), accretionMass(aNorm, met, zeta), "чистая функция")
	}
	seq := func() []float64 {
		g := NewGenerator(nil, 4242)
		out := make([]float64, 50)
		for i := range out {
			out[i] = accretionMass(orbitRadiusByIndex(1+i%8), 0, math.Exp(0.6*g.rng.NormFloat64()))
		}
		return out
	}
	assert.Equal(t, seq(), seq(), "один seed → одна последовательность")
}

// ==================== T12: ЕДИНЫЙ ИСТОЧНИК НОМИНАЛА ПОДКРУТКИ (K1) ====================

// T12 — номинал подкрутки рас = ядро каскада без ζ, M_диск, f_обр (±1%) для
// M/K/G/F/A и орбит 1–2 (race_tuning.go — второй потребитель формулы, §5.4).
func TestRaceTuningNominalMassMatchesCascade(t *testing.T) {
	for _, cls := range []string{"M", "K", "G", "F", "A"} {
		for _, orbit := range []int{1, 2} {
			g := NewGenerator(nil, 1)
			g.raceID = "ammonia"
			g.raceSoftness = 1
			sp := stellarParamsFromClass(cls, 0, g.rng)
			rNat := orbitRadiusScaled(orbit, sp.Luminosity)
			tune := g.raceTunePlanet(sp, rNat, true)
			require.NotNil(t, tune, "%s орб. %d: подкрутка применена", cls, orbit)

			rEff := rNat * tune.orbitMult
			aNorm := rEff / math.Sqrt(sp.Luminosity)
			want := coreMass(aNorm, sp.Metallicity)
			assert.InDelta(t, want, tune.nominalMass, want*0.01,
				"%s орб. %d: номинал = ядро каскада", cls, orbit)
			assert.InDelta(t, want, accretionMass(aNorm, sp.Metallicity, 1), want*0.01,
				"%s орб. %d: ядро каскада без ζ, M_диск, f_обр", cls, orbit)
		}
	}
}

// ==================== T17–T22: ОБРЕЗКА ГИГАНТОМ-СОСЕДОМ (f_обр) ====================

// T17 — retentionFactor(i, j) = 0.08 при j > 0 и |i − j| ≤ 2, иначе 1 —
// чистая детерминированная функция (роллов не добавляет, RNG не сдвигает).
func TestGiantTruncationStuntsNeighbours(t *testing.T) {
	cases := []struct {
		orbit, giant int
		want         float64
	}{
		{3, 5, massTruncationFactor}, {4, 5, massTruncationFactor},
		{5, 5, massTruncationFactor}, {6, 5, massTruncationFactor},
		{7, 5, massTruncationFactor},
		{2, 5, 1}, {1, 5, 1}, {8, 5, 1},
		{1, 0, 1}, {5, 0, 1},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, retentionFactor(c.orbit, c.giant),
			"retentionFactor(%d, %d)", c.orbit, c.giant)
	}
}

// T18 — обрезка у G и M одинакова: f_обр — функция номеров орбит, светимость
// не входит (самоподобие по √L сохранено).
func TestTruncationSelfSimilar(t *testing.T) {
	ratio := func(cls string, temp int) float64 {
		g := NewGenerator(nil, 20260926)
		sp := stellarParamsFromClass(cls, temp, g.rng)
		g.giantOrbit = 3 // орбита 3 обрезана (|3−3| = 0)
		cut := medianOf(cascadeMasses(g, sp, 3, 3000))
		g.giantOrbit = 0 // без гиганта — потенциал
		plain := medianOf(cascadeMasses(g, sp, 3, 3000))
		return cut / plain
	}
	rG := ratio("G", 5772)
	rM := ratio("M", 0)
	t.Logf("доля обрезки: G=%.4f, M=%.4f (ожидание %.2f)", rG, rM, massTruncationFactor)
	assert.InDelta(t, massTruncationFactor, rG, 0.002, "G: множитель обрезки")
	assert.InDelta(t, massTruncationFactor, rM, 0.002, "M: множитель обрезки")
	assert.InDelta(t, rG, rM, 0.002, "обрезка G и M одинакова (светимость не входит)")
}

// T19 — порядок внутри зоны обрезки сохранён: при гиганте на орбите 5
// медианы орбит 3/4/6/7 строго возрастают (f_обр — общий множитель зоны).
func TestTruncationKeepsTrend(t *testing.T) {
	g := NewGenerator(nil, 20260927)
	sp := stellarParamsFromClass("G", 5772, g.rng)
	g.giantOrbit = 5
	m3 := medianOf(cascadeMasses(g, sp, 3, 1500))
	m4 := medianOf(cascadeMasses(g, sp, 4, 1500))
	m6 := medianOf(cascadeMasses(g, sp, 6, 1500))
	m7 := medianOf(cascadeMasses(g, sp, 7, 1500))
	t.Logf("зона обрезки (гигант на 5): %.3f / %.3f / %.3f / %.3f (орб. 3/4/6/7)", m3, m4, m6, m7)
	assert.Less(t, m3, m4, "орб. 3 < 4")
	assert.Less(t, m4, m6, "орб. 4 < 6")
	assert.Less(t, m6, m7, "орб. 6 < 7")
}

// T20 — f_обр не применяется к гигантам, экзотике и MassOverride.
func TestTruncationNotAppliedToGiantsAndExotic(t *testing.T) {
	// Масса гиганта не читает giantOrbit/f_обр.
	giantMass := func(giantOrbit int) float64 {
		g := NewGenerator(nil, 777)
		g.giantOrbit = giantOrbit
		return g.runCascadeGiant(cascadeInput{
			Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, Metallicity: 0, TEff: 5772,
			OrbitRadiusAU: 5.2, OrbitIndex: 5,
		}).Mass
	}
	assert.Equal(t, giantMass(0), giantMass(5), "масса гиганта не зависит от f_обр")

	// Масса экзотики — своё происхождение, f_обр не читает.
	exoticMass := func(giantOrbit int) float64 {
		g := NewGenerator(nil, 779)
		g.giantOrbit = giantOrbit
		p := g.generateExoticPlanet(WorldInfo{ID: "w", Name: "W", StarType: "white_dwarf"}, 5)
		require.NotNil(t, p)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(p.Data, &data))
		return data["mass"].(float64)
	}
	assert.Equal(t, exoticMass(0), exoticMass(5), "масса экзотики не зависит от f_обр")

	// MassOverride идёт мимо блока массы — ни M_диск, ни f_обр.
	g := NewGenerator(nil, 780)
	g.giantOrbit = 2
	res := g.runCascade(cascadeInput{
		MassOverride: 3.0, Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, Metallicity: 0,
		TEff: 5772, OrbitRadiusAU: orbitRadiusByIndex(2), OrbitIndex: 2,
	})
	assert.Equal(t, 3.0, res.Mass, "MassOverride мимо f_обр")
}

// T21 — провалы массы стали обычными: доля пар M(i+2) < M(i)/8 ≥ 0.3%
// (факт этапа 1 — 0.87%, спека §8.2). Газовые гиганты исключены.
func TestMassDipsBecomeOrdinary(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла, гоняется отдельно")
	}
	g := NewGenerator(nil, 20260931)
	const systems = 5000
	dips, pairs := 0, 0
	buf := newBatchBuffers(16)
	for i := 0; i < systems; i++ {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 6, buf)
		m := map[int]float64{}
		for _, row := range buf.planetRows {
			fields := row.([]interface{})
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
			if data["is_gas_giant"] == true {
				continue
			}
			orbit, ok := fields[3].(int)
			require.True(t, ok)
			m[orbit] = data["mass"].(float64)
		}
		for o := 1; o+2 <= 6; o++ {
			x, ok1 := m[o]
			y, ok2 := m[o+2]
			if !ok1 || !ok2 {
				continue
			}
			pairs++
			if y < x/8 {
				dips++
			}
		}
	}
	require.Greater(t, pairs, 5000, "выборка пар достаточна")
	frac := float64(dips) / float64(pairs)
	t.Logf("провалы M(i+2) < M(i)/8: %.2f%% (%d из %d, факт 0.87%%)", frac*100, dips, pairs)
	assert.GreaterOrEqual(t, frac, 0.003, "провалы стали обычными (≥ 0.3%%)")
}

// T22 — межсистемный разброс вырос: p95/p5 системных медиан ≥ 10
// (факт этапа 1 — ×19.6, спека §8.2; ср. структурный T14 с порогом ≥ 2).
func TestSystemBudgetSpreadGrew(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла, гоняется отдельно")
	}
	g := NewGenerator(nil, 20260932)
	const systems = 4000
	medians := make([]float64, 0, systems)
	buf := newBatchBuffers(16)
	for i := 0; i < systems; i++ {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 8, buf)
		var ms []float64
		for _, row := range buf.planetRows {
			fields := row.([]interface{})
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))
			if data["is_gas_giant"] == true {
				continue
			}
			ms = append(ms, data["mass"].(float64))
		}
		if len(ms) == 0 {
			continue
		}
		medians = append(medians, medianOf(ms))
	}
	sort.Float64s(medians)
	p5 := percentile(medians, 0.05)
	p95 := percentile(medians, 0.95)
	t.Logf("системные медианы: p5=%.3f, p95=%.3f (×%.1f, факт ×19.6)", p5, p95, p95/p5)
	assert.GreaterOrEqual(t, p95/p5, 10.0, "межсистемный разброс p95/p5 ≥ 10")
}
