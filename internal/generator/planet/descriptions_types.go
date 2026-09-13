// internal/generator/planet/descriptions_types.go
package planet

import "sync"

// ==================== ЗАПИСИ БИБЛИОТЕКИ ====================

// Opening — зачин описания. Загружается из openings_*.json.
type Opening struct {
	Text     string   `json:"text"`
	Requires []string `json:"requires"`
}

// Closing — концовка описания. Загружается из closings_*.json.
type Closing struct {
	Text     string   `json:"text"`
	Requires []string `json:"requires"`
}

// openingFile — формат JSON-файла с зачинами.
type openingFile struct {
	Openings []Opening `json:"openings"`
}

// closingFile — формат JSON-файла с концовками.
type closingFile struct {
	Closings []Closing `json:"closings"`
}

// ==================== КОНТЕКСТ ПЛАНЕТЫ ====================

// DescriptionContext — всё, что нужно для вычисления тегов
// и выбора текста. Заполняется в местах генерации планет.
//
// Поля соответствуют свойствам из Properties и дополнительным
// данным (orbit_index, hydrosphere), которые Properties не хранит.
type DescriptionContext struct {
	// Идентификация
	PlanetID string // UUID планеты (для детерминированного выбора)
	Type     string // геймдизайнерский тип (TypeVolcanic, TypeOceanic, ...)

	// Орбита
	OrbitIndex int

	// Атмосфера и гидросфера
	Atmosphere  string
	Hydrosphere string

	// Физические свойства
	Temperature  float64
	WaterPercent float64
	Mass         float64
	Density      float64
	Moons        int

	// Биосфера
	Life bool

	// Композиция и ядро
	Surface Composition
	Core    Core

	// Газовый гигант — флаг из классификатора
	IsGasGiant bool
}

// ==================== МЕНЕДЖЕР ====================

// descriptionsManager — хранит загруженные описания для всех типов.
// Один экземпляр на процесс, инициализируется при старте сервера.
//
// Поля под RWMutex: после LoadAll менеджер только читается из горутин.
type descriptionsManager struct {
	mu sync.RWMutex

	// Ключ — геймдизайнерский тип (TypeVolcanic и т.п.).
	// Значение — все зачины/концовки, загруженные для этого типа.
	openings map[string][]Opening
	closings map[string][]Closing

	// Статистика для логов (заполняется один раз при загрузке).
	stats map[string]descriptionsStats
}

// descriptionsStats — счётчики по одному типу (для лога при старте).
type descriptionsStats struct {
	OpeningsCount  int
	ClosingsCount  int
	OpeningsFiles  int
	ClosingsFiles  int
}

// ==================== КОНСТАНТЫ ====================

const (
	// Префиксы имён файлов, которые менеджер ищет в папке типа.
	openingsFilePrefix = "openings_"
	closingsFilePrefix = "closings_"
	jsonSuffix         = ".json"

	// Разделители для хэша — разные соли для зачина и концовки,
	// чтобы выбор одного не влиял на другой.
	openingHashSalt = ":opening"
	closingHashSalt = ":closing"

	// Разделитель между зачином и концовкой в итоговом тексте.
	// Пока средних блоков нет — просто два переноса строки.
	// Когда появятся средние блоки — заменить на strings.Join.
	descriptionSeparator = "\n\n"
)