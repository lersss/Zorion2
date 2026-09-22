package postproc

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// magentaBG — цвет magenta-хромакея (210,30,135): «магента-ность» min(R,B)−G = 105.
var magentaBG = color.NRGBA{R: 210, G: 30, B: 135, A: 255}

// writeTolPNG — сохраняет тестовый кадр в PNG (вход ShipCutTol).
func writeTolPNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
}

// TestShipCutTol — порог выреза edge выбирается по фону кадра: magenta-рамка →
// 100 (фон-градиент шире), чёрная/тёмно-серая рамка → 40 (tol 100 выедает
// тёмный корпус). Детект — по рамке (перенос is_magenta_bg из
// tools/ship_recut_pool.py).
func TestShipCutTol(t *testing.T) {
	const size = 200
	cases := []struct {
		name string
		fill color.NRGBA
		want int
	}{
		{"magenta", magentaBG, shipCutTolMagenta},
		{"black", color.NRGBA{0, 0, 0, 255}, shipCutTolBlack},
		{"near-black", color.NRGBA{10, 10, 10, 255}, shipCutTolBlack},
		{"dark-grey", color.NRGBA{40, 40, 40, 255}, shipCutTolBlack},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, size, size))
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					img.SetNRGBA(x, y, c.fill)
				}
			}
			path := filepath.Join(t.TempDir(), "frame.png")
			writeTolPNG(t, path, img)
			got, err := ShipCutTol(path)
			if err != nil {
				t.Fatalf("ShipCutTol: %v", err)
			}
			if got != c.want {
				t.Errorf("ShipCutTol(%s) = %d, want %d", c.name, got, c.want)
			}
		})
	}
}

// TestShipCutTolMagentaBodyBlackBorder — magenta-корпус в центре на чёрной рамке:
// фон НЕ magenta (рамка чёрная) → порог 40; деталь корпуса фон не решает.
func TestShipCutTolMagentaBodyBlackBorder(t *testing.T) {
	const size = 200
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
		}
	}
	for y := 60; y < 140; y++ {
		for x := 60; x < 140; x++ {
			img.SetNRGBA(x, y, magentaBG)
		}
	}
	path := filepath.Join(t.TempDir(), "frame.png")
	writeTolPNG(t, path, img)
	got, err := ShipCutTol(path)
	if err != nil {
		t.Fatalf("ShipCutTol: %v", err)
	}
	if got != shipCutTolBlack {
		t.Errorf("ShipCutTol (magenta-корпус на чёрной рамке) = %d, want %d", got, shipCutTolBlack)
	}
}

// TestShipCutTolMissingFile — нечитаемый кадр → ошибка (не тихий фолбек порога).
func TestShipCutTolMissingFile(t *testing.T) {
	if _, err := ShipCutTol(filepath.Join(t.TempDir(), "nope.png")); err == nil {
		t.Errorf("ShipCutTol отсутствующего файла: err = nil, want ошибку")
	}
}

// TestMagentaNess — «магента-ность» min(R,B)−G: magenta ≈ 105, чёрный ≈ 0,
// серый ≈ 0, чисто-синий отрицательный (как is_magenta_bg в Python).
func TestMagentaNess(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	pix := []color.NRGBA{magentaBG, {0, 0, 0, 255}, {40, 40, 40, 255}, {40, 60, 80, 255}}
	for i, p := range pix {
		img.SetNRGBA(i, 0, p)
	}
	want := []float64{105, 0, 0, -20}
	for i, w := range want {
		if got := magentaNess(img, i, 0); got != w {
			t.Errorf("magentaNess[%d] = %v, want %v", i, got, w)
		}
	}
}
