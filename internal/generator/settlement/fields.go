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
		Key: "temperature", Label: "Температура", Type: FieldNumber, Unit: "°C",
		Min: floatPtr(-223), Max: floatPtr(927),
	},
	{
		Key: "water_percent", Label: "Вода", Type: FieldNumber, Unit: "%",
		Min: floatPtr(0), Max: floatPtr(100),
	},
	{
		Key: "mass", Label: "Масса", Type: FieldNumber, Unit: "M⊕",
		Min: floatPtr(0.1), Max: floatPtr(10),
	},
	{
		Key: "size", Label: "Радиус", Type: FieldNumber, Unit: "R⊕",
		Min: floatPtr(0.4), Max: floatPtr(12.6),
	},
	{
		Key: "gravity", Label: "Гравитация", Type: FieldNumber, Unit: "g",
		Min: floatPtr(0.2), Max: floatPtr(4),
	},
	{
		Key: "density", Label: "Плотность", Type: FieldNumber, Unit: "g/cm³",
		Min: floatPtr(0.5), Max: floatPtr(8),
	},
	{
		Key: "moons", Label: "Спутники", Type: FieldNumber,
		Min: floatPtr(0), Max: floatPtr(30),
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
			"скалы", "пески_пустыни", "кратеры", "стеклянные_поля",
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