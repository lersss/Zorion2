// internal/generator/planet/cascade_minineptune.go
//
// Каскад мини-нептуна (спека
// 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны, §8, §11.3): тело
// (M_crit, 16] M⊕ с водородно-гелиевой оболочкой умеренной массы. Отдельный
// путь: runCascade дал бы скалистые биомы/недра (физически неверно), а
// runCascadeGiant жёстко зашивает кривую гигантов и флаг is_gas_giant.
package planet

import "math"

// ==================== КРИВАЯ M→R (§8.2) ====================

const (
	// MiniNeptuneMassRef/MiniNeptuneRadiusRef — анкер кривой: K2-18 b
	// (8.6 M⊕, 2.61 R⊕) — реальная опора класса.
	MiniNeptuneMassRef   = 8.6
	MiniNeptuneRadiusRef = 2.61
	// minineptuneAlpha — показатель: ln(3.88/2.61)/ln(17.1/8.6) ≈ 0.577;
	// вторая опора — Нептун (17.1 M⊕, 3.88 R⊕) — точка СТЫКА кривых (§8.1).
	minineptuneAlpha = 0.577
)

// MiniNeptuneRadius — радиус мини-нептуна по массе: R = 2.61·(M/8.6)^0.577.
// R(8) ≈ 2.50, R(16) ≈ 3.73 (стык с гигантом: R_giant(16) = 3.70, §7.5).
// Кривая построена по H/He-обогащённым телам: у плотных ледяных (Уран)
// недооценивает радиус ≈12% — названное расхождение (§8.1).
func MiniNeptuneRadius(mass float64) float64 {
	return MiniNeptuneRadiusRef * math.Pow(mass/MiniNeptuneMassRef, minineptuneAlpha)
}

// ==================== КАСКАД МИНИ-НЕПТУНА (§8.3, §11.3) ====================

// runCascadeMiniNeptune — каскад мини-нептуна: оболочечная атмосфера H₂/He
// (как у гиганта: P = 1 атм, малое τ_IR, F_KH), но радиус — по кривой
// мини-нептуна (§8.2), поверхностей/биомов/недр нет, life/settleable — false.
// Светимость в массу не входит (масса приходит готовой из пред-слоя, §4.2).
func (g *Generator) runCascadeMiniNeptune(in cascadeInput) *cascadeResult {
	res := &cascadeResult{}

	mass := in.MassOverride
	if mass <= 0 {
		mass = GasGiantMassMin // страховка прямого вызова
	}
	size := MiniNeptuneRadius(mass)
	density := mass / (size * size * size)
	gravity := computeGravity(mass, size)
	res.Mass, res.Size, res.Density, res.Gravity = mass, size, density, gravity
	res.EscapeVelocity = escapeVelocity(mass, size)

	res.OrbitalPeriod = orbitalPeriod(in.OrbitRadiusAU, in.StellarMass)
	res.Eccentricity = 0.03 + g.rng.Float64()*0.27
	res.TidalLock = res.OrbitalPeriod < tidalLockThresholdYears

	core := GenerateCore(mass, "жаркий", Composition{}, in.AgeGyr, g.rng)
	heatFlux := internalHeatFlux(core, in.AgeGyr, mass, true)
	core.HeatFluxWm2 = heatFlux
	res.Core, res.HeatFluxWm2 = core, heatFlux

	fStar := solarFluxAt1AU * in.Luminosity / (in.OrbitRadiusAU * in.OrbitRadiusAU)
	aSurface := 0.15 // условное альбедо оболочки
	fc := 0.30       // водородно-гелиевая (99.2.20 §3.5, таблица f_c)
	albedo := aSurface*(1-fc) + 0.7*fc
	tau := 0.05 + g.rng.Float64()*0.05
	tFinal := tempFromFlux(fStar, albedo, heatFlux, 0, tau)
	res.T0 = tempFromFlux(fStar, aSurface, heatFlux, 0, 0)
	res.T1 = res.T0 * math.Pow(1+0.75*nominalTau, 0.25)
	res.TFinal = tFinal

	comp := normalizeComposition(map[string]float64{
		"H2": 0.85 + g.rng.Float64()*0.10, // 0.85–0.95
		"He": 0.05 + g.rng.Float64()*0.10, // 0.05–0.15
	})
	mu := meanMolecularWeight(comp)
	res.AtmosphereData = AtmosphereData{
		Composition:         compositionToPct(comp),
		PressureAtm:         1.0, // условный уровень «поверхности» оболочки — 1 бар
		MassEarthAtm:        1.0 * size * size / gravity,
		TauIR:               tau,
		ScaleHeightKm:       scaleHeight(tFinal, mu, gravity),
		MeanMolecularWeight: mu,
	}
	res.AtmosphereLabel = classifyAtmosphere(comp, 1.0)
	res.LiquidWater = false
	res.Hydrosphere = "сухая"
	res.Biosphere = "стерильная"
	res.WaterPercent = 0
	res.Life = false
	res.Settleable = false
	res.Political = "нет"
	res.ArchetypeBand = "жаркий"
	return res
}

// ==================== МОЛОДЫЕ СПУТНИКИ (§8.3) ====================

// minineptuneMoons — спутники мини-нептуна: 0–4 (гипотеза §8.3, меньше, чем
// у гигантов 3–10; Уран/Нептун — исключение, не правило).
func (g *Generator) minineptuneMoons() int {
	return g.rng.Intn(5)
}
