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

// runGoTests — запускает `go test -json ./...` в каталоге сервера,
// разбирает вывод и возвращает человекочитаемую сводку.
func runGoTests() *TestSummary {
	if _, err := os.Stat("go.mod"); err != nil {
		return errTestSummary("Каталог сервера не содержит go.mod. Запустите сервер из корня проекта, чтобы вкладка «Тесты» работала.")
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "-json", "./...")
	cmd.Dir = "."

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

// errTestSummary — сводка с ошибкой запуска (без выполнения go test).
func errTestSummary(message string) *TestSummary {
	return &TestSummary{
		Status:    "error",
		Message:   message,
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