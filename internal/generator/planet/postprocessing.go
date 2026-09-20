// internal/generator/planet/postprocessing.go
//
// Постобработка картинки планеты (спека 2026-09-21 §4.2, контракт B/M3):
// тень/терминатор/спекл/атмосферный ореол — от единого светового вектора L
// (lightVector от seed). Контракт applyPostProcessing(img, size, fx PostFX, rng):
// specular = nil — спекла нет (заглушка, контракт D); atmGlint = nil — ореола
// нет (honest); оба — только full/честные.
package planet

import (
	"image"
	"image/color"
	"math"
	"math/rand"
)

// PostFX — параметры постобработки (спека §4.2): единый световой вектор L,
// сила тени, спекл (nil = нет), атмосферный ореол (nil = нет).
type PostFX struct {
	L              [3]float64
	ShadowStrength float64
	Specular       *Specular
	AtmGlint       *Glint
}

// Specular — спекл-блик: pow, сила, цвет (спека §4.2, таблица типов).
type Specular struct {
	Pow      float64
	Strength float64
	Color    color.RGBA
}

// Glint — атмосферная составляющая (только full): сила ореола, цвет дымки.
type Glint struct {
	GlowStrength float64
	Haze         color.RGBA
}

// applyPostProcessing — тень/терминатор/спекл/ореол от единого L (спека §4.2).
// Параметр hasAtmosphere старой сигнатуры убран — атмосферный слой генератора
// заменяет его (прошлая спека §4.1 п.6).
func applyPostProcessing(img *image.RGBA, size int, fx PostFX, rng *rand.Rand) {
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	L := fx.L
	lenL := math.Sqrt(L[0]*L[0] + L[1]*L[1] + L[2]*L[2])
	if lenL == 0 {
		lenL = 1
	}
	nx, ny, nz := L[0]/lenL, L[1]/lenL, L[2]/lenL

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			normDist := dist / radius
			if normDist > 1 {
				continue
			}
			c := img.RGBAAt(x, y)
			z := math.Sqrt(math.Max(0, radius*radius-dx*dx-dy*dy))
			normLen := math.Sqrt(dx*dx + dy*dy + z*z)
			if normLen == 0 {
				continue
			}
			normDx, normDy, normDz := dx/normLen, dy/normLen, z/normLen
			diffuse := normDx*nx + normDy*ny + normDz*nz
			if diffuse < 0 {
				diffuse = 0
			}
			if diffuse > 1 {
				diffuse = 1
			}

			// Мягкий терминатор: плавное затухание вместо резкого перехода 0/1.
			diffuse = softStep(diffuse)
			shadow := 1 - fx.ShadowStrength*(1-diffuse)

			r := float64(c.R) * shadow
			g := float64(c.G) * shadow
			b := float64(c.B) * shadow

			// Спекл (nil = нет — заглушка, контракт D/M4).
			if fx.Specular != nil {
				si := specIntensity(diffuse, normDz, nz, fx.Specular)
				if si > 0.01 {
					hr := float64(fx.Specular.Color.R)
					hg := float64(fx.Specular.Color.G)
					hb := float64(fx.Specular.Color.B)
					r = r + (hr-r)*si
					g = g + (hg-g)*si
					b = b + (hb-b)*si
				}
			}

			// Атмосферный ореол на дневном лимбе (только full):
			// atmGlint = glowStrength·(0.5+0.5·diffuse)·edgeFactor, кламп ≤ 0.2.
			if fx.AtmGlint != nil {
				edgeFactor := math.Max(0, normDist-0.55) / 0.45
				glint := fx.AtmGlint.GlowStrength * (0.5 + 0.5*diffuse) * edgeFactor
				if glint > 0.2 {
					glint = 0.2
				}
				if glint > 0.01 {
					ar := float64(fx.AtmGlint.Haze.R)
					ag := float64(fx.AtmGlint.Haze.G)
					ab := float64(fx.AtmGlint.Haze.B)
					r = r*(1-glint) + ar*glint
					g = g*(1-glint) + ag*glint
					b = b*(1-glint) + ab*glint
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

// softStep — плавный S-образный переход 0→1 на [0,1].
func softStep(t float64) float64 {
	// Растягиваем интервал перехода: почти 0 при t<0.15, почти 1 при t>0.85
	t = (t - 0.15) / 0.7
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return t * t * (3 - 2*t)
}