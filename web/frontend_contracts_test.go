// web/frontend_contracts_test.go
// Node-тест чистых функций доски контрактов карточки планеты (спека
// 2026-09-22-контракт-перелёт-и-доска §2.1/§2.2/§3): подписи типа/автора,
// читаемое требование, «осталось N», доступность публикации. Паттерн — как в
// TestDepositsGroupingInNode (frontend_deposits_test.go): модуль исполняется в
// Node (чистая функция, DOM не нужен).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestContractsBoardInNode — modal/contracts.js.
func TestContractsBoardInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(contractsBoardScript, "__CONTRACTS__", root+"/web/static/js/modal/contracts.js", 1)
	script = strings.Replace(script, "__WORKS__", root+"/web/static/js/modal/contracts_works.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка доски контрактов упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "CONTRACTS_BOARD_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const contractsBoardScript = `
const c = await import(new URL("__CONTRACTS__").href);
const w = await import(new URL("__WORKS__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}

// Подписи типа и автора (§2.1): известные ключи — человекочитаемо, неизвестные
// показываются как есть (выдуманных имён не вводим).
eq('type travel', c.contractTypeLabel('travel'), 'Перелёт');
eq('type unknown', c.contractTypeLabel('delivery'), 'delivery');
eq('type empty', c.contractTypeLabel(''), '—');
eq('author player', c.authorLabel('player'), 'игрок');
eq('author faction', c.authorLabel('faction'), 'фракция');
eq('author unknown', c.authorLabel('guild'), 'guild');
eq('icon player', c.authorIcon('player'), '👤');
eq('icon unknown', c.authorIcon('guild'), '•');

// Требование читаемо (§2.1): «двигатель не хуже 0.3» (op='le').
eq('req le', c.requirementText({ kind: 'gear', subject: 'speed_factor', op: 'le', threshold_num: 0.3 }),
    'двигатель не хуже 0.3');
// op='ge' в итерации 1 не встречается, но формулировка не должна врать
// («не хуже» для ge вводит в заблуждение — мелкое 7 ревью).
eq('req ge', c.requirementText({ kind: 'gear', subject: 'speed_factor', op: 'ge', threshold_num: 0.3 }),
    'двигатель не медленнее 0.3');
eq('req text fallback', c.requirementText({ kind: 'cargo', threshold_text: 'трюм 10 т' }), 'трюм 10 т');
eq('req null', c.requirementText(null), '');
// Требование-поставка (kind='goods', §4.2): объём доли (quantity) и название
// товара. good_name (идея 2026-10-01_сдача-груза-не-по-роли ЧК1) предпочитается
// внутреннему subject; без него — прежнее поведение (старый контракт без JOIN).
eq('req goods', c.requirementText({ kind: 'goods', subject: 'ore', op: 'in', quantity: 10 }), 'товар без названия: 10 ед.');
eq('req goods named', c.requirementText({ kind: 'goods', subject: '378', op: 'in', quantity: 10, good_name: 'Пища' }),
    'Пища: 10 ед.');
eq('req goods no name', c.requirementText({ kind: 'goods', subject: '378', op: 'in', quantity: 10 }), 'товар без названия: 10 ед.');
eq('req goods no qty', c.requirementText({ kind: 'goods', subject: 'ore', op: 'in' }), 'товар без названия');
eq('req goods no qty name', c.requirementText({ kind: 'goods', subject: 'ore', op: 'in', good_name: 'Руда' }), 'Руда');
// Позиция-доля экранируется и здесь (stored XSS через subject).
eq('req goods escaped', c.requirementText({ kind: 'goods', subject: '<img src=x>', op: 'in', quantity: 1 }),
    'товар без названия: 1 ед.');
// quantity тоже экранируется (дисциплина модуля: все строки с сервера).
eq('req goods qty escaped', c.requirementText({ kind: 'goods', subject: 'ore', op: 'in', quantity: '<b>9</b>' }),
    'товар без названия: &lt;b&gt;9&lt;/b&gt; ед.');

// «Осталось N» от expires_at (§2.1). now — миллисекунды.
const now = Date.parse('2026-09-22T12:00:00Z');
eq('left minutes', c.timeLeftText('2026-09-22T12:30:00Z', now), 'осталось 30 мин');
eq('left hours', c.timeLeftText('2026-09-22T15:00:00Z', now), 'осталось 3 ч');
eq('left days', c.timeLeftText('2026-09-24T12:00:00Z', now), 'осталось 2 дн');
eq('left expired', c.timeLeftText('2026-09-22T11:00:00Z', now), '');
eq('left empty', c.timeLeftText(null, now), '');

// Доска (§2.1): пусто — «Контрактов нет»; строка содержит заголовок, цену,
// требования и автора. Балансы/исполнитель в разметку не попадают (§2.2):
// проверяем по полям, которых в строке быть не должно.
eq('board empty', c.boardHtml([], now).includes('Контрактов нет'), true);
eq('board null', c.boardHtml(null, now).includes('Контрактов нет'), true);
const row = c.contractRowHtml({
    id: 'c1', type: 'travel', title: 'До Альфы', reward: 1650,
    author_type: 'player', expires_at: '2026-09-22T15:00:00Z',
    requirements: [{ kind: 'gear', subject: 'speed_factor', op: 'le', threshold_num: 0.3 }],
}, now);
eq('row title', row.includes('До Альфы'), true);
eq('row reward', row.includes('1650'), true);
eq('row left', row.includes('осталось 3 ч'), true);
eq('row req', row.includes('двигатель не хуже 0.3'), true);
eq('row author', row.includes('игрок'), true);
eq('row take btn', row.includes('data-contract-take="c1"'), true);
// §2.2: балансы и «кто взял» на доске не видны — ни escrow, ни executor.
eq('row no escrow', row.includes('escrow'), false);
eq('row no executor', row.includes('executor'), false);

// Группировка доски по пакету (§4.5): открытые доли одной нужды (общий
// package_key) — один блок «нужда»; доли несут размер (quantity из
// goods-требования), награду и срок. Контракт без package_key (перелёт) —
// отдельной плоской строкой, вне блока группы.
const grouped = c.boardHtml([
    { id: 's1', type: 'supply', title: 'Снабжение рудой', author_type: 'building',
      package_key: 'supply:p1:b1:ore', share_index: 1, reward: 100, expires_at: '2026-09-22T15:00:00Z',
      requirements: [{ kind: 'goods', subject: 'ore', op: 'in', quantity: 10 }] },
    { id: 's2', type: 'supply', title: 'Снабжение рудой', author_type: 'building',
      package_key: 'supply:p1:b1:ore', share_index: 2, reward: 50, expires_at: '2026-09-22T15:00:00Z',
      requirements: [{ kind: 'goods', subject: 'ore', op: 'in', quantity: 5 }] },
    { id: 't1', type: 'travel', title: 'До Альфы', reward: 1650, author_type: 'player' },
], now);
eq('group one block', (grouped.match(/data-contract-package=/g) || []).length, 1);
const gStart = grouped.indexOf('data-contract-package="supply:p1:b1:ore"');
const tStart = grouped.indexOf('data-contract-take="t1"');
eq('both shares in one block',
    gStart >= 0 && tStart > gStart &&
    grouped.slice(gStart, tStart).includes('data-contract-take="s1"') &&
    grouped.slice(gStart, tStart).includes('data-contract-take="s2"'), true);
eq('share sizes shown', grouped.includes('товар без названия: 10 ед.') && grouped.includes('товар без названия: 5 ед.'), true);
eq('group no internal good_id', grouped.includes('>ore'), false);
eq('share reward shown', grouped.includes('100') && grouped.includes('50'), true);
eq('share term shown', grouped.includes('осталось 3 ч'), true);
// Контракт без пакета — плоско: без обёртки группы.
const flatOnly = c.boardHtml([{ id: 't1', type: 'travel', title: 'T', reward: 1 }], now);
eq('flat no package block', flatOnly.includes('data-contract-package'), false);
eq('flat row present', flatOnly.includes('data-contract-take="t1"'), true);
// Разные пакеты — разные блоки.
const twoPkgs = c.boardHtml([
    { id: 'a1', type: 'supply', title: 'A', package_key: 'p1', reward: 1 },
    { id: 'b1', type: 'supply', title: 'B', package_key: 'p2', reward: 1 },
], now);
eq('two package blocks', (twoPkgs.match(/data-contract-package=/g) || []).length, 2);

// XSS (блокирующее 1): заголовок/описание задаёт ДРУГОЙ игрок, сервер их не
// чистит. Тег не должен попасть в разметку как тег — только как текст.
const xss = c.contractRowHtml({
    id: 'c2', type: 'travel', title: '<img src=x onerror=alert(1)>',
    description: '<script>alert(2)</script>', reward: 1, author_type: 'player',
    requirements: [{ kind: 'cargo', threshold_text: '<b>boom</b>' }],
}, now);
eq('xss title escaped', xss.includes('<img src=x onerror=alert(1)>'), false);
eq('xss title text', xss.includes('&lt;img src=x onerror=alert(1)&gt;'), true);
eq('xss desc escaped', xss.includes('<script>alert(2)</script>'), false);
eq('xss req escaped', xss.includes('<b>boom</b>'), false);
eq('xss req text', xss.includes('&lt;b&gt;boom&lt;/b&gt;'), true);
// id в атрибуте тоже экранируется (кавычка не вырвется из data-атрибута).
const xssId = c.contractRowHtml({ id: 'x" onmouseover="alert(1)', type: 'travel', title: 't', reward: 1 }, now);
eq('xss id escaped', xssId.includes('onmouseover="alert(1)"'), false);
eq('xss id text', xssId.includes('&quot;'), true);
// escapeHtml экспортируется — им экранирует имена систем tabs.js (loadDestWorlds).
eq('escapeHtml export', c.escapeHtml('<a href="x">&'), '&lt;a href=&quot;x&quot;&gt;&amp;');
eq('escapeHtml null', c.escapeHtml(null), '');

// Публикация — только с планеты, где стоит игрок (§3, О-п1).
eq('publish on planet orbit', c.canPublishHere({ status: 'orbit', object_type: 'planet', object_id: 'p1' }, 'p1'), true);
eq('publish on planet surface', c.canPublishHere({ status: 'surface', object_type: 'planet', object_id: 'p1' }, 'p1'), true);
eq('publish on star orbit', c.canPublishHere({ status: 'orbit', object_type: 'star', object_id: 'w1' }, 'p1'), false);
eq('publish in flight', c.canPublishHere({ status: 'in_flight', object_type: 'planet', object_id: 'p1' }, 'p1'), false);
eq('publish other planet', c.canPublishHere({ status: 'orbit', object_type: 'planet', object_id: 'p2' }, 'p1'), false);
eq('publish no position', c.canPublishHere(null, 'p1'), false);

// ЧК2б §6: словари типов/авторов знают supply/settlement (иначе сырой ключ).
eq('type supply', c.contractTypeLabel('supply'), 'Снабжение');
eq('author settlement', c.authorLabel('settlement'), 'поселение');
eq('icon settlement', c.authorIcon('settlement'), '🏘');

// «Мои заказы (в работе)» (ЧК2б §6): только мои взятые supply-заказы
// (executor_type='player'); открытые/перелёты не показываются; кнопка «Сдать»
// несёт data-contract-deliver с id. Строка товара — название (good_name),
// «осталось N» и «в трюме M» из cargo (идея 2026-10-01_сдача-груза-не-по-роли).
const cargo = w.cargoByGoodId([{ good_id: 378, quantity: 12 }]);
const mine = w.myWorksHtml([
    { id: 'w1', type: 'supply', status: 'taken', executor_type: 'player', title: 'Вода', author_type: 'settlement',
      expires_at: '2026-09-22T15:00:00Z',
      requirements: [{ kind: 'goods', subject: '378', op: 'in', quantity: 40, good_name: 'Пища' }] },
    { id: 'w2', type: 'supply', status: 'open', executor_type: null, title: 'Открытый', requirements: [] },
    { id: 'w3', type: 'travel', status: 'taken', executor_type: 'player', title: 'Перелёт', requirements: [] },
], now, cargo);
eq('works block header', mine.includes('Мои заказы (в работе)'), true);
eq('works deliver btn', mine.includes('data-contract-deliver="w1"'), true);
eq('works goods line', mine.includes('Пища · осталось 40 ед. · в трюме 12 ед.'), true);
eq('works no internal id', mine.includes('товар 378'), false);
eq('works author readable', mine.includes('поселение'), true);
eq('works excludes open', mine.includes('data-contract-deliver="w2"'), false);
eq('works excludes travel', mine.includes('data-contract-deliver="w3"'), false);
eq('works empty', w.myWorksHtml([], now, cargo), '');
eq('works null', w.myWorksHtml(null, now, cargo), '');
// Груз не читали (cargo === null) — «в трюме» не выводим: молчаливое «0» врало бы.
const mineNoCargo = w.myWorksHtml([
    { id: 'w1', type: 'supply', status: 'taken', executor_type: 'player', title: 'Вода', author_type: 'settlement',
      requirements: [{ kind: 'goods', subject: '378', op: 'in', quantity: 40, good_name: 'Пища' }] },
], now, null);
eq('works no cargo number', mineNoCargo.includes('в трюме'), false);
eq('works no cargo keeps rest', mineNoCargo.includes('Пища · осталось 40 ед.'), true);
// Пустая карта (груз прочитан, но товара в трюме нет — как на живом клиенте,
// где cargoByGoodId вернул пустое) — тоже молчит, а не «в трюме 0 ед.».
eq('works empty cargo map', w.myWorksHtml([
    { id: 'w1', type: 'supply', status: 'taken', executor_type: 'player', title: 'Вода', author_type: 'settlement',
      requirements: [{ kind: 'goods', subject: '378', op: 'in', quantity: 40, good_name: 'Пища' }] },
], now, w.cargoByGoodId([])).includes('в трюме'), false);
// Нет названия в каталоге — читаемая заглушка вместо внутреннего good_id.
const mineNoName = w.myWorksHtml([
    { id: 'w1', type: 'supply', status: 'taken', executor_type: 'player', title: 'Вода', author_type: 'settlement',
      requirements: [{ kind: 'goods', subject: '378', op: 'in', quantity: 40 }] },
], now, cargo);
eq('works unnamed good', mineNoName.includes('товар без названия'), true);
eq('works unnamed no id', mineNoName.includes('378'), false);
// Карта трюма: строки без числовых good_id/quantity пропускаются, дробные — как есть.
eq('cargo map get', cargo.get(378), 12);
eq('cargo map junk', w.cargoByGoodId([{ good_id: 'x' }, { good_id: 5 }, null, { good_id: 7, quantity: 1.25 }]).get(7), 1.25);
eq('cargo map null', w.cargoByGoodId(null).size, 0);
// XSS: заголовок/товар экранируются и в «моих заказах».
const mineXss = w.myWorksHtml([
    { id: 'x1', type: 'supply', status: 'taken', executor_type: 'player', title: '<img src=x>', author_type: 'settlement',
      requirements: [{ kind: 'goods', subject: '<b>w</b>', op: 'in', quantity: 1, good_name: '<b>w</b>' }] },
], now, null);
eq('works xss title', mineXss.includes('<img src=x>'), false);
eq('works xss goods', mineXss.includes('<b>w</b>'), false);

// Тексты сдачи (ЧК2б §6): человеческий результат и разбор {error}.
eq('deliver partial text', w.deliverResultText({ status: 'taken', delivered: 40, remaining: 60, paid: 400 }),
    'Сдано 40 ед., остаток требования 60 ед., выплачено 400 кр.');
eq('deliver full text', w.deliverResultText({ status: 'completed', delivered: 100, remaining: 0, paid: 0 }),
    'Заказ выполнен: сдано 100 ед.');
eq('deliver err from body', w.deliverErrorText({ error: 'Сдать можно только с орбиты планеты заказа' }, 422),
    'Сдать можно только с орбиты планеты заказа');
// Отказ сдачи с названием товара (ЧК3) доходит до игрока как есть.
eq('deliver err no cargo', w.deliverErrorText({ error: 'В трюме нет товара «Пища», а по заказу осталось сдать 40 ед.' }, 422),
    'В трюме нет товара «Пища», а по заказу осталось сдать 40 ед.');
eq('deliver err fallback', w.deliverErrorText(null, 409), 'Заказ уже не взят или хранилище недоступно');
eq('remaining units', w.remainingUnits({ requirements: [{ kind: 'goods', subject: 'w', op: 'in', quantity: 7 }] }), 7);
eq('remaining none', w.remainingUnits({ requirements: [] }), null);

console.log('CONTRACTS_BOARD_OK');
`
