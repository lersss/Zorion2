// internal/generator/planet/classify.go
package planet

// ==================== ГЕЙМДИЗАЙНЕРСКИЕ ТИПЫ ====================

const (
	TypeGasGiant    = "газовый гигант"
	TypeRadioactive = "радиоактивная"
	TypeEarthlike   = "землеподобная"
	TypeOceanic     = "океаническая"
	TypeIce         = "ледяная"
	TypeVolcanic    = "вулканическая"
	TypeDesert      = "пустынная"
	TypeGlass       = "стеклянная"
	TypeMetal       = "металлическая"
	TypeOrganic     = "органик"
	TypeRocky       = "скалистая (базовая)"
	TypeDead        = "мёртвая" // планеты экзотических объектов (99.2.4 §5.3)
)

// AllGameDesignTypes — все возможные типы (для UI, фильтров, статистики).
var AllGameDesignTypes = []string{
	TypeEarthlike,
	TypeOceanic,
	TypeIce,
	TypeVolcanic,
	TypeDesert,
	TypeGlass,
	TypeMetal,
	TypeOrganic,
	TypeRocky,
	TypeRadioactive,
	TypeGasGiant,
	TypeDead,
}

// ==================== ПОРОГИ ====================

const (
	// Минимальная доля формы, при которой она считается «присутствующей»
	// для классификации.
	presenceThreshold = 15.0

	// Порог для вулканической: сумма лавы + вулканических полей.
	volcanicThreshold = 20.0

	// Порог для пустынной: минимальная доля песков + максимум воды.
	desertSandsThreshold = 25.0
	desertWaterMax       = 25.0

	// Порог температуры для ледяной: если доминируют ледники
	// и температура ниже этой отметки — ледяная.
	iceTempMax = 250.0

	// Порог для органик: сумма биосферных форм (луга, леса, джунгли, болота, рифы).
	biosphereSumThreshold = 25.0
)

// PlanetClassificationInput — входные данные для классификации.
type PlanetClassificationInput struct {
	IsGasGiant    bool
	IsRadioactive bool
	Surface       Composition
	Temperature   float64
	WaterPercent  float64
	Settleable    bool
	Life          bool
}

// ClassifyGameDesignType — определяет геймдизайнерский тип планеты
// на основе её свойств и композиции поверхности.
//
// Порядок проверок: от самых специфичных к общим. Fallback — скалистая.
func ClassifyGameDesignType(in PlanetClassificationInput) string {
	// 1. Газовый гигант
	if in.IsGasGiant {
		return TypeGasGiant
	}

	// 2. Радиоактивная
	if in.IsRadioactive {
		return TypeRadioactive
	}

	// 3. Землеподобная: пригодна под поселение + жизнь + горы + вода
	if in.Settleable && in.Life &&
		in.Surface.Has(SurfaceRocks) &&
		(in.Surface.Has(SurfaceOceans) || in.Surface.Has(SurfaceLakes)) {
		return TypeEarthlike
	}

	// 4. Океаническая: доминируют океаны, много воды
	if in.Surface.DominantForm() == SurfaceOceans && in.WaterPercent > 60 {
		return TypeOceanic
	}

	// 5. Ледяная: доминируют ледники И холодно
	//    (раньше было "ИЛИ T < 200" — отсюда перекос)
	if in.Surface.DominantForm() == SurfaceGlaciers && in.Temperature < iceTempMax {
		return TypeIce
	}
	if in.Temperature < 150 {
		return TypeIce
	}

	// 6. Вулканическая: заметная доля лавы или вулканических полей
	if in.Surface.ShareOf(SurfaceLavaFields)+in.Surface.ShareOf(SurfaceVolcanicFields) >= volcanicThreshold {
		return TypeVolcanic
	}

	// 7. Пустынная: много песков и мало воды
	if in.Surface.ShareOf(SurfaceSands) >= desertSandsThreshold &&
		in.WaterPercent < desertWaterMax {
		return TypeDesert
	}

	// 8. Стеклянная
	if in.Surface.ShareOf(SurfaceGlassFields) >= presenceThreshold {
		return TypeGlass
	}

	// 9. Металлическая
	if in.Surface.ShareOf(SurfaceMetalFields) >= presenceThreshold {
		return TypeMetal
	}

	// 10. Органик — сумма биосферных форм >= 25% (луга, леса, джунгли, болота, рифы)
	if in.Surface.BiosphereSum() >= biosphereSumThreshold {
		return TypeOrganic
	}

	// 11. Fallback
	return TypeRocky
}

// ==================== ХЕЛПЕРЫ ====================

// isBiosphereForm — относится ли форма к биосферным (жизнь).
func isBiosphereForm(form string) bool {
	switch form {
	case SurfaceMeadows,
		SurfaceForests,
		SurfaceJungles,
		SurfaceSwamps,
		SurfaceCoralReefs:
		return true
	}
	return false
}

// GameDesignTypeName — читаемое название типа (для UI).
func GameDesignTypeName(code string) string {
	return code
}