// web/frontend_deeplink_test.go
// Node-тест стыка «дашборд → карта» (спека
// 2026-09-26-собственность-игрока-в-дашборде §7.2/§7.3/§8): чистый разбор
// intent'а из location.search (включая флаг ЧК2 fly=1), разрешение мира из кэша
// карты, сборка starInfo для попапа, предикат готовности попапа к запуску
// маршрута и флаг подавления авто-открывателей (С3). Модуль map/deeplink.js
// намеренно без статических импортов — тянет граф карты/модалки только
// динамически внутри handleDashboardDeepLink, поэтому здесь Node-безопасен без DOM.
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeeplinkHelpersInNode — map/deeplink.js.
func TestDeeplinkHelpersInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(deeplinkScript, "__DEEPLINK__", root+"/web/static/js/map/deeplink.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка разбора deeplink упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "DEEPLINK_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const deeplinkScript = `
// Модуль без статических импортов — грузится в Node без DOM.
const d = await import(new URL("__DEEPLINK__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}
function ok(name, cond) {
    if (!cond) throw new Error(name + ': false');
}

// T15. Разбор строки запроса: полный intent, нет system → null, пустая строка.
// fly — флаг ЧК2 «Перелететь» (§8.2): true только при fly=1.
eq('parse full', d.parseIntentSearch('?system=w1&planet=p1&wname=' + encodeURIComponent('Кеплер-3')),
    { worldId: 'w1', planetId: 'p1', worldName: 'Кеплер-3', fly: false });
eq('parse no planet', d.parseIntentSearch('?system=w1'), { worldId: 'w1', planetId: null, worldName: '', fly: false });
eq('parse fly', d.parseIntentSearch('?system=w1&planet=p1&fly=1'),
    { worldId: 'w1', planetId: 'p1', worldName: '', fly: true });
eq('parse fly not 1', d.parseIntentSearch('?system=w1&planet=p1&fly=0'),
    { worldId: 'w1', planetId: 'p1', worldName: '', fly: false });
eq('parse no system', d.parseIntentSearch('?planet=p1'), null);
eq('parse empty', d.parseIntentSearch(''), null);
eq('parse null', d.parseIntentSearch(undefined), null);

// Готовность попапа к запуску маршрута (§8.2, чистый предикат): мир совпал,
// планеты уже загружены и /me отработал (meLoaded); иначе — false.
eq('ready yes', d.isModalReadyForFly({ worldId: 'w1', planets: [], meLoaded: true }, 'w1'), true);
eq('ready no me', d.isModalReadyForFly({ worldId: 'w1', planets: [], meLoaded: false }, 'w1'), false);
eq('ready me undefined', d.isModalReadyForFly({ worldId: 'w1', planets: [] }, 'w1'), false);
eq('ready other world', d.isModalReadyForFly({ worldId: 'w2', planets: [], meLoaded: true }, 'w1'), false);
eq('ready no planets', d.isModalReadyForFly({ worldId: 'w1', planets: null, meLoaded: true }, 'w1'), false);
eq('ready null state', d.isModalReadyForFly(null, 'w1'), false);

// Целевая планета в списке попапа (§8.2): ограниченная/пустая система —
// маршрут не запускаем (иначе тост «планета не найдена»).
eq('planet present', d.planetInModalList([{ id: 'p1' }, { id: 'p2' }], 'p2'), true);
eq('planet absent', d.planetInModalList([{ id: 'p1' }], 'p9'), false);
eq('planet empty list', d.planetInModalList([], 'p1'), false);
eq('planet non-array', d.planetInModalList(null, 'p1'), false);

// Разрешение мира из кэша карты (С2): найден / нет / не-массив.
const worlds = [{ id: 'w1', name: 'A' }, { id: 'w2', name: 'B' }];
eq('cache found', (d.worldFromCache(worlds, 'w2') || {}).name, 'B');
eq('cache missing', d.worldFromCache(worlds, 'w9'), null);
eq('cache non-array', d.worldFromCache(null, 'w1'), null);

// starInfo для попапа (§7.2): поля мира + флаги карты (hasEngine/shipIcon/shipColor).
const si = d.starInfoFromWorld(
    { star_type: 'star', temperature: 5800, system_type: 'single', stellar_mods: { x: 1 }, coord_x: 10, coord_y: 20 },
    { hasEngine: true, userShipIcon: 'ship.png', userShipColor: '#fff' });
eq('starInfo stype', si.stype, 'star');
eq('starInfo stemp', si.stemp, 5800);
eq('starInfo systype', si.systype, 'single');
eq('starInfo smods', si.smods, { x: 1 });
eq('starInfo x', si.x, 10);
eq('starInfo y', si.y, 20);
eq('starInfo hasEngine', si.hasEngine, true);
eq('starInfo shipIcon', si.shipIcon, 'ship.png');
eq('starInfo shipColor', si.shipColor, '#fff');
eq('starInfo no world', d.starInfoFromWorld(null, {}), null);

// parseIntent + флаг подавления авто-открывателей (С3): нет system — no-op и
// флага нет; с system — intent есть и флаг поднят (data.js по нему пропустит
// compositeRoute/beltReturn и вычистит маркеры).
globalThis.location = { search: '' };
eq('intent none', d.parseIntent(), null);
eq('suppress none', d.shouldSuppressAutoOpen(), false);
globalThis.location = { search: '?system=w1&wname=X' };
eq('intent set', d.parseIntent(), { worldId: 'w1', planetId: null, worldName: 'X', fly: false });
eq('suppress set', d.shouldSuppressAutoOpen(), true);

eq('handleDashboardDeepLink export', typeof d.handleDashboardDeepLink, 'function');
eq('isModalReadyForFly export', typeof d.isModalReadyForFly, 'function');
eq('planetInModalList export', typeof d.planetInModalList, 'function');

console.log('DEEPLINK_OK');
`
