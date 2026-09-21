// cmd/art-studio/handlers/ships_edit.go
// «Вписать в кадр» кандидата корабля (вкладка «Корабли рас»): crop по bbox +
// центрирование в 200×200. Действия ориентации (rotate/rot90/rot180/flipH/
// setangle/auto) пишут пару (A, F) в метаданные и пиксели НЕ трогают — показ
// применяет пару (студия — CSS, игра — при отрисовке, спека
// 2026-09-21-угол-корабля-в-метаданных §4). Пиксели не поворачиваются нигде,
// поэтому «вписать в кадр» — единственная операция, перезаписывающая файл.
//
// Контракт: /ships/act?file=&what=fit (перезапись файла кандидата атомарно,
// tmp + rename).
package handlers

import (
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

// transformShipImage — вписать PNG-файл кандидата в 200×200 и перезаписать его
// на месте атомарно («Вписать в кадр»).
func transformShipImage(path string) error {
	src, err := openImage(path)
	if err != nil {
		return err
	}
	return saveShipPNG(path, refitShip(src, shipSpriteSize))
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
			dst.SetRGBA(x, y, bilinearPremul(src, fx, fy))
		}
	}
	return dst
}

// bilinearPremul — билинейная выборка premultiplied RGBA в точке (fx,fy)
// (пиксельный центр — целое число). Координаты зажимаются к границе
// (масштабирование: крайние пиксели не «размываются» в полупрозрачное,
// сумма весов = 1).
func bilinearPremul(src image.Image, fx, fy float64) color.RGBA {
	b := src.Bounds()
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	tx := fx - float64(x0)
	ty := fy - float64(y0)
	var r, g, bl, a float64
	for j := 0; j <= 1; j++ {
		for i := 0; i <= 1; i++ {
			px := clampIndex(x0+i, b.Min.X, b.Max.X-1)
			py := clampIndex(y0+j, b.Min.Y, b.Max.Y-1)
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
