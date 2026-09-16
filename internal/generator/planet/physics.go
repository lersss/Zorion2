// internal/generator/planet/physics.go
package planet

import (
	"math"
	"math/rand"

	"zorion/internal/astro"
)

// ==================== ФИЗИЧЕСКИЕ ГРАНИЦЫ ====================

const (
	// Абсолютные границы температуры поверхности (K).
	TempAbsoluteMin = 20.0
	TempAbsoluteMax = 2500.0

	// Солнечная постоянная (K) — равновесная температура Земли без атмосферы.
	// Используется как база для формулы равновесной температуры.
	solarConstant = 278.7
)

// ==================== РАВНОВЕСНАЯ ТЕМПЕРАТУРА ====================

// computeEquilibriumTemp — равновесная температура планеты от звезды.
//
// Формула: T_eq = 278.7 × L^0.25 / sqrt(r)
// где L — светимость звезды (в солнечных), r — орбитальный радиус (а.е.).
//
// Планета не учитывает атмосферу и альбедо — это «голая» температура,
// которую затем корректируют AlbedoFactor и GreenhouseFactor.
func computeEquilibriumTemp(luminosity, orbitRadius float64) float64 {
	if luminosity <= 0 {
		luminosity = 1.0
	}
	if orbitRadius <= 0 {
		orbitRadius = 0.4
	}
	return solarConstant * math.Pow(luminosity, 0.25) / math.Sqrt(orbitRadius)
}

// ==================== АЛЬБЕДО ====================

// albedoByForm — отражательная способность (0–1) по доминирующей форме.
// Больше — холоднее, меньше — горячее.
var albedoByForm = map[string]float64{
	SurfaceGlaciers:       0.65,
	SurfaceFrozenGases:    0.60,
	SurfaceSands:          0.35,
	SurfaceGlassFields:    0.25,
	SurfaceMetalFields:    0.40,
	SurfaceCraters:        0.15,
	SurfaceRocks:          0.15,
	SurfaceOceans:         0.06,
	SurfaceLakes:          0.08,
	SurfaceMeadows:        0.15,
	SurfaceForests:        0.15,
	SurfaceJungles:        0.13,
	SurfaceSwamps:         0.12,
	SurfaceCoralReefs:     0.10,
	SurfaceLavaFields:     0.10,
	SurfaceVolcanicFields: 0.12,
}

// computeAlbedo — альбедо по доминирующей форме поверхности.
// Возвращает значение в диапазоне [0.05, 0.7].
func computeAlbedo(surface Composition) float64 {
	dominant := surface.DominantForm()
	if dominant == "" {
		return 0.3 // среднее по умолчанию
	}
	if a, ok := albedoByForm[dominant]; ok {
		return a
	}
	return 0.3
}

// ==================== ГРАВИТАЦИЯ ====================

// computeGravity — поверхностная гравитация в земных g.
// g = M / R², где M — масса в земных, R — радиус в земных радиусах.
func computeGravity(mass, size float64) float64 {
	if size <= 0 {
		size = 1
	}
	return mass / (size * size)
}

// ==================== ХЕЛПЕРЫ ====================

// orbitRadiusByIndex — радиус орбиты по индексу (0.4 × 1.7^index).
func orbitRadiusByIndex(orbitIndex int) float64 {
	if orbitIndex < 1 {
		orbitIndex = 1
	}
	return 0.4 * math.Pow(1.7, float64(orbitIndex))
}

// ==================== СВЕТИМОСТЬ ====================

// luminosityBySpectral — светимость звезды по спектральному классу.
// Единый источник таблицы — internal/astro (35b §3): galaxy сравнивает
// светимости при сортировке «главная = ярче», planet считает температуры
// по L. Fallback 1.0 («как Солнце») закреплён тестами physics_test.go.
func luminosityBySpectral(spectralClass string) float64 {
	return astro.LuminosityBySpectral(spectralClass)
}

// ==================== ЧИСЛО ПЛАНЕТ: MEAN-МОДЕЛЬ (99.2.4 §5.2) ====================

// meanPlanetCount — число планет по среднему: n = floor(mean) + Бернулли(frac),
// потолок max (по умолчанию 8 — Kepler-90). При mean < 1 даёт n ∈ {0, 1}
// («чаще 0» сохраняется: P(0) = 1 − mean). Честное среднее E[n] = mean
// выполняется только при mean ≤ max.
func meanPlanetCount(rng *rand.Rand, mean float64, max int) int {
	if mean <= 0 {
		return 0
	}
	if max <= 0 {
		max = 8
	}
	n := int(math.Floor(mean))
	if frac := mean - math.Floor(mean); rng.Float64() < frac {
		n++
	}
	if n > max {
		n = max
	}
	return n
}