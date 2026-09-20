package generator

import (
	"image"
	"image/color"
	"testing"
)

// TestCandidateAutoFilter — метки авто-фильтра кандидата (диагноз визуального
// аудита, п.7): «палитра» (тёплые пиксели у холодной расы), «форма» (масса
// слева — нос не читается), «текстура» (мало уникальных цветов); хороший
// кандидат — без меток.
func TestCandidateAutoFilter(t *testing.T) {
	// тёплый кандидат холодной расы → «палитра»
	warm := solidImage(200, 200, 230, 120, 40) // оранжевый
	labels := ShipCandidateLabels(warm, true)
	if !containsStr(labels, "палитра") {
		t.Errorf("тёплый кандидат холодной расы: labels = %v, want «палитра»", labels)
	}
	// тёплый кандидат тёплой расы → без «палитра»
	labels = ShipCandidateLabels(warm, false)
	if containsStr(labels, "палитра") {
		t.Errorf("тёплый кандидат тёплой расы: labels = %v, не want «палитра»", labels)
	}
	// масса слева → «форма»
	left := leftMassImage(200, 200)
	labels = ShipCandidateLabels(left, false)
	if !containsStr(labels, "форма") {
		t.Errorf("масса слева: labels = %v, want «форма»", labels)
	}
	// мало уникальных цветов → «текстура»
	labels = ShipCandidateLabels(warm, false)
	if !containsStr(labels, "текстура") {
		t.Errorf("1 цвет: labels = %v, want «текстура»", labels)
	}
	// хороший кандидат (масса справа, много цветов, без тёплых) → без меток
	good := goodCandidateImage(200, 200)
	labels = ShipCandidateLabels(good, true)
	if len(labels) != 0 {
		t.Errorf("хороший кандидат: labels = %v, want пусто", labels)
	}
}

func solidImage(w, h int, r, g, b uint8) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{r, g, b, 255})
		}
	}
	return img
}

func leftMassImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w/2; x++ {
			img.SetNRGBA(x, y, color.NRGBA{120, 140, 170, 255})
		}
	}
	return img
}

func goodCandidateImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := w * 2 / 5; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(50 + x%100), uint8(100 + y%100), 220, 255})
		}
	}
	return img
}