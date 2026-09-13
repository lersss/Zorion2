// internal/handlers/admin_tests.go
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	testsMu         sync.Mutex // защищает прогон и кэш результата
	testsLastResult *TestSummary
	testsGoVersion  string
)

// GetTestsHandler — HTTP-обработчик результатов тестов.
//
// Маршрут: GET /admin/tests
//
// Без параметра — возвращает последний результат (при первом вызове гоняет тесты).
// С ?refresh=1 — запускает `go test -json ./...` заново и возвращает свежий результат.
// Пока идёт прогон, другие запросы ждут его завершения (простые взаимные блокировки).
func (h *AdminHandlers) GetTestsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	testsMu.Lock()
	defer testsMu.Unlock()

	refresh := r.URL.Query().Get("refresh") == "1"
	if refresh || testsLastResult == nil {
		testsLastResult = runGoTests()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(testsLastResult); err != nil {
		log.Printf("❌ tests: JSON encode failed: %v", err)
	}
}

// runGoTests — запускает `go test -json ./...` в корне модуля,
// разбирает вывод и возвращает человекочитаемую сводку.
func runGoTests() *TestSummary {
	modDir := findGoModDir()
	if modDir == "" {
		// На проде исходники не развёрнуты (бинарь копируется отдельно) —
		// тесты физически не могут работать, это честный быстрый ответ.
		return errTestSummary("Исходники Go не развёрнуты (go.mod не найден). Вкладка «Тесты» доступна только там, где есть код проекта — локально или в CI.")
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "-json", "./...")
	cmd.Dir = modDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	elapsed := time.Since(start)

	if testsGoVersion == "" {
		testsGoVersion = probeGoVersion()
	}

	sum, parseErr := parseGoTestJSON(&stdout)
	sum.DurationMs = elapsed.Milliseconds()
	sum.GoVersion = testsGoVersion
	sum.Message = ""

	if parseErr != nil {
		sum.Status = "error"
		sum.Message = "Не удалось разобрать вывод go test: " + parseErr.Error()
		if s := strings.TrimSpace(stderr.String()); s != "" {
			sum.Message += " Сообщение go: " + truncate(s, maxFailureLen)
		}
		return sum
	}

	if runErr != nil {
		// Ненулевой код выхода — это либо упавшие тесты (уже отражены в сводке),
		// либо сбой запуска без структурированного вывода.
		if sum.PackagesTotal == 0 {
			sum.Status = "error"
			sum.Message = "go test завершился неудачно: " + runErr.Error()
			if s := strings.TrimSpace(stderr.String()); s != "" {
				sum.Message += " " + truncate(s, maxFailureLen)
			}
		}
	}
	return sum
}

// findGoModDir — ищет каталог с go.mod: подъёмом от рабочего каталога, затем
// от каталога исполняемого файла. Возвращает "" если модуль не найден
// (например, на проде: бинарь и web копируются без исходников).
func findGoModDir() string {
	var roots []string
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd)
	}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}

	seen := make(map[string]bool)
	for _, root := range roots {
		for dir := root; ; {
			if seen[dir] {
				break
			}
			seen[dir] = true
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return ""
}

// errTestSummary — сводка с ошибкой запуска (без выполнения go test).
func errTestSummary(message string) *TestSummary {
	return &TestSummary{
		Status:     "error",
		Available:  false,
		Message:    message,
		DurationMs: 0,
	}
}

// probeGoVersion — разовая проверка версии Go (кэшируется).
func probeGoVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}