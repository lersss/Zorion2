// internal/generator/planet/generator_gas.go
package planet

import (
	"image"
	"image/color"
	"math"
	"math/rand"
)

func generateGas(img *image.RGBA, size int, rng *rand.Rand) {
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	vn := newValueNoise(rng.Int63())

	// Полосы: пара тёплых + пара глубоких тонов одного семейства
	baseHue := 10 + rng.Intn(40)
	bandA := hslPal(baseHue, 60+rng.Intn(20), 45+rng.Intn(15))
	bandB := hslPal(baseHue+20+rng.Intn(20), 55+rng.Intn(20), 55+rng.Intn(15))
	bandC := hslPal(baseHue-15+rng.Intn(20), 50+rng.Intn(20), 32+rng.Intn(12))
	bandD := hslPal(baseHue+35+rng.Intn(25), 45+rng.Intn(20), 62+rng.Intn(12))

	bands := [4][3]float64{bandA, bandB, bandC, bandD}
	numBands := 4 + rng.Intn(5)

	// Вихрь (Большое Красное Пятно)
	hasStorm := rng.Float64() < 0.6
	stormX := (rng.Float64() - 0.5) * radius * 0.8
	stormY := (rng.Float64() - 0.5) * radius * 0.5
	stormR := 4 + rng.Float64()*10
	stormCol := hslPal(baseHue+30, 80+rng.Intn(15), 62+rng.Intn(10))

	scale := 2.2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > radius {
				continue
			}

			sx, sy, sz := spherePoint(dx, dy, radius, scale)

			// Турбулентность — варп координат, полосы ломаются и плывут
			wx, wy, wz := vn.warp(sx, sy, sz, 1.6)
			turb := vn.fbm(wx*2.2, wy*2.2, wz*2.2, 4, 2.1, 0.5)

			// Широта на сфере + турбулентное смещение = позиция полосы
			lat := math.Atan2(sz, math.Sqrt(sx*sx+sy*sy)+1e-9) // -1..1
			bandPos := (lat + (turb-0.5)*0.7) * 0.5 + 0.5     // 0..1
			bandIndex := int(bandPos * float64(numBands))
			if bandIndex < 0 {
				bandIndex = 0
			}
			if bandIndex >= numBands {
				bandIndex = numBands - 1
			}
			// Плавный переход между полосами
			pos := bandPos*float64(numBands) - float64(bandIndex)
			blend := smoothstep(pos)

			colA := bands[bandIndex%4]
			colB := bands[(bandIndex+1)%4]
			var r, g, b float64
			r = colA[0] + (colB[0]-colA[0])*blend
			g = colA[1] + (colB[1]-colA[1])*blend
			b = colA[2] + (colB[2]-colA[2])*blend

			// Тонкая штриховка внутри полос
			stripe := vn.fbm(wx*8, wy*8, wz*8, 3, 2.3, 0.5)
			var sm float64 = (stripe - 0.5) * 0.12
			r += sm * 255
			g += sm * 255
			b += sm * 255

			// Вихрь: завихрение
			if hasStorm {
				ddx := float64(x) - stormX
				ddy := float64(y) - stormY
				ddist := math.Sqrt(ddx*ddx + ddy*ddy)
				if ddist < stormR {
					f := 1 - ddist/stormR
					swirl := math.Sin(ddist*0.8 + math.Atan2(ddy, ddx)*2)
					factor := f*f*(0.5+0.5*swirl)
					r = r*(1-factor) + stormCol[0]*factor
					g = g*(1-factor) + stormCol[1]*factor
					b = b*(1-factor) + stormCol[2]*factor
				}
				// Лёгкое возмущение полос вокруг вихря
				if ddist < stormR*1.8 {
					f := 1 - ddist/(stormR*1.8)
					var pert float64 = f * 18
					r += pert * (0.6 - 0.4)
					g += pert * (0.6 - 0.4)
					b += pert * (0.6 - 0.4)
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

// smoothstep — плавный переход 0→1 на интервале [0,1].
func smoothstep(t float64) float64 {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return t * t * (3 - 2*t)
}