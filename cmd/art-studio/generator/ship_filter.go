package generator

import (
	"image"
	"image/png"
	"os"
)

// ShipCandidateLabels — метки авто-фильтра кандидата (страховка, НЕ
// авто-отклонение; диагноз визуального аудита, п.7): (а) доля тёплых пикселей
// у холодных рас → «палитра»; (б) масса справа < 55% → «форма» (нос не
// читается); (в) уникальных цветов < 1000 → «текстура». Метки показываются
// на превью. Прозрачные пиксели (фон) не считаются.
func ShipCandidateLabels(img image.Image, cold bool) []string {
	b := img.Bounds()
	total, warm, right := 0, 0, 0
	colors := map[[3]byte]bool{}
	midX := (b.Min.X + b.Max.X) / 2
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a>>8 == 0 {
				continue // прозрачный фон
			}
			rr, gg, bb := int(r>>8), int(g>>8), int(bl>>8)
			total++
			colors[[3]byte{byte(rr), byte(gg), byte(bb)}] = true
			if rr > 180 && gg > 80 && gg < 190 && bb < 120 {
				warm++ // оранжевые px (art_ships §3.4)
			}
			if x >= midX {
				right++
			}
		}
	}
	if total == 0 {
		return nil
	}
	var labels []string
	if cold && float64(warm)/float64(total) > 0.05 {
		labels = append(labels, "палитра")
	}
	if float64(right)/float64(total) < 0.55 {
		labels = append(labels, "форма")
	}
	if len(colors) < 1000 {
		labels = append(labels, "текстура")
	}
	return labels
}

// ShipCandidateLabelsFile — метки кандидата из PNG-файла (после process_ship).
// Ошибка чтения/декодирования → nil (метки — страховка, не блокер).
func ShipCandidateLabelsFile(path string, cold bool) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil
	}
	return ShipCandidateLabels(img, cold)
}

// specColdHull — true, если корпус расы холодный (цвет корпуса cold_hull по
// цветовым словам ТЗ) — для метки «палитра» (тёплые пиксели у холодной расы).
func specColdHull(spec SilhouetteSpec) bool {
	for _, m := range spec.Modules {
		if m.Type == "hull" && m.Color == "cold_hull" {
			return true
		}
	}
	return false
}