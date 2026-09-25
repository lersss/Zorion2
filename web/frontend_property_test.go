// web/frontend_property_test.go
// Node-тест вкладки «Собственность» дашборда (спека
// 2026-09-26-собственность-игрока-в-дашборде §6.3/§6.4/§7.2/§8): сборка URL
// «Посмотреть» с экранированием (T11), URL «Перелететь» с &fly=1, гейт кнопки по
// двигателю (hasEngineFromMe, §8.4), пустое состояние (T12), плашка знания по
// режимам presence/snapshot/scan/none (T13), строка списка (иконка/адрес/
// экранирование) и Node-безопасность dashboard/property.js. Паттерн — как в
// TestMoneyBlockInNode (frontend_money_test.go).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPropertyTabInNode — dashboard/property.js.
func TestPropertyTabInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(propertyTabScript, "__PROPERTY__", root+"/web/static/js/dashboard/property.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка вкладки «Собственность» упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "PROPERTY_TAB_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const propertyTabScript = `
// Импорт без DOM — модуль обязан быть Node-безопасным (DOM/fetch только в initProperty).
const p = await import(new URL("__PROPERTY__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}
function ok(name, cond) {
    if (!cond) throw new Error(name + ': false');
}

// T11. URL «Посмотреть» с экранированием; форма &fly=1 — задел ЧК2 (§7.2/§8).
const item = { world_id: 'w1', planet_id: 'p1', world_name: 'Кеплер-3' };
eq('url plain', p.propertyMapUrl(item), '/map?system=w1&planet=p1&wname=' + encodeURIComponent('Кеплер-3'));
eq('url fly', p.propertyMapUrl(item, { fly: true }).endsWith('&fly=1'), true);
eq('url escaping', p.propertyMapUrl({ world_id: 'w/1', planet_id: 'p 2', world_name: 'A&B <b>' }),
    '/map?system=w%2F1&planet=p%202&wname=A%26B%20%3Cb%3E');
eq('url missing fields', p.propertyMapUrl({}), '/map?system=&planet=&wname=');

// T13. Плашка знания по режимам (§4.3): presence — без даты, snapshot — с датой
// (+«устарело» при fresh=false), scan — с датой, none — «Нет данных».
const date = '23.09.3026, 12:00';
eq('badge presence', p.knowledgeBadge({ mode: 'presence', at: null, fresh: false }).text, '● Свежие данные');
eq('badge snapshot fresh', p.knowledgeBadge({ mode: 'snapshot', at: '2026-09-23T12:00:00Z', fresh: true }).text,
    '📷 Данные на ' + date);
eq('badge snapshot stale', p.knowledgeBadge({ mode: 'snapshot', at: '2026-09-23T12:00:00Z', fresh: false }).text,
    '📷 Данные на ' + date + ' · устарело');
eq('badge scan', p.knowledgeBadge({ mode: 'scan', at: '2026-09-23T12:00:00Z', fresh: true }).text,
    '🔍 Данные сканера на ' + date);
eq('badge none', p.knowledgeBadge({ mode: 'none', at: null, fresh: false }).text, 'Нет данных');
eq('badge unknown', p.knowledgeBadge(undefined).text, 'Нет данных');
// snapshot без даты — «—» (битые данные не рисуем как NaN).
eq('badge broken date', p.knowledgeBadge({ mode: 'snapshot', at: 'nope', fresh: true }).text, '📷 Данные на —');

// T12. Пустое состояние (§6.3); не-массив не ломает UI.
ok('items empty', p.propertyItemsHtml([]).includes('У вас пока нет собственности'));
ok('items null', p.propertyItemsHtml(null).includes('У вас пока нет собственности'));
ok('items object', p.propertyItemsHtml({}).includes('У вас пока нет собственности'));

// T11/§6.4. Строка: иконка по kind, имя, подпись, адрес world › planet,
// плашка, ссылка «Посмотреть» с url-стыком.
const row = p.propertyRowHtml({
    kind: 'settlement', name: 'Люди', subtitle: 'Поселение',
    world_id: 'w1', planet_id: 'p1', world_name: 'Кеплер-3', planet_name: 'Кеплер-3 b',
    knowledge: { mode: 'snapshot', at: '2026-09-23T12:00:00Z', fresh: true },
});
ok('row icon settlement', row.includes('🏠'));
ok('row name', row.includes('Люди') && row.includes('Поселение'));
ok('row address', row.includes('Кеплер-3 › Кеплер-3 b'));
ok('row badge', row.includes('📷 Данные на ' + date));
ok('row view link', row.includes('href="/map?system=w1&amp;planet=p1&amp;wname=') && row.includes('Посмотреть'));
// T11/§8. Кнопка «Перелететь» (ЧК2): активная ссылка с &fly=1 по умолчанию.
ok('row fly btn', row.includes('Перелететь'));
ok('row fly href', row.includes('property-fly-btn') && row.includes('&amp;fly=1'));

// §8.4. Гейт по двигателю: hasEngine:false — неактивный span (is-disabled) с
// тултипом и без href, «Посмотреть» остаётся активной.
const rowNoEngine = p.propertyRowHtml({
    kind: 'settlement', name: 'Люди', subtitle: 'Поселение',
    world_id: 'w1', planet_id: 'p1', world_name: 'Кеплер-3', planet_name: 'Кеплер-3 b',
    knowledge: { mode: 'snapshot', at: '2026-09-23T12:00:00Z', fresh: true },
}, { hasEngine: false });
ok('row fly disabled', rowNoEngine.includes('property-fly-btn is-disabled') && rowNoEngine.includes('Перелететь'));
ok('row fly disabled tooltip', rowNoEngine.includes('title="Двигатель не установлен — полёт невозможен"'));
ok('row fly disabled no href', !rowNoEngine.includes('fly=1'));
ok('row view stays', rowNoEngine.includes('property-view-btn') && rowNoEngine.includes('Посмотреть'));

// §8.4. hasEngineFromMe — то же правило, что state.hasEngine карты (тип 'engine'
// из каталога); /me не загрузился (null) → true (сервер валидирует).
eq('engine yes', p.hasEngineFromMe({ equipment: { engine: 'e1' }, ship_catalog: [{ id: 'e1', type: 'engine' }] }), true);
eq('engine wrong type', p.hasEngineFromMe({ equipment: { engine: 'e1' }, ship_catalog: [{ id: 'e1', type: 'weapon' }] }), false);
eq('engine empty catalog', p.hasEngineFromMe({ equipment: { engine: 'e1' }, ship_catalog: [] }), false);
eq('engine no equipment', p.hasEngineFromMe({ ship_catalog: [{ id: 'e1', type: 'engine' }] }), false);
eq('engine me null', p.hasEngineFromMe(null), true);

const brow = p.propertyRowHtml({ kind: 'building', name: 'Аутпост', knowledge: { mode: 'none' } });
ok('row icon building', brow.includes('🏗'));
ok('row none badge', brow.includes('Нет данных'));

// XSS: серверные name/subtitle/world_name экранируются.
const xss = p.propertyRowHtml({
    kind: 'building', name: '<img src=x onerror=alert(1)>', subtitle: '<b>boom</b>',
    world_id: 'w', planet_id: 'p', world_name: '<i>x</i>', planet_name: 'P',
    knowledge: { mode: 'none' },
});
ok('xss name', !xss.includes('<img src=x onerror=alert(1)>') && xss.includes('&lt;img src=x onerror=alert(1)&gt;'));
ok('xss subtitle', !xss.includes('<b>boom</b>') && xss.includes('&lt;b&gt;boom&lt;/b&gt;'));
ok('xss address', !xss.includes('<i>x</i>') && xss.includes('&lt;i&gt;x&lt;/i&gt;'));

eq('initProperty export', typeof p.initProperty, 'function');
eq('propertyMapUrl export', typeof p.propertyMapUrl, 'function');
eq('knowledgeBadge export', typeof p.knowledgeBadge, 'function');
eq('hasEngineFromMe export', typeof p.hasEngineFromMe, 'function');

console.log('PROPERTY_TAB_OK');
`
