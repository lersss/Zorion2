// Тесты вкладки «Тесты» (admin_tests.go, B14).
package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findGoModDir находит корень модуля подъёмом от cwd — в тесте это пакет
// internal/handlers, корень репозитория находится подъёмом на два уровня.
func TestFindGoModDir(t *testing.T) {
	dir := findGoModDir()
	if dir == "" {
		t.Fatal("go.mod должен находиться подъёмом от рабочего каталога (в репозитории он есть)")
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("найденный каталог %q не содержит go.mod: %v", dir, err)
	}
}

// errTestSummary помечает тесты недоступными (prod без исходников).
func TestErrTestSummaryUnavailable(t *testing.T) {
	s := errTestSummary("нет исходников")
	if s.Status != "error" {
		t.Fatalf("status = %q, ожидалось error", s.Status)
	}
	if s.Available {
		t.Fatal("сводка ошибки запуска должна помечать тесты недоступными")
	}
	if s.Message != "нет исходников" {
		t.Fatalf("message = %q", s.Message)
	}
}

// Успешный разбор go test — тесты доступны (Available=true).
func TestParseGoTestJSONAvailable(t *testing.T) {
	input := `{"Action":"pass","Package":"zorion/internal/handlers","Elapsed":0.02}
`
	sum, err := parseGoTestJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseGoTestJSON: %v", err)
	}
	if !sum.Available {
		t.Fatal("успешный прогон должен помечать тесты доступными")
	}
}