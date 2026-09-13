// internal/generator/planet/composition_generator.go
package planet

import "math/rand"

// GenerateSurfaceComposition — гибридная генерация композиции поверхности.
//
// Шаги:
//  1. Копия базовых весов из архетипа.
//  2. Корректировка по температуре (лава, лёд, стекло).
//  3. Корректировка по воде (океаны/озёра).
//  4. Рандом ±20% на каждую форму.
//  5. Удаление несовместимых пар через матрицу.
//  6. Нормализация к 100%.
func GenerateSurfaceComposition(
	base map[string]float64,
	temperature, waterPercent float64,
	rng *rand.Rand,
) Composition {
	c := copyWeights(base)
	if len(c) == 0 {
		return Composition{}
	}

	// 2. Температурные корректировки
	applySurfaceTempModifiers(c, temperature)

	// 3. Корректировки по воде
	applySurfaceWaterModifiers(c, waterPercent)

	// 3а. Биосферные ниши: луга/джунгли/болота получают доминирование в своём архетипе
	applyBiosphereModifiers(c, temperature, waterPercent)

	// 4. Рандом ±20%
	applyRandomJitter(c, rng)

	// 5. Убираем несовместимые пары
	c = resolveConflicts("surface", c, base, rng)

	// 6. Нормализация
	result := Composition(c).Normalize().NonZero()
	if len(result) == 0 {
		return fallbackComposition(base)
	}
	return result
}

// GenerateSubterrainComposition — генерирует композицию недр.
// Учитывает:
//   - базовые веса из архетипа;
//   - температуру и воду планеты;
//   - связь с поверхностью (мягкая);
//   - несовместимости.
func GenerateSubterrainComposition(
	base map[string]float64,
	surface Composition,
	temperature, waterPercent float64,
	rng *rand.Rand,
) Composition {
	c := copyWeights(base)
	if len(c) == 0 {
		return Composition{}
	}

	// 1. Корректировки по температуре
	applySubterrainTempModifiers(c, temperature)

	// 2. Корректировки по воде
	applySubterrainWaterModifiers(c, waterPercent)

	// 3. Мягкое влияние поверхности
	applySurfaceToSubterrainLinks(c, surface)

	// 4. Рандом ±20%
	applyRandomJitter(c, rng)

	// 5. Убираем несовместимые пары
	c = resolveConflicts("subterrain", c, base, rng)

	// 6. Нормализация
	result := Composition(c).Normalize().NonZero()
	if len(result) == 0 {
		return fallbackComposition(base)
	}
	return result
}