// web/frontend_surface_net_test.go
// Node-тест сетевого слоя прогулки (идея 2026-10-02, МЧК1б): обрыв связи не
// должен ломать страницу. `post()` (web/static/js/surface/surface_net.js) —
// единственная точка сети для высадки и ухода; при исключении от `fetch`
// (offline/обрыв) она обязана РЕЗОЛВИТЬСЯ в {ok:false,status:0}, а не отклонять
// промис: иначе исключение уходит в `boot()`/`callShip()`/`onDeath()`
// (surface_main.js) мимо их написанных веток `!res.ok` — на смерти страница
// замирает без экрана смерти и без пути назад (находка @tester, прогон
// 2026-10-02, сценарий 5a). Ответы сервера 4xx/5xx не меняются.
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSurfaceNetInNode — поведение post()/land()/leave() при подменённых
// глобалах fetch/localStorage/sessionStorage.
func TestSurfaceNetInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(surfaceNetScript, "__NET_MODULE__", root+"/web/static/js/surface/surface_net.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка сетевого слоя прогулки упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "SURFACE_NET_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const surfaceNetScript = `
// Модуль читает глобалы в момент вызова — подменяем их заранее.
const store = (v) => ({ getItem: (k) => (k === 'token' ? v : null) });
globalThis.localStorage = store(null);
globalThis.sessionStorage = store(null);

const net = await import(new URL("__NET_MODULE__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}
function ok(name, cond) {
    if (!cond) throw new Error(name + ': false');
}

// fetch-заглушка: помнит последний вызов, отвечает заданным кодом/телом.
let last = null;
function stub(status, text) {
    globalThis.fetch = (path, opts) => {
        last = { path: path, opts: opts };
        return Promise.resolve({ ok: status >= 200 && status < 300, status: status, text: () => Promise.resolve(text) });
    };
}

// N1. Обрыв связи: fetch отклоняется — leave() резолвится {ok:false,status:0}
// и НЕ бросает (иначе смерть замораживает страницу — идея 2026-10-02, МЧК1б).
globalThis.fetch = () => Promise.reject(new TypeError('Failed to fetch'));
let r1;
try {
    r1 = await net.leave();
} catch (e) {
    throw new Error('N1 leave() отклонился на обрыве: ' + e.message);
}
eq('N1 ok', r1.ok, false);
eq('N1 status', r1.status, 0);
ok('N1 error непустой', typeof r1.error === 'string' && r1.error.length > 0);

// Тот же обрыв на высадке — тоже резолвится (иначе страница висит на прелоадере).
globalThis.fetch = () => Promise.reject(new TypeError('Failed to fetch'));
let r1b;
try {
    r1b = await net.land('p1');
} catch (e) {
    throw new Error('N1b land() отклонился на обрыве: ' + e.message);
}
eq('N1b ok', r1b.ok, false);
eq('N1b status', r1b.status, 0);

// N2. Ошибка сервера 500 — прежний контракт {ok:false,status,error}.
stub(500, JSON.stringify({ error: 'boom' }));
const r2 = await net.leave();
eq('N2 ok', r2.ok, false);
eq('N2 status', r2.status, 500);
eq('N2 error', r2.error, 'boom');

// N3. Успех 200 c JSON-телом — прежний контракт {ok:true,data}.
stub(200, JSON.stringify({ cause: 'токсичная атмосфера' }));
const r3 = await net.leave();
eq('N3 ok', r3.ok, true);
eq('N3 status', r3.status, 200);
eq('N3 data', r3.data, { cause: 'токсичная атмосфера' });

// N3b. Не-JSON тело на 200 не должно ронять разбор (прежнее поведение: {error: text}).
stub(200, 'не json');
const r3b = await net.leave();
eq('N3b ok', r3b.ok, true);
eq('N3b data.error', r3b.data, { error: 'не json' });

// N4. Токен: сначала localStorage, при пустом — sessionStorage.
stub(200, '{}');
globalThis.localStorage = store('t-local');
globalThis.sessionStorage = store('t-session');
await net.leave();
eq('N4 localStorage first', last.opts.headers.Authorization, 'Bearer t-local');

globalThis.localStorage = store(null);
globalThis.sessionStorage = store('t-session');
await net.leave();
eq('N4 sessionStorage fallback', last.opts.headers.Authorization, 'Bearer t-session');

globalThis.localStorage = store(null);
globalThis.sessionStorage = store(null);
await net.leave();
eq('N4 без токена', last.opts.headers.Authorization, 'Bearer null');

// N5. land(): planet_id всегда, biome — только когда задан.
stub(200, '{}');
await net.land('p1');
eq('N5 path', last.path, '/api/surface/land');
let body = JSON.parse(last.opts.body);
eq('N5 planet_id', body.planet_id, 'p1');
eq('N5 biome без аргумента', Object.prototype.hasOwnProperty.call(body, 'biome'), false);

await net.land('p2', 'тайга');
body = JSON.parse(last.opts.body);
eq('N5 planet_id c biome', body.planet_id, 'p2');
eq('N5 biome', body.biome, 'тайга');

eq('N5 leave path', await net.leave().then(() => last.path), '/api/surface/leave');

eq('exports land/leave', [typeof net.land, typeof net.leave], ['function', 'function']);

console.log('SURFACE_NET_OK');
`
