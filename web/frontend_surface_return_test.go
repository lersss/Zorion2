// web/frontend_surface_return_test.go
// Node-тест возврата с прогулки (идея 2026-10-02, МЧК1): смерть и «вызвать
// корабль» — один исход (спека высадки §6.3/И4). Оба выхода идут через общий
// `returnToMap` (web/static/js/surface/surface_return.js): он ставит одноразовый
// маркер `surfaceReturn` = id планеты прогулки (его читает карта — map/data.js) и
// уходит на `/map`. Проверяем поведением (порядок «маркер → редирект») и тем, что
// оба выхода в surface_main.js идут через этот общий хелпер, а маркер ставится в
// момент редиректа (внутри колбэка экрана смерти), а не при его показе — иначе
// маркер «залипает», если игрок закрыл вкладку, не нажав «продолжить».
package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSurfaceReturnMarkerInNode — поведение общего выхода с прогулки.
func TestSurfaceReturnMarkerInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(surfaceReturnScript, "__RETURN_MODULE__", root+"/web/static/js/surface/surface_return.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка возврата с прогулки упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "SURFACE_RETURN_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const surfaceReturnScript = `
// Модуль читает глобалы в момент вызова — подменяем их заранее.
function installEnv(log, storage) {
    globalThis.sessionStorage = storage;
    const loc = { _href: '' };
    Object.defineProperty(loc, 'href', {
        get() { return loc._href; },
        set(v) { loc._href = v; log.push('redirect:' + v); },
    });
    globalThis.window = { location: loc };
    return loc;
}

const r = await import(new URL("__RETURN_MODULE__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}

const okStore = (log) => {
    const m = new Map();
    return {
        setItem(k, v) { log.push('setItem:' + k + '=' + String(v)); m.set(k, String(v)); },
        getItem(k) { return m.has(k) ? m.get(k) : null; },
    };
};

// R1. Обычный уход: маркер записан ДО редиректа, значение — строковый id планеты.
const log = [];
const store = okStore(log);
const loc = installEnv(log, store);
r.returnToMap('859a267f-1111-2222-3333-444455556666');
eq('R1 order', log, ['setItem:surfaceReturn=859a267f-1111-2222-3333-444455556666', 'redirect:/map']);
eq('R1 marker value', store.getItem('surfaceReturn'), '859a267f-1111-2222-3333-444455556666');
eq('R1 redirect', loc.href, '/map');

// R2. Id приходит не строкой (число/объект UUID-подобный) — в маркер кладём строку.
const log2 = [];
const store2 = okStore(log2);
installEnv(log2, store2);
r.returnToMap(42);
eq('R2 string coercion', store2.getItem('surfaceReturn'), '42');

// R3. Повторный уход (двойной клик/гонка) — маркер тот же, редирект один.
const log3 = [];
installEnv(log3, okStore(log3));
r.returnToMap('p1');
r.returnToMap('p1');
eq('R3 idempotent', log3, [
    'setItem:surfaceReturn=p1', 'redirect:/map',
    'setItem:surfaceReturn=p1', 'redirect:/map',
]);

// R4. Хранилище недоступно (приватный режим/переполнение) — уход состоялся:
// маркер пропущен, исключение не выходит наружу, редирект на месте.
const log4 = [];
installEnv(log4, {
    setItem() { throw new Error('QuotaExceededError'); },
    getItem() { return null; },
});
r.returnToMap('p1');
eq('R4 storage throws', log4, ['redirect:/map']);

// R5. Глобала sessionStorage нет вовсе (строгий режим/Node) — тот же исход.
const log5 = [];
installEnv(log5, okStore(log5));
delete globalThis.sessionStorage;
r.returnToMap('p1');
eq('R5 no sessionStorage', log5, ['redirect:/map']);

eq('export returnToMap', typeof r.returnToMap, 'function');

console.log('SURFACE_RETURN_OK');
`

// TestSurfaceExitsShareReturnToMap — оба выхода из прогулки (смерть и «вызвать
// корабль») идут через общий хелпер; в onDeath маркер ставится внутри колбэка
// экрана смерти (после showDeath), а не при его показе.
func TestSurfaceExitsShareReturnToMap(t *testing.T) {
	src := readSurfaceMain(t)

	death := surfaceFuncBody(t, src, "onDeath")
	if !strings.Contains(death, "returnToMap(") {
		t.Fatalf("onDeath уходит с прогулки мимо returnToMap — маркер surfaceReturn не встанет:\n%s", death)
	}
	showAt := strings.Index(death, "showDeath(")
	returnAt := strings.Index(death, "returnToMap(")
	if showAt < 0 {
		t.Fatalf("onDeath не показывает экран смерти:\n%s", death)
	}
	if returnAt < showAt {
		t.Fatalf("в onDeath returnToMap идёт до showDeath — маркер встанет при показе экрана и залипнет, если игрок не нажмёт «продолжить»:\n%s", death)
	}

	ship := surfaceFuncBody(t, src, "callShip")
	if !strings.Contains(ship, "returnToMap(") {
		t.Fatalf("callShip уходит с прогулки мимо returnToMap:\n%s", ship)
	}
	if strings.Contains(ship, "location.href") {
		t.Fatalf("callShip редиректит мимо общего хелпера — маркер и уход разъедутся:\n%s", ship)
	}

	if !strings.Contains(src, "import { returnToMap } from './surface_return.js';") {
		t.Fatalf("surface_main.js не импортирует returnToMap из surface_return.js")
	}
}

// readSurfaceMain — исходник страницы прогулки (CRLF приведён к LF, разбор тел
// функций идёт по строкам).
func readSurfaceMain(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "static", "js", "surface", "surface_main.js"))
	if err != nil {
		t.Fatalf("чтение surface_main.js: %v", err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// surfaceFuncBody — тело верхнеуровневой функции surface_main.js: от сигнатуры
// до первой строки, равной `}` (в этом файле верхние функции закрыты так).
func surfaceFuncBody(t *testing.T, src, name string) string {
	t.Helper()
	sig := "function " + name + "("
	i := strings.Index(src, sig)
	if i < 0 {
		t.Fatalf("в surface_main.js нет функции %s", name)
	}
	rest := src[i:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("не нашёл конец тела %s", name)
	}
	return rest[:end]
}