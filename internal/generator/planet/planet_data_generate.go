// internal/generator/planet/planet_data_generate.go
package planet

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"

	"github.com/google/uuid"
	"zorion/internal/names"
)

// determinePlanetCount — сколько планет у звезды данного класса.
// Mean-модель (99.2.4 §5.2): n = floor(mean) + Бернулли(frac), потолок 8.
// Профиль региона (59a §8 P3): mean × planet_count_mult перед потолком 8.
func (g *Generator) determinePlanetCount(spectralClass string) int {
	return meanPlanetCount(g.rng, g.means.meanForClass(spectralClass)*g.planetCountMult(), g.means.Max)
}

// planetCountFor — число планет для мира (99.2.4 §5.2): mean-модель по типу.
// Обычные звёзды — mean по классу; двойные широкие ×0.9 (S-тип), тесные
// mean 0.2 (P-тип); кратные ×0.9; остатки/прочая экзотика — свои mean
// (максимум генератора 1, «чаще 0»); протозвезда — 0 (диск вместо планет).
// Профиль региона (59a §8 P3): mean × planet_count_mult перед потолком 8.
func (g *Generator) planetCountFor(w WorldInfo) int {
	m := g.means
	mult := g.planetCountMult()
	switch w.StarType {
	case "star", "":
		if w.Mods != nil && w.Mods.IsSupergiantExotic() {
			// Прочая экзотика (сверхгиганты O–B–A, фаза I): mean 0.1, максимум 1.
			return capRemnant(meanPlanetCount(g.rng, m.Exotic*mult, m.Max))
		}
		mean := m.meanForClass(w.SpectralClass) * mult
		switch w.SystemType {
		case "binary":
			if w.Mods != nil && w.Mods.BinaryType == "close" {
				// P-тип редок (Kepler-16/47): mean 0.2.
				return meanPlanetCount(g.rng, m.BinaryCloseMean*mult, m.Max)
			}
			// S-тип: планеты у главного компонента, ×0.9.
			return meanPlanetCount(g.rng, mean*m.BinaryWideFactor, m.Max)
		case "multiple":
			// wide-семантика: S-тип у главного компонента, ×0.9.
			return meanPlanetCount(g.rng, mean*m.MultipleFactor, m.Max)
		default:
			return meanPlanetCount(g.rng, mean, m.Max)
		}
	case "black_hole":
		return capRemnant(meanPlanetCount(g.rng, m.BlackHole*mult, m.Max))
	case "neutron":
		return capRemnant(meanPlanetCount(g.rng, m.Neutron*mult, m.Max))
	case "white_dwarf":
		return capRemnant(meanPlanetCount(g.rng, m.WhiteDwarf*mult, m.Max))
	case "protostar":
		return 0 // диск вместо планет (§5.3)
	}
	return 0
}

// planetCountMult — эффективный множитель среднего числа планет: 1.0 без
// профиля и расы; иначе произведение planet_count_mult профиля (59a §8 P3,
// с масштабом интенсивности) и множителя кластеров рас (99.2.22 §3.3 ручка 6,
// слой 1: eff = 1 + (config−1)·s).
func (g *Generator) planetCountMult() float64 {
	mult := 1.0
	if g.profile != nil {
		mult *= g.profile.PlanetCountMult(g.profileIntensity)
	}
	if g.raceID != "" && g.racePlanetCountMult > 0 {
		mult *= 1 + (g.racePlanetCountMult-1)*g.raceSoftness
	}
	return mult
}

// resourceBias — эффективные веса категорий ресурсов (59a §8 P2):
// nil без профиля, иначе resource_bias с масштабом интенсивности.
func (g *Generator) resourceBias() map[string]float64 {
	if g.profile == nil {
		return nil
	}
	return g.profile.ResourceBias(g.profileIntensity)
}

// surfaceBias — эффективные множители весов форм поверхности полосы
// (59a §8): nil без профиля, иначе surface_bias с масштабом интенсивности.
func (g *Generator) surfaceBias() map[string]float64 {
	if g.profile == nil {
		return nil
	}
	return g.profile.SurfaceBias(g.profileIntensity)
}

// subterrainBias — эффективные множители весов форм недр полосы (59a §8):
// nil без профиля, иначе subterrain_bias с масштабом интенсивности.
func (g *Generator) subterrainBias() map[string]float64 {
	if g.profile == nil {
		return nil
	}
	return g.profile.SubterrainBias(g.profileIntensity)
}

// capRemnant — «максимум генератора 1» у остатков (99.2.4 §5.2, решение §4к.1).
func capRemnant(n int) int {
	if n > 1 {
		return 1
	}
	return n
}

// ==================== СРЕДНЕЕ ЧИСЛО ПЛАНЕТ (99.2.4 §5.2, конфиг 99.2.3 §4.3) ====================

// PlanetMeans — среднее число планет по типу звезды. Механика целого счёта:
// n = floor(mean) + Бернулли(frac), потолок 8 (Kepler-90). При mean < 1 —
// n ∈ {0, 1} («чаще 0»), верхний предел остатков — 1 (решение §4к.1).
//
// Дефолты — физический реализм (ревизия астронома §4е, решения §4ж/§4и.3;
// спека 2026-09-20 «Реализм распределения» §5.1 — решение создателя
// 2026-09-20 «число планет поднять, 4–8»): G/F ~6.5, K/M ~5.5 (Солнце — 8,
// TRAPPIST-1 — 7, Kepler-90 — 8 — ориентиры диапазона, не обещание
// генератора: mean 6.5 → n ∈ {6, 7}, mean 5.5 → n ∈ {5, 6}; 8 достижимо
// только множителями — ручка 6 расы ×1.3 → 8.45 → кламп 8, конфиг админки),
// A ~1, O/B ~0.3, L/T/Y ~1 (n ≡ 1, pre-existing); экзотика: ЧД 0.1,
// НЗ 0.05, WD 0.3, протозвезда 0 (диск), прочая экзотика 0.1; двойные
// широкие ×0.9, тесные mean 0.2, кратные ×0.9.
type PlanetMeans struct {
	O float64 `json:"O"`
	B float64 `json:"B"`
	A float64 `json:"A"`
	F float64 `json:"F"`
	G float64 `json:"G"`
	K float64 `json:"K"`
	M float64 `json:"M"`
	L float64 `json:"L"`
	T float64 `json:"T"`
	Y float64 `json:"Y"`

	// Экзотика (максимум генератора 1, «чаще 0»).
	BlackHole  float64 `json:"black_hole"`
	Neutron    float64 `json:"neutron"`
	WhiteDwarf float64 `json:"white_dwarf"`
	Protostar  float64 `json:"protostar"`
	Exotic     float64 `json:"exotic"` // прочая экзотика (сверхгиганты O–B–A, фаза I)

	// Двойные/кратные: S-тип у главного компонента, P-тип редок.
	BinaryWideFactor float64 `json:"binary_wide_factor"` // ×0.9
	BinaryCloseMean  float64 `json:"binary_close_mean"`  // mean 0.2
	MultipleFactor   float64 `json:"multiple_factor"`    // ×0.9

	// Max — потолок числа планет (Kepler-90 = 8, решение §4е).
	Max int `json:"max"`
}

// DefaultPlanetMeans — физические дефолты (99.2.4 §5.2; спека 2026-09-20 §5.1).
func DefaultPlanetMeans() PlanetMeans {
	return PlanetMeans{
		O: 0.3, B: 0.3, A: 1, F: 6.5, G: 6.5, K: 5.5, M: 5.5, L: 1, T: 1, Y: 1,
		BlackHole: 0.1, Neutron: 0.05, WhiteDwarf: 0.3, Protostar: 0, Exotic: 0.1,
		BinaryWideFactor: 0.9, BinaryCloseMean: 0.2, MultipleFactor: 0.9,
		Max: 8,
	}
}

// meanForClass — среднее по спектральному классу обычной звезды.
func (m PlanetMeans) meanForClass(cls string) float64 {
	switch cls {
	case "O":
		return m.O
	case "B":
		return m.B
	case "A":
		return m.A
	case "F":
		return m.F
	case "G":
		return m.G
	case "K":
		return m.K
	case "M":
		return m.M
	case "L":
		return m.L
	case "T":
		return m.T
	case "Y":
		return m.Y
	}
	return 0
}

// Validate — 0 ≤ mean ≤ 8 (99.2.3 §4.3): mean > 8 молча обрежет распределение
// (E[n] ≠ mean), поэтому значение отклоняется валидацией, а не клампится.
func (m PlanetMeans) Validate() error {
	for name, v := range m.all() {
		if v < 0 || v > 8 {
			return fmt.Errorf("%s: mean %.2f вне [0, 8]", name, v)
		}
	}
	return nil
}

// all — все редактируемые значения таблицы средних (для валидации).
func (m PlanetMeans) all() map[string]float64 {
	return map[string]float64{
		"O": m.O, "B": m.B, "A": m.A, "F": m.F, "G": m.G,
		"K": m.K, "M": m.M, "L": m.L, "T": m.T, "Y": m.Y,
		"black_hole":         m.BlackHole,
		"neutron":            m.Neutron,
		"white_dwarf":        m.WhiteDwarf,
		"protostar":          m.Protostar,
		"exotic":             m.Exotic,
		"binary_wide_factor": m.BinaryWideFactor,
		"binary_close_mean":  m.BinaryCloseMean,
		"multiple_factor":    m.MultipleFactor,
	}
}

// determineSystemAge — возраст звёздной системы в млрд лет.
func determineSystemAge(spectralClass string, rng *rand.Rand) float64 {
	switch spectralClass {
	case "O", "B":
		return 0.1 + rng.Float64()*0.9
	case "A":
		return 0.3 + rng.Float64()*1.7
	case "F":
		return 1.0 + rng.Float64()*2.0
	case "G":
		return 2.0 + rng.Float64()*6.0
	case "K":
		return 4.0 + rng.Float64()*7.0
	case "M":
		return 6.0 + rng.Float64()*7.0
	case "L", "T", "Y":
		return 5.0 + rng.Float64()*8.0
	default:
		return 2.0 + rng.Float64()*8.0
	}
}

// ==================== ГАЗОВЫЕ ГИГАНТЫ ====================

// gasGiantChance — базовый шанс газового гиганта по спектральному классу
// (спека 2026-09-20 §5.2, per-системный ролл «есть гигант»). Перевёрнут
// под реальность: O/B 0.5% (практически не наблюдаются), A 2% (β Pic,
// HR 8799), F/G/K 10% (Cumming 2008), M 3% (Endl 2006 / Kepler: 2–5%),
// L/T/Y 0.5% (коричневые карлики: планеты редки, 2M1207).
func gasGiantChance(spectralClass string) float64 {
	switch spectralClass {
	case "O", "B":
		return 0.005
	case "A":
		return 0.02
	case "F", "G", "K":
		return 0.10
	case "M":
		return 0.03
	case "L", "T", "Y":
		return 0.005
	default:
		return 0.01
	}
}

// giantChanceEffective — P_giant_eff (спека 2026-09-20 §4.4): шанс гиганта
// по классу (§5.2) × множитель металличности 10^(0.5×[Fe/H]) (Fischer &
// Valenti 2005: гиганты чаще у металличных звёзд; k = 0.5 — умеренный,
// E[10^(0.5×[Fe/H])] = 1.045 по фактической механике N(0,0.3)+кламп
// [−0.8,+0.5]). Кламп [0.001, 0.5].
func giantChanceEffective(sp StellarParams) float64 {
	p := gasGiantChance(sp.SpectralClass) * math.Pow(10, 0.5*sp.Metallicity)
	if p < 0.001 {
		p = 0.001
	}
	if p > 0.5 {
		p = 0.5
	}
	return p
}

// gasGiantChanceShifted — эффективный шанс гиганта с профилем региона
// (спека 2026-09-20 §4.5, рескейл контракта 99.2.10 §8 P4):
// P_eff = clamp(P_base × (1 + shift/0.3), 0.001, 0.5), где P_base —
// P_giant_eff (класс × металличность, §4.4), shift — gas_giant_shift
// профиля. При старой опорной базе 0.3 (K/M) поведение идентично старому
// аддитивному (0.3×(1±0.667) = 0.1/0.5); при малых базах сдвиг
// масштабируется (G 10% × 1.667 = 16.7%, M 3% → 5%/1%). Кламп [0.001, 0.5]:
// пол «почти ноль» (металло-бедный регион без гигантов), потолок «не
// доминируют» (>50% звёзд с гигантом — абсурд). Без профиля — P_giant_eff.
func (g *Generator) gasGiantChanceShifted(sp StellarParams) float64 {
	c := giantChanceEffective(sp)
	if g.profile != nil {
		c *= 1 + g.profile.GasGiantShift(g.profileIntensity)/0.3
		if c < 0.001 {
			c = 0.001
		}
		if c > 0.5 {
			c = 0.5
		}
	}
	return c
}

// hotGiantChance — шанс миграции гиганта к звезде (P_hot|giant, спека
// 2026-09-20 §5.3): F/G/K/M — 10% (миграция), A — 10% (но n ≡ 1 — фолбэк
// «горячий по необходимости»), O/B и L/T/Y — 0 (фолбэк, миграции нет).
func hotGiantChance(spectralClass string) float64 {
	switch spectralClass {
	case "A", "F", "G", "K", "M":
		return 0.10
	default:
		return 0
	}
}

// rollGiantOrbit — per-системное решение гиганта (спека 2026-09-20 §4.2):
// 0 — гиганта нет; иначе индекс орбиты гиганта. Ролл «есть гигант» —
// P_giant_eff (класс × металличность × профиль, §5.2/§4.4/§4.5); при
// наличии — ролл «мигрировал?» (P_hot|giant, §5.3): да → орбита 1–2,
// нет → равномерно в [3, planetCount]. planetCount < 3 — гигант на орбите
// 1–2 («горячий по необходимости», §5.4; при planetCount = 1 — орбита 1).
// Решение — один раз на систему, до цикла орбит (не потребляет энтропию
// в циклах; при planetCount = 0 — без ролла).
func (g *Generator) rollGiantOrbit(sp StellarParams, planetCount int) int {
	if planetCount <= 0 {
		return 0
	}
	if g.rng.Float64() >= g.gasGiantChanceShifted(sp) {
		return 0
	}
	// Гигант есть.
	if planetCount < 3 {
		// Фолбэк «горячий по необходимости» (§5.4): орбита 1–2.
		if planetCount == 1 {
			return 1
		}
		return 1 + g.rng.Intn(2)
	}
	if g.rng.Float64() < hotGiantChance(sp.SpectralClass) {
		// Миграция: орбита 1–2.
		return 1 + g.rng.Intn(2)
	}
	// Дальняя орбита: равномерно в [3, planetCount].
	return 3 + g.rng.Intn(planetCount-2)
}

// rollCloudBudget — бюджет облака M_диск (спека поясов малых тел §4.0,
// ревизия спеки 2026-09-21 §4; бывший B): M_диск ~ logN(ln S₀, 0.5), один
// ролл на мир (образец rollGiantOrbit): ПРИОР ПЕРЕ-КАЛИБРОВАН — медиана
// S₀ = cloudProfileSum (≈ 18.551, вся лестница орбит 1..8), а не 1. Так
// M_диск становится ОБЩЕЙ МАССОЙ диска (планеты берут нормированные доли
// профиля w_i = c_i/S₀, cascade.go), а не масштабом. Значения планет при
// этом не меняются: median(M_диск) = S₀ и деление на S₀ взаимно
// сокращаются (§4.0.1). Поток RNG не сдвигается — тот же один нормальный
// ролл. Применяется к массе каменистых/ледяных; к газовым гигантам
// (эталон 99.2.15) и экзотике (exotic.go) — нет.
func (g *Generator) rollCloudBudget() float64 {
	return cloudProfileSum * math.Exp(0.5*g.rng.NormFloat64())
}

// ==================== ПАРАМЕТРЫ ЗВЕЗДЫ (99.2.20 §3.1) ====================

// StellarParams — параметры звезды для каскада (слой 1).
type StellarParams struct {
	SpectralClass string
	Luminosity    float64 // L☉
	StellarMass   float64 // M☉
	AgeGyr        float64 // млрд лет
	Metallicity   float64 // [Fe/H]
	TEff          float64 // K
}

// stellarParamsFromWorld — параметры звезды из WorldInfo с фолбэками:
// масса — worlds.stellar_mass (фолбэк серединой диапазона класса),
// возраст — worlds.age (фолбэк determineSystemAge), металличность —
// StellarMods.Metallicity (фолбэк 0 — солнечная), T_eff — worlds.temperature.
func stellarParamsFromWorld(w WorldInfo, rng *rand.Rand) StellarParams {
	sp := StellarParams{
		SpectralClass: w.SpectralClass,
		Luminosity:    luminosityBySpectral(w.SpectralClass),
		TEff:          float64(w.Temperature),
	}
	if w.StellarMass != nil && *w.StellarMass > 0 {
		sp.StellarMass = *w.StellarMass
	} else {
		sp.StellarMass = fallbackStellarMass(w.SpectralClass)
	}
	if w.Age != nil && *w.Age > 0 {
		sp.AgeGyr = *w.Age
	} else {
		sp.AgeGyr = determineSystemAge(w.SpectralClass, rng)
	}
	if w.Mods != nil && w.Mods.Metallicity != nil {
		sp.Metallicity = *w.Mods.Metallicity
	}
	return sp
}

// stellarParamsFromClass — параметры звезды по классу (старые пути без
// WorldInfo: GeneratePlanetsForWorld, прототип): все фолбэки.
func stellarParamsFromClass(spectralClass string, temperature int, rng *rand.Rand) StellarParams {
	return StellarParams{
		SpectralClass: spectralClass,
		Luminosity:    luminosityBySpectral(spectralClass),
		StellarMass:   fallbackStellarMass(spectralClass),
		AgeGyr:        determineSystemAge(spectralClass, rng),
		Metallicity:   0,
		TEff:          float64(temperature),
	}
}

// ==================== ОБЫЧНАЯ ПЛАНЕТА ====================

// generatePlanet — планета обычной звезды (99.2.20): газовый гигант на
// орбите giantOrbit (per-системное решение, спека 2026-09-20 §4.2) или
// физический каскад. Подветки океанических/радиоактивных растворены в
// каскаде (типы возникают из физики: гидросфера «океаны» + вода > 60;
// core.radioactivity > 50).
func (g *Generator) generatePlanet(worldID, worldName string, orbitIndex int, sp StellarParams) *PlanetData {
	// --- ПОДКРУТКА ПОД РАСУ-ДОМА (99.2.22 §3.3–§4) ---
	// Слой 2: ролл «планета подстроена» в фиксированной позиции (до каскада,
	// early-exit при s ≤ 0 / s ≥ 1 — ролл не потребляет энтропию, «s = 0 →
	// как есть» строго для того же seed). Возраст (ручка 4) применяется
	// сразу к параметрам звезды; остальные ручки — в каскад.
	tune := g.raceTunePlanet(sp, orbitRadiusScaled(orbitIndex, sp.Luminosity), true)
	if tune != nil && tune.ageGyr > 0 {
		sp.AgeGyr = tune.ageGyr
	}

	// --- ГАЗОВЫЙ ГИГАНТ (спека 2026-09-20 §4.2) ---
	// Per-системное решение: гигант только на орбите giantOrbit (0 = нет).
	// Решение вынесено из цикла орбит (generateWorldWithCountIntoBuffer /
	// GeneratePlanetsForWorld) — здесь только проверка, без ролла.
	if orbitIndex == g.giantOrbit {
		return g.generateGasGiant(worldID, worldName, orbitIndex, sp, tune)
	}

	// --- СТАНДАРТНАЯ ГЕНЕРАЦИЯ ЧЕРЕЗ ФИЗИЧЕСКИЙ КАСКАД ---
	return g.generateStandardPlanet(worldID, worldName, orbitIndex, sp, false, tune, nil)
}

// generateStandardPlanet — планета по физическому каскаду (стандартный путь).
// forceLife — форсировать жизнь в каскаде (прототип поселения §6.9:
// жизненный проход даёт азотно-кислородную атмосферу, консистентную
// с контрактом инструмента). tune — подкрутка расы-дома (99.2.22): сдвиг
// орбиты, бюджет летучих, состав, поверхность-альбедо; nil — без подкрутки.
// proto — оверрайды прототипа поселения (99.2.28 §16.1): проводятся через
// каскад (FinalTempOverride/WaterPercentOverride), чтобы биомы были
// согласованы с форсированными данными; nil — без оверрайдов.
func (g *Generator) generateStandardPlanet(
	worldID, worldName string,
	orbitIndex int,
	sp StellarParams,
	forceLife bool,
	tune *raceTune,
	proto *prototypeOverrides,
) *PlanetData {
	orbitRadius := orbitRadiusScaled(orbitIndex, sp.Luminosity)
	if tune != nil && tune.orbitMult > 0 {
		orbitRadius *= tune.orbitMult
	}
	in := cascadeInput{
		Luminosity:    sp.Luminosity,
		StellarMass:   sp.StellarMass,
		AgeGyr:        sp.AgeGyr,
		Metallicity:   sp.Metallicity,
		TEff:          sp.TEff,
		OrbitRadiusAU: orbitRadius,
		OrbitIndex:    orbitIndex,
		ForceLife:     forceLife,
	}
	if proto != nil {
		in.FinalTempOverride = proto.finalTempK
		in.WaterPercentOverride = proto.waterPercent
	}
	if tune != nil {
		// Ручки 2–4, 7/7-холод: сдвиг входов каскада (99.2.22 §3.3).
		in.FVolOverride = tune.fVol
		in.SurfaceOverride = tune.surface
		in.CompositionOverride = tune.composition
		in.CompositionRegime = tune.compositionRegime
	}
	res := g.runCascade(in)

	name := names.GeneratePlanetName(g.rng, g.usedNames)
	if name == "" {
		name = "Планета-" + uuidShort()
	}

	dominant := res.Surface.DominantForm()
	if dominant == "" {
		dominant = SurfaceRocks
	}

	radioactive := res.Core.IsRadioactive()

	// Геймдизайнерский тип по композиции (существующий механизм; типы
	// «океаническая»/«радиоактивная» возникают из физики каскада).
	gdType := ClassifyGameDesignType(PlanetClassificationInput{
		IsGasGiant:    false,
		IsRadioactive: radioactive,
		Surface:       res.Surface,
		Temperature:   res.TFinal,
		WaterPercent:  res.WaterPercent,
		Settleable:    res.Settleable,
		Life:          res.Life,
	})

	// UUID генерируется ЗАРАНЕЕ — нужен для детерминированного выбора описания.
	planetID := uuid.New().String()

	descCtx := DescriptionContext{
		PlanetID:     planetID,
		Type:         gdType,
		OrbitIndex:   orbitIndex,
		Atmosphere:   res.AtmosphereLabel,
		Hydrosphere:  res.Hydrosphere,
		Temperature:  res.TFinal,
		WaterPercent: res.WaterPercent,
		Mass:         res.Mass,
		Density:      res.Density,
		Moons:        res.Moons,
		Life:         res.Life,
		Settleable:   res.Settleable,
		Surface:      res.Surface,
		Core:         res.Core,
		IsGasGiant:   false,
	}

	data := map[string]interface{}{
		"size":              res.Size,
		"mass":              res.Mass,
		"density":           res.Density,
		"gravity":           res.Gravity,
		"atmosphere":        res.AtmosphereLabel,
		"atmosphere_data":   atmosphereDataToJSON(res.AtmosphereData),
		"hydrosphere":       res.Hydrosphere,
		"biosphere":         res.Biosphere,
		"temperature":       res.TFinal,
		"water_percent":     res.WaterPercent,
		"life":              res.Life,
		"liquid_water_possible": res.LiquidWater,
		"political_system":  res.Political,
		"moons":             res.Moons,
		"development_level": res.Development,
		"archetype":         res.ArchetypeBand,
		"system_age":        sp.AgeGyr,

		// Орбитальный контекст S-планеты (35b §2.2): вокруг главной.
		"orbit_center":    "main",
		"orbit_radius_au": orbitRadius,

		// Новые поля каскада (99.2.20 §4.1).
		"orbital_period":  res.OrbitalPeriod,
		"eccentricity":    res.Eccentricity,
		"escape_velocity": res.EscapeVelocity,
		"tidal_lock":      res.TidalLock,

		"surface_composition":    composeToJSON(res.Surface),
		"subterrain_composition": composeToJSON(res.Subterrain),
		"surface_dominant":       dominant,
		"type":                   gdType,
		"radioactive":            radioactive,
		"core":                   coreToJSON(res.Core),

		// Биомы и зоны недр объектами (99.2.28 §9): финальная поверхность/недра.
		// surface_composition/subterrain_composition — производные от них.
		"biomes":     biomesToJSON(res.Biomes),
		"subterrain": zonesToJSON(res.SubterrainZones),

		"description": GenerateDescription(descCtx),
	}

	// Ресурсы генерируются здесь же: summary попадает в JSON планеты
	// (data["resources"]), сами ресурсы живут только в памяти.
	resources := attachResources(
		data, planetID, dominant,
		map[string]float64(res.Subterrain),
		sp.SpectralClass, g.rng, g.resourceBias(),
	)

	dataJSON, _ := json.Marshal(data)

	return &PlanetData{
		ID:         planetID,
		WorldID:    worldID,
		Name:       name,
		OrbitIndex: orbitIndex,
		Data:       dataJSON,
		Resources:  resources,
		// Mass — масса из бюджета облака (для мягкого клампа суммы §4.0.2);
		// у гигантов/экзотики остаётся 0 (они бюджет не потребляют).
		Mass: res.Mass,
	}
}

// prototypeOverrides — оверрайды прототипа поселения (99.2.28 §16.1):
// проводятся через каскад как оверрайды входов, чтобы биомы были
// согласованы с форсированными данными (находка @critic №1).
type prototypeOverrides struct {
	finalTempK   float64 // FinalTempOverride: применяется после жизненного прохода
	waterPercent float64 // WaterPercentOverride: применяется до слоя 8
}

// GeneratePrototypePlanet — землеподобная планета для прототипа поселения.
// Контракт инструмента (99.2.20 §6.9): полоса «умеренный», T = 288 K,
// флаг true, вода 80%, жизнь true. Оверрайды проводятся ЧЕРЕЗ КАСКАД
// (99.2.28 §16.1): FinalTempOverride=288 / WaterPercentOverride=80 /
// ForceLife=true — биомы генерируются от форсированных значений и
// согласованы с данными (леса/луга/океаны при T=288, вода=80, life).
// В данные пишется отметка "prototype_forced": true (для аудита/осознанности).
// Население задаётся отдельной вставкой поселения в admin_universe.go.
func (g *Generator) GeneratePrototypePlanet(worldID, worldName, spectralClass string) *PlanetData {
	sp := stellarParamsFromClass(spectralClass, 0, g.rng)
	pd := g.generateStandardPlanet(worldID, worldName, 1, sp, true, nil,
		&prototypeOverrides{finalTempK: 288, waterPercent: 80})
	// Залежи поверхности (спека 2026-09-22-поселение-... §3.2/§3.4): прототип
	// не идёт через generateWorldWithCountIntoBuffer — точка вызова здесь.
	g.generateDeposits(pd)

	var data map[string]interface{}
	if err := json.Unmarshal(pd.Data, &data); err != nil {
		return pd
	}
	data["prototype_forced"] = true
	data["political_system"] = "демократия"
	data["development_level"] = 0.5
	dataJSON, _ := json.Marshal(data)
	pd.Data = dataJSON
	return pd
}

// ==================== УТИЛИТЫ ====================

// coreToJSON — сериализует ядро для JSON-поля (включая внутренний поток
// heat_flux_w_m2, 99.2.20 §4.2).
func coreToJSON(c Core) map[string]interface{} {
	return map[string]interface{}{
		"type":           c.Type,
		"mass_percent":   c.MassPercent,
		"activity":       c.Activity,
		"radioactivity":  c.Radioactivity,
		"age":            c.Age,
		"is_active":      c.IsActive(),
		"is_metallic":    c.IsMetallic(),
		"heat_flux_w_m2": c.HeatFluxWm2,
	}
}

// atmosphereDataToJSON — сериализует атмосферу-объект (99.2.20 §4.1).
func atmosphereDataToJSON(a AtmosphereData) map[string]interface{} {
	return map[string]interface{}{
		"composition":            a.Composition,
		"pressure_atm":           a.PressureAtm,
		"mass_earth_atm":         a.MassEarthAtm,
		"tau_ir":                 a.TauIR,
		"scale_height_km":        a.ScaleHeightKm,
		"mean_molecular_weight":  a.MeanMolecularWeight,
	}
}