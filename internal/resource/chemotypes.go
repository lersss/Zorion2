// internal/resource/chemotypes.go — шаблоны хемотипов (спека 94a §3.1):
// канонический профиль ресурса-«еды» каждой consumption-оси.
package resource

// Phase — пригодная фаза пищи хемотипа (§3.3, «нужная фаза»).
type Phase string

const (
	PhaseLiquid        Phase = "liquid"        // жидкая (растворители)
	PhaseSolid         Phase = "solid"         // твёрдая (CO₂-лёд, кремний)
	PhaseSupercritical Phase = "supercritical" // сверхкритическая (T > T_boil)
	PhaseAny           Phase = "any"           // любая (ГРД/ПЫЛ/ИЗЛ — без T-окон)
)

// Interval — включительный интервал [Lo, Hi] (границы входят, §8.1 инвариант 10).
type Interval struct {
	Lo float64
	Hi float64
}

// Contains — входит ли значение в интервал (включительно).
func (iv Interval) Contains(v float64) bool {
	return v >= iv.Lo && v <= iv.Hi
}

// AxisWindow — окно по оси свойств.
type AxisWindow struct {
	Axis string // имя оси (Axis*)
	Lo   float64
	Hi   float64
}

// ChemotypeTemplate — шаблон хемотипа (§3.1): окна T_melt/T_boil, окна по
// осям свойств, уместные категории, пригодная фаза пищи.
type ChemotypeTemplate struct {
	Axis       string       // consumption-ось: ВОД, АММ, ...
	TMelt      *Interval    // окно T_melt, nil — нет окна (ГРД/ПЫЛ/ИЗЛ)
	TBoil      *Interval    // окно T_boil, nil — нет окна
	Phase      Phase        // пригодная фаза пищи (§3.3)
	Windows    []AxisWindow // окна по осям свойств
	Categories []string     // уместные категории (§3.1)

	// Sublimating — сублимирующий шаблон (CO2): T-окна перекрываются
	// (T_boil [185,195] vs T_melt [190,200]), признак из чисел не выводится.
	Sublimating bool
}

// 13 consumption-осей (config/races.json, спека 94a §3.1).
const (
	AxisWater         = "ВОД"
	AxisAmmonia       = "АММ"
	AxisMethane       = "МЕТ"
	AxisCO2           = "CO2"
	AxisSulfur        = "СЕР"
	AxisSalt          = "РАС"
	AxisSupercritical = "СКФ"
	AxisSilicon       = "КРЕ"
	AxisOrganic       = "ОРГ"
	AxisRedox         = "ГРД"
	AxisHydrogen      = "ВГЕ"
	AxisDust          = "ПЫЛ"
	AxisRadiation     = "ИЗЛ"
)

// consumptionAxes — все 13 consumption-осей (для перебора, валидации).
var consumptionAxes = []string{
	AxisWater, AxisAmmonia, AxisMethane, AxisCO2, AxisSulfur, AxisSalt,
	AxisSupercritical, AxisSilicon, AxisOrganic, AxisRedox, AxisHydrogen,
	AxisDust, AxisRadiation,
}

// ConsumptionAxes — все 13 consumption-осей в каноническом порядке (read-only).
func ConsumptionAxes() []string {
	return consumptionAxes
}

// layerTemplates — шаблоны хемотипов (13, спека 94a §3.1). Read-only.
var layerTemplates = map[string]*ChemotypeTemplate{
	AxisWater: {
		Axis:  AxisWater,
		TMelt: &Interval{Lo: 250, Hi: 280},
		TBoil: &Interval{Lo: 350, Hi: 400},
		Phase: PhaseLiquid,
		Windows: []AxisWindow{
			{Axis: AxisBiocompatibility, Lo: 60, Hi: 100},
			{Axis: AxisToxicity, Lo: 0, Hi: 30},
		},
		Categories: []string{CategoryWater},
	},
	AxisAmmonia: {
		Axis:  AxisAmmonia,
		TMelt: &Interval{Lo: 185, Hi: 210},
		TBoil: &Interval{Lo: 230, Hi: 250},
		Phase: PhaseLiquid,
		Windows: []AxisWindow{
			{Axis: AxisChemicalActivity, Lo: 30, Hi: 60},
			{Axis: AxisToxicity, Lo: 40, Hi: 80},
		},
		Categories: []string{CategoryWater},
	},
	AxisMethane: {
		Axis:  AxisMethane,
		TMelt: &Interval{Lo: 85, Hi: 100},
		TBoil: &Interval{Lo: 105, Hi: 120},
		Phase: PhaseLiquid,
		Windows: []AxisWindow{
			{Axis: AxisFlammability, Lo: 60, Hi: 100},
			{Axis: AxisEnergyDensity, Lo: 60, Hi: 100},
		},
		Categories: []string{CategoryWater},
	},
	AxisCO2: {
		Axis:        AxisCO2,
		TMelt:       &Interval{Lo: 190, Hi: 200},
		TBoil:       &Interval{Lo: 185, Hi: 195}, // сублимирующий (T_boil-окно ниже T_melt-окна)
		Phase:       PhaseSolid,
		Sublimating: true,
		Windows: []AxisWindow{
			{Axis: AxisChemicalActivity, Lo: 20, Hi: 50},
		},
		Categories: []string{CategoryGas},
	},
	AxisSulfur: {
		Axis:  AxisSulfur,
		TMelt: &Interval{Lo: 380, Hi: 420},
		TBoil: &Interval{Lo: 700, Hi: 750},
		Phase: PhaseLiquid,
		Windows: []AxisWindow{
			{Axis: AxisChemicalActivity, Lo: 30, Hi: 60},
			{Axis: AxisToxicity, Lo: 40, Hi: 80},
		},
		Categories: []string{CategoryMineral},
	},
	AxisSalt: {
		Axis:  AxisSalt,
		TMelt: &Interval{Lo: 500, Hi: 900},
		TBoil: &Interval{Lo: 1500, Hi: 2500},
		Phase: PhaseLiquid,
		Windows: []AxisWindow{
			{Axis: AxisToxicity, Lo: 50, Hi: 90},
			{Axis: AxisBiocompatibility, Lo: 0, Hi: 30},
		},
		Categories: []string{CategoryMineral},
	},
	AxisSupercritical: {
		Axis:  AxisSupercritical,
		TMelt: &Interval{Lo: 280, Hi: 295},
		TBoil: &Interval{Lo: 305, Hi: 320}, // крит. точка ~304 K
		Phase: PhaseSupercritical,
		Windows: []AxisWindow{
			{Axis: AxisDensity, Lo: 0, Hi: 20},
			{Axis: AxisChemicalActivity, Lo: 30, Hi: 60},
		},
		Categories: []string{CategoryGas},
	},
	AxisSilicon: {
		Axis:  AxisSilicon,
		TMelt: &Interval{Lo: 1600, Hi: 1800},
		TBoil: &Interval{Lo: 3000, Hi: 4000},
		Phase: PhaseSolid,
		Windows: []AxisWindow{
			{Axis: AxisHardness, Lo: 60, Hi: 100},
			{Axis: AxisDensity, Lo: 40, Hi: 70},
		},
		Categories: []string{CategoryMineral},
	},
	AxisOrganic: {
		Axis:  AxisOrganic,
		TMelt: &Interval{Lo: 250, Hi: 350},
		TBoil: &Interval{Lo: 400, Hi: 600},
		Phase: PhaseLiquid,
		Windows: []AxisWindow{
			{Axis: AxisBiocompatibility, Lo: 60, Hi: 100},
			{Axis: AxisElasticity, Lo: 30, Hi: 70},
		},
		Categories: []string{CategoryOrganic},
	},
	AxisRedox: {
		Axis:  AxisRedox,
		Phase: PhaseAny, // без T-окон: пища в любой фазе в окне обитания (§3.1)
		Windows: []AxisWindow{
			{Axis: AxisChemicalActivity, Lo: 60, Hi: 100},
			{Axis: AxisEnergyDensity, Lo: 40, Hi: 80},
		},
		Categories: []string{CategoryFuel},
	},
	AxisHydrogen: {
		Axis:  AxisHydrogen,
		TMelt: &Interval{Lo: 10, Hi: 15},
		TBoil: &Interval{Lo: 20, Hi: 30},
		Phase: PhaseLiquid,
		Windows: []AxisWindow{
			{Axis: AxisDensity, Lo: 0, Hi: 10},
			{Axis: AxisFlammability, Lo: 55, Hi: 75}, // занижено намеренно (§3.1 примечания)
			{Axis: AxisEnergyDensity, Lo: 55, Hi: 75},
		},
		Categories: []string{CategoryGas},
	},
	AxisDust: {
		Axis:  AxisDust,
		Phase: PhaseAny, // без T-окон: субстрат (§3.1)
		Windows: []AxisWindow{
			{Axis: AxisHardness, Lo: 0, Hi: 30},
			{Axis: AxisDensity, Lo: 0, Hi: 30},
			{Axis: AxisBiocompatibility, Lo: 0, Hi: 40},
		},
		Categories: []string{CategoryMineral},
	},
	AxisRadiation: {
		Axis:  AxisRadiation,
		Phase: PhaseAny, // без T-окон: излучение (§3.1)
		Windows: []AxisWindow{
			{Axis: AxisRadioactivity, Lo: 60, Hi: 100},
			{Axis: AxisToxicity, Lo: 40, Hi: 80},
		},
		Categories: []string{CategoryRare},
	},
}

// LayerTemplates — шаблоны хемотипов по consumption-оси (read-only).
func LayerTemplates() map[string]*ChemotypeTemplate {
	return layerTemplates
}