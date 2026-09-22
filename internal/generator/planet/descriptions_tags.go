// internal/generator/planet/descriptions_tags.go
package planet

import "zorion/internal/models"

// ==================== ВЫЧИСЛЕНИЕ ТЕГОВ ====================
//
// Теги используются для фильтрации записей из библиотеки описаний.
// Каждый тег — булев признак планеты. Полный список и условия — в
// docs/DESCRIPTIONS_WORK.md, раздел 3.
//
// Правило: если тег не может быть вычислен (например, ядро отсутствует
// по типу), считаем, что условие не выполнено.

// Пороги для тегов. Вынесены сюда, чтобы было видно все константы
// в одном месте и легко калибровать.
const (
	// Температура (K)
	tempColdMax      = 220.0
	tempTemperateMin = 220.0
	tempTemperateMax = 380.0
	tempHotMin       = 380.0

	// Возраст (млрд лет) — берём из core.Age
	ageYoungMax = 2.0
	ageOldMin   = 6.0

	// Плотность (в единицах Земли)
	densityLowMax  = 0.5
	densityHighMin = 1.5

	// Масса (в единицах Земли)
	massThreshold       = 5.0
	massThresholdGas    = 100.0

	// Орбита (1-based)
	orbitInnerMax = 2
	orbitOuterMin = 5

	// Спутники
	moonsManyThreshold = 5

	// Подповерхностный океан (эвристика)
	subsurfaceTempMax    = 250.0
	subsurfaceWaterMin   = 30.0

	// Криовулканизм
	cryovolcanicGlacierMin = 30.0 // % ледников в композиции
	cryovolcanicActivityMin = 50.0 // активность ядра

	// Ядро
	coreActiveMin      = 40.0 // Activity — порог IsActive()
	coreRadioactiveMin = 50.0 // Radioactivity — порог IsRadioactive()
)

// computeTags — возвращает множество тегов, применимых к планете.
// Пустая карта — планета не подходит ни под один тег.
func computeTags(ctx DescriptionContext) map[string]bool {
	tags := make(map[string]bool, 24)

	addMoonTags(tags, ctx.Moons)
	addBiosphereTags(tags, ctx.Life, inhabited(ctx))
	addOrbitTags(tags, ctx.OrbitIndex)
	addTemperatureTags(tags, ctx.Temperature)
	addAgeTags(tags, ctx.Core.Age)
	addDensityTags(tags, ctx.Density)
	addMassTags(tags, ctx.Mass, ctx.IsGasGiant)
	addCoreTags(tags, ctx.Core)
	addCryovolcanicTag(tags, ctx.Surface, ctx.Core)
	addSubsurfaceOceanTag(tags, ctx)
	addAtmosphereTags(tags, ctx.Atmosphere)
	addFormationTags(tags, ctx.FormationHistory)

	return tags
}

// ==================== ИСТОРИЯ ФОРМИРОВАНИЯ (этап 2 облака, §6.3) ====================

// addFormationTags — теги из истории формирования: каждый тип записи
// formation_history даёт одноимённый тег (записи библиотеки описаний
// подключают текст через requires). См. config/descriptions/.
func addFormationTags(tags map[string]bool, history []models.PlanetFormationEvent) {
	for _, ev := range history {
		switch ev.Type {
		case "formed_early", "formed_late", "migrated", "ice_lost", "stripped_embryo":
			tags[ev.Type] = true
		}
	}
}

// ==================== СПУТНИКИ ====================

func addMoonTags(tags map[string]bool, moons int) {
	if moons == 0 {
		tags["no_satellites"] = true
		return
	}
	tags["has_satellites"] = true
	if moons >= moonsManyThreshold {
		tags["many_satellites"] = true
	}
}

// ==================== БИОСФЕРА ====================

// inhabited — есть ли на планете население. Считается по пригодности для
// людей (races.HumansSuitable, флаг Settleable каскада, 65a): где поселение
// появится — там население. Биологическая жизнь не обязательна.
func inhabited(ctx DescriptionContext) bool {
	return ctx.Settleable
}

func addBiosphereTags(tags map[string]bool, life, inhabited bool) {
	if life {
		tags["has_biosphere"] = true
	} else {
		tags["no_biosphere"] = true
	}
	if inhabited {
		tags["inhabited"] = true
	}
}

// ==================== ОРБИТА ====================

func addOrbitTags(tags map[string]bool, orbitIndex int) {
	if orbitIndex <= 0 {
		return
	}
	if orbitIndex <= orbitInnerMax {
		tags["inner_orbit"] = true
	}
	if orbitIndex >= orbitOuterMin {
		tags["outer_orbit"] = true
	}
}

// ==================== ТЕМПЕРАТУРА ====================

func addTemperatureTags(tags map[string]bool, temp float64) {
	switch {
	case temp < tempColdMax:
		tags["cold"] = true
	case temp <= tempTemperateMax:
		tags["temperate"] = true
	default:
		tags["hot"] = true
	}
}

// ==================== ВОЗРАСТ ====================

func addAgeTags(tags map[string]bool, age float64) {
	if age < ageYoungMax {
		tags["young"] = true
	}
	if age > ageOldMin {
		tags["old"] = true
	}
}

// ==================== ПЛОТНОСТЬ ====================

func addDensityTags(tags map[string]bool, density float64) {
	if density < densityLowMax {
		tags["low_density"] = true
	}
	if density > densityHighMin {
		tags["high_density"] = true
	}
}

// ==================== МАССА ====================

func addMassTags(tags map[string]bool, mass float64, isGasGiant bool) {
	threshold := massThreshold
	if isGasGiant {
		threshold = massThresholdGas
	}
	if mass > threshold {
		tags["massive"] = true
	}
}

// ==================== ЯДРО ====================

func addCoreTags(tags map[string]bool, core Core) {
	switch core.Type {
	case CoreIce:
		tags["ice_core"] = true
	case CoreMetallic:
		tags["metallic_core"] = true
	case CoreSilicate:
		tags["silicate_core"] = true
	case CoreExotic:
		tags["exotic_core"] = true
	}
	if core.Activity > coreActiveMin {
		tags["active_core"] = true
	}
	if core.Radioactivity > coreRadioactiveMin {
		tags["radioactive_core"] = true
	}
}

// ==================== КРИОВУЛКАНИЗМ ====================

func addCryovolcanicTag(tags map[string]bool, surface Composition, core Core) {
	if surface == nil {
		return
	}
	if surface.ShareOf(SurfaceGlaciers) >= cryovolcanicGlacierMin &&
		core.Activity > cryovolcanicActivityMin {
		tags["cryovolcanic"] = true
	}
}

// ==================== ПОДПОВЕРХНОСТНЫЙ ОКЕАН ====================

func addSubsurfaceOceanTag(tags map[string]bool, ctx DescriptionContext) {
	if ctx.Hydrosphere == "подлёдная" {
		tags["has_subsurface_ocean"] = true
		return
	}
	if ctx.Temperature < subsurfaceTempMax &&
		ctx.WaterPercent > subsurfaceWaterMin {
		tags["has_subsurface_ocean"] = true
	}
}

// ==================== АТМОСФЕРА ====================

func addAtmosphereTags(tags map[string]bool, atmosphere string) {
	switch atmosphere {
	case "разреженная":
		tags["no_atmosphere"] = true
	case "плотная", "парниковая":
		tags["dense_atmosphere"] = true
	case "ядовитая":
		tags["toxic_atmosphere"] = true
	}
}