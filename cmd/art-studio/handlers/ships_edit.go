// cmd/art-studio/handlers/ships_edit.go
// Ручная правка ориентации кандидата корабля (вкладка «Корабли рас»):
// rot90 (90° по часовой), rot180, flipH (зеркало по горизонтали — нос
// влево ↔ нос вправо), rotate (произвольный угол по часовой, слайдер
// −180…+180) и fit («вписать в кадр» — crop по bbox + 200×200, как у
// эталонных спрайтов). После любого поворота выполняется ре-нормализация
// (refitShip), иначе корабль встаёт криво в квадрате.
//
// Контракт: /ships/act?file=&what=rot90|rot180|flipH|rotate&angle=<deg>|fit
// (перезапись файла кандидата атомарно, tmp + rename) и
// /ships/preview?file=&angle=<deg> — та же нормализация без записи
// (живой предпросмотр слайдера без перезагрузки страницы).
package handlers

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
)

// shipSpriteSize — финальный размер спрайта корабля (как у 21 эталона).
const shipSpriteSize = 200

// shipAlphaThresh — порог alpha для bbox содержимого (как в
// tools/spike_ship_sprite_cut.py и tools/process_ship.py: mask = alpha > 40).
const shipAlphaThresh = 40

// transformShipImage — применить действие ориентации к PNG-файлу кандидата
// и перезаписать его на месте атомарно. angle учитывается только для
// op == "rotate".
func transformShipImage(path, op string, angle float64) error {
	src, err := openImage(path)
	if err != nil {
		return err
	}
	dst, err := transformShip(src, op, angle)
	if err != nil {
		return err
	}
	return saveShipPNG(path, dst)
}

// transformShip — чистое преобразование без диска: используется и правкой
// файла (transformShipImage), и живым предпросмотром (/ships/preview).
// op ∈ {rot90, rot180, flipH, rotate, fit}; иное — ошибка.
func transformShip(src image.Image, op string, angle float64) (*image.RGBA, error) {
	switch op {
	case "rot90":
		return refitShip(rotate90CW(src), shipSpriteSize), nil
	case "rot180":
		return refitShip(rotate180(src), shipSpriteSize), nil
	case "flipH":
		return refitShip(flipHorizontal(src), shipSpriteSize), nil
	case "rotate":
		return refitShip(rotateDeg(src, angle), shipSpriteSize), nil
	case "fit":
		return refitShip(src, shipSpriteSize), nil
	default:
		return nil, fmt.Errorf("неизвестное действие %q", op)
	}
}

// saveShipPNG — записать PNG атомарно: tmp-файл рядом + rename поверх
// исходного (на Windows os.Rename перезаписывает существующий файл).
func saveShipPNG(path string, img image.Image) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// rotate90CW — поворот на 90° по часовой: ширина и высота меняются местами.
func rotate90CW(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// rotate180 — поворот на 180° (по и против часовой совпадают).
func rotate180(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// flipHorizontal — зеркало по горизонтали: нос влево ↔ нос вправо.
func flipHorizontal(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// rotateDeg — поворот на deg градусов ПО ЧАСОВОЙ стрелке (визуально, как
// кнопка ⟳), вокруг центра, на холсте размером с диагональ (углы не
// обрезаются), билинейно. Положительный угол = по часовой.
func rotateDeg(src image.Image, deg float64) *image.RGBA {
	if deg == 0 {
		return toRGBA(src)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	n := int(math.Ceil(math.Hypot(float64(w), float64(h))))
	if n < 1 {
		n = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, n, n))
	rad := deg * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	cx, cy := float64(w)/2, float64(h)/2
	dcx, dcy := float64(n)/2, float64(n)/2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx := float64(x) + 0.5 - dcx
			dy := float64(y) + 0.5 - dcy
			// обратное (CW) преобразование: координаты источника
			sx := cos*dx + sin*dy + cx - 0.5 + float64(b.Min.X)
			sy := -sin*dx + cos*dy + cy - 0.5 + float64(b.Min.Y)
			out.SetRGBA(x, y, bilinearPremul(src, sx, sy, false))
		}
	}
	return out
}

// refitShip — «вписать в кадр»: обрезать по bbox непрозрачных пикселей
// (alpha > 40), вписать длинную сторону в size×size и центрировать на
// прозрачном холсте — нормализация эталонных спрайтов (bbox заполняет кадр
// по длинной стороне, прозрачный фон). Пустое изображение → пустой холст.
func refitShip(src image.Image, size int) *image.RGBA {
	bb := alphaBounds(src)
	if bb.Empty() {
		return image.NewRGBA(image.Rect(0, 0, size, size))
	}
	crop := image.NewRGBA(image.Rect(0, 0, bb.Dx(), bb.Dy()))
	draw.Draw(crop, crop.Bounds(), src, bb.Min, draw.Src)
	w, h := crop.Bounds().Dx(), crop.Bounds().Dy()
	long := w
	if h > long {
		long = h
	}
	nw := int(math.Round(float64(w) * float64(size) / float64(long)))
	nh := int(math.Round(float64(h) * float64(size) / float64(long)))
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	scaled := scaleBilinear(crop, nw, nh)
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	off := image.Pt((size-nw)/2, (size-nh)/2)
	draw.Draw(out, image.Rect(off.X, off.Y, off.X+nw, off.Y+nh), scaled, image.Point{}, draw.Src)
	return out
}

// alphaBounds — bbox пикселей с alpha > shipAlphaThresh; пустой Rectangle,
// если непрозрачных пикселей нет.
func alphaBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a>>8 > shipAlphaThresh {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < minX || maxY < minY {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

// toRGBA — привести к *image.RGBA (premultiplied), без копии, если уже он.
func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}

// scaleBilinear — масштабирование билинейно (premultiplied: билинейная
// интерполяция непрозрачного цвета не даёт тёмного ореола на краях).
func scaleBilinear(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if sw == 0 || sh == 0 || w == 0 || h == 0 {
		return dst
	}
	xr := float64(sw) / float64(w)
	yr := float64(sh) / float64(h)
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*yr - 0.5 + float64(b.Min.Y)
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*xr - 0.5 + float64(b.Min.X)
			dst.SetRGBA(x, y, bilinearPremul(src, fx, fy, true))
		}
	}
	return dst
}

// bilinearPremul — билинейная выборка premultiplied RGBA в точке (fx,fy)
// (пиксельный центр — целое число). clampEdge=true — координаты зажимаются
// к границе (масштабирование: крайние пиксели не «размываются» в полупрозрачное,
// сумма весов = 1); false — за границей прозрачно (поворот: срез углов даёт
// корректный полупрозрачный край).
func bilinearPremul(src image.Image, fx, fy float64, clampEdge bool) color.RGBA {
	b := src.Bounds()
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	tx := fx - float64(x0)
	ty := fy - float64(y0)
	var r, g, bl, a float64
	for j := 0; j <= 1; j++ {
		for i := 0; i <= 1; i++ {
			px, py := x0+i, y0+j
			if clampEdge {
				px = clampIndex(px, b.Min.X, b.Max.X-1)
				py = clampIndex(py, b.Min.Y, b.Max.Y-1)
			} else if px < b.Min.X || px >= b.Max.X || py < b.Min.Y || py >= b.Max.Y {
				continue
			}
			wx := 1 - tx
			if i == 1 {
				wx = tx
			}
			wy := 1 - ty
			if j == 1 {
				wy = ty
			}
			w := wx * wy
			c := color.RGBAModel.Convert(src.At(px, py)).(color.RGBA)
			r += float64(c.R) * w
			g += float64(c.G) * w
			bl += float64(c.B) * w
			a += float64(c.A) * w
		}
	}
	return color.RGBA{R: clamp255(r), G: clamp255(g), B: clamp255(bl), A: clamp255(a)}
}

// clampIndex — зажать индекс в [lo, hi].
func clampIndex(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// clamp255 — округлить до ближайшего целого и зажать в [0,255].
func clamp255(v float64) uint8 {
	v = math.Round(v)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
