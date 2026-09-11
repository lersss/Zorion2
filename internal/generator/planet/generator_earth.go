// internal/generator/planet/generator_earth.go
package planet

import (
	"image"
	"image/color"
	"math"
	"math/rand"
)

func generateEarth(img *image.RGBA, size int, rng *rand.Rand, temperature float64) {
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	vn := newValueNoise(rng.Int63())

	// Палитра
	deepOcean := hslPal(215+rng.Intn(12), 55+rng.Intn(15), 20+rng.Intn(8))
	shallow := hslPal(205+rng.Intn(15), 50+rng.Intn(20), 42+rng.Intn(14))
	grass := hslPal(90+rng.Intn(35), 45+rng.Intn(20), 30+rng.Intn(12))
	forest := hslPal(120+rng.Intn(30), 45+rng.Intn(20), 24+rng.Intn(10))
	mountain := hslPal(30+rng.Intn(20), 20+rng.Intn(15), 45+rng.Intn(12))
	snow := hslPal(0, 0, 88+rng.Intn(8))

	// Уровень воды — чем холоднее, тем больше ледяных шапок
	waterLevel := 0.48
	if temperature < 260 {
		waterLevel = 0.5
	}
	// Порог ледяных шапок по широте (z координата сферы)
	iceThreshold := 0.32 + (temperature-200)/400
	if iceThreshold > 0.55 {
		iceThreshold = 0.55
	}
	if iceThreshold < 0.2 {
		iceThreshold = 0.2
	}

	scale := 2.8
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > radius {
				continue
			}

			sx, sy, sz := spherePoint(dx, dy, radius, scale)
			sx, sy, sz = vn.warp(sx, sy, sz, 1.0)

			// Высота рельефа
			e := vn.fbm(sx, sy, sz, 6, 2.1, 0.5)

			var r, g, b float64
			if e < waterLevel {
				// Океан: глубокий → мелководье
				t := e / waterLevel
				t = math.Pow(t, 0.6)
				r = deepOcean[0] + (shallow[0]-deepOcean[0])*t
				g = deepOcean[1] + (shallow[1]-deepOcean[1])*t
				b = deepOcean[2] + (shallow[2]-deepOcean[2])*t
				// Лёгкая рябь воды
				wave := vn.fbm(sx*9, sy*9, sz*9, 2, 2.2, 0.5)
				var w = (wave - 0.5) * 0.04
				r += w * 255
				g += w * 255
				b += w * 255
			} else {
				// Суша: низменность → леса → горы → снег
				t := (e - waterLevel) / (1 - waterLevel)
				if t < 0.35 {
					tt := t / 0.35
					r = grass[0] + (forest[0]-grass[0])*tt
					g = grass[1] + (forest[1]-grass[1])*tt
					b = grass[2] + (forest[2]-grass[2])*tt
				} else if t < 0.75 {
					tt := (t - 0.35) / 0.4
					r = forest[0] + (mountain[0]-forest[0])*tt
					g = forest[1] + (mountain[1]-forest[1])*tt
					b = forest[2] + (mountain[2]-forest[2])*tt
				} else {
					tt := (t - 0.75) / 0.25
					r = mountain[0] + (snow[0]-mountain[0])*tt
					g = mountain[1] + (snow[1]-mountain[1])*tt
					b = mountain[2] + (snow[2]-mountain[2])*tt
				}
				// Детализация рельефа
				det := vn.fbm(sx*6+31, sy*6-17, sz*6+5, 3, 2.3, 0.5)
				var dm float64 = (det - 0.5) * 0.08
				r += dm * 255
				g += dm * 255
				b += dm * 255
			}

			// Полярные шапки (по широте с неровным краем)
			if math.Abs(sz) > iceThreshold {
				edge := (math.Abs(sz) - iceThreshold) / (1 - iceThreshold)
				edgeMask := vn.fbm(sx*3+17, sy*3-9, sz*3+11, 3, 2.1, 0.5)
				ice := edge * edgeMask
				iceMix := ice * ice * (3 - 2*ice)
				if iceMix > 1 {
					iceMix = 1
				}
				r = r*(1-iceMix) + snow[0]*iceMix
				g = g*(1-iceMix) + snow[1]*iceMix
				b = b*(1-iceMix) + snow[2]*iceMix
			}

			// Облака: порог по fBm, плотнее в умеренных широтах
			cloudN := vn.fbm(sx*3.2+7, sy*3.2-13, sz*3.2+23, 5, 2.2, 0.5)
			midLat := 1 - math.Abs(sz)
			if cloudN > 0.58+midLat*0.1 {
				cA := (cloudN - 0.58 - midLat*0.1) / 0.42
				cA = cA * 0.75
				if cA > 0.8 {
					cA = 0.8
				}
				r = r*(1-cA) + 255*cA
				g = g*(1-cA) + 255*cA
				b = b*(1-cA) + 255*cA
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