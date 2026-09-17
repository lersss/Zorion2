package postproc

import (
	"image"
	"image/draw"
	"os"

	"github.com/disintegration/imaging"
)

// CompositeOnCockpit накладывает прозрачный PNG на кабину (cockpit_1024.png
// в корне пула; спека 67a.1 §7.5). Кабина ресайзится под размер PNG.
// Если кабины нет — возвращает исходное изображение (как прототип).
func CompositeOnCockpit(person image.Image, cockpitPath string) image.Image {
	if cockpitPath == "" {
		return person
	}
	f, err := os.Open(cockpitPath)
	if err != nil {
		return person
	}
	defer f.Close()
	bg, _, err := image.Decode(f)
	if err != nil {
		return person
	}
	pr := person.Bounds()
	br := bg.Bounds()
	if br.Dx() != pr.Dx() || br.Dy() != pr.Dy() {
		bg = imaging.Resize(bg, pr.Dx(), pr.Dy(), imaging.Lanczos)
	}
	dst := image.NewNRGBA(bg.Bounds())
	draw.Draw(dst, dst.Bounds(), bg, bg.Bounds().Min, draw.Src)
	draw.Draw(dst, pr, person, pr.Min, draw.Over)
	return dst
}