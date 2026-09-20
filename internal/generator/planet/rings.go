// internal/generator/planet/rings.go
//
// Кольца планеты, освещённые от единого светового вектора L (спека 2026-09-21
// §4.2, S3): яркость по дню/ночи (день 0.8–1.0 / ночь 0.35–0.5), тень планеты
// (×0.25–0.35), лёгкий тинт дымкой (tint 0.1–0.2). Геометрия/базовый цвет —
// как раньше (tilt/rotation/alpha от rng).
package planet

import (
	"image"
	"image/color"
	"math"
	"math/rand"
)

// drawRings добавляет кольца к изображению планеты, освещённые от L.
// haze — цвет дымки для лёгкого тинта (full — реальный haze, honest — палитра
// по типу); tint — сила тинта (0.1–0.2).
func drawRings(img *image.RGBA, size int, rng *rand.Rand, L [3]float64, haze color.RGBA, tint float64) {
	cx, cy := float64(size)/2, float64(size)/2
	radius := float64(size)/2 - 2
	ringOuter := radius * 1.6
	ringInner := radius * 1.1
	tilt := 0.3 + rng.Float64()*0.4
	rotation := 0.2 + rng.Float64()*0.3
	alpha := 0.3 + rng.Float64()*0.3
	nightBright := 0.35 + rng.Float64()*0.15 // 0.35–0.5
	dayBright := 0.8 + rng.Float64()*0.2     // 0.8–1.0
	shadowFactor := 0.25 + rng.Float64()*0.1 // 0.25–0.35

	// Нормаль плоскости кольца: наклон tilt вокруг x, поворот rotation вокруг z.
	sinT := math.Sqrt(math.Max(0, 1-tilt*tilt))
	nr := [3]float64{sinT * math.Sin(rotation), sinT * math.Cos(rotation), tilt}
	// Базис плоскости кольца (в экранных координатах).
	ex := [3]float64{math.Cos(rotation), -math.Sin(rotation), 0}
	ey := [3]float64{tilt * math.Sin(rotation), tilt * math.Cos(rotation), -sinT}
	// Проекция L на плоскость кольца (направление света в плоскости).
	ln := L[0]*nr[0] + L[1]*nr[1] + L[2]*nr[2]
	lr := [3]float64{L[0] - ln*nr[0], L[1] - ln*nr[1], L[2] - ln*nr[2]}
	lrLen := math.Sqrt(lr[0]*lr[0] + lr[1]*lr[1] + lr[2]*lr[2])

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			cosR := math.Cos(rotation)
			sinR := math.Sin(rotation)
			xRot := dx*cosR - dy*sinR
			yRot := (dx*sinR + dy*cosR) / tilt
			dist := math.Sqrt(xRot*xRot + yRot*yRot)
			if dist < ringInner || dist > ringOuter {
				continue
			}
			pos := (dist - ringInner) / (ringOuter - ringInner)
			fade := 1.0
			if pos < 0.1 {
				fade = pos / 0.1
			} else if pos > 0.8 {
				fade = 1 - (pos-0.8)/0.2
			}
			if fade < 0 {
				fade = 0
			}

			// Освещение: день/ночь от L_r, тень планеты (цилиндр вдоль L).
			bright := nightBright + (dayBright-nightBright)*(0.5+0.5*dayFactor(lr, lrLen, ex, ey, xRot, yRot))
			if inPlanetShadow(ex, ey, nr, L, xRot, yRot, radius) {
				bright *= shadowFactor
			}

			// Базовый цвет + лёгкий тинт дымкой.
			baseColor := color.RGBA{200, 180, 150, uint8(alpha * 255 * fade)}
			if tint > 0 {
				baseColor = blend(baseColor, haze, tint)
			}
			// Применяем яркость к цвету кольца.
			ringColor := color.RGBA{
				uint8(clampF(float64(baseColor.R) * bright)),
				uint8(clampF(float64(baseColor.G) * bright)),
				uint8(clampF(float64(baseColor.B) * bright)),
				baseColor.A,
			}

			c := img.RGBAAt(x, y)
			if c.A == 0 {
				img.SetRGBA(x, y, ringColor)
			} else {
				sa := float64(ringColor.A) / 255.0
				da := float64(c.A) / 255.0
				outA := sa + da*(1-sa)
				if outA > 0 {
					r := (float64(ringColor.R)*sa + float64(c.R)*da*(1-sa)) / outA
					g := (float64(ringColor.G)*sa + float64(c.G)*da*(1-sa)) / outA
					b := (float64(ringColor.B)*sa + float64(c.B)*da*(1-sa)) / outA
					img.SetRGBA(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), uint8(outA * 255)})
				}
			}
		}
	}
}

// dayFactor — 0.5 + 0.5·dot(normalize(L_r), normalize(d_r)): 0.5 — ночь,
// 1.0 — день (спека §4.2, S3).
func dayFactor(lr [3]float64, lrLen float64, ex, ey [3]float64, xRot, yRot float64) float64 {
	// Направление пикселя от центра в плоскости кольца.
	dr := [3]float64{xRot*ex[0] + yRot*ey[0], xRot*ex[1] + yRot*ey[1], xRot*ex[2] + yRot*ey[2]}
	drLen := math.Sqrt(dr[0]*dr[0] + dr[1]*dr[1] + dr[2]*dr[2])
	if lrLen == 0 || drLen == 0 {
		return 0.5
	}
	dot := (lr[0]*dr[0] + lr[1]*dr[1] + lr[2]*dr[2]) / (lrLen * drLen)
	return 0.5 + 0.5*dot
}

// inPlanetShadow — пиксель кольца в цилиндре тени планеты: луч от пикселя
// вдоль L проходит в пределах радиуса планеты (проекция диска на плоскость
// кольца вдоль L, спека §4.2).
func inPlanetShadow(ex, ey, nr, L [3]float64, xRot, yRot, radius float64) bool {
	// 3D-позиция пикселя кольца.
	p := [3]float64{xRot*ex[0] + yRot*ey[0], xRot*ex[1] + yRot*ey[1], xRot*ex[2] + yRot*ey[2]}
	// Пиксель на ночной стороне (за планетой от света).
	if p[0]*L[0]+p[1]*L[1]+p[2]*L[2] >= 0 {
		return false
	}
	// Расстояние от луча (p + t·L) до центра планеты < radius.
	cross := [3]float64{
		p[1]*L[2] - p[2]*L[1],
		p[2]*L[0] - p[0]*L[2],
		p[0]*L[1] - p[1]*L[0],
	}
	dist := math.Sqrt(cross[0]*cross[0] + cross[1]*cross[1] + cross[2]*cross[2])
	return dist < radius
}

// clampF — кламп 0..255.
func clampF(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}