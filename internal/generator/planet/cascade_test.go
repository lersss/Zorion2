// internal/generator/planet/cascade_test.go
//
// Тесты физического каскада (99.2.20 §13): калибровки Земля/Венера/Марс/
// O-звезда, гиганты (T⁴ + F_KH), инвариант флага, сумма состава атмосферы,
// орбитальная шкала √L, приливный захват, смоук 1000 планет.
package planet

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== КАСКАД-ЭТАЛОНЫ (§13.2, калибровки §3.6) ====================

// earthInput — Земля-режим: G, орбита 2 (r = 1.156 а.е.), M = 1, ρ = 1
// (R = 1, g = 1 — SizeOverride), water_share = 0.7 (доставка воды),
// f_vol = 1.5·10⁻⁴, жизнь.
func earthInput() cascadeInput {
	return cascadeInput{
		Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, Metallicity: 0, TEff: 5772,
		OrbitRadiusAU: 1.156, OrbitIndex: 2,
		MassOverride:       1,
		SizeOverride:       1,
		SurfaceOverride:    Composition{SurfaceOceans: 100}, // A_surface = 0.06
		FVolOverride:       1.5e-4,
		WaterShareOverride: 0.7,
		ForceLife:          true,
	}
}

// TestCascadeEarthMode — Земля-режим: T_final ≈ 287.2 K (диапазон 285–295 —
// состав жизненного прохода роллится), P ≈ 1.05 атм, τ_IR ≈ 1.4,
// флаг true (273 < 287.2 < 374.5).
func TestCascadeEarthMode(t *testing.T) {
	g := NewGenerator(nil, 1)
	res := g.runCascade(earthInput())
	assert.Greater(t, res.TFinal, 285.0, "T_final > 285")
	assert.Less(t, res.TFinal, 295.0, "T_final < 295")
	assert.InDelta(t, 1.05, res.AtmosphereData.PressureAtm, 0.1, "P")
	assert.Greater(t, res.AtmosphereData.TauIR, 1.3, "τ_IR > 1.3")
	assert.Less(t, res.AtmosphereData.TauIR, 1.7, "τ_IR < 1.7")
	assert.True(t, res.LiquidWater, "флаг жидкой воды")
	assert.Equal(t, "азотно-кислородная", res.AtmosphereLabel, "ярлык после жизненного прохода")
}

// venusInput — Венера-аналог игрового лэддера: G, орбита 1 (0.68 а.е.),
// лавовая поверхность A_surface = 0.10, f_vol = 10⁻⁴, M = 0.815, R = 0.967.
func venusInput() cascadeInput {
	return cascadeInput{
		Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, Metallicity: 0, TEff: 5772,
		OrbitRadiusAU: 0.68, OrbitIndex: 1,
		MassOverride:    0.815,
		SizeOverride:    0.967,
		SurfaceOverride: Composition{SurfaceLavaFields: 100}, // A_surface = 0.10
		FVolOverride:    1e-4,
	}
}

// TestCascadeVenusMode — Венера-аналог: P ≈ 88 атм, τ_IR ≈ 87,
// T_final ≈ 747 K, флаг false (T > T_кип(88) = 508).
func TestCascadeVenusMode(t *testing.T) {
	g := NewGenerator(nil, 2)
	res := g.runCascade(venusInput())
	assert.InDelta(t, 88.0, res.AtmosphereData.PressureAtm, 2.0, "P")
	assert.InDelta(t, 87.0, res.AtmosphereData.TauIR, 2.0, "τ_IR")
	assert.InDelta(t, 747.0, res.TFinal, 5.0, "T_final")
	assert.False(t, res.LiquidWater, "флаг false (вода кипит)")
	assert.Equal(t, "парниковая", res.AtmosphereLabel, "ярлык")
}

// marsInput — Марс-режим: G, орбита 3 (r = 1.96 а.е.), M = 0.1, ρ = 1.0
// (R = (0.1/1)^(1/3) = 0.464, g = 0.464 — SizeOverride), f_vol = 10⁻⁴.
func marsInput() cascadeInput {
	return cascadeInput{
		Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, Metallicity: 0, TEff: 5772,
		OrbitRadiusAU: 1.96, OrbitIndex: 3,
		MassOverride:    0.1,
		SizeOverride:    0.464,
		SurfaceOverride: Composition{SurfaceRocks: 100}, // A_surface = 0.15
		FVolOverride:    1e-4,
	}
}

// TestCascadeMarsMode — Марс-режим: T_final ≈ 191 K, P ≈ 0.0075 атм,
// τ_IR ≈ 0, флаг false (T < 273; P = 0.0075 ≥ 0.006 — решает температура).
func TestCascadeMarsMode(t *testing.T) {
	g := NewGenerator(nil, 3)
	res := g.runCascade(marsInput())
	assert.InDelta(t, 191.0, res.TFinal, 3.0, "T_final")
	assert.InDelta(t, 0.0075, res.AtmosphereData.PressureAtm, 0.001, "P")
	assert.InDelta(t, 0.0, res.AtmosphereData.TauIR, 0.05, "τ_IR")
	assert.False(t, res.LiquidWater, "флаг false")
}

// oStarInput — O-звезда, орбита 1 (152 а.е. по √L-масштабу): f_strip = 0.01
// (T_eff > 15000 K) → воздушная сухая планета, ярлык «разреженная».
func oStarInput() cascadeInput {
	return cascadeInput{
		Luminosity: 5e4, StellarMass: 37.5, AgeGyr: 0.5, Metallicity: 0, TEff: 35000,
		OrbitRadiusAU: 152.0, OrbitIndex: 1,
		MassOverride:    1,
		SurfaceOverride: Composition{SurfaceSands: 100}, // A_surface = 0.35 → умеренный режим
		FVolOverride:    1e-4,
	}
}

// TestCascadeOStar — O-звезда: XUV-стриппинг снимает атмосферу (P < 0.1 →
// «разреженная»), флаг false (вода не живёт), T ≈ 327 K (воздушное тело).
func TestCascadeOStar(t *testing.T) {
	g := NewGenerator(nil, 4)
	res := g.runCascade(oStarInput())
	assert.InDelta(t, 327.0, res.TFinal, 25.0, "T_final")
	assert.Less(t, res.AtmosphereData.PressureAtm, 0.1, "P < 0.1 — атмосфера снята стриппингом")
	assert.Equal(t, "разреженная", res.AtmosphereLabel, "ярлык")
	assert.False(t, res.LiquidWater, "флаг false")
}

// ==================== ГИГАНТЫ: T⁴ + F_KH (§13.2) ====================

// TestCascadeGiantJupiter — Юпитер-режим (M = 317.8, age 4.6, r = 5.2 а.е.):
// F_KH = 6.06 Вт/м² → T ≈ 118 K (реальный ~124).
func TestCascadeGiantJupiter(t *testing.T) {
	g := NewGenerator(nil, 5)
	res := g.runCascadeGiant(cascadeInput{
		Luminosity: 1, StellarMass: 1, AgeGyr: 4.6, Metallicity: 0, TEff: 5772,
		OrbitRadiusAU: 5.2, OrbitIndex: 5,
		MassOverride: 317.8,
	})
	assert.InDelta(t, 118.0, res.TFinal, 3.0, "T_final")
	assert.InDelta(t, 6.06, res.HeatFluxWm2, 0.1, "F_KH")
	assert.Equal(t, 1.0, res.AtmosphereData.PressureAtm, "P = 1 атм (условный уровень)")
	assert.InDelta(t, 100.0, sumCompositionPct(res.AtmosphereData.Composition), 0.5, "сумма состава = 100")
}

// TestCascadeGiantYoung — молодой гигант 13 MJ (age 0.1): F_KH ≈ 25 000 Вт/м²
// → T_int ≈ 577 K (горячий молодой гигант).
func TestCascadeGiantYoung(t *testing.T) {
	g := NewGenerator(nil, 6)
	res := g.runCascadeGiant(cascadeInput{
		Luminosity: 1, StellarMass: 1, AgeGyr: 0.1, Metallicity: 0, TEff: 5772,
		OrbitRadiusAU: 5.2, OrbitIndex: 5,
		MassOverride: 4131,
	})
	assert.InDelta(t, 25000.0, res.HeatFluxWm2, 3000.0, "F_KH")
	assert.Greater(t, res.TFinal, 550.0, "T > 550 K — горячий молодой гигант")
	assert.LessOrEqual(t, res.TFinal, 2500.0, "в [20, 2500]")
}

// ==================== ОРБИТАЛЬНАЯ ШКАЛА И ПРИЛИВНЫЙ ЗАХВАТ (§13.6) ====================

// TestCascadeOrbitScale — G-звезда (√1 = 1): орбиты без изменений;
// инсоляция орбиты i одинакова для всех классов (F★ = 2943 / 1018.5 / 354
// Вт/м² на орбитах 1/2/3).
func TestCascadeOrbitScale(t *testing.T) {
	assert.InDelta(t, 0.4*1.7, orbitRadiusScaled(1, 1.0), 1e-9, "G орбита 1")
	assert.InDelta(t, 0.4*1.7*1.7, orbitRadiusScaled(2, 1.0), 1e-9, "G орбита 2")
	for _, l := range []float64{1, 1000, 5e4} {
		f1 := 1361 * l / (orbitRadiusScaled(1, l) * orbitRadiusScaled(1, l))
		assert.InDelta(t, 2943.0, f1, 1.0, "L=%.0f орбита 1", l)
		f2 := 1361 * l / (orbitRadiusScaled(2, l) * orbitRadiusScaled(2, l))
		assert.InDelta(t, 1018.5, f2, 1.0, "L=%.0f орбита 2", l)
		f3 := 1361 * l / (orbitRadiusScaled(3, l) * orbitRadiusScaled(3, l))
		assert.InDelta(t, 354.0, f3, 2.0, "L=%.0f орбита 3 (спека: ≈354)", l)
	}
}

// TestCascadeTidalLock — приливный захват: M-звезда орбита 1 (0.068 а.е.):
// P_orb = 8–21 сут → true при M★ ≥ 0.42 (P = 10 сут), false при M★ < 0.42
// (M★ = 0.25 → 12.9 сут); обитаемая орбита 2 (0.116 а.е., P = 14–19 сут) →
// false.
func TestCascadeTidalLock(t *testing.T) {
	p1 := orbitalPeriod(0.068, 0.42)
	assert.InDelta(t, 10.0/365.25, p1, 0.002, "M★=0.42: P ≈ 10 сут")
	assert.True(t, p1 < tidalLockThresholdYears, "M★=0.42: захват есть")

	p2 := orbitalPeriod(0.068, 0.25)
	assert.InDelta(t, 12.9/365.25, p2, 0.002, "M★=0.25: P ≈ 12.9 сут")
	assert.False(t, p2 < tidalLockThresholdYears, "M★=0.25: захвата нет")

	p3 := orbitalPeriod(0.116, 0.6)
	assert.Greater(t, p3, tidalLockThresholdYears, "обитаемая орбита 2: P > 10 сут → false")
}

// ==================== СМОУК 1000 ПЛАНЕТ (§13.9) ====================

// TestCascadeSmoke1000 — 1000 планет каскада (смесь классов и орбит):
// все T ∈ [20, 2500], P ∈ [0, 1000], флаг консистентен, сумма атмосферы =
// 100; запись обитаемой доли в отчёт.
func TestCascadeSmoke1000(t *testing.T) {
	g := NewGenerator(nil, 42)
	classes := []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}
	habitable := 0
	withFlag := 0
	typeCount := map[string]int{}
	for i := 0; i < 1000; i++ {
		cls := classes[g.rng.Intn(len(classes))]
		orbit := 1 + g.rng.Intn(8)
		pd := g.generatePlanet("w", "World", orbit, stellarParamsFromClass(cls, 0, g.rng))
		require.NotNil(t, pd)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(pd.Data, &data))

		if typ, ok := data["type"].(string); ok {
			typeCount[typ]++
		}

		temp, ok := data["temperature"].(float64)
		require.True(t, ok, "temperature обязателен")
		assert.GreaterOrEqual(t, temp, 20.0, "T ≥ 20")
		assert.LessOrEqual(t, temp, 2500.0, "T ≤ 2500")

		// Атмосфера-объект: сумма состава = 100, P ∈ [0, 1000].
		if atm, ok := data["atmosphere_data"].(map[string]interface{}); ok {
			comp, ok := atm["composition"].(map[string]interface{})
			require.True(t, ok, "composition обязателен в atmosphere_data")
			sum := 0.0
			for _, v := range comp {
				f, ok := v.(float64)
				require.True(t, ok, "доля газа — число")
				sum += f
			}
			assert.InDelta(t, 100.0, sum, 0.5, "сумма состава атмосферы")
			p, ok := atm["pressure_atm"].(float64)
			require.True(t, ok, "pressure_atm обязателен")
			assert.GreaterOrEqual(t, p, 0.0, "P ≥ 0")
			assert.LessOrEqual(t, p, 1000.0, "P ≤ 1000 (кламп)")
		}

		// Флаг консистентен (инвариант §3.7).
		if flag, ok := data["liquid_water_possible"].(bool); ok {
			withFlag++
			p := 0.0
			if atm, ok := data["atmosphere_data"].(map[string]interface{}); ok {
				p, _ = atm["pressure_atm"].(float64)
			}
			expected := p >= 0.006 && temp > 273 && temp < 373+30.2*math.Log(p)
			assert.Equal(t, expected, flag, "флаг не консистентен: T=%.1f P=%.4f", temp, p)
		}

		// Обитаемая доля (запись в отчёт смоука).
		if flag, ok := data["liquid_water_possible"].(bool); ok && flag {
			water, _ := data["water_percent"].(float64)
			if water > 10 && temp > 200 && temp < 350 {
				habitable++
			}
		}
	}
	assert.Greater(t, withFlag, 600, "флаг пишется каменистым/ледяным (гиганты без флага — нет поверхности)")
	// Лог-цель обитаемой доли (спека 2026-09-20 §7): ~5–8% → ~3–4%
	// (галактическая; числитель не растёт, знаменатель ×2.1 — новые mean
	// §5.1). Фактическая мера — смоук 20 000 миров (§11).
	t.Logf("обитаемая доля: %.1f%% (цель ~3–4%%)", float64(habitable)/10)

	// Частоты типов (99.2.20 §13.9): океаническая/радиоактивная/ледяная
	// возникают из физики. Радиоактивные и ледяные встречаются; океанические
	// редки (узкое окно: гидросфера «океаны» + вода > 60 + доминирование
	// океанов в композиции при T ∈ (273, 300)) — следствие растворения
	// подветки (спека §5: «частота меняется»), фиксируется в отчёте.
	t.Logf("частоты типов: %v", typeCount)
	assert.Greater(t, typeCount[TypeRadioactive], 0, "радиоактивные встречаются (core.radioactivity > 50)")
	assert.Greater(t, typeCount[TypeIce], 0, "ледяные встречаются (T < 250)")
	assert.Less(t, typeCount[TypeOceanic], 300, "океанические не доминируют")
	assert.Less(t, typeCount[TypeRadioactive], 500, "радиоактивные не доминируют")
}

// ==================== ХЕЛПЕРЫ ====================

// sumCompositionPct — сумма процентов состава атмосферы.
func sumCompositionPct(comp map[string]float64) float64 {
	sum := 0.0
	for _, v := range comp {
		sum += v
	}
	return sum
}