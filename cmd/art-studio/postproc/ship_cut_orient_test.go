package postproc

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// findStudioPython — python студии (venv ComfyUI, затем PATH) с зависимостями
// скрипта кораблей (PIL/numpy/scipy); пустая строка — python/зависимости
// недоступны, пиксельный тест пропускается.
func findStudioPython() string {
	var candidates []string
	if _, err := os.Stat(`C:\ComfyUI\venv\Scripts\python.exe`); err == nil {
		candidates = append(candidates, `C:\ComfyUI\venv\Scripts\python.exe`)
	}
	if p, err := exec.LookPath("python"); err == nil {
		candidates = append(candidates, p)
	}
	for _, p := range candidates {
		if exec.Command(p, "-c", "import PIL, numpy, scipy").Run() == nil {
			return p
		}
	}
	return ""
}

// TestShipSpriteCutNoOrient — вырез с --no-orient не доворачивает пиксели, но
// отчёт всё равно несёт подсказку profile_orientation (спека §5, §9 п.7).
// Skip, если python/зависимости недоступны.
func TestShipSpriteCutNoOrient(t *testing.T) {
	python := findStudioPython()
	if python == "" {
		t.Skip("python (PIL/numpy/scipy) недоступен — тест выреза пропущен")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "raw.png")
	writeDiagonalShip(t, src)

	run := func(noOrient bool) (orientAngle float64, out []byte) {
		t.Helper()
		outPath := filepath.Join(dir, map[bool]string{true: "no.png", false: "yes.png"}[noOrient])
		report := filepath.Join(dir, map[bool]string{true: "no.json", false: "yes.json"}[noOrient])
		args := []string{"../../../tools/ship_sprite_cut.py", src, outPath, "--method", "hyst",
			"--tol-close", "12", "--tol-wide", "40", "--fill-holes", "--report", report}
		if noOrient {
			args = append(args, "--no-orient")
		}
		if b, err := exec.Command(python, args...).CombinedOutput(); err != nil {
			t.Fatalf("ship_sprite_cut: %v: %s", err, b)
		}
		data, err := os.ReadFile(report)
		if err != nil {
			t.Fatalf("ReadFile report: %v", err)
		}
		var rep struct {
			Orient struct {
				Angle  float64 `json:"angle"`
				Reason string  `json:"reason"`
			} `json:"orient"`
		}
		if err := json.Unmarshal(data, &rep); err != nil {
			t.Fatalf("json отчёта: %v", err)
		}
		if rep.Orient.Reason == "" {
			t.Errorf("отчёт без подсказки orient (reason пуст)")
		}
		b, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("ReadFile out: %v", err)
		}
		return rep.Orient.Angle, b
	}

	noAngle, noOut := run(true)
	yesAngle, yesOut := run(false)
	if noAngle != yesAngle {
		t.Errorf("подсказка orient ≠ между --no-orient и обычным вырезом: %v vs %v", noAngle, yesAngle)
	}
	if noAngle == 0 {
		t.Errorf("подсказка orient пуста (angle 0) — profile_orientation не сработал")
	}
	if string(noOut) == string(yesOut) {
		t.Errorf("--no-orient дал тот же спрайт, что обычный вырез — доворот не отключён")
	}
}

// writeDiagonalShip — тестовый кадр: диагональная полоса (угол ~30°) на
// прозрачном фоне — вырез и profile_orientation дают ненулевой угол.
func writeDiagonalShip(t *testing.T, path string) {
	t.Helper()
	const size = 160
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	rad := 30 * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-size/2, float64(y)-size/2
			u := dx*cos + dy*sin  // вдоль полосы
			v := -dx*sin + dy*cos // поперёк
			if math.Abs(u) <= 60 && math.Abs(v) <= 10 {
				img.Set(x, y, color.NRGBA{R: 240, G: 240, B: 240, A: 255})
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
}
