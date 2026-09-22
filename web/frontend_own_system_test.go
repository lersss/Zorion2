// web/frontend_own_system_test.go
// Node-проверка правила «своя система» (баг 2026-09-22, спека поясов этап 2 +
// композитный маршрут 99.2.30): flightModeForSystem в modal/state.js определяет
// режим полёта по ЯВНОМУ флагу сервера inOwnSystem, а не по my_position != null.
// Паттерн — как TestBranchesRenderInNode (frontend_branches_test.go): node
// импортирует чистый модуль состояния, без DOM.
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnSystemFlightModeInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node нет в PATH — пропускаю Node-проверку (напоминание, не блокер)")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(ownSystemScript, "__STATE__", root+"/web/static/js/modal/state.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Node-проверка режима полёта упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "OWN_SYSTEM_OK") {
		t.Fatalf("node не подтвердил проверку; вывод:\n%s", out)
	}
}

const ownSystemScript = `
globalThis.window = globalThis;
globalThis.localStorage = { getItem: () => null, setItem(){}, removeItem(){} };

const state = await import(new URL("__STATE__").href);
const assert = (cond, msg) => { if (!cond) throw new Error(msg); };
const s = state.modalState;

// Баг 2026-09-22: своя система (явный флаг сервера) + пустая позиция (окно
// прибытия/межзвёздный полёт) → внутрисистемный полёт, НЕ композитный.
s.inOwnSystem = true;
s.interstellarFlight = null;
s.myPosition = null;
assert(state.flightModeForSystem() === 'intra',
    'in_own_system + пустая позиция → внутрисистемный полёт');

// Активный межзвёздный полёт в своей системе — композитный (редирект /travel).
s.interstellarFlight = { to: 'w9' };
assert(state.flightModeForSystem() === 'composite',
    'межзвёздный в своей системе → композитный');

// Чужая система (in_own_system = false) → композитный, даже если позиция есть
// (устаревшая) — my_position не является признаком «своей системы».
s.interstellarFlight = null;
s.inOwnSystem = false;
s.myPosition = { status: 'orbit', object_type: 'belt', object_id: 'b1' };
assert(state.flightModeForSystem() === 'composite',
    'чужая система → композитный, несмотря на my_position');

// resetState сбрасывает флаг.
state.resetState();
assert(s.inOwnSystem === false, 'resetState сбрасывает inOwnSystem');

console.log('OWN_SYSTEM_OK');
`
