// internal/generator/planet/generator_lava.go
package planet

import (
	"image"
	"image/color"
	"math"
	"math/rand"
)

func generateLava(img *image.RGBA, size int, rng *rand.Rand) {
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	vn := newValueNoise(rng.Int63())

	// Палитра: чёрный базальт → тёмная кора → светящаяся лава
	basalt := hslPal(15+rng.Intn(10), 20+rng.Intn(10), 10+rng.Intn(6))
	crack := hslPal(25+rng.Intn(15), 30+rng.Intn(15), 22+rng.Intn(10))
	lavaDark := hslPal(15+rng.Intn(10), 100, 45+rng.Intn(10))
	lavaBright := hslPal(40+rng.Intn(15), 100, 60+rng.Intn(12))

	scale := 3.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > radius {
				continue
			}

			sx, sy, sz := spherePoint(dx, dy, radius, scale)
			sx, sy, sz = vn.warp(sx, sy, sz, 1.1)

			// Базальтовая кора с рельефом
			base := vn.fbm(sx, sy, sz, 5, 2.1, 0.5)
			// Трещины и жгуты лавы
			flow := vn.ridged(sx*2.0+7, sy*2.0-13, sz*2.0+29, 5, 2.1, 0.5)
			// Мелкая зернистость
			det := vn.fbm(sx*8+31, sy*8+7, sz*8-19, 3, 2.3, 0.5)

			var r, g, b float64
			if base < 0.45 {
				t := base / 0.45
				r = basalt[0] + (crack[0]-basalt[0])*t
				g = basalt[1] + (crack[1]-basalt[1])*t
				b = basalt[2] + (crack[2]-basalt[2])*t
			} else {
				t := (base - 0.45) / 0.55
				r = crack[0] + (basalt[0]-crack[0])*t
				g = crack[1] + (basalt[1]-crack[1])*t
				b = crack[2] + (basalt[2]-crack[2])*t
			}

			// Зернистость
			var dm float64 = (det - 0.5) * 0.06
			r += dm * 255
			g += dm * 255
			b += dm * 255

			// Лава в трещинах — жгуты с ярким сердцем и свечением по краям
			if flow > 0.30 {
				t := (flow - 0.30) / 0.70
				core := t*t*(3-2*t)*0.9 + 0.1
				if core > 1 {
					core = 1
				}
				if core < 0.5 {
					ct := core / 0.5
					r = r*(1-core) + (lavaDark[0]+(lavaBright[0]-lavaDark[0])*ct)*core
					g = g*(1-core) + (lavaDark[1]+(lavaBright[1]-lavaDark[1])*ct)*core
					b = b*(1-core) + (lavaDark[2]+(lavaBright[2]-lavaDark[2])*ct)*core
				} else {
					// Свечение вокруг, если рядом — добавим ореол
					halo := math.Max(0, 1-core) * 0.4
					r += lavaDark[0] * halo
					g += lavaDark[1] * halo
					b += lavaDark[2] * halo
				}
			}

			clamp := func(v float64) uint8 {
				if v < 0 {
					return 0
				}
				if v > 255 {
					return 255
				}
				return uint8(v)
			}
			img.SetRGBA(x, y, color.RGBA{clamp(r), clamp(g), clamp(b), 255})
		}
	}
}