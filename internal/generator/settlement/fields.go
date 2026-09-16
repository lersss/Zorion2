// internal/generator/settlement/fields.go
//
// Реестр полей planet.data для правил модели генерации. Отдаётся админке
// через GET /admin/settlement-fields: название, тип, экстремумы выпускаемых
// значений и допустимые значения — чтобы форма «добавить поле» показывала
// подсказки, а не просила пользователя угадывать.

package settlement

// FieldType — тип поля planet.data.
type FieldType string

const (
	FieldNumber FieldType = "number"
	FieldString FieldType = "string"
	FieldBool   FieldType = "bool"
)

// FieldSpec — описание поля для формы правила.
type FieldSpec struct {
	Key    string    `json:"key"`
	Label  string    `json:"label"`
	Type   FieldType `json:"type"`
	Unit   string    `json:"unit,omitempty"`
	Min    *float64  `json:"min,omitempty"`    // подсказка: возможный минимум
	Max    *float64  `json:"max,omitempty"`    // подсказка: возможный максимум
	Values []string  `json:"values,omitempty"` // подсказка: допустимые значения
}

// HasValue — есть ли значение среди допустимых (для string-полей).
func (s FieldSpec) HasValue(v string) bool {
	return containsString(s.Values, v)
}

// fieldSpecs — полный реестр. Диапазоны взяты из конфигов генерации:
// planet_archetypes.json, physics.go (greenhouseByAtmosphere), спутники и
// газовые гиганты. Если поле может побывать вне диапазона — спроси и поправь.
var fieldSpecs = []FieldSpec{
	{
		// Кельвины в данных планет, но форма работает в °C (K = °C + 273).
		// 20…2500 K → −253…2227 °C (physics.go:10-11, TempAbsoluteMin/Max).
		Key: "temperature", Label: "Температура", Type: FieldNumber, Unit: "°C",
		Min: floatPtr(-253), Max: floatPtr(2227),
	},
	{
		Key: "water_percent", Label: "Вода", Type: FieldNumber, Unit: "%",
		Min: floatPtr(0), Max: floatPtr(100),
	},
	{
		// Архетипы 0.1–8, гиганты до 13 MJ (GasGiantMassMax).
		Key: "mass", Label: "Масса", Type: FieldNumber, Unit: "M⊕",
		Min: floatPtr(0.1), Max: floatPtr(4131),
	},
	{
		// Плато насыщения гигантов (GasGiantRadiusMax); минимум 0.4 покрывает
		// реальный минимум генератора 0.425.
		Key: "size", Label: "Радиус", Type: FieldNumber, Unit: "R⊕",
		Min: floatPtr(0.4), Max: floatPtr(11.2),
	},
	{
		// g = M/R²: 4131/11.2² = 32.9 (честные гиганты, без потолка).
		Key: "gravity", Label: "Гравитация", Type: FieldNumber, Unit: "g",
		Min: floatPtr(0.29), Max: floatPtr(33),
	},
	{
		// Единица — ρ⊕ (ед. Земли), НЕ g/cm³: генератор выдаёт ед. Земли.
		// 15.9/5.9³ = 0.0774 … 4131/11.2³ = 2.9403 (рамки наружу).
		Key: "density", Label: "Плотность", Type: FieldNumber, Unit: "ρ⊕",
		Min: floatPtr(0.077), Max: floatPtr(2.95),
	},
	{
		// Гиганты 3–10, стандартные ≤ 4 (determineMoons).
		Key: "moons", Label: "Спутники", Type: FieldNumber,
		Min: floatPtr(0), Max: floatPtr(10),
	},
	{
		Key: "development_level", Label: "Развитие", Type: FieldNumber,
		Min: floatPtr(0), Max: floatPtr(1),
	},
	{
		Key: "system_age", Label: "Возраст системы", Type: FieldNumber, Unit: "млрд лет",
		Min: floatPtr(0.1), Max: floatPtr(13),
	},
	{
		// Вложенное поле: живёт как core.radioactivity в planet.data; dot-путь
		// раскрывается до вложенного map при оверрайдах (twin.go cloneTwinData).
		Key: "core.radioactivity", Label: "Радиоактивность ядра", Type: FieldNumber,
		Min: floatPtr(0), Max: floatPtr(100),
	},
	{
		Key: "archetype", Label: "Архетип", Type: FieldString,
		Values: []string{
			"жаркий", "умеренный", "холодный", "экстремальный", "изменчивый",
		},
	},
	{
		Key: "atmosphere", Label: "Атмосфера", Type: FieldString,
		Values: []string{
			"разреженная", "азотная", "азотно-кислородная", "туманная",
			"облачная", "углекислая", "электрическая", "ядовитая", "метановая",
			"плотная", "водородная", "гелиевая", "водородно-гелиевая", "парниковая",
		},
	},
	{
		Key: "biosphere", Label: "Биосфера", Type: FieldString,
		Values: []string{
			"стерильная", "микробная", "растительная", "симбиотическая",
			"светящаяся", "грибная",
		},
	},
	{
		Key: "hydrosphere", Label: "Гидросфера", Type: FieldString,
		Values: []string{
			"сухая", "кислотная", "океаны", "озёра", "подлёдная", "ледяной покров",
		},
	},
	{
		Key: "type", Label: "Тип планеты", Type: FieldString,
		Values: []string{
			"газовый гигант", "радиоактивная", "землеподобная", "океаническая",
			"ледяная", "вулканическая", "пустынная", "стеклянная", "металлическая",
			"органик", "скалистая (базовая)",
		},
	},
	{
		Key: "surface_dominant", Label: "Доминирующая поверхность", Type: FieldString,
		Values: []string{
			"горы", "пески_пустыни", "кратеры", "стеклянные_поля",
			"металлические_поля", "лавовые_поля", "вулканические_поля", "ледники",
			"мёрзлые_газы", "океаны", "озёра_реки", "луга_степи", "леса", "джунгли",
			"болота", "коралловые_рифы", "газовый_гигант",
		},
	},
	{
		Key: "life", Label: "Жизнь", Type: FieldBool,
	},
	{
		Key: "radioactive", Label: "Радиоактивная", Type: FieldBool,
	},
	{
		Key: "is_gas_giant", Label: "Газовый гигант", Type: FieldBool,
	},
	// === Новые поля физического каскада (99.2.20 §6.4) ===
	// Рамки = границы генератора (инвариант «рамки == границы», 99.2.15 §7.2).
	{
		// Кламп P_surf ≤ 1000 атм (§3.6 п.5).
		Key: "atmosphere_data.pressure_atm", Label: "Давление атмосферы", Type: FieldNumber, Unit: "атм",
		Min: floatPtr(0), Max: floatPtr(1000),
	},
	{
		Key: "liquid_water_possible", Label: "Возможна жидкая вода", Type: FieldBool,
	},
	{
		// Генератор 0.0007–127 000 лет (Y-звезда орбита 1 → O-звезда орбита 8).
		Key: "orbital_period", Label: "Орбитальный период", Type: FieldNumber, Unit: "лет",
		Min: floatPtr(0.0001), Max: floatPtr(200000),
	},
	{
		// Генератор 4–215 км/с (11.2×√(M/R): M=0.1, ρ=0.34 → 4.3; гигант 13 MJ → 215).
		Key: "escape_velocity", Label: "Скорость убегания", Type: FieldNumber, Unit: "км/с",
		Min: floatPtr(0.5), Max: floatPtr(250),
	},
	{
		// Ролл 0.03–0.3 (§3.2).
		Key: "eccentricity", Label: "Эксцентриситет", Type: FieldNumber,
		Min: floatPtr(0), Max: floatPtr(0.3),
	},
	{
		Key: "tidal_lock", Label: "Приливный захват", Type: FieldBool,
	},
}

// FieldRegistry — список полей для формы правил.
func FieldRegistry() []FieldSpec {
	out := make([]FieldSpec, len(fieldSpecs))
	copy(out, fieldSpecs)
	return out
}

// FieldSpecFor — описание поля по ключу (известно ли поле вообще).
func FieldSpecFor(key string) (FieldSpec, bool) {
	for _, s := range fieldSpecs {
		if s.Key == key {
			return s, true
		}
	}
	return FieldSpec{}, false
}

// fieldType — тип поля (для eval без реестра; неизвестное поле → number).
func fieldType(key string) FieldType {
	if s, ok := FieldSpecFor(key); ok {
		return s.Type
	}
	return FieldNumber
}