// internal/generator/settlement/preset.go
//
// СКРЫТ (65a): пресет человеческого генератора поселений устарел — расовый
// генератор (races.go + race_preset.go) заменяет. Код не удаляется (можно
// вернуть/удалить позже); settlement.Suitable больше не используется
// генерацией планет (cascade.go/descriptions_tags.go переведены на
// races.HumansSuitable, 65a).
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
// СКРЫТ (65a): устарел — пригодность людей считается через
// races.HumansSuitable.
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

// Preset — пресет генерации поселений. СКРЫТ (65a): устарел — расовый
// генератор заменяет.
type Preset struct {
	Suitability Suitability `json:"пригодность"`
	// NeighborChance — шанс заселения соседней расы на выбросе (0–1,
	// спека 99.2.21 §7.3, идея 56a «Крутилка»). Дефолт 0.3.
	// СКРЫТ (65a): переехал в RacePreset (config/race_settlement.json).
	NeighborChance float64 `json:"шанс_заселения_соседней_расы"`
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

// Current — текущий пресет (дефолтный, если не загружен).
func Current() *Preset {
	return loadCurrent()
}

// Suitable — проходит ли планета физические критерии пригодности
// текущего пресета. Шанс заселения здесь НЕ учитывается (это случайность
// генератора), только детерминированные условия.
// СКРЫТ (65a): устарел — пригодность людей считается через
// races.HumansSuitable (cascade.go, descriptions_tags.go).
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
		NeighborChance: 0.3,
	}
}