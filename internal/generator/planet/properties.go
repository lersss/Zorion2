// internal/generator/planet/properties.go
package planet

import (
	"math"
	"math/rand"

	"zorion/internal/generator/settlement"
)

// Properties — физические параметры конкретной планеты.
type Properties struct {
	Size                  float64 // радиус в земных
	Mass                  float64 // масса в земных
	Density               float64 // плотность (в единицах Земли)
	Atmosphere            string
	Temperature           float64
	WaterPercent          float64
	Moons                 int
	Settleable            bool
	Life                  bool
	Political             string
	ConflictLevel         float64
	Development           float64
	SurfaceComposition    Composition
	SubterrainComposition Composition
	Core                  Core
}

// GenerateProperties — рассчитывает параметры планеты на основе архетипа.
func GenerateProperties(
	archetype *Archetype,
	orbitIndex int,
	spectralClass string,
	systemAge float64,
	rng *rand.Rand,
) *Properties {
	// 1. Масса — первичный параметр
	mass := archetype.MassMin + rng.Float64()*(archetype.MassMax-archetype.MassMin)

	// 2. Предварительная композиция поверхности
	preliminarySurface := GenerateSurfaceComposition(
		archetype.BaseSurface,
		0, 0,
		rng,
	)

	// 3. Плотность и размер
	density := densityForPlanet(mass, preliminarySurface, rng)
	size := computeRadius(mass, density)

	// 4. Предварительная композиция недр
	preliminarySubterrain := GenerateSubterrainComposition(
		archetype.BaseSubterrain,
		preliminarySurface,
		0, 0,
		rng,
	)

	// 5. Ядро
	core := GenerateCore(mass, archetype.Climate, preliminarySubterrain, systemAge, rng)

	// 6. Температура
	luminosity := luminosityBySpectral(spectralClass)
	orbitRadius := orbitRadiusByIndex(orbitIndex)

	temp := computeSurfaceTemp(SurfaceTempInput{
		Luminosity:   luminosity,
		OrbitRadius:  orbitRadius,
		Surface:      preliminarySurface,
		Atmosphere:   archetype.Atmosphere,
		Core:         core,
		TidalHeat:    0,
		ArchetypeMin: archetype.TemperatureMin,
		ArchetypeMax: archetype.TemperatureMax,
		SkipInternal: false,
	})

	// 7. Вода
	waterPercent := generateWater(archetype, temp, rng)

	// 8. Финальные композиции
	surfaceComp := GenerateSurfaceComposition(
		archetype.BaseSurface,
		temp,
		waterPercent,
		rng,
	)
	subterrainComp := GenerateSubterrainComposition(
		archetype.BaseSubterrain,
		surfaceComp,
		temp,
		waterPercent,
		rng,
	)

	// ~3–4% планет — «примитивные» тела: 1–2 типа поверхности и недр.
	if rng.Float64() < primitivePlanetProbability {
		surfaceComp = simplifyComposition(surfaceComp, rng)
		subterrainComp = simplifyComposition(subterrainComp, rng)
	}

	// 9. Жизнь и пригодность под поселение
	life := generateLife(archetype, waterPercent, temp, rng)
	settleable := settlement.Suitable(temp, waterPercent, archetype.Atmosphere, life, false, false)

	moons := determineMoons(size, archetype.Climate, rng)

	political := "нет"
	if settleable {
		systems := []string{
			"демократия", "диктатура", "теократия",
			"корпоратократия", "анархия", "ИИ-управление",
		}
		political = systems[rng.Intn(len(systems))]
	}

	conflict := 0.0
	devLevel := 0.0
	if settleable {
		conflict = rng.Float64()
		devLevel = 0.1 + rng.Float64()*0.9
	}

	return &Properties{
		Size:                  size,
		Mass:                  mass,
		Density:               density,
		Atmosphere:            archetype.Atmosphere,
		Temperature:           temp,
		WaterPercent:          waterPercent,
		Moons:                 moons,
		Settleable:            settleable,
		Life:                  life,
		Political:             political,
		ConflictLevel:         conflict,
		Development:           devLevel,
		SurfaceComposition:    surfaceComp,
		SubterrainComposition: subterrainComp,
		Core:                  core,
	}
}

// ==================== ПЛОТНОСТЬ И РАЗМЕР ====================

// densityForPlanet — плотность планеты (в единицах Земли).
func densityForPlanet(mass float64, surface Composition, rng *rand.Rand) float64 {
	if mass > 30 {
		return 0.2 + rng.Float64()*0.1
	}

	dominant := surface.DominantForm()
	switch dominant {
	case SurfaceGlaciers, SurfaceFrozenGases:
		return 0.5 + rng.Float64()*0.3
	case SurfaceOceans, SurfaceLakes:
		return 0.8 + rng.Float64()*0.2
	case SurfaceMetalFields:
		return 1.2 + rng.Float64()*0.4
	case SurfaceGlassFields:
		return 1.0 + rng.Float64()*0.2
	default:
		return 0.9 + rng.Float64()*0.4
	}
}

// computeRadius — радиус из массы и плотности.
// R = (M / ρ)^(1/3), в земных единицах.
func computeRadius(mass, density float64) float64 {
	if density <= 0 {
		density = 1.0
	}
	if mass <= 0 {
		mass = 0.1
	}
	return math.Pow(mass/density, 1.0/3.0)
}

// ==================== ВОДА, ЖИЗНЬ, СПУТНИКИ ====================

// Пороги физических состояний воды для гейта гидросфер:
//
//	heatThreshold — граница «жары» (совпадает с порогом аудита
//	                oceans_in_heat): выше жидкой воды не бывает;
//	freezingPoint — точка замерзания воды.
const (
	heatThreshold = 400.0
	freezingPoint = 273.0
)

func generateWater(archetype *Archetype, temp float64, rng *rand.Rand) float64 {
	switch archetype.Hydrosphere {
	case "океаны":
		// Жидкие океаны — только до порога «жары» (тот же, что у аудита
		// oceans_in_heat). Выше — воды нет: пара, а не океан.
		if temp <= heatThreshold {
			return 70 + rng.Float64()*25
		}
		return 0
	case "озёра":
		if temp <= heatThreshold {
			return 20 + rng.Float64()*40
		}
		return 0
	case "ледяной покров":
		// Лёд — только ниже точки замерзания.
		if temp < freezingPoint {
			return 5 + rng.Float64()*20
		}
		return 0
	case "подлёдная":
		// Подлёдный океан — только под замёрзшей поверхностью.
		if temp < freezingPoint {
			return 50 + rng.Float64()*40
		}
		return 0
	case "кислотная":
		return 0
	case "сухая":
		return 0
	}

	if temp > 250 && temp < 400 {
		if rng.Float64() < archetype.WaterChance {
			return 30 + rng.Float64()*60
		}
	} else if temp > 150 && temp <= 250 {
		if rng.Float64() < archetype.WaterChance*0.7 {
			return 20 + rng.Float64()*40
		}
	} else {
		if rng.Float64() < archetype.WaterChance*0.3 {
			return 10 + rng.Float64()*20
		}
	}
	return 0
}

func generateLife(archetype *Archetype, waterPercent, temp float64, rng *rand.Rand) bool {
	if archetype.Biosphere == "стерильная" {
		return false
	}
	if waterPercent <= 10 {
		return false
	}
	if temp <= 200 || temp >= 400 {
		return false
	}
	return rng.Float64() < archetype.LifeChance
}

func determineMoons(size float64, climate string, rng *rand.Rand) int {
	base := 0
	switch climate {
	case "hot", "extreme":
		base = int(size)
	case "cold":
		base = int(size * 1.5)
	default:
		base = int(size * 1.2)
	}
	if base < 0 {
		base = 0
	}
	jitter := rng.Intn(3) - 1
	moons := base + jitter
	if moons < 0 {
		moons = 0
	}
	return moons
}