// internal/generator/planet/gas_giant_registry_test.go
//
// Инвариант «рамки реестра == границы генератора» (99.2.15 §4, §7.2): форма
// близнецов обязана принимать каждое значение, которое генератор способен
// выдать (покрытие ⊇, рамки округлены наружу). Плюс §7.3: числовые значения
// пресетов hypothesisPresets.js валидны в новых рамках.
//
// Тест в пакете planet (импортирует settlement — цикл исключён).
package planet

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/generator/settlement"
)

// TestFieldRegistryCoversGenerator — рамки реестра покрывают фактические
// диапазоны генератора (нижняя ≤ минимума, верхняя ≥ максимума).
func TestFieldRegistryCoversGenerator(t *testing.T) {
	reg := map[string]settlement.FieldSpec{}
	for _, s := range settlement.FieldRegistry() {
		reg[s.Key] = s
	}
	requireSpec := func(key string) settlement.FieldSpec {
		s, ok := reg[key]
		require.True(t, ok, "нет поля %q в реестре", key)
		require.NotNil(t, s.Min, "%s: нет min", key)
		require.NotNil(t, s.Max, "%s: нет max", key)
		return s
	}

	// temperature: 20…2500 K → −253…2227 °C (в реестре °C).
	s := requireSpec("temperature")
	assert.LessOrEqual(t, *s.Min, 20.0-273.0, "temperature min")
	assert.GreaterOrEqual(t, *s.Max, 2500.0-273.0, "temperature max")

	// mass: физический каскад 0.02–8 (пол 0.02 — страховка реестра, спека
	// 2026-09-21 §5.3), гиганты до GasGiantMassMax, экзотика 0.05–0.45.
	s = requireSpec("mass")
	assert.LessOrEqual(t, *s.Min, 0.02, "mass min")
	assert.GreaterOrEqual(t, *s.Max, GasGiantMassMax, "mass max")

	// size: страховка реестра от пола массы 0.02 → 0.25 (факт генератора
	// ≈ 0.43); максимум — плато насыщения гигантов 11.2.
	s = requireSpec("size")
	assert.LessOrEqual(t, *s.Min, 0.25, "size min")
	assert.GreaterOrEqual(t, *s.Max, GasGiantRadiusMax, "size max")

	// density: ρ(15.9)=0.0774 … ρ(4131)=2.9403.
	s = requireSpec("density")
	assert.LessOrEqual(t, *s.Min, GasGiantMassMin/math.Pow(GasGiantRadiusMin, 3), "density min")
	assert.GreaterOrEqual(t, *s.Max, GasGiantMassMax/math.Pow(GasGiantRadiusMax, 3), "density max")

	// gravity: страховка от пола массы 0.02 → 0.13 (факт генератора ≈ 0.33);
	// верх — g(4131, 11.2) = 32.93.
	s = requireSpec("gravity")
	assert.LessOrEqual(t, *s.Min, 0.13, "gravity min")
	assert.GreaterOrEqual(t, *s.Max, GasGiantMassMax/(GasGiantRadiusMax*GasGiantRadiusMax), "gravity max")

	// moons: 0…10 (гиганты 3–10, мини-нептуны 0–4, стандартные ≤ 4).
	s = requireSpec("moons")
	assert.LessOrEqual(t, *s.Min, 0.0, "moons min")
	assert.GreaterOrEqual(t, *s.Max, 10.0, "moons max")

	// Мини-нептун — новый класс (спека 2026-09-23 §11.2): поле-флаг в реестре,
	// значение типа в списке (иначе класс молча недоступен формам).
	if _, ok := reg["is_mini_neptune"]; !ok {
		t.Error("нет поля is_mini_neptune в реестре (спека 2026-09-23 §11.2)")
	}
	assert.Contains(t, reg["type"].Values, "мини-нептун", "тип «мини-нептун» в реестре")
	assert.Contains(t, reg["surface_dominant"].Values, "мини-нептун", "surface_dominant «мини-нептун»")

	// Остальные числовые поля.
	for _, tc := range []struct {
		key      string
		min, max float64
	}{
		{"water_percent", 0, 100},
		{"development_level", 0, 1},
		{"system_age", 0.1, 13},
		{"core.radioactivity", 0, 100},
	} {
		s = requireSpec(tc.key)
		assert.LessOrEqual(t, *s.Min, tc.min, "%s min", tc.key)
		assert.GreaterOrEqual(t, *s.Max, tc.max, "%s max", tc.key)
	}

	// Новые поля физического каскада (99.2.20 §6.4): рамки реестра покрывают
	// фактические диапазоны генератора.
	for _, tc := range []struct {
		key      string
		min, max float64
	}{
		{"atmosphere_data.pressure_atm", 0, 1000},
		{"orbital_period", 0.0007, 127000},
		{"escape_velocity", 4, 215},
		{"eccentricity", 0.03, 0.3},
	} {
		s = requireSpec(tc.key)
		assert.LessOrEqual(t, *s.Min, tc.min, "%s min", tc.key)
		assert.GreaterOrEqual(t, *s.Max, tc.max, "%s max", tc.key)
	}
}

// ==================== ПРЕСЕТЫ БЛИЗНЕЦОВ (§7.3) ====================

// Тест парсит hypothesisPresets.js лёгким сканером: JS-литералы имеют
// регулярную структуру (ключ: значение на строке), вложенность объектов
// не важна — проверяются все числовые пары с ключами реестра и axisValue
// групп по полю оси пресета.
var (
	// «ключ: число» в JS-объекте (без экспоненты).
	jsNumericPairRe = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(-?\d+(?:\.\d+)?)`)
	jsIDRe          = regexp.MustCompile(`id:\s*'([^']+)'`)
	jsAxisKeyRe     = regexp.MustCompile(`axis:\s*\{[^}]*key:\s*'([^']+)'`)
	jsAxisValueRe   = regexp.MustCompile(`axisValue:\s*(-?\d+(?:\.\d+)?)`)
)

// jsObjEnd — индекс закрывающей скобки для объекта/массива, открытого на
// start (учитывает строки '...' и "..." и вложенные скобки).
func jsObjEnd(src string, start int) int {
	depth := 0
	inStr := byte(0)
	for i := start; i < len(src); i++ {
		c := src[i]
		if inStr != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			inStr = c
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// stripJSComments — убирает однострочные //-комментарии (в пресетах других
// нет; скобок в комментариях нет — парсер их не ломает).
func stripJSComments(src string) string {
	lines := strings.Split(src, "\n")
	for i, ln := range lines {
		if j := strings.Index(ln, "//"); j >= 0 {
			lines[i] = ln[:j]
		}
	}
	return strings.Join(lines, "\n")
}

type presetGroupInfo struct {
	axisValue *float64
}

type presetInfo struct {
	id      string
	axisKey string
	groups  []presetGroupInfo
}

// extractPresets — пресеты из HYPOTHESIS_PRESETS: id, ключ оси, axisValue групп.
func extractPresets(t *testing.T, src string) []presetInfo {
	t.Helper()
	const marker = "export const HYPOTHESIS_PRESETS = ["
	start := strings.Index(src, marker)
	require.True(t, start >= 0, "не найден массив HYPOTHESIS_PRESETS")
	arrStart := strings.IndexByte(src[start:], '[') + start
	arrEnd := jsObjEnd(src, arrStart)
	require.True(t, arrEnd > arrStart, "массив HYPOTHESIS_PRESETS не закрыт")
	arrContent := src[arrStart : arrEnd+1]

	var presets []presetInfo
	i := strings.IndexByte(arrContent, '{')
	for i >= 0 {
		objEnd := jsObjEnd(arrContent, i)
		obj := arrContent[i : objEnd+1]
		presets = append(presets, parsePresetObject(obj))
		next := strings.IndexByte(arrContent[objEnd+1:], '{')
		if next < 0 {
			break
		}
		i = objEnd + 1 + next
	}
	return presets
}

func parsePresetObject(obj string) presetInfo {
	p := presetInfo{}
	if m := jsIDRe.FindStringSubmatch(obj); len(m) > 0 {
		p.id = m[1]
	}
	if m := jsAxisKeyRe.FindStringSubmatch(obj); len(m) > 0 {
		p.axisKey = m[1]
	}
	if gi := strings.Index(obj, "groups:"); gi >= 0 {
		br := strings.IndexByte(obj[gi:], '[')
		if br >= 0 {
			arrStart := gi + br
			arrEnd := jsObjEnd(obj, arrStart)
			groupsContent := obj[arrStart : arrEnd+1]
			i := strings.IndexByte(groupsContent, '{')
			for i >= 0 {
				gEnd := jsObjEnd(groupsContent, i)
				gObj := groupsContent[i : gEnd+1]
				var g presetGroupInfo
				if m := jsAxisValueRe.FindStringSubmatch(gObj); len(m) > 0 {
					if v, err := strconv.ParseFloat(m[1], 64); err == nil {
						g.axisValue = &v
					}
				}
				p.groups = append(p.groups, g)
				next := strings.IndexByte(groupsContent[gEnd+1:], '{')
				if next < 0 {
					break
				}
				i = gEnd + 1 + next
			}
		}
	}
	return p
}

func unitStr(s settlement.FieldSpec) string {
	if s.Unit == "" {
		return ""
	}
	return " " + s.Unit
}

func readPresetsJS(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "web", "static", "js", "admin", "hypothesisPresets.js")
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// TestHypothesisPresetsWithinRegistry — все числовые значения пресетов
// (канон + baked-оверрайды + axisValue) внутри рамок реестра. Температура в
// пресетах хранится в K — сверяется ПОСЛЕ конверсии −273 (без неё 700 K <
// 2227 °C прошло бы молча и тест потерял бы смысл).
func TestHypothesisPresetsWithinRegistry(t *testing.T) {
	src := stripJSComments(readPresetsJS(t))

	reg := map[string]settlement.FieldSpec{}
	for _, s := range settlement.FieldRegistry() {
		reg[s.Key] = s
	}
	// core.radioactivity в пресетах лежит вложенно (core: { radioactivity: N }).
	reg["radioactivity"] = reg["core.radioactivity"]

	var failures []string

	// Базы + baked-оверрайды групп: все числовые пары файла.
	for _, m := range jsNumericPairRe.FindAllStringSubmatch(src, -1) {
		key := m[1]
		spec, ok := reg[key]
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(m[2], 64)
		require.NoError(t, err)
		if key == "temperature" {
			val -= 273 // K → °C
		}
		if val < *spec.Min || val > *spec.Max {
			failures = append(failures, fmt.Sprintf("hypothesisPresets.js: %s = %v вне [%v, %v]%s",
				key, val, *spec.Min, *spec.Max, unitStr(spec)))
		}
	}

	// axisValue групп — по полю оси пресета (для температуры тоже K → °C).
	for _, p := range extractPresets(t, src) {
		spec, ok := reg[p.axisKey]
		if !ok {
			continue // ось — string/bool поле
		}
		for _, g := range p.groups {
			if g.axisValue == nil {
				continue
			}
			val := *g.axisValue
			if p.axisKey == "temperature" {
				val -= 273 // K → °C
			}
			if val < *spec.Min || val > *spec.Max {
				failures = append(failures, fmt.Sprintf("пресет %s: axisValue = %v вне [%v, %v]%s",
					p.id, val, *spec.Min, *spec.Max, unitStr(spec)))
			}
		}
	}

	assert.Empty(t, failures, strings.Join(failures, "\n"))
}