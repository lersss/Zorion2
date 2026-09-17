package postproc

import (
	"image"
	"image/color"
	"testing"
)

// TestPadCenterBottom — ресайз 200×200: пропорции сохранены, прижатие к низу,
// центрирование по X (DoD 67a.1 §10, спека §7.4).
func TestPadCenterBottom(t *testing.T) {
	// широкое изображение 400×200 → вписывается как 200×100, прижато к низу
	src := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			src.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	dst := PadCenterBottom(src, 200, 200)
	if dst.Bounds().Dx() != 200 || dst.Bounds().Dy() != 200 {
		t.Fatalf("размер = %dx%d, want 200x200", dst.Bounds().Dx(), dst.Bounds().Dy())
	}
	// верхняя половина (y < 100) — прозрачная
	for x := 0; x < 200; x++ {
		if dst.NRGBAAt(x, 50).A != 0 {
			t.Errorf("(%d,50): верх не прозрачен", x)
		}
	}
	// нижняя половина (y >= 100) — непрозрачная
	for x := 0; x < 200; x++ {
		if dst.NRGBAAt(x, 150).A == 0 {
			t.Errorf("(%d,150): низ пуст", x)
		}
	}
}

// TestPadCenterBottomTall — высокое изображение 200×400 → 100×200, по центру X.
func TestPadCenterBottomTall(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 200, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 200; x++ {
			src.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	dst := PadCenterBottom(src, 200, 200)
	// объект 100×200: x от 50 до 150, y от 0 до 200
	if dst.NRGBAAt(10, 100).A != 0 {
		t.Error("левый край не прозрачен")
	}
	if dst.NRGBAAt(100, 100).A == 0 {
		t.Error("центр пуст")
	}
	if dst.NRGBAAt(190, 100).A != 0 {
		t.Error("правый край не прозрачен")
	}
}

// TestCropBottom — обрезка снизу: остаётся pct% высоты, мин 50.
func TestCropBottom(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	cropped := CropBottom(img, 60)
	if cropped.Bounds().Dy() != 60 {
		t.Errorf("высота = %d, want 60", cropped.Bounds().Dy())
	}
	// мин 50: pct=10 → 50
	cropped2 := CropBottom(img, 10)
	if cropped2.Bounds().Dy() != 50 {
		t.Errorf("высота при pct=10 = %d, want 50", cropped2.Bounds().Dy())
	}
}