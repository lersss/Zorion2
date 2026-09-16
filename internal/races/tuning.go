// internal/races/tuning.go — подкрутка генератора под расу-дома кластера
// (спека 99.2.22 §3.3): производный объект RaceTuning, функция карточки.
//
// Ручки 1–5, 7, 7-холод выводятся из окон карточки (принцип «один источник»):
// 51-я раса с need-газом X получает обогащение X автоматически, без правок
// кода и конфига. Механизм общий, не per-газ: конкретные газы (NH₃/CH₄/H₂S)
// — данные карточки, а не код.
//
// Здесь живут только карточные параметры. Звёздно-зависимые величины
// (T_target₀, orbit_radius_mult, f_vol, возраст) считает планетный генератор
// (internal/generator/planet/race_tuning.go) по этим параметрам.
package races

import "math"

// Режимы атмосферы подкрутки (спека §3.3 ручка 1): границы T_target₀
// холод [20, 237], умер [237, 324], горяч [330, 500] (T₁ = T₀·1.1505:
// холод T₁ < 273, умер T₁ ∈ [273, 373], горяч T₁ > 373; нижний запас 330 —
// страховка от флипа режима).
const (
	RegimeCold      = "cold"
	RegimeTemperate = "temperate"
	RegimeHot       = "hot"

	// Калибровочные константы (спека §6): κ_eff — эффективный вес парника
	// состава режима (CO₂/CH₄-доминанта; H₂S/SO₂/NH₃ в τ_IR каскада не
	// входят — формула фиксирована, 99.2.20 §3.6). G(P) = (1 + ¾·τ)^0.25,
	// τ = P·κ_eff — калибровки каскада: Марс G≈1.0, Земля 287/255 = 1.125,
	// Венера-аналог 747/329 = 2.27.
	kappaCold      = 0.9 // холодный: CO₂ 0.90–0.95 (+CH₄ 0–0.05 внешняя зона)
	kappaTemperate = 0.5 // умеренный: CO₂ 0.30–0.70
	kappaHot       = 0.9 // горячий: CO₂ 0.85–0.97

	// Границы режимов T_target₀ (спека §6) — ЕДИНЫЙ ИСТОЧНИК: планетный
	// генератор (internal/generator/planet/race_tuning.go) ссылается на них
	// (races.RegimeColdLo и т.д.), дублирования нет.
	RegimeColdLo, RegimeColdHi = 20.0, 237.0
	RegimeTempLo, RegimeTempHi = 237.0, 324.0
	RegimeHotLo, RegimeHotHi   = 330.0, 500.0
	HotFloor                   = 330.0  // горячий пол (T₁ = 379.8 > 373)
	regimeBoundaryCold         = 273.0  // T₁ < 273 → холодный
	nominalGreenhouseFactor    = 1.1505 // 1.1505 = (1 + ¾·1.0)^0.25 (T₁-стиль)
)

// Tuning — производный объект подкрутки (спека §3.3): карточные параметры,
// из которых планетный генератор считает сдвиги входов каскада.
type Tuning struct {
	// Regime — режим атмосферы по T-окну: cold | temperate | hot.
	Regime string
	// TempCenter — центр opt-окна температуры (K), ручка 1.
	TempCenter float64
	// PLo, PHi — opt-окно давления (атм), ручка 3.
	PLo, PHi float64
	// Enrich — обогащение состава: газ → целевая доля (0–1), ручка 2.
	// Только need-газы с min > 0 и режим-совместимые (спека §3.3, фикс
	// итерации 3: «need: {X: 0}» = «не требуется»).
	Enrich map[string]float64
	// FIntTarget — целевой внутренний поток F_int_target (Вт/м²), ручка 4;
	// 0 = раса не тепловая (тепла не нужно).
	FIntTarget float64
	// SpectralMult — множители спектральных весов (класс → mult), ручка 5.
	SpectralMult map[string]float64
	// SurfaceDark — тёмная поверхность (A = 0.15), ручки 7/7-холод.
	SurfaceDark bool
}

// Tuning — производный объект подкрутки (функция карточки, кэшируется при
// загрузке каталога). Прямая конструкция Race без LoadCatalog — вывод на
// лету (чистая функция, безопасно).
func (r *Race) Tuning() *Tuning {
	if r.tuning != nil {
		return r.tuning
	}
	return deriveTuning(r)
}

// deriveTuning — вывод ручек из карточки (спека §3.3, числа §6).
// Оф-модельные расы (раса 40 Водородные, P «1000+»): планет в каскаде для
// них нет (99.2.21 §10) — подкрутка не нужна (дыра не маскируется), nil.
func deriveTuning(r *Race) *Tuning {
	if r.OffCascade {
		return nil
	}
	t := &Tuning{
		Enrich:       map[string]float64{},
		SpectralMult: map[string]float64{},
	}

	opt := r.Conditions.Temperature.Opt
	t.TempCenter = (opt.Lo + *opt.Hi) / 2
	t.PLo = r.Conditions.Pressure.Opt.Lo
	t.PHi = *r.Conditions.Pressure.Opt.Hi

	// Режим: горячий по центру окна; холодный/умеренный — по T₁ с холодным
	// составом (CO₂-доминанта): T₁ = center·1.1505/G(P, κ_cold). Глубинники
	// (окно [273, 300]) получают холодный режим: T₁ = 197.6 < 273 — режим
	// не флипает (смоук §14.8).
	if t.TempCenter > 373 {
		t.Regime = RegimeHot
	} else {
		p := logCenter(t.PLo, t.PHi)
		t1 := t.TempCenter * nominalGreenhouseFactor / GFactor(p, kappaCold)
		if t1 < regimeBoundaryCold {
			t.Regime = RegimeCold
		} else {
			t.Regime = RegimeTemperate
		}
	}

	// Ручка 2: обогащение — только need-газы с min > 0 и режим-совместимые
	// (холод: NH₃/CH₄; жар: H₂S/SO₂; умер: нет). Цель max(min, 5)%.
	allowed := map[string]bool{}
	switch t.Regime {
	case RegimeCold:
		allowed["NH3"], allowed["CH4"] = true, true
	case RegimeHot:
		allowed["H2S"], allowed["SO2"] = true, true
	}
	for gas, min := range r.Conditions.Atmosphere.Need {
		if min > 0 && allowed[gas] {
			t.Enrich[gas] = math.Max(min, 5) / 100
		}
	}

	// Ручка 4: тепло — F_int_target = heat_flux_window_lo × 3 (запас на
	// разброс массы √M ∈ [0.45, 2.83], спека §6).
	if r.Conditions.HeatFlux != nil && r.Conditions.HeatFlux.Opt.Lo > 0 {
		t.FIntTarget = r.Conditions.HeatFlux.Opt.Lo * 3
	}

	// Ручка 5: веса спектральных классов. Звёздный слой синергии
	// «звёзды+планеты» (решение создателя 2026-09-17, спека §1): home-классы
	// ×1.6 (диапазон ×1.3–2.0; усиление против ×1.3 v1 — звёзды «свои» чаще,
	// планетный слой умереннее). Мягко: остальные классы = 1.0, все веса > 0 —
	// инвариант «все типы возможны» (как профили 59a spectral_mult).
	// Тепловые расы (heat_flux Lo > 0), классификация по opt_hi:
	// умеренно-горячие (373 < opt_hi ≤ 600) — O/B ×1.3 (O/B-истончённые
	// атмосферы — естественный путь умеренно-горячих); очень горячие
	// (opt_lo ≥ 600) / окно через 600 / холодные тепловые (opt_hi ≤ 373) —
	// O/B нейтральны (дефолт итерации 3, спека §3.3 ручка 5).
	for _, cls := range r.Home.StarClasses {
		t.SpectralMult[cls] = 1.6
	}
	if r.Conditions.HeatFlux != nil && r.Conditions.HeatFlux.Opt.Lo > 0 {
		if optHi := r.Conditions.Temperature.Opt.Hi; optHi != nil && *optHi > 373 && *optHi <= 600 {
			t.SpectralMult["O"] = 1.3
			t.SpectralMult["B"] = 1.3
		}
	}

	// Ручки 7/7-холод: тёмная поверхность (A = 0.15) для холодных и горячих
	// (умеренные живут на жизненном проходе — оверрайд не нужен).
	t.SurfaceDark = t.Regime != RegimeTemperate

	return t
}

// KappaFor — κ_eff режима (спека §6): эффективный вес парника состава.
func KappaFor(regime string) float64 {
	switch regime {
	case RegimeCold:
		return kappaCold
	case RegimeTemperate:
		return kappaTemperate
	default:
		return kappaHot
	}
}

// GFactor — G(P) = (1 + ¾·P·κ_eff)^0.25: перевод T₀ → T_final (спека §6).
func GFactor(p, kappa float64) float64 {
	return math.Pow(1+0.75*p*kappa, 0.25)
}

// logCenter — лог-центр окна давления (спека §3.3 ручка 3: холод/умер —
// лог-центр окна).
func logCenter(lo, hi float64) float64 {
	return math.Sqrt(lo * hi)
}
