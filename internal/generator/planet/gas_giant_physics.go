// internal/generator/planet/gas_giant_physics.go
//
// Физика газовых гигантов: кривая масса→радиус с насыщением и границы
// генератора. Канон чисел — docs/gamedesign/03_planets.md §3.3 (99.2.15, с
// переанкеровкой кривой решением создателя 2026-09-23 — спека
// 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны §7.5); рамки реестра
// полей (internal/generator/settlement/fields.go) берутся от этих констант,
// консистентность проверяется тестом TestFieldRegistryCoversGenerator.
package planet

import "math"

// Экспортируемые границы и анкеры генератора газовых гигантов.
const (
	// 4131 = 13 MJ (13×317.8 = 4131.4, округлено вниз) — верх физического
	// предела газового гиганта: выше — коричневые карлики (>13 MJ). Отсечение
	// хвоста — ПЕРЕСЭМПЛИНГОМ системного M_gas (overflow.go), не клампом §7.3.
	GasGiantMassMax = 4131.0
	// GasGiantMassMin — низ класса гиганта = верх зоны мини-нептунов (§7.1,
	// §11.1). Производная от границы бифуркации, не «0.05 MJ».
	GasGiantMassMin = 16.0
	// GasGiantMassRef — анкер кривой M→R (Mref, Rref = 11.2): НЕ медиана
	// распределения (§7.4). Целевая медиана — GasGiantTargetMedian.
	GasGiantMassRef = 317.8
	// GasGiantTargetMedian — целевая медиана распределения M_body (1 MJ) —
	// константа калибровки (§10.5), не анкер кривой.
	GasGiantTargetMedian = 317.8
	// GasGiantRadiusMin — низ ПЕРЕАНКЕРЕННОЙ кривой (16 → 3.7): стыкуется с
	// кривой мини-нептуна (R_minineptune(16) ≈ 3.73), Δ ≈ 0.03 R⊕ (§7.5).
	GasGiantRadiusMin = 3.7
	GasGiantRadiusMax = 11.2
	// gasGiantMassMid/gasGiantRadiusMid — средний анкер кривой (Сатурн:
	// 95 M⊕ → 9.4 R⊕, §7.5).
	gasGiantMassMid   = 95.0
	gasGiantRadiusMid = 9.4
)

// GasGiantRadius — радиус газового гиганта по массе: переанкеренная кривая
// (решение создателя 2026-09-23, §7.5) через реальные объекты:
//
//	16 → 3.7   (низ, стык с мини-нептуном)
//	95 → 9.4   (Сатурн)
//	317.8 → 11.2 (Юпитер; плато насыщения — электронное вырождение)
//
// Три сегмента степенной кривой; выше 1 MJ — плато 11.2 R⊕. Наивная формула
// (M/ρ)^(1/3) для гигантов отклонена создателем.
func GasGiantRadius(mass float64) float64 {
	switch {
	case mass < gasGiantMassMid:
		// α1 = ln(9.4/3.7)/ln(95/16) ≈ 0.5235
		alpha1 := math.Log(gasGiantRadiusMid/GasGiantRadiusMin) /
			math.Log(gasGiantMassMid/GasGiantMassMin)
		return GasGiantRadiusMin * math.Pow(mass/GasGiantMassMin, alpha1)
	case mass < GasGiantMassRef:
		// α2 = ln(11.2/9.4)/ln(317.8/95) ≈ 0.1451
		alpha2 := math.Log(GasGiantRadiusMax/gasGiantRadiusMid) /
			math.Log(GasGiantMassRef/gasGiantMassMid)
		return gasGiantRadiusMid * math.Pow(mass/gasGiantMassMid, alpha2)
	default:
		return GasGiantRadiusMax
	}
}

// gasGiantMass — масса газового гиганта: усечённое логнормальное
// распределение (медиана 317.8 M⊕ = 1 MJ, σ = 0.9 декады) на
// [GasGiantMassMin, GasGiantMassMax] = [16, 4131].
// Пересэмплинг до попадания в диапазон — НЕ кламп: кламп дал бы пайк ~10%
// всех гигантов ровно на 4131 (моно-масса 13 MJ) и исказил бы доли
// распределения (99.2.15 §3.2).
//
// Нижняя граница — GasGiantMassMin = 16 (граница класса мини-нептун/гигант,
// спека 2026-09-23 §7.4/§11.1), а не прежние 15.9: у P-ветки гигант теперь
// тоже ≥ 16 (мелкий сдвиг, назван — спека §9.2 синхронизируется дизайнером).
//
// Сохранена только для P-ветки двойных (circumbinary.go: у циркумбинарной
// P-планеты переполняться нечему — решение создателя 2026-09-23, спека
// 2026-09-23 §9.2). На пути перелива масса гиганта — выход аккреции
// (overflow.go, M_body = ядро + оболочка).
func (g *Generator) gasGiantMass() float64 {
	for {
		z := g.rng.NormFloat64()
		m := GasGiantMassRef * math.Pow(10, 0.9*z)
		if m >= GasGiantMassMin && m <= GasGiantMassMax {
			return m
		}
	}
}
