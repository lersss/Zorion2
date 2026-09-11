// internal/generator/planet/postprocessing.go
package planet

import (
	"image"
	"image/color"
	"math"
	"math/rand"
)

// applyPostProcessing добавляет тень, блик и атмосферу к изображению планеты
func applyPostProcessing(img *image.RGBA, size int, visualType string, hasAtmosphere bool, rng *rand.Rand) {
	radius := float64(size)/2 - 2
	cx, cy := float64(size)/2, float64(size)/2
	shadowStrength := 0.6
	highlightStrength := 0.3
	atmStrength := 0.0
	atmColor := color.RGBA{100, 150, 255, 50}
	highlightColor := color.RGBA{255, 255, 255, 230}

	if hasAtmosphere {
		atmStrength = 0.2
	}
	switch visualType {
	case "ice":
		atmColor = color.RGBA{200, 230, 255, 40}
		highlightStrength = 0.8
		shadowStrength = 0.4
	case "lava":
		atmColor = color.RGBA{255, 100, 50, 60}
		highlightStrength = 0.2
		shadowStrength = 0.6
		if hasAtmosphere {
			atmStrength = 0.15
		}
	case "earth":
		atmColor = color.RGBA{70, 150, 255, 50}
		highlightStrength = 0.3
		shadowStrength = 0.6
	case "gas":
		atmColor = color.RGBA{200, 180, 150, 40}
		highlightStrength = 0.2
		shadowStrength = 0.6
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			normDist := dist / radius
			if normDist > 1.2 {
				img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0})
				continue
			}
			if normDist > 1 {
				if hasAtmosphere && atmStrength > 0 {
					outer := (normDist - 1) / 0.2
					alpha := float64(atmColor.A) / 255.0 * (1 - outer) * 0.5
					c := img.RGBAAt(x, y)
					r := float64(c.R)*(1-alpha) + float64(atmColor.R)*alpha
					g := float64(c.G)*(1-alpha) + float64(atmColor.G)*alpha
					b := float64(c.B)*(1-alpha) + float64(atmColor.B)*alpha
					img.SetRGBA(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), uint8(float64(c.A)*(1-alpha) + 255*alpha)})
				}
				continue
			}
			c := img.RGBAAt(x, y)
			lightX, lightY, lightZ := -0.5, -0.4, 0.2
			lenL := math.Sqrt(lightX*lightX + lightY*lightY + lightZ*lightZ)
			nx, ny, nz := lightX/lenL, lightY/lenL, lightZ/lenL
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

		// Мягкий терминатор: плавное затухание вместо резкого перехода 0/1
		diffuse = softStep(diffuse)
		shadow := 1 - shadowStrength*(1-diffuse)

		// Рассеяние света у края диска (свет заходит на теневую сторону)
		if hasAtmosphere {
			edgeFactor := math.Max(0, normDist-0.55) / 0.45
			scatter := 0.35 * edgeFactor
			shadow = shadow + scatter*(1-shadow)
		}
		r := float64(c.R) * shadow
		g := float64(c.G) * shadow
		b := float64(c.B) * shadow

			spec := math.Max(0, 2*diffuse*normDz-nz)
			specIntensity := math.Pow(spec, 20) * highlightStrength * 2
			if specIntensity > 0.01 {
				hr := float64(highlightColor.R)
				hg := float64(highlightColor.G)
				hb := float64(highlightColor.B)
				r = r + (hr-r)*specIntensity
				g = g + (hg-g)*specIntensity
				b = b + (hb-b)*specIntensity
			}
			if hasAtmosphere && atmStrength > 0 && normDist > 0.7 {
				edge := (normDist - 0.7) / 0.3
				atmAlpha := atmStrength * edge * 0.8
				ar := float64(atmColor.R)
				ag := float64(atmColor.G)
				ab := float64(atmColor.B)
				r = r*(1-atmAlpha) + ar*atmAlpha
				g = g*(1-atmAlpha) + ag*atmAlpha
				b = b*(1-atmAlpha) + ab*atmAlpha
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