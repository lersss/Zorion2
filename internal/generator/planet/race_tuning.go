// internal/generator/planet/race_tuning.go — применение подкрутки генератора
// под расу-дома кластера (спека 99.2.22 §3.3–§4): сдвиг входов каскада.
//
// Двухслойная мягкость (§4): слой 1 (непрерывный) — число планет и веса
// звёзд (eff = 1 + (config−1)·s); слой 2 (вероятностный) — физические ручки
// (орбита, f_vol, состав, возраст, поверхность) на полную силу с
// вероятностью s (ролл в фиксированной позиции, early-exit при s ≤ 0 / s ≥ 1 —
// ролл не потребляет энтропию, «s = 0 → как есть» строго для того же seed).
//
// Каскад НЕ модифицируется: сдвигаются только входы (OrbitRadiusAU,
// FVolOverride, AgeGyr, CompositionOverride, SurfaceOverride).
package planet

import (
	"math"

	"zorion/internal/races"
)

// Константы подкрутки (спека §6): клампы и целевые уровни.
const (
	// Кламп orbit_radius_mult (спека §6, синергия «звёзды+планеты»,
	// решение создателя 2026-09-17): умеренный [0.35, 3.0] — планета
	// подстраивается «чуть-чуть» (добирает остаток до окна, а не тащит
	// с орбиты 1 на орбиту 8): звезда уже «своя» (звёздный слой сдвигает
	// веса к home.star_classes), планетный слой лишь доводит. Симметрия по
	// температуре: T ∝ 1/√mult → диапазон [0.577, 1.69]× вокруг 1.0.
	// Верх 3.0 — холодные добирают с ближних орбит (метановые с орбит 4–7);
	// низ 0.35 — горячие добирают до горячего пола T_target₀ = 330
	// (T₁ = 379.8 > 373 — режим не флипает в умеренный, иначе секвестрация
	// ×0.006 убивает давление): на орбите 2 нужен mult 0.568, на орбите 3 —
	// 0.334. В v1/v2 был [0.5, 14.0] — сильный сдвиг (каждый мир кластера
	// «выкручивался» полностью); при синергии верх ужат в 4.7×.
	orbitMultMin = 0.35
	orbitMultMax = 3.0

	// Кламп f_vol — глобальный [10⁻⁶, 0.3] (спека §3.6): пол 10⁻⁶ открывает
	// тонкие горячие атмосферы (умеренно-горячий путь), потолок 0.3 —
	// физический максимум (ледяные миры).
	fVolMin = 1e-6
	fVolMax = 0.3

	// Возраст тепловых рас: кламп [0.1, 0.499] (спека §6; при age = 0.5
	// F_accretion = 0 — тепло пропадает).
	ageMin = 0.1
	ageMax = 0.499

	// 1.16·10⁶ = 5.97·10²⁴/5.15·10¹⁸ (массы Земли/атмосферы Земли, спека §3.3
	// ручка 3; в v1 была ошибка 1160 — в 1000× меньше).
	earthMassPerAtmMass = 1.16e6

	// Разбавление парника (горячий режим): w_CO2 = clamp(τ_target/P_window_lo,
	// 0.05, 0.9) (спека §3.3 ручка 2).
	diluteCO2Min = 0.05
	diluteCO2Max = 0.9

	// Границы режимов T_target₀ (спека §6): холод [20, 237], умер [237, 324],
	// горяч [330, 500] (T₁ = T₀·1.1505: холод T₁ < 273, умер T₁ ∈ [273, 373],
	// горяч T₁ > 373; нижний запас 330 — страховка от флипа режима).
	regimeColdLo, regimeColdHi = 20.0, 237.0
	regimeTempLo, regimeTempHi = 237.0, 324.0
	regimeHotLo, regimeHotHi   = 330.0, 500.0
	hotFloor                   = 330.0 // горячий пол (T₁ = 379.8 > 373)
)

// raceTuning — производный объект подкрутки текущего мира (nil — нет расы
// или каталог не загружен). Легаси-вселенные (регионы без race_id) и
// фоновые регионы — nil (подкрутки нет, нейтрально как profile nil).
func (g *Generator) raceTuning() *races.Tuning {
	if g.raceID == "" {
		return nil
	}
	r := races.ByID(g.raceID)
	if r == nil {
		return nil
	}
	return r.Tuning()
}

// raceTuned — слой 2: ролл «планета подстроена» (спека §4.3). Early-exit:
// при s ≤ 0 — «не подстроена» без потребления энтропии; при s ≥ 1 —
// «подстроена» без потребления энтропии. Иначе один Float64 — детерминировано
// по seed. Ролл в фиксированной позиции (до каскада, в функции подкрутки).
func (g *Generator) raceTuned() bool {
	if g.raceSoftness <= 0 {
		return false
	}
	if g.raceSoftness >= 1 {
		return true
	}
	return g.rng.Float64() < g.raceSoftness
}

// raceTunePlanet — применяет подкрутку расы к одной планете (слой 2).
// rNat — естественный радиус орбиты (а.е.): для S-планет orbitRadiusScaled,
// для P-планет r_P = 3·companion_sep_au (99.2.18). shiftOrbit — применять ли
// ручку 1 (сдвиг орбиты): для P-планет НЕТ (r_P фиксирован разделением пары,
// спека §3.3) — f_vol/возраст считаются по rNat без сдвига. Возвращает nil,
// если планета не подстроена (нет расы / ролл не прошёл / s ≤ 0). Ролл — в
// фиксированной позиции (early-exit при s ≤ 0 / s ≥ 1).
func (g *Generator) raceTunePlanet(sp StellarParams, rNat float64, shiftOrbit bool) *raceTune {
	t := g.raceTuning()
	if t == nil || !g.raceTuned() {
		return nil
	}

	rt := &raceTune{}

	// M_nom — номинальная масса аккреции без ζ и без B (спека §3.3 ручка 3/4):
	// единый источник — ядро каскада (coreMass, §5.4 спеки 2026-09-21).
	// Для H_int и T_target₀ — на естественной орбите (до сдвига); для f_vol и
	// возраста — на сдвинутой (каскад аккрецирует массу на in.OrbitRadiusAU —
	// сдвинутой орбите, иначе давление систематически мимо P_target).
	mNomNat := coreMass(raceNominalANorm(rNat, sp.Luminosity, shiftOrbit), sp.Metallicity)
	sqrtMNat := math.Sqrt(mNomNat)

	// H_int — поправка на внутреннее тепло (спека §6): тепловые расы
	// перегревались (+30…+80 K) — F_int входит в T⁴-баланс каскада.
	hInt := 1.0
	if t.FIntTarget > 0 {
		fStarNat := solarFluxAt1AU * sp.Luminosity / (rNat * rNat)
		hInt = math.Pow(1+t.FIntTarget*sqrtMNat/(fStarNat*0.85), 0.25)
	}

	// Ручка 3: целевое давление (холод/умер — лог-центр окна; горячий —
	// решение из T-окна и орбиты, спека §3.3).
	pTarget := racePTarget(t, hInt)

	// Ручка 1: целевая до-парниковая температура и сдвиг орбиты.
	tTarget0 := raceTTarget0(t, pTarget, hInt)
	t0Base := tempFromFlux(solarFluxAt1AU*sp.Luminosity/(rNat*rNat), 0.15, 0, 0, 0)
	mult := (t0Base / tTarget0) * (t0Base / tTarget0)
	rt.orbitMult = clamp(mult, orbitMultMin, orbitMultMax)

	// Радиус для f_vol/возраста: сдвинутый (S-планеты) или естественный
	// (P-планеты — сдвиг орбиты не применяется).
	rEff := rNat
	if shiftOrbit {
		rEff = rNat * rt.orbitMult
	} else {
		rt.orbitMult = 0
	}
	mNom := coreMass(raceNominalANorm(rEff, sp.Luminosity, shiftOrbit), sp.Metallicity)
	sqrtM := math.Sqrt(mNom)
	rt.nominalMass = mNom

	// Ручка 4: тепло — адаптивный возраст (заменяет determineSystemAge).
	if t.FIntTarget > 0 {
		age := 0.5 * (1 - t.FIntTarget/(1000*sqrtM))
		rt.ageGyr = clamp(age, ageMin, ageMax)
	}

	// Ручка 3: бюджет летучих f_vol = f_vol_target × ζ_fvol (ζ ~ logN(0, 0.3)
	// — один ролл на месте, спека §3.3 ручка 3).
	if pTarget > 0 {
		zone := volatileZoneFor(rEff, sp.Luminosity)
		rhoNom := nominalDensity(zone, mNom)
		rNom := math.Cbrt(mNom / rhoNom)
		gNom := mNom / (rNom * rNom)
		seq := sequestrationFactor(regimeName(t.Regime))
		esc := escapeFactor(escapeVelocity(mNom, rNom))
		strip := stripFactor(sp.TEff)
		fVol := pTarget / (seq * esc * strip * mNom * earthMassPerAtmMass * gNom / (rNom * rNom))
		rt.fVol = clamp(fVol, fVolMin, fVolMax) * math.Exp(0.3*g.rng.NormFloat64())
	}

	// Ручки 7/7-холод: тёмная поверхность (A = 0.15 = базовое альбедо
	// T₀_base → t0_actual = T_target₀ точно, спека §3.5а).
	if t.SurfaceDark {
		rt.surface = Composition{SurfaceRocks: 100}
	}

	// Ручка 2: состав — обогащение + разбавление парника (горячий).
	if len(t.Enrich) > 0 || t.Regime == races.RegimeHot {
		rt.composition = compositionOverride(t, dilutionCO2(t, tTarget0))
		rt.compositionRegime = regimeName(t.Regime)
	}

	return rt
}

// raceTune — результат применения подкрутки к одной планете (входы каскада).
type raceTune struct {
	orbitMult         float64     // ручка 1: множитель орбиты (1.0 = без сдвига)
	fVol              float64     // ручка 3: бюджет летучих (0 = каскад сам)
	ageGyr            float64     // ручка 4: возраст (0 = каскад сам)
	nominalMass       float64     // номинал ядра без ζ и B — единый источник (§5.4)
	surface           Composition // ручки 7/7-холод: оверрайд поверхности (nil = нет)
	composition       Composition // ручка 2: оверрайд состава (nil = нет)
	compositionRegime string      // ожидаемый режим оверрайда состава
}

// raceNominalANorm — a_норм для номинала подкрутки: S-ветка r/√L (shiftOrbit);
// P-ветка (shiftOrbit = false) — физическое r_P, нормализация √L не применима
// (§4.1 спеки 2026-09-21-масса-каменистых-и-ледяных-планет).
func raceNominalANorm(r, luminosity float64, shiftOrbit bool) float64 {
	if shiftOrbit {
		return aNormOf(r, luminosity)
	}
	return r
}

// racePTarget — целевое давление (ручка 3, спека §3.3): холодный/умеренный —
// лог-центр окна; горячий — решение из T-окна и орбиты:
// P_target = clamp(((4/3)·((center/(T_target₀·H_int))⁴ − 1))/κ_eff, lo, hi),
// где T_target₀ = 330 (горячий пол) — уравнения P_target и T_target₀
// самосогласованы (подстановка P_target в G(P_target) даёт T_final = center).
func racePTarget(t *races.Tuning, hInt float64) float64 {
	if t.Regime == races.RegimeHot {
		kappa := races.KappaFor(t.Regime)
		p := (4.0 / 3.0) * (math.Pow(t.TempCenter/(hotFloor*hInt), 4) - 1) / kappa
		return clamp(p, t.PLo, t.PHi)
	}
	return math.Sqrt(t.PLo * t.PHi)
}

// raceTTarget0 — целевая до-парниковая температура (ручка 1, спека §3.3):
// T_target₀ = clamp(center(opt)/(G(P_target)·H_int), floor_regime, ceil_regime).
func raceTTarget0(t *races.Tuning, pTarget, hInt float64) float64 {
	g := races.GFactor(pTarget, races.KappaFor(t.Regime))
	t0 := t.TempCenter / (g * hInt)
	switch t.Regime {
	case races.RegimeCold:
		return clamp(t0, regimeColdLo, regimeColdHi)
	case races.RegimeTemperate:
		return clamp(t0, regimeTempLo, regimeTempHi)
	default:
		return clamp(t0, regimeHotLo, regimeHotHi)
	}
}

// dilutionCO2 — целевая доля CO₂ при разбавлении парника (ручка 2, горячий
// режим, спека §3.3): если P_target из окна давления требует слабого парника
// (τ_target/P_window_lo < 1.0), состав сдвигается к N₂-доминанте:
// w_CO2 = clamp(τ_target/P_window_lo, 0.05, 0.9), где τ_target — парник,
// нужный для достижения центра окна от T_target₀:
// τ_target = ((center/T_target₀)⁴ − 1)/0.75. 0 = разбавления нет.
func dilutionCO2(t *races.Tuning, tTarget0 float64) float64 {
	if t.Regime != races.RegimeHot {
		return 0
	}
	tauReq := (math.Pow(t.TempCenter/tTarget0, 4) - 1) / 0.75
	if tauReq/t.PLo >= 1.0 {
		return 0
	}
	return clamp(tauReq/t.PLo, diluteCO2Min, diluteCO2Max)
}

// compositionOverride — состав атмосферы под расу (ручка 2, спека §3.3):
// обогащение need-газов (доли = target) + разбавление парника (горячий:
// CO₂ = w_CO2, остальное N₂ + need-газы). Сумма = 1 (инвариант §7.4).
// Возвращает nil, если состав не меняется (нет обогащения и нет разбавления).
func compositionOverride(t *races.Tuning, diluteCO2 float64) Composition {
	if diluteCO2 > 0 {
		comp := Composition{"CO2": diluteCO2}
		sum := diluteCO2
		for gas, v := range t.Enrich {
			comp[gas] = v
			sum += v
		}
		comp["N2"] = 1 - sum
		return comp
	}
	if len(t.Enrich) == 0 {
		return nil
	}
	// Без разбавления — только цели обогащения: доли газа = target,
	// остальные масштабируются в каскаде (applyCompositionOverride).
	return Composition(t.Enrich)
}

// nominalDensity — номинальная плотность зоны (спека §3.3 ручка 3): ρ_nom —
// по серединам диапазонов состава compositionByZone (99.2.20 §3.3) без роллов.
func nominalDensity(zone volatileZone, mass float64) float64 {
	var rock, iron, ice float64
	switch zone {
	case zoneInner:
		rock, iron, ice = 0.575, 0.325, 0.05
	case zoneTransition:
		rock, iron, ice = 0.60, 0.15, 0.20
	default:
		rock, iron, ice = 0.40, 0.10, 0.50
	}
	return planetDensity(rock, iron, ice, mass)
}

// regimeName — имя режима каскада (atmosphereRegime: «холодный»/«умеренный»/
// «горячий») по режиму подкрутки. Используется и для sequestrationFactor
// (умеренный/холодный — одна ветка секвестрации ×0.006), и для проверки
// режима оверрайда состава (CompositionRegime).
func regimeName(regime string) string {
	switch regime {
	case races.RegimeCold:
		return "холодный"
	case races.RegimeTemperate:
		return "умеренный"
	default:
		return "горячий"
	}
}

// applyCompositionOverride — обогащение состава в каскаде: доли газа = target,
// остальные масштабируются, сумма = 1 (спека §3.3 ручка 2, инвариант §7.4).
// override — полный состав (сумма 1, разбавление) или частичный (только цели).
func applyCompositionOverride(override, base Composition) Composition {
	targetSum := 0.0
	for _, v := range override {
		targetSum += v
	}
	if targetSum >= 1 {
		return normalizeComposition(override)
	}
	rest := 1 - targetSum
	baseSum := 0.0
	for k, v := range base {
		if _, isTarget := override[k]; !isTarget {
			baseSum += v
		}
	}
	out := make(Composition, len(base)+len(override))
	for k, v := range override {
		out[k] = v
	}
	if baseSum > 0 {
		for k, v := range base {
			if _, isTarget := override[k]; !isTarget {
				out[k] = v / baseSum * rest
			}
		}
	}
	return out
}
