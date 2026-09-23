// internal/generator/planet/biome_catalog.go
//
// Справочник биомов (99.2.28 §5): редактируемый каталог определений
// «что бывает» — биомы поверхности, типы недр, правила типов планет,
// параметры токсичности. Данные (конфиг-файл), не жёсткие константы кода;
// константы composition_forms.go остаются сидом имён для обратной
// совместимости старых миров.
//
// Хранение — вариант А (спека §5.2): конфиг-файл + in-memory store +
// админка пишет файл атомарно. Паттерн LoadCompatibilityMatrix:
// файл грузится при старте, битый JSON/нет файла → лог + встроенный сид
// (сервер не падает); hot-reload атомарной заменой store под RWMutex.
package planet

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// ==================== СУЩНОСТИ СПРАВОЧНИКА ====================

// BiomeDef — определение биома (99.2.28 §5.3). Поля условий — диапазоны,
// где биом в принципе возможен (гейты §6.2); 0 = граница не задана.
type BiomeDef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"` // литосфера/вода/биосфера/вулканизм/крио/экзотика
	Description string `json:"description,omitempty"`

	TMin     float64 `json:"t_min,omitempty"` // K
	TMax     float64 `json:"t_max,omitempty"`
	PMin     float64 `json:"p_min,omitempty"` // атм
	PMax     float64 `json:"p_max,omitempty"`
	WaterMin float64 `json:"water_min,omitempty"`
	WaterMax float64 `json:"water_max,omitempty"`
	IronMin  float64 `json:"iron_min,omitempty"`
	IceMin   float64 `json:"ice_min,omitempty"`
	RockMin  float64 `json:"rock_min,omitempty"`
	GMin     float64 `json:"g_min,omitempty"` // g
	GMax     float64 `json:"g_max,omitempty"`

	// NeedsLight — фотосинтез требует света; nil = true (по умолчанию).
	// false у хемосадов/светящихся чащ/радиационных ковров (приложение §1).
	NeedsLight *bool `json:"needs_light,omitempty"`

	LiquidMedium string   `json:"liquid_medium,omitempty"` // вода/метан/аммиак/co2; пусто = нет
	AtmosphereOK []string `json:"atmosphere_ok,omitempty"` // признаки §0 (AND); пусто = любая
	Bands        []string `json:"bands"`                   // полосы э/ж/у/х — whitelist из справочника
	Volcanism    string   `json:"volcanism,omitempty"`     // any (V) / hot (V·smooth T≥500) / magma (V·smooth T≥1200)

	RequiresRadiation   bool `json:"requires_radiation,omitempty"`    // источник: P_surf < 0.5 атм
	RequiresTidalLock   bool `json:"requires_tidal_lock,omitempty"`   // терминаторные
	RequiresLife        bool `json:"requires_life,omitempty"`         // строгий гейт
	RequiresLiquidWater bool `json:"requires_liquid_water,omitempty"` // водный биосферный

	TypeTags   []string `json:"type_tags,omitempty"` // теги типа планеты (вулканический/биосферный/...)
	WeightBase float64  `json:"weight_base"`         // 0–10: насколько биом вообще распространён
	Albedo     float64  `json:"albedo"`              // 0–1: альбедо поверхности (слой 5)

	// Color — переопределение цвета поверхности (спека 2026-09-20 §4.2):
	// необязательный hex #RRGGBB; пусто = база категории + сдвиги. Правка —
	// только через админку (вкладка «Основное», редактор биома).
	Color string `json:"color,omitempty"`

	// View — рецепт вида биома (спека 2026-09-23 §2.2): дельта к семейству
	// вида. Свободная структура (скаляры/карты/списки) — резолвится сервером
	// (§2.3), в пакет прогулки едет готовым (biome_view). Пусто = фолбэк вида.
	View map[string]any `json:"view,omitempty"`
}

// SubterrainTypeDef — определение типа недр (99.2.28 §5.5, приложение §2).
type SubterrainTypeDef struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Category   string               `json:"category"` // породы/вулканические/осадочные/биогенные/водные/ледяные/рудные/радиоактивные/карстовые
	Bands      []string             `json:"bands"`
	WeightBase float64              `json:"weight_base"`
	Conditions SubterrainConditions `json:"conditions,omitempty"`
}

// SubterrainConditions — условия появления типа недр (гейты §8).
type SubterrainConditions struct {
	TMax             float64 `json:"t_max,omitempty"`
	WaterMin         float64 `json:"water_min,omitempty"`
	VMin             float64 `json:"v_min,omitempty"`
	VMax             float64 `json:"v_max,omitempty"`
	IronMin          float64 `json:"iron_min,omitempty"`
	IceMin           float64 `json:"ice_min,omitempty"`
	RockMin          float64 `json:"rock_min,omitempty"`
	FlagRequired     bool    `json:"flag_required,omitempty"`
	LifeRequired     bool    `json:"life_required,omitempty"`
	PMin             float64 `json:"p_min,omitempty"`
	RadioactivityMin float64 `json:"radioactivity_min,omitempty"`
	DraftOceans      bool    `json:"draft_oceans,omitempty"` // океаны > 0 в draft поверхности
}

// PlanetTypeRule — правило типа планеты (99.2.28 §11): предикаты + пороги —
// данные; код — интерпретатор без чисел. Порядок правил сверху вниз,
// первое сработавшее → тип.
type PlanetTypeRule struct {
	ID         string      `json:"id"`
	Predicates []Predicate `json:"predicates"`
}

// Predicate — предикат правила типа (приложение §3).
type Predicate struct {
	Type  string      `json:"type"` // settleable/life/radioactive_core/dominant_form/share_of/tag_sum/temperature/water_percent/any_of/and/or
	Form  string      `json:"form,omitempty"`
	Forms []string    `json:"forms,omitempty"`
	Tag   string      `json:"tag,omitempty"`
	Min   float64     `json:"min,omitempty"`
	Lt    float64     `json:"lt,omitempty"`
	Gt    float64     `json:"gt,omitempty"`
	Rules []Predicate `json:"rules,omitempty"` // для and/or
}

// CatalogParams — параметры справочника (99.2.28 §15.1): пороги токсичности.
type CatalogParams struct {
	ToxicThresholds map[string]float64 `json:"toxic_thresholds"`
}

// BiomeCatalog — справочник «что бывает» целиком.
type BiomeCatalog struct {
	Biomes          []BiomeDef          `json:"biomes"`
	SubterrainTypes []SubterrainTypeDef `json:"subterrain_types"`
	PlanetTypes     []PlanetTypeRule    `json:"planet_types"`
	FallbackType    string              `json:"fallback_type"`
	Params          CatalogParams       `json:"params"`

	// Рецепт вида (спека 2026-09-23 §2.1): семейства вида (пресеты грамматики)
	// и реестр допустимых примитивов. Часть справочника — в обеих копиях (§2.7),
	// переносятся GET/PATCH админки (§7).
	ViewFamilies   []map[string]any `json:"view_families,omitempty"`
	ViewPrimitives []ViewPrimitive  `json:"view_primitives,omitempty"`
}

// ==================== STORE (RWMutex, hot-reload) ====================

var (
	biomeCatalogMu sync.RWMutex
	biomeCatalog   = defaultBiomeCatalog() // сид; заменяется LoadBiomeCatalog
	// biomeCatalogPath — путь рабочего файла (config/biome_catalog.json),
	// задаётся LoadBiomeCatalog; SaveBiomeCatalog пишет сюда атомарно.
	biomeCatalogPath string
)

// GetBiomeCatalog — текущий справочник (сид, если не загружен).
// Возвращает указатель на неизменяемый каталог: читатели не мутируют.
func GetBiomeCatalog() *BiomeCatalog {
	biomeCatalogMu.RLock()
	defer biomeCatalogMu.RUnlock()
	return biomeCatalog
}

// LoadBiomeCatalog — загружает справочник из JSON (99.2.28 §15.1).
// Нет файла / битый JSON / невалидный каталог → ошибка (вызывающий логирует;
// store остаётся на сиде — сервер не падает). Атомарная замена store
// (hot-reload, паттерн LoadCompatibilityMatrix).
func LoadBiomeCatalog(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	var cat BiomeCatalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return err
	}
	if err := cat.Validate(); err != nil {
		return err
	}
	// Проверка вида — нефатальная, отдельная от Validate() (§2.8): ошибки
	// рецептов видимы в логе, но не роняют каталог/генерацию.
	logViewDiagnostics(&cat)
	biomeCatalogMu.Lock()
	biomeCatalog = &cat
	biomeCatalogPath = absPath
	biomeCatalogMu.Unlock()
	return nil
}

// RebuildBiomeCatalog — атомарная замена store (для админки/hot-reload).
func RebuildBiomeCatalog(cat *BiomeCatalog) error {
	if cat == nil {
		return fmt.Errorf("справочник биомов: nil")
	}
	if err := cat.Validate(); err != nil {
		return err
	}
	logViewDiagnostics(cat) // нефатальная проверка вида (§2.8)
	biomeCatalogMu.Lock()
	biomeCatalog = cat
	biomeCatalogMu.Unlock()
	return nil
}

// ResetBiomeCatalogToSeed — сброс к заводскому сиду («Сбросить к заводским»).
func ResetBiomeCatalogToSeed() {
	biomeCatalogMu.Lock()
	biomeCatalog = defaultBiomeCatalog()
	biomeCatalogMu.Unlock()
}

// SeedBiomeCatalog — копия встроенного сида (для «Сбросить к заводским»:
// store + файл). defaultBiomeCatalog парсит embed-JSON заново — свежая копия.
func SeedBiomeCatalog() *BiomeCatalog {
	return defaultBiomeCatalog()
}

// SaveBiomeCatalog — атомарная запись справочника в файл (tmp + rename,
// паттерн race_balancer/пресетов 99.2.17 §5). Путь — из LoadBiomeCatalog;
// не загружен → ошибка. Ошибка записи — на стороне вызывающего (лог,
// правки сессионные, сервер не падает).
func SaveBiomeCatalog(cat *BiomeCatalog) error {
	biomeCatalogMu.RLock()
	path := biomeCatalogPath
	biomeCatalogMu.RUnlock()
	if path == "" {
		return fmt.Errorf("справочник биомов: путь файла не задан (LoadBiomeCatalog не вызывался)")
	}
	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return fmt.Errorf("справочник биомов: marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("справочник биомов: каталог %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("справочник биомов: запись tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("справочник биомов: rename: %w", err)
	}
	return nil
}

// ==================== ДОСТУП ====================

// BiomeByID — определение биома по id (nil, если нет).
func (c *BiomeCatalog) BiomeByID(id string) *BiomeDef {
	for i := range c.Biomes {
		if c.Biomes[i].ID == id {
			return &c.Biomes[i]
		}
	}
	return nil
}

// SubterrainByID — определение типа недр по id (nil, если нет).
func (c *BiomeCatalog) SubterrainByID(id string) *SubterrainTypeDef {
	for i := range c.SubterrainTypes {
		if c.SubterrainTypes[i].ID == id {
			return &c.SubterrainTypes[i]
		}
	}
	return nil
}

// ==================== ВАЛИДАЦИЯ (инвариант 17) ====================

// validAtmosphereFeature — признаки атмосферы по составу (приложение §0).
var validAtmosphereFeature = map[string]bool{
	"no_toxic": true, "toxic": true, "acidic": true, "methane": true,
	"nitrogen": true, "co2": true, "oxygen_free": true, "dense": true,
}

// validBand — допустимые полосы климатов (э/ж/у/х). Мусорная полоса
// (не из перечня) дала бы bandTempRange 0,0 и молча прошла пересечение
// с открытым t_range — биом никогда бы не родился (замечание ревью).
var validBand = map[string]bool{"э": true, "ж": true, "у": true, "х": true}

// Validate — инвариант 17 (99.2.28 §17): каждый биом достижим — bands не
// пусты, признаки атмосферы из перечня §0, bands пересекаются с t_range;
// типы недр — bands не пусты; правила типов и фолбэк не пусты.
func (c *BiomeCatalog) Validate() error {
	seen := make(map[string]bool, len(c.Biomes))
	for i := range c.Biomes {
		b := &c.Biomes[i]
		if b.ID == "" {
			return fmt.Errorf("биом #%d: пустой id", i)
		}
		if seen[b.ID] {
			return fmt.Errorf("биом %q: дубликат id", b.ID)
		}
		seen[b.ID] = true
		if len(b.Bands) == 0 {
			return fmt.Errorf("биом %q: bands пусты (инвариант 17)", b.ID)
		}
		for _, band := range b.Bands {
			if !validBand[band] {
				return fmt.Errorf("биом %q: неизвестная полоса %q (допустимы э/ж/у/х)", b.ID, band)
			}
		}
		for _, f := range b.AtmosphereOK {
			if !validAtmosphereFeature[f] {
				return fmt.Errorf("биом %q: неизвестный признак атмосферы %q (перечень §0)", b.ID, f)
			}
		}
		if !bandsOverlapTRange(b) {
			return fmt.Errorf("биом %q: bands не пересекаются с t_range (инвариант 17)", b.ID)
		}
		if b.TMin > 0 && b.TMax > 0 && b.TMin > b.TMax {
			return fmt.Errorf("биом %q: t_min > t_max", b.ID)
		}
		if b.PMin > 0 && b.PMax > 0 && b.PMin > b.PMax {
			return fmt.Errorf("биом %q: p_min > p_max", b.ID)
		}
		if b.WaterMin > 0 && b.WaterMax > 0 && b.WaterMin > b.WaterMax {
			return fmt.Errorf("биом %q: water_min > water_max", b.ID)
		}
		if b.GMin > 0 && b.GMax > 0 && b.GMin > b.GMax {
			return fmt.Errorf("биом %q: g_min > g_max", b.ID)
		}
		if b.WeightBase <= 0 {
			return fmt.Errorf("биом %q: weight_base <= 0", b.ID)
		}
		if b.Color != "" {
			if _, ok := parseHexColor(b.Color); !ok {
				return fmt.Errorf("биом %q: color %q — невалидный hex #RRGGBB (инвариант 17)", b.ID, b.Color)
			}
		}
	}
	seenSub := make(map[string]bool, len(c.SubterrainTypes))
	for i := range c.SubterrainTypes {
		s := &c.SubterrainTypes[i]
		if s.ID == "" {
			return fmt.Errorf("тип недр #%d: пустой id", i)
		}
		if seenSub[s.ID] {
			return fmt.Errorf("тип недр %q: дубликат id", s.ID)
		}
		seenSub[s.ID] = true
		if len(s.Bands) == 0 {
			return fmt.Errorf("тип недр %q: bands пусты", s.ID)
		}
		for _, band := range s.Bands {
			if !validBand[band] {
				return fmt.Errorf("тип недр %q: неизвестная полоса %q (допустимы э/ж/у/х)", s.ID, band)
			}
		}
		if s.Conditions.VMin > 0 && s.Conditions.VMax > 0 && s.Conditions.VMin > s.Conditions.VMax {
			return fmt.Errorf("тип недр %q: v_min > v_max", s.ID)
		}
	}
	if len(c.PlanetTypes) == 0 {
		return fmt.Errorf("planet_types пуст")
	}
	if c.FallbackType == "" {
		return fmt.Errorf("fallback_type пуст")
	}
	return nil
}

// bandTempRange — температурный диапазон полосы (99.2.20 §5: 250/350/500).
func bandTempRange(band string) (lo, hi float64) {
	switch band {
	case "э":
		return 500, math.Inf(1)
	case "ж":
		return 350, 500
	case "у":
		return 250, 350
	case "х":
		return math.Inf(-1), 250
	}
	return 0, 0
}

// bandsOverlapTRange — пересекается ли хоть одна полоса биома с его t_range
// (0 = граница не задана). Мёртвые полосы (биом не рождается в молча
// усечённом диапазоне) — ошибка загрузки (инвариант 17).
func bandsOverlapTRange(b *BiomeDef) bool {
	for _, band := range b.Bands {
		blo, bhi := bandTempRange(band)
		tlo, thi := b.TMin, b.TMax
		if tlo == 0 {
			tlo = math.Inf(-1)
		}
		if thi == 0 {
			thi = math.Inf(1)
		}
		if tlo < bhi && thi > blo {
			return true
		}
	}
	return false
}

// ==================== ЛОГ-ХЕЛПЕР ====================

// logCatalogError — единый формат лога ошибок справочника (паттерн
// LoadCompatibilityMatrix: сервер не падает).
func logCatalogError(err error) {
	log.Printf("⚠️ Справочник биомов: %v — использую встроенный сид", err)
}
