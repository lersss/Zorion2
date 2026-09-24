// web/frontend_market_test.go
// Node-тест чистых функций витрины «Магазин» карточки планеты (спека
// 2026-09-24-магазин-модулей-локальный-рынок §10): подписи типа и параметры
// модуля, ключи универсальных слотов, трейд-ин и итоговая цена, разметка
// витрины (раздел «Модули», ячейки слотов, гейт can_trade, пустое состояние).
// Паттерн — как в TestContractsBoardInNode (frontend_contracts_test.go):
// модуль исполняется в Node (чистые функции, DOM не нужен).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMarketTabInNode — modal/market.js.
func TestMarketTabInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(marketTabScript, "__MARKET__", root+"/web/static/js/modal/market.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка вкладки «Магазин» упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "MARKET_TAB_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const marketTabScript = `
// market.js статически тянет ui/toast.js (window.notifyError на верхнем уровне,
// как в графе админки) — тот же window-стаб, что в TestAdminFrontendLoadsInNode.
globalThis.window = globalThis;
const m = await import(new URL("__MARKET__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}

// Подписи типа модуля (§10): известные — человекочитаемо, неизвестные — как есть.
eq('type cargo', m.moduleTypeLabel('cargo'), 'грузовой');
eq('type radar', m.moduleTypeLabel('radar'), 'радар');
eq('type unknown', m.moduleTypeLabel('warp'), 'warp');
eq('type empty', m.moduleTypeLabel(''), '—');

// Параметры модуля читаемо (§10): числа только с сервера (params).
eq('params cargo', m.moduleParamsText({ type: 'cargo', params: { capacity: 30 } }), 'Ёмкость: +30 т');
eq('params radar', m.moduleParamsText({ type: 'radar', params: { radius: 800 } }), 'Радиус: 800 px');
eq('params engine', m.moduleParamsText({ type: 'engine', params: { speed_factor: 0.3 } }), 'Скорость: 0.3 сек/px');
eq('params scanner', m.moduleParamsText({ type: 'scanner', params: { depth: 'surface', settlements: true } }),
    'Глубина: поверхность · Поселения: да');
eq('params missing', m.moduleParamsText({ type: 'cargo' }), 'Ёмкость: +— т');

// Ключи универсальных слотов (§20.2): universal / universal2 / universal3.
eq('keys 3', m.universalSlotKeys(3), ['universal', 'universal2', 'universal3']);
eq('keys 1', m.universalSlotKeys(1), ['universal']);
eq('keys 0', m.universalSlotKeys(0), []);
eq('keys null', m.universalSlotKeys(null), []);
eq('keys string', m.universalSlotKeys('2'), ['universal', 'universal2']);

// Трейд-ин: floor(цена_старого / 2) из витрины по item.id (§7.2 п.5);
// модуль вне витрины → 0 (§18 п.4); пустой слот → 0.
const offers = [
    { id: 1, kind: 'module', item: { id: 'cargo_1', type: 'cargo', name: 'Грузовой модуль-1', params: { capacity: 30 } }, price: 3000 },
    { id: 3, kind: 'module', item: { id: 'radar_1', type: 'radar', name: 'Радар-1', params: { radius: 800 } }, price: 3000 },
];
eq('tradein cargo', m.offerTradein('cargo_1', offers), 1500);
eq('tradein radar', m.offerTradein('radar_1', offers), 1500);
eq('tradein not offered', m.offerTradein('engine_1', offers), 0);
eq('tradein empty slot', m.offerTradein('', offers), 0);
eq('tradein null offers', m.offerTradein('cargo_1', null), 0);

// Итоговая цена: max(0, price − tradein) — выкуп не даёт денег сверх цены (И-М3).
eq('total with tradein', m.finalPrice(3000, 1500), 1500);
eq('total no tradein', m.finalPrice(3000, 0), 3000);
eq('total floor zero', m.finalPrice(1000, 1500), 0);

// Разметка витрины (§10): 4 карточки, 3 ячейки слота, выбран первый слот
// (universal) — занятый грузовым модулем, выкуп 1500, итог 1500.
const me = {
    ship_model: { id: 'starter', name: 'Стартовый', slots: { radar: 1, scanner: 1, engine: 1, universal: 3 } },
    equipment: { radar: 'radar_1', universal: 'cargo_1' },
    ship_catalog: [
        { id: 'cargo_1', type: 'cargo', name: 'Грузовой модуль-1', params: { capacity: 30 } },
        { id: 'radar_1', type: 'radar', name: 'Радар-1', params: { radius: 800 } },
    ],
};
const market = {
    offers: [
        { id: 1, kind: 'module', item: { id: 'cargo_1', type: 'cargo', name: 'Грузовой модуль-1', params: { capacity: 30 } }, price: 3000 },
        { id: 2, kind: 'module', item: { id: 'engine_1', type: 'engine', name: 'Двигатель-1', params: { speed_factor: 0.3 } }, price: 3000 },
        { id: 3, kind: 'module', item: { id: 'radar_1', type: 'radar', name: 'Радар-1', params: { radius: 800 } }, price: 3000 },
        { id: 4, kind: 'module', item: { id: 'scanner_1', type: 'scanner', name: 'Сканер-1', params: { depth: 'surface', settlements: true } }, price: 3000 },
    ],
    can_trade: true,
};
const html = m.marketHtml(market, me, null);
eq('module section', html.includes('Модули'), true);
eq('four buy buttons', (html.match(/data-market-buy=/g) || []).length, 4);
eq('three slot cells', (html.match(/data-market-slot=/g) || []).length, 3);
eq('occupied tradein shown', html.includes('выкуп старого: −1500 Cr'), true);
eq('total recalculated', html.includes('Итого: <strong>1500 Cr</strong>'), true);
eq('price from server', html.includes('3000 Cr'), true);
eq('params from server', html.includes('Радиус: 800 px'), true);
eq('gate note hidden', html.includes('Магазин доступен только с орбиты планеты'), false);
eq('buy enabled', html.includes('data-market-buy="1" disabled'), false);

// Пустой слот: без выкупа, итог = полная цена (§7.2 п.5).
const meEmpty = { ship_model: { slots: { universal: 3 } }, equipment: {}, ship_catalog: [] };
const htmlEmptySlot = m.marketHtml(market, meEmpty, 'universal2');
eq('empty slot label', htmlEmptySlot.includes('без выкупа'), true);
eq('empty slot total full', htmlEmptySlot.includes('Итого: <strong>3000 Cr</strong>'), true);
eq('empty slot no tradein', htmlEmptySlot.includes('выкуп старого'), false);

// Выбор занятого слота: universal2 выбран явно — выкуп по его содержимому.
const meRadarSlot = { ship_model: { slots: { universal: 3 } }, equipment: { universal2: 'radar_1' },
    ship_catalog: [{ id: 'radar_1', type: 'radar', name: 'Радар-1', params: { radius: 800 } }] };
const htmlRadar = m.marketHtml(market, meRadarSlot, 'universal2');
eq('selected radar tradein', htmlRadar.includes('выкуп старого: −1500 Cr'), true);

// Гейт can_trade == false (§10): витрина видна, кнопка неактивна + подпись.
const htmlGated = m.marketHtml({ offers: market.offers, can_trade: false }, me, null);
eq('gate note shown', htmlGated.includes('Магазин доступен только с орбиты планеты'), true);
eq('buy disabled', htmlGated.includes('data-market-buy="1" disabled'), true);
eq('offers still visible', htmlGated.includes('Грузовой модуль-1'), true);

// Пустое состояние (§10): offers: [] — «На планете нет рынка».
eq('no market', m.marketHtml({ offers: [], can_trade: true }, me, null).includes('На планете нет рынка'), true);
eq('no market null', m.marketHtml(null, me, null).includes('На планете нет рынка'), true);

// Экранирование имён с сервера (дисциплина модуля): тег не попадает как тег.
const xss = m.marketHtml({
    offers: [{ id: 9, item: { id: 'x', type: 'cargo', name: '<img src=x onerror=alert(1)>', params: { capacity: 1 } }, price: 1 }],
    can_trade: true,
}, meEmpty, null);
eq('xss escaped', xss.includes('<img src=x onerror=alert(1)>'), false);
eq('xss text', xss.includes('&lt;img src=x onerror=alert(1)&gt;'), true);

console.log('MARKET_TAB_OK');
`
