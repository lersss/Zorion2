// web/frontend_cargo_test.go
// Node-тест блока «Трюм» дашборда (спека
// 2026-09-22-трюм-грузоподъёмность-корабля §9.1/§9.3): форматирование
// масса/процент, форма строки, экранирование имён товаров (stored XSS),
// пустое состояние, Node-безопасность dashboard/cargo.js. Паттерн — как в
// TestContractsBoardInNode (frontend_contracts_test.go).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCargoBlockInNode — dashboard/cargo.js.
func TestCargoBlockInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(cargoBlockScript, "__CARGO__", root+"/web/static/js/dashboard/cargo.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка блока «Трюм» упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "CARGO_BLOCK_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const cargoBlockScript = `
// Импорт без DOM — модуль обязан быть Node-безопасным (DOM только в initCargo).
const c = await import(new URL("__CARGO__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}

// Форматирование чисел: целые как есть, дробные — до 2 знаков, битые — «—».
eq('num int', c.cargoNum(12), '12');
eq('num frac', c.cargoNum(12.5), '12.5');
eq('num round', c.cargoNum(12.345), '12.35');
eq('num string', c.cargoNum('x'), '—');
eq('num null', c.cargoNum(null), '—');

// Полоса: занято/всего, used > total не вылезает за 100.
eq('mass label', c.cargoMassLabel(12, 100), '12 / 100 т');
eq('mass label zero', c.cargoMassLabel(0, 100), '0 / 100 т');
eq('percent half', c.cargoPercent(50, 100), 50);
eq('percent zero total', c.cargoPercent(5, 0), 0);
eq('percent over', c.cargoPercent(200, 100), 100);

// Пустое состояние — человекочитаемо.
eq('empty html', c.cargoEmptyHtml().includes('Трюм пуст'), true);
eq('items empty', c.cargoItemsHtml([]).includes('Трюм пуст'), true);
eq('items null', c.cargoItemsHtml(null).includes('Трюм пуст'), true);

// Строка: товар — количество — масса + поле/кнопка сброса (§9.3).
const row = c.cargoRowHtml({ good_id: 21, name: 'Железо Fe', quantity: 12, weight: 1, mass: 12 });
eq('row name', row.includes('Железо Fe'), true);
eq('row qty', row.includes('12 ед.'), true);
eq('row mass', row.includes('12 т'), true);
eq('row input', row.includes('class="cargo-qty"'), true);
eq('row button', row.includes('data-good-id="21"'), true);
eq('items one', c.cargoItemsHtml([{ good_id: 1, name: 'N', quantity: 2, mass: 2 }]).includes('cargo-row'), true);

// XSS (блокирующее): имя товара и id приходят из каталога студии — в разметку
// попадают только как текст, кавычка не вырывается из data-атрибута.
const xss = c.cargoRowHtml({ good_id: 'x" onmouseover="alert(1)', name: '<img src=x onerror=alert(1)>', quantity: 1, mass: 1 });
eq('xss name raw', xss.includes('<img src=x onerror=alert(1)>'), false);
eq('xss name text', xss.includes('&lt;img src=x onerror=alert(1)&gt;'), true);
eq('xss id raw', xss.includes('onmouseover="alert(1)"'), false);
eq('escapeHtml export', c.escapeHtml('<a href="x">&'), '&lt;a href=&quot;x&quot;&gt;&amp;');
eq('escapeHtml null', c.escapeHtml(null), '');
eq('initCargo export', typeof c.initCargo, 'function');

console.log('CARGO_BLOCK_OK');
`
