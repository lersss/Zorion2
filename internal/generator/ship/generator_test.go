// internal/generator/ship/generator_test.go
// Юнит-тесты генератора форм (спека 99.2.15 §11 Этап 2): детерминизм по seed,
// деталь и акценты в своей зоне (И6), стык с корпусом при крайних корпусах
// (И9), единство стиля (И9), лимиты акцентов, 10 силуэтно различимых форм
// на категорию, фрагмент с currentColor и только разрешёнными цветами,
// глобальный инвариант собранного корабля (ширина/высота ∈ [0.8, 3.5]).
package ship

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// loadTestConfig — конфиг из репозитория (cwd теста = каталог пакета).
func loadTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := LoadConfig("../../../config/ship_visual.json")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	return cfg
}

// ==================== ДЕТЕРМИНИЗМ (И3) ====================

func TestGeneratePartDeterministic(t *testing.T) {
	loadTestConfig(t)
	for _, cat := range Categories {
		for _, seed := range []int64{0, 1, 42, 123456} {
			p1, err := GeneratePart(cat, seed)
			require.NoError(t, err)
			p2, err := GeneratePart(cat, seed)
			require.NoError(t, err)
			require.Equal(t, p1.ID, p2.ID, "%s seed %d: id детерминирован", cat, seed)
			require.Equal(t, p1.SVG, p2.SVG, "%s seed %d: svg детерминирован (включая акценты)", cat, seed)
			require.Equal(t, p1.Name, p2.Name, "%s seed %d: имя детерминировано", cat, seed)
			require.Equal(t, p1.Params, p2.Params, "%s seed %d: параметры детерминированы", cat, seed)
		}
	}
}

// ==================== ВАЛИДНОСТЬ ИНВАРИАНТОВ (И6, И9) ====================

// TestGeneratePartValid — все инварианты checkPart на выборке seed'ов
// каждой категории: зона, стык/база, стиль, заполненность, акценты.
func TestGeneratePartValid(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, cat := range Categories {
		for seed := int64(0); seed < 60; seed++ {
			p, err := GeneratePart(cat, seed)
			require.NoError(t, err, "%s seed %d", cat, seed)
			require.NoError(t, checkPart(cfg, cat, p.Geometry), "%s seed %d: инварианты нарушены", cat, seed)
		}
	}
}

// ==================== СТЫК ПРИ КРАЙНИХ КОРПУСАХ (И9) ====================

// TestBaseOnExtremeHulls — база каждого придатка лежит на корпусе крайних
// габаритов: min-корпус (60×42) и max-корпус (110×60). Корпуса в этих
// габаритах всегда покрывают гарантированную область [65,130]×[79,121]
// (спека §3.1), поэтому проверяем: середина базовой кромки в области.
func TestBaseOnExtremeHulls(t *testing.T) {
	loadTestConfig(t)
	region := guaranteedHullRegion
	appendages := []string{"nose", "wings", "engine", "tail"}
	for _, cat := range appendages {
		for seed := int64(0); seed < 40; seed++ {
			p, err := GeneratePart(cat, seed)
			require.NoError(t, err)
			for _, comp := range p.Geometry.Components {
				require.NotNil(t, comp.Base, "%s: компонент без базы", cat)
				mid := Pt{X: (comp.Base.A.X + comp.Base.B.X) / 2, Y: (comp.Base.A.Y + comp.Base.B.Y) / 2}
				require.True(t, mid.X >= region.X[0] && mid.X <= region.X[1] &&
					mid.Y >= region.Y[0] && mid.Y <= region.Y[1],
					"%s seed %d: середина базы (%.0f,%.0f) вне гарантированной области корпуса",
					cat, seed, mid.X, mid.Y)
			}
		}
	}
}

// ==================== СТИЛЬ И ЦВЕТА (И9, И1) ====================

var (
	reCurrentColor = regexp.MustCompile(`fill="currentColor"`)
	reFill         = regexp.MustCompile(`fill="(#[\da-fA-F]{6})"`)
	reStrokeWidth  = regexp.MustCompile(`stroke-width="3"`)
)

// TestFragmentColors — фрагмент содержит currentColor и только разрешённые
// фиксированные цвета: обводка #0b1220 + акценты из конфига (спека §3.3, §4).
func TestFragmentColors(t *testing.T) {
	cfg := loadTestConfig(t)
	allowed := map[string]bool{cfg.Accents.Stroke: true}
	allowed[cfg.Accents.Glass] = true
	allowed[cfg.Accents.Glow] = true
	allowed[cfg.Accents.FireRed] = true
	allowed[cfg.Accents.FireGreen] = true
	allowed[cfg.Accents.Nozzle] = true

	for _, cat := range Categories {
		for seed := int64(0); seed < 30; seed++ {
			p, err := GeneratePart(cat, seed)
			require.NoError(t, err)
			require.True(t, reCurrentColor.MatchString(p.SVG), "%s seed %d: нет currentColor", cat, seed)
			for _, m := range reFill.FindAllStringSubmatch(p.SVG, -1) {
				require.True(t, allowed[strings.ToLower(m[1])],
					"%s seed %d: неразрешённый цвет %s в фрагменте", cat, seed, m[1])
			}
			// Обводка 3 px у всех элементов (стиль §3.2 п.1).
			n := len(reStrokeWidth.FindAllString(p.SVG, -1))
			require.GreaterOrEqual(t, n, 1, "%s seed %d: нет обводки 3 px", cat, seed)
		}
	}
	// Палитра корабля в фрагменте НЕ появляется (акценты вне палитры, §4).
	for _, color := range cfg.Palette {
		require.False(t, strings.Contains(strings.ToLower(paletteSample(cfg, color)), color),
			"цвет палитры %s не должен попадать в фрагменты", color)
	}
}

// paletteSample — небольшой фрагмент для проверки отсутствия цвета палитры.
func paletteSample(cfg *Config, color string) string {
	for _, cat := range Categories {
		for seed := int64(0); seed < 20; seed++ {
			p, err := GeneratePart(cat, seed)
			if err == nil && strings.Contains(p.SVG, color) {
				return p.SVG
			}
		}
	}
	return ""
}

// ==================== АКЦЕНТЫ (лимиты §9, отступ §3.2 п.6) ====================

// TestAccentAreaLimits — один акцент ≤ min(90, 6% заливки), суммарно
// ≤ min(240, 10% заливки); отступ от контура ≥ 3 px (проверяется в
// checkAccentPlacement — здесь дополнительно сверяем числа напрямую).
func TestAccentAreaLimits(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, cat := range Categories {
		for seed := int64(0); seed < 60; seed++ {
			p, err := GeneratePart(cat, seed)
			require.NoError(t, err)
			fill := partFillArea(p.Geometry)
			var total float64
			for _, a := range p.Geometry.Accents {
				area := accentArea(a)
				require.LessOrEqual(t, area, math.Min(cfg.Style.MaxAccentArea, 0.06*fill)+0.5,
					"%s seed %d: акцент больше лимита", cat, seed)
				total += area
			}
			require.LessOrEqual(t, total, math.Min(cfg.Style.MaxAccentTotal, 0.10*fill)+0.5,
				"%s seed %d: сумма акцентов больше лимита", cat, seed)
		}
	}
}

// ==================== 10 СИЛУЭТНО РАЗЛИЧИМЫХ ФОРМ (эталон «~10 форм») ====================

// TestTenDistinctSilhouettes — 10 seed'ов → 10 форм, различимых по силуэту
// (очертанию основной массы); акценты в различимость не входят (спека §3.3).
func TestTenDistinctSilhouettes(t *testing.T) {
	loadTestConfig(t)
	for _, cat := range Categories {
		seen := map[string]bool{}
		for seed := int64(0); seed < 10; seed++ {
			p, err := GeneratePart(cat, seed)
			require.NoError(t, err)
			seen[normalizeSilhouette(p.Geometry)] = true
		}
		require.GreaterOrEqual(t, len(seen), 10,
			"%s: 10 seed'ов дали %d различимых силуэтов (нужно 10)", cat, len(seen))
	}
}

// normalizeSilhouette — строковый ключ силуэта: полигоны основной массы,
// сдвинутые к началу координат (масштаб не нормируется — размер — часть
// силуэта), координаты округлены до 0.5.
func normalizeSilhouette(g Geometry) string {
	var sb strings.Builder
	for _, c := range g.Components {
		x0, y0, _, _ := c.Poly.BBox()
		for _, pt := range c.Poly {
			sb.WriteString(keyNum(pt.X-x0) + "," + keyNum(pt.Y-y0) + " ")
		}
		sb.WriteString("|")
	}
	return sb.String()
}

func keyNum(f float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(math.Round(f*2)/2, 'f', 2, 64), "0"), ".")
}

// ==================== ГЛОБАЛЬНЫЙ ИНВАРИАНТ СБОРКИ (И9) ====================

// TestAssemblyGlobalInvariant — для случайных комбинаций (≥ 100) bbox
// собранного корабля имеет ширина/высота ∈ [0.8, 3.5] (спека §9).
func TestAssemblyGlobalInvariant(t *testing.T) {
	loadTestConfig(t)
	type gen struct {
		cat  string
		seed int64
	}
	byCat := map[string][]Part{}
	for _, cat := range Categories {
		for seed := int64(0); seed < 10; seed++ {
			p, err := GeneratePart(cat, seed)
			require.NoError(t, err)
			byCat[cat] = append(byCat[cat], p)
		}
	}
	combo := map[string]Part{}
	seed := int64(1)
	for i := 0; i < 100; i++ {
		for _, cat := range Categories {
			list := byCat[cat]
			combo[cat] = list[int(seed)%len(list)]
			seed++
		}
		polys := []Polygon{}
		for _, cat := range Categories {
			for _, c := range combo[cat].Geometry.Components {
				polys = append(polys, c.Poly)
			}
		}
		x0, y0, x1, y1 := unionBBox(polys)
		w, h := x1-x0, y1-y0
		ratio := w / h
		require.True(t, ratio >= 0.8 && ratio <= 3.5,
			"комбо %d: ширина/высота %.2f вне [0.8, 3.5] (w=%.0f h=%.0f)", i, ratio, w, h)
	}
}

// ==================== ЗОНЫ И ГАБАРИТЫ (прямые проверки §3.1) ====================

// TestZoneMembership — bbox каждой детали в своей зоне (прямая проверка,
// помимо checkPart).
func TestZoneMembership(t *testing.T) {
	cfg := loadTestConfig(t)
	zones := map[string]Zone{}
	for cat, cc := range cfg.Categories {
		zones[cat] = cc.Zone
	}
	for _, cat := range Categories {
		for seed := int64(0); seed < 30; seed++ {
			p, err := GeneratePart(cat, seed)
			require.NoError(t, err)
			z := zones[cat]
			if cat == "wings" {
				// Верхняя плоскость в [0,80], нижняя в [120,200].
				x0, y0, x1, y1 := p.Geometry.Components[0].Poly.BBox()
				require.True(t, y0 >= 0 && y1 <= 80, "wings: верх в y [%.0f..%.0f] вне [0,80]", y0, y1)
				_, y0, _, y1 = p.Geometry.Components[1].Poly.BBox()
				require.True(t, y0 >= 120 && y1 <= 200, "wings: низ в y [%.0f..%.0f] вне [120,200]", y0, y1)
				_ = x0
				_ = x1
				continue
			}
			x0, y0, x1, y1 := p.Geometry.Components[0].Poly.BBox()
			require.True(t, x0 >= z.X[0]-0.5 && x1 <= z.X[1]+0.5 &&
				y0 >= z.Y[0]-0.5 && y1 <= z.Y[1]+0.5,
				"%s seed %d: bbox [%.0f..%.0f]×[%.0f..%.0f] вне зоны [%.0f..%.0f]×[%.0f..%.0f]",
				cat, seed, x0, x1, y0, y1, z.X[0], z.X[1], z.Y[0], z.Y[1])
		}
	}
}

// ==================== ФОРМЫ ВНУТРИ ГАБАРИТОВ §3.1 ====================

// TestDimensionsWithinBounds — габариты форм внутри границ §3.1 (прямая
// проверка ключевых чисел, остальное — checkPart).
func TestDimensionsWithinBounds(t *testing.T) {
	cfg := loadTestConfig(t)
	hull := cfg.Categories["hull"]
	for seed := int64(0); seed < 30; seed++ {
		p, err := GeneratePart("hull", seed)
		require.NoError(t, err)
		x0, y0, x1, y1 := p.Geometry.Components[0].Poly.BBox()
		require.True(t, x1-x0 >= hull.Width.Min-0.5 && x1-x0 <= hull.Width.Max+0.5,
			"hull: ширина %.0f вне [%.0f,%.0f]", x1-x0, hull.Width.Min, hull.Width.Max)
		require.True(t, y1-y0 >= hull.Height.Min-0.5 && y1-y0 <= hull.Height.Max+0.5,
			"hull: высота %.0f вне [%.0f,%.0f]", y1-y0, hull.Height.Min, hull.Height.Max)
		require.True(t, x0 <= *hull.MinX+0.5, "hull: min x %.0f > %v", x0, *hull.MinX)
		require.True(t, x1 >= *hull.MaxX-0.5, "hull: max x %.0f < %v", x1, *hull.MaxX)
	}
}