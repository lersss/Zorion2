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
	Consumption map[string]float64 `json:"consumption"`

	Forage Forage `json:"forage"`
	Home   Home   `json:"home"`

	// Bulge — производное выпирание окна (§3.1): "cold" | "hot" | "highP" |
	// "lowP" | "none". Направление, куда раса реально подселится (§7.3).
	Bulge string `json:"bulge"`
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

	// 1. Композиция потребления = 100 (§16 п.1).
	sum := 0.0
	for _, v := range r.Consumption {
		sum += v
	}
	if math.Abs(sum-100) > 0.01 {
		return fmt.Errorf("сумма потребления = %.2f, ожидается 100", sum)
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

	return nil
}