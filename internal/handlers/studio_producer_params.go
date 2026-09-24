// internal/handlers/studio_producer_params.go
// Типизированные поля редактора стадии у PUT /studio/api/producers/{id}
// (спека 2026-09-23-стадии-поселения-и-скорость-производства §10, итерация И1):
// eat (позиция → норма, ед/сутки/млрд), effects (позиция → имя типа эффекта),
// stage (пороги {enter, exit}). Всё ложится в params — приоритет полей M6:
// при одновременной передаче строки params и типизированных полей побеждают
// типизированные (ключи eat/effects/stage перезаписываются целиком, без слияния
// по позициям); остальные ключи params сохраняются. Только строка params без
// типизированных — поведение как раньше, признак eat_units не ставится; при
// типизированном eat ставится признак params.eat_units (§2.5).
package handlers

import (
	"encoding/json"
	"sort"

	"zorion/internal/repository"
)

// stageParamsInput — пороги стадии в params.stage (в людях, спека §4.3/§10):
// enter — порог входа, exit — порог выхода. nil-поле не пишется.
type stageParamsInput struct {
	Enter *float64 `json:"enter,omitempty"`
	Exit  *float64 `json:"exit,omitempty"`
}

// buildTypedProducerParams — сборка итогового params с типизированными полями
// (спека §10). base — строка params из тела (может быть nil) или текущий params
// типа; типизированные ключи перезаписываются целиком. Возвращает JSON-строку
// (для передачи в UpdateProducerType — единственная точка записи params) и
// предупреждения (валидация §10: позиция в eat без эффекта — не 422).
func (h *StudioHandlers) buildTypedProducerParams(id int64, raw *string, eat *map[string]float64, effects *map[string]string, stage *stageParamsInput) (string, []string, error) {
	base := map[string]json.RawMessage{}
	// База: строка params из тела (приоритет M6) или текущий params из БД.
	if raw != nil {
		if err := json.Unmarshal([]byte(*raw), &base); err != nil {
			return "", nil, &repository.ErrCatalog{Status: 400, Msg: "params — невалидный JSON"}
		}
	} else {
		cur, err := h.repo.ProducerParamsRaw(id)
		if err != nil {
			return "", nil, err
		}
		if len(cur) > 0 {
			if err := json.Unmarshal(cur, &base); err != nil {
				return "", nil, &repository.ErrCatalog{Status: 500, Msg: "params типа — невалидный JSON"}
			}
		}
	}

	// Валидация (сервер — источник истины, 422; спека 2026-09-24 §9.2): позиции —
	// в goods по name_norm (позиция = ТОВАР, категория как позиция снята),
	// типы эффектов — в effect_types по name_norm, нормы ≥ 0, пороги
	// отрицательные или без зазора (exit ≥ enter) — ошибка: требуется
	// exit < enter (зазор гистерезиса, §4.3).
	positions, err := h.repo.GoodNameNorms()
	if err != nil {
		return "", nil, err
	}
	if eat != nil {
		for pos, norm := range *eat {
			if !positions[pos] {
				return "", nil, &repository.ErrCatalog{Status: 422, Msg: "позиция «" + pos + "» отсутствует в товарах — выберите товар"}
			}
			if norm < 0 {
				return "", nil, &repository.ErrCatalog{Status: 422, Msg: "норма позиции «" + pos + "» < 0"}
			}
		}
	}
	effectNames, err := h.effectTypeNameNorms()
	if err != nil {
		return "", nil, err
	}
	if effects != nil {
		for pos, typeName := range *effects {
			if !positions[pos] {
				return "", nil, &repository.ErrCatalog{Status: 422, Msg: "позиция «" + pos + "» отсутствует в товарах — выберите товар"}
			}
			if !effectNames[typeName] {
				return "", nil, &repository.ErrCatalog{Status: 422, Msg: "тип эффекта «" + typeName + "» не найден"}
			}
		}
	}
	if stage != nil {
		if stage.Enter != nil && *stage.Enter < 0 {
			return "", nil, &repository.ErrCatalog{Status: 422, Msg: "порог входа < 0"}
		}
		if stage.Exit != nil && *stage.Exit < 0 {
			return "", nil, &repository.ErrCatalog{Status: 422, Msg: "порог выхода < 0"}
		}
		if stage.Enter != nil && stage.Exit != nil && *stage.Exit >= *stage.Enter {
			return "", nil, &repository.ErrCatalog{Status: 422, Msg: "порог выхода должен быть меньше порога входа (зазор гистерезиса)"}
		}
	}

	if eat != nil {
		b, err := json.Marshal(*eat)
		if err != nil {
			return "", nil, err
		}
		base["eat"] = b
		// Признак единицы (§2.5): только типизированная правка eat отмечает, что
		// значения уже в «ед/сутки/млрд» (ручная строка params признак не ставит).
		base["eat_units"] = json.RawMessage(`"per_day_per_billion"`)
	}
	if effects != nil {
		b, err := json.Marshal(*effects)
		if err != nil {
			return "", nil, err
		}
		base["effects"] = b
	}
	if stage != nil {
		b, err := json.Marshal(stage)
		if err != nil {
			return "", nil, err
		}
		base["stage"] = b
	}

	out, err := json.Marshal(base)
	if err != nil {
		return "", nil, err
	}
	return string(out), typedParamsWarnings(base), nil
}

// typedParamsWarnings — предупреждения (§8.2/§10): позиция объявлена в eat, но
// её нет в effects — мир её не потребляет (мёртвый ключ нормы). Не 422.
func typedParamsWarnings(base map[string]json.RawMessage) []string {
	finalEat := map[string]float64{}
	finalEffects := map[string]string{}
	if raw, ok := base["eat"]; ok {
		_ = json.Unmarshal(raw, &finalEat)
	}
	if raw, ok := base["effects"]; ok {
		_ = json.Unmarshal(raw, &finalEffects)
	}
	var warnings []string
	for pos := range finalEat {
		if _, ok := finalEffects[pos]; !ok {
			warnings = append(warnings, "позиция «"+pos+"» без эффекта — не потребляется")
		}
	}
	sort.Strings(warnings)
	return warnings
}

// effectTypeNameNorms — словарь name_norm типов эффектов (валидация positions
// effects редактора стадии, §10).
func (h *StudioHandlers) effectTypeNameNorms() (map[string]bool, error) {
	list, err := h.effects.EffectTypes()
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(list))
	for _, e := range list {
		out[e.NameNorm] = true
	}
	return out, nil
}
