// Frontend sanity-тест админки. Ловит регрессии вида «админка не грузится»:
// модуль, попавший в import-граф от admin/main.js, падает на верхнем уровне
// (обращение к отсутствующему DOM/глобалам) и роняет все вкладки.
package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// статические импорты: import ... from 'path' и side-effect import 'path'.
// Динамический import(...) намеренно НЕ матчится: он выполняется по клику,
// а не при загрузке страницы — смежные модули (map/*) в админку не тянутся.
var staticImportRe = regexp.MustCompile(`^\s*import\s+(?:[^'"]+?\s+from\s+)?['"]([^'"]+)['"]`)

// resolveImport — приводит путь импорта к абсолютному (импорты в проекте с
// явным .js; относительные от файла-источника).
func resolveImport(fromFile, imp string) (string, bool) {
	if !strings.HasPrefix(imp, ".") {
		return "", false // bare-импорт (сторонний пакет) — в web графе нет
	}
	abs := filepath.Clean(filepath.Join(filepath.Dir(fromFile), imp))
	if !strings.HasSuffix(abs, ".js") {
		abs += ".js"
	}
	return abs, true
}

// repoRoot — корень репозитория (каталог с go.mod), от него строятся пути.
// go test запускает тест-бинарь с cwd = каталог пакета, поэтому корень
// ищем подъёмом вверх, а не из относительного пути.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod не найден выше cwd")
		}
		dir = parent
	}
}

// adminImportGraph — транзитивный список файлов, которые тянет admin/main.js.
// Второй проход не нужен: тест маленький, циклов в импортах нет (ES-модули
// дедуплицируются браузером, здесь следим за этим map'ом).
func adminImportGraph(t *testing.T) []string {
	t.Helper()
	root := filepath.Join(repoRoot(t), "web", "static", "js", "admin", "main.js")
	seen := map[string]bool{}
	var order []string
	var walk func(string)
	walk = func(f string) {
		if seen[f] {
			return
		}
		seen[f] = true
		order = append(order, f)
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("не читается модуль из графа админки: %v", err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			m := staticImportRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			abs, ok := resolveImport(f, m[1])
			if !ok {
				continue
			}
			walk(abs)
		}
	}
	walk(root)
	return order
}

// TestAdminFrontendLoadsInNode — «админка не грузится»-регрессия. Каждый
// модуль графа существует, а главный (admin/main.js) со всем графом
// выполняется в Node под DOM-стабом без верхнеуровневого падения — именно
// так падала админка: modal/events.js тянул map/flight.js → map/config.js,
// который на верхнем уровне дёргал отсутствующий в admin.html #mapCanvas.
func TestAdminFrontendLoadsInNode(t *testing.T) {
	files := adminImportGraph(t)
	if len(files) < 15 {
		t.Fatalf("граф админки подозрительно мал: %d файлов", len(files))
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение графа")
	}

	// DOM/глобал-стаб: админка — это не игровая страница, canvas карты в
	// admin.html нет. Любая попытка модуля дотянуться до него на верхнем
	// уровне должна уронить тест (TypeError от null), а не уйти в молчание.
	stub := `
globalThis.window = globalThis;
globalThis.localStorage = { getItem: () => null, setItem: () => {}, removeItem: () => {} };
const elementStub = () => ({
    style: {}, classList: { add(){}, toggle(){}, remove(){} },
    appendChild(){}, remove(){}, addEventListener(){}, removeEventListener(){},
    querySelector(){ return null; }, querySelectorAll(){ return []; },
    setAttribute(){}, textContent: '', innerHTML: '', checked: false, value: '',
    dataset: {}, getContext(){ return null; }
});
globalThis.document = {
    getElementById(){ return null; },
    querySelector(){ return null; },
    querySelectorAll(){ return []; },
    createElement(){ return elementStub(); },
    createTextNode(){ return {}; },
    addEventListener(){}, removeEventListener(){},
    head: elementStub(), body: elementStub()
};
const entry = new URL("__ENTRY__");
await import(entry.href);
console.log('ADMIN_JS_LOADED');
`
	entryURL := "file:///" + filepath.ToSlash(filepath.Join(repoRoot(t), "web", "static", "js", "admin", "main.js"))
	stub = strings.Replace(stub, "__ENTRY__", entryURL, 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", stub)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("граф админки упал при импорте (админка не грузится):\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "ADMIN_JS_LOADED") {
		t.Fatalf("node не дошёл до конца графа; вывод:\n%s", out)
	}
}

// TestAdminFrontendGraphResolves — все файлы графа существуют (защита от
// опечаток в import-путях; исполнение теста выше уже это проверяет, но этот
// тест работает и без node — например, в CI без дистрибутива).
func TestAdminFrontendGraphResolves(t *testing.T) {
	for _, f := range adminImportGraph(t) {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("модуль из графа админки не существует: %s", f)
		}
	}
}