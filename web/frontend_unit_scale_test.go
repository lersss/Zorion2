// web/frontend_unit_scale_test.go
// Node-тест единой точки конверсии масштаба единицы темпа (задача «переключатель
// масштаба единицы», спека 2026-09-23-стадии-поселения §2.1). Модуль
// web/static/js/unit_scale.js исполняется в Node без DOM/сети на верхнем уровне;
// множители «хранимое (ед/сутки/млрд) → отображаемое» — ровно здесь (Go-дубль —
// settlement.UnitScale). Паттерн — как в frontend_branches_test.go.
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnitScaleInNode — прямой/обратный перевод, «пусто/0», смена масштаба не
// меняет отправляемое значение, общий ключ localStorage и дефолт «на млрд».
func TestUnitScaleInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(unitScaleScript, "__US__", root+"/web/static/js/unit_scale.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка масштаба единицы упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "UNIT_SCALE_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const unitScaleScript = `
const U = await import(new URL("__US__").href);

function assert(cond, msg) { if (!cond) throw new Error(msg); }
const near = (a, b) => Math.abs(a - b) <= 1e-6 * Math.max(1, Math.abs(b));

// Прямой перевод 600 ед/сутки/млрд по четырём масштабам.
assert(near(U.storedToDisplay(600, 'billion'), 600), 'на млрд ×1');
assert(near(U.storedToDisplay(600, 'mega'), 0.6), 'на 10⁶ ×1e-3');
assert(near(U.storedToDisplay(600, 'kilo'), 0.0006), 'на 1000 ×1e-6');
assert(near(U.storedToDisplay(600, 'person'), 6e-7), 'на человека ×1e-9');

// Ввод 0.6 в режиме «на человека» → на сервер уходит хранимое 6·10⁸.
assert(near(U.displayToStored(0.6, 'person'), 6e8), 'ввод 0.6 «на человека» → 6e8');

// Пусто (отдельно в студии) и 0: ноль остаётся нулём в любом масштабе.
assert(U.displayToStored(0, 'person') === 0, 'ввод 0 → 0');
assert(U.storedToDisplay(0, 'kilo') === 0, 'хранимое 0 → 0');

// Смена масштаба не меняет отправляемое значение: хранимое → показ → хранимое.
for (const k of ['person', 'kilo', 'mega', 'billion']) {
    assert(near(U.displayToStored(U.storedToDisplay(667, k), k), 667), 'roundtrip ' + k);
}

// Общий ключ localStorage и дефолт «на млрд» (прежний вид настроек).
assert(U.UNIT_SCALE_KEY === 'gs_unitScale', 'общий ключ localStorage');
assert(U.DEFAULT_UNIT_SCALE === 'billion', 'дефолт — на млрд');
assert(U.readUnitScale({ getItem: () => 'kilo' }) === 'kilo', 'чтение выбранного масштаба');
assert(U.readUnitScale({ getItem: () => 'мусор' }) === 'billion', 'неизвестный → дефолт');
assert(U.readUnitScale(null) === 'billion', 'нет storage → дефолт');

// Явные подписи единиц (задача A4).
const units = U.UNIT_SCALE_LIST.map(s => s.unit).join('|');
assert(units.includes('1 чел') && units.includes('1 000') && units.includes('10⁶') && units.includes('10⁹'), 'подписи единиц: ' + units);

console.log('UNIT_SCALE_OK');
`
