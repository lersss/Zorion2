// Package postproc — пост-обработка аватаров (спека 67a.1 §7):
// rembg-подпроцесс, крупнейший связный компонент (BFS), ресайз 200×200
// (центр/низ), композит на кабину, обрезка снизу.
package postproc

import (
	"image"
	"image/color"
)

// LargestComponent оставляет крупнейший связный компонент (4-связность)
// по альфа-маске (alpha > alphaThreshold), остальные компоненты обнуляет
// (убирает «мусор»/фон-хвосты; перенос crop_bottom/прототипа, спека §7.2).
func LargestComponent(img *image.NRGBA, alphaThreshold uint8) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return
	}
	mask := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			mask[y*w+x] = img.NRGBAAt(b.Min.X+x, b.Min.Y+y).A > alphaThreshold
		}
	}
	visited := make([]bool, w*h)
	var best []int
	for i := 0; i < w*h; i++ {
		if !mask[i] || visited[i] {
			continue
		}
		comp := bfsComponent(mask, visited, i, w, h)
		if len(comp) > len(best) {
			best = comp
		}
	}
	keep := make([]bool, w*h)
	for _, i := range best {
		keep[i] = true
	}
	if len(best) == 0 {
		return
	}
	// обнуляем всё, что не в крупнейшем компоненте (включая полупрозрачный
	// «мусор»/фон-хвосты — как прототип: a[labels != keep_i] = 0)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !keep[y*w+x] {
				img.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{0, 0, 0, 0})
			}
		}
	}
}

func bfsComponent(mask, visited []bool, start, w, h int) []int {
	queue := []int{start}
	visited[start] = true
	comp := []int{}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		comp = append(comp, i)
		x, y := i%w, i/w
		for _, nb := range [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
			nx, ny := nb[0], nb[1]
			if nx < 0 || ny < 0 || nx >= w || ny >= h {
				continue
			}
			ni := ny*w + nx
			if mask[ni] && !visited[ni] {
				visited[ni] = true
				queue = append(queue, ni)
			}
		}
	}
	return comp
}