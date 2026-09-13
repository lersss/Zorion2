// internal/generator/planet/physics.go
package planet

import "math"

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

// albedoFactor — множитель температуры от альбедо.
//
// Формула: (1 - albedo)^0.25
// Пример: albedo 0.65 → (0.35)^0.25 ≈ 0.77 (на 23% холоднее).
func albedoFactor(albedo float64) float64 {
	if albedo < 0 {
		albedo = 0
	}
	if albedo > 0.9 {
		albedo = 0.9
	}
	return math.Pow(1.0-albedo, 0.25)
}

// ==================== ПАРНИКОВЫЙ ЭФФЕКТ ====================

// greenhouseByAtmosphere — множитель температуры от типа атмосферы.
// Значения подобраны эмпирически: Марс 1.0, Земля 1.1, Венера 2.5.
var greenhouseByAtmosphere = map[string]float64{
	"разреженная":         1.00,
	"":                    1.00,
	"азотная":             1.05,
	"азотно-кислородная":  1.10,
	"туманная":            1.15,
	"облачная":            1.20,
	"углекислая":          1.30,
	"электрическая":       1.30,
	"ядовитая":            1.40,
	"метановая":           1.50,
	"плотная":             1.60,
	"водородная":          1.80,
	"гелиевая":            1.80,
	"водородно-гелиевая":  1.90,
	"парниковая":          2.50,
}

// computeGreenhouse — множитель парникового эффекта по типу атмосферы.
func computeGreenhouse(atmosphere string) float64 {
	if g, ok := greenhouseByAtmosphere[atmosphere]; ok {
		return g
	}
	return 1.0
}

// ==================== ВНУТРЕННИЙ НАГРЕВ (от ядра) ====================

// computeInternalHeat — вклад ядра в температуру поверхности.
// Делегируется в Core.HeatContribution().
func computeInternalHeat(core Core) float64 {
	return core.HeatContribution()
}

// ==================== ПРИЛИВНЫЙ НАГРЕВ ====================

// computeTidalHeat — приливный нагрев (только для спутников).
//
// Обратно пропорционален кубу орбитального индекса.
// Для спутников газовых гигантов — существенно, для планет — пренебрежимо.
func computeTidalHeat(giantTemp float64, orbitIndex int) float64 {
	if orbitIndex < 1 {
		orbitIndex = 1
	}
	return 400.0 / math.Pow(float64(orbitIndex), 3)
}

// ==================== ИТОГОВАЯ ТЕМПЕРАТУРА ====================

// SurfaceTempInput — входные данные для расчёта температуры поверхности.
type SurfaceTempInput struct {
	Luminosity      float64     // светимость звезды (в солнечных)
	OrbitRadius     float64     // радиус орбиты (а.е.)
	Surface         Composition // композиция поверхности (для альбедо)
	Atmosphere      string      // тип атмосферы (для парника)
	Core            Core        // ядро (для внутреннего тепла)
	TidalHeat       float64     // приливный нагрев (0 для планет)
	ArchetypeMin    float64     // нижняя граница архетипа
	ArchetypeMax    float64     // верхняя граница архетипа
	SkipInternal    bool        // true для газовых гигантов
}

// computeSurfaceTemp — итоговая температура поверхности.
//
// Формула:
//
//	T_eq      = 278.7 × L^0.25 / sqrt(r)
//	T_albedo  = T_eq × (1 - albedo)^0.25
//	T_green   = T_albedo × greenhouse_factor
//	T_final   = T_green + internal_heat + tidal_heat
//	T_clamped = clamp(T_final, archetype_min, archetype_max)
//	T_clamped = clamp(T_clamped, TempAbsoluteMin, TempAbsoluteMax)
func computeSurfaceTemp(in SurfaceTempInput) float64 {
	tEq := computeEquilibriumTemp(in.Luminosity, in.OrbitRadius)

	albedo := computeAlbedo(in.Surface)
	tAfterAlbedo := tEq * albedoFactor(albedo)

	greenhouse := computeGreenhouse(in.Atmosphere)
	tAfterGreenhouse := tAfterAlbedo * greenhouse

	var internalHeat float64
	if !in.SkipInternal {
		internalHeat = computeInternalHeat(in.Core)
	}

	tFinal := tAfterGreenhouse + internalHeat + in.TidalHeat

	// Ограничение диапазоном архетипа
	if in.ArchetypeMin > 0 && tFinal < in.ArchetypeMin {
		tFinal = in.ArchetypeMin
	}
	if in.ArchetypeMax > 0 && tFinal > in.ArchetypeMax {
		tFinal = in.ArchetypeMax
	}

	// Абсолютные границы
	return clamp(tFinal, TempAbsoluteMin, TempAbsoluteMax)
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

// luminosityBySpectral — светимость звезды по спектральному классу.
func luminosityBySpectral(spectralClass string) float64 {
	table := map[string]float64{
		"O": 1000, "B": 100, "A": 10, "F": 2, "G": 1,
		"K": 0.1, "M": 0.01, "L": 0.001, "T": 0.0001, "Y": 0.00001,
	}
	if l, ok := table[spectralClass]; ok && l > 0 {
		return l
	}
	return 1.0
}