package generator

import (
	"os"
	"path/filepath"
	"testing"
)

// Кроссплатформенный фейковый python (helper-процесс): роль python исполняет
// сам тест-бинарник, режим задаётся env. Вместо Windows-батника (.cmd), который
// Linux не запускает, — один код эмуляции на обеих ОС, без шелла.
const (
	fakePythonEnv      = "ART_STUDIO_FAKE_PYTHON"
	fakePythonFrameEnv = "ART_STUDIO_FAKE_FRAME"
)

// TestMain перехватывает повторный запуск тест-бинарника в роли фейкового
// python (env непустой): выполняет эмуляцию и завершает процесс до m.Run.
// Внимание: режим фейка задаёт env-скоуп t.Setenv — если джоб-горутина
// переживёт конец теста, дочерний тест-бинарник без env отработает как полный
// прогон тестов (рекурсия), а не как фейк. Поэтому job-тесты обязаны
// дожидаться завершения джоба.
func TestMain(m *testing.M) {
	if mode := os.Getenv(fakePythonEnv); mode != "" {
		os.Exit(fakePythonMain(mode))
	}
	os.Exit(m.Run())
}

// fakePythonMain — эмуляция python джоба кораблей (аргументы как у реального
// вызова: os.Args[1] — скрипт, [2] — вход, [3] — третий аргумент):
// ветка `--frame-check` пишет отчёт кадра в --report-файл (os.Args[5]),
// иначе копирует вход (os.Args[2]) в выход (os.Args[3]).
func fakePythonMain(mode string) int {
	if mode != "ship" {
		return 0
	}
	if len(os.Args) > 3 && os.Args[3] == "--frame-check" {
		frame := `{"touch":[],"elong":2.0,"ok":true}`
		if os.Getenv(fakePythonFrameEnv) == "bad" {
			frame = `{"touch":["left"],"elong":1.0,"ok":false}`
		}
		if len(os.Args) > 5 {
			_ = os.WriteFile(os.Args[5], []byte(frame), 0o644)
		}
		return 0
	}
	if len(os.Args) > 3 {
		_ = copyFile(os.Args[2], os.Args[3])
	}
	return 0
}

// fakePythonCmd — путь к самому тест-бинарнику: он же запускается как python
// (helper-процесс). Абсолютный — чтобы exec нашёл его из любого рабочего каталога.
func fakePythonCmd() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	if p, err := filepath.Abs(os.Args[0]); err == nil {
		return p
	}
	return os.Args[0]
}
