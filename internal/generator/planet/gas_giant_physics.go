// internal/generator/planet/gas_giant_physics.go
//
// Физика газовых гигантов: кривая масса→радиус с насыщением и границы
// генератора. Канон чисел — docs/gamedesign/03_planets.md §3.3 (99.2.15);
// рамки реестра полей (internal/generator/settlement/fields.go) берутся от
// этих констант, консистентность проверяется тестом TestFieldRegistryCoversGenerator.
package planet

import "math"

// Экспортируемые границы генератора газовых гигантов.
const (
	// 4131 = 13 MJ (13×317.8 = 4131.4, округлено вниз) — единственная
	// константа верхней массы: выше — коричневые карлики (>13 MJ).
	GasGiantMassMin   = 15.9   // 0.05 MJ — низ диапазона (Нептун ≈ 17 M⊕)
	GasGiantMassMax   = 4131.0 // 13 MJ — верх диапазона
	GasGiantMassRef   = 317.8  // 1 MJ — медиана распределения и анкер кривой
	GasGiantRadiusMin = 5.9
	GasGiantRadiusMax = 11.2
)

// GasGiantRadius — радиус газового гиганта по массе (кривая с насыщением,
// вариант А спеки 99.2.15 §3.3): степенной участок до 1 MJ через опорные
// точки P1=(15.9; 5.9), P2=(317.8; 11.2), плато 11.2 R⊕ выше (электронное
// вырождение держит радиус ~1 RJ). Наивная формула (M/ρ)^(1/3) для гигантов
// отклонена создателем.
func GasGiantRadius(mass float64) float64 {
	if mass <= GasGiantMassRef {
		// α = ln(5.9/11.2)/ln(15.9/317.8) ≈ 0.214
		alpha := math.Log(GasGiantRadiusMin/GasGiantRadiusMax) /
			math.Log(GasGiantMassMin/GasGiantMassRef)
		return GasGiantRadiusMax * math.Pow(mass/GasGiantMassRef, alpha)
	}
	return GasGiantRadiusMax
}

// gasGiantMass — масса газового гиганта: усечённое логнормальное
// распределение (медиана 317.8 M⊕ = 1 MJ, σ = 0.9 декады) на [15.9, 4131].
// Пересэмплинг до попадания в диапазон — НЕ кламп: кламп дал бы пайк ~10%
// всех гигантов ровно на 4131 (моно-масса 13 MJ) и исказил бы доли
// распределения (спека §3.2).
func (g *Generator) gasGiantMass() float64 {
	for {
		z := g.rng.NormFloat64()
		m := GasGiantMassRef * math.Pow(10, 0.9*z)
		if m >= GasGiantMassMin && m <= GasGiantMassMax {
			return m
		}
	}
}