// Package config — загрузка и валидация конфигов арт-студии
// (config/art/*.json: studio, forms, families, humans).
package config

// StudioConfig — параметры инструмента (config/art/studio.json, спека 67a.1 §4.4).
type StudioConfig struct {
	Port            int     `json:"port"`
	ComfyURL        string  `json:"comfy_url"`
	ComfyInput      string  `json:"comfy_input"`
	PythonCmd       string  `json:"python_cmd"`
	RembgCLI        string  `json:"rembg_cli"`
	Checkpoint      string  `json:"checkpoint"`
	Steps           int     `json:"steps"`
	Cfg             float64 `json:"cfg"`
	CfgImg          float64 `json:"cfg_img"`
	DenoiseRef      float64 `json:"denoise_ref"`
	CNStrength      float64 `json:"cn_strength"`
	CNEnd           float64 `json:"cn_end"`
	Workers         int     `json:"workers"`
	PoolRoot        string  `json:"pool_root"`
	PollIntervalS   int     `json:"poll_interval_s"`
	HistoryTimeoutS int     `json:"history_timeout_s"`
	AutoRefreshMS   int     `json:"auto_refresh_ms"`
	MaxCount        int     `json:"max_count"`
	Ships           *ShipsParams `json:"ships"`
}

// ShipsParams — параметры вкладки «Корабли рас» (блок ships в studio.json,
// рецепт 2026-09-21): модель только для кораблей (общий селект «Модель» и
// другие вкладки не трогает) + этап детализации Hi-Res.
type ShipsParams struct {
	Model string      `json:"model"`
	Steps int         `json:"steps"`
	Cfg   float64     `json:"cfg"`
	Hires HiresParams `json:"hires"`
}

// HiresParams — этап детализации кораблей (4x-UltraSharp → ImageScale →
// img2img): выполняется на кандидатах, прошедших автопроверку кадра.
type HiresParams struct {
	Enabled  bool    `json:"enabled"`
	Denoise  float64 `json:"denoise"`
	Steps    int     `json:"steps"`
	Cfg      float64 `json:"cfg"`
	Upscaler string  `json:"upscaler"`
	Scale    int     `json:"scale"`
}

// ShipParams — параметры кораблей с дефолтами (рецепт 2026-09-21): блок ships
// в studio.json опционален, тесты и старые конфиги получают те же числа.
func (c *StudioConfig) ShipParams() ShipsParams {
	sp := ShipsParams{
		Model: "juggernaut-xl-v9.safetensors",
		Steps: 32,
		Cfg:   6.0,
		Hires: HiresParams{Denoise: 0.40, Steps: 30, Cfg: 6.0, Upscaler: "4x-UltraSharp.pth", Scale: 1536},
	}
	if c.Ships == nil {
		return sp
	}
	if c.Ships.Model != "" {
		sp.Model = c.Ships.Model
	}
	if c.Ships.Steps != 0 {
		sp.Steps = c.Ships.Steps
	}
	if c.Ships.Cfg != 0 {
		sp.Cfg = c.Ships.Cfg
	}
	sp.Hires.Enabled = c.Ships.Hires.Enabled
	if c.Ships.Hires.Denoise != 0 {
		sp.Hires.Denoise = c.Ships.Hires.Denoise
	}
	if c.Ships.Hires.Steps != 0 {
		sp.Hires.Steps = c.Ships.Hires.Steps
	}
	if c.Ships.Hires.Cfg != 0 {
		sp.Hires.Cfg = c.Ships.Hires.Cfg
	}
	if c.Ships.Hires.Upscaler != "" {
		sp.Hires.Upscaler = c.Ships.Hires.Upscaler
	}
	if c.Ships.Hires.Scale != 0 {
		sp.Hires.Scale = c.Ships.Hires.Scale
	}
	return sp
}

// KnownCheckpoints — доступные чекпоинты SDXL (селект «Модель» в UI).
// Список захардкожен (не ходим в ComfyUI за списком); дефолт — cfg.Checkpoint
// из studio.json. Выбор на сессию: диск не переписывается, рестарт студии
// вернёт cfg.Checkpoint.
var KnownCheckpoints = []string{
	"juggernaut-xl-v9.safetensors",
	"dreamshaper-xl-v1.safetensors",
}

// IsKnownCheckpoint — имя ∈ KnownCheckpoints (валидация POST /checkpoint).
func IsKnownCheckpoint(name string) bool {
	for _, c := range KnownCheckpoints {
		if c == name {
			return true
		}
	}
	return false
}

// FormsConfig — словарь форм (config/art/forms.json, спека 67a.1 §4.1).
// Комбинаторный: phrase_template + shapes x struct x character x parts.
type FormsConfig struct {
	PhraseTemplate string            `json:"phrase_template"`
	Shapes         []Shape           `json:"shapes"`
	Struct         []string          `json:"struct"`
	Character      []string          `json:"character"`
	Parts          []string            `json:"parts"`
	AnthroForms    []string            `json:"anthro_forms"`
	BeastForms     []string            `json:"beast_forms"`
	XenoForms      []string            `json:"xeno_forms"`
	AmorphousForms []string            `json:"amorphous_forms"`
	CrystalForms   []string            `json:"crystal_forms"`
	MechForms      []string            `json:"mech_forms"`
	TitanForms     []string            `json:"titan_forms"`
	CategoryKeys   map[string][]string `json:"category_keys"`
	PaletteAccents []string          `json:"palette_accents"`
}

// Shape — базовая форма с категориями (фильтр форм под семейство).
type Shape struct {
	Shape      string   `json:"shape"`
	Categories []string `json:"categories"`
}

// FamiliesConfig — семейства F2–F9 (config/art/families.json, спека 67a.1 §4.2).
type FamiliesConfig map[string]Family

// Family — семейство рас: общие шаблоны + расы.
type Family struct {
	Name         string   `json:"name"`
	Races        []Race   `json:"races"`
	Extra        []string `json:"extra"`
	Anchor       []string `json:"anchor"`
	Scene        string   `json:"scene"`
	Neg          string   `json:"neg"`
	AnthroForms  []string `json:"anthro_forms,omitempty"`  // свои антропо-формы (иначе глобальные)
	BeastForms   []string `json:"beast_forms,omitempty"`   // свои звериные формы (иначе глобальные)
	AnthroClothes []string `json:"anthro_clothes,omitempty"` // фантастическая одежда антропоморфов
	AnthroNeg    string   `json:"anthro_neg,omitempty"`    // негатив для антропо (иначе fam.Neg)
}

// Race — раса семейства (id — номер из 99.2.21, строка).
type Race struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Basis      string   `json:"basis"`
	Forms      []string `json:"forms"`
	Materials  []string `json:"materials"`
	Glows      []string `json:"glows"`
	Appearance string   `json:"appearance,omitempty"` // промпт-фраза внешности (98a §4.1)
	Blocked    []string `json:"blocked,omitempty"`    // токены-запреты, substring-матчинг (98a §4.1)
}

// HumansConfig — студия людей (config/art/humans.json, спека 67a.1 §4.3).
type HumansConfig struct {
	Family         string            `json:"family"`
	Name           string            `json:"name"`
	Axes           map[string][]string `json:"axes"`
	Neg            string            `json:"neg"`
	PromptTemplate string            `json:"prompt_template"`
	Params         HumanParams       `json:"params"`
}

// HumanParams — параметры генерации людей.
type HumanParams struct {
	Width              int     `json:"width"`
	Height             int     `json:"height"`
	Steps              int     `json:"steps"`
	Cfg                float64 `json:"cfg"`
	Denoise            float64 `json:"denoise"`
	BeardChance        float64 `json:"beard_chance"`
	SecondFacialChance float64 `json:"second_facial_chance"`
	MaxCount           int     `json:"max_count"`
}