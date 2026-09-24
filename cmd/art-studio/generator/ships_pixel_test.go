package generator

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"zorion/cmd/art-studio/config"
)

// findPython — python студии (venv ComfyUI, затем PATH) с зависимостями
// скрипта силуэтов (PIL); пустая строка — python/зависимости недоступны,
// пиксельный тест пропускается.
func findPython() string {
	var candidates []string
	if _, err := os.Stat(`C:\ComfyUI\venv\Scripts\python.exe`); err == nil {
		candidates = append(candidates, `C:\ComfyUI\venv\Scripts\python.exe`)
	}
	if p, err := exec.LookPath("python"); err == nil {
		candidates = append(candidates, p)
	}
	for _, p := range candidates {
		if exec.Command(p, "-c", "import PIL").Run() == nil {
			return p
		}
	}
	return ""
}

// TestShipSilhouettePixel — пиксельная проверка скрипта силуэтов (спека §8
// критерий 3): 1024×1024, светлая кабина в правой половине, тёмные дюзы
// в левой, запас ~90 px от краёв. Skip, если python недоступен.
func TestShipSilhouettePixel(t *testing.T) {
	python := findPython()
	if python == "" {
		t.Skip("python недоступен — пиксельный тест пропущен")
	}
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	sil := "крыло-корпус в плане: широкий нос (светлая кабина-стекло) справа, сужающаяся корма с дюзами слева; асимметрия по оси «нос-корма»; крылья-стабилизаторы; модули: корпус крем, крылья сталь, дюзы тёмные, кабина светлая; запас от краёв ~90 px"
	spec := ParseSilhouette(sil, dc)
	specJSON, err := SilhouetteSpecJSON(spec)
	if err != nil {
		t.Fatalf("SilhouetteSpecJSON: %v", err)
	}
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "spec.json")
	outPath := filepath.Join(tmp, "humans.png")
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
	b := img.Bounds()
	if b.Dx() != 1024 || b.Dy() != 1024 {
		t.Fatalf("размер = %dx%d, want 1024×1024", b.Dx(), b.Dy())
	}
	lightRight := false // кабина (180,210,235) справа
	darkLeft := false   // дюзы (40,44,55) слева
	minX, minY, maxX, maxY := 1024, 1024, 0, 0
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			rr, gg, bb := int(r>>8), int(g>>8), int(bl>>8)
			if rr == 5 && gg == 5 && bb == 5 {
				continue // фон
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
			if rr == 180 && gg == 210 && bb == 235 && x > 512 {
				lightRight = true
			}
			if rr == 40 && gg == 44 && bb == 55 && x < 512 {
				darkLeft = true
			}
		}
	}
	if !lightRight {
		t.Errorf("нет светлой кабины в правой половине")
	}
	if !darkLeft {
		t.Errorf("нет тёмных дюз в левой половине")
	}
	// запас ~90 px (масштаб ~82% + центрирование)
	if minX < 80 || maxX > 944 || minY < 80 || maxY > 944 {
		t.Errorf("bbox = x[%d..%d] y[%d..%d], want в пределах ~90 px от краёв", minX, maxX, minY, maxY)
	}
}

// TestShipSilhouettePixelAmmonia — пиксельная проверка силуэта аммиачников
// (диагноз визуального аудита): ≥4 модуля (холодный корпус, тяжи слева,
// заплаты, иллюминатор) — не голая капля с носом. Skip, если python недоступен.
func TestShipSilhouettePixelAmmonia(t *testing.T) {
	python := findPython()
	if python == "" {
		t.Skip("python недоступен — пиксельный тест пропущен")
	}
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	sil := "корпус-«капля» в плане: скруглённый нос (светлый иней-иллюминатор) справа, корма с пучком свисающих корневых тяжей слева; на бортах — пятна гелевых заплат и иней-кромка; модули: корпус бледно-голубой, тяжи светлые, заплаты полупрозрачные, иллюминатор почти белый; асимметрия по оси «нос-корма»; запас от краёв ~90 px"
	spec := ParseSilhouette(sil, dc)
	if len(spec.Modules) < 4 {
		t.Fatalf("модулей = %d, want ≥4: %v", len(spec.Modules), spec.Modules)
	}
	specJSON, err := SilhouetteSpecJSON(spec)
	if err != nil {
		t.Fatalf("SilhouetteSpecJSON: %v", err)
	}
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "spec.json")
	outPath := filepath.Join(tmp, "ammonia.png")
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
	coldHull := false // корпус (170,205,235) — холодный бледно-голубой
	steelLeft := false
	lightLeft := false  // заплаты (180,210,235) в средней части
	lightRight := false // иллюминатор (180,210,235) справа
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			rr, gg, bb := int(r>>8), int(g>>8), int(bl>>8)
			if rr == 170 && gg == 205 && bb == 235 {
				coldHull = true
			}
			if rr == 120 && gg == 140 && bb == 170 && x < 92 {
				steelLeft = true // тяжи за кормой влево
			}
			if rr == 180 && gg == 210 && bb == 235 {
				if x < 512 {
					lightLeft = true // заплаты по бортам
				} else {
					lightRight = true // иллюминатор справа
				}
			}
		}
	}
	if !coldHull {
		t.Errorf("нет холодного корпуса (170,205,235) — корпус не параметризован")
	}
	if !steelLeft {
		t.Errorf("нет тяжей (сталь) слева от кормы")
	}
	if !lightLeft {
		t.Errorf("нет заплат (светлые) в средней части")
	}
	if !lightRight {
		t.Errorf("нет иллюминатора (светлый) справа")
	}
}

func openPNGFile(fp string) (image.Image, error) {
	f, err := os.Open(fp)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}