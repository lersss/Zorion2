package postproc

import (
	"image"
	"image/draw"
)

// CropBottom обрезает снизу: оставляет pct% высоты (мин 50), затем
// крупнейший связный компонент (перенос crop_bottom, спека 67a.1 §7.6).
func CropBottom(img *image.NRGBA, pct int) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	keep := h * pct / 100
	if keep < 50 {
		keep = 50
	}
	if keep > h {
		keep = h
	}
	cropped := image.NewNRGBA(image.Rect(0, 0, w, keep))
	draw.Draw(cropped, cropped.Bounds(), img, image.Pt(b.Min.X, b.Min.Y), draw.Src)
	LargestComponent(cropped, 40)
	return cropped
}