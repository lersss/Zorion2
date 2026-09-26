// web/frontend_auth_logout_test.go
// Разлогин на проде (идея 2026-09-27 «разлогин-на-проде», дефекты Д-1/Д-2/Д-3):
// клиент стирал вход по ЛЮБОМУ отказу сервера, а не только по «вход недействителен».
//
// П-1: 403 от обычной игровой ручки — вход остаётся, причина показана, редиректа
//      на /login-page нет; 401 — по-прежнему стирает (регресс).
// П-2: validateToken при 5xx/504 (сервер недоступен) — токен НЕ стирается и экран
//      входа НЕ показывается; при 401 — стирает и показывает (регресс).
// П-3: кнопка «в админка» не затирает непустой вход студии ('adminToken').
// Плюс контракт-скан: в игровых файлах web/ не осталось условия
// «401 || 403 → разлогин» (Д-1 целиком, а не только проверенные места).
//
// Паттерн — как в frontend_cargo_test.go / frontend_money_test.go (Node-репрест
// фронта): модуль исполняется в Node под стабом localStorage/fetch/document.
package web

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// runNode — исполняет скрипт в Node и требует маркер OK в выводе.
func runNode(t *testing.T, script, marker string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node упал:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), marker) {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

// jsURL — file://-URL модуля web/static/js из корня репозитория.
func jsURL(t *testing.T, rel string) string {
	t.Helper()
	return "file:///" + filepath.ToSlash(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
}

// ==================== П-1: 403 ≠ «вход мёртв» ====================

// TestDashboardForbiddenKeepsLogin — 403 от обычной ручки (груз «здесь нельзя
// торговать») НЕ стирает вход и НЕ уводит на страницу входа: в трюме и счёте
// показывается причина из тела ответа.
func TestDashboardForbiddenKeepsLogin(t *testing.T) {
	for _, tc := range []struct {
		name     string
		module   string
		init     string
		statusID string
	}{
		{"cargo", "web/static/js/dashboard/cargo.js", "initCargo", "cargo-status"},
		{"money", "web/static/js/dashboard/money.js", "initMoney", "money-status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := forbiddenBlockScript
			script = strings.Replace(script, "__MODULE__", jsURL(t, tc.module), 1)
			script = strings.Replace(script, "__INIT__", tc.init, 1)
			script = strings.Replace(script, "__STATUSID__", tc.statusID, 1)
			runNode(t, script, "FORBIDDEN_KEEPS_LOGIN_OK")
		})
	}
}

// TestDashboardUnauthorizedStillLogsOut — регресс П-1: 401 («вход недействителен»)
// по-прежнему уводит на /login-page, а счёт дополнительно чистит токен.
func TestDashboardUnauthorizedStillLogsOut(t *testing.T) {
	for _, tc := range []struct {
		name       string
		module     string
		init       string
		clearCheck string
	}{
		// Трюм и счёт чистят токен сами (Н-1 QA 2026-09-27: раньше чистил только
		// счёт, мёртвый токен оставался в браузере) — проверяем и редирект, и стирание.
		{"cargo", "web/static/js/dashboard/cargo.js", "initCargo", "eq('токен стёрт', store.token, undefined);"},
		{"money", "web/static/js/dashboard/money.js", "initMoney", "eq('токен стёрт', store.token, undefined);"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := unauthorizedBlockScript
			script = strings.Replace(script, "__MODULE__", jsURL(t, tc.module), 1)
			script = strings.Replace(script, "__INIT__", tc.init, 1)
			script = strings.Replace(script, "__CLEAR_TOKEN__", tc.clearCheck, 1)
			runNode(t, script, "UNAUTHORIZED_LOGS_OUT_OK")
		})
	}
}

// Сторож Д-1: «403 приравнен к 401» в ОДНОМ условии. Комментарии вырезаются
// (иначе сработало бы на записи вида «401/403 не ведёт»), окно условия — без
// `;` и без перевода строки, поэтому ДВЕ отдельные ветки на соседних строках
// (штатная проверка 401, затем отдельная ветка 403) дефектом не считаются.
var (
	// status 401/403 в одном условии, в любом порядке (=== и ==, нашлось по факту).
	forbiddenLogoutRe      = regexp.MustCompile(`status\s*(?:===|==)\s*401[^;\n]{0,80}(?:status\s*(?:===|==)\s*)?403`)
	forbiddenLogoutFirstRe = regexp.MustCompile(`403[^;\n]{0,80}status\s*(?:===|==)\s*401`)
	// Формы «список кодов» (возврат дефекта в них не выявлен — готовим сторож):
	// [401,403].includes(res.status) / [401, 403] в массиве.
	forbiddenLogoutListRe = regexp.MustCompile(`\[[^\]\n]{0,20}\b401\b[^\]\n]{0,20}\b403\b[^\]\n]{0,20}\]`)
	forbiddenLogoutInclRe = regexp.MustCompile(`includes\s*\(\s*[^\n;)]{0,40}\b401\b[^\n;)]{0,40}\b403\b[^\n;)]{0,20}\)`)
	forbiddenLogoutInclR2 = regexp.MustCompile(`includes\s*\(\s*[^\n;)]{0,40}\b403\b[^\n;)]{0,40}\b401\b[^\n;)]{0,20}\)`)
	// Массовая проверка «4xx и выше» рядом со сбросом входа — по факту в web/
	// таких мест нет, сторож на будущее (сам дефект = уход в разлогин).
	forbiddenLogoutRangeRe = regexp.MustCompile(`status\s*(?:>=|>)\s*4\d\d[^;\n]{0,120}(?:redirectToLogin|handleUnauthorized|clearToken|removeItem\(\s*'token')`)
)

// stripJSComments — вырезает `//`-комментарии до конца строки и `/* … */`
// (в том числе многострочные: заменяем переводом строки, чтобы не склеивать
// строки и не создавать ложных соседств). `://` маскируется, иначе протокол в
// строке ('wss://') съел бы остаток строки.
var (
	lineCommentRe  = regexp.MustCompile(`//[^\n]*`)
	blockCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	protoRe        = regexp.MustCompile(`://`)
)

func stripJSComments(src string) string {
	src = protoRe.ReplaceAllString(src, ":%PROTO%")
	src = blockCommentRe.ReplaceAllString(src, "\n")
	src = lineCommentRe.ReplaceAllString(src, "")
	return strings.ReplaceAll(src, ":%PROTO%", "://")
}

// hasForbiddenLogout — ловит возврат Д-1 в любой из форм записи.
func hasForbiddenLogout(src string) bool {
	clean := stripJSComments(src)
	for _, re := range []*regexp.Regexp{
		forbiddenLogoutRe, forbiddenLogoutFirstRe,
		forbiddenLogoutListRe, forbiddenLogoutInclRe, forbiddenLogoutInclR2,
		forbiddenLogoutRangeRe,
	} {
		if re.MatchString(clean) {
			return true
		}
	}
	return false
}

// TestForbiddenLogoutGuardMatchesForms — сам сторож (ревьюер, п. 3): на
// синтетике доказываем, что он ловит ВСЕ формы записи дефекта и не ловит
// ни комментарии, ни две отдельные ветки.
func TestForbiddenLogoutGuardMatchesForms(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		offense bool
	}{
		{"or-401-first", "if (res.status === 401 || res.status === 403) { redirectToLogin(); return; }", true},
		{"or-403-first", "if (res.status === 403 || res.status === 401) { handleUnauthorized(); return; }", true},
		{"loose-eq", "if (res.status == 403 || res.status == 401) { drop(); }", true},
		{"no-spaces", "if(res.status===401||res.status===403){drop();}", true},
		{"array-includes", "if ([401, 403].includes(res.status)) { redirectToLogin(); return; }", true},
		{"includes-403", "if (AUTH_CODES.includes(res.status)) { clearToken(); } // 401 и 403\nconst AUTH_CODES = [401, 403];", true},
		{"range-4xx-to-logout", "if (res.status >= 400) { handleUnauthorized(); return; }", true},
		{"comment-mentions-both", "// раньше было 401 || 403 — теперь только 401\nif (res.status === 401) { redirectToLogin(); return; }", false},
		{"block-comment-mentions-both", "/* 401 || 403 — нельзя */\nif (res.status === 401) { redirectToLogin(); return; }", false},
		{"separate-branches", "if (res.status === 401) { redirectToLogin(); return; }\nif (res.status === 403) { setStatus('нельзя'); return; }", false},
		{"only-401", "if (res.status === 401) { redirectToLogin(); return; }", false},
		{"not-ok", "if (!res.ok) { throw new Error('HTTP ' + res.status); }", false},
		{"separate-lines-401-then-403-far", "if (res.status === 401) { a(); }\nsomeOther();\nif (res.status === 403) { b(); }", false},
		{"url-protocol-kept", "const ws = location.protocol === 'https:' ? 'wss://' : 'ws://'; // 401 и 403\nif (res.status === 401) { a(); }", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasForbiddenLogout(tc.src); got != tc.offense {
				t.Fatalf("hasForbiddenLogout = %v, ждали %v\nисходник:\n%s", got, tc.offense, tc.src)
			}
		})
	}
}

// TestGameClientNeverLogsOutOnForbidden — контракт Д-1 целиком: в игровых файлах
// web/ (js + html) нет условия «401 || 403 → стирание входа». Ловит возврат
// дефекта в любом месте, включая те, что не покрыты поведенческим тестом.
func TestGameClientNeverLogsOutOnForbidden(t *testing.T) {
	webDir := filepath.Join(repoRoot(t), "web")
	var offenders []string
	err := filepath.WalkDir(webDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".js", ".html":
		default:
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if hasForbiddenLogout(string(b)) {
			rel, _ := filepath.Rel(repoRoot(t), p)
			offenders = append(offenders, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход web/: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("403 снова приравнен к 401 (стирание входа): %s", strings.Join(offenders, ", "))
	}
}

// ==================== П-2: проверка входа не стирает вход сама ====================

// TestEnsureAdminAuthUnavailableKeepsSession — Д-2: 5xx/504 от прокси/сервера —
// это «не удалось проверить», а не «вход мёртв»: токен на месте, экран входа не
// показан, игрок уведомлён.
func TestEnsureAdminAuthUnavailableKeepsSession(t *testing.T) {
	for _, status := range []int{500, 504} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			script := authScript(t, strconv.Itoa(status),
				"'admin-token'", // токен админки сохранён
				"false",         // оверлей входа не показан
				"if (!hasToast()) throw new Error('нет уведомления о недоступности сервера');",
			)
			runNode(t, script, "AUTH_KEEPS_SESSION_OK")
		})
	}
}

// TestEnsureAdminAuthUnauthorizedClears — регресс П-2: 401 — вход недействителен,
// токен стирается, экран входа показывается.
func TestEnsureAdminAuthUnauthorizedClears(t *testing.T) {
	script := authScript(t, "401",
		"undefined", // токен админки стёрт
		"true",      // оверлей входа показан
		"if (hasToast()) throw new Error('тост показан при 401');",
	)
	runNode(t, script, "AUTH_KEEPS_SESSION_OK")
}

// authScript — подставляет в сценарий адаптера код ответа и ожидания.
func authScript(t *testing.T, status, wantToken, wantOverlay, wantToast string) string {
	t.Helper()
	script := unavailableAuthScript
	script = strings.Replace(script, "__AUTH__", jsURL(t, "web/static/js/admin/auth.js"), 1)
	script = strings.Replace(script, "__STATUS__", status, 1)
	script = strings.Replace(script, "__WANT_TOKEN__", wantToken, 1)
	script = strings.Replace(script, "__WANT_OVERLAY__", wantOverlay, 1)
	script = strings.Replace(script, "__WANT_TOAST__", wantToast, 1)
	return script
}

// TestEnsureAdminAuthPlayerRoleClears — регресс прежнего поведения (ревьюер):
// `/me` в норме, но роль player — для админки вход недействителен: токен
// стирается, оверлей логина показывается. Не путать с `unavailable`.
func TestEnsureAdminAuthPlayerRoleClears(t *testing.T) {
	script := playerRoleAuthScript
	script = strings.Replace(script, "__AUTH__", jsURL(t, "web/static/js/admin/auth.js"), 1)
	runNode(t, script, "PLAYER_ROLE_CLEARS_OK")
}

// TestStudioAuthUnavailableKeepsSession — Д-2 для студии: `/me` → 5xx — токен
// цел, оверлей входа НЕ показан, причина уходит в отчёт (`window.showReport`).
func TestStudioAuthUnavailableKeepsSession(t *testing.T) {
	script := studioUnavailableScript
	script = strings.Replace(script, "__AUTH__", jsURL(t, "web/static/js/studio/auth.js"), 1)
	runNode(t, script, "STUDIO_KEEPS_SESSION_OK")
}

// TestValidateTokenWithoutTokenIsInvalid — ядро проверки входа: токена нет —
// «недействителен» (`invalid`), и запроса не было (в студию/админку без входа
// ходить незачем).
func TestValidateTokenWithoutTokenIsInvalid(t *testing.T) {
	script := noTokenScript
	script = strings.Replace(script, "__AUTH__", jsURL(t, "web/static/js/auth.js"), 1)
	runNode(t, script, "NO_TOKEN_INVALID_OK")
}

// ==================== П-3: кнопка «в админка» ====================

// TestGoToAdminKeepsStudioToken — Д-3: непустой слот студии/админки не
// затирается игровым токеном; пустой — копируется (прежнее удобство).
func TestGoToAdminKeepsStudioToken(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "static", "js", "main.js"))
	if err != nil {
		t.Fatalf("чтение main.js: %v", err)
	}
	fn := jsFuncBody(t, string(src), "goToAdmin")
	runNode(t, "const goToAdmin = "+fn+";\n"+goToAdminScript, "GOTO_ADMIN_OK")
}

// TestGameLogoutClearsAdminToken — регресс Д-3 (ревьюер): «Выйти» в игре
// чистит ОБА слота, иначе вход студии/админки (до 24 ч) переживает выход и
// следующий аккаунт в этом браузере получает чужую админку. Проверяем те же
// строки обработчика, что в `web/index.html` (файл общий — поэтому берём их
// из исходника, а не дублируем здесь).
func TestGameLogoutClearsAdminToken(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "index.html"))
	if err != nil {
		t.Fatalf("чтение index.html: %v", err)
	}
	body := logoutHandlerBody(t, string(src))
	if !strings.Contains(body, "removeItem('adminToken')") {
		t.Fatalf("обработчик «Выйти» не чистит adminToken — после П-3 чужой вход студии переживёт выход.\n%s", body)
	}
}

// logoutHandlerBody — тело обработчика клика по #logoutBtn из web/index.html.
func logoutHandlerBody(t *testing.T, src string) string {
	t.Helper()
	const anchor = "document.getElementById('logoutBtn').addEventListener('click', () => {"
	i := strings.Index(src, anchor)
	if i < 0 {
		t.Fatalf("в web/index.html нет обработчика #logoutBtn")
	}
	rest := src[i+len(anchor):]
	end := strings.Index(rest, "});")
	if end < 0 {
		t.Fatalf("обработчик #logoutBtn не закрыт")
	}
	return rest[:end]
}

// ==================== СКРИПТЫ NODE ====================

// forbiddenBlockScript — 403 от /api/cargo или /me/money: вход остаётся,
// редиректа нет, причина из тела ответа — в элементе статуса блока.
const forbiddenBlockScript = `
globalThis.window = globalThis;
const store = { token: 'game-token' };
globalThis.localStorage = {
    getItem: k => (Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null),
    setItem: (k, v) => { store[k] = String(v); },
    removeItem: k => { delete store[k]; },
};
const loc = { href: '' };
globalThis.location = loc;
const nodes = new Map();
const elStub = () => ({
    textContent: '', innerHTML: '', value: '', hidden: false, disabled: false, id: '',
    style: {}, dataset: {},
    classList: { add(){}, remove(){}, toggle(){}, contains(){ return false; } },
    addEventListener(){}, removeEventListener(){}, setAttribute(){}, hasAttribute(){ return false; },
    appendChild(){}, removeChild(){}, remove(){}, focus(){}, contains(){ return false; },
    querySelector(){ return null; }, querySelectorAll(){ return []; },
    closest(){ return null; }, getContext(){ return null; },
});
globalThis.document = {
    getElementById(id) { if (!nodes.has(id)) nodes.set(id, elStub()); return nodes.get(id); },
    querySelector(){ return null; }, querySelectorAll(){ return []; },
    createElement(){ return elStub(); }, createTextNode(){ return {}; },
    addEventListener(){}, removeEventListener(){},
    head: elStub(), body: elStub(), documentElement: elStub(), activeElement: null,
};
globalThis.fetch = async () => ({
    ok: false, status: 403,
    json: async () => ({ error: 'здесь нельзя торговать' }),
});
const mod = await import(new URL("__MODULE__").href);
mod.__INIT__();
await new Promise(r => setTimeout(r, 20));

function eq(name, got, want) {
    if (String(got) !== String(want)) throw new Error(name + ': "' + got + '" != "' + want + '"');
}
eq('вход остался', store.token, 'game-token');
eq('редиректа на вход нет', loc.href, '');
const status = nodes.get('__STATUSID__');
const text = status ? String(status.textContent) : '';
if (text.indexOf('здесь нельзя торговать') < 0) throw new Error('причина 403 не показана в статусе: "' + text + '"');
console.log('FORBIDDEN_KEEPS_LOGIN_OK');
`

// unauthorizedBlockScript — регресс: 401 уводит на страницу входа.
const unauthorizedBlockScript = `
globalThis.window = globalThis;
const store = { token: 'game-token' };
globalThis.localStorage = {
    getItem: k => (Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null),
    setItem: (k, v) => { store[k] = String(v); },
    removeItem: k => { delete store[k]; },
};
const loc = { href: '' };
globalThis.location = loc;
const nodes = new Map();
const elStub = () => ({
    textContent: '', innerHTML: '', value: '', hidden: false, disabled: false, id: '',
    style: {}, dataset: {},
    classList: { add(){}, remove(){}, toggle(){}, contains(){ return false; } },
    addEventListener(){}, removeEventListener(){}, setAttribute(){}, hasAttribute(){ return false; },
    appendChild(){}, removeChild(){}, remove(){}, focus(){}, contains(){ return false; },
    querySelector(){ return null; }, querySelectorAll(){ return []; },
    closest(){ return null; }, getContext(){ return null; },
});
globalThis.document = {
    getElementById(id) { if (!nodes.has(id)) nodes.set(id, elStub()); return nodes.get(id); },
    querySelector(){ return null; }, querySelectorAll(){ return []; },
    createElement(){ return elStub(); }, createTextNode(){ return {}; },
    addEventListener(){}, removeEventListener(){},
    head: elStub(), body: elStub(), documentElement: elStub(), activeElement: null,
};
globalThis.fetch = async () => ({ ok: false, status: 401, json: async () => ({ error: 'токен истёк' }) });
const mod = await import(new URL("__MODULE__").href);
mod.__INIT__();
await new Promise(r => setTimeout(r, 20));

function eq(name, got, want) {
    if (String(got) !== String(want)) throw new Error(name + ': "' + got + '" != "' + want + '"');
}
eq('редирект на вход', loc.href, '/login-page');
__CLEAR_TOKEN__
console.log('UNAUTHORIZED_LOGS_OUT_OK');
`

// unavailableAuthScript — ensureAdminAuth при коде __STATUS__ (500/504 — сервер
// недоступен; 401 — вход недействителен). Токен админки не стирается, оверлей
// входа не показывается; при 401 — наоборот (регресс).
const unavailableAuthScript = `
globalThis.window = globalThis;
const store = { adminToken: 'admin-token' };
globalThis.localStorage = {
    getItem: k => (Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null),
    setItem: (k, v) => { store[k] = String(v); },
    removeItem: k => { delete store[k]; },
};
globalThis.location = { href: '' };
const appended = [];
const nodes = new Map();
const elStub = () => ({
    textContent: '', innerHTML: '', value: '', hidden: false, disabled: false, id: '',
    style: {}, dataset: {},
    classList: { add(){}, remove(){}, toggle(){}, contains(){ return false; } },
    addEventListener(){}, removeEventListener(){}, setAttribute(){}, hasAttribute(){ return false; },
    appendChild(c) { appended.push(c); return c; }, removeChild(){}, remove(){}, focus(){},
    contains(){ return false; }, querySelector(){ return elStub(); }, querySelectorAll(){ return []; },
    closest(){ return null; }, getContext(){ return null; }, isConnected: true,
});
globalThis.document = {
    getElementById(id) { if (!nodes.has(id)) nodes.set(id, elStub()); return nodes.get(id); },
    querySelector(){ return null; }, querySelectorAll(){ return []; },
    createElement(){ return elStub(); }, createTextNode(){ return {}; },
    addEventListener(){}, removeEventListener(){},
    head: elStub(), body: elStub(), documentElement: elStub(), activeElement: null,
};
function hasToast() {
    return appended.some(n => String(n.innerHTML).indexOf('toast-msg') >= 0);
}
// Оверлей логина: элемент создаётся лениво, поэтому «не показан» — это и
// отсутствие обращения, и display, отличный от 'flex'.
function overlayShown() {
    const n = nodes.get('adminLoginOverlay');
    return !!n && n.style.display === 'flex';
}
globalThis.fetch = async () => ({ ok: false, status: __STATUS__, json: async () => ({ error: 'сервер недоступен' }) });
const auth = await import(new URL("__AUTH__").href);
const ok = await auth.ensureAdminAuth();

function eq(name, got, want) {
    if (String(got) !== String(want)) throw new Error(name + ': "' + got + '" != "' + want + '"');
}
eq('вход не пущен', ok, false);
eq('токен админки', store.adminToken, __WANT_TOKEN__);
eq('оверлей входа', overlayShown(), __WANT_OVERLAY__);
__WANT_TOAST__
console.log('AUTH_KEEPS_SESSION_OK');
`

// goToAdminScript — правило «не перезаписывать непустой слот» (Д-3).
const goToAdminScript = `
globalThis.window = globalThis;
const store = {};
globalThis.localStorage = {
    getItem: k => (Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null),
    setItem: (k, v) => { store[k] = String(v); },
    removeItem: k => { delete store[k]; },
};
globalThis.location = { href: '' };

function eq(name, got, want) {
    if (String(got) !== String(want)) throw new Error(name + ': "' + got + '" != "' + want + '"');
}

// Слот студии занят — вход студии не затираем игровым.
store.token = 'game-token';
store.adminToken = 'studio-token';
goToAdmin();
eq('вход студии сохранён', store.adminToken, 'studio-token');
eq('переход в админку', globalThis.location.href, '/admin');

// Слот пуст — копируем игровой токен (прежнее удобство).
delete store.adminToken;
goToAdmin();
eq('токен скопирован', store.adminToken, 'game-token');

// Игрового токена нет — пустой слот не заводим.
delete store.token;
delete store.adminToken;
goToAdmin();
eq('слот не заведён', store.adminToken, undefined);

console.log('GOTO_ADMIN_OK');
`

// playerRoleAuthScript — `/me` в норме (200), но роль player: вход для админки
// недействителен → токен стёрт, оверлей показан (прежнее поведение).
const playerRoleAuthScript = `
globalThis.window = globalThis;
const store = { adminToken: 'admin-token' };
globalThis.localStorage = {
    getItem: k => (Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null),
    setItem: (k, v) => { store[k] = String(v); },
    removeItem: k => { delete store[k]; },
};
globalThis.location = { href: '' };
const nodes = new Map();
const elStub = () => ({
    textContent: '', innerHTML: '', value: '', hidden: false, disabled: false, id: '',
    style: {}, dataset: {},
    classList: { add(){}, remove(){}, toggle(){}, contains(){ return false; } },
    addEventListener(){}, removeEventListener(){}, setAttribute(){}, hasAttribute(){ return false; },
    appendChild(){}, removeChild(){}, remove(){}, focus(){},
    contains(){ return false; }, querySelector(){ return elStub(); }, querySelectorAll(){ return []; },
    closest(){ return null; }, getContext(){ return null; }, isConnected: true,
});
globalThis.document = {
    getElementById(id) { if (!nodes.has(id)) nodes.set(id, elStub()); return nodes.get(id); },
    querySelector(){ return null; }, querySelectorAll(){ return []; },
    createElement(){ return elStub(); }, createTextNode(){ return {}; },
    addEventListener(){}, removeEventListener(){},
    head: elStub(), body: elStub(), documentElement: elStub(), activeElement: null,
};
globalThis.fetch = async () => ({ ok: true, status: 200, json: async () => ({ role: 'player' }) });
const auth = await import(new URL("__AUTH__").href);
const ok = await auth.ensureAdminAuth();

function eq(name, got, want) {
    if (String(got) !== String(want)) throw new Error(name + ': "' + got + '" != "' + want + '"');
}
eq('вход не пущен', ok, false);
eq('токен админки стёрт', store.adminToken, undefined);
const overlay = nodes.get('adminLoginOverlay');
eq('оверлей входа показан', !!overlay && overlay.style.display, 'flex');
console.log('PLAYER_ROLE_CLEARS_OK');
`

// studioUnavailableScript — студия при `/me` → 5xx: вход сохранён, оверлей входа
// НЕ показан, причина в отчёте. Модуль сам зовёт ensureStudioAuth при загрузке
// (bootstrap), поэтому window.start/showReport ставим ДО импорта.
const studioUnavailableScript = `
globalThis.window = globalThis;
const store = { adminToken: 'studio-token' };
globalThis.localStorage = {
    getItem: k => (Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null),
    setItem: (k, v) => { store[k] = String(v); },
    removeItem: k => { delete store[k]; },
};
globalThis.location = { href: '' };
const reported = [];
let started = 0;
globalThis.showReport = (lines) => { reported.push(...lines); };
globalThis.start = () => { started++; };
const nodes = new Map();
const elStub = () => ({
    textContent: '', innerHTML: '', value: '', hidden: false, disabled: false, id: '',
    style: {}, dataset: {},
    classList: { add(){}, remove(){}, toggle(){}, contains(){ return false; } },
    addEventListener(){}, removeEventListener(){}, setAttribute(){}, hasAttribute(){ return false; },
    appendChild(){}, removeChild(){}, remove(){}, focus(){},
    contains(){ return false; }, querySelector(){ return elStub(); }, querySelectorAll(){ return []; },
    closest(){ return null; }, getContext(){ return null; }, isConnected: true,
});
globalThis.document = {
    getElementById(id) { if (!nodes.has(id)) nodes.set(id, elStub()); return nodes.get(id); },
    querySelector(){ return null; }, querySelectorAll(){ return []; },
    createElement(){ return elStub(); }, createTextNode(){ return {}; },
    addEventListener(){}, removeEventListener(){},
    head: elStub(), body: elStub(), documentElement: elStub(), activeElement: null,
};
globalThis.fetch = async () => ({ ok: false, status: 503, json: async () => ({ error: 'прокси недоступен' }) });
await import(new URL("__AUTH__").href);
await new Promise(r => setTimeout(r, 20));

function eq(name, got, want) {
    if (String(got) !== String(want)) throw new Error(name + ': "' + got + '" != "' + want + '"');
}
eq('токен студии сохранён', store.adminToken, 'studio-token');
const overlay = nodes.get('studioLoginOverlay');
eq('оверлей входа не показан', !!overlay && overlay.style.display === 'flex', false);
eq('старт студии не вызван', started, 0);
if (reported.join(' ').indexOf('недоступен') < 0) throw new Error('в отчёте нет сообщения о недоступности: ' + JSON.stringify(reported));
console.log('STUDIO_KEEPS_SESSION_OK');
`

// noTokenScript — токена нет вовсе: validateToken возвращает «недействителен»
// и НЕ ходит в /me (вход нечего проверять).
const noTokenScript = `
globalThis.window = globalThis;
const store = {};
globalThis.localStorage = {
    getItem: k => (Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null),
    setItem: (k, v) => { store[k] = String(v); },
    removeItem: k => { delete store[k]; },
};
globalThis.location = { href: '' };
let fetches = 0;
globalThis.fetch = async () => { fetches++; return { ok: true, status: 200, json: async () => ({ role: 'admin' }) }; };
const auth = await import(new URL("__AUTH__").href);
const v = await auth.validateToken();

function eq(name, got, want) {
    if (String(got) !== String(want)) throw new Error(name + ': "' + got + '" != "' + want + '"');
}
eq('ok', v.ok, false);
eq('reason', v.reason, 'invalid');
eq('role', v.role, null);
eq('запросов к /me не было', fetches, 0);
console.log('NO_TOKEN_INVALID_OK');
`
