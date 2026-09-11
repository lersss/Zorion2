// internal/generator/planet/utils_color.go
package planet

// hslToRgb преобразует цвет из формата HSL в RGB
// h: 0-360, s: 0-100, l: 0-100
// Возвращает три uint8 компонента (R, G, B)
func hslToRgb(h, s, l int) (uint8, uint8, uint8) {
	H := float64(h) / 360.0
	S := float64(s) / 100.0
	L := float64(l) / 100.0
	var r, g, b float64
	if S == 0 {
		r, g, b = L, L, L
	} else {
		var hue2rgb func(p, q, t float64) float64
		hue2rgb = func(p, q, t float64) float64 {
			if t < 0 {
				t += 1
			}
			if t > 1 {
				t -= 1
			}
			if t < 1.0/6.0 {
				return p + (q-p)*6*t
			}
			if t < 1.0/2.0 {
				return q
			}
			if t < 2.0/3.0 {
				return p + (q-p)*(2.0/3.0-t)*6
			}
			return p
		}
		q := 0.0
		if L < 0.5 {
			q = L * (1 + S)
		} else {
			q = L + S - L*S
		}
		p := 2*L - q
		r = hue2rgb(p, q, H+1.0/3.0)
		g = hue2rgb(p, q, H)
		b = hue2rgb(p, q, H-1.0/3.0)
	}
	return uint8(r * 255), uint8(g * 255), uint8(b * 255)
}

// hslPal — удобный вариант hslToRgb с возвратом [3]float64 для арифметики.
func hslPal(h, s, l int) [3]float64 {
	r, g, b := hslToRgb(h, s, l)
	return [3]float64{float64(r), float64(g), float64(b)}
}