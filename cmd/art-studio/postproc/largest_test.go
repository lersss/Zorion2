package postproc

import (
	"image"
	"image/color"
	"testing"
)

// TestLargestComponent — маска с двумя компонентами: остаётся крупнейший
// (DoD 67a.1 §10, спека §7.2).
func TestLargestComponent(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	// компонент 1: квадрат 3×3 в левом верхнем углу
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
		}
	}
	// компонент 2: квадрат 5×5 в правом нижнем углу (крупнее, не соприкасается)
	for y := 5; y < 10; y++ {
		for x := 5; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{0, 255, 0, 255})
		}
	}
	LargestComponent(img, 40)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			inBig := x >= 5 && x < 10 && y >= 5 && y < 10
			a := img.NRGBAAt(x, y).A
			if inBig && a == 0 {
				t.Errorf("(%d,%d): крупнейший компонент обнулён", x, y)
			}
			if !inBig && a != 0 {
				t.Errorf("(%d,%d): мусорный компонент остался", x, y)
			}
		}
	}
}

// TestLargestComponentThreshold — пиксели с альфой ≤ порога не считаются.
func TestLargestComponentThreshold(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	// полупрозрачный пиксель (альфа 20 ≤ 40) — не компонент
	img.SetNRGBA(0, 0, color.NRGBA{255, 255, 255, 20})
	// непрозрачный квадрат 2×2
	for y := 1; y < 3; y++ {
		for x := 1; x < 3; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	LargestComponent(img, 40)
	if img.NRGBAAt(0, 0).A != 0 {
		t.Error("полупрозрачный пиксель не обнулён")
	}
	if img.NRGBAAt(1, 1).A == 0 {
		t.Error("непрозрачный компонент обнулён")
	}
}