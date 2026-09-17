package postproc

import (
	"image"
	"math"

	"github.com/disintegration/imaging"
)

// PadCenterBottom — аналог ImageOps.pad(img, (w,h), centering=(0.5, 1.0))
// (спека 67a.1 §7.4): масштабирование с сохранением пропорций (вписывание),
// вставка в холст w×h с прозрачным фоном, центрирование по X, прижатие к низу по Y.
func PadCenterBottom(src image.Image, w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw == 0 || sh == 0 {
		return dst
	}
	scale := math.Min(float64(w)/float64(sw), float64(h)/float64(sh))
	nw := int(math.Round(float64(sw) * scale))
	nh := int(math.Round(float64(sh) * scale))
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	resized := imaging.Resize(src, nw, nh, imaging.Lanczos)
	x := (w - nw) / 2
	y := h - nh
	return imaging.Paste(dst, resized, image.Pt(x, y))
}