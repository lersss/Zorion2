// internal/generator/planet/generator_ice.go
package planet

import (
	"image"
	"image/color"
	"math"
	"math/rand"
)

func generateIce(img *image.RGBA, size int, rng *rand.Rand) {
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	vn := newValueNoise(rng.Int63())

	// Палитра: белый лёд → голубой → трещины
	white := hslPal(210, 15+rng.Intn(10), 88+rng.Intn(8))
	paleBlue := hslPal(205+rng.Intn(12), 35+rng.Intn(15), 72+rng.Intn(10))
	deepBlue := hslPal(212, 55+rng.Intn(20), 45+rng.Intn(12))

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
			sx, sy, sz = vn.warp(sx, sy, sz, 1.2)

			// Базовая структура льда (плотные поля vs голубой лёд)
			base := vn.fbm(sx, sy, sz, 5, 2.1, 0.5)
			// Трещины — хребтовый шум
			crack := vn.ridged(sx*2.2-9, sy*2.2+31, sz*2.2+17, 5, 2.1, 0.5)
			// Мелкая зернистость
			det := vn.fbm(sx*9+13, sy*9-7, sz*9+5, 3, 2.3, 0.5)

			var r, g, b float64
			if base < 0.5 {
				t := base / 0.5
				r = white[0] + (paleBlue[0]-white[0])*t
				g = white[1] + (paleBlue[1]-white[1])*t
				b = white[2] + (paleBlue[2]-white[2])*t
			} else {
				t := (base - 0.5) / 0.5
				r = paleBlue[0] + (deepBlue[0]-paleBlue[0])*t
				g = paleBlue[1] + (deepBlue[1]-paleBlue[1])*t
				b = paleBlue[2] + (deepBlue[2]-paleBlue[2])*t
			}

			// Зернистость
			var dm float64 = (det - 0.5) * 0.07
			r += dm * 255
			g += dm * 255
			b += dm * 255

			// Трещины: тёмные глубокие линии
			if crack > 0.45 {
				t := (crack - 0.45) / 0.55
				t = t * t
				// Края трещины светлые, дно тёмное
				edge := math.Sin(t * math.Pi)
				r = r*(1-edge) + deepBlue[0]*edge
				g = g*(1-edge) + deepBlue[1]*edge
				b = b*(1-edge) + deepBlue[2]*edge
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