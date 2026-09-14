// internal/generator/ship/config.go
// Конфиг генератора форм кораблей — config/ship_visual.json (спека 99.2.15
// §2): палитра 12 цветов, акценты + accentRules, зоны/границы + габариты
// форм (§3.1), общие правила стиля (§3.2), порядок слоёв. Загружается один
// раз при старте (как архетипы планет); палитра и акценты read-only в
// админке (правка — будущее, не сейчас).
package ship

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config — корневая структура config/ship_visual.json.
type Config struct {
	Palette     []string            `json:"palette"`     // 12 цветов корабля (§4)
	Accents     Accents             `json:"accents"`     // фиксированные акцентные цвета + обводка
	AccentRules map[string][]string `json:"accentRules"` // какие акценты каким категориям
	Style       Style               `json:"style"`       // общие правила стиля (§3.2)
	Categories  map[string]Category `json:"categories"`  // зоны/границы/габариты (§3.1)
	LayerOrder  []string            `json:"layerOrder"`  // корпус → крылья → нос → двигатели → хвост
}

// Accents — фиксированные акцентные цвета (вне палитры корабля, §4).
type Accents struct {
	Glass     string `json:"glass"`      // кабина, иллюминаторы
	Glow      string `json:"glow"`       // свечение сопел двигателей
	FireRed   string `json:"fire_red"`   // навигационный огонь левого борта
	FireGreen string `json:"fire_green"` // навигационный огонь правого борта
	Nozzle    string `json:"nozzle"`     // тёмное сопло двигателей
	Stroke    string `json:"stroke"`     // обводка (общая, как в legacy-спрайтах)
}

// Color — цвет акцента по имени ("glass", "glow", "fire_red", ...).
func (a Accents) Color(name string) string {
	switch name {
	case "glass":
		return a.Glass
	case "glow":
		return a.Glow
	case "fire_red":
		return a.FireRed
	case "fire_green":
		return a.FireGreen
	case "nozzle":
		return a.Nozzle
	}
	return ""
}

// Style — единство стиля (спека §3.2, инвариант И9).
type Style struct {
	StrokeWidth    float64   `json:"strokeWidth"`    // обводка 3 px
	CornerRadius   float64   `json:"cornerRadius"`   // внешние углы r=6
	InnerRadius    float64   `json:"innerRadius"`    // внутренние/вырезы r=3
	MinRadius      float64   `json:"minRadius"`      // минимум r=2
	SegmentsMin    int       `json:"segmentsMin"`    // контур основной массы 4–8 сегментов
	SegmentsMax    int       `json:"segmentsMax"`
	MaxArcs        int       `json:"maxArcs"` // не более 1 дуги
	FillRatio      float64   `json:"fillRatio"` // заполненность ≥ 55% bbox
	AccentOffset   float64   `json:"accentOffset"` // отступ акцента от контура ≥ 3 px
	AccentRadius   float64   `json:"accentRadius"` // скругление акцента 6 px
	MaxAccentArea  float64   `json:"maxAccentArea"` // один акцент ≤ 6% заливки (≤ 90 px²)
	MaxAccentTotal float64   `json:"maxAccentTotal"` // суммарно ≤ 10% заливки (≤ 240 px²)
	Angles         []float64 `json:"angles"` // наклонные кромки 20/25/30°
}

// Category — зона, границы и габариты форм одной категории (спека §3.1).
// Поля-указатели: nil = параметр не применим к категории.
type Category struct {
	Zone     Zone     `json:"zone"`     // bbox детали на холсте
	Width    *Range   `json:"width,omitempty"`    // корпус: ширина 60–110
	Height   *Range   `json:"height,omitempty"`   // корпус: высота 42–60
	MinX     *float64 `json:"minX,omitempty"`     // корпус: min x ≤ 65 (корма достаёт до двигателей)
	MaxX     *float64 `json:"maxX,omitempty"`     // корпус: max x ≥ 130 (нос достаёт до носа)
	Base     *Range   `json:"base,omitempty"`     // нос/двигатели: база 24–40 / 22–40 (центр y=100); хвост: ширина базы 18–40
	Length   *Range   `json:"length,omitempty"`   // нос/двигатели: длина (x)
	TipMaxX  *Range   `json:"tipMaxX,omitempty"`  // нос: bbox max x ∈ [185, 197]
	TipMinX  *float64 `json:"tipMinX,omitempty"`  // нос: bbox min x ≤ 130 (база на корпусе)
	Chord    *Range   `json:"chord,omitempty"`    // крылья: хорда у борта 6–38
	Reach    *Range   `json:"reach,omitempty"`    // крылья: вылет 15–50; хвост: высота 15–55 (отдельно Height нет)
	Span     *Range   `json:"span,omitempty"`     // крылья: размах (x) 20–65; хвост: 20–60
	MaxXPart *float64 `json:"maxXPart,omitempty"` // крылья: max x ≤ 150
	ReachY   *float64 `json:"reachY,omitempty"`   // крылья/хвост: база достигает y ≥ 79 (y ≤ 121 — зеркало)
	MinLeft  *float64 `json:"minLeft,omitempty"`  // двигатели: левый край ≥ 3 px
	BaseMaxX *float64 `json:"baseMaxX,omitempty"` // двигатели: bbox max x ≥ 65 (база на корпусе)
}

// Range — границы параметра [min, max].
type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// Zone — bbox зоны категории на холсте 200×200.
type Zone struct {
	X [2]float64 `json:"x"`
	Y [2]float64 `json:"y"`
}

// гарантированная область корпуса (пересечение всех допустимых корпусов,
// спека §3.1): база любого придатка в ней лежит на корпусе при любой форме.
var guaranteedHullRegion = Zone{X: [2]float64{65, 130}, Y: [2]float64{79, 121}}

// LoadConfig читает config/ship_visual.json и валидирует обязательные части.
// Вызывается при старте; при ошибке сервер не стартует (как описания планет).
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("ship_visual.json: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	config = &cfg
	return &cfg, nil
}

// Validate — минимальная проверка целостности конфига: все 5 категорий,
// 12 цветов палитры, акценты и порядок слоёв.
func (c *Config) Validate() error {
	if len(c.Palette) != 12 {
		return fmt.Errorf("ship_visual: палитра должна содержать 12 цветов, найдено %d", len(c.Palette))
	}
	if len(c.LayerOrder) != 5 {
		return fmt.Errorf("ship_visual: layerOrder должен содержать 5 категорий")
	}
	if c.Accents.Stroke == "" || c.Accents.Glass == "" || c.Accents.Glow == "" ||
		c.Accents.FireRed == "" || c.Accents.FireGreen == "" || c.Accents.Nozzle == "" {
		return fmt.Errorf("ship_visual: неполная палитра акцентов")
	}
	for _, cat := range Categories {
		cc, ok := c.Categories[cat]
		if !ok {
			return fmt.Errorf("ship_visual: нет категории %q", cat)
		}
		if len(cc.Zone.X) != 2 || len(cc.Zone.Y) != 2 {
			return fmt.Errorf("ship_visual: категория %q без зоны", cat)
		}
	}
	for cat := range c.Categories {
		if !categoryValid(cat) {
			return fmt.Errorf("ship_visual: неизвестная категория %q", cat)
		}
	}
	return nil
}

// Categories — константа списка категорий в одном месте (спека §2, FAQ В1).
var Categories = []string{"hull", "nose", "wings", "engine", "tail"}

func categoryValid(cat string) bool {
	for _, c := range Categories {
		if c == cat {
			return true
		}
	}
	return false
}

// config — глобальный конфиг, загружается LoadConfig при старте. Чтение —
// только после загрузки; GeneratePart возвращает ошибку, если конфиг не
// загружен (тесты грузят файл через LoadConfig).
var config *Config

// defaultConfig — текущий загруженный конфиг (nil, если не загружен).
func defaultConfig() (*Config, error) {
	if config == nil {
		return nil, fmt.Errorf("ship: конфиг не загружен (вызовите LoadConfig)")
	}
	return config, nil
}