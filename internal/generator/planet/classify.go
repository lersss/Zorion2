// internal/generator/planet/classify.go
//
// Классификация типов планет (99.2.28 §11): тип — производный ярлык,
// вычисляемый правилами справочника типов (секция planet_types файла
// biome_catalog.json). Код — интерпретатор без чисел: порядок правил сверху
// вниз, первое сработавшее → тип; фолбэк — fallback_type справочника.
// Газовый гигант — отдельный флаг вне правил (как раньше: гигант →
// радиоактивная → остальные). Пороги 15/20/25 — данные, не код.
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
// Правила справочника покрывают 11 типов; «мёртвая» — ветка экзотики.
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
// правилами справочника типов (99.2.28 §11): порядок сверху вниз, первое
// сработавшее → тип; фолбэк — fallback_type справочника.
func ClassifyGameDesignType(in PlanetClassificationInput) string {
	// 1. Газовый гигант — отдельный флаг вне правил (как в старом classify.go).
	if in.IsGasGiant {
		return TypeGasGiant
	}

	cat := GetBiomeCatalog()
	for _, rule := range cat.PlanetTypes {
		if evalPredicates(rule.Predicates, in, cat) {
			return rule.ID
		}
	}
	if cat.FallbackType != "" {
		return cat.FallbackType
	}
	return TypeRocky
}

// ==================== ИНТЕРПРЕТАТОР ПРЕДИКАТОВ ====================

// evalPredicates — все предикаты правила должны выполниться (AND).
func evalPredicates(preds []Predicate, in PlanetClassificationInput, cat *BiomeCatalog) bool {
	for _, p := range preds {
		if !evalPredicate(p, in, cat) {
			return false
		}
	}
	return true
}

// evalPredicate — один предикат (приложение §3).
func evalPredicate(p Predicate, in PlanetClassificationInput, cat *BiomeCatalog) bool {
	switch p.Type {
	case "settleable":
		return in.Settleable
	case "life":
		return in.Life
	case "radioactive_core":
		return in.IsRadioactive
	case "is_gas_giant":
		return in.IsGasGiant
	case "dominant_form":
		return in.Surface.DominantForm() == p.Form
	case "share_of":
		return in.Surface.ShareOf(p.Form) >= p.Min
	case "tag_sum":
		return tagSum(in.Surface, p.Tag, cat) >= p.Min
	case "temperature":
		if p.Lt > 0 && in.Temperature >= p.Lt {
			return false
		}
		if p.Gt > 0 && in.Temperature <= p.Gt {
			return false
		}
		return true
	case "water_percent":
		if p.Lt > 0 && in.WaterPercent >= p.Lt {
			return false
		}
		if p.Gt > 0 && in.WaterPercent <= p.Gt {
			return false
		}
		return true
	case "any_of":
		for _, form := range p.Forms {
			if in.Surface.ShareOf(form) >= p.Min {
				return true
			}
		}
		return false
	case "and":
		return evalPredicates(p.Rules, in, cat)
	case "or":
		for _, r := range p.Rules {
			if evalPredicate(r, in, cat) {
				return true
			}
		}
		return false
	}
	return false
}

// tagSum — сумма долей биомов с тегом типа (теги — из справочника).
func tagSum(surface Composition, tag string, cat *BiomeCatalog) float64 {
	total := 0.0
	for form, share := range surface {
		b := cat.BiomeByID(form)
		if b != nil && hasTag(b.TypeTags, tag) {
			total += share
		}
	}
	return total
}

// hasTag — есть ли тег в списке тегов биома.
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ==================== ХЕЛПЕРЫ ====================

// isBiosphereForm — относится ли форма к биосферным (жизнь).
// Используется Composition.BiosphereSum (легаси-хелпер композиции).
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