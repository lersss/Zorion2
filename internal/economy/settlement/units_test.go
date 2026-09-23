// internal/economy/settlement/units_test.go
// T-Е1 (единица): конверсия «ед/сутки/млрд → батч/сек» живёт ровно в одной
// функции (PerSecond, units.go); норма 600 + население 1e9 + сутки = 600 батчей;
// литералы `1e9`/`86400` вне этой функции — красный тест (спека 2026-09-23
// §2.2/§15.2).
package settlement

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T-Е1: 600 ед/сутки/млрд при населении 1e9 за сутки = 600 батчей (единица
// модели читается буквально); конверсия линейна и обнуляется при population 0.
func TestPerSecondUnit(t *testing.T) {
	if got := PerSecond(600, 1e9) * 86400; math.Abs(got-600) > 1e-9 {
		t.Fatalf("600 ед/сутки/млрд × 1e9 × сутки = 600 батчей: got %v", got)
	}
	if got := PerSecond(600, 0); got != 0 {
		t.Fatalf("population 0 → 0: got %v", got)
	}
	if got, want := PerSecond(1200, 1e9), 2*PerSecond(600, 1e9); math.Abs(got-want) > 1e-18 {
		t.Fatalf("конверсия не линейна по единице: got %v want %v", got, want)
	}
}

// T-Е1 (единственная точка): ни один не-тестовый файл пакета, кроме units.go,
// не пересчитывает единицы — литералов `1e9`/`86400` вне функции нет.
func TestPerSecondIsSingleConversionPoint(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if name == "units.go" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		src := string(body)
		if strings.Contains(src, "1e9") || strings.Contains(src, "86400") {
			t.Fatalf("%s: литерал единицы (1e9/86400) вне PerSecond — пересчёт единиц вне единой точки (§2.2)", name)
		}
	}
}
