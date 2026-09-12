// internal/probe/presets.go
package probe

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Preset — именованная точка в пространстве параметров.
// Список открытый: новый пресет — запись в JSON, без правки кода.
type Preset struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Params CurveParams `json:"params"`
}

// rawParams — параметры пресета с указателями, чтобы отличить
// «поле опущено» от «поле равно нулю» (α=0 — легитимное значение).
type rawParams struct {
	PBase          *float64    `json:"p_base"`
	KBase          *float64    `json:"k_base"`
	Alpha          *float64    `json:"alpha"`
	NCritMode      *string     `json:"n_crit_mode"`
	NCrit          *float64    `json:"n_crit"`
	NDead          *float64    `json:"n_dead"`
	T0             *float64    `json:"t0"`
	HgMode         *string     `json:"hg_mode"`
	TMin           *float64    `json:"t_min"`
	TMax           *float64    `json:"t_max"`
	AtmoPenalty    *[3]float64 `json:"atmo_penalty"`
	WaterThreshold *float64    `json:"water_threshold"`
	WaterPenalty   *float64    `json:"water_penalty"`
}

type rawPreset struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Params rawParams  `json:"params"`
}

var (
	presetsMu  sync.RWMutex
	presetList []Preset
)

// LoadPresets — читает пресеты из JSON. Опущенные поля берутся из дефолтов.
// Ошибка файла не фатальна: до первой загрузки список пуст, работают крутилки.
func LoadPresets(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		log.Printf("⚠️ probe: пресеты не загружены: %v", err)
		return err
	}
	var raw []rawPreset
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("⚠️ probe: битый JSON пресетов: %v", err)
		return err
	}

	list := make([]Preset, 0, len(raw))
	for _, r := range raw {
		if r.ID == "" {
			continue
		}
		list = append(list, Preset{ID: r.ID, Name: r.Name, Params: applyOverrides(DefaultCurveParams(), r.Params)})
	}

	presetsMu.Lock()
	presetList = list
	presetsMu.Unlock()
	log.Printf("✅ probe: загружено пресетов: %d", len(list))
	return nil
}

// Presets — текущий список пресетов (копия).
func Presets() []Preset {
	presetsMu.RLock()
	defer presetsMu.RUnlock()
	out := make([]Preset, len(presetList))
	copy(out, presetList)
	return out
}

// PresetByID — пресет по id, ok=false если нет.
func PresetByID(id string) (Preset, bool) {
	for _, p := range Presets() {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// ApplyOverrides — применяет набор оверрайдов крутилок к параметрам.
// Оверрайды задаются так же, как rawParams: nil — поле не меняется.
func ApplyOverrides(base CurveParams, ov rawParams) CurveParams {
	return applyOverrides(base, ov)
}

func applyOverrides(base CurveParams, ov rawParams) CurveParams {
	if ov.PBase != nil {
		base.PBase = *ov.PBase
	}
	if ov.KBase != nil {
		base.KBase = *ov.KBase
	}
	if ov.Alpha != nil {
		base.Alpha = *ov.Alpha
	}
	if ov.NCritMode != nil {
		base.NCritMode = *ov.NCritMode
	}
	if ov.NCrit != nil {
		base.NCrit = *ov.NCrit
	}
	if ov.NDead != nil {
		base.NDead = *ov.NDead
	}
	if ov.T0 != nil {
		base.T0 = *ov.T0
	}
	if ov.HgMode != nil {
		base.HgMode = *ov.HgMode
	}
	if ov.TMin != nil {
		base.TMin = *ov.TMin
	}
	if ov.TMax != nil {
		base.TMax = *ov.TMax
	}
	if ov.AtmoPenalty != nil {
		base.AtmoPenalty = *ov.AtmoPenalty
	}
	if ov.WaterThreshold != nil {
		base.WaterThreshold = *ov.WaterThreshold
	}
	if ov.WaterPenalty != nil {
		base.WaterPenalty = *ov.WaterPenalty
	}
	return base
}

// ParseOverrides — превращает map из JSON-тела запроса в rawParams.
// Неизвестные ключи игнорируются; числовые поля — через float64.
func ParseOverrides(m map[string]interface{}) rawParams {
	var ov rawParams
	if v, ok := num(m["p_base"]); ok {
		ov.PBase = &v
	}
	if v, ok := num(m["k_base"]); ok {
		ov.KBase = &v
	}
	if v, ok := num(m["alpha"]); ok {
		ov.Alpha = &v
	}
	if v, ok := str(m["n_crit_mode"]); ok {
		ov.NCritMode = &v
	}
	if v, ok := num(m["n_crit"]); ok {
		ov.NCrit = &v
	}
	if v, ok := num(m["n_dead"]); ok {
		ov.NDead = &v
	}
	if v, ok := num(m["t0"]); ok {
		ov.T0 = &v
	}
	if v, ok := str(m["hg_mode"]); ok {
		ov.HgMode = &v
	}
	if v, ok := num(m["t_min"]); ok {
		ov.TMin = &v
	}
	if v, ok := num(m["t_max"]); ok {
		ov.TMax = &v
	}
	if arr, ok := m["atmo_penalty"].([]interface{}); ok && len(arr) == 3 {
		var a [3]float64
		for i, item := range arr {
			if f, ok := item.(float64); ok {
				a[i] = f
			}
		}
		ov.AtmoPenalty = &a
	}
	if v, ok := num(m["water_threshold"]); ok {
		ov.WaterThreshold = &v
	}
	if v, ok := num(m["water_penalty"]); ok {
		ov.WaterPenalty = &v
	}
	return ov
}

func num(v interface{}) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func str(v interface{}) (string, bool) {
	s, ok := v.(string)
	return s, ok
}