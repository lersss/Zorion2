// internal/races/catalog.go — карточка расы, загрузка каталога, валидация §16.
package races

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Range — диапазон [Lo, Hi]; Hi == nil — без верхней границы (∞).
// JSON-формат: [lo, hi], hi может быть null (например, heat_flux «≥1» →
// [1, null]; раса 40 Водородные, P «1000+» → [1000, null]).
type Range struct {
	Lo float64
	Hi *float64
}

// UnmarshalJSON — формат [lo, hi]; hi может быть null (∞).
func (r *Range) UnmarshalJSON(b []byte) error {
	var arr []*float64
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	if len(arr) != 2 || arr[0] == nil {
		return fmt.Errorf("диапазон: ожидается [lo, hi], получил %s", b)
	}
	r.Lo = *arr[0]
	r.Hi = arr[1]
	return nil
}

// Contains — входит ли значение в диапазон.
func (r Range) Contains(v float64) bool {
	if v < r.Lo {
		return false
	}
	if r.Hi != nil && v > *r.Hi {
		return false
	}
	return true
}

// SubsetOf — r ⊆ other (для инварианта opt ⊆ surv).
func (r Range) SubsetOf(other Range) bool {
	if r.Lo < other.Lo {
		return false
	}
	if r.Hi == nil {
		return other.Hi == nil
	}
	if other.Hi != nil && *r.Hi > *other.Hi {
		return false
	}
	return true
}

// Window — окно по оси: opt (оптимум) и surv (выживание), §3.1.
type Window struct {
	Opt  Range `json:"opt"`
	Surv Range `json:"surv"`
}

// Atmosphere — требования к составу атмосферы (ключи каскада 99.2.20 §3.6,
// значения в %): need — газ должен быть ≥ min%; poison — газ должен быть ≤ max%.
type Atmosphere struct {
	Need   map[string]float64 `json:"need"`
	Poison map[string]float64 `json:"poison"`
}

// Conditions — физические условия существования расы (§3.1).
// HeatFlux/Gravity — nil = «не влияет» (полный диапазон); LiquidWater —
// nil = не важно, true = требуется, false = исключена.
type Conditions struct {
	Temperature Window     `json:"temperature"`
	Pressure    Window     `json:"pressure"`
	Atmosphere  Atmosphere `json:"atmosphere"`
	Radiation   Window     `json:"radiation"`
	HeatFlux    *Window    `json:"heat_flux,omitempty"`
	Gravity     *Window    `json:"gravity,omitempty"`
	LiquidWater *bool      `json:"liquid_water,omitempty"`
}

// Attributes — числовые атрибуты расы (§3.2): 0–100 (50 = человек),
// Reproduction — множитель × человек (лог-шкала).
type Attributes struct {
	Aggression   float64 `json:"aggression"`
	Curiosity    float64 `json:"curiosity"`
	Reproduction float64 `json:"reproduction"`
	Intelligence float64 `json:"intelligence"`
	Diplomacy    float64 `json:"diplomacy"`
	Resilience   float64 `json:"resilience"`
}

// Forage — каркас подножного корма (§4): source — что ест; works_where —
// где корм работает (для рас — свои окна выживания); basic_resources —
// категории ресурсов универсального слоя (данные при 99.2.5).
type Forage struct {
	Source         string   `json:"source"`
	WorksWhere     string   `json:"works_where"`
	BasicResources []string `json:"basic_resources"`
}

// Home — кластер-дом (§7.1): типы звёзд + планетная ниша.
type Home struct {
	StarClasses []string `json:"star_classes"`
	PlanetNiche string   `json:"planet_niche"`
}

// Robotic — слой потребления роботов (99.2.24 §3.1): энергия + сырьё вместо
// 13 биоосей consumption. Наличие блока отличает робота от био-расы.
type Robotic struct {
	PowerSource []string         `json:"power_source"`
	Materials   RoboticMaterials `json:"materials"`
	Heat        string           `json:"heat"`
	HeatTwist   bool             `json:"heat_twist"`
}

// RoboticMaterials — сырьё роботов на языке 09_resources §9.1: категории
// веществ (6) + оси свойств (10) с направлением окна (high/low). Числовые
// окна и конкретные вещества — при 99.2.5 (basic_resources — каркас).
type RoboticMaterials struct {
	Categories     []string          `json:"categories"`
	Axes           map[string]string `json:"axes"`
	BasicResources []string          `json:"basic_resources"`
}

// Race — карточка расы (§3.1).
type Race struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Basis string `json:"basis"`

	// OffCascade — осознанное исключение §16 п.2: окна выходят за диапазоны
	// каскада намеренно (раса 40 Водородные, P «1000+»; жидкий H₂ требует
	// глубин, вне [0, 1000]). Валидация диапазонов для такой расы пропускается.
	OffCascade bool `json:"off_cascade,omitempty"`

	Conditions Conditions `json:"conditions"`
	Attributes Attributes `json:"attributes"`

	// Consumption — композиция потребления по 13 осям §4 (сумма = 100).
	// Ключи: ВОД АММ МЕТ CO2 СЕР РАС СКФ КРЕ ОРГ ГРД ВГЕ ПЫЛ ИЗЛ.
	// У роботов (Robotic != nil) пуст — их слой robotic (99.2.24 §6 п.2).
	Consumption map[string]float64 `json:"consumption"`

	// Robotic — слой потребления роботов (99.2.24 §3.1). Наличие блока
	// отличает робота: consumption пуст, инвариант «сумма = 100» не
	// применяется (99.2.24 §6 п.2).
	Robotic *Robotic `json:"robotic,omitempty"`

	// Territory — территория расы (99.2.24 §3.2): "adjacency" (дефолт,
	// био-расы, 99.2.21 §7.1) | "conditions" (роботы — по условиям среды,
	// не по соседству; 99.2.24 §6 п.5).
	Territory string `json:"territory,omitempty"`

	// Dormancy — флаг механики-кандидата «выключенные» колонии (99.2.24 §8.2).
	Dormancy bool `json:"dormancy,omitempty"`

	// FamineAggression — флаг механики-кандидата «абсолютная агрессия при
	// сырьевом голоде» (99.2.24 §8.3).
	FamineAggression bool `json:"famine_aggression,omitempty"`

	Forage Forage `json:"forage"`
	Home   Home   `json:"home"`

	// Bulge — производное выпирание окна (§3.1): "cold" | "hot" | "highP" |
	// "lowP" | "none". Направление, куда раса реально подселится (§7.3).
	Bulge string `json:"bulge"`

	// tuning — кэш производного объекта подкрутки (99.2.22 §3.3): вычисляется
	// при загрузке каталога (LoadCatalog), read-only после — блокировка не
	// нужна (паттерн catalog). Прямые конструкции Race — вывод на лету.
	tuning *Tuning
}

// catalog — загруженный каталог (read-only после LoadCatalog; загрузка при
// старте до горутин — блокировка не нужна, паттерн archetypeCache).
var catalog []*Race

// LoadCatalog — загружает каталог рас из файла и валидирует каждую карточку.
// При ошибке текущий каталог не трогается.
func LoadCatalog(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("races catalog: %w", err)
	}
	var file struct {
		Races []*Race `json:"races"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("races catalog %s: %w", absPath, err)
	}
	for i, r := range file.Races {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("races catalog %s: раса %d (%s): %w", absPath, i+1, r.ID, err)
		}
		// Кэш производного объекта подкрутки (99.2.22 §3.3): вычисляется при
		// загрузке каталога (до горутин), read-only после.
		r.tuning = deriveTuning(r)
	}
	catalog = file.Races
	return nil
}

// Catalog — текущий каталог рас (read-only; пуст, если не загружен).
func Catalog() []*Race {
	return catalog
}

// ByID — раса по ключу (nil, если нет в каталоге).
func ByID(id string) *Race {
	for _, r := range catalog {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// cascadeRanges — диапазоны каскада 99.2.20 для валидации surv-окон (§16 п.2):
// T [20, 2500] K, P [0, 1000] атм, rad [0, 100]. heat_flux/gravity не
// проверяются (в §16 не перечислены).
var cascadeRanges = map[string][2]float64{
	"temperature": {20, 2500},
	"pressure":    {0, 1000},
	"radiation":   {0, 100},
}

// windows — все окна карточки по имени оси (для валидации).
func (r *Race) windows() map[string]Window {
	w := map[string]Window{
		"temperature": r.Conditions.Temperature,
		"pressure":    r.Conditions.Pressure,
		"radiation":   r.Conditions.Radiation,
	}
	if r.Conditions.HeatFlux != nil {
		w["heat_flux"] = *r.Conditions.HeatFlux
	}
	if r.Conditions.Gravity != nil {
		w["gravity"] = *r.Conditions.Gravity
	}
	return w
}

// Validate — инварианты §16: (1) сумма потребления = 100; (2) opt ⊆ surv по
// каждой оси; (3) surv в пределах диапазонов каскада (кроме OffCascade).
// Плюс базовые проверки карточки (id/name, атрибуты, bulge).
func (r *Race) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("id пуст")
	}
	if r.Name == "" {
		return fmt.Errorf("name пуст")
	}

	// 1. Композиция потребления = 100 (§16 п.1). Роботы (наличие robotic)
	// исключаются: их слой — robotic (энергия + сырьё), 13 биоосей пусты
	// (99.2.24 §6 п.2).
	if r.Robotic == nil {
		sum := 0.0
		for _, v := range r.Consumption {
			sum += v
		}
		if math.Abs(sum-100) > 0.01 {
			return fmt.Errorf("сумма потребления = %.2f, ожидается 100", sum)
		}
	}

	// 2. opt ⊆ surv по каждой оси (§16 п.2).
	for name, w := range r.windows() {
		if !w.Opt.SubsetOf(w.Surv) {
			return fmt.Errorf("opt ⊄ surv по оси %s (opt [%v, %v], surv [%v, %v])",
				name, w.Opt.Lo, w.Opt.Hi, w.Surv.Lo, w.Surv.Hi)
		}
	}

	// 3. surv в пределах диапазонов каскада (§16 п.2); OffCascade — исключение.
	if !r.OffCascade {
		for name, w := range r.windows() {
			rng, ok := cascadeRanges[name]
			if !ok {
				continue
			}
			if w.Surv.Lo < rng[0] || (w.Surv.Hi != nil && *w.Surv.Hi > rng[1]) {
				return fmt.Errorf("surv по оси %s вне диапазона каскада [%v, %v] (surv [%v, %v])",
					name, rng[0], rng[1], w.Surv.Lo, w.Surv.Hi)
			}
		}
	}

	// Атрибуты: 0–100 (кроме reproduction — множитель > 0).
	attrs := []struct {
		name  string
		value float64
	}{
		{"aggression", r.Attributes.Aggression},
		{"curiosity", r.Attributes.Curiosity},
		{"intelligence", r.Attributes.Intelligence},
		{"diplomacy", r.Attributes.Diplomacy},
		{"resilience", r.Attributes.Resilience},
	}
	for _, a := range attrs {
		if a.value < 0 || a.value > 100 {
			return fmt.Errorf("атрибут %s = %v вне [0, 100]", a.name, a.value)
		}
	}
	if r.Attributes.Reproduction <= 0 {
		return fmt.Errorf("reproduction = %v, ожидается множитель > 0", r.Attributes.Reproduction)
	}

	// Bulge — из допустимого набора.
	switch r.Bulge {
	case "cold", "hot", "highP", "lowP", "none":
	default:
		return fmt.Errorf("bulge = %q, ожидается cold|hot|highP|lowP|none", r.Bulge)
	}

	// Territory — из допустимого набора (99.2.24 §3.2); пусто = дефолт
	// "adjacency" (био-расы, 99.2.21 §7.1).
	switch r.Territory {
	case "", "adjacency", "conditions":
	default:
		return fmt.Errorf("territory = %q, ожидается adjacency|conditions", r.Territory)
	}

	// Слой robotic (99.2.24 §6 п.3): power_source непуст (enum), heat из enum,
	// categories ⊆ 6 категорий, axes ⊆ 10 осей (high|low). Роботы: consumption
	// пуст (13 биоосей — не их слой, §6 п.2), territory = "conditions" (§6 п.5).
	if r.Robotic != nil {
		if err := r.Robotic.Validate(); err != nil {
			return err
		}
		if len(r.Consumption) > 0 {
			return fmt.Errorf("робот: consumption должен быть пуст (слой robotic вместо 13 биоосей)")
		}
		if r.Territory != "conditions" {
			return fmt.Errorf("робот: territory = %q, ожидается \"conditions\"", r.Territory)
		}
	}

	return nil
}

// powerSources — enum источников энергии роботов (99.2.24 §3.1).
var powerSources = map[string]bool{
	"солнце": true, "геотерма": true, "реактор": true, "термопары": true, "тепло": true,
}

// heatModes — enum способов сброса тепла (99.2.24 §3.1).
var heatModes = map[string]bool{
	"радиаторы": true, "горячая_работа": true, "лёд": true, "среда": true,
}

// materialCategories — 6 категорий веществ (09_resources §9.1.3).
var materialCategories = map[string]bool{
	"🪨": true, "🌿": true, "⭐": true, "🔥": true, "💧": true, "💨": true,
}

// materialAxes — 10 осей свойств (09_resources §9.1.2).
var materialAxes = map[string]bool{
	"твёрдость": true, "эластичность": true, "проводимость": true, "плотность": true,
	"энергоёмкость": true, "биосовместимость": true, "радиоактивность": true,
	"токсичность": true, "горючесть": true, "химическая активность": true,
}

// Validate — инварианты слоя robotic (99.2.24 §6 п.3): power_source непуст
// (значения из enum), heat из enum, categories ⊆ 6 категорий, axes ⊆ 10 осей
// (направление окна high|low).
func (rb *Robotic) Validate() error {
	if len(rb.PowerSource) == 0 {
		return fmt.Errorf("robotic.power_source пуст")
	}
	for _, ps := range rb.PowerSource {
		if !powerSources[ps] {
			return fmt.Errorf("robotic.power_source = %q, ожидается солнце|геотерма|реактор|термопары|тепло", ps)
		}
	}
	if !heatModes[rb.Heat] {
		return fmt.Errorf("robotic.heat = %q, ожидается радиаторы|горячая_работа|лёд|среда", rb.Heat)
	}
	for _, c := range rb.Materials.Categories {
		if !materialCategories[c] {
			return fmt.Errorf("robotic.materials.categories = %q, ожидается одна из 6 категорий 09_resources §9.1.3", c)
		}
	}
	for axis, dir := range rb.Materials.Axes {
		if !materialAxes[axis] {
			return fmt.Errorf("robotic.materials.axes = %q, ожидается одна из 10 осей 09_resources §9.1.2", axis)
		}
		if dir != "high" && dir != "low" {
			return fmt.Errorf("robotic.materials.axes[%s] = %q, ожидается high|low", axis, dir)
		}
	}
	return nil
}