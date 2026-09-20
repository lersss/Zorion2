package generator

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"zorion/cmd/art-studio/config"
)

// Пиксельные проверки выразительности форм силуэтов кораблей рас
// (диагноз визуального аудита, 2026-09-20): каждая из 13 форм — характерный
// силуэт (не овал): масса справа заметно ≠ 50% (диапазон на форму — в
// комментарии формы в tools/make_ship_silhouettes.py), форма-специфичные
// маркеры, запас ~90 px от краёв. Skip, если python недоступен.

// Геометрия скрипта (tools/make_ship_silhouettes.py): холст 1024×1024,
// корабль x[92..931] (W=839), y[282..742] (H=460).
const (
	formW = 839
	formH = 460
	formX = 92
	formY = 282
)

var (
	colSteel  = [3]int{120, 140, 170} // крылья
	colBronze = [3]int{140, 90, 45}   // хвост/оперение
)

// fx/fy — координата по доле ширины/высоты корабля (как в скрипте: X0 + W*frac).
func fx(frac float64) int { return formX + int(float64(formW)*frac) }
func fy(frac float64) int { return formY + int(float64(formH)*frac) }

// renderShipSpec — рендер силуэта скриптом (spec → PNG → image.Image).
// Skip, если python недоступен.
func renderShipSpec(t *testing.T, spec SilhouetteSpec) image.Image {
	t.Helper()
	python := findPython()
	if python == "" {
		t.Skip("python недоступен — пиксельный тест пропущен")
	}
	specJSON, err := SilhouetteSpecJSON(spec)
	if err != nil {
		t.Fatalf("SilhouetteSpecJSON: %v", err)
	}
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "spec.json")
	outPath := filepath.Join(tmp, "form.png")
	if err := os.WriteFile(specPath, specJSON, 0644); err != nil {
		t.Fatalf("WriteFile spec: %v", err)
	}
	cmd := exec.Command(python, "../../../tools/make_ship_silhouettes.py", "--spec="+specPath, "--out="+outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make_ship_silhouettes: %v: %s", err, out)
	}
	img, err := openPNGFile(outPath)
	if err != nil {
		t.Fatalf("openPNG: %v", err)
	}
	return img
}

// renderShipForm — рендер формы без модулей (проверка самой формы).
func renderShipForm(t *testing.T, form string) image.Image {
	t.Helper()
	return renderShipSpec(t, SilhouetteSpec{Form: form})
}

// isBG — пиксель фона (5,5,5).
func isBG(img image.Image, x, y int) bool {
	r, g, bl, _ := img.At(x, y).RGBA()
	return int(r>>8) == 5 && int(g>>8) == 5 && int(bl>>8) == 5
}

// formMetrics — масса справа (%), bbox не-фона.
func formMetrics(img image.Image) (massRightPct float64, minX, minY, maxX, maxY int) {
	b := img.Bounds()
	minX, minY, maxX, maxY = b.Max.X, b.Max.Y, 0, 0
	var n, nRight int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isBG(img, x, y) {
				continue
			}
			n++
			if x > 512 {
				nRight++
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if n == 0 {
		return 0, minX, minY, maxX, maxY
	}
	return 100.0 * float64(nRight) / float64(n), minX, minY, maxX, maxY
}

// hasColorIn — есть ли пиксель цвета color в регионе [x0..x1]×[y0..y1].
func hasColorIn(img image.Image, color [3]int, x0, y0, x1, y1 int) bool {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if int(r>>8) == color[0] && int(g>>8) == color[1] && int(bl>>8) == color[2] {
				return true
			}
		}
	}
	return false
}

// widthAt — вертикальная протяжённость корпуса (не-фон) в колонке x.
func widthAt(img image.Image, x int) int {
	top, bot := -1, -1
	for y := 0; y < 1024; y++ {
		if isBG(img, x, y) {
			continue
		}
		if top < 0 {
			top = y
		}
		bot = y
	}
	if top < 0 {
		return 0
	}
	return bot - top + 1
}

// rowWidth — горизонтальная протяжённость корпуса в строке y.
func rowWidth(img image.Image, y int) int {
	left, right := -1, -1
	for x := 0; x < 1024; x++ {
		if isBG(img, x, y) {
			continue
		}
		if left < 0 {
			left = x
		}
		right = x
	}
	if left < 0 {
		return 0
	}
	return right - left + 1
}

// scanlineRuns — число отдельных сегментов корпуса на строке y.
func scanlineRuns(img image.Image, y int) int {
	runs, inRun := 0, false
	for x := 0; x < 1024; x++ {
		fg := !isBG(img, x, y)
		if fg && !inRun {
			runs++
			inRun = true
		} else if !fg {
			inRun = false
		}
	}
	return runs
}

// fillRatio — доля заполнения bbox не-фоном, % (рой: не сплошной блоб).
func fillRatio(img image.Image) float64 {
	_, minX, minY, maxX, maxY := formMetrics(img)
	area := (maxX - minX + 1) * (maxY - minY + 1)
	if area == 0 {
		return 0
	}
	var n int
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if !isBG(img, x, y) {
				n++
			}
		}
	}
	return 100.0 * float64(n) / float64(area)
}

// TestShipSilhouetteForms — все 13 форм: масса справа в целевом диапазоне
// (заметно ≠ 50%), форма-специфичные маркеры, запас ~90 px от краёв.
func TestShipSilhouetteForms(t *testing.T) {
	cases := []struct {
		name    string
		form    string
		massMin float64
		massMax float64
		check   func(t *testing.T, img image.Image)
	}{
		{
			// крыло: стреловидный корпус + стальные крылья с размахом по Y
			name: "wing", form: "wing", massMin: 35, massMax: 50,
			check: func(t *testing.T, img image.Image) {
				_, _, minY, _, maxY := formMetrics(img)
				if maxY-minY < 410 {
					t.Errorf("wing: y-протяжённость = %d, want ≥ 410 (40%% холста)", maxY-minY)
				}
				if !hasColorIn(img, colSteel, 0, 0, 1023, 400) {
					t.Errorf("wing: нет стальных крыльев выше оси корпуса (y<400)")
				}
				if !hasColorIn(img, colSteel, 0, 620, 1023, 1023) {
					t.Errorf("wing: нет стальных крыльев ниже оси корпуса (y>620)")
				}
			},
		},
		{
			// капсула: широкий корпус-корма слева + чёткий нос-конус справа
			name: "capsule", form: "capsule", massMin: 35, massMax: 50,
			check: func(t *testing.T, img image.Image) {
				wl, wr := widthAt(img, fx(0.1)), widthAt(img, fx(0.9))
				if wl <= wr {
					t.Errorf("capsule: нос-конус не читается: ширина слева %d ≤ справа %d", wl, wr)
				}
			},
		},
		{
			// баржа: широкая плоскодонная корма + стальная надстройка (второй ярус)
			name: "barge", form: "barge", massMin: 40, massMax: 55,
			check: func(t *testing.T, img image.Image) {
				if !hasColorIn(img, colSteel, fx(0.2), fy(0.35), fx(0.55), fy(0.65)) {
					t.Errorf("barge: нет стальной надстройки (второй ярус) в средней части")
				}
			},
		},
		{
			// тигель: расширение кверху, уши-ручки по бокам, нос-выступ справа
			name: "crucible", form: "crucible", massMin: 52, massMax: 70,
			check: func(t *testing.T, img image.Image) {
				if !hasColorIn(img, colSteel, fx(0.42), formY, fx(0.58), fy(0.2)) {
					t.Errorf("crucible: нет уха-ручки (сталь) сверху")
				}
				if !hasColorIn(img, colSteel, fx(0.42), fy(0.8), fx(0.58), formY+formH) {
					t.Errorf("crucible: нет уха-ручки (сталь) снизу")
				}
				wt, wb := rowWidth(img, fy(0.3)), rowWidth(img, fy(0.7))
				if wt <= wb {
					t.Errorf("crucible: расширение кверху не читается: ширина верха %d ≤ низа %d", wt, wb)
				}
			},
		},
		{
			// оболочка: округлый баллон + бронзовое хвостовое оперение слева
			name: "envelope", form: "envelope", massMin: 35, massMax: 50,
			check: func(t *testing.T, img image.Image) {
				if !hasColorIn(img, colBronze, 0, 0, 250, 400) {
					t.Errorf("envelope: нет бронзового оперения слева сверху")
				}
				if !hasColorIn(img, colBronze, 0, 620, 250, 1023) {
					t.Errorf("envelope: нет бронзового оперения слева снизу")
				}
			},
		},
		{
			// сосуд: горлышко-нос справа, расширяющееся тело, узкое дно слева
			name: "vessel", form: "vessel", massMin: 35, massMax: 50,
			check: func(t *testing.T, img image.Image) {
				wl, wr := widthAt(img, fx(0.1)), widthAt(img, fx(0.9))
				if wl <= wr {
					t.Errorf("vessel: горлышко-нос не читается: ширина слева %d ≤ справа %d", wl, wr)
				}
			},
		},
		{
			// колба: широкое тело, узкое горлышко-нос, плоское дно слева
			name: "flask", form: "flask", massMin: 38, massMax: 55,
			check: func(t *testing.T, img image.Image) {
				wl, wr := widthAt(img, fx(0.15)), widthAt(img, fx(0.9))
				if wl <= wr {
					t.Errorf("flask: плоское дно/горлышко не читаются: ширина слева %d ≤ справа %d", wl, wr)
				}
			},
		},
		{
			// капля: заострённый нос справа, широкая корма слева
			name: "drop", form: "drop", massMin: 35, massMax: 50,
			check: func(t *testing.T, img image.Image) {
				wl, wr := widthAt(img, fx(0.1)), widthAt(img, fx(0.9))
				if wl <= wr {
					t.Errorf("drop: заострённый нос не читается: ширина слева %d ≤ справа %d", wl, wr)
				}
			},
		},
		{
			// клин: треугольный в плане — широкая корма слева, острый нос справа
			name: "wedge", form: "wedge", massMin: 35, massMax: 52,
			check: func(t *testing.T, img image.Image) {
				wl, wr := widthAt(img, fx(0.3)), widthAt(img, fx(0.9))
				if wl <= wr {
					t.Errorf("wedge: треугольность не читается: ширина слева %d ≤ справа %d", wl, wr)
				}
			},
		},
		{
			// диск: сплюснутый, выступ-кабина в носу справа
			name: "disc", form: "disc", massMin: 35, massMax: 52,
			check: func(t *testing.T, img image.Image) {
				_, _, minY, _, maxY := formMetrics(img)
				if maxY-minY >= 256 {
					t.Errorf("disc: сплюснутость не читается: y-протяжённость = %d, want < 256 (25%% холста)", maxY-minY)
				}
				if !hasColorIn(img, colSteel, fx(0.75), fy(0.3), fx(0.98), fy(0.7)) {
					t.Errorf("disc: нет выступа-кабины (сталь) в носу справа")
				}
			},
		},
		{
			// кольцо: круг-обод с дыркой, ступица-корпус, нос-выступ справа
			name: "ring", form: "ring", massMin: 48, massMax: 65,
			check: func(t *testing.T, img image.Image) {
				if !isBG(img, 512, 400) {
					t.Errorf("ring: нет дырки (фон) внутри контура")
				}
				if isBG(img, 512, 512) {
					t.Errorf("ring: нет ступицы-корпуса в центре")
				}
			},
		},
		{
			// шар: круглый корпус + кабина-выступ справа + хвостовые стабилизаторы слева
			name: "sphere", form: "sphere", massMin: 50, massMax: 68,
			check: func(t *testing.T, img image.Image) {
				if !hasColorIn(img, colSteel, fx(0.85), fy(0.3), fx(0.98), fy(0.7)) {
					t.Errorf("sphere: нет кабины-выступа (сталь) в носу справа")
				}
				if !hasColorIn(img, colBronze, 0, 0, 250, 400) {
					t.Errorf("sphere: нет хвостового стабилизатора (бронза) слева сверху")
				}
				if !hasColorIn(img, colBronze, 0, 620, 250, 1023) {
					t.Errorf("sphere: нет хвостового стабилизатора (бронза) слева снизу")
				}
			},
		},
		{
			// рой: россыпь мелких модулей (не сплошной блоб), плотнее к носу
			name: "swarm", form: "swarm", massMin: 50, massMax: 70,
			check: func(t *testing.T, img image.Image) {
				if runs := scanlineRuns(img, 512); runs < 3 {
					t.Errorf("swarm: россыпь не читается: %d сегментов на средней строке, want ≥ 3", runs)
				}
				if fr := fillRatio(img); fr >= 50 {
					t.Errorf("swarm: сплошной блоб: заполненность bbox = %.1f%%, want < 50%%", fr)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := renderShipForm(t, tc.form)
			mass, minX, minY, maxX, maxY := formMetrics(img)
			if mass < tc.massMin || mass > tc.massMax {
				t.Errorf("%s: масса справа = %.1f%%, want [%.0f..%.0f]%%", tc.form, mass, tc.massMin, tc.massMax)
			}
			// запас ~90 px от краёв (масштаб ~82% + центрирование)
			if minX < 80 || maxX > 944 || minY < 80 || maxY > 944 {
				t.Errorf("%s: bbox = x[%d..%d] y[%d..%d], want в пределах ~90 px от краёв", tc.form, minX, maxX, minY, maxY)
			}
			tc.check(t, img)
		})
	}
}

// TestShipSilhouetteHumansWingRegression — регрессия humans («крыло»): форма
// читается как стреловидное крыло, не овал (диагноз визуального аудита):
// масса справа в целевом диапазоне, y-протяжённость ≥ 40% холста, стальные
// крылья выше/ниже оси корпуса. Skip, если python недоступен.
func TestShipSilhouetteHumansWingRegression(t *testing.T) {
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	sil := "крыло-корпус в плане: широкий нос (светлая кабина-стекло) справа, сужающаяся корма с дюзами слева; асимметрия по оси «нос-корма»; крылья-стабилизаторы; модули: корпус крем, крылья сталь, дюзы тёмные, кабина светлая; запас от краёв ~90 px"
	spec := ParseSilhouette(sil, dc)
	if spec.Form != "wing" {
		t.Fatalf("форма = %q, want wing", spec.Form)
	}
	img := renderShipSpec(t, spec)
	mass, _, minY, _, maxY := formMetrics(img)
	if mass < 35 || mass > 50 {
		t.Errorf("humans: масса справа = %.1f%%, want [35..50]%%", mass)
	}
	if maxY-minY < 410 {
		t.Errorf("humans: y-протяжённость = %d, want ≥ 410 (40%% холста)", maxY-minY)
	}
	if !hasColorIn(img, colSteel, 0, 0, 1023, 400) {
		t.Errorf("humans: нет стальных крыльев выше оси корпуса")
	}
	if !hasColorIn(img, colSteel, 0, 620, 1023, 1023) {
		t.Errorf("humans: нет стальных крыльев ниже оси корпуса")
	}
}