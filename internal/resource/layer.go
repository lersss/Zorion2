// internal/resource/layer.go — универсальный слой (спека 94a §5): каталог
// 20 ресурсов (13 ядерных + 7 мостовых), 10 осей свойств + T_melt/T_boil.
package resource

// Имена 10 осей свойств (09_resources §9.1.2; совпадают с materialAxes
// в internal/races/catalog.go).
const (
	AxisHardness         = "твёрдость"
	AxisElasticity       = "эластичность"
	AxisConductivity     = "проводимость"
	AxisDensity          = "плотность"
	AxisEnergyDensity    = "энергоёмкость"
	AxisBiocompatibility = "биосовместимость"
	AxisRadioactivity    = "радиоактивность"
	AxisToxicity         = "токсичность"
	AxisFlammability     = "горючесть"
	AxisChemicalActivity = "химическая активность"
)

// allAxes — все 10 осей свойств (для перебора, валидации).
var allAxes = []string{
	AxisHardness, AxisElasticity, AxisConductivity, AxisDensity,
	AxisEnergyDensity, AxisBiocompatibility, AxisRadioactivity,
	AxisToxicity, AxisFlammability, AxisChemicalActivity,
}

// Resource — ресурс универсального слоя (спека 94a §5): 10 осей 0–100,
// T_melt/T_boil (K), категория, рабочие имена-заглушки (финальные — @writer).
type Resource struct {
	ID       string   // рабочий id (заглушка)
	Name     string   // рабочее имя-заглушка
	Category string   // одна из 6 категорий (categories.go)
	Closes   []string // consumption-оси, которые ресурс закрывает (§5.1/§5.2)

	// Bridge — мостовой ресурс (§5.2): мульти-потребностный, закрывает
	// вторичные доли нескольких рас. Ядерные (13) — по одному на хемотип.
	Bridge bool

	// 10 осей свойств 0–100 (§5.3).
	Hardness         float64 // твёрдость
	Elasticity       float64 // эластичность
	Conductivity     float64 // проводимость
	Density          float64 // плотность
	EnergyDensity    float64 // энергоёмкость
	Biocompatibility float64 // биосовместимость
	Radioactivity    float64 // радиоактивность
	Toxicity         float64 // токсичность
	Flammability     float64 // горючесть
	ChemicalActivity float64 // химическая активность

	TMelt float64 // температура плавления, K
	TBoil float64 // температура кипения, K

	// Supercritical — сверхкритический ресурс (T_melt/T_boil вокруг
	// критической точки, конвенция фазы §3.1). Не выводится из чисел.
	Supercritical bool
}

// Sublimating — сублимирующий ресурс (T_boil ≤ T_melt, §4 уточнение 1).
// Выводится из чисел, не хранится (один факт — одно место).
func (r *Resource) Sublimating() bool {
	return r.TBoil <= r.TMelt
}

// Value — значение оси свойств по имени (09_resources §9.1.2).
// ok=false — неизвестная ось.
func (r *Resource) Value(axis string) (float64, bool) {
	switch axis {
	case AxisHardness:
		return r.Hardness, true
	case AxisElasticity:
		return r.Elasticity, true
	case AxisConductivity:
		return r.Conductivity, true
	case AxisDensity:
		return r.Density, true
	case AxisEnergyDensity:
		return r.EnergyDensity, true
	case AxisBiocompatibility:
		return r.Biocompatibility, true
	case AxisRadioactivity:
		return r.Radioactivity, true
	case AxisToxicity:
		return r.Toxicity, true
	case AxisFlammability:
		return r.Flammability, true
	case AxisChemicalActivity:
		return r.ChemicalActivity, true
	}
	return 0, false
}

// layerCatalog — каталог универсального слоя (20 ресурсов, спека 94a §5.3).
// Read-only после инициализации (паттерн races.catalog).
var layerCatalog = []*Resource{
	// --- Ядерные (13): по одному на хемотип (§5.1) ---
	{
		ID: "water", Name: "вода-ресурс", Category: CategoryWater, Closes: []string{AxisWater},
		Hardness: 10, Elasticity: 15, Conductivity: 30, Density: 35, EnergyDensity: 8,
		Biocompatibility: 63, Radioactivity: 3, Toxicity: 12, Flammability: 5, ChemicalActivity: 35,
		TMelt: 268, TBoil: 372,
	},
	{
		ID: "ammonia", Name: "аммиак-ресурс", Category: CategoryWater, Closes: []string{AxisAmmonia},
		Hardness: 5, Elasticity: 10, Conductivity: 25, Density: 30, EnergyDensity: 25,
		Biocompatibility: 15, Radioactivity: 5, Toxicity: 62, Flammability: 45, ChemicalActivity: 52,
		TMelt: 198, TBoil: 240,
	},
	{
		ID: "methane", Name: "метан-ресурс", Category: CategoryWater, Closes: []string{AxisMethane},
		Hardness: 2, Elasticity: 5, Conductivity: 10, Density: 15, EnergyDensity: 61,
		Biocompatibility: 8, Radioactivity: 2, Toxicity: 30, Flammability: 63, ChemicalActivity: 40,
		TMelt: 92, TBoil: 112,
	},
	{
		ID: "co2_ice", Name: "CO₂-лёд", Category: CategoryGas, Closes: []string{AxisCO2},
		Hardness: 15, Elasticity: 5, Conductivity: 5, Density: 25, EnergyDensity: 10,
		Biocompatibility: 10, Radioactivity: 3, Toxicity: 35, Flammability: 5, ChemicalActivity: 40,
		TMelt: 196, TBoil: 189, // сублимирующий (T_boil ≤ T_melt, §4 уточнение 1)
	},
	{
		ID: "sulfur", Name: "сера-ресурс", Category: CategoryMineral, Closes: []string{AxisSulfur},
		Hardness: 25, Elasticity: 15, Conductivity: 5, Density: 55, EnergyDensity: 15,
		Biocompatibility: 10, Radioactivity: 5, Toxicity: 65, Flammability: 55, ChemicalActivity: 55,
		TMelt: 398, TBoil: 720,
	},
	{
		ID: "salt_melt", Name: "солевой расплав", Category: CategoryMineral, Closes: []string{AxisSalt},
		Hardness: 30, Elasticity: 10, Conductivity: 40, Density: 60, EnergyDensity: 10,
		Biocompatibility: 15, Radioactivity: 10, Toxicity: 75, Flammability: 5, ChemicalActivity: 45,
		TMelt: 700, TBoil: 1800,
	},
	{
		ID: "supercritical_fluid", Name: "сверхкритический флюид", Category: CategoryGas, Closes: []string{AxisSupercritical},
		Hardness: 3, Elasticity: 5, Conductivity: 20, Density: 15, EnergyDensity: 30,
		Biocompatibility: 12, Radioactivity: 5, Toxicity: 40, Flammability: 30, ChemicalActivity: 55,
		TMelt: 290, TBoil: 310, Supercritical: true, // крит. точка ~304 K
	},
	{
		ID: "silicon", Name: "кремний-ресурс", Category: CategoryMineral, Closes: []string{AxisSilicon},
		Hardness: 63, Elasticity: 20, Conductivity: 30, Density: 55, EnergyDensity: 10,
		Biocompatibility: 5, Radioactivity: 8, Toxicity: 25, Flammability: 5, ChemicalActivity: 35,
		TMelt: 1700, TBoil: 3500,
	},
	{
		ID: "organic", Name: "органика-ресурс", Category: CategoryOrganic, Closes: []string{AxisOrganic},
		Hardness: 15, Elasticity: 55, Conductivity: 10, Density: 25, EnergyDensity: 40,
		Biocompatibility: 65, Radioactivity: 5, Toxicity: 20, Flammability: 45, ChemicalActivity: 30,
		TMelt: 300, TBoil: 500,
	},
	{
		ID: "redox_gradient", Name: "редокс-градиент", Category: CategoryFuel, Closes: []string{AxisRedox},
		Hardness: 10, Elasticity: 10, Conductivity: 35, Density: 40, EnergyDensity: 55,
		Biocompatibility: 10, Radioactivity: 10, Toxicity: 45, Flammability: 50, ChemicalActivity: 63,
		TMelt: 300, TBoil: 500, // без T-окон у ГРД — числа информационные
	},
	{
		ID: "hydrogen_aerosol", Name: "водород-аэрозоль", Category: CategoryGas, Closes: []string{AxisHydrogen},
		Hardness: 2, Elasticity: 5, Conductivity: 15, Density: 8, EnergyDensity: 58,
		Biocompatibility: 5, Radioactivity: 5, Toxicity: 20, Flammability: 62, ChemicalActivity: 30,
		TMelt: 14, TBoil: 24,
	},
	{
		ID: "dust", Name: "пыль", Category: CategoryMineral, Closes: []string{AxisDust},
		Hardness: 20, Elasticity: 5, Conductivity: 10, Density: 22, EnergyDensity: 5,
		Biocompatibility: 15, Radioactivity: 15, Toxicity: 30, Flammability: 5, ChemicalActivity: 20,
		TMelt: 1200, TBoil: 2500, // без T-окон у ПЫЛ — числа информационные
	},
	{
		ID: "radioactive", Name: "радиоактивный материал", Category: CategoryRare, Closes: []string{AxisRadiation},
		Hardness: 35, Elasticity: 10, Conductivity: 25, Density: 50, EnergyDensity: 45,
		Biocompatibility: 8, Radioactivity: 65, Toxicity: 60, Flammability: 10, ChemicalActivity: 40,
		TMelt: 800, TBoil: 1500, // без T-окон у ИЗЛ — числа информационные
	},

	// --- Мостовые (7): мульти-потребностные (§5.2) ---
	{
		ID: "sea_biomass", Name: "морская биомасса", Category: CategoryOrganic, Closes: []string{AxisWater, AxisOrganic},
		Bridge: true,
		Hardness: 10, Elasticity: 50, Conductivity: 12, Density: 25, EnergyDensity: 30,
		Biocompatibility: 62, Radioactivity: 4, Toxicity: 18, Flammability: 30, ChemicalActivity: 25,
		TMelt: 265, TBoil: 400,
	},
	{
		ID: "radio_dust", Name: "радио-пыль", Category: CategoryRare, Closes: []string{AxisRadiation, AxisDust},
		Bridge: true,
		Hardness: 25, Elasticity: 8, Conductivity: 15, Density: 25, EnergyDensity: 20,
		Biocompatibility: 10, Radioactivity: 62, Toxicity: 55, Flammability: 10, ChemicalActivity: 30,
		TMelt: 900, TBoil: 1800,
	},
	{
		ID: "redox_fuel", Name: "редокс-топливо", Category: CategoryFuel, Closes: []string{AxisRedox},
		Bridge: true,
		Hardness: 8, Elasticity: 8, Conductivity: 30, Density: 35, EnergyDensity: 58,
		Biocompatibility: 8, Radioactivity: 8, Toxicity: 40, Flammability: 55, ChemicalActivity: 60,
		TMelt: 250, TBoil: 450,
	},
	{
		ID: "organic_aerosols", Name: "органические аэрозоли", Category: CategoryGas, Closes: []string{AxisOrganic},
		Bridge: true,
		Hardness: 5, Elasticity: 35, Conductivity: 8, Density: 15, EnergyDensity: 25,
		Biocompatibility: 62, Radioactivity: 4, Toxicity: 25, Flammability: 35, ChemicalActivity: 40,
		TMelt: 350, TBoil: 550,
	},
	{
		ID: "sulfur_gradients", Name: "серные градиенты", Category: CategoryMineral, Closes: []string{AxisSulfur, AxisRedox},
		Bridge: true,
		Hardness: 20, Elasticity: 10, Conductivity: 5, Density: 50, EnergyDensity: 55,
		Biocompatibility: 10, Radioactivity: 5, Toxicity: 60, Flammability: 40, ChemicalActivity: 60,
		TMelt: 400, TBoil: 720,
	},
	{
		ID: "thermoredox_melts", Name: "терморедоксные расплавы", Category: CategoryMineral, Closes: []string{AxisSalt, AxisRedox},
		Bridge: true,
		Hardness: 25, Elasticity: 8, Conductivity: 35, Density: 58, EnergyDensity: 55,
		Biocompatibility: 15, Radioactivity: 8, Toxicity: 75, Flammability: 5, ChemicalActivity: 60,
		TMelt: 700, TBoil: 1800,
	},
	{
		ID: "silicon_gradients", Name: "кремниевые градиенты", Category: CategoryMineral, Closes: []string{AxisSilicon, AxisRedox},
		Bridge: true,
		Hardness: 62, Elasticity: 15, Conductivity: 25, Density: 55, EnergyDensity: 55,
		Biocompatibility: 5, Radioactivity: 8, Toxicity: 25, Flammability: 5, ChemicalActivity: 60,
		TMelt: 1700, TBoil: 3500,
	},
}

// LayerCatalog — каталог универсального слоя (read-only).
func LayerCatalog() []*Resource {
	return layerCatalog
}

// LayerByID — ресурс каталога по id (nil, если нет).
func LayerByID(id string) *Resource {
	for _, res := range layerCatalog {
		if res.ID == id {
			return res
		}
	}
	return nil
}