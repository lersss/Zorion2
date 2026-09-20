// internal/generator/planet/biome_physics.go
//
// Физика долей биомов и недр (99.2.28 §6–§8): вес = weight_base ×
// категория-формула × гейты условий справочника. Детерминированно (без RNG)
// до шага джиттера; draft слоя 8 — пробник без джиттера/матрицы (не
// потребляет энтропию RNG — детерминизм по seed сохраняется).
package planet

import (
	"math"
	"math/rand"

	"zorion/internal/models"
)

// ==================== ВХОДЫ (99.2.28 §6.1) ====================

// biomeEnv — входы физики биомов/недр: все доступны к слою 10 (ядро — слой 4,
// T_final/атмосфера/флаг — слои 5–7, недра — слой 9).
type biomeEnv struct {
	TFinal       float64 // K
	PressureAtm  float64 // атм
	WaterPercent float64 // 0–100
	Flag         bool    // флаг жидкой воды
	Rock, Iron   float64 // объёмный состав (доли 0–1)
	Ice          float64
	Gravity      float64 // g
	Life         bool
	Atmosphere   map[string]float64 // состав атмосферы, газ → % (сумма 100)
	V            float64            // вулканический индекс (§7)
	Radioactivity float64           // 0–100
	TidalLock    bool
	Band         string // полоса по T_final: экстремальный/жаркий/умеренный/холодный
	DraftOceans  float64 // доля океанов в draft поверхности (0–1), для мягких связей недр
}

// volcanicIndex — вулканический индекс V (99.2.28 §7): техническая величина
// 0–1, общий корень вулканизма поверхности и недр. Не хранится в данных.
// V = clamp(0.4·(Activity/100) + 0.6·clamp(F_int/50, 0, 1), 0, 1).
func volcanicIndex(activity, heatFluxWm2 float64) float64 {
	return clamp(0.4*(activity/100)+0.6*clamp(heatFluxWm2/50, 0, 1), 0, 1)
}

// ==================== ПРИЗНАКИ АТМОСФЕРЫ (приложение §0) ====================

// atmosphereFeatures — признаки атмосферы по составу (единый механизм,
// не ярлыки классификатора): no_toxic/toxic/acidic/methane/nitrogen/co2/
// oxygen_free/dense. Пороги токсичности — из справочника (params).
func atmosphereFeatures(comp map[string]float64, pressureAtm float64, thresholds map[string]float64) map[string]bool {
	toxic := false
	for gas, th := range thresholds {
		if comp[gas] > th {
			toxic = true
			break
		}
	}
	acidic := comp["SO2"]+comp["H2S"]+comp["HCl"]+comp["HF"] > 0.1
	return map[string]bool{
		"no_toxic":    !toxic,
		"toxic":       toxic,
		"acidic":      acidic,
		"methane":     comp["CH4"] > 1,
		"nitrogen":    comp["N2"] > 50,
		"co2":         comp["CO2"] > 50,
		"oxygen_free": comp["O2"] < 1,
		"dense":       pressureAtm > 1,
	}
}

// ==================== ВЕС БИОМА (99.2.28 §6.2) ====================

// biomeWeight — вес биома: weight_base × категория-формула × гейты условий.
// Детерминированно, без RNG. Whitelist-гейт полосы (bands) — отдельным
// шагом конвейера (шаг 3 §6.2), здесь не применяется.
func biomeWeight(b *BiomeDef, env *biomeEnv, features map[string]bool) float64 {
	w := b.WeightBase * biomeCategoryFormula(b, env, features)
	if w <= 0 {
		return 0
	}
	return w * biomeGates(b, env, features)
}

// biomeCategoryFormula — категория-формула (99.2.28 §6.2, ~6 формул).
func biomeCategoryFormula(b *BiomeDef, env *biomeEnv, features map[string]bool) float64 {
	switch b.Category {
	case "литосфера":
		// Горы — страховка: рельеф от гравитации, всегда > 0.
		if b.ID == SurfaceRocks {
			return 0.55 + 0.45*clamp(env.Gravity/1.5, 0, 1)
		}
		return 1.0

	case "вода":
		// Водные: flag ? (w/100)^k : 0, k = 2 для океанов, 1 для озёр/мелководий.
		if b.LiquidMedium == "вода" {
			if !env.Flag {
				return 0
			}
			k := 1.0
			if b.ID == SurfaceOceans {
				k = 2.0
			}
			return math.Pow(env.WaterPercent/100, k)
		}
		// Метан/аммиак/CO₂/нет — гейты условий (атмосфера/состав/T) решают.
		return 1.0

	case "биосфера":
		if hasFeature(b.AtmosphereOK, "no_toxic") {
			// Земные биосферные: life + жидкая вода + нетоксичная атмосфера
			// (решение п.19) × температурный комфорт.
			if !env.Life || !env.Flag || features["toxic"] {
				return 0
			}
			return smooth(env.TFinal, 250, 330)
		}
		// Токсикоманы/хемосады/радиационные ковры: жизнь без нетоксичной.
		if !env.Life {
			return 0
		}
		return 1.0

	case "вулканизм":
		switch b.Volcanism {
		case "hot":
			return env.V * smooth(env.TFinal, 500, 700) // лава — только горячо
		case "magma":
			return env.V * smooth(env.TFinal, 1200, 1500) // магмовый океан
		case "any":
			return env.V // вулканы при любой T (Ио)
		}
		return 1.0 // венерианские плоскогорья — без V (гейты атмосферы)

	case "крио":
		// (1 − smooth(T, lo, hi))·(вода или ice) — холоднее = больше.
		factor := env.Ice
		if b.WaterMin > 0 {
			factor = env.WaterPercent / 100
		}
		return (1 - smooth(env.TFinal, b.TMin, b.TMax)) * factor

	case "экзотика":
		return 1.0 // специфические условия — гейты справочника
	}
	return 1.0
}

// biomeGates — гейты условий справочника (гладкие smoothstep на границах
// диапазонов; атмосфера/радиация/прилив/life — жёсткие).
func biomeGates(b *BiomeDef, env *biomeEnv, features map[string]bool) float64 {
	g := 1.0
	g *= smoothGate(b.TMin, b.TMax, 20, env.TFinal)          // T, K
	g *= smoothGate(b.PMin, b.PMax, 0.1, env.PressureAtm)    // P, атм
	g *= smoothGate(b.WaterMin, b.WaterMax, 5, env.WaterPercent) // вода, %
	g *= smoothGate(b.IronMin, 0, 0.05, env.Iron)            // состав (min)
	g *= smoothGate(b.IceMin, 0, 0.05, env.Ice)
	g *= smoothGate(b.RockMin, 0, 0.05, env.Rock)
	g *= smoothGate(b.GMin, b.GMax, 0.1, env.Gravity)        // g
	if g <= 0 {
		return 0
	}
	// Атмосфера: все признаки должны выполняться (AND); пусто = «любая».
	for _, f := range b.AtmosphereOK {
		if !features[f] {
			return 0
		}
	}
	// Радиация: источник флага — среда поверхности, P_surf < 0.5 атм
	// (тонкая атмосфера/магнитосфера → радиолиз), НЕ радиоактивность ядра.
	if b.RequiresRadiation && env.PressureAtm >= 0.5 {
		return 0
	}
	if b.RequiresTidalLock && !env.TidalLock {
		return 0
	}
	if b.RequiresLife && !env.Life {
		return 0
	}
	return g
}

// ==================== КОНВЕЙЕР СЛОЯ 10 (99.2.28 §6.2) ====================

// biomeWeights — шаги 1–3 слоя 10: физические веса + surface_bias +
// whitelist-гейт полосы (bands). Детерминированно, без RNG.
func biomeWeights(env *biomeEnv, bias map[string]float64) map[string]float64 {
	cat := GetBiomeCatalog()
	features := atmosphereFeatures(env.Atmosphere, env.PressureAtm, cat.Params.ToxicThresholds)
	weights := make(map[string]float64, len(cat.Biomes))
	for i := range cat.Biomes {
		b := &cat.Biomes[i]
		w := biomeWeight(b, env, features)
		if w <= 0 {
			continue
		}
		// surface_bias (99.2.28 §12): множитель физических весов; форма вне
		// bands полосы — no-op (O4) — whitelist-гейт ниже обнулит её.
		if m, ok := bias[b.ID]; ok {
			w *= m
		}
		if !bandAllowed(b.Bands, env.Band) {
			continue
		}
		weights[b.ID] = w
	}
	return weights
}

// generateBiomeDraft — слой 8 (99.2.28 §6.3): пробная поверхность для мягких
// связей недр — физические веса без джиттера и матрицы, нормализованные.
// Не потребляет энтропию RNG (детерминизм по seed).
func generateBiomeDraft(env *biomeEnv, bias map[string]float64) Composition {
	weights := biomeWeights(env, bias)
	if len(weights) == 0 {
		return Composition{}
	}
	return Composition(weights).Normalize().NonZero()
}

// generateBiomes — слой 10 (99.2.28 §6.2): полный конвейер биомов —
// веса + bias + whitelist + джиттер ±20% + матрица + нормализация +
// страховка «минимум 1 биом» + примитивные ~3.5%.
func generateBiomes(env *biomeEnv, bias map[string]float64, rng *rand.Rand) []models.Biome {
	base := biomeWeights(env, bias) // шаги 1–3, без RNG
	if len(base) == 0 {
		return nil
	}
	c := copyWeights(base)
	applyRandomJitter(c, rng)                     // шаг 4
	c = resolveConflicts("surface", c, base, rng) // шаг 5 (матрица + авто-жидкость)
	result := Composition(c).Normalize().NonZero() // шаг 6
	if len(result) == 0 {
		result = fallbackComposition(base) // шаг 7: минимум 1 биом
	}
	if rng.Float64() < primitivePlanetProbability { // шаг 8
		result = simplifyComposition(result, rng)
	}
	return compositionToBiomes(result)
}

// ==================== КОНВЕЙЕР СЛОЯ 9 (99.2.28 §8) ====================

// subterrainWeights — физические веса типов недр: weight_base × формула
// приложения §2 × гейты условий + subterrain_bias + whitelist (bands).
func subterrainWeights(env *biomeEnv, bias map[string]float64) map[string]float64 {
	cat := GetBiomeCatalog()
	weights := make(map[string]float64, len(cat.SubterrainTypes))
	for i := range cat.SubterrainTypes {
		s := &cat.SubterrainTypes[i]
		w := subterrainWeight(s, env)
		if w <= 0 {
			continue
		}
		if m, ok := bias[s.ID]; ok {
			w *= m
		}
		if !bandAllowed(s.Bands, env.Band) {
			continue
		}
		weights[s.ID] = w
	}
	return weights
}

// generateSubterrain — слой 9: веса + bias + whitelist + мягкие связи от
// draft (кроме костыля лава→магмакамеры) + джиттер + матрица + нормализация
// + примитивные ~3.5%.
func generateSubterrain(env *biomeEnv, bias map[string]float64, draft Composition, rng *rand.Rand) []models.SubterrainZone {
	base := subterrainWeights(env, bias)
	if len(base) == 0 {
		return nil
	}
	c := copyWeights(base)
	// Мягкие связи от draft (99.2.28 §7): костыль «лава → магмакамеры» удалён.
	applySurfaceToSubterrainLinks(c, draft)
	applyRandomJitter(c, rng)
	c = resolveConflicts("subterrain", c, base, rng)
	result := Composition(c).Normalize().NonZero()
	if len(result) == 0 {
		result = fallbackComposition(base)
	}
	if rng.Float64() < primitivePlanetProbability {
		result = simplifyComposition(result, rng)
	}
	return compositionToZones(result)
}

// subterrainWeight — вес типа недр: weight_base × категория-формула × гейты
// условий справочника (99.2.28 §8). Гейты читают Conditions (данные — админка
// правит их); формула — код (как категория-формулы биомов §6.2).
func subterrainWeight(s *SubterrainTypeDef, env *biomeEnv) float64 {
	w := s.WeightBase * subterrainCategoryFormula(s, env)
	if w <= 0 {
		return 0
	}
	return w * subterrainGates(s, env)
}

// subterrainCategoryFormula — категория-формула типа недр (приложение §2):
// базовая распространённость по физике планеты. Гейты условий — отдельно
// (subterrainGates), из данных справочника.
func subterrainCategoryFormula(s *SubterrainTypeDef, env *biomeEnv) float64 {
	flag01 := 0.0
	if env.Flag {
		flag01 = 1.0
	}
	bio := 0.0
	if env.Life {
		bio = 1.0
	}
	switch s.ID {
	case SubterrainEmptyRock:
		return 1.0
	case SubterrainMagmaticRocks:
		return 0.15 + 0.35*env.V + 0.1*env.Rock
	case SubterrainMetamorphicRocks:
		return 0.12*(1-env.V) + 0.08*env.Rock
	case SubterrainSedimentaryRocks:
		return 0.25 * (0.5*flag01 + 0.5*clamp(env.PressureAtm/5, 0, 1))
	case SubterrainOreVeins:
		return 0.12 * env.Iron * (1 + 0.5*env.V)
	case SubterrainRareEarthVeins:
		return 0.05 * (1 - env.V)
	case SubterrainRadioactiveZones:
		return 0.15 * (env.Radioactivity / 100)
	case SubterrainCoalSeams:
		return 0.15 * bio
	case SubterrainOilPockets:
		return 0.10 * bio * env.DraftOceans
	case SubterrainGasPockets:
		return 0.08 * env.Ice
	case SubterrainGroundwater:
		return 0.20 * (env.WaterPercent / 100)
	case SubterrainGroundIce:
		return 0.20 * (1 - smooth(env.TFinal, 200, 273)) * (env.WaterPercent / 100)
	case SubterrainMagmaChambers:
		return 0.30 * env.V
	case SubterrainCrystalVeins:
		return 0.08 * (1 - env.V)
	case SubterrainSaltDomes:
		return 0.06 * env.DraftOceans
	case SubterrainCaveSystems:
		return 0.05 * (env.WaterPercent / 100) * (1 - env.V)
	case SubterrainMetalCores:
		return 0.10 * env.Iron
	}
	return 1.0
}

// subterrainGates — гейты условий типа недр из данных справочника
// (Conditions): жёсткие (вне диапазона → 0). Админка правит Conditions —
// эти гейты и есть то, что реально влияет на генерацию (замечание ревью:
// раньше гейты были захардкожены switch по ID, Conditions не читались —
// двойной источник истины).
func subterrainGates(s *SubterrainTypeDef, env *biomeEnv) float64 {
	c := &s.Conditions
	if c.TMax > 0 && env.TFinal > c.TMax {
		return 0
	}
	if c.WaterMin > 0 && env.WaterPercent < c.WaterMin {
		return 0
	}
	if c.VMin > 0 && env.V < c.VMin {
		return 0
	}
	if c.VMax > 0 && env.V > c.VMax {
		return 0
	}
	if c.IronMin > 0 && env.Iron < c.IronMin {
		return 0
	}
	if c.IceMin > 0 && env.Ice < c.IceMin {
		return 0
	}
	if c.RockMin > 0 && env.Rock < c.RockMin {
		return 0
	}
	if c.FlagRequired && !env.Flag {
		return 0
	}
	if c.LifeRequired && !env.Life {
		return 0
	}
	if c.PMin > 0 && env.PressureAtm < c.PMin {
		return 0
	}
	if c.RadioactivityMin > 0 && env.Radioactivity < c.RadioactivityMin {
		return 0
	}
	if c.DraftOceans && env.DraftOceans <= 0 {
		return 0
	}
	return 1.0
}

// ==================== ПРОИЗВОДНЫЕ КАРТЫ (99.2.28 §9.2) ====================

// compositionFromBiomes — производная карта surface_composition из объектов
// биомов (форма → share). «Один факт — одно место»: генератор пишет объекты,
// карты считаются этой функцией.
func compositionFromBiomes(biomes []models.Biome) Composition {
	c := make(Composition, len(biomes))
	for _, b := range biomes {
		c[b.Form] = b.Share
	}
	return c
}

// compositionFromZones — производная карта subterrain_composition из зон недр.
func compositionFromZones(zones []models.SubterrainZone) Composition {
	c := make(Composition, len(zones))
	for _, z := range zones {
		c[z.Type] = z.Share
	}
	return c
}

// compositionToBiomes — карта форма→доля → объекты биомов.
func compositionToBiomes(c Composition) []models.Biome {
	result := make([]models.Biome, 0, len(c))
	for form, share := range c {
		result = append(result, models.Biome{Form: form, Share: share})
	}
	return result
}

// compositionToZones — карта тип→доля → объекты зон недр.
func compositionToZones(c Composition) []models.SubterrainZone {
	result := make([]models.SubterrainZone, 0, len(c))
	for t, share := range c {
		result = append(result, models.SubterrainZone{Type: t, Share: share})
	}
	return result
}

// ==================== ХЕЛПЕРЫ ====================

// bandAllowed — биом/тип недр возможен в полосе, если его bands содержат
// полосу (whitelist из справочника, 99.2.28 §6.2 шаг 3).
func bandAllowed(bands []string, band string) bool {
	letter := map[string]string{
		"экстремальный": "э", "жаркий": "ж", "умеренный": "у", "холодный": "х",
	}[band]
	for _, b := range bands {
		if b == letter {
			return true
		}
	}
	return false
}

// hasFeature — есть ли признак в списке атмосферных признаков биома.
func hasFeature(features []string, f string) bool {
	for _, x := range features {
		if x == f {
			return true
		}
	}
	return false
}

// smooth — smoothstep 0→1 на [lo, hi]; вне диапазона — 0/1; hi ≤ lo → 1.
func smooth(x, lo, hi float64) float64 {
	if hi <= lo {
		return 1
	}
	if x <= lo {
		return 0
	}
	if x >= hi {
		return 1
	}
	t := (x - lo) / (hi - lo)
	return t * t * (3 - 2*t)
}

// smoothGate — гладкий гейт диапазона [min, max] (0 = граница не задана):
// 1 внутри (границы ВКЛЮЧИТЕЛЬНЫ — PITFALLS «окна включительные»), 0 вне,
// плавный переход шириной width ЗА пределами диапазона. Для узких
// диапазонов ширина ужимается.
func smoothGate(min, max, width, v float64) float64 {
	if max <= min && min <= 0 {
		return 1 // гейта нет
	}
	g := 1.0
	if min > 0 {
		w := width
		if max > min && (max-min)/4 < w {
			w = (max - min) / 4
		}
		// Нижняя граница: 0 при v ≤ min−w, 1 при v ≥ min.
		g *= smooth(v, min-w, min)
	}
	if max > 0 {
		w := width
		if max > min && (max-min)/4 < w {
			w = (max - min) / 4
		}
		// Верхняя граница: 1 при v ≤ max, 0 при v ≥ max+w.
		g *= 1 - smooth(v, max, max+w)
	}
	return g
}