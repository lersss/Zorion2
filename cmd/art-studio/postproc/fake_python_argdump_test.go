package postproc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Кроссплатформенный фейковый python (helper-процесс): роль python исполняет
// сам тест-бинарник, режим задаётся env. Вместо Windows-батника (.cmd), который
// Linux не запускает, — один код эмуляции на обеих ОС, без шелла.
const (
	fakePythonEnv    = "ART_STUDIO_FAKE_PYTHON"
	fakePythonOutEnv = "ART_STUDIO_FAKE_OUT"
)

// TestMain перехватывает повторный запуск тест-бинарника в роли фейкового
// python (env непустой): выполняет эмуляцию и завершает процесс до m.Run —
// тестовый фреймворк не трогает python-аргументы.
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

// fakePythonMain — эмуляция вызовов python для тестов пакета: дамп argv
// (аналог `echo %*`) в файл из env.
func fakePythonMain(mode string) int {
	if mode == "argdump" {
		_ = os.WriteFile(os.Getenv(fakePythonOutEnv), []byte(strings.Join(os.Args[1:], " ")), 0o644)
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
