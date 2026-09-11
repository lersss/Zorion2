// internal/generator/planet/generator_rocky.go
package planet

import (
	"image"
	"image/color"
	"math"
	"math/rand"
)

func generateRocky(img *image.RGBA, size int, rng *rand.Rand) {
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	vn := newValueNoise(rng.Int63())

	// Палитра: тёмный реголит → светлая порода → светлые возвышенности
	low := hslPal(20+rng.Intn(12), 30+rng.Intn(15), 18+rng.Intn(8))
	mid := hslPal(24+rng.Intn(14), 25+rng.Intn(15), 32+rng.Intn(12))
	high := hslPal(32+rng.Intn(20), 20+rng.Intn(15), 48+rng.Intn(15))
	// Региональный оттенок (марсианские красные поля, серые равнины и т.п.)
	tintHue := rng.Intn(360)
	tintR, tintG, tintB := hslToRgb(tintHue, 55, 50)

	// Кратеры: распределяем ближе к центру диска, чтобы не свешивались за край
	type crater struct {
		x, y, r float64
	}
	craterCount := 3 + rng.Intn(6)
	craters := make([]crater, craterCount)
	for i := 0; i < craterCount; i++ {
		angle := rng.Float64() * 2 * math.Pi
		dist := math.Sqrt(rng.Float64()) * radius * 0.8
		craters[i] = crater{
			x: cx + math.Cos(angle)*dist,
			y: cy + math.Sin(angle)*dist,
			r: 3 + rng.Float64()*radius*0.2,
		}
	}

	scale := 3.5
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

			// Основной рельеф + крупные регионы + мелкая детализация
			e := vn.fbm(sx, sy, sz, 5, 2.1, 0.5)
			region := vn.fbm(sx*0.45, sy*0.45, sz*0.45, 3, 2.0, 0.5)
			det := vn.fbm(sx*7+31, sy*7-17, sz*7+5, 3, 2.3, 0.5)

			var r, g, b float64
			if e < 0.5 {
				t := e / 0.5
				r = low[0] + (mid[0]-low[0])*t
				g = low[1] + (mid[1]-low[1])*t
				b = low[2] + (mid[2]-low[2])*t
			} else {
				t := (e - 0.5) / 0.5
				r = mid[0] + (high[0]-mid[0])*t
				g = mid[1] + (high[1]-mid[1])*t
				b = mid[2] + (high[2]-mid[2])*t
			}

			// Региональный оттенок (только где он выше порога)
			tintMix := (region - 0.45) * 0.5
			if tintMix < 0 {
				tintMix = 0
			}
			if tintMix > 0.45 {
				tintMix = 0.45
			}
			r = r*(1-tintMix) + float64(tintR)*tintMix
			g = g*(1-tintMix) + float64(tintG)*tintMix
			b = b*(1-tintMix) + float64(tintB)*tintMix

			// Мелкая зернистость поверхности
			var dm float64 = (det - 0.5) * 0.1
			r += dm * 255
			g += dm * 255
			b += dm * 255

			// Кратеры: тёмное дно + светлый обод
			for _, cr := range craters {
				ddx := float64(x) - cr.x
				ddy := float64(y) - cr.y
				dc := math.Sqrt(ddx*ddx + ddy*ddy)
				if dc < cr.r {
					t := dc / cr.r
					shade := (1 - t) * 0.4
					r -= shade * float64(mid[0])
					g -= shade * float64(mid[1])
					b -= shade * float64(mid[2])
					rim := 1 - math.Abs(t-0.85)*8
					if rim > 0 {
						r += rim * 30
						g += rim * 30
						b += rim * 30
					}
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