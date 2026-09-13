// internal/generator/settlement/model.go
//
// Модель генерации поселений — задаётся в админке и управляет и выбором
// планет, и расчётом населения. Два режима:
//
//	simple  — обход всех планет без условий по полям: только шанс + население;
//	complex — правила по полям planet.data (терапевтический AND), шаблоны
//	          строк «линейно и т.п.» — следующий этап (в этом срезе не собран).
//
// «Как работает» должно быть видно из админки: реестр полей (fields.go)
// отдаёт типы, экстремумы и допустимые значения для формы правил.
package settlement

import (
	"fmt"
	"math/rand"
)

// Mode — режим модели.
type Mode string

const (
	ModeSimple  Mode = "simple"
	ModeComplex Mode = "complex"
)

// Population — как заполняется население поселения.
type Population struct {
	Kind  string `json:"kind"` // "fixed" | "random"
	Fixed int    `json:"fixed,omitempty"`
	Min   int    `json:"min,omitempty"`
	Max   int    `json:"max,omitempty"`
}

// FieldRule — правило «поле планеты → условие». Работает по типу поля:
//
//	number  — Min/Max (нижняя/верхняя границы, nil = без границы);
//	string  — In/NotIn (разрешённые / запрещённые значения);
//	bool    — Is (требуемое значение).
type FieldRule struct {
	Field string    `json:"field"`
	Min   *float64  `json:"min,omitempty"`
	Max   *float64  `json:"max,omitempty"`
	In    []string  `json:"in,omitempty"`
	NotIn []string  `json:"not_in,omitempty"`
	Is    *bool     `json:"is,omitempty"`
}

// Model — модель генерации поселений.
type Model struct {
	Mode       Mode        `json:"mode"`
	Chance     float64     `json:"chance"` // 0..1, шанс заселения
	Population Population  `json:"population"`
	Rules      []FieldRule `json:"rules,omitempty"`
}

// DefaultModel — значения по умолчанию, совпадают с прежним пресетом
// пригодности (config/settlement_preset.json), переведённым на правила.
func DefaultModel() *Model {
	return &Model{
		Mode:   ModeComplex,
		Chance: 1.0,
		Population: Population{
			Kind: "random",
			Min:  100_000,
			Max:  1_000_000_000,
		},
		Rules: []FieldRule{
			{Field: "temperature", Min: floatPtr(200), Max: floatPtr(350)},
			{Field: "water_percent", Min: floatPtr(10)},
			{Field: "atmosphere", NotIn: []string{"ядовитая"}},
			{Field: "is_gas_giant", Is: boolPtr(false)},
			{Field: "radioactive", Is: boolPtr(false)},
		},
	}
}

// Validate — проверяет модель перед генерацией.
func (m *Model) Validate() error {
	if m.Mode != ModeSimple && m.Mode != ModeComplex {
		return fmt.Errorf("mode: ожидается %q или %q, получил %q", ModeSimple, ModeComplex, m.Mode)
	}
	if m.Chance < 0 || m.Chance > 1 {
		return fmt.Errorf("chance: должно быть от 0 до 1, получил %v", m.Chance)
	}

	p := m.Population
	switch p.Kind {
	case "fixed":
		if p.Fixed <= 0 {
			return fmt.Errorf("population.fixed: должно быть > 0, получил %d", p.Fixed)
		}
	case "random":
		if p.Min <= 0 || p.Max < p.Min {
			return fmt.Errorf("population.random: нужно 0 < min <= max, получил %d..%d", p.Min, p.Max)
		}
	default:
		return fmt.Errorf("population.kind: ожидается \"fixed\" или \"random\", получил %q", p.Kind)
	}

	if m.Mode == ModeComplex {
		for i, r := range m.Rules {
			if err := validateRule(i, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRule(i int, r FieldRule) error {
	spec, ok := FieldSpecFor(r.Field)
	if !ok {
		return fmt.Errorf("rules[%d].field: неизвестное поле %q", i, r.Field)
	}

	switch spec.Type {
	case FieldNumber:
		if r.Min != nil && r.Max != nil && *r.Min > *r.Max {
			return fmt.Errorf("rules[%d]: min > max", i)
		}
	case FieldString:
		if len(r.In) > 0 && len(r.NotIn) > 0 {
			return fmt.Errorf("rules[%d]: in и not_in вместе не задаются", i)
		}
		for _, v := range append(append([]string{}, r.In...), r.NotIn...) {
			if !spec.HasValue(v) {
				return fmt.Errorf("rules[%d]: значение %q не из допустимых для %s (%s)",
					i, v, r.Field, spec.Label)
			}
		}
	case FieldBool:
		if r.Is == nil {
			return fmt.Errorf("rules[%d]: для bool-поля %s задай is", i, r.Field)
		}
	}
	return nil
}

// Matches — проходит ли планета правила модели (только complex; в simple
// режиме условия не задаются, планеты отбираются только по шансу).
func (m *Model) Matches(data map[string]interface{}) bool {
	if m.Mode != ModeComplex {
		return true
	}
	for _, r := range m.Rules {
		if !r.matches(data) {
			return false
		}
	}
	return true
}

func (r FieldRule) matches(data map[string]interface{}) bool {
	switch fieldType(r.Field) {
	case FieldNumber:
		v := getFloat(data, r.Field)
		if r.Min != nil && v < *r.Min {
			return false
		}
		if r.Max != nil && v > *r.Max {
			return false
		}
	case FieldString:
		v := getString(data, r.Field)
		if len(r.In) > 0 && !containsString(r.In, v) {
			return false
		}
		if containsString(r.NotIn, v) {
			return false
		}
	case FieldBool:
		v := getBool(data, r.Field)
		// is_gas_giant не всегда сохранён как ключ data — выводится по
		// surface_dominant (газовый_гигант), см. isGasGiant.
		if r.Field == "is_gas_giant" {
			v = isGasGiant(data)
		}
		if r.Is != nil && v != *r.Is {
			return false
		}
	default:
		return true
	}
	return true
}

// value — население по стратегии модели.
func (p Population) value(rng *rand.Rand) int {
	if p.Kind == "fixed" {
		return p.Fixed
	}
	return p.Min + rng.Intn(p.Max-p.Min+1)
}

func floatPtr(f float64) *float64 { return &f }
func boolPtr(b bool) *bool        { return &b }

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}