// web/frontend_interstellar_arrival_test.go
// Node-проверка прибытия межзвёздного полёта в открытой модалке (баг 2026-09-24):
// clearArrivedInterstellarFlight (снятие устаревшего признака по данным системы)
// и переключение flightModeForSystem в modal/state.js. Событие прилёта приходит
// из карты (map/data.js → checkCompositeArrival → refreshPlanets), поэтому в
// state.js проверяется именно снятие признака. Паттерн — как
// TestOwnSystemFlightModeInNode: node импортирует чистый модуль, без DOM.
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInterstellarArrivalInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node нет в PATH — пропускаю Node-проверку (напоминание, не блокер)")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(interstellarArrivalScript, "__STATE__", root+"/web/static/js/modal/state.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Node-проверка прибытия межзвёздного полёта упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "INTERSTELLAR_ARRIVAL_OK") {
		t.Fatalf("node не подтвердил проверку; вывод:\n%s", out)
	}
}

const interstellarArrivalScript = `
globalThis.window = globalThis;
globalThis.localStorage = { getItem: () => null, setItem(){}, removeItem(){} };

const state = await import(new URL("__STATE__").href);
const assert = (cond, msg) => { if (!cond) throw new Error(msg); };
const s = state.modalState;

// Композитный редирект в своей системе: позиция NULL (межзвёздный сегмент) —
// признак НЕ снимается, режим остаётся композитным.
s.interstellarFlight = { to: 'wB', start_time: 1000, duration: 5 };
s.inOwnSystem = true;
s.myPosition = null;
assert(state.clearArrivedInterstellarFlight() === false, 'межзвёздный сегмент — не снимаем');
assert(s.interstellarFlight !== null, 'признак полёта сохранён');
assert(state.flightModeForSystem() === 'composite', 'пока летим — композитный');

// Активный внутрисистемный сегмент (in_flight) — тоже не снимаем.
s.myPosition = { status: 'in_flight', to_type: 'planet', to_id: 'p1' };
assert(state.clearArrivedInterstellarFlight() === false, 'внутрисистемный полёт — не снимаем');
assert(s.interstellarFlight !== null, 'признак полёта сохранён (сегмент идёт)');

// Чужая система (in_own_system = false) — не снимаем, даже если позиция орбита.
s.inOwnSystem = false;
s.myPosition = { status: 'orbit', object_type: 'star' };
assert(state.clearArrivedInterstellarFlight() === false, 'чужая система — не снимаем');
assert(s.interstellarFlight !== null, 'признак полёта сохранён (чужая система)');

// Прибыли в целевую систему: in_own_system + позиция орбиты звезды —
// признак снимается, режим → внутрисистемный.
s.interstellarFlight = { to: 'wB', start_time: 1000, duration: 5 };
s.interstellarFlightName = 'B';
s.inOwnSystem = true;
s.myPosition = { status: 'orbit', object_type: 'star' };
assert(state.clearArrivedInterstellarFlight() === true, 'прибытие — признак снят');
assert(s.interstellarFlight === null, 'признак полёта обнулён');
assert(s.interstellarFlightName === null, 'имя цели полёта обнулено');
assert(state.flightModeForSystem() === 'intra', 'после прибытия — внутрисистемный');

// Сценарий бага целиком: модалка открыта во время полёта (позиция NULL,
// чужая система, полёт активен) → прибытие (данные системы перечитаны) →
// режим переходит в 'intra' без закрытия модалки.
state.resetState();
s.worldId = 'wB';
s.interstellarFlight = { to: 'wB', start_time: 1000, duration: 5 };
s.inOwnSystem = false;
s.myPosition = null;
assert(state.flightModeForSystem() === 'composite', 'в полёте — композитный режим');
// refreshPlanets перечитал данные системы прибытия:
s.inOwnSystem = true;
s.myPosition = { status: 'orbit', object_type: 'star' };
state.clearArrivedInterstellarFlight();
assert(state.flightModeForSystem() === 'intra', 'без закрытия модалки — внутрисистемный режим');

// resetState сбрасывает признак.
state.resetState();
assert(s.interstellarFlight === null, 'resetState сбрасывает interstellarFlight');

console.log('INTERSTELLAR_ARRIVAL_OK');
`
