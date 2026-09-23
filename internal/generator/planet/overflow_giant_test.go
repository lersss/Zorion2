// internal/generator/planet/overflow_giant_test.go
//
// Тесты перелива массы в гигантов и мини-нептуны (спека
// 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны §12.1, O1–O11, O14,
// O19–O23): двухпроходный пред-слой мира, порог убегающей аккреции,
// миграция/горячие юпитеры, честная аккреция массы гиганта, пояса/регионы.
package planet

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/regionprofile"
)

// ==================== ХЕЛПЕРЫ ====================

// orbitData — данные планет мира по орбитам (включая гигантов/мини-нептунов).
func orbitData(t *testing.T, buf *batchBuffers) map[int]map[string]interface{} {
	t.Helper()
	out := map[int]map[string]interface{}{}
	for _, row := range buf.planetRows {
		f := row.([]interface{})
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(f[4].(string)), &data))
		out[f[3].(int)] = data
	}
	return out
}

// runOverflowWorld — один мир обычной звезды класса cls (count из mean-модели).
func runOverflowWorld(g *Generator, buf *batchBuffers, cls string) int {
	count := g.determinePlanetCount(cls)
	buf.reset()
	g.generateWorldWithCountIntoBuffer(
		WorldInfo{ID: "w", Name: "W", SpectralClass: cls, StarType: "star"}, count, buf)
	return count
}

// overflowRate — доля миров класса cls с переполнением порога (§10.2: P_ov).
func overflowRate(g *Generator, buf *batchBuffers, cls string, n int) float64 {
	ov := 0
	for i := 0; i < n; i++ {
		runOverflowWorld(g, buf, cls)
		if g.overflow.birthOrbit > 0 {
			ov++
		}
	}
	return float64(ov) / float64(n)
}

// ==================== O1: ЦИКЛ РАЗОРВАН (§4.3) ====================

// O1 — g₀ зависит только от (M_диск, ζ, M_crit): при фиксированных M_диск и ζ
// смена M_crit меняет g₀, но сырые ядра M_i⁰ не меняются (проход 1 не знает о
// гиганте — цикл K1 разорван).
func TestOverflowNoCycle(t *testing.T) {
	changed, overflowed := 0, 0
	for seed := int64(1); seed <= 1500; seed++ {
		g1 := NewGenerator(nil, seed)
		g1.cloudBudget = cloudProfileSum
		p1 := g1.rollOverflow(6, 0, massMax)
		g2 := NewGenerator(nil, seed)
		g2.cloudBudget = cloudProfileSum
		p2 := g2.rollOverflow(6, 0, massMax/2)

		assert.Equal(t, p1.coreMass, p2.coreMass, "M_i⁰ не зависят от M_crit (сид %d)", seed)
		if p1.birthOrbit > 0 {
			require.NotZero(t, p2.birthOrbit, "ниже порог не отменяет переполнение (сид %d)", seed)
			assert.LessOrEqual(t, p2.birthOrbit, p1.birthOrbit,
				"ниже порог — орбита рождения не дальше (сид %d)", seed)
			overflowed++
		}
		if p1.birthOrbit != p2.birthOrbit {
			changed++
		}
	}
	require.Greater(t, overflowed, 60, "переполнения в выборке достаточно")
	require.Greater(t, changed, 20, "M_crit меняет g₀")
}

// ==================== O2: ПОЛКИ НА M_CRIT НЕТ (§4.2) ====================

// O2 — доля тел «ровно M_crit» ≈ 0: все тела с ядром > M_crit уходят в ветку
// оболочки (кламп «ровно M_crit» снят); брат с ядром ≤ 16 — мини-нептун, с
// ядром > 16 — дополнительный гигант; пайка на 16 нет.
func TestOverflowNoPileupAtMcrit(t *testing.T) {
	g := NewGenerator(nil, 20260924)
	buf := newBatchBuffers(16)
	const worlds = 3000
	atMcrit, total, mini, giants := 0, 0, 0, 0
	for i := 0; i < worlds; i++ {
		runOverflowWorld(g, buf, "G")
		for _, d := range orbitData(t, buf) {
			m := d["mass"].(float64)
			total++
			if m == massMax {
				atMcrit++
			}
			if d["is_mini_neptune"] == true {
				mini++
			}
			if d["is_gas_giant"] == true {
				giants++
			}
		}
		// Класс тела согласован с ядром и массой (§7.1).
		for o := 1; o <= overflowOrbitMax; o++ {
			switch g.overflow.class[o] {
			case bodyMiniNeptune:
				assert.LessOrEqual(t, g.overflow.bodyMass[o], GasGiantMassMin,
					"мини-нептун ≤ 16 (орбита %d)", o)
			case bodyGiant:
				assert.Greater(t, g.overflow.bodyMass[o], GasGiantMassMin,
					"гигант > 16 (орбита %d)", o)
			}
		}
	}
	share := float64(atMcrit) / float64(total)
	t.Logf("ровно M_crit: %.4f%%; мини-нептуны: %d, гиганты: %d", share*100, mini, giants)
	assert.Less(t, share, 0.001, "полки «ровно M_crit» нет (ветка оболочки забирает всё)")
	assert.Greater(t, mini, 0, "мини-нептуны встречаются")
	assert.Greater(t, giants, 0, "гиганты встречаются")
}

// ==================== O3: ЧАСТОТА ПО ЧИСЛУ ОРБИТ (§10.2) ====================

// O3 — P(система с переполнением) растёт с n: G/F > K/M; A/O/B/L/T/Y ≈ 0.
// Классовая таблица 10/3/2/0.5% снята (решение создателя 2026-09-23 п. 7):
// частота физически зависит от числа орбит, а не от класса звезды.
func TestOverflowRateByPlanetCount(t *testing.T) {
	g := NewGenerator(nil, 42)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(16)
	const n = 20000

	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла")
	}
	rateG := overflowRate(g, buf, "G", n)
	rateK := overflowRate(g, buf, "K", n)
	rateM := overflowRate(g, buf, "M", n)
	rateL := overflowRate(g, buf, "L", n)
	rateO := overflowRate(g, buf, "O", n)
	t.Logf("P_ov: G=%.2f%% K=%.2f%% M=%.2f%% L=%.2f%% O=%.2f%%",
		rateG*100, rateK*100, rateM*100, rateL*100, rateO*100)

	assert.InDelta(t, 0.20, rateG, 0.05, "G/F: P_ov ≈ 20%% (§10.7: 20.3%%)")
	assert.InDelta(t, 0.11, rateM, 0.05, "K/M: P_ov ≈ 11%%")
	assert.Less(t, rateL, 0.03, "n ≡ 1: P_ov ≈ 0")
	assert.Less(t, rateO, 0.03, "O: n ∈ {0,1}: P_ov ≈ 0")
	// Вилка по n, не по классу: у K и M одно n — одна частота.
	assert.InDelta(t, rateK, rateM, 0.04, "K и M: одна вилка по n")
	assert.Greater(t, rateG, rateM, "G/F чаще K/M (больше орбит)")
}

// ==================== O4: ОРБИТА РОЖДЕНИЯ — ВНУТРЕННЯЯ ГРАНИЦА (§4.2) ====================

// O4 — g₀ = min{i : M_i⁰ > M_crit}; при росте M_диск g₀ не растёт (уезжает
// внутрь: в массивном диске порог достигается ближе к звезде).
func TestOverflowBirthOrbitInnerBoundary(t *testing.T) {
	// Чистая часть: первое превышение порога по возрастающему профилю.
	cores := [overflowOrbitMax + 1]float64{}
	cores[1], cores[2], cores[3], cores[4], cores[5] = 1, 2, 3, 30, 40
	assert.Equal(t, 4, overflowBirthOrbit(cores, 5, massMax), "min{i : M_i⁰ > M_crit}")

	// Мировой поток: тот же seed и та же последовательность ζ → рост бюджета
	// двигает орбиту рождения внутрь (монотонность по каждому миру).
	low, high := cloudProfileSum*math.Exp(-0.3), cloudProfileSum*math.Exp(0.9)
	monotone, moved := 0, 0
	for seed := int64(1); seed <= 6000 && monotone < 500; seed++ {
		g1 := NewGenerator(nil, seed)
		g1.cloudBudget = low
		p1 := g1.rollOverflow(7, 0, massMax)
		if p1.birthOrbit == 0 {
			continue
		}
		g2 := NewGenerator(nil, seed)
		g2.cloudBudget = high
		p2 := g2.rollOverflow(7, 0, massMax)
		require.NotZero(t, p2.birthOrbit, "больший бюджет не отменяет переполнение")
		assert.LessOrEqual(t, p2.birthOrbit, p1.birthOrbit,
			"рост M_диск не уводит g₀ наружу (сид %d: %d → %d)", seed, p1.birthOrbit, p2.birthOrbit)
		monotone++
		if p2.birthOrbit < p1.birthOrbit {
			moved++
		}
	}
	require.Greater(t, monotone, 100, "выборка миров с переполнением достаточна")
	assert.Greater(t, moved, 0, "есть миры, где g₀ сдвинулась внутрь")
}

// ==================== O5: ОДИН МИГРИРУЮЩИЙ ГИГАНТ (§4.2, §7.2) ====================

// O5 — ровно один мигрирующий g на систему (min + одна миграция); вторичные
// тела с ядром > 16 дают дополнительные гиганты на своих орбитах рождения —
// инвариант ослаблен до «один МИГРИРУЮЩИЙ гигант».
func TestOverflowSingleGiantPerSystem(t *testing.T) {
	g := NewGenerator(nil, 20260925)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(16)
	const worlds = 4000
	maxGiants, withSecondary := 0, 0
	for i := 0; i < worlds; i++ {
		runOverflowWorld(g, buf, "G")
		p := g.overflow
		rows := orbitData(t, buf)
		// Гигант мира — ровно на своей орбите (один g на систему).
		if p.giantOrbit > 0 {
			require.NotNil(t, rows[p.giantOrbit], "гигант стоит на орбите g")
			assert.Equal(t, true, rows[p.giantOrbit]["is_gas_giant"], "на g — газовый гигант")
			assert.Equal(t, bodyGiant, p.class[p.giantOrbit], "класс на g — гигант")
		}
		// Гигантов не может быть больше, чем тел класса bodyGiant; вторичные
		// тела > 16 дают дополнительные гиганты на своих орбитах (§7.2).
		cnt := 0
		for o, d := range rows {
			if d["is_gas_giant"] == true {
				cnt++
				assert.Equal(t, bodyGiant, p.class[o], "гигант — только у тела класса bodyGiant")
			}
			if p.class[o] == bodyGiant {
				assert.Equal(t, true, d["is_gas_giant"], "тело класса bodyGiant — гигант")
			}
		}
		if cnt > maxGiants {
			maxGiants = cnt
		}
		if cnt > 1 {
			withSecondary++
		}
	}
	t.Logf("максимум гигантов в системе: %d; систем с вторичными гигантами: %d", maxGiants, withSecondary)
	assert.Greater(t, maxGiants, 0, "гиганты встречаются")
	assert.Greater(t, withSecondary, 0, "вторичные гиганты разрешены (§7.2)")
}

// ==================== O6: СЧЁТ РОЛЛОВ ПРЕД-СЛОЯ (§4.4) ====================

// O6 — поток пред-слоя НЕ ветвится (§4.4): число и порядок роллов не зависят
// ни от числа орбит сверх n роллов ζ, ни от исхода (переполнение/миграция/
// попадание в верхний предел M_gas). Мир: M_диск (1 нормальный), M_gas
// (1 нормальный), migrationMode (1 равномерный); пред-слой: n ζ + режим u_gas
// + решение о миграции + орбита миграции (безусловные) + добор M_gas
// (gasReservoirCandidates − 1 нормальных, всегда) = n + 3 + доборы.
// Поток после пред-слоя равен эталону, собранному из тех же вызовов rand —
// для миров с разным исходом (проверяется отдельно: выборка обязана содержать
// и мигрировавшие, и не мигрировавшие системы).
func TestOverflowNoEntropyInLoop(t *testing.T) {
	const count = 6

	pipeline := func(seed int64) overflowPlan {
		g := NewGenerator(nil, seed)
		g.cloudBudget = g.rollCloudBudget()
		g.gasReservoir = g.rollGasReservoir()
		g.migrationMode = g.rollMigrationMode()
		return g.rollOverflow(count, 0, massMax)
	}
	after := func(seed int64) float64 {
		g := NewGenerator(nil, seed)
		g.cloudBudget = g.rollCloudBudget()
		g.gasReservoir = g.rollGasReservoir()
		g.migrationMode = g.rollMigrationMode()
		g.rollOverflow(count, 0, massMax)
		return g.rng.Float64()
	}
	// Эталон: те же вызовы rand.Rand в том же порядке, без ветвлений.
	ref := func(seed int64) float64 {
		g := NewGenerator(nil, seed)
		g.rng.NormFloat64()  // M_диск
		g.rng.NormFloat64()  // M_gas (ролл мира)
		g.rng.Float64()      // migrationMode
		for i := 0; i < count; i++ {
			g.rng.NormFloat64() // ζ_i
		}
		g.rng.Float64() // режим u_gas
		g.rng.Float64() // решение о миграции
		g.rng.Intn(2)   // орбита миграции (применяется только при миграции)
		for i := 1; i < gasReservoirCandidates; i++ {
			g.rng.NormFloat64() // безусловные доборы M_gas
		}
		return g.rng.Float64()
	}

	migrated, plain, overflowed := 0, 0, 0
	for seed := int64(1); seed <= 1000; seed++ {
		p := pipeline(seed)
		if p.birthOrbit > 0 {
			overflowed++
		}
		if p.giantOrbit > 0 && p.giantOrbit != p.birthOrbit {
			migrated++
		} else {
			plain++
		}
		assert.Equal(t, ref(seed), after(seed),
			"сид %d: поток пред-слоя фиксирован (n + 3 + доборы), сид-исход g₀=%d g=%d",
			seed, p.birthOrbit, p.giantOrbit)
	}
	require.Greater(t, overflowed, 20, "переполнения в выборке есть (ветка исполняется)")
	require.Greater(t, migrated, 5, "мигрировавшие системы в выборке есть")
	require.Greater(t, plain, 5, "немигрировавшие системы в выборке есть")
}

// ==================== O7: F_ОБР ПОСЛЕ ФИКСАЦИИ G (§4.2 проход 5) ====================

// O7 — f_обр действует на M_i⁰ ПОСЛЕ фиксации g; проход 1 f_обр не знает;
// орбиты g, g−1 и покинутая g₀ исключены из прохода 5; при миграции орбита
// g₀ пуста (фантомной планеты нет).
func TestOverflowFObRAppliedAfterGiant(t *testing.T) {
	g := NewGenerator(nil, 20260926)
	buf := newBatchBuffers(16)
	const worlds = 3000
	worldsWithTruncation, abandoned := 0, 0
	for i := 0; i < worlds; i++ {
		runOverflowWorld(g, buf, "G")
		p := g.overflow
		rows := orbitData(t, buf)
		// Масса каменистого тела = k · clamp(f_обр·M_i⁰, 0.02, M_crit), где k —
		// общий множитель мягкого клампа суммы мира (§4.0.2): f_обр применён к
		// M_i⁰ ПОСЛЕ фиксации g, а не в проходе 1.
		minR, maxR, rocky := math.Inf(1), 0.0, 0
		truncated := false
		for o := 1; o <= overflowOrbitMax; o++ {
			if p.class[o] != bodyRocky || rows[o] == nil {
				continue
			}
			want := clamp(retentionFactor(o, p.giantOrbit)*p.coreMass[o], massMin, massMax)
			r := rows[o]["mass"].(float64) / want
			rocky++
			if r < minR {
				minR = r
			}
			if r > maxR {
				maxR = r
			}
			if retentionFactor(o, p.giantOrbit) < 1 {
				truncated = true
			}
		}
		// В тяжёлом диске все орбиты могут уйти в ветку оболочки — сверять нечего.
		if rocky > 0 {
			assert.InDelta(t, minR, maxR, 1e-9, "единый множитель клампа, f_обр применён (мир %d)", i)
		}
		if truncated {
			worldsWithTruncation++
		}
		// Покинутая орбита рождения: планеты нет.
		if p.abandonedOrbit(p.birthOrbit) {
			abandoned++
			assert.Nil(t, rows[p.birthOrbit], "покинутая g₀ пуста (мир %d)", i)
		}
	}
	require.Greater(t, worldsWithTruncation, 100, "миров с обрезкой f_обр достаточно")
	require.Greater(t, abandoned, 10, "мигрировавшие системы в выборке есть")

	// Проход 1 не знает f_обр: ядра не меняются при смене g (через M_crit).
	gA := NewGenerator(nil, 99)
	gA.cloudBudget = cloudProfileSum
	pA := gA.rollOverflow(6, 0, massMax)
	gB := NewGenerator(nil, 99)
	gB.cloudBudget = cloudProfileSum
	pB := gB.rollOverflow(6, 0, massMax/4)
	assert.Equal(t, pA.coreMass, pB.coreMass, "M_i⁰ не зависят от g")
	assert.NotEqual(t, pA.birthOrbit, pB.birthOrbit, "g₀ различается (тест не пустой)")
}

// ==================== O8: ГОРЯЧИЕ ЮПИТЕРЫ ИЗ МИГРАЦИИ (§6) ====================

// O8 — горячий юпитер = гигант с g ∈ {1,2} (миграция); без миграции
// g = g₀; отдельного ролла «горячий» нет. Глобально ∈ [0.5%, 1.0%] (Wright
// 2012), у G/F ≈ 2.0% (per-class цель пересмотрена, §10.4).
func TestHotJupiterFromMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла")
	}
	g := NewGenerator(nil, 7)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(16)
	const n = 20000
	migrated := 0

	giants := 0
	hot := 0
	for i := 0; i < n; i++ {
		runOverflowWorld(g, buf, "G")
		if g.overflow.giantOrbit == 0 {
			continue
		}
		giants++
		if g.overflow.birthOrbit != g.overflow.giantOrbit {
			migrated++
			assert.Contains(t, []int{1, 2}, g.overflow.giantOrbit, "миграция ведёт на орбиты 1–2")
		}
		if g.overflow.giantOrbit <= 2 {
			hot++
		}
		// Орбиты вне {1,2} — только без миграции.
		if g.overflow.giantOrbit > 2 {
			assert.Equal(t, g.overflow.birthOrbit, g.overflow.giantOrbit,
				"без миграции гигант стоит на орбите рождения")
		}
	}
	require.Greater(t, giants, 100, "гиганты G в выборке есть")
	migFrac := float64(migrated) / float64(giants)
	t.Logf("гигантов G: %d; мигрировало %.1f%% (P_mig = %.2f); горячих (g ≤ 2): %.2f%%",
		giants, migFrac*100, giantMigrationChance, float64(hot)/float64(n)*100)
	assert.InDelta(t, giantMigrationChance, migFrac, 0.05,
		"P_mig = 0.15 (калибровка под вилку горячих §13; ссылка §6.3/§17.7)")

	// Глобальная частота горячих по весам 99.2.4 §4.1.
	weights := map[string]float64{
		"O": 0.005, "B": 0.02, "A": 0.04, "F": 0.07, "G": 0.12,
		"K": 0.17, "M": 0.32, "L": 0.08, "T": 0.08, "Y": 0.095,
	}
	global := 0.0
	for cls, w := range weights {
		hot := 0
		for i := 0; i < n; i++ {
			runOverflowWorld(g, buf, cls)
			if g.overflow.giantOrbit >= 1 && g.overflow.giantOrbit <= 2 {
				hot++
			}
		}
		global += w * float64(hot) / float64(n)
	}
	t.Logf("горячие юпитеры глобально: %.3f%% (цель спеки §10.4: ≈0.94%%, вилка Wright 0.5–1.0%%)",
		global*100)
	assert.GreaterOrEqual(t, global, 0.005, "горячие юпитеры: низ вилки 0.5% (§13)")
	assert.LessOrEqual(t, global, 0.010, "горячие юпитеры: верх вилки 1.0% (§13)")
}

// ==================== O9: МАССА ГИГАНТА — АККРЕЦИЯ (§7) ====================

// O9 — M_body = M_core + κ_rich·M_gas·w_{g₀} (M_gas — отдельный резервуар,
// газ по орбите РОЖДЕНИЯ g₀); класс — по M_body; газовый режим с M_body ≤ 16
// → мини-нептун; безгазовый с ядром ≥ 16 → гигант; нижнего пересэмплинга нет;
// пайка на 16 нет; верх > 4131 — пересэмплинг M_gas (пайка нет, atMax = 0).
func TestGiantMassFromAccretion(t *testing.T) {
	// Формула оболочки — чистая функция от резервуара и орбиты рождения.
	assert.InDelta(t,
		envelopeRichKappa*1000*profileWeight(5),
		envelopeMass(7, 5, true, 1000), 1e-12, "оболочка = κ·M_gas·w_{g₀}")

	g := NewGenerator(nil, 20260927)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(16)
	atMax, atSixteen := 0, 0
	giants := 0
	for i := 0; i < 5000; i++ {
		runOverflowWorld(g, buf, "G")
		for o := 1; o <= overflowOrbitMax; o++ {
			if g.overflow.class[o] != bodyGiant {
				continue
			}
			giants++
			core := g.overflow.coreMass[o]
			// Газ набирается на орбите РОЖДЕНИЯ (§7.2): у мигрировавшего
			// первичного гиганта это g₀, у вторичных — своя орбита.
			birth := o
			if o == g.overflow.giantOrbit && g.overflow.birthOrbit > 0 {
				birth = g.overflow.birthOrbit
			}
			env := envelopeMass(core, birth, g.overflow.gasMode, g.gasReservoir)
			want := core + env
			got := g.overflow.bodyMass[o]
			if want > GasGiantMassMax {
				// Документированный фолбэк верхнего хвоста (ни один кандидат
				// M_gas не уложился) — «газ не удержан»: масса = ядро.
				assert.InDelta(t, core, got, 1e-9,
					"орбита %d: оболочка снята (верхний хвост, §7.3 п.5)", o)
			} else {
				assert.InDelta(t, want, got, 1e-9,
					"орбита %d: M_body = M_core + оболочка (газ по орбите рождения)", o)
			}
			assert.GreaterOrEqual(t, got, core, "ядро входит в массу тела")
			assert.LessOrEqual(t, got, GasGiantMassMax,
				"верх 4131 — пересэмплинг/снятие оболочки, не превышение")
			if got == GasGiantMassMax {
				atMax++
			}
			if g.overflow.bodyMass[o] == GasGiantMassMin {
				atSixteen++
			}
		}
	}
	require.Greater(t, giants, 100, "гиганты в выборке есть")
	assert.Zero(t, atMax, "клампа/пайка на 4131 нет (пересэмплинг)")
	assert.Zero(t, atSixteen, "пайка на 16 нет (граница — следствие класса)")
}

// ==================== O10: ЦЕЛЕВАЯ ЛИНИЯ МАССЫ (§7.3, §10.5) ====================

// O10 — статистика: медиана ≈ GasGiantTargetMedian, ЭМПИРИЧЕСКАЯ σ ∈
// [0.5, 0.75] декады (≈0.62, §7.3 п.1/§12.1), диапазон [16, 4131]; параметр
// σ_gas = 0.9 ДЕКАДЫ — отдельным утверждением. Обе величины — в декадах
// (log10), как σ эталона 99.2.15; лог-цель.
func TestGiantMassTargetLine(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла")
	}
	assert.InDelta(t, 0.9, gasReservoirSigmaDecades, 1e-9, "σ_gas — параметр, декады")

	g := NewGenerator(nil, 20260928)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(16)
	var masses []float64
	for i := 0; i < 4000 && len(masses) < 3000; i++ {
		runOverflowWorld(g, buf, "G")
		for o := 1; o <= overflowOrbitMax; o++ {
			if g.overflow.class[o] == bodyGiant {
				masses = append(masses, g.overflow.bodyMass[o])
			}
		}
	}
	require.Greater(t, len(masses), 500, "выборка гигантов достаточна")

	med := medianOf(masses)
	decs := make([]float64, 0, len(masses))
	for _, m := range masses {
		require.GreaterOrEqual(t, m, GasGiantMassMin)
		require.LessOrEqual(t, m, GasGiantMassMax)
		decs = append(decs, math.Log10(m))
	}
	meanDec := 0.0
	for _, v := range decs {
		meanDec += v
	}
	meanDec /= float64(len(decs))
	varS := 0.0
	for _, v := range decs {
		varS += (v - meanDec) * (v - meanDec)
	}
	sigma := math.Sqrt(varS / float64(len(decs)-1))
	under, over := 0, 0
	for _, m := range masses {
		if m < 100 {
			under++
		}
		if m > 1000 {
			over++
		}
	}
	t.Logf("гиганты: n=%d медиана=%.1f (цель %.1f) эмпирическая σ=%.3f декады (цель 0.62); <100: %.1f%%, >1000: %.1f%%",
		len(masses), med, GasGiantTargetMedian, sigma,
		float64(under)/float64(len(masses))*100, float64(over)/float64(len(masses))*100)

	assert.InDelta(t, GasGiantTargetMedian, med, GasGiantTargetMedian*0.5,
		"медиана ≈ целевой линии (допуск широкий: калибровка M_gas0)")
	assert.GreaterOrEqual(t, sigma, 0.5, "эмпирическая σ ≥ 0.5 декады (§7.3 п.1)")
	assert.LessOrEqual(t, sigma, 0.75, "эмпирическая σ ≤ 0.75 декады (§7.3 п.1)")
	assert.InDelta(t, 0.62, sigma, 0.13, "эмпирическая σ ≈ 0.62 декады (цель спеки)")
}

// ==================== O14: РЕГИОНАЛЬНЫЙ СДВИГ — ГАЗОУДЕРЖАНИЕ (§5.3) ==========

// O14 — gas_giant_shift масштабирует ДОЛЮ газового режима f_rich_eff (решение
// создателя 2026-09-23): f_rich_eff = clamp(f_rich·k_region, 0, 1), где
// k_region(s) = clamp(1 + s/0.3, 1/3, 5/3). Порог M_crit и граница класса 16 —
// ГЛОБАЛЬНЫЕ константы, регион их не трогает (прежний M_crit_eff снят).
// «Не доминируют» держит f_rich ≤ 0.5.
func TestGasGiantShiftShiftsGas(t *testing.T) {
	shiftProfile := func(shift float64) *regionprofile.Profile {
		return &regionprofile.Profile{Planet: regionprofile.PlanetMods{GasGiantShift: shift}}
	}
	richFor := func(shift float64) float64 {
		g := NewGenerator(nil, 1)
		g.profile = shiftProfile(shift)
		g.profileIntensity = regionprofile.Strong
		return g.gasRichFractionEffective()
	}

	// k_region: без сдвига 1; на границах рабочего диапазона ±0.2 → [1/3, 5/3].
	assert.InDelta(t, 1.0, kRegion(0), 1e-9, "k_region(0) = 1")
	assert.InDelta(t, 5.0/3.0, kRegion(0.2), 1e-9, "k_region(+0.2) = 5/3")
	assert.InDelta(t, 1.0/3.0, kRegion(-0.2), 1e-9, "k_region(−0.2) = 1/3")

	// f_rich_eff: масштабируется сдвигом и зажата в [0, 1].
	assert.InDelta(t, gasRichFraction, richFor(0), 1e-9, "без сдвига f_rich_eff = f_rich")
	assert.InDelta(t, gasRichFraction*5.0/3.0, richFor(0.2), 1e-9, "shift +0.2 → газоудержание выше")
	assert.InDelta(t, gasRichFraction/3.0, richFor(-0.2), 1e-9, "shift −0.2 → газоудержание ниже")
	assert.GreaterOrEqual(t, richFor(0.2), 0.0, "f_rich_eff ≥ 0")
	assert.LessOrEqual(t, richFor(0.2), 1.0, "f_rich_eff ≤ 1")

	// Глобальность: M_crit и граница класса 16 от региона НЕ зависят.
	assert.InDelta(t, 8.0, massMax, 1e-9, "M_crit — глобальная константа (не регион)")
	assert.InDelta(t, 16.0, GasGiantMassMin, 1e-9, "граница класса 16 — глобальная константа")
	for _, shift := range []float64{-0.2, 0, 0.2} {
		g := NewGenerator(nil, 7)
		g.cloudBudget = cloudProfileSum
		g.gasReservoir = gasReservoirMedian
		g.profile = shiftProfile(shift)
		g.profileIntensity = regionprofile.Strong
		p := g.rollOverflow(6, 0, massMax)
		assert.InDelta(t, massMax, p.crit, 1e-9, "порог пред-слоя = M_crit при shift %.1f", shift)
	}

	// Поведенчески: положительный сдвиг («газовый регион») даёт больше гигантов.
	countGiants := func(shift float64) float64 {
		g := NewGenerator(nil, 20260929)
		g.means = DefaultPlanetMeans()
		prof := shiftProfile(shift)
		buf := newBatchBuffers(16)
		gi := 0
		const n = 2000
		for i := 0; i < n; i++ {
			count := g.determinePlanetCount("G")
			buf.reset()
			g.generateWorldWithCountIntoBuffer(WorldInfo{
				ID: "w", Name: "W", SpectralClass: "G", StarType: "star",
				Profile: prof, ProfileIntensity: regionprofile.Strong,
			}, count, buf)
			for _, d := range orbitData(t, buf) {
				if d["is_gas_giant"] == true {
					gi++
					break
				}
			}
		}
		return float64(gi) / float64(n)
	}
	gPlus, gMinus := countGiants(0.2), countGiants(-0.2)
	t.Logf("доля миров с гигантом: shift +0.2 → %.2f%%, −0.2 → %.2f%%", gPlus*100, gMinus*100)
	assert.Greater(t, gPlus, gMinus, "положительный сдвиг → больше гигантов")
	assert.Less(t, gPlus, 0.5, "«не доминируют»: доля миров с гигантом < 50%")

	// f_rich ≤ 0.5 — достаточное условие: P_giant ≤ P_ov·(f_rich + (1−f_rich)·q_poor).
	assert.LessOrEqual(t, gasRichFraction, 0.5, "f_rich ≤ 0.5 (условие «не доминируют»)")
}

// ==================== O19: ГАЗОВЫЙ РЕЗЕРВУАР НЕЗАВИСИМ (§7.2) ====================

// O19 — M_gas — ОТДЕЛЬНЫЙ ролл, не функция M_диск: изменение M_диск не меняет
// оболочку, и наоборот; твёрдый инвариант Σ твёрдых ≤ M_диск держится (ядро
// гиганта входит в сумму, оболочка — нет).
func TestGasReservoirIndependentOfSolid(t *testing.T) {
	// (а) структурно: M_gas — ровно один нормальный ролл, поток не зависит
	// от бюджета облака (ролл стоит своей позицией).
	a := NewGenerator(nil, 20260930)
	_ = a.rollGasReservoir()
	va := a.rng.Float64()
	b := NewGenerator(nil, 20260930)
	_ = b.rng.NormFloat64()
	vb := b.rng.Float64()
	assert.Equal(t, vb, va, "M_gas = M_gas0·exp(σ·NormFloat64()) — один нормальный ролл")

	// (б) оболочка не зависит от твёрдого бюджета: при том же M_gas та же
	// формула; ядро — зависит (пропорционально бюджету).
	g1 := NewGenerator(nil, 20260931)
	g1.cloudBudget = cloudProfileSum * 0.5
	g1.gasReservoir = 2600
	p1 := g1.rollOverflow(6, -0.4, massMax) // метфактор бедный — меньше гигантов
	g2 := NewGenerator(nil, 20260931)
	g2.cloudBudget = cloudProfileSum * 0.5
	g2.gasReservoir = 2600
	p2 := g2.rollOverflow(6, -0.4, massMax)
	// Один и тот же бюджет и seed → одинаковые ядра (контроль).
	assert.Equal(t, p1.coreMass, p2.coreMass)
	// Оболочка = κ·M_gas·w_{g₀} — от бюджета не зависит по формуле.
	env1 := envelopeMass(p1.coreMass[5], 5, true, 2600)
	assert.InDelta(t, envelopeRichKappa*2600*profileWeight(5), env1, 1e-9,
		"M_env — функция от M_gas и w_{g₀}, не от M_диск")
	// Ядро масштабируется бюджетом: удвоение бюджета → удвоение ядра (тот же ζ).
	g3 := NewGenerator(nil, 20260931)
	g3.cloudBudget = cloudProfileSum
	p3 := g3.rollOverflow(6, -0.4, massMax)
	for i := 1; i <= 6; i++ {
		assert.InDelta(t, 2.0, p3.coreMass[i]/p1.coreMass[i], 1e-9, "ядро ∝ M_диск (орбита %d)", i)
	}

	// (в) твёрдый инвариант: applySumClamp ужимает ТОЛЬКО твёрдые доли; ядро
	// гиганта входит в сумму (p.Mass), оболочка — нет (витринная масса цела).
	gg := NewGenerator(nil, 20260932)
	gg.cloudBudget = 10
	// Гигант: ядро 6 (в твёрдую сумму), витринная масса 600 (на кривой).
	giantData, err := json.Marshal(map[string]interface{}{
		"mass": 600.0, "size": GasGiantRadius(600), "density": 600 / math.Pow(GasGiantRadius(600), 3),
		"is_gas_giant": true,
	})
	require.NoError(t, err)
	giant := &PlanetData{Mass: 6, Data: giantData}
	// Каменистая: масса 8 из бюджета.
	rockyData, err := json.Marshal(map[string]interface{}{"mass": 8.0, "density": 1.0})
	require.NoError(t, err)
	rocky := &PlanetData{Mass: 8, Data: rockyData}
	gg.applySumClamp([]*PlanetData{giant, rocky}, nil)

	var gd map[string]interface{}
	require.NoError(t, json.Unmarshal(giant.Data, &gd))
	assert.InDelta(t, 600.0, gd["mass"].(float64), 1e-9,
		"оболочка гиганта клампом не ужимается (кривая M→R держится)")
	assert.Less(t, giant.Mass, 6.0, "твёрдое ядро гиганта входит в сумму и ужимается")
	assert.LessOrEqual(t, giant.Mass+rocky.Mass, gg.cloudBudget+1e-9,
		"Σ твёрдых (ядро + каменистые) ≤ M_диск")
	assert.InDelta(t, 8.0, func() float64 {
		var rd map[string]interface{}
		require.NoError(t, json.Unmarshal(rocky.Data, &rd))
		return rd["mass"].(float64)
	}(), 8.0*gg.cloudBudget/(6.0+8.0), 1e-6, "каменистая ужата пропорционально")
}

// ==================== O20: ПОКИНУТАЯ g₀ ИСКЛЮЧЕНА (§4.2 проход 5) ====================

// O20 — при g₀ ≠ g: орбита g₀ пуста (фантомной планеты нет), g−1 — пояс,
// g — гигант.
func TestOverflowG0ExcludedFromPass5(t *testing.T) {
	g := NewGenerator(nil, 20260933)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(16)
	const worlds = 4000
	seen := 0
	for i := 0; i < worlds; i++ {
		runOverflowWorld(g, buf, "G")
		p := g.overflow
		if !p.abandonedOrbit(p.birthOrbit) {
			continue
		}
		seen++
		rows := orbitData(t, buf)
		assert.Nil(t, rows[p.birthOrbit], "покинутая g₀ пуста (мир %d)", i)
		assert.Contains(t, []int{1, 2}, p.giantOrbit, "миграция ведёт на 1–2")
		d := rows[p.giantOrbit]
		require.NotNil(t, d, "гигант на g")
		assert.Equal(t, true, d["is_gas_giant"], "на g — газовый гигант")
		// Пояс — только при g ≥ 2 и на орбите g−1.
		if p.giantOrbit >= 2 {
			ast := false
			for _, b := range beltsOf(t, buf) {
				if b.kind == "asteroid" {
					ast = true
					require.NotNil(t, b.orbit)
					assert.Equal(t, p.giantOrbit-1, *b.orbit, "beltOrbit = g−1")
				}
			}
			assert.True(t, ast, "при g ≥ 2 есть пояс на g−1")
		}
	}
	require.Greater(t, seen, 50, "системы с миграцией в выборке есть")
}

// ==================== O21: КОРИДОР M_CRIT (§5.2) ====================

// O21 — M_crit ∈ [5, 10] (валидация константы, жёстко < 16); зона мини-нептуна
// (M_crit, 16] не вырождается.
func TestMcritCorridor(t *testing.T) {
	assert.GreaterOrEqual(t, massMax, 5.0, "M_crit ≥ 5")
	assert.LessOrEqual(t, massMax, 10.0, "M_crit ≤ 10")
	assert.Less(t, massMax, GasGiantMassMin, "M_crit < 16 — зона мини-нептуна не вырождена")
	assert.Greater(t, GasGiantMassMin-massMax, 5.0, "ширина зоны мини-нептуна > 5 M⊕")
}

// ==================== O22: КЛАСС — ЕДИНСТВЕННЫЙ КРИТЕРИЙ M_BODY (§7.1) ====================

// O22 — класс = M_body: газовый режим с M_body ≤ 16 → мини-нептун (не
// гигант); безгазовый с ядром ≥ 16 → гигант; режим класс не переопределяет;
// пересэмплинга по нижней границе нет.
func TestGiantClassByMass(t *testing.T) {
	// Границы класса — чистая функция.
	assert.Equal(t, bodyMiniNeptune, classifyBodyClass(10, 16.0, massMax), "M_body = 16 → мини-нептун")
	assert.Equal(t, bodyGiant, classifyBodyClass(10, 16.0001, massMax), "M_body > 16 → гигант")
	assert.Equal(t, bodyRocky, classifyBodyClass(massMax, massMax, massMax), "ядро ≤ M_crit → каменистое")
	assert.Equal(t, bodyMiniNeptune, classifyBodyClass(massMax+0.01, 12, massMax), "переполнение ниже 16 → мини-нептун")

	// Газовый режим, но оболочка не убежала → мини-нептун (режим не решает класс).
	core, mGas := 12.0, 20.0
	body := core + envelopeMass(core, 3, true, mGas)
	assert.LessOrEqual(t, body, GasGiantMassMin, "лёгкая газовая оболочка не выводит за 16")
	assert.Equal(t, bodyMiniNeptune, classifyBodyClass(core, body, massMax),
		"газовый режим с M_body ≤ 16 → мини-нептун")

	// Безгазовый режим с ядром ≥ 16 → гигант (оболочка 0).
	assert.Zero(t, envelopeMass(17, 4, false, 0), "ядро ≥ 16: тонкой оболочки нет")
	assert.Equal(t, bodyGiant, classifyBodyClass(17, 17, massMax), "безгазовый с ядром ≥ 16 → гигант")

	// Безгазовый с ядром < 16 держится ВНУТРИ зоны мини-нептуна (переполнения
	// класса нет).
	for core := massMax + 0.01; core < GasGiantMassMin; core += 0.5 {
		b := core + envelopeMass(core, 5, false, 0)
		assert.LessOrEqual(t, b, GasGiantMassMin, "тонкая оболочка не выводит за 16 (core %.2f)", core)
		assert.Equal(t, bodyMiniNeptune, classifyBodyClass(core, b, massMax))
	}
}

// ==================== O23: ПЕРВИЧНЫЙ МИНИ-НЕПТУН — БЕЗ ПОЯСА (§4.2 проход 3) ====================

// O23 — первичное тело — мини-нептун (газовый режим без убегания либо
// безгазовый остаток): g = 0, пояса нет, f_обр не применяется, орбита g₀
// занята мини-нептуном.
func TestPrimaryMiniNeptuneNoBelt(t *testing.T) {
	g := NewGenerator(nil, 20260934)
	g.means = DefaultPlanetMeans()
	buf := newBatchBuffers(16)
	const worlds = 4000
	seen := 0
	for i := 0; i < worlds; i++ {
		runOverflowWorld(g, buf, "G")
		p := g.overflow
		if p.birthOrbit == 0 || p.class[p.birthOrbit] != bodyMiniNeptune {
			continue
		}
		seen++
		assert.Zero(t, p.giantOrbit, "первичный мини-нептун → гиганта нет (g = 0)")
		for _, b := range beltsOf(t, buf) {
			assert.NotEqual(t, "asteroid", b.kind, "пояса нет (окно g−1 не определено)")
		}
		rows := orbitData(t, buf)
		d := rows[p.birthOrbit]
		require.NotNil(t, d, "орбита g₀ занята мини-нептуном")
		assert.Equal(t, true, d["is_mini_neptune"], "на g₀ — мини-нептун")
		assert.NotEqual(t, true, d["is_gas_giant"], "не гигант")
		// f_обр не применяется: каменистые массы = k·clamp(M_i⁰) (f_обр = 1 при
		// g = 0), k — общий множитель мягкого клампа суммы мира.
		minR, maxR, rocky := math.Inf(1), 0.0, 0
		for o := 1; o <= overflowOrbitMax; o++ {
			if p.class[o] != bodyRocky || rows[o] == nil {
				continue
			}
			r := rows[o]["mass"].(float64) / clamp(p.coreMass[o], massMin, massMax)
			rocky++
			if r < minR {
				minR = r
			}
			if r > maxR {
				maxR = r
			}
		}
		// В тяжёлом диске все орбиты могут уйти в ветку оболочки — тогда
		// каменистых тел нет и сверять нечего.
		if rocky > 0 {
			assert.InDelta(t, minR, maxR, 1e-9, "единый множитель, f_обр не применён (мир %d)", i)
		}
	}
	require.Greater(t, seen, 30, "миры с первичным мини-нептуном в выборке есть")
}

