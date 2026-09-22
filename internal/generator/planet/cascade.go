// internal/generator/planet/cascade.go
//
// Физический каскад генератора планет (99.2.20 §2–§3): звезда → орбита →
// планета → ядро → T⁴-энергобаланс → атмосфера → флаг жидкой воды →
// поверхность → вода/жизнь/классификация. Физика вычисляется первым слоем,
// косметика (композиция, ярлыки) — вторым и не может противоречить физике.
//
// Слои и формулы — спека 99.2.20; числа не «улучшаются» (эталон создателя).
package planet

import (
	"math"
	"math/rand"

	"zorion/internal/models"
	"zorion/internal/races"
	"zorion/internal/regionprofile"
)

// ==================== КОНСТАНТЫ КАСКАДА (99.2.20 §3) ====================

const (
	// Солнечная постоянная на 1 а.е. (Вт/м²) — вход F★ = 1361·L/r².
	solarFluxAt1AU = 1361.0
	// Постоянная Стефана–Больцмана (Вт/м²·K⁴).
	stefanBoltzmann = 5.67e-8
	// Масса атмосферы Земли (кг) — нормировка давления.
	earthAtmMassKg = 5.15e18
	// Масса Земли (кг) — нормировка массы атмосферы.
	earthMassKg = 5.97e24

	// Фазовая диаграмма воды (99.2.20 §3.7).
	waterFreezeK      = 273.0
	waterBoilAt1AtmK  = 373.0
	waterMinPressure  = 0.006 // атм — тройная точка
	waterBoilSlope    = 30.2  // Клаузиус–Клапейрон: T_кип = 373 + 30.2·ln(P)

	// Приливный захват: P_orb < 10 сут (≈ 0.0274 года).
	tidalLockThresholdYears = 10.0 / 365.25

	// Веса κ парниковых газов (99.2.20 §3.6 п.7): τ_IR = P × Σ κ·w.
	kappaH2O = 200.0
	kappaCO2 = 1.0
	kappaCH4 = 1.2
	kappaN2O = 3.0

	// Номинальный парник предварительной атмосферы (99.2.20 §3.5 п.2).
	nominalTau = 1.0

	// Границы массы каменистых/ледяных планет (спека
	// 2026-09-21-масса-каменистых-и-ледяных-планет §5): пол — страховка
	// реестра (генератором не достигается), потолок — граница ветки
	// газовых гигантов (эталон 99.2.15).
	massMin = 0.02 // M⊕
	massMax = 8.0  // M⊕

	// Анкер и показатель аннулярной массы диска (§4 спеки 2026-09-21):
	// a_⊕ = a₂ = 1.156 а.е. — орбита 2 солнцеподобной звезды = 1 M⊕;
	// β = 0.5 — середина физического коридора [0.4, 0.6].
	massAnchorAU = 1.156
	massBeta     = 0.5

	// Обрезка роста гигантом-соседом (f_обр, §4.1/§4.6 спеки
	// 2026-09-21-протопылевое-облако-архитектура-и-масса, этап 1):
	// f_обр = 0.08, если гигант есть (giantOrbit > 0) и планета в пределах
	// двух орбит от него (|orbitIndex − giantOrbit| ≤ 2), иначе 1.
	// Единственный якорь калибровки — Марс ярдстика:
	// 0.107 / M_ядро(орб. 3) = 0.082 → 0.08.
	massTruncationFactor = 0.08
)

// ==================== СЛОЙ 2 — ОРБИТА (99.2.20 §3.2) ====================

// orbitRadiusScaled — радиус орбиты по индексу с масштабом √L:
// r_i = 0.4 × 1.7^i × √L. G-звёзды (√1 = 1) — без изменений (нулевая
// регрессия); M-звёзды получают тесные обитаемые орбиты, O/B — честные
// далёкие (стерилизация XUV-стриппингом, не нагревом).
func orbitRadiusScaled(orbitIndex int, luminosity float64) float64 {
	return orbitRadiusByIndex(orbitIndex) * math.Sqrt(luminosity)
}

// orbitalPeriod — период Кеплера III: P_orb = √(r³/M★) лет.
func orbitalPeriod(orbitRadiusAU, stellarMass float64) float64 {
	if stellarMass <= 0 {
		stellarMass = 1.0
	}
	return math.Sqrt(orbitRadiusAU*orbitRadiusAU*orbitRadiusAU / stellarMass)
}

// ==================== СЛОЙ 3 — ПЛАНЕТА (99.2.20 §3.3) ====================

// coreMass — ядро массы по аккреции без случайных множителей (§4 спеки
// 2026-09-21): M_ядро = M₀·(a_норм/a_⊕)^β·10^(0.5·[Fe/H]), M₀ = 1 M⊕.
// Единый источник каскада и номинала подкрутки рас (§5.4, K1).
func coreMass(aNorm, metallicity float64) float64 {
	return math.Pow(aNorm/massAnchorAU, massBeta) * math.Pow(10, 0.5*metallicity)
}

// aNormOf — нормированное расстояние a_норм = r/√L (S-планеты, 99.2.20 §3.2):
// «номер орбиты = одни условия» для всех классов (инсоляция, снеговая линия,
// режим атмосферы). Для P-планет нормализация √L не применима (барицентр,
// §4.1 спеки 2026-09-21) — вызывающий передаёт физическое r_P напрямую.
func aNormOf(orbitRadiusAU, luminosity float64) float64 {
	if luminosity <= 0 {
		return orbitRadiusAU
	}
	return orbitRadiusAU / math.Sqrt(luminosity)
}

// accretionMass — масса по аккреции (заменяет рулетку архетипа):
// M_ядро = M₀·(a_норм/a_⊕)^0.5·10^(0.5·[Fe/H])·ζ, ζ ~ logN(0, 0.6).
// Чистая функция без RNG (§8 T6/T8); ролл ζ — на месте вызова (runCascade).
// Возвращает СЫРОЕ ядро: кламп [0.02, 8] применяется ОДИН раз — к
// произведению B·M_ядро (§4), в runCascade. Внутренний кламп ядра убран
// (ревью 2026-09-21: двойной кламп занижал верхнюю метку 2.0% → 1.2%).
// Светимость в массу не входит — каскад самоподобен по √L (§4.2 п.1).
func accretionMass(aNorm, metallicity, zeta float64) float64 {
	return coreMass(aNorm, metallicity) * zeta
}

// cloudProfileSum — S₀ (спека поясов малых тел §4.0.1): нормировочная сумма
// профиля M_ядро = c_i по ВСЕЙ лестнице орбит (i = 1..N_max, N_max = 8 —
// потолок PlanetMeans.Max / Kepler-90). Фиксированный референс: от
// реализованной системы не зависит, поэтому Σ_{занятые} w_i ≤ 1 при любом
// n ≤ N_max. При median(M_диск) = S₀ нормировка профиля (w_i = c_i/S₀) и
// пере-калибровка приора взаимно сокращаются — медианы ярдстика
// сохраняются тождественно (§4.0.1). Число ≈ 18.551 — на @balancetester.
var cloudProfileSum = func() float64 {
	sum := 0.0
	const ladderMax = 8 // N_max = Max = 8 (PlanetMeans.Max)
	for i := 1; i <= ladderMax; i++ {
		sum += coreMass(orbitRadiusByIndex(i), 0)
	}
	return sum
}()

// retentionFactor — обрезка роста гигантом-соседом (f_обр, §4.1 спеки
// 2026-09-21-протопылевое-облако-архитектура-и-масса, этап 1): 0.08, если
// гигант есть и планета в пределах двух орбит от него, иначе 1. Чистая
// детерминированная функция от уже принятых per-системных решений — роллов
// не добавляет, поток RNG не сдвигает. Самоподобие по √L сохранено:
// аргумент — номера орбит, а не физическое r (f_обр — «обрезка по щели
// гиганта», света гиганта/звезды не входит).
func retentionFactor(orbitIndex, giantOrbit int) float64 {
	if giantOrbit <= 0 {
		return 1
	}
	d := orbitIndex - giantOrbit
	if d < 0 {
		d = -d
	}
	if d <= 2 {
		return massTruncationFactor
	}
	return 1
}

// compositionByZone — объёмный состав (породы/железо/лёд) по зоне снеговой
// линии r_ice = 2.7·√L. Доли нормализуются к 1 (99.2.20 §3.3, таблица зон).
func compositionByZone(orbitRadiusAU, luminosity float64, rng *rand.Rand) (rock, iron, ice float64) {
	rIce := 2.7 * math.Sqrt(luminosity)
	switch {
	case orbitRadiusAU < 0.7*rIce:
		// Внутренняя: породы 0.40–0.75, железо 0.15–0.50, лёд 0–0.10.
		rock = 0.40 + rng.Float64()*0.35
		iron = 0.15 + rng.Float64()*0.35
		ice = rng.Float64() * 0.10
	case orbitRadiusAU <= 1.5*rIce:
		// Переходная: 0.50–0.70 / 0.10–0.20 / 0.10–0.30.
		rock = 0.50 + rng.Float64()*0.20
		iron = 0.10 + rng.Float64()*0.10
		ice = 0.10 + rng.Float64()*0.20
	default:
		// Внешняя: 0.30–0.50 / 0.05–0.15 / 0.40–0.60.
		rock = 0.30 + rng.Float64()*0.20
		iron = 0.05 + rng.Float64()*0.10
		ice = 0.40 + rng.Float64()*0.20
	}
	total := rock + iron + ice
	if total <= 0 {
		return 0.7, 0.25, 0.05
	}
	return rock / total, iron / total, ice / total
}

// planetDensity — плотность от объёмного состава + гравитационного сжатия
// (заменяет «плотность от формы поверхности»):
// ρ = (0.49·w_rock + 2.0·w_iron + 0.17·w_ice) × (1 + 0.04·log10 M).
func planetDensity(rock, iron, ice, mass float64) float64 {
	rhoMix := 0.49*rock + 2.0*iron + 0.17*ice
	return rhoMix * (1 + 0.04*math.Log10(mass))
}

// escapeVelocity — скорость убегания: 11.2 × √(M/R) км/с (99.2.20 §4.1).
func escapeVelocity(mass, size float64) float64 {
	if size <= 0 {
		size = 1
	}
	return 11.2 * math.Sqrt(mass/size)
}

// ==================== СЛОЙ 4 — ЯДРО: ВНУТРЕННИЙ ПОТОК (99.2.20 §3.4) ====================

// internalHeatFlux — внутренний тепловой поток F_int (Вт/м²):
// радиогенный (экспоненциальное затухание, период 2.5 млрд лет) +
// аккреционный (только каменистые/ледяные, age < 0.5 млрд лет) +
// Кельвина–Гельмгольца (только газовые гиганты). Зрелые каменистые:
// вклад в T < 0.1 K («обнуление для зрелых» — цель аудита @Scientist).
func internalHeatFlux(core Core, ageGyr, mass float64, isGasGiant bool) float64 {
	fRadiogenic := 0.2 * (core.Radioactivity / 100) * math.Pow(2, -ageGyr/2.5)
	fAccretion := 0.0
	if !isGasGiant {
		fAccretion = 1000 * math.Max(0, 1-ageGyr/0.5) * math.Sqrt(mass)
	}
	fKH := 0.0
	if isGasGiant {
		fKH = 13 * math.Pow(mass/317.8, 2.5) * math.Max(0.1, math.Sqrt(1/ageGyr))
	}
	return fRadiogenic + fAccretion + fKH
}

// ==================== СЛОЙ 5 — T⁴-ЭНЕРГОБАЛАНС (99.2.20 §3.5) ====================

// tempFromFlux — T⁴-энергобаланс:
// T⁴ = (F★(1−A) + F_int + F_tidal)/(4σ) × (1 + ¾τ_IR), кламп [20, 2500].
// Архетип-клампы отменены (99.2.11): остаются только абсолютные границы.
func tempFromFlux(fStar, albedo, fInt, fTidal, tau float64) float64 {
	flux := fStar*(1-albedo) + fInt + fTidal
	t := math.Pow(flux/(4*stefanBoltzmann), 0.25) * math.Pow(1+0.75*tau, 0.25)
	return clamp(t, TempAbsoluteMin, TempAbsoluteMax)
}

// ==================== СЛОЙ 7 — ФЛАГ ЖИДКОЙ ВОДЫ (99.2.20 §3.7) ====================

// liquidWaterPossible — фазовая проверка воды при (T, P):
// P ≥ 0.006 ∧ 273 < T < 373 + 30.2·ln(P) (Клаузиус–Клапейрон).
func liquidWaterPossible(temp, pressureAtm float64) bool {
	if pressureAtm < waterMinPressure {
		return false
	}
	tBoil := waterBoilAt1AtmK + waterBoilSlope*math.Log(pressureAtm)
	return temp > waterFreezeK && temp < tBoil
}

// ==================== ПОЛОСЫ КОМПОЗИЦИИ (99.2.20 §5) ====================

// cascadeBand — полоса композиции каскада: базовые веса, allowed-списки
// и шансы из конфига архетипов (или дефолт). Поля weight/temperature_min/max/
// mass_min/max конфига не читаются (физика решает; рулетка отменена).
type cascadeBand struct {
	id             string
	baseSurface    map[string]float64
	baseSubterrain map[string]float64
	hydrospheres   []string
	biospheres     []string
	waterChance    float64
	lifeChance     float64
}

// bandForTemp — полоса по температуре (99.2.20 §5: полосы 250/350/500).
// «Изменчивый» из генерации выведен (остаётся в конфиге как допустимое
// значение, больше не выдаётся).
func bandForTemp(temp float64) cascadeBand {
	switch {
	case temp >= 500:
		return cascadeBandByID("экстремальный")
	case temp >= 350:
		return cascadeBandByID("жаркий")
	case temp >= 250:
		return cascadeBandByID("умеренный")
	default:
		return cascadeBandByID("холодный")
	}
}

// cascadeBandByID — полоса из конфига архетипов (или дефолт).
func cascadeBandByID(id string) cascadeBand {
	archetypeMu.RLock()
	cache := archetypeCache
	archetypeMu.RUnlock()
	if cache != nil {
		for i := range cache.Climates {
			c := &cache.Climates[i]
			if c.ID != id {
				continue
			}
			return cascadeBand{
				id:             c.ID,
				baseSurface:    copyWeights(c.BaseSurface),
				baseSubterrain: copyWeights(c.BaseSubterrain),
				hydrospheres:   append([]string(nil), c.AllowedHydrospheres...),
				biospheres:     append([]string(nil), c.AllowedBiospheres...),
				waterChance:    c.WaterChance,
				lifeChance:     c.LifeChance,
			}
		}
	}
	return fallbackCascadeBand(id)
}

// fallbackCascadeBand — дефолтные полосы (значения совпадают с
// config/planet_archetypes.json; используются, пока конфиг не загружен).
func fallbackCascadeBand(id string) cascadeBand {
	switch id {
	case "экстремальный":
		return cascadeBand{
			id: "экстремальный",
			baseSurface: map[string]float64{
				SurfaceLavaFields: 0.25, SurfaceVolcanicFields: 0.2,
				SurfaceRocks: 0.2, SurfaceGlassFields: 0.15,
				SurfaceMetalFields: 0.1, SurfaceCraters: 0.1,
			},
			baseSubterrain: map[string]float64{
				SubterrainEmptyRock: 0.15, SubterrainMagmaticRocks: 0.25,
				SubterrainMagmaChambers: 0.15, SubterrainOreVeins: 0.15,
				SubterrainRadioactiveZones: 0.15, SubterrainMetalCores: 0.15,
			},
			hydrospheres: []string{"кислотная", "сухая"},
			biospheres:   []string{"стерильная"},
			waterChance:  0, lifeChance: 0,
		}
	case "жаркий":
		return cascadeBand{
			id: "жаркий",
			baseSurface: map[string]float64{
				SurfaceRocks: 0.2, SurfaceSands: 0.4, SurfaceGlassFields: 0.1,
				SurfaceLavaFields: 0.08, SurfaceVolcanicFields: 0.07,
				SurfaceJungles: 0.05, SurfaceCraters: 0.1,
			},
			baseSubterrain: map[string]float64{
				SubterrainEmptyRock: 0.25, SubterrainMagmaticRocks: 0.2,
				SubterrainMetamorphicRocks: 0.1, SubterrainOreVeins: 0.15,
				SubterrainMagmaChambers: 0.08, SubterrainRadioactiveZones: 0.12,
				SubterrainCrystalVeins: 0.1,
			},
			hydrospheres: []string{"сухая", "кислотная"},
			biospheres:   []string{"стерильная", "микробная"},
			waterChance:  0.2, lifeChance: 0.1,
		}
	case "умеренный":
		return cascadeBand{
			id: "умеренный",
			baseSurface: map[string]float64{
				SurfaceRocks: 0.13, SurfaceSands: 0.05, SurfaceLakes: 0.10,
				SurfaceOceans: 0.13, SurfaceMeadows: 0.16, SurfaceForests: 0.20,
				SurfaceJungles: 0.12, SurfaceSwamps: 0.07, SurfaceCraters: 0.04,
			},
			baseSubterrain: map[string]float64{
				SubterrainEmptyRock: 0.25, SubterrainSedimentaryRocks: 0.2,
				SubterrainMagmaticRocks: 0.1, SubterrainOreVeins: 0.1,
				SubterrainGroundwater: 0.15, SubterrainCoalSeams: 0.1,
				SubterrainOilPockets: 0.05, SubterrainCrystalVeins: 0.05,
			},
			hydrospheres: []string{"океаны", "озёра", "подлёдная", "сухая"},
			biospheres:   []string{"растительная", "симбиотическая", "светящаяся", "микробная"},
			waterChance:  0.7, lifeChance: 0.7,
		}
	default: // холодный
		return cascadeBand{
			id: "холодный",
			baseSurface: map[string]float64{
				SurfaceRocks: 0.25, SurfaceGlaciers: 0.35, SurfaceFrozenGases: 0.15,
				SurfaceCraters: 0.15, SurfaceLakes: 0.1,
			},
			baseSubterrain: map[string]float64{
				SubterrainEmptyRock: 0.3, SubterrainMagmaticRocks: 0.15,
				SubterrainMetamorphicRocks: 0.15, SubterrainGroundIce: 0.2,
				SubterrainCrystalVeins: 0.1, SubterrainOreVeins: 0.1,
			},
			hydrospheres: []string{"ледяной покров", "подлёдная", "сухая"},
			biospheres:   []string{"стерильная", "микробная"},
			waterChance:  0.3, lifeChance: 0.05,
		}
	}
}

// ==================== ВХОДЫ И РЕЗУЛЬТАТ КАСКАДА ====================

// cascadeInput — входы физического каскада. Нулевые значения — «вычислить
// каскадом» (роллы), явные — оверрайды (калибровки §3.6, прототип §6.9).
type cascadeInput struct {
	// Слой 1 — звезда.
	Luminosity  float64 // L☉
	StellarMass float64 // M☉
	AgeGyr      float64 // млрд лет
	Metallicity float64 // [Fe/H]
	TEff        float64 // K

	// Слой 2 — орбита.
	OrbitRadiusAU float64 // а.е.
	OrbitIndex    int
	// Circumbinary — P-планета тесной двойной (99.2.18): a_норм = r_P
	// (физическое расстояние) — нормализация √L не применима (§4.1 спеки
	// 2026-09-21-масса-каменистых-и-ледяных-планет).
	Circumbinary bool

	// Оверрайды калибровок (§3.6): 0/пустое — вычислить каскадом.
	MassOverride       float64     // M⊕
	SizeOverride       float64     // R⊕
	SurfaceOverride    Composition // для альбедо T₀
	FVolOverride       float64     // доля летучих
	WaterShareOverride float64     // доля воды в летучих
	ForceLife          bool        // форсировать жизнь (прототип §6.9)

	// Оверрайд состава атмосферы (99.2.22 §3.3 ручка 2): обогащение
	// need-газов + разбавление парника. Применяется только если фактический
	// режим (по T₁ с оверрайдом поверхности) совпадает с CompositionRegime
	// (честный отказ при несовпадении — мир остаётся нетюнингованным).
	CompositionOverride Composition // доли (0–1); пустое — нет оверрайда
	CompositionRegime   string      // ожидаемый режим: "холодный"/"умеренный"/"горячий"

	// Оверрайды прототипа поселения (99.2.28 §16.1): проводятся через каскад,
	// чтобы биомы были согласованы с форсированными данными. 0 = нет оверрайда.
	FinalTempOverride    float64 // применяется после жизненного прохода; флаг пересчитывается
	WaterPercentOverride float64 // применяется до слоя 8
}

// cascadeResult — результат физического каскада.
type cascadeResult struct {
	Mass, Size, Density, Gravity float64
	EscapeVelocity               float64
	Core                         Core
	HeatFluxWm2                  float64
	T0, T1, TFinal               float64
	AtmosphereData               AtmosphereData
	AtmosphereLabel              string
	LiquidWater                  bool
	Surface                      Composition
	Subterrain                   Composition
	Hydrosphere                  string
	Biosphere                    string
	WaterPercent                 float64
	Life                         bool
	Settleable                   bool
	Political                    string
	Development                  float64
	Moons                        int
	ArchetypeBand                string
	OrbitalPeriod                float64
	Eccentricity                 float64
	TidalLock                    bool

	// Биомы и зоны недр объектами (99.2.28 §9): финальная поверхность/недра.
	// Surface/Subterrain — производные карты из этих объектов (аддитивность).
	Biomes          []models.Biome
	SubterrainZones []models.SubterrainZone
	// DraftSurface — пробная поверхность слоя 8 (транзит для мягких связей
	// недр); в данные не пишется.
	DraftSurface Composition

	// FormationHistory — история формирования планеты (этап 2 облака, §6):
	// маркер planet.data["formation_history"]. Пусто у гигантов/экзотики и
	// при MassOverride (прежняя зонная функция состава).
	FormationHistory []models.PlanetFormationEvent
}

// ==================== КАСКАД: КАМЕНИСТАЯ/ЛЕДЯНАЯ ПЛАНЕТА ====================

// runCascade — физический каскад каменистой/ледяной планеты (99.2.20 §2–§3).
// Одна итерация, без цикла фиксированной точки (граница §12).
func (g *Generator) runCascade(in cascadeInput) *cascadeResult {
	res := &cascadeResult{}

	// --- Слой 3 — планета: масса, состав, плотность, радиус, гравитация ---
	// Нормированное расстояние (S: r/√L; P: физическое r_P — нормализация √L
	// не применима, §4.1 спеки массы).
	aNorm := aNormOf(in.OrbitRadiusAU, in.Luminosity)
	if in.Circumbinary {
		aNorm = in.OrbitRadiusAU
	}
	mass := in.MassOverride
	if mass <= 0 {
		// Масса — от нормированного расстояния (номер орбиты), светимость
		// не входит (спека 2026-09-21 §4); ζ — ролл на месте вызова,
		// M_диск (cloudBudget) — ОБЩАЯ МАССА облака мира (один ролл на мир,
		// median = S₀), f_обр — обрезка гигантом-соседом (чистая функция,
		// ноль роллов, поток RNG не сдвигает). Профиль нормирован: w_i =
		// c_i/S₀ (спека поясов §4.0), поэтому M_i = clamp(f_обр·M_диск·ζ·w_i).
		// Шум ζ НЕ нормируется (решение создателя 2026-09-22, вариант 2):
		// значения планет сохраняются (нормировка и приор сокращаются).
		zeta := math.Exp(0.6 * g.rng.NormFloat64())
		mass = clamp(retentionFactor(in.OrbitIndex, g.giantOrbit)*
			g.cloudBudget*accretionMass(aNorm, in.Metallicity, zeta)/cloudProfileSum,
			massMin, massMax)
	}
	// Состав и история формирования (этап 2 облака, спека 2026-09-22 §4.3–§4.6):
	// резервуары/конденсация/миграция/потеря/эпоха. compositionByZone
	// сохранена для поясов (planet_data_belt.go). +2 ролла/планету — названный
	// сдвиг потока RNG (ζ массы идёт ДО состава, ярдстик массы не сдвинут).
	// При MassOverride (калибровки/близнецы, §1.3) — прежняя зонная функция,
	// без нового состава, маркера и роллов.
	var rock, iron, ice float64
	var history []models.PlanetFormationEvent
	if in.MassOverride > 0 {
		rock, iron, ice = compositionByZone(in.OrbitRadiusAU, in.Luminosity, g.rng)
	} else {
		rock, iron, ice, history = compositionFromHistory(
			aNorm,
			g.rng.Float64(), // u_t — эпоха формирования
			g.rng.Float64(), // u_mig — миграция
			g.rng.Float64(), // u_loss — потеря мантии
			g.rng.Float64(), // J_ice
			g.rng.Float64(), // u_iron
			g.migrationMode,
			in.OrbitIndex, g.giantOrbit,
			defaultFormationParams(),
		)
	}
	density := planetDensity(rock, iron, ice, mass)
	size := in.SizeOverride
	if size <= 0 {
		size = computeRadius(mass, density)
	}
	gravity := computeGravity(mass, size)
	escapeVel := escapeVelocity(mass, size)
	res.Mass, res.Size, res.Density, res.Gravity = mass, size, density, gravity
	res.EscapeVelocity = escapeVel
	res.FormationHistory = history

	// --- Слой 2 — орбита: период, эксцентриситет, приливный захват ---
	res.OrbitalPeriod = orbitalPeriod(in.OrbitRadiusAU, in.StellarMass)
	res.Eccentricity = 0.03 + g.rng.Float64()*0.27
	res.TidalLock = res.OrbitalPeriod < tidalLockThresholdYears

	// --- Полоса композиции по предварительной T (для T₀-альбедо) ---
	band := bandForTemp(prelimBandTemp(in))
	// Профиль региона (59a §8): surface_bias/subterrain_bias умножают веса
	// полосы (форма вне шаблона полосы — no-op, ограничение O4). Каскад
	// не модифицируется — сдвигаются только веса композиционного шаблона.
	bandSurface := regionprofile.ApplyMultMap(band.baseSurface, g.surfaceBias(), g.profileIntensity)
	bandSubterrain := regionprofile.ApplyMultMap(band.baseSubterrain, g.subterrainBias(), g.profileIntensity)

	// --- Слой 4 — ядро (существующая генерация + F_int) ---
	prelimSub := GenerateSubterrainComposition(bandSubterrain, Composition{}, 0, 0, g.rng)
	core := GenerateCore(mass, band.id, prelimSub, in.AgeGyr, g.rng)
	heatFlux := internalHeatFlux(core, in.AgeGyr, mass, false)
	core.HeatFluxWm2 = heatFlux
	res.Core, res.HeatFluxWm2 = core, heatFlux

	// --- Слой 5 — T⁴-энергобаланс: T₀ → T₁ (режим) → атмосфера → T_final ---
	prelimSurface := in.SurfaceOverride
	if len(prelimSurface) == 0 {
		prelimSurface = GenerateSurfaceComposition(bandSurface, 0, 0, g.rng)
	}
	aSurface := computeAlbedo(prelimSurface)
	fStar := solarFluxAt1AU * in.Luminosity / (in.OrbitRadiusAU * in.OrbitRadiusAU)
	t0 := tempFromFlux(fStar, aSurface, heatFlux, 0, 0)
	t1 := t0 * math.Pow(1+0.75*nominalTau, 0.25)
	res.T0, res.T1 = t0, t1

	// --- Слой 6 — атмосфера-объект ---
	regime := atmosphereRegime(t1)
	zone := volatileZoneFor(in.OrbitRadiusAU, in.Luminosity)
	fVol := in.FVolOverride
	if fVol <= 0 {
		fVol = volatileBudget(zone, g.rng)
	}
	waterShare := in.WaterShareOverride
	if waterShare <= 0 {
		waterShare = waterShareByZone(zone, g.rng)
	}
	fAtm := fVol * sequestrationFactor(regime) * escapeFactor(escapeVel) * stripFactor(in.TEff)
	massAtmKg := fAtm * mass * earthMassKg
	pressure := (massAtmKg / earthAtmMassKg) * gravity / (size * size)
	if pressure > 1000 {
		pressure = 1000
	}
	comp := normalizeComposition(atmosphereComposition(regime, zone, waterShare, false, g.rng))
	// Оверрайд состава (99.2.22 §3.3 ручка 2): применяется только если
	// фактический режим (по T₁ с оверрайдом поверхности) совпадает с
	// ожидаемым — иначе мир остаётся нетюнингованным (честный отказ).
	if len(in.CompositionOverride) > 0 && regime == in.CompositionRegime {
		comp = applyCompositionOverride(in.CompositionOverride, comp)
	}
	tau := tauIR(pressure, comp)
	label := classifyAtmosphere(comp, pressure)
	fc := cloudFraction(label, pressure)
	albedo := aSurface*(1-fc) + 0.7*fc
	tFinal := tempFromFlux(fStar, albedo, heatFlux, 0, tau)
	mu := meanMolecularWeight(comp)
	res.AtmosphereData = AtmosphereData{
		Composition:         compositionToPct(comp),
		PressureAtm:         pressure,
		MassEarthAtm:        pressure * size * size / gravity,
		TauIR:               tau,
		ScaleHeightKm:       scaleHeight(tFinal, mu, gravity),
		MeanMolecularWeight: mu,
	}
	res.AtmosphereLabel = label
	res.TFinal = tFinal

	// --- Слой 7 — флаг жидкой воды (предварительный: для гидросферы и
	// жизни; финальный — после жизненного прохода, инвариант §13.3) ---
	flagPre := liquidWaterPossible(tFinal, pressure)

	// --- Слой 9 — гидросфера, вода ---
	res.Hydrosphere = pickHydrosphere(band.hydrospheres, flagPre, g.rng)
	res.Biosphere = pickBiosphere(band.biospheres, g.rng)
	res.WaterPercent = generateWater(
		&Archetype{Hydrosphere: res.Hydrosphere, WaterChance: band.waterChance},
		tFinal, g.rng,
	)

	// --- Жизнь (99.2.20 §3.9): требует жидкой воды или подлёдного океана ---
	life := in.ForceLife || (flagPre || res.Hydrosphere == "подлёдная") &&
		res.WaterPercent > 10 && tFinal > 200 && tFinal < 400 &&
		res.Biosphere != "стерильная" && g.rng.Float64() < band.lifeChance
	res.Life = life

	// --- Жизненный проход (99.2.20 §3.5 п.5): состав пересчитывается
	// (O₂, CO₂ ↓), τ_IR и T_final обновляются. ---
	if life {
		compLife := normalizeComposition(atmosphereComposition(regime, zone, waterShare, true, g.rng))
		tauLife := tauIR(pressure, compLife)
		labelLife := classifyAtmosphere(compLife, pressure)
		fcLife := cloudFraction(labelLife, pressure)
		albedoLife := aSurface*(1-fcLife) + 0.7*fcLife
		tFinalLife := tempFromFlux(fStar, albedoLife, heatFlux, 0, tauLife)
		muLife := meanMolecularWeight(compLife)
		res.AtmosphereData = AtmosphereData{
			Composition:         compositionToPct(compLife),
			PressureAtm:         pressure,
			MassEarthAtm:        pressure * size * size / gravity,
			TauIR:               tauLife,
			ScaleHeightKm:       scaleHeight(tFinalLife, muLife, gravity),
			MeanMolecularWeight: muLife,
		}
		res.AtmosphereLabel = labelLife
		res.TFinal = tFinalLife
	}

	// --- Флаг финальный: инвариант «флаг ⟺ (P, T_final)» (§13.3) держится
	// по построению — флаг считается от финальной T_final (после жизненного
	// прохода), а не от предварительной. ---
	res.LiquidWater = liquidWaterPossible(res.TFinal, pressure)

	// --- Оверрайды прототипа (99.2.28 §16.1): FinalTempOverride применяется
	// после жизненного прохода; флаг пересчитывается от оверрайда
	// (инвариант «флаг ⟺ (P, T_final)» сохраняется). ---
	if in.FinalTempOverride > 0 {
		res.TFinal = in.FinalTempOverride
		res.LiquidWater = liquidWaterPossible(res.TFinal, pressure)
	}

	// --- Оверрайд воды прототипа (99.2.28 §16.1): применяется до слоя 8. ---
	if in.WaterPercentOverride > 0 {
		res.WaterPercent = in.WaterPercentOverride
	}

	// --- Слой 8 — draft поверхности (99.2.28 §4): пробник для мягких связей
	// недр — физические веса биомов без джиттера/RNG (детерминизм по seed). ---
	env := &biomeEnv{
		TFinal:        res.TFinal,
		PressureAtm:   pressure,
		WaterPercent:  res.WaterPercent,
		Flag:          res.LiquidWater,
		Rock:          rock,
		Iron:          iron,
		Ice:           ice,
		Gravity:       gravity,
		Life:          life,
		Atmosphere:    res.AtmosphereData.Composition,
		V:             volcanicIndex(core.Activity, heatFlux),
		Radioactivity: core.Radioactivity,
		TidalLock:     res.TidalLock,
		Band:          bandForTemp(res.TFinal).id,
	}
	draft := generateBiomeDraft(env, g.surfaceBias())
	res.DraftSurface = draft
	env.DraftOceans = draft.ShareOf(SurfaceOceans) / 100

	// --- Слой 9 — недра объектами (99.2.28 §8): физические веса + bias +
	// whitelist (bands) + мягкие связи от draft + джиттер + матрица. ---
	res.SubterrainZones = generateSubterrain(env, g.subterrainBias(), draft, g.rng)
	res.Subterrain = compositionFromZones(res.SubterrainZones)

	// --- Слой 10 — биомы (99.2.28 §6.2): финальная поверхность объектами.
	// Производные карты (surface_composition/surface_dominant) считаются из
	// объектов одной функцией (аддитивность §9.2). ---
	res.Biomes = generateBiomes(env, g.surfaceBias(), g.rng)
	res.Surface = compositionFromBiomes(res.Biomes)

	// --- Пригодность, спутники, политика ---
	// Пригодность для людей — единый источник races.HumansSuitable (65a,
	// Вариант А): вместо пресета поселений (settlement.Suitable, скрыт).
	res.Settleable = races.HumansSuitable(humansSuitableData(res))
	res.Moons = determineMoons(size, band.id, g.rng)
	res.Political = "нет"
	if res.Settleable {
		systems := []string{
			"демократия", "диктатура", "теократия",
			"корпоратократия", "анархия", "ИИ-управление",
		}
		res.Political = systems[g.rng.Intn(len(systems))]
		res.Development = 0.1 + g.rng.Float64()*0.9
	}

	// --- Полоса (производный ярлык) по T_final (99.2.20 §5) ---
	res.ArchetypeBand = bandForTemp(res.TFinal).id

	return res
}

// humansSuitableData — собирает map planet.data для races.HumansSuitable
// из результата каскада: поля, которые читает карточка людей (температура,
// давление/состав атмосферы, радиация ядра, гравитация, флаг жидкой воды,
// фактическая вода). Единый источник пригодности для людей (65a).
func humansSuitableData(res *cascadeResult) map[string]interface{} {
	comp := make(map[string]interface{}, len(res.AtmosphereData.Composition))
	for gas, pct := range res.AtmosphereData.Composition {
		comp[gas] = pct
	}
	return map[string]interface{}{
		"temperature":           res.TFinal,
		"gravity":               res.Gravity,
		"water_percent":         res.WaterPercent,
		"liquid_water_possible": res.LiquidWater,
		"atmosphere_data": map[string]interface{}{
			"pressure_atm": res.AtmosphereData.PressureAtm,
			"composition":  comp,
		},
		"core": map[string]interface{}{
			"radioactivity":  res.Core.Radioactivity,
			"heat_flux_w_m2": res.Core.HeatFluxWm2,
		},
	}
}

// prelimBandTemp — предварительная температура для выбора полосы: T₁ с
// дефолтным альбедо 0.15 (горы) — «физическая» температура без атмосферы.
// Полоса — композиционный шаблон; температурные модификаторы
// GenerateSurfaceComposition доводят поверхность до консистентности с T_final.
func prelimBandTemp(in cascadeInput) float64 {
	fStar := solarFluxAt1AU * in.Luminosity / (in.OrbitRadiusAU * in.OrbitRadiusAU)
	t0 := tempFromFlux(fStar, 0.15, 0, 0, 0)
	return t0 * math.Pow(1+0.75*nominalTau, 0.25)
}

// ==================== КАСКАД: ГАЗОВЫЙ ГИГАНТ (99.2.20 §3.6 «Гиганты») ====================

// runCascadeGiant — каскад газового гиганта: кривая M→R не меняется
// (99.2.15), температура — T⁴ + F_KH (чинит «гигант 13 MJ → 20 K»),
// атмосфера-объект (H₂/He, P = 1 атм, τ ≈ 0.05–0.1).
func (g *Generator) runCascadeGiant(in cascadeInput) *cascadeResult {
	res := &cascadeResult{}

	mass := in.MassOverride
	if mass <= 0 {
		mass = g.gasGiantMass()
	}
	size := GasGiantRadius(mass)
	density := mass / (size * size * size)
	gravity := computeGravity(mass, size)
	res.Mass, res.Size, res.Density, res.Gravity = mass, size, density, gravity
	res.EscapeVelocity = escapeVelocity(mass, size)

	res.OrbitalPeriod = orbitalPeriod(in.OrbitRadiusAU, in.StellarMass)
	res.Eccentricity = 0.03 + g.rng.Float64()*0.27
	res.TidalLock = res.OrbitalPeriod < tidalLockThresholdYears

	core := GenerateCore(mass, "жаркий", Composition{}, in.AgeGyr, g.rng)
	heatFlux := internalHeatFlux(core, in.AgeGyr, mass, true)
	core.HeatFluxWm2 = heatFlux
	res.Core, res.HeatFluxWm2 = core, heatFlux

	fStar := solarFluxAt1AU * in.Luminosity / (in.OrbitRadiusAU * in.OrbitRadiusAU)
	aSurface := 0.15 // условное альбедо поверхности гиганта
	fc := 0.30       // водородно-гелиевая (99.2.20 §3.5, таблица f_c)
	albedo := aSurface*(1-fc) + 0.7*fc
	tau := 0.05 + g.rng.Float64()*0.05
	tFinal := tempFromFlux(fStar, albedo, heatFlux, 0, tau)
	res.T0 = tempFromFlux(fStar, aSurface, heatFlux, 0, 0)
	res.T1 = res.T0 * math.Pow(1+0.75*nominalTau, 0.25)
	res.TFinal = tFinal

	comp := normalizeComposition(map[string]float64{
		"H2": 0.85 + g.rng.Float64()*0.10, // 0.85–0.95
		"He": 0.05 + g.rng.Float64()*0.10, // 0.05–0.15
	})
	mu := meanMolecularWeight(comp)
	res.AtmosphereData = AtmosphereData{
		Composition:         compositionToPct(comp),
		PressureAtm:         1.0, // условный уровень «поверхности» — 1 бар
		MassEarthAtm:        1.0 * size * size / gravity,
		TauIR:               tau,
		ScaleHeightKm:       scaleHeight(tFinal, mu, gravity),
		MeanMolecularWeight: mu,
	}
	res.AtmosphereLabel = classifyAtmosphere(comp, 1.0)
	res.LiquidWater = false
	res.Hydrosphere = "сухая"
	res.Biosphere = "стерильная"
	res.WaterPercent = 0
	res.Life = false
	res.Settleable = false
	res.Political = "нет"
	res.ArchetypeBand = "жаркий"
	return res
}

// ==================== ХЕЛПЕРЫ ====================

// pickHydrosphere — гидросфера из allowed-списка полосы, отфильтрованного
// флагом (99.2.20 §3.9): флаг true → океаны/озёра/сухая; false →
// ледяной покров/подлёдная/сухая/кислотная.
func pickHydrosphere(allowed []string, flag bool, rng *rand.Rand) string {
	allowedTrue := map[string]bool{"океаны": true, "озёра": true, "сухая": true}
	allowedFalse := map[string]bool{"ледяной покров": true, "подлёдная": true, "сухая": true, "кислотная": true}
	var candidates []string
	for _, h := range allowed {
		if flag && allowedTrue[h] {
			candidates = append(candidates, h)
		}
		if !flag && allowedFalse[h] {
			candidates = append(candidates, h)
		}
	}
	if len(candidates) == 0 {
		return "сухая"
	}
	return candidates[rng.Intn(len(candidates))]
}

// pickBiosphere — биосфера из allowed-списка полосы.
func pickBiosphere(allowed []string, rng *rand.Rand) string {
	if len(allowed) == 0 {
		return "стерильная"
	}
	return allowed[rng.Intn(len(allowed))]
}

// compositionToPct — массовые доли (0–1) → проценты (сумма 100).
func compositionToPct(comp map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(comp))
	for k, v := range comp {
		out[k] = v * 100
	}
	return out
}

// fallbackStellarMass — середина диапазона массы класса (M☉). Источник —
// DefaultStellarMassRanges (galaxy.go, 29a §4м); дубликат: planet не может
// импортировать galaxy (цикл через twin.go → galaxy).
func fallbackStellarMass(spectralClass string) float64 {
	switch spectralClass {
	case "O":
		return 37.5
	case "B":
		return 10.5
	case "A":
		return 2.5
	case "F":
		return 1.35
	case "G":
		return 1.05
	case "K":
		return 0.75
	case "M":
		return 0.35
	case "L":
		return 0.055
	case "T":
		return 0.0225
	case "Y":
		return 0.015
	}
	return 1.0
}