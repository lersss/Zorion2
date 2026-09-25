package handlers

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"zorion/internal/models"
)

// Кроссплатформенный фейковый python (helper-процесс): роль python исполняет
// сам тест-бинарник, режим задаётся env. Вместо Windows-батников (.cmd), которые
// Linux не запускает, — один код эмуляции на обеих ОС, без шелла.
const (
	fakePythonEnv       = "ART_STUDIO_FAKE_PYTHON"
	fakePythonFrameEnv  = "ART_STUDIO_FAKE_FRAME"
	fakePythonOrientEnv = "ART_STUDIO_FAKE_ORIENT"
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
	// Реестр кораблей игры — файл данных (спека 2026-09-25 §3.5): витрина
	// «В игре» читает непустой реестр.
	if err := models.LoadShipRegistry("../../../config/ships_registry.json"); err != nil {
		panic("LoadShipRegistry: " + err.Error())
	}
	os.Exit(m.Run())
}

// fakePythonMain — эмуляция python-подпроцессов арт-студии по режиму env:
//
//	ship   — отчёт кадра (--frame-check) или копия входа в выход;
//	orient — отчёт profile_orientation в --report-файл;
//	copy   — копия входа в выход с ретраями (эмуляция rembg);
//	slow   — «зависший» python (проверка таймаута /ships/auto).
func fakePythonMain(mode string) int {
	switch mode {
	case "ship":
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
			_ = fakeCopy(os.Args[2], os.Args[3])
		}
	case "orient":
		if len(os.Args) > 5 {
			_ = os.WriteFile(os.Args[5], []byte(`{"orient": `+os.Getenv(fakePythonOrientEnv)+`}`), 0o644)
		}
	case "copy":
		if len(os.Args) > 3 && !fakeCopyRetry(os.Args[2], os.Args[3]) {
			return 1
		}
	case "slow":
		time.Sleep(3 * time.Second)
	}
	return 0
}

// fakeCopy — копирование файла (аналог `copy %2 %3`).
func fakeCopy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// fakeCopyRetry — копия с ретраями: свежесозданный файл может быть мгновенно
// недоступен (Windows Defender сканирует) — copy даёт транзиентную ошибку.
func fakeCopyRetry(src, dst string) bool {
	for i := 0; i < 20; i++ {
		if err := fakeCopy(src, dst); err == nil {
			if _, err := os.Stat(dst); err == nil {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
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
