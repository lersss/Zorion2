package postproc

import (
	"image"
	"image/draw"

	"github.com/disintegration/imaging"
)

// NormalizeAlpha делает силуэт полностью непрозрачным: пиксели с alpha >
// threshold — 255, остальные — 0. Убирает полупрозрачный «бледный» низ после
// rembg (низ картинки просвечивал фон — решение создателя 2026-09-17).
func NormalizeAlpha(img *image.NRGBA, threshold uint8) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			if img.Pix[i+3] > threshold {
				img.Pix[i+3] = 255
			} else {
				img.Pix[i+3] = 0
			}
		}
	}
}

// NormalizeAlphaFile читает PNG, нормализует альфу, сохраняет.
func NormalizeAlphaFile(path string, threshold uint8) error {
	img, err := imaging.Open(path)
	if err != nil {
		return err
	}
	nrgba := image.NewNRGBA(img.Bounds())
	draw.Draw(nrgba, nrgba.Bounds(), img, img.Bounds().Min, draw.Src)
	NormalizeAlpha(nrgba, threshold)
	return imaging.Save(nrgba, path)
}