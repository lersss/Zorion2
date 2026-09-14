// internal/generator/ship/svg.go
// Эмиссия SVG-фрагмента детали (спека §3.3): основная масса —
// fill="currentColor" (цвет приходит из схемы, инвариант И1), обводка —
// фиксированная #0b1220 шириной 3 px у всех деталей и акцентных элементов
// (стиль §3.2 п.1, «тёмная обводка акцента обязательна» §4), фон отсутствует.
package ship

import (
	"fmt"
	"strings"
)

// SVG — фрагмент детали (без <svg>-обёртки) в холсте 200×200.
func (g Geometry) SVG(cfg *Config) string {
	st := cfg.Style
	stroke := cfg.Accents.Stroke
	var sb strings.Builder
	for _, c := range g.Components {
		sb.WriteString(`<path d="`)
		sb.WriteString(pathD(c.Poly))
		sb.WriteString(`" fill="currentColor" stroke="`)
		sb.WriteString(stroke)
		sb.WriteString(`" stroke-width="`)
		sb.WriteString(num(st.StrokeWidth))
		sb.WriteString(`"/>`)
	}
	for _, a := range g.Accents {
		sb.WriteString(accentSVG(a, stroke, st))
	}
	return sb.String()
}

// accentSVG — акцентный элемент с явным fill (не currentColor) и общей
// обводкой; скругление — accentRadius (стиль §3.2 п.6).
func accentSVG(a Accent, stroke string, st Style) string {
	w := num(st.StrokeWidth)
	switch a.Kind {
	case "circle":
		return fmt.Sprintf(
			`<circle cx="%s" cy="%s" r="%s" fill="%s" stroke="%s" stroke-width="%s"/>`,
			num(a.Center.X), num(a.Center.Y), num(a.Radius), a.Color, stroke, w)
	case "rect":
		return fmt.Sprintf(
			`<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="%s" stroke="%s" stroke-width="%s"/>`,
			num(a.Rect.X), num(a.Rect.Y), num(a.Rect.W), num(a.Rect.H),
			num(st.AccentRadius), a.Color, stroke, w)
	}
	return ""
}