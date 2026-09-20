// internal/resource/categories.go
package resource

// ==================== 6 КАТЕГОРИЙ РЕСУРСОВ ====================

const (
	CategoryMineral = "mineral"
	CategoryOrganic = "organic"
	CategoryRare    = "rare"
	CategoryFuel    = "fuel"
	CategoryWater   = "water"
	CategoryGas     = "gas"
)

// Category — метаданные категории ресурса.
type Category struct {
	Code    string // код (совпадает с ключом в маппингах)
	Name    string // русское название для UI
	Icon    string // эмодзи-иконка
	Purpose string // краткое назначение (для тултипа/GDD)
}

// Categories — все 6 категорий по коду.
var Categories = map[string]Category{
	CategoryMineral: {
		Code:    CategoryMineral,
		Name:    "Минералы",
		Icon:    "🪨",
		Purpose: "Металлы, стекло, инструменты",
	},
	CategoryOrganic: {
		Code:    CategoryOrganic,
		Name:    "Органика",
		Icon:    "🌿",
		Purpose: "Пища, ткани, лекарства",
	},
	CategoryRare: {
		Code:    CategoryRare,
		Name:    "Редкие",
		Icon:    "⭐",
		Purpose: "Детали, электроника, оружие",
	},
	CategoryFuel: {
		Code:    CategoryFuel,
		Name:    "Топливо",
		Icon:    "🔥",
		Purpose: "Энергия для фабрик, транспорт",
	},
	CategoryWater: {
		Code:    CategoryWater,
		Name:    "Вода",
		Icon:    "💧",
		Purpose: "Питьё, орошение, химия",
	},
	CategoryGas: {
		Code:    CategoryGas,
		Name:    "Газы",
		Icon:    "💨",
		Purpose: "Удобрения, химия, топливо",
	},
}

// AllCategories — список кодов всех категорий (для перебора, валидации).
var AllCategories = []string{
	CategoryMineral,
	CategoryOrganic,
	CategoryRare,
	CategoryFuel,
	CategoryWater,
	CategoryGas,
}

// GetCategory — метаданные по коду. Если код неизвестен — пустая структура.
func GetCategory(code string) Category {
	return Categories[code]
}

// ==================== МАППИНГ: ПОВЕРХНОСТЬ → КАТЕГОРИИ ====================

// surfaceToCategories — какие категории ресурсов может дать каждая форма
// поверхности. Используется для генерации ресурсов.
var surfaceToCategories = map[string][]string{
	"горы":              {CategoryMineral, CategoryRare},
	"пески_пустыни":      {CategoryMineral},
	"кратеры":            {CategoryMineral, CategoryRare},
	"стеклянные_поля":    {CategoryRare, CategoryMineral},
	"металлические_поля": {CategoryMineral, CategoryRare},
	"лавовые_поля":       {CategoryMineral, CategoryRare, CategoryFuel},
	"вулканические_поля": {CategoryMineral, CategoryFuel},
	"ледники":            {CategoryWater},
	"мёрзлые_газы":       {CategoryGas, CategoryWater},
	"океаны":             {CategoryWater, CategoryOrganic},
	"озёра_реки":         {CategoryWater, CategoryOrganic},
	"луга_степи":         {CategoryOrganic},
	"леса":               {CategoryOrganic},
	"джунгли":            {CategoryOrganic},
	"болота":             {CategoryOrganic, CategoryFuel},
	"коралловые_рифы":    {CategoryOrganic, CategoryWater},
}

// ==================== МАППИНГ: НЕДРА → КАТЕГОРИИ ====================

// subterrainToCategories — какие категории ресурсов может дать каждый тип недр.
var subterrainToCategories = map[string][]string{
	"пустая_порода":         {},
	"магматические_породы":  {CategoryMineral},
	"метаморфические_породы": {CategoryMineral},
	"осадочные_породы":      {CategoryMineral},
	"рудные_жилы":           {CategoryMineral, CategoryRare},
	"редкоземельные_жилы":   {CategoryRare},
	"радиоактивные_зоны":    {CategoryRare},
	"угольные_пласты":       {CategoryFuel},
	"нефтяные_карманы":      {CategoryFuel},
	"газовые_карманы":       {CategoryGas, CategoryFuel},
	"подземные_воды":        {CategoryWater},
	"подземные_льды":        {CategoryWater},
	"магматические_камеры":  {CategoryMineral, CategoryRare},
	"кристаллические_жилы":  {CategoryRare},
	"соляные_купола":        {CategoryMineral, CategoryWater},
	"пещерные_системы":      {CategoryMineral},
	"металлические_ядра":    {CategoryRare, CategoryMineral},
}

// ==================== ПУБЛИЧНЫЕ ФУНКЦИИ ====================

// CategoriesForSurface — категории, доступные по форме поверхности.
func CategoriesForSurface(form string) []string {
	if cats, ok := surfaceToCategories[form]; ok {
		return cats
	}
	return []string{CategoryMineral}
}

// CategoriesForSubterrain — категории, доступные по типу недр.
func CategoriesForSubterrain(subType string) []string {
	if cats, ok := subterrainToCategories[subType]; ok {
		return cats
	}
	return nil
}

// MergeCategories — объединяет категории из списка источников, убирая дубликаты.
// Пример: [surface=[mineral, rare], subterrain=[fuel, rare]] → [mineral, rare, fuel]
func MergeCategories(sources ...[]string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, src := range sources {
		for _, cat := range src {
			if !seen[cat] {
				seen[cat] = true
				result = append(result, cat)
			}
		}
	}
	return result
}