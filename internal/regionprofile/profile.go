// internal/regionprofile — профили регионов (59a, спека 99.2.10).
//
// Профиль региона — постоянная особенность региона: класс (квазарный,
// токсичный, пиратский...) с мягким влиянием на генерацию звёзд и планет.
// Каталог классов — config/region_profiles/ (один JSON на класс, список
// открыт). Пакет нейтрален: его импортируют и galaxy (генерация звёзд),
// и planet (генерация планет), и handlers (привязка планет к региону).
package regionprofile

// Intensity — интенсивность профиля (0/1/2: слабая/средняя/сильная).
type Intensity int

const (
	Weak Intensity = iota
	Medium
	Strong
)

// intensityScale — масштаб эффективного сдвига (спека §7): конфиг задаёт
// сдвиги для сильной интенсивности; слабая ×0.3, средняя ×0.6, сильная ×1.0.
func intensityScale(i Intensity) float64 {
	switch i {
	case Weak:
		return 0.3
	case Medium:
		return 0.6
	default:
		return 1.0
	}
}

// multEff — эффективный мультипликативный сдвиг: eff = 1 + (config − 1) × scale
// (интерполяция от нейтрального 1.0).
func multEff(config float64, i Intensity) float64 {
	return 1 + (config-1)*intensityScale(i)
}

// addEff — эффективный аддитивный сдвиг: eff = config × scale.
func addEff(config float64, i Intensity) float64 {
	return config * intensityScale(i)
}

// StarMods — сдвиги генерации звёзд (спека §7). Неупомянутые ключи = 1.0.
// VariableMult — указатель: nil = не задано (нейтрально 1.0), явный 0/отрицательный
// отклоняется валидацией (мягкость §11.1).
type StarMods struct {
	SpectralMult     map[string]float64 `json:"spectral_mult"`
	SystemTypeMult   map[string]float64 `json:"system_type_mult"`
	MassBias         float64            `json:"mass_bias"`
	MetallicityShift float64            `json:"metallicity_shift"`
	VariableMult     *float64           `json:"variable_mult"`
}

// PlanetMods — сдвиги генерации планет (спека §8). Каскад 99.2.20 не
// модифицируется — сдвигаются только входы и веса композиционного шаблона.
// PlanetCountMult — указатель: nil = не задано (нейтрально 1.0), явный
// 0/отрицательный отклоняется валидацией (мягкость §11.1).
type PlanetMods struct {
	PlanetCountMult *float64           `json:"planet_count_mult"`
	GasGiantShift   float64            `json:"gas_giant_shift"`
	SurfaceBias     map[string]float64 `json:"surface_bias"`
	SubterrainBias  map[string]float64 `json:"subterrain_bias"`
	ResourceBias    map[string]float64 `json:"resource_bias"`
}

// Profile — класс профиля региона (спека §9).
type Profile struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Weight float64    `json:"weight"`
	Cause  string     `json:"cause"`
	Crises []string   `json:"crises"`
	Boons  []string   `json:"boons"`
	Star   StarMods   `json:"star"`
	Planet PlanetMods `json:"planet"`
}

// ==================== ЭФФЕКТИВНЫЕ СДВИГИ (с учётом интенсивности) ====================

// SpectralMult — эффективные множители спектральных классов.
func (p *Profile) SpectralMult(i Intensity) map[string]float64 {
	return scaleMultMap(p.Star.SpectralMult, i)
}

// SystemTypeMult — эффективные множители типов систем.
func (p *Profile) SystemTypeMult(i Intensity) map[string]float64 {
	return scaleMultMap(p.Star.SystemTypeMult, i)
}

// MassBias — эффективный сдвиг массы (аддитив).
func (p *Profile) MassBias(i Intensity) float64 {
	return addEff(p.Star.MassBias, i)
}

// MetallicityShift — эффективный сдвиг металличности (аддитив).
func (p *Profile) MetallicityShift(i Intensity) float64 {
	return addEff(p.Star.MetallicityShift, i)
}

// VariableMult — эффективный множитель вероятностей переменности.
// nil (не задано) — нейтрально 1.0.
func (p *Profile) VariableMult(i Intensity) float64 {
	if p.Star.VariableMult == nil {
		return 1.0
	}
	return multEff(*p.Star.VariableMult, i)
}

// PlanetCountMult — эффективный множитель среднего числа планет.
// nil (не задано) — нейтрально 1.0.
func (p *Profile) PlanetCountMult(i Intensity) float64 {
	if p.Planet.PlanetCountMult == nil {
		return 1.0
	}
	return multEff(*p.Planet.PlanetCountMult, i)
}

// GasGiantShift — эффективный сдвиг доли газовых гигантов (аддитив).
func (p *Profile) GasGiantShift(i Intensity) float64 {
	return addEff(p.Planet.GasGiantShift, i)
}

// SurfaceBias — эффективные множители весов форм поверхности.
func (p *Profile) SurfaceBias(i Intensity) map[string]float64 {
	return scaleMultMap(p.Planet.SurfaceBias, i)
}

// SubterrainBias — эффективные множители весов форм недр.
func (p *Profile) SubterrainBias(i Intensity) map[string]float64 {
	return scaleMultMap(p.Planet.SubterrainBias, i)
}

// ResourceBias — эффективные множители весов категорий ресурсов.
func (p *Profile) ResourceBias(i Intensity) map[string]float64 {
	return scaleMultMap(p.Planet.ResourceBias, i)
}

// scaleMultMap — эффективные множители по ключам: eff = 1 + (config − 1) × scale.
func scaleMultMap(config map[string]float64, i Intensity) map[string]float64 {
	if len(config) == 0 {
		return nil
	}
	out := make(map[string]float64, len(config))
	for k, v := range config {
		out[k] = multEff(v, i)
	}
	return out
}

// ApplyMultMap — умножает веса base на эффективные множители mult (по ключу).
// Ключи вне mult — без изменений (нейтрально 1.0); mult == nil — base как есть.
// Используется для сдвига весов звёзд (Weights.Spectral/SystemTypes) и весов
// полосы композиции (surface_bias/subterrain_bias: форма вне шаблона полосы —
// no-op, спека §8 O4).
func ApplyMultMap(base, mult map[string]float64, i Intensity) map[string]float64 {
	if len(mult) == 0 {
		return base
	}
	out := make(map[string]float64, len(base))
	for k, v := range base {
		m := 1.0
		if mv, ok := mult[k]; ok {
			m = multEff(mv, i)
		}
		out[k] = v * m
	}
	return out
}

// ==================== ВАЛИДАЦИЯ ====================

// Validate — инвариант мягкости (спека §11.1): все множители весов строго
// > 0, иначе тип может исчезнуть (weightedPick нормирует по сумме).
// Диапазоны аддитивных ручек — спека §7 (mass_bias/metallicity_shift ±0.3)
// и §8 (gas_giant_shift ±0.2).
func (p *Profile) Validate() error {
	if p.ID == "" {
		return errInvalid("id пуст")
	}
	if err := validateMultMap("spectral_mult", p.Star.SpectralMult); err != nil {
		return err
	}
	if err := validateMultMap("system_type_mult", p.Star.SystemTypeMult); err != nil {
		return err
	}
	if p.Star.VariableMult != nil && *p.Star.VariableMult <= 0 {
		return errInvalid("variable_mult: %v <= 0 — тип может исчезнуть (нарушение мягкости)", *p.Star.VariableMult)
	}
	if p.Star.MassBias < -0.3 || p.Star.MassBias > 0.3 {
		return errInvalid("mass_bias: %v вне [−0.3, +0.3] (спека §7)", p.Star.MassBias)
	}
	if p.Star.MetallicityShift < -0.3 || p.Star.MetallicityShift > 0.3 {
		return errInvalid("metallicity_shift: %v вне [−0.3, +0.3] (спека §7)", p.Star.MetallicityShift)
	}
	if p.Planet.PlanetCountMult != nil && *p.Planet.PlanetCountMult <= 0 {
		return errInvalid("planet_count_mult: %v <= 0 — тип может исчезнуть (нарушение мягкости)", *p.Planet.PlanetCountMult)
	}
	if p.Planet.GasGiantShift < -0.2 || p.Planet.GasGiantShift > 0.2 {
		return errInvalid("gas_giant_shift: %v вне [−0.2, +0.2] (спека §8)", p.Planet.GasGiantShift)
	}
	if err := validateMultMap("surface_bias", p.Planet.SurfaceBias); err != nil {
		return err
	}
	if err := validateMultMap("subterrain_bias", p.Planet.SubterrainBias); err != nil {
		return err
	}
	if err := validateMultMap("resource_bias", p.Planet.ResourceBias); err != nil {
		return err
	}
	return nil
}

// ValidateForms — имена форм поверхности/недр ТОЛЬКО из точных констант
// composition_forms.go (спека §9): неизвестное имя валидация отклоняет.
func (p *Profile) ValidateForms(surfaceForms, subterrainForms []string) error {
	if err := validateKeys("surface_bias", p.Planet.SurfaceBias, surfaceForms); err != nil {
		return err
	}
	return validateKeys("subterrain_bias", p.Planet.SubterrainBias, subterrainForms)
}

func validateMultMap(name string, m map[string]float64) error {
	for k, v := range m {
		if v <= 0 {
			return errInvalid("%s[%q]: %v <= 0 — тип может исчезнуть (нарушение мягкости)", name, k, v)
		}
	}
	return nil
}

func validateKeys(name string, m map[string]float64, valid []string) error {
	if len(valid) == 0 {
		return nil // списки форм не переданы — валидация имён пропущена
	}
	validSet := make(map[string]bool, len(valid))
	for _, v := range valid {
		validSet[v] = true
	}
	for k := range m {
		if !validSet[k] {
			return errInvalid("%s[%q]: неизвестное имя формы (нет в composition_forms.go)", name, k)
		}
	}
	return nil
}