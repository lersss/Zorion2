// internal/resource/generator.go
package resource

import (
	"math/rand"

	"github.com/google/uuid"
	"zorion/internal/models"
	"zorion/internal/names"
)

// GenerateResources — создаёт набор ресурсов для планеты.
//
// Параметры:
//   - planetID — ID планеты.
//   - surfaceDominant — доминирующая форма поверхности (например, "океаны").
//   - subterrainComposition — композиция недр (тип → процент).
//     Ресурсы генерируются только из типов с долей > minSubterrainShare.
//   - spectralClass — спектральный класс звезды (влияет на редкость).
//   - rng — источник случайности.
//   - resourceBias — веса категорий профиля региона (59a §8 P2): весовой
//     выбор категории (вес = bias, отсутствующая категория = 1.0); nil —
//     равномерный выбор. Bias НЕ добавляет категорию — только взвешивает
//     существующие в наборе.
//
// Возвращает список ресурсов с уникальными именами (в кириллице и латинице).
func GenerateResources(
	planetID string,
	surfaceDominant string,
	subterrainComposition map[string]float64,
	spectralClass string,
	rng *rand.Rand,
	resourceBias map[string]float64,
) []*models.PlanetResource {
	// 1. Собираем категории из поверхности и недр
	surfaceCats := CategoriesForSurface(surfaceDominant)

	subterrainCats := []string{}
	for subType, share := range subterrainComposition {
		if share < minSubterrainShare {
			continue
		}
		subterrainCats = append(subterrainCats, CategoriesForSubterrain(subType)...)
	}

	allCategories := MergeCategories(surfaceCats, subterrainCats)
	if len(allCategories) == 0 {
		// Fallback — если ничего не сгенерировалось
		allCategories = []string{CategoryMineral}
	}

	// 2. Определяем общее количество ресурсов
	total := determineTotal(surfaceDominant, subterrainComposition, spectralClass, rng)
	if total == 0 {
		return nil
	}

	// 3. Половина ресурсов — known, половина — unknown
	knownCount := total / 2
	if knownCount < 1 {
		knownCount = 1
	}
	unknownCount := total - knownCount

	existingNames := make(map[string]bool)
	result := make([]*models.PlanetResource, 0, total)

	// 4. Известные
	for i := 0; i < knownCount; i++ {
		cat := pickCategory(allCategories, rng, resourceBias)
		res := generateResource(planetID, cat, true, rng, existingNames)
		if res != nil {
			result = append(result, res)
		}
	}

	// 5. Неизвестные
	for i := 0; i < unknownCount; i++ {
		cat := pickCategory(allCategories, rng, resourceBias)
		res := generateResource(planetID, cat, false, rng, existingNames)
		if res != nil {
			result = append(result, res)
		}
	}

	return result
}

// GenerateGasGiantResource — создаёт один ресурс для газового гиганта.
// Гигант не имеет поверхности/недр — берём из атмосферы: газ или топливо.
func GenerateGasGiantResource(planetID string, rng *rand.Rand) []*models.PlanetResource {
	category := CategoryGas
	if rng.Intn(2) == 1 {
		category = CategoryFuel
	}

	existingNames := make(map[string]bool)
	res := generateResource(planetID, category, true, rng, existingNames)
	if res == nil {
		return nil
	}
	return []*models.PlanetResource{res}
}

// minSubterrainShare — минимальная доля типа недр, чтобы он давал ресурсы.
// Например, если тип занимает 5% недр — ресурсы из него не генерируются.
const minSubterrainShare = 5.0

// ==================== ХЕЛПЕРЫ ====================

// pickCategory — случайная категория из списка. Без bias — равномерно;
// с bias — весовой выбор (вес = bias[категория], отсутствующая = 1.0).
// Bias не добавляет категорию — только взвешивает существующие (59a §8 P2).
func pickCategory(categories []string, rng *rand.Rand, bias map[string]float64) string {
	if len(categories) == 0 {
		return CategoryMineral
	}
	if len(bias) == 0 {
		return categories[rng.Intn(len(categories))]
	}
	total := 0.0
	for _, c := range categories {
		total += categoryWeight(c, bias)
	}
	r := rng.Float64() * total
	for _, c := range categories {
		r -= categoryWeight(c, bias)
		if r <= 0 {
			return c
		}
	}
	return categories[len(categories)-1]
}

// categoryWeight — вес категории: bias[категория] (если > 0), иначе 1.0.
func categoryWeight(category string, bias map[string]float64) float64 {
	if w, ok := bias[category]; ok && w > 0 {
		return w
	}
	return 1.0
}

// generateResource — генерирует один ресурс заданной категории.
func generateResource(
	planetID string,
	category string,
	known bool,
	rng *rand.Rand,
	existingNames map[string]bool,
) *models.PlanetResource {
	name := names.GenerateResourceName(category, rng, existingNames)
	base := GetBaseProps(category)

	// Процент = base ± propJitter, с обрезкой в [0, 100]
	return &models.PlanetResource{
		ID:               uuid.New().String(),
		PlanetID:         planetID,
		Name:             name.Cyr,
		Category:         category,
		Hardness:         jittered(base.Hardness, rng),
		Elasticity:       jittered(base.Elasticity, rng),
		Conductivity:     jittered(base.Conductivity, rng),
		HeatResistance:   jittered(base.HeatResistance, rng),
		ChemicalActivity: jittered(base.ChemicalActivity, rng),
		Density:          jittered(base.Density, rng),
		Biocompatibility: jittered(base.Biocompatibility, rng),
		EnergyDensity:    jittered(base.EnergyDensity, rng),
		Volatility:       jittered(base.Volatility, rng),
		Quantity:         100 + rng.Intn(401),
		IsKnown:          known,
	}
}

// jittered — базовое значение ± propJitter, обрезанное в [0, 100].
func jittered(base float64, rng *rand.Rand) float64 {
	shift := (rng.Float64() - 0.5) * 2 * propJitter
	return clamp(base+shift, 0, 100)
}

// clamp — ограничивает значение диапазоном [min, max].
func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
