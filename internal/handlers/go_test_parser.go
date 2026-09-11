// internal/handlers/go_test_parser.go
package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
)

// testEvent — один JSON-объект из потока `go test -json`.
type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

// TestSummary — человекочитаемая сводка одного прогона тестов.
type TestSummary struct {
	Status        string           `json:"status"`              // success | fail | error
	Message       string           `json:"message,omitempty"`   // только при status=error
	DurationMs    int64            `json:"duration_ms"`
	GoVersion     string           `json:"go_version,omitempty"`
	PackagesTotal int              `json:"packages_total"`
	PackagesOK    int              `json:"packages_ok"`
	PackagesSkip  int              `json:"packages_skip"`
	TestsTotal    int              `json:"tests_total"`
	TestsFailed   int              `json:"tests_failed"`
	TestsSkipped  int              `json:"tests_skipped"`
	Packages      []PackageSummary `json:"packages"`
}

// PackageSummary — результат по одному пакету.
type PackageSummary struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Status     string    `json:"status"` // ok | fail | skip
	Tests      int       `json:"tests"`
	Failed     int       `json:"failed"`
	Skipped    int       `json:"skipped"`
	DurationMs int64     `json:"duration_ms"`
	Failures   []Failure `json:"failures,omitempty"`
}

// Failure — отдельный упавший тест (или ошибка сборки пакета).
type Failure struct {
	Test    string `json:"test"` // имя теста; "«пакет»" — ошибка сборки
	Message string `json:"message"`
}

const maxFailureLen = 3000

// parseGoTestJSON — разбирает поток `go test -json ./...` (JSON-строки).
// Возвращает сводку: статус, счётчики, упавшие тесты с сообщениями.
func parseGoTestJSON(r io.Reader) (*TestSummary, error) {
	pkgs := make(map[string]*pkgResult)
	order := []string{}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev testEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("строка %q: %w", truncate(line, 120), err)
		}

		p := pkgs[ev.Package]
		if p == nil {
			p = &pkgResult{
				path:    ev.Package,
				outputs: make(map[string]*strings.Builder),
			}
			pkgs[ev.Package] = p
			order = append(order, ev.Package)
		}

		switch ev.Action {
		case "run":
			if ev.Test != "" {
				p.testsRun++
			}
		case "pass":
			if ev.Test == "" {
				p.status = "ok"
				if ev.Elapsed > 0 {
					p.durationMs = int64(ev.Elapsed * 1000)
				}
			}
		case "fail":
			if ev.Test == "" {
				p.status = "fail"
				if ev.Elapsed > 0 {
					p.durationMs = int64(ev.Elapsed * 1000)
				}
			} else {
				p.failed++
				p.failedNames = append(p.failedNames, ev.Test)
			}
		case "skip":
			if ev.Test == "" {
				p.status = "skip"
				if ev.Elapsed > 0 {
					p.durationMs = int64(ev.Elapsed * 1000)
				}
			} else {
				p.skipped++
			}
		case "output":
			if ev.Output == "" {
				continue
			}
			key := ev.Test
			if key == "" {
				key = "«пакет»"
			}
			if b := p.outputs[key]; b != nil {
				b.WriteString(ev.Output)
			} else {
				p.outputs[key] = &strings.Builder{}
				p.outputs[key].WriteString(ev.Output)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sum := &TestSummary{Status: "success"}
	for _, name := range order {
		p := pkgs[name]
		p.finish()
		ps := PackageSummary{
			Path:       p.path,
			Name:       friendlyPkgName(p.path),
			Status:     p.status,
			Tests:      p.testsRun,
			Failed:     p.failed,
			Skipped:    p.skipped,
			DurationMs: p.durationMs,
		}
		sum.PackagesTotal++
		switch p.status {
		case "ok":
			sum.PackagesOK++
		case "skip":
			sum.PackagesSkip++
		case "fail":
			sum.Status = "fail"
			ps.Failures = p.failures()
		}
		sum.TestsTotal += p.testsRun
		sum.TestsFailed += p.failed
		sum.TestsSkipped += p.skipped
		sum.Packages = append(sum.Packages, ps)
	}
	return sum, nil
}

// pkgResult — агрегированное состояние пакета во время разбора.
type pkgResult struct {
	path        string
	status      string // ok | fail | skip (пусто, если нет финального события)
	testsRun    int
	failed      int
	skipped     int
	failedNames []string
	durationMs  int64
	outputs     map[string]*strings.Builder
}

// finish — пакет без финального события помечается как ok (пустышка не проверяется,
// но go test всегда шлёт финальное событие; страховка на случай обрезки вывода).
func (p *pkgResult) finish() {
	if p.status == "" {
		p.status = "ok"
	}
}

// failures — собирает сообщения для упавших тестов и, при ошибке сборки,
// сообщение пакета. Упавшие тесты сортированы по имени.
func (p *pkgResult) failures() []Failure {
	out := make([]Failure, 0, p.failed+1)
	names := append([]string(nil), p.failedNames...)
	for _, name := range names {
		out = append(out, Failure{Test: name, Message: extractFailureMessage(p.outputs[name])})
	}
	// Ошибка сборки/вето без отдельного упавшего теста
	if p.failed == 0 && p.status == "fail" {
		out = append(out, Failure{Test: "«пакет»", Message: extractFailureMessage(p.outputs["«пакет»"])})
	}
	return out
}

// extractFailureMessage — из накопленного вывода теста достаёт текст ошибки:
// отбрасывает заголовок "--- FAIL: ..." и служебные строки списка.
func extractFailureMessage(b *strings.Builder) string {
	if b == nil {
		return "тест упал (без подробностей)"
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "тест упал (без подробностей)"
	}
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "--- FAIL:") {
			continue
		}
		if strings.HasPrefix(trimmed, "=== ") {
			continue
		}
		kept = append(kept, trimmed)
	}
	msg := strings.TrimSpace(strings.Join(kept, " "))
	msg = strings.TrimSpace(stripAnsi(msg))
	if msg == "" {
		return "тест упал"
	}
	return truncate(msg, maxFailureLen)
}

// friendlyPkgName — короткий читаемый ярлык пакета: "zorion/internal/generator/planet" → "planet".
func friendlyPkgName(p string) string {
	if i := strings.Index(p, "zorion/"); i >= 0 {
		p = p[i+len("zorion/"):]
	}
	return path.Base(p)
}

// truncate — обрезка строки с многоточием.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// stripAnsi — вычищает ANSI-управляющие последовательности из вывода go test.
func stripAnsi(s string) string {
	if !strings.ContainsRune(s, '\x1b') {
		return s
	}
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}