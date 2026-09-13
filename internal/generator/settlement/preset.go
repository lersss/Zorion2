// internal/generator/settlement/preset.go
package settlement

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Suitability — физические критерии «куда селить». Ключи по-русски,
// чтобы было просто крутить руками в пресете и через админку.
type Suitability struct {
	MinWater             float64  `json:"минимальная_вода"`
	MinTemperature       float64  `json:"минимальная_температура"`
	MaxTemperature       float64  `json:"максимальная_температура"`
	ForbiddenAtmospheres []string `json:"запрещённые_атмосферы"`
	ExcludeGasGiants     bool     `json:"исключить_газовых_гигантов"`
	ExcludeRadioactive   bool     `json:"исключить_радиоактивные"`
	OnlyWithLife         bool     `json:"заселять_только_с_жизнью"`
	Chance               float64  `json:"шанс_заселения"`
}

// Preset — пресет генерации поселений.
type Preset struct {
	Suitability Suitability `json:"пригодность"`
}

// current — пресет, загруженный последним вызовом LoadPreset.
// Используется генерацией планет (тег inhabited, классификация) и
// генератором поселений. Защищён от гонок через RWMutex.
var (
	presetMu sync.RWMutex
	current  = DefaultPreset()
)

// LoadPreset — загружает пресет из файла и делает его текущим.
// При ошибке текущий пресет не трогается.
func LoadPreset(path string) error {
	p, err := loadPresetFromFile(path)
	if err != nil {
		return err
	}
	setCurrent(p)
	return nil
}

// loadPresetFromFile — читает пресет из файла без смены текущего.
func loadPresetFromFile(path string) (*Preset, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("settlement preset: %w", err)
	}
	var p Preset
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("settlement preset %s: %w", absPath, err)
	}
	return &p, nil
}

func setCurrent(p *Preset) {
	presetMu.Lock()
	current = p
	presetMu.Unlock()
}

func loadCurrent() *Preset {
	presetMu.RLock()
	defer presetMu.RUnlock()
	return current
}

// ApplyOverrides — возвращает копию текущего пресета с переопределёнными
// полями пригодности (ключи — как в JSON пресета). Пустая карта — без изменений.
func ApplyOverrides(body map[string]interface{}) (*Preset, error) {
	p := Current().copy()
	for k, v := range body {
		switch k {
		case "минимальная_вода":
			f, err := toFloat(v)
			if err != nil {
				return nil, fmt.Errorf("поле %s: %w", k, err)
			}
			p.Suitability.MinWater = f
		case "минимальная_температура":
			f, err := toFloat(v)
			if err != nil {
				return nil, fmt.Errorf("поле %s: %w", k, err)
			}
			p.Suitability.MinTemperature = f
		case "максимальная_температура":
			f, err := toFloat(v)
			if err != nil {
				return nil, fmt.Errorf("поле %s: %w", k, err)
			}
			p.Suitability.MaxTemperature = f
		case "запрещённые_атмосферы":
			list, err := toStringSlice(v)
			if err != nil {
				return nil, fmt.Errorf("поле %s: %w", k, err)
			}
			p.Suitability.ForbiddenAtmospheres = list
		case "исключить_газовых_гигантов":
			b, err := toBool(v)
			if err != nil {
				return nil, fmt.Errorf("поле %s: %w", k, err)
			}
			p.Suitability.ExcludeGasGiants = b
		case "исключить_радиоактивные":
			b, err := toBool(v)
			if err != nil {
				return nil, fmt.Errorf("поле %s: %w", k, err)
			}
			p.Suitability.ExcludeRadioactive = b
		case "заселять_только_с_жизнью":
			b, err := toBool(v)
			if err != nil {
				return nil, fmt.Errorf("поле %s: %w", k, err)
			}
			p.Suitability.OnlyWithLife = b
		case "шанс_заселения":
			f, err := toFloat(v)
			if err != nil {
				return nil, fmt.Errorf("поле %s: %w", k, err)
			}
			p.Suitability.Chance = f
		default:
			return nil, fmt.Errorf("неизвестный параметр %q", k)
		}
	}
	return p, nil
}

// Current — текущий пресет (дефолтный, если не загружен).
func Current() *Preset {
	return loadCurrent()
}

// Suitable — проходит ли планета физические критерии пригодности
// текущего пресета. Шанс заселения здесь НЕ учитывается (это случайность
// генератора), только детерминированные условия.
func Suitable(water, temp float64, atmosphere string, life, isGasGiant, radioactive bool) bool {
	return Current().Suitability.suitable(water, temp, atmosphere, life, isGasGiant, radioactive)
}

func (s Suitability) suitable(water, temp float64, atmosphere string, life, isGasGiant, radioactive bool) bool {
	if water < s.MinWater {
		return false
	}
	if temp < s.MinTemperature || temp > s.MaxTemperature {
		return false
	}
	for _, a := range s.ForbiddenAtmospheres {
		if atmosphere == a {
			return false
		}
	}
	if s.ExcludeGasGiants && isGasGiant {
		return false
	}
	if s.ExcludeRadioactive && radioactive {
		return false
	}
	if s.OnlyWithLife && !life {
		return false
	}
	return true
}

func (p *Preset) copy() *Preset {
	if p == nil {
		return DefaultPreset()
	}
	out := *p
	out.Suitability.ForbiddenAtmospheres = append([]string(nil), p.Suitability.ForbiddenAtmospheres...)
	return &out
}

// ==================== ДЕФОЛТЫ ====================

// DefaultPreset — значения по умолчанию (совпадают с прежней формулой
// generateHabitable без требования жизни).
func DefaultPreset() *Preset {
	return &Preset{
		Suitability: Suitability{
			MinWater:             10,
			MinTemperature:       200,
			MaxTemperature:       350,
			ForbiddenAtmospheres: []string{"ядовитая"},
			ExcludeGasGiants:     true,
			ExcludeRadioactive:   true,
			OnlyWithLife:         false,
			Chance:               1.0,
		},
	}
}

// ==================== ХЕЛПЕРЫ ====================

func toFloat(v interface{}) (float64, error) {
	switch t := v.(type) {
	case float64:
		return t, nil
	case int:
		return float64(t), nil
	}
	return 0, fmt.Errorf("ожидалось число, получил %T", v)
}

func toBool(v interface{}) (bool, error) {
	if b, ok := v.(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("ожидалось true/false, получил %T", v)
}

func toStringSlice(v interface{}) ([]string, error) {
	raw, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("ожидался список, получил %T", v)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("ожидалась строка, получил %T", item)
		}
		out = append(out, s)
	}
	return out, nil
}