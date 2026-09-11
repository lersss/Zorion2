// internal/generator/planet/composition_forms.go
package planet

// ==================== ФОРМЫ ПОВЕРХНОСТИ (16) ====================

const (
	SurfaceRocks          = "скалы"
	SurfaceSands          = "пески_пустыни"
	SurfaceCraters        = "кратеры"
	SurfaceGlassFields    = "стеклянные_поля"
	SurfaceMetalFields    = "металлические_поля"
	SurfaceLavaFields     = "лавовые_поля"
	SurfaceVolcanicFields = "вулканические_поля"
	SurfaceGlaciers       = "ледники"
	SurfaceFrozenGases    = "мёрзлые_газы"
	SurfaceOceans         = "океаны"
	SurfaceLakes          = "озёра_реки"
	SurfaceMeadows        = "луга_степи"
	SurfaceForests        = "леса"
	SurfaceJungles        = "джунгли"
	SurfaceSwamps         = "болота"
	SurfaceCoralReefs     = "коралловые_рифы"
)

// AllSurfaceForms — все формы поверхности (для валидации, перебора, UI)
var AllSurfaceForms = []string{
	SurfaceRocks,
	SurfaceSands,
	SurfaceCraters,
	SurfaceGlassFields,
	SurfaceMetalFields,
	SurfaceLavaFields,
	SurfaceVolcanicFields,
	SurfaceGlaciers,
	SurfaceFrozenGases,
	SurfaceOceans,
	SurfaceLakes,
	SurfaceMeadows,
	SurfaceForests,
	SurfaceJungles,
	SurfaceSwamps,
	SurfaceCoralReefs,
}

// ==================== ФОРМЫ ПОВЕРХНОСТИ СПУТНИКОВ ====================
//
// Отдельный набор форм для спутников газовых гигантов: вместо
// планетарных форм (океаны, леса, дюны...) — реголит, ледяная кора,
// криовулканы и т.п. Некоторые формы переиспользуются: кратеры,
// мёрзлые газы, вулканические и лавовые поля.
const (
	SurfaceRegolith        = "реголит"
	SurfaceIceCrust        = "ледяная_кора"
	SurfaceCryovolcanoes   = "криовулканы"
	SurfaceTectonicRifts   = "тектонические_разломы"
	SurfaceGeyserFields    = "гейзерные_поля"
)

// AllSatelliteSurfaceForms — все спутниковые формы поверхности
// (для валидации, перебора, UI).
var AllSatelliteSurfaceForms = []string{
	SurfaceRegolith,
	SurfaceIceCrust,
	SurfaceCryovolcanoes,
	SurfaceTectonicRifts,
	SurfaceGeyserFields,
	SurfaceCraters,
	SurfaceFrozenGases,
	SurfaceVolcanicFields,
	SurfaceLavaFields,
}

// ==================== ТИПЫ НЕДР (17) ====================

const (
	SubterrainEmptyRock        = "пустая_порода"
	SubterrainMagmaticRocks    = "магматические_породы"
	SubterrainMetamorphicRocks = "метаморфические_породы"
	SubterrainSedimentaryRocks = "осадочные_породы"
	SubterrainOreVeins         = "рудные_жилы"
	SubterrainRareEarthVeins   = "редкоземельные_жилы"
	SubterrainRadioactiveZones = "радиоактивные_зоны"
	SubterrainCoalSeams        = "угольные_пласты"
	SubterrainOilPockets       = "нефтяные_карманы"
	SubterrainGasPockets       = "газовые_карманы"
	SubterrainGroundwater      = "подземные_воды"
	SubterrainGroundIce        = "подземные_льды"
	SubterrainMagmaChambers    = "магматические_камеры"
	SubterrainCrystalVeins     = "кристаллические_жилы"
	SubterrainSaltDomes        = "соляные_купола"
	SubterrainCaveSystems      = "пещерные_системы"
	SubterrainMetalCores       = "металлические_ядра"
)

// AllSubterrainTypes — все типы недр
var AllSubterrainTypes = []string{
	SubterrainEmptyRock,
	SubterrainMagmaticRocks,
	SubterrainMetamorphicRocks,
	SubterrainSedimentaryRocks,
	SubterrainOreVeins,
	SubterrainRareEarthVeins,
	SubterrainRadioactiveZones,
	SubterrainCoalSeams,
	SubterrainOilPockets,
	SubterrainGasPockets,
	SubterrainGroundwater,
	SubterrainGroundIce,
	SubterrainMagmaChambers,
	SubterrainCrystalVeins,
	SubterrainSaltDomes,
	SubterrainCaveSystems,
	SubterrainMetalCores,
}