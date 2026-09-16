// internal/races/suitable.go — пригодность планеты для расы.
package races

// minWaterPercentForLiquidWater — минимальная фактическая вода для рас с
// liquid_water: true (наследие пресета поселений «минимальная_вода: 10»,
// скрыт 65a). Флаг liquid_water_possible — физическая возможность жидкой
// воды при (T, P), а не фактическая вода: «сухая» гидросфера при флаге
// true даёт water_percent 0 — раса с требованием жидкой воды там не живёт.
const minWaterPercentForLiquidWater = 10.0

// Suitable — пригодность планеты для расы (спека §16 п.4): конъюнкция
// surv-окон по всем осям карточки + атмосфера (need/poison по composition)
// + liquid_water (если задан). Оси читаются из planet.data (JSONB):
// temperature, atmosphere_data.pressure_atm, atmosphere_data.composition
// (газ → %, сумма 100), core.radioactivity, core.heat_flux_w_m2, gravity,
// liquid_water_possible, water_percent (для liquid_water: true).
//
// Отсутствующая ось = «не влияет» (полный диапазон): проверка по ней
// пропускается. Единый источник пригодности для людей (65a): флаг
// Settleable в данных планет и тег inhabited считаются через
// HumansSuitable, а не через пресет поселений (скрыт 65a).
func (r *Race) Suitable(data map[string]interface{}) bool {
	if v, ok := floatAtPath(data, "temperature"); ok {
		if !r.Conditions.Temperature.Surv.Contains(v) {
			return false
		}
	}
	if v, ok := floatAtPath(data, "atmosphere_data.pressure_atm"); ok {
		if !r.Conditions.Pressure.Surv.Contains(v) {
			return false
		}
	}
	if v, ok := floatAtPath(data, "core.radioactivity"); ok {
		if !r.Conditions.Radiation.Surv.Contains(v) {
			return false
		}
	}
	if r.Conditions.HeatFlux != nil {
		if v, ok := floatAtPath(data, "core.heat_flux_w_m2"); ok {
			if !r.Conditions.HeatFlux.Surv.Contains(v) {
				return false
			}
		}
	}
	if r.Conditions.Gravity != nil {
		if v, ok := floatAtPath(data, "gravity"); ok {
			if !r.Conditions.Gravity.Surv.Contains(v) {
				return false
			}
		}
	}
	if r.Conditions.LiquidWater != nil {
		if v, ok := boolAtPath(data, "liquid_water_possible"); ok {
			if v != *r.Conditions.LiquidWater {
				return false
			}
		}
		// Требование жидкой воды — это и фактическая вода (water_percent),
		// а не только физическая возможность флага (65a).
		if *r.Conditions.LiquidWater {
			if v, ok := floatAtPath(data, "water_percent"); ok {
				if v < minWaterPercentForLiquidWater {
					return false
				}
			}
		}
	}

	// Атмосфера: need (газ ≥ min%) и poison (газ ≤ max%) по composition.
	// Состав отсутствует — атмосфера «не влияет».
	if comp, ok := compositionPct(data); ok {
		for gas, min := range r.Conditions.Atmosphere.Need {
			if comp[gas] < min {
				return false
			}
		}
		for gas, max := range r.Conditions.Atmosphere.Poison {
			if comp[gas] > max {
				return false
			}
		}
	}
	return true
}

// HumansSuitable — пригодность планеты для людей (раса humans, карточка
// config/races.json). Единый источник пригодности для людей (65a): флаг
// Settleable в данных планет (cascade.go) и тег inhabited (descriptions)
// считаются через него вместо пресета поселений (скрыт 65a).
func HumansSuitable(data map[string]interface{}) bool {
	h := ByID("humans")
	if h == nil {
		return false
	}
	return h.Suitable(data)
}

// floatAtPath — число по dot-пути ("core.radioactivity" → data["core"]
// ["radioactivity"]); ok=false — ось отсутствует или не число.
func floatAtPath(data map[string]interface{}, key string) (float64, bool) {
	v, ok := valueAtPath(data, key)
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// boolAtPath — bool по dot-пути; ok=false — ось отсутствует.
func boolAtPath(data map[string]interface{}, key string) (bool, bool) {
	v, ok := valueAtPath(data, key)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// compositionPct — состав атмосферы (газ → %) из planet.data;
// ok=false — данных нет (атмосфера «не влияет»).
func compositionPct(data map[string]interface{}) (map[string]float64, bool) {
	v, ok := valueAtPath(data, "atmosphere_data.composition")
	if !ok {
		return nil, false
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	out := make(map[string]float64, len(m))
	for gas, val := range m {
		if f, ok := val.(float64); ok {
			out[gas] = f
		}
	}
	return out, true
}

// valueAtPath — значение по dot-ключу. Плоские ключи читаются как раньше;
// отсутствующий промежуточный map даёт отсутствующее значение.
func valueAtPath(data map[string]interface{}, key string) (interface{}, bool) {
	i := indexByte(key, '.')
	if i < 0 {
		v, ok := data[key]
		return v, ok
	}
	head, tail := key[:i], key[i+1:]
	if child, ok := data[head].(map[string]interface{}); ok {
		return valueAtPath(child, tail)
	}
	return nil, false
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}