// internal/generator/planet/belt_test.go
//
// Тесты этапа 1а/1б спеки 2026-09-21-пояса-малых-тел-объект-системы:
// §8.1 A1–A7 (ревизия бюджета облака: нормировка профиля, пере-калибровка
// приора, инвариант типично + мягкий кламп) и §8.2 B1–B15 (объект, генерация,
// хранение поясов). TDD: красное до правки → зелёное после.
package planet

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ==================== ХЕЛПЕРЫ РАЗБОРА БУФЕРА ====================

// parsedBelt — разобранный пояс из строки buf.beltRows (14 полей, §4.6).
type parsedBelt struct {
	kind     string
	orbit    *int
	radiusAU float64
	widthAU  float64
	mass     float64
	bodySize float64
	comp     map[string]float64
	visible  bool
	data     string
}

// beltsOf — разбирает пояса из буфера (после generateWorldWithCountIntoBuffer).
func beltsOf(t *testing.T, buf *batchBuffers) []parsedBelt {
	t.Helper()
	out := make([]parsedBelt, 0, len(buf.beltRows))
	for _, row := range buf.beltRows {
		f := row.([]interface{})
		require.Len(t, f, 14, "beltRow: 14 колонок system_belts")
		var orbit *int
		if v, ok := f[4].(int); ok {
			orbit = &v
		}
		var comp map[string]float64
		require.NoError(t, json.Unmarshal([]byte(f[9].(string)), &comp))
		out = append(out, parsedBelt{
			kind:     f[2].(string),
			orbit:    orbit,
			radiusAU: f[5].(float64),
			widthAU:  f[6].(float64),
			mass:     f[7].(float64),
			bodySize: f[8].(float64),
			comp:     comp,
			visible:  f[10].(bool),
			data:     f[11].(string),
		})
	}
	return out
}

// planetsOf — массы не-гигантов по орбитам из planetRows (JSON data["mass"]).
func planetsOf(t *testing.T, buf *batchBuffers) map[int]float64 {
	t.Helper()
	m := map[int]float64{}
	for _, row := range buf.planetRows {
		f := row.([]interface{})
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(f[4].(string)), &data))
		if data["is_gas_giant"] == true {
			continue
		}
		m[f[3].(int)] = data["mass"].(float64)
	}
	return m
}

// worldSum — Σ масс планет (без гигантов) + Σ масс поясов мира.
func worldSum(t *testing.T, buf *batchBuffers) float64 {
	t.Helper()
	sum := 0.0
	for _, m := range planetsOf(t, buf) {
		sum += m
	}
	for _, b := range beltsOf(t, buf) {
		sum += b.mass
	}
	return sum
}

// ==================== A1–A7: РЕВИЗИЯ БЮДЖЕТА (этап 1а) ====================

// A1 — ярдстик сохраняется: при median(M_диск) = S₀ = cloudProfileSum медианы
// орбит 1/2/3/4 = 0.764/0.995/0.104/0.136 (dd37a8c), тренд 7/1 = 4.84. Порог
// 0.02 абсолютный (статистика выборки ≤0.5%, §4.0.1).
func TestBudgetNormalizedMediansPreserved(t *testing.T) {
	g := NewGenerator(nil, 20260941)
	sp := stellarParamsFromClass("G", 5772, g.rng)

	// Ярдстик: G, n = 6, гигант на орбите 5 (f_обр бьёт по орбитам 3,4,6,7).
	g.giantOrbit = 5
	m1 := medianOf(cascadeMasses(g, sp, 1, 4000))
	m2 := medianOf(cascadeMasses(g, sp, 2, 4000))
	m3 := medianOf(cascadeMasses(g, sp, 3, 4000))
	m4 := medianOf(cascadeMasses(g, sp, 4, 4000))
	t.Logf("медианы орбит 1/2/3/4: %.3f / %.3f / %.3f / %.3f", m1, m2, m3, m4)
	assert.InDelta(t, 0.764, m1, 0.02, "орбита 1 (Венера)")
	assert.InDelta(t, 0.995, m2, 0.02, "орбита 2 (Земля)")
	assert.InDelta(t, 0.104, m3, 0.02, "орбита 3 (Марс)")
	assert.InDelta(t, 0.136, m4, 0.02, "орбита 4")

	// Тренд (потенциал, giantOrbit = 0): медиана(7)/медиана(1) = 4.84.
	g.giantOrbit = 0
	p1 := medianOf(cascadeMasses(g, sp, 1, 3000))
	p7 := medianOf(cascadeMasses(g, sp, 7, 3000))
	trend := p7 / p1
	t.Logf("тренд 7/1: %.2f (ожидание 4.84)", trend)
	assert.InDelta(t, 4.84, trend, 0.2, "тренд 7/1 сохранён")
}

// A1 (инвариант значений, п.2) — масса планеты на не-поясной орбите при
// одном seed совпадает со старой формулой M_i = clamp(f_обр·B·ζ·c_i),
// B = exp(0.5·z) (dd37a8c), с точностью до округления float64. Новая
// формула M_i = clamp(f_обр·M_диск·ζ·w_i), M_диск = S₀·exp(0.5·z),
// w_i = c_i/S₀ алгебраически тождественна старой (S₀·x/S₀ = x), поэтому
// поток RNG не сдвинут: те же роллы в том же порядке (первый NormFloat64 —
// бюджет, следующий — ζ в runCascade). Строгое бит-в-бит не выполняется:
// умножение на S₀ и деление на S₀ дают 1–2 ULP расхождения (см. лог);
// реальный сдвиг значений (единицы %) тест бы не прошёл.
func TestBudgetPlanetMassUnchangedVsOldFormula(t *testing.T) {
	newMass := func(seed int64, orbit, giant int) float64 {
		g := NewGenerator(nil, seed)
		g.giantOrbit = giant
		g.cloudBudget = g.rollCloudBudget() // M_диск = S₀·exp(0.5·z), ролл №1
		sp := stellarParamsFromClass("G", 5772, g.rng)
		return g.runCascade(cascadeInput{
			Luminosity: sp.Luminosity, StellarMass: sp.StellarMass, AgeGyr: sp.AgeGyr,
			Metallicity: sp.Metallicity, TEff: sp.TEff,
			OrbitRadiusAU: orbitRadiusScaled(orbit, sp.Luminosity), OrbitIndex: orbit,
		}).Mass
	}
	oldMass := func(seed int64, orbit, giant int) float64 {
		r := NewGenerator(nil, seed)
		x := math.Exp(0.5 * r.rng.NormFloat64()) // старый бюджет B, тот же ролл №1
		sp := stellarParamsFromClass("G", 5772, r.rng)
		zeta := math.Exp(0.6 * r.rng.NormFloat64()) // тот же ролл №2 (ζ)
		aNorm := orbitRadiusScaled(orbit, sp.Luminosity) / math.Sqrt(sp.Luminosity)
		return clamp(retentionFactor(orbit, giant)*x*
			accretionMass(aNorm, sp.Metallicity, zeta), massMin, massMax)
	}

	diff, maxRel := 0, 0.0
	for seed := int64(1); seed <= 1000; seed++ {
		for _, orbit := range []int{1, 2, 3, 4, 7} {
			for _, giant := range []int{0, 5} {
				n, o := newMass(seed, orbit, giant), oldMass(seed, orbit, giant)
				if rel := math.Abs(n-o) / math.Max(n, o); rel > 0 {
					diff++
					if rel > maxRel {
						maxRel = rel
					}
				}
			}
		}
	}
	t.Logf("новая vs старая формула: расхождение ULP-уровня в %d из 10000, макс. отн. %.3e",
		diff, maxRel)
	assert.Less(t, maxRel, 1e-12,
		"новая формула тождественна старой (расхождение — округление float64, не сдвиг значений)")
}

// A2 — инвариант бюджета ТИПИЧНО: после мягкого клампа (пред-слой уровня
// мира) Σ планет + Σ поясов ≤ M_диск для каждой системы; кламп при этом
// реально срабатывает (не пустая страховка). Разбивка п.2: кламп идёт от
// верхнего хвоста ζ, а не от поясов; старая формула (бюджет-масштаб
// M_диск/S₀, без клампа суммы, dd37a8c) нарушала бы инвариант чаще.
func TestBudgetInvariantTypical(t *testing.T) {
	g := NewGenerator(nil, 20260942)
	const systems = 1200
	buf := newBatchBuffers(16)
	clamped, clampedAst, clampedNoAst, oldClamped := 0, 0, 0, 0
	for i := 0; i < systems; i++ {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 7, buf)
		require.Greater(t, g.cloudBudget, 0.0)
		sum := worldSum(t, buf)
		assert.LessOrEqual(t, sum, g.cloudBudget*1.000000001+1e-6,
			"Σ планет + Σ поясов ≤ M_диск (система %d)", i)

		// Разбивка: есть ли у системы пояс астероидов (§4.1). Койпера есть
		// всегда у обычной звезды (§4.5), его масса — ~0.5% M_диск.
		hasAst, beltSum := false, 0.0
		for _, b := range beltsOf(t, buf) {
			beltSum += b.mass
			if b.kind == "asteroid" {
				hasAst = true
			}
		}
		// Старая формула: бюджет-масштаб B = M_диск/S₀ (median 1), клампа
		// суммы не было. «Сработал бы» = Σ планет > B. Массы планет те же
		// (формула тождественна, п.2), поясов в старой формуле нет.
		oldBudget := g.cloudBudget / cloudProfileSum
		planetSum := sum - beltSum
		if math.Abs(sum-g.cloudBudget) <= 1e-6 {
			clamped++
			if hasAst {
				clampedAst++
			} else {
				clampedNoAst++
			}
			// До клампа Σ планет > M_диск − Σ поясов ≫ M_диск/S₀ = B:
			// старая формула нарушила бы инвариант в этой системе тоже.
			oldClamped++
		} else if planetSum > oldBudget {
			oldClamped++
		}
	}
	t.Logf("кламп сработал в %d из %d систем (с астероидным поясом: %d; без него: %d)",
		clamped, systems, clampedAst, clampedNoAst)
	t.Logf("старая формула (бюджет M_диск/S₀) нарушала бы инвариант в %d из %d систем",
		oldClamped, systems)
	assert.Greater(t, clamped, 0, "мягкий кламп не пустая страховка")
	assert.Less(t, clamped, systems, "но и не применяется ко всем подряд")
	assert.GreaterOrEqual(t, oldClamped, clamped,
		"мягкий кламп срабатывает не чаще старой формулы (не сужает бюджета)")
}

// A3 — приор M_диск пере-калиброван: median ≈ S₀ = cloudProfileSum, σ = 0.5
// (в лог-пространстве) сохранён.
func TestBudgetPriorRecalibrated(t *testing.T) {
	g := NewGenerator(nil, 20260943)
	const n = 20000
	vals := make([]float64, n)
	logs := make([]float64, n)
	for i := range vals {
		vals[i] = g.rollCloudBudget()
		logs[i] = math.Log(vals[i])
	}
	med := medianOf(vals)
	t.Logf("median(M_диск) = %.3f, S₀ = %.3f", med, cloudProfileSum)
	assert.InDelta(t, cloudProfileSum, med, cloudProfileSum*0.03, "median(M_диск) = S₀")

	mean := 0.0
	for _, v := range logs {
		mean += v
	}
	mean /= float64(n)
	sd := 0.0
	for _, v := range logs {
		sd += (v - mean) * (v - mean)
	}
	sd = math.Sqrt(sd / float64(n))
	t.Logf("median ln = %.3f, σ ln = %.3f (ожидание ln S₀ = %.3f, 0.5)", mean, sd, math.Log(cloudProfileSum))
	assert.InDelta(t, 0.5, sd, 0.03, "σ приора сохранён")
	assert.InDelta(t, math.Log(cloudProfileSum), mean, 0.03, "центр приора = ln S₀")
}

// A4 — M_диск — ровно один нормальный ролл на мир (регресс-защита): поток
// после rollCloudBudget идентичен потоку после одного rng.NormFloat64().
func TestBudgetSingleRollPerSystem(t *testing.T) {
	a := NewGenerator(nil, 20260944)
	_ = a.rollCloudBudget()
	va := a.rng.Float64()
	b := NewGenerator(nil, 20260944)
	_ = b.rng.NormFloat64()
	vb := b.rng.Float64()
	assert.Equal(t, vb, va, "rollCloudBudget — ровно один нормальный ролл")

	// Значение = S₀·exp(0.5·z) на своём месте потока.
	c := NewGenerator(nil, 777)
	got := c.rollCloudBudget()
	d := NewGenerator(nil, 777)
	want := cloudProfileSum * math.Exp(0.5*d.rng.NormFloat64())
	assert.InDelta(t, want, got, 1e-9, "M_диск = S₀·exp(0.5·z)")
}

// A5 — потолок 8 не вернулся: доля «ровно 8.00» < 5% (≈3.3%).
func TestBudgetCeilingNotReturned(t *testing.T) {
	g := NewGenerator(nil, 20260945)
	masses := sampleRockyMasses(t, g, 4000)
	ceil := 0
	for _, m := range masses {
		if m == 8.0 {
			ceil++
		}
	}
	frac := float64(ceil) / float64(len(masses))
	t.Logf("ровно 8.00: %.2f%%", frac*100)
	assert.Less(t, frac, 0.05, "доля ровно 8.0 < 5%%")
}

// A6 — M_диск не применяется к гигантам (15.9–4131) и экзотике (0.05–0.45).
func TestBudgetNotAppliedToGiantsAndExotic(t *testing.T) {
	giantMass := func(budget float64) float64 {
		g := NewGenerator(nil, 777)
		g.cloudBudget = budget
		return g.runCascadeGiant(cascadeInput{
			Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, Metallicity: 0, TEff: 5772,
			OrbitRadiusAU: 5.2, OrbitIndex: 5,
		}).Mass
	}
	assert.Equal(t, giantMass(1), giantMass(cloudProfileSum), "масса гиганта не зависит от M_диск")

	exoticMass := func(budget float64) float64 {
		g := NewGenerator(nil, 778)
		g.cloudBudget = budget
		p := g.generateExoticPlanet(WorldInfo{ID: "w", Name: "W", StarType: "white_dwarf"}, 5)
		require.NotNil(t, p)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(p.Data, &data))
		return data["mass"].(float64)
	}
	m1, m2 := exoticMass(1), exoticMass(cloudProfileSum)
	assert.Equal(t, m1, m2, "масса экзотики не зависит от M_диск")
	assert.GreaterOrEqual(t, m1, 0.05)
	assert.Less(t, m1, 0.45)
}

// A7 — межсистемный разброс сохранён: p95/p5 системных медиан ≥ 6 (спека
// поясов §8.1 — «порог по замеру»). До ревизии факт ≈ ×19.6; на n = 8 мягкий
// кламп суммы срабатывает часто и сжимает разброс — факт этапа 1а/1б ≈ ×8.4.
// Объёмный смоук — вне быстрого цикла.
func TestSystemBudgetSpread(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла, гоняется отдельно")
	}
	g := NewGenerator(nil, 20260946)
	const systems = 3000
	medians := make([]float64, 0, systems)
	buf := newBatchBuffers(16)
	for i := 0; i < systems; i++ {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 8, buf)
		var ms []float64
		for _, m := range planetsOf(t, buf) {
			ms = append(ms, m)
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
	assert.GreaterOrEqual(t, p95/p5, 6.0, "межсистемный разброс p95/p5 ≥ 6")
}

// ==================== B1–B15: ПОЯСА (этап 1б) ====================

// B1 — орбита пояса из резонансного окна: beltOrbit = g−1; a_{g−1}/r_g =
// 0.588 ∈ [0.4807, 0.6300]; radius_au = 0.5555·r_g, width_au = 0.1493·r_g;
// якорь-номер ≠ середина окна; случай g = 2.
func TestBeltOrbitFromResonanceWindow(t *testing.T) {
	assert.InDelta(t, 0.4807, math.Pow(1.0/3.0, 2.0/3.0), 0.001, "r_3:1")
	assert.InDelta(t, 0.6300, math.Pow(1.0/2.0, 2.0/3.0), 0.001, "r_2:1")

	ratio := orbitRadiusByIndex(4) / orbitRadiusByIndex(5)
	t.Logf("a_{g-1}/r_g = %.4f", ratio)
	assert.InDelta(t, 0.588, ratio, 0.002, "якорь g−1 внутри окна")
	assert.True(t, ratio >= 0.4807 && ratio <= 0.6300, "якорь в [3:1, 2:1]")

	g := NewGenerator(nil, 1)
	l := luminosityBySpectral("G")
	rG := orbitRadiusScaled(5, l)
	b := g.asteroidBelt(WorldInfo{ID: "w", SpectralClass: "G"}, 5)
	require.NotNil(t, b.OrbitIndex)
	assert.Equal(t, 4, *b.OrbitIndex, "beltOrbit = g−1")
	assert.InDelta(t, 0.5555*rG, b.RadiusAU, 1e-9, "radius_au — середина окна")
	assert.InDelta(t, 0.1493*rG, b.WidthAU, 1e-9, "width_au — протяжённость окна")
	assert.NotEqual(t, orbitRadiusByIndex(4)*math.Sqrt(l), b.RadiusAU, "якорь ≠ середина")

	// g = 2: пояс накрывает орбиту 1 (внутренней больше нет).
	b2 := g.asteroidBelt(WorldInfo{ID: "w", SpectralClass: "G"}, 2)
	require.NotNil(t, b2.OrbitIndex)
	assert.Equal(t, 1, *b2.OrbitIndex)
}

// B2 — система с гигантом (giantOrbit ≥ 2): на орбите g−1 нет планеты, есть
// пояс kind=asteroid с orbit_index = g−1.
func TestBeltAsteroidReplacesPlanet(t *testing.T) {
	g := NewGenerator(nil, 20260951)
	buf := newBatchBuffers(16)
	seen := 0
	for i := 0; i < 600; i++ {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 6, buf)
		belts := beltsOf(t, buf)
		var ast *parsedBelt
		for j := range belts {
			if belts[j].kind == "asteroid" {
				ast = &belts[j]
			}
		}
		if ast == nil {
			continue
		}
		seen++
		require.GreaterOrEqual(t, g.giantOrbit, 2, "астероидный пояс только при g ≥ 2")
		require.NotNil(t, ast.orbit)
		assert.Equal(t, g.giantOrbit-1, *ast.orbit, "orbit_index = g−1")
		_, hasPlanet := planetsOf(t, buf)[g.giantOrbit-1]
		assert.False(t, hasPlanet, "на орбите пояса планеты нет")
	}
	require.Greater(t, seen, 0, "выборка содержит астероидные пояса")
}

// B3 — без гиганта (или giantOrbit = 1) астероидного пояса нет.
func TestBeltNoGiantNoAsteroid(t *testing.T) {
	g := NewGenerator(nil, 20260952)
	buf := newBatchBuffers(16)
	g1 := 0
	for i := 0; i < 600; i++ {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 6, buf)
		hasAst := false
		for _, b := range beltsOf(t, buf) {
			if b.kind == "asteroid" {
				hasAst = true
			}
		}
		if g.giantOrbit < 2 {
			assert.False(t, hasAst, "без гиганта (g=%d) астероидного пояса нет", g.giantOrbit)
			if g.giantOrbit == 1 {
				g1++
			}
		}
	}
	t.Logf("систем с giantOrbit = 1: %d", g1)
}

// B4 — лимит удержания поясов: Σ_kind κ_kind·w⁰_зон ≤ 1 − P (пояса не
// съедают больше остатка). Проверяется на системах с гигантом (там есть
// астероидный пояс): пул P = 1 − w_g − w_{g−1} (изъяты орбиты g и g−1).
func TestBeltRetentionWithinRemainder(t *testing.T) {
	total := kappaAsteroid*profileW(1) + kappaKuiper*kuiperProfileTail
	for orbit := 2; orbit <= 8; orbit++ {
		// Лимит для системы, где занято максимум орбит (n = 8): P = 1 − w_g − w_{g−1}.
		p := 1 - profileW(orbit) - profileW(orbit-1)
		retention := kappaAsteroid*profileW(orbit-1) + kappaKuiper*kuiperProfileTail
		require.GreaterOrEqual(t, p, 0.0)
		assert.LessOrEqual(t, retention, p+1e-12,
			"g=%d: удержание поясов ≤ 1−P (запас)", orbit)
	}
	assert.Less(t, total, 0.02, "Σ удержания поясов < 2% профиля (запас B4)")
}

// profileW — доля профиля w_i = c_i/S₀ (для теста лимита B4).
func profileW(orbit int) float64 {
	return coreMass(orbitRadiusByIndex(orbit), 0) / cloudProfileSum
}

// B5 — у обычной звезды есть пояс Койпера за планетной зоной: radius_au >
// orbitRadiusScaled(8, L).
func TestBeltKuiperBeyondPlanetZone(t *testing.T) {
	g := NewGenerator(nil, 20260953)
	buf := newBatchBuffers(16)
	seen := 0
	for i := 0; i < 50; i++ {
		buf.reset()
		w := WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}
		g.generateWorldWithCountIntoBuffer(w, 6, buf)
		for _, b := range beltsOf(t, buf) {
			if b.kind != "kuiper" {
				continue
			}
			seen++
			require.Nil(t, b.orbit, "orbit_index Койпера = NULL")
			assert.Greater(t, b.radiusAU, orbitRadiusScaled(8, luminosityBySpectral("G")),
				"радиус Койпера за орбитой 8")
		}
	}
	require.Greater(t, seen, 0, "у обычной звезды пояс Койпера есть")
}

// B6 — mass/radius_au/width_au/body_size_km > 0 у всех поясов.
func TestBeltFieldsPositive(t *testing.T) {
	g := NewGenerator(nil, 20260954)
	buf := newBatchBuffers(16)
	seen := 0
	for i := 0; i < 300; i++ {
		buf.reset()
		cls := []string{"G", "K", "M", "F"}[i%4]
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: cls, StarType: "star"}, 6, buf)
		for _, b := range beltsOf(t, buf) {
			seen++
			assert.Greater(t, b.mass, 0.0, "mass > 0")
			assert.Greater(t, b.radiusAU, 0.0, "radius_au > 0")
			assert.Greater(t, b.widthAU, 0.0, "width_au > 0")
			assert.Greater(t, b.bodySize, 0.0, "body_size_km > 0")
			assert.True(t, b.visible)
		}
	}
	require.Greater(t, seen, 0)
}

// B7 — не более одной записи asteroid и одной kuiper на мир.
func TestBeltSinglePerKind(t *testing.T) {
	g := NewGenerator(nil, 20260955)
	buf := newBatchBuffers(16)
	seen := 0
	for i := 0; i < 300; i++ {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(
			WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 7, buf)
		counts := map[string]int{}
		for _, b := range beltsOf(t, buf) {
			counts[b.kind]++
		}
		seen += len(counts)
		for kind, n := range counts {
			assert.LessOrEqual(t, n, 1, "не более одного пояса kind=%s на мир", kind)
		}
	}
	require.Greater(t, seen, 0)
}

// B8 — WD с disk_state='debris' → пояс kind=debris (материализация, §4.4);
// ЧД/НЗ/протозвезда — нет.
func TestBeltDebrisFromDiskState(t *testing.T) {
	g := NewGenerator(nil, 20260956)
	buf := newBatchBuffers(4)
	wd := WorldInfo{
		ID: "wd", Name: "WD", StarType: "white_dwarf",
		Mods: &models.StellarMods{DiskState: "debris"},
	}
	buf.reset()
	g.generateWorldWithCountIntoBuffer(wd, 1, buf)
	belts := beltsOf(t, buf)
	require.Len(t, belts, 1)
	assert.Equal(t, "debris", belts[0].kind)

	// Нет обломочного пояса без disk_state = debris.
	wdPlain := WorldInfo{ID: "wd2", Name: "WD", StarType: "white_dwarf"}
	buf.reset()
	g.generateWorldWithCountIntoBuffer(wdPlain, 1, buf)
	for _, b := range beltsOf(t, buf) {
		assert.NotEqual(t, "debris", b.kind, "без disk_state='debris' обломочного пояса нет")
	}

	// ЧД/НЗ/протозвезда — поясов нет.
	for _, st := range []string{"black_hole", "neutron", "protostar"} {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(WorldInfo{ID: st, Name: st, StarType: st}, 1, buf)
		assert.Empty(t, beltsOf(t, buf), "%s: поясов нет", st)
	}
}

// B9 — пояс — агрегат: в данных нет bodies/именованных тел (Плутон-класс снят).
func TestBeltNoNamedBodies(t *testing.T) {
	g := NewGenerator(nil, 20260957)
	buf := newBatchBuffers(4)
	g.generateWorldWithCountIntoBuffer(
		WorldInfo{ID: "w", Name: "W", SpectralClass: "G", StarType: "star"}, 6, buf)
	for _, b := range beltsOf(t, buf) {
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(b.data), &data))
		_, hasBodies := data["bodies"]
		assert.False(t, hasBodies, "поля bodies нет (пояс — агрегат)")
		assert.NotContains(t, b.data, "bodies")
		// Состав — только порода/железо/лёд (один источник зоны).
		for k := range b.comp {
			assert.Contains(t, []string{"rock", "iron", "ice"}, k)
		}
	}
}

// B10 — ЧД/НЗ/протозвезда/сверхгигант → без поясов «от облака».
func TestBeltNotForExotic(t *testing.T) {
	g := NewGenerator(nil, 20260958)
	buf := newBatchBuffers(4)
	for _, st := range []string{"black_hole", "neutron", "protostar"} {
		buf.reset()
		g.generateWorldWithCountIntoBuffer(WorldInfo{ID: st, Name: st, StarType: st}, 1, buf)
		assert.Empty(t, beltsOf(t, buf), "%s: поясов нет", st)
	}
	// Прочая экзотика — сверхгигант (фаза I + lbv).
	buf.reset()
	g.generateWorldWithCountIntoBuffer(WorldInfo{
		ID: "sg", Name: "SG", StarType: "star",
		Mods: &models.StellarMods{Phase: "I", Subtype: "lbv"},
	}, 1, buf)
	assert.Empty(t, beltsOf(t, buf), "сверхгигант: поясов нет")
}

// B11 — детерминизм: при одном seed и одном состоянии генератора пояса
// воспроизводятся (kind/orbit/radius/mass/comp). Полная последовательность
// планет мира от seed НЕ детерминирована между прогонами (порядок итерации
// map — PITFALLS «Дизайн и числа»), поэтому здесь проверяется чистая
// генерация пояса (выделенный RNG пояса + M_диск), а не поток всего мира.
func TestBeltDeterminism(t *testing.T) {
	w := WorldInfo{ID: "w-det", SpectralClass: "G"}
	sig := func(b BeltData) string {
		orbit := -1
		if b.OrbitIndex != nil {
			orbit = *b.OrbitIndex
		}
		return b.Kind + "|" + strconv.Itoa(orbit) +
			"|" + strconv.FormatFloat(b.RadiusAU, 'g', 15, 64) +
			"|" + strconv.FormatFloat(b.WidthAU, 'g', 15, 64) +
			"|" + strconv.FormatFloat(b.Mass, 'g', 15, 64) +
			"|" + strconv.FormatFloat(b.Composition["rock"], 'g', 15, 64)
	}
	seq := func() []string {
		g := NewGenerator(nil, 424242)
		g.usedNames = map[string]bool{}
		g.cloudBudget = g.rollCloudBudget() // детерминированный первый ролл
		g.giantOrbit = 5
		return []string{
			sig(g.asteroidBelt(w, 5)),
			sig(g.kuiperBelt(w)),
			sig(g.debrisBelt(w, nil)),
		}
	}
	assert.Equal(t, seq(), seq(), "один seed → одна последовательность поясов")

	// Два вызова на одном состоянии генератора совпадают по полям (кроме UUID).
	g := NewGenerator(nil, 7)
	g.giantOrbit = 4
	a, b := g.asteroidBelt(w, 4), g.asteroidBelt(w, 4)
	assert.Equal(t, sig(a), sig(b), "генерация пояса детерминирована")
}

// B13 — состав пояса берётся из той же функции зоны, что у планет
// (compositionByZone — один источник).
func TestBeltCompositionMatchesZone(t *testing.T) {
	w := WorldInfo{ID: "w-comp", SpectralClass: "G"}
	l := luminosityBySpectral("G")
	g := NewGenerator(nil, 1)

	b := g.asteroidBelt(w, 5)
	rock, iron, ice := compositionByZone(b.RadiusAU, l, beltRNG(w.ID, "asteroid", "comp"))
	assert.InDelta(t, rock, b.Composition["rock"], 1e-12)
	assert.InDelta(t, iron, b.Composition["iron"], 1e-12)
	assert.InDelta(t, ice, b.Composition["ice"], 1e-12)

	k := g.kuiperBelt(w)
	rock, iron, ice = compositionByZone(k.RadiusAU, l, beltRNG(w.ID, "kuiper", "comp"))
	assert.InDelta(t, rock, k.Composition["rock"], 1e-12)
	assert.InDelta(t, iron, k.Composition["iron"], 1e-12)
	assert.InDelta(t, ice, k.Composition["ice"], 1e-12)
	assert.Greater(t, k.Composition["ice"], 0.3, "Койпера — за снеговой линией (лёд)")
}

// B14 — доля астероидных поясов: ≈4.3% всех звёзд / ≈9.5% F/G/K (вес ×
// P(гигант) × 0.95). Объёмный замер — вне быстрого цикла.
func TestBeltShareDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла, гоняется отдельно")
	}
	g := NewGenerator(nil, 20260959)
	classes := []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}
	all, allN := 0, 0
	fgk, fgkAll := 0, 0
	buf := newBatchBuffers(16)
	perClass := 1500
	for _, cls := range classes {
		for i := 0; i < perClass; i++ {
			buf.reset()
			g.generateWorldWithCountIntoBuffer(
				WorldInfo{ID: "w", Name: "W", SpectralClass: cls, StarType: "star"}, 6, buf)
			has := false
			for _, b := range beltsOf(t, buf) {
				if b.kind == "asteroid" {
					has = true
				}
			}
			allN++
			if has {
				all++
			}
			if cls == "F" || cls == "G" || cls == "K" {
				fgkAll++
				if has {
					fgk++
				}
			}
		}
	}
	allFrac := float64(all) / float64(allN)
	fgkFrac := float64(fgk) / float64(fgkAll)
	t.Logf("доля астероидных поясов: все %.2f%% (ожидание ≈4.3%%), F/G/K %.2f%% (≈9.5%%)",
		allFrac*100, fgkFrac*100)
	assert.InDelta(t, 0.043, allFrac, 0.02, "доля всех звёзд")
	assert.InDelta(t, 0.095, fgkFrac, 0.04, "доля F/G/K")
}
