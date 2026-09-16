// internal/generator/planet/atmosphere.go
//
// Атмосфера-объект (99.2.20 §3.6, §4.1): состав газов в % (сумма 100),
// давление у поверхности, масса, оптическая толщина в ИК, масштабная
// высота, средняя молекулярная масса. Тип атмосферы («азотно-кислородная»,
// «парниковая»…) — производный ярлык из состава и давления (кэш для
// совместимости с пресетами/аудитом/UI).
package planet

import (
	"math"
	"math/rand"
)

// AtmosphereData — атмосфера-объект (99.2.20 §4.1).
type AtmosphereData struct {
	Composition         map[string]float64 // газ → % (сумма 100)
	PressureAtm         float64            // давление у поверхности, атм
	MassEarthAtm        float64            // масса в массах атмосферы Земли (5.15·10¹⁸ кг)
	TauIR               float64            // оптическая толщина в ИК
	ScaleHeightKm       float64            // масштабная высота, км
	MeanMolecularWeight float64            // средняя молекулярная масса
}

// ==================== ЗОНЫ СНЕГОВОЙ ЛИНИИ (99.2.20 §3.6 п.1) ====================

type volatileZone int

const (
	zoneInner volatileZone = iota
	zoneTransition
	zoneOuter
)

// volatileZoneFor — зона по снеговой линии r_ice = 2.7·√L а.е.
func volatileZoneFor(orbitRadiusAU, luminosity float64) volatileZone {
	rIce := 2.7 * math.Sqrt(luminosity)
	switch {
	case orbitRadiusAU < 0.7*rIce:
		return zoneInner
	case orbitRadiusAU <= 1.5*rIce:
		return zoneTransition
	default:
		return zoneOuter
	}
}

// volatileBudget — бюджет летучих f_vol (доля массы планеты) по зоне:
// внутренняя log-uniform [10⁻⁴, 10⁻³], переходная [10⁻⁴, 10⁻²],
// внешняя [10⁻², 0.3].
func volatileBudget(zone volatileZone, rng *rand.Rand) float64 {
	switch zone {
	case zoneInner:
		return logUniformRange(rng, 1e-4, 1e-3)
	case zoneTransition:
		return logUniformRange(rng, 1e-4, 1e-2)
	default:
		return logUniformRange(rng, 1e-2, 0.3)
	}
}

// waterShareByZone — доля воды в бюджете летучих (доставка воды стохастична,
// особенно во внутреннюю зону — Земля получила воду, Венера/Меркурий нет):
// внутренняя 0.05–0.9 (log-uniform), переходная 0.5–0.8, внешняя 0.7–0.9.
func waterShareByZone(zone volatileZone, rng *rand.Rand) float64 {
	switch zone {
	case zoneInner:
		return logUniformRange(rng, 0.05, 0.9)
	case zoneTransition:
		return 0.5 + rng.Float64()*0.3
	default:
		return 0.7 + rng.Float64()*0.2
	}
}

// logUniformRange — log-uniform ролл в [min, max] (min > 0).
func logUniformRange(rng *rand.Rand, min, max float64) float64 {
	return math.Exp(math.Log(min) + rng.Float64()*(math.Log(max)-math.Log(min)))
}

// ==================== РЕЖИМ И СЕКВЕСТРАЦИЯ (99.2.20 §3.5 п.2, §3.6 п.2) ====================

// atmosphereRegime — режим атмосферы по T₁: < 273 холодный, 273–373
// умеренный, > 373 горячий (порог 373 K = точка кипения воды при 1 атм:
// выше вода не конденсируется → секвестрации нет → CO₂-режим).
func atmosphereRegime(t1 float64) string {
	switch {
	case t1 < waterFreezeK:
		return "холодный"
	case t1 <= waterBoilAt1AtmK:
		return "умеренный"
	default:
		return "горячий"
	}
}

// sequestrationFactor — секвестрация летучих по режиму: горячий — нет
// (океанов/карбонатов нет), умеренный/холодный — ×0.006 (вода в океанах,
// CO₂ в карбонатах; калибровка Земля → ~1 атм, Марс).
func sequestrationFactor(regime string) float64 {
	if regime == "горячий" {
		return 1.0
	}
	return 0.006
}

// escapeFactor — удержание атмосферы: f_esc = 0.05 при v_esc < 6 км/с
// (калибровка Марс), иначе 1.
func escapeFactor(escapeVel float64) float64 {
	if escapeVel < 6 {
		return 0.05
	}
	return 1.0
}

// stripFactor — XUV-стриппинг у горячих звёзд (аудит «планет у O/B почти
// нет»): 1 при T_eff < 8000 K, 0.1 при 8000–15000 K, 0.01 при > 15000 K.
func stripFactor(tEff float64) float64 {
	switch {
	case tEff < 8000:
		return 1.0
	case tEff <= 15000:
		return 0.1
	default:
		return 0.01
	}
}

// ==================== СОСТАВ ГАЗОВ (99.2.20 §3.6 п.6) ====================

// atmosphereComposition — состав газов по режиму (массовые доли, 0–1;
// нормализуется к 1 вызывающим). life=true — жизненный проход (O₂, CO₂ ↓).
// Ключи: N2, O2, CO2, H2, He, CH4, H2O, SO2, NH3, Ar.
func atmosphereComposition(regime string, zone volatileZone, waterShare float64, life bool, rng *rand.Rand) map[string]float64 {
	switch regime {
	case "холодный":
		comp := map[string]float64{
			"CO2": 0.90 + rng.Float64()*0.05, // 0.90–0.95
			"N2":  0.03 + rng.Float64()*0.05, // 0.03–0.08
			"H2O": 0,                         // выморожена
		}
		if zone == zoneOuter {
			comp["CH4"] = rng.Float64() * 0.05 // 0–0.05 (внешняя зона)
		}
		return comp
	case "умеренный":
		if life {
			// Жизненный проход: фотосинтез (O₂, CO₂ ↓).
			return map[string]float64{
				"N2":  0.75 + rng.Float64()*0.05,  // 0.75–0.80
				"O2":  0.15 + rng.Float64()*0.07,  // 0.15–0.22
				"CO2": 0.001 + rng.Float64()*0.009, // 0.001–0.01
				"H2O": 0.010 * waterShare,
				"Ar":  0.01,
			}
		}
		return map[string]float64{
			"CO2": 0.30 + rng.Float64()*0.40, // 0.30–0.70
			"N2":  0.25 + rng.Float64()*0.40, // 0.25–0.65
			"H2O": 0.010 * waterShare,        // влажность (Земля water_share 0.7 → 0.007)
			"CH4": 0.001 + rng.Float64()*0.009, // 0.001–0.01
		}
	case "горячий":
		return map[string]float64{
			"CO2": 0.85 + rng.Float64()*0.12, // 0.85–0.97
			"N2":  0.03 + rng.Float64()*0.07, // 0.03–0.10
			"H2O": 0.0001,                    // истощена (фотолиз + утечка водорода)
			"SO2": 0.001 + rng.Float64()*0.009, // 0.001–0.01 (вулканическая)
		}
	}
	// Гиганты (для каменистых не вызывается).
	return map[string]float64{"H2": 0.9, "He": 0.1}
}

// normalizeComposition — нормализация массовых долей к сумме 1.
func normalizeComposition(comp map[string]float64) map[string]float64 {
	total := 0.0
	for _, v := range comp {
		total += v
	}
	if total <= 0 {
		return comp
	}
	out := make(map[string]float64, len(comp))
	for k, v := range comp {
		out[k] = v / total
	}
	return out
}

// ==================== ОПТИЧЕСКАЯ ТОЛЩИНА И МАСШТАБНАЯ ВЫСОТА ====================

// tauIR — оптическая толщина в ИК (99.2.20 §3.6 п.7):
// τ = P × (200·w_H2O + 1.0·w_CO2 + 1.2·w_CH4 + 3·w_N2O), w — массовые доли.
func tauIR(pressureAtm float64, comp map[string]float64) float64 {
	return pressureAtm * (kappaH2O*comp["H2O"] + kappaCO2*comp["CO2"] +
		kappaCH4*comp["CH4"] + kappaN2O*comp["N2O"])
}

// meanMolecularWeight — средняя молекулярная масса смеси (N₂ 28, O₂ 32,
// CO₂ 44, H₂O 18, H₂ 2, He 4, CH₄ 16, SO₂ 64, NH₃ 17, Ar 40, N₂O 44).
func meanMolecularWeight(comp map[string]float64) float64 {
	weights := map[string]float64{
		"N2": 28, "O2": 32, "CO2": 44, "H2O": 18, "H2": 2,
		"He": 4, "CH4": 16, "SO2": 64, "NH3": 17, "Ar": 40, "N2O": 44,
	}
	mu := 0.0
	for k, v := range comp {
		mu += v * weights[k]
	}
	if mu <= 0 {
		mu = 29
	}
	return mu
}

// scaleHeight — масштабная высота: H = 8.5 × (T/288) × (29/μ) × (1/g) км.
func scaleHeight(temp, mu, gravity float64) float64 {
	if gravity <= 0 {
		gravity = 1
	}
	return 8.5 * (temp / 288) * (29 / mu) * (1 / gravity)
}

// ==================== ПРОИЗВОДНЫЙ ЯРЛЫК (99.2.20 §3.6 п.9) ====================

// classifyAtmosphere — классификатор типа атмосферы по составу (массовые
// доли 0–1) и давлению. Порядок проверок — из спеки; все 14 существующих
// ярлыков покрыты (потребители ярлыка работают без изменений).
func classifyAtmosphere(comp map[string]float64, pressureAtm float64) string {
	pct := func(k string) float64 { return comp[k] * 100 }
	h2, he := pct("H2"), pct("He")
	if h2+he > 90 {
		switch {
		case h2 > 90:
			return "водородная"
		case h2 > he:
			return "водородно-гелиевая"
		default:
			return "гелиевая"
		}
	}
	if pct("CO2") > 80 {
		if pressureAtm > 5 {
			return "парниковая"
		}
		return "углекислая"
	}
	if pct("CH4") > 50 {
		return "метановая"
	}
	if pct("O2") > 15 && pct("N2") > 50 {
		return "азотно-кислородная"
	}
	if pct("N2") > 70 {
		return "азотная"
	}
	if pct("SO2")+pct("NH3")+pct("H2S") > 5 {
		return "ядовитая"
	}
	if pct("H2O") > 5 {
		if pressureAtm > 1 {
			return "облачная"
		}
		return "туманная"
	}
	if pressureAtm < 0.1 {
		return "разреженная"
	}
	if pressureAtm > 5 {
		return "плотная"
	}
	return "азотная"
}

// cloudFraction — облачная доля f_c по ярлыку атмосферы (99.2.20 §3.5,
// таблица). Тонкие атмосферы (P < 0.1 атм) облачного покрова не образуют
// (f_c = 0) — калибровка Марс-режима (T_final = T₀, §3.6 «Проверка»).
func cloudFraction(label string, pressureAtm float64) float64 {
	if pressureAtm < 0.1 {
		return 0
	}
	switch label {
	case "разреженная", "нет":
		return 0
	case "азотная":
		return 0.15
	case "азотно-кислородная":
		return 0.30
	case "облачная", "туманная":
		return 0.45
	case "углекислая", "плотная":
		return 0.60
	case "парниковая":
		return 0.90
	case "ядовитая":
		return 0.40
	case "метановая":
		return 0.35
	case "водородно-гелиевая", "водородная", "гелиевая":
		return 0.30
	}
	return 0
}