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

console.log('CONTRACTS_BOARD_OK');
`
