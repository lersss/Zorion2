// web/frontend_money_test.go
// Node-тест блока «Счёт» дашборда (идея 2026-09-23 «Деньги на счёте в шапке и
// история» §5.2–§5.4): форматирование подписи баланса, знака суммы, подписей
// типов операций (открытый список), даты общего игрового календаря, пустое
// состояние и Node-безопасность dashboard/money.js. Паттерн — как в
// TestCargoBlockInNode (frontend_cargo_test.go).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMoneyBlockInNode — dashboard/money.js.
func TestMoneyBlockInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(moneyBlockScript, "__MONEY__", root+"/web/static/js/dashboard/money.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка блока «Счёт» упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "MONEY_BLOCK_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const moneyBlockScript = `
// Импорт без DOM — модуль обязан быть Node-безопасным (DOM/fetch только в initMoney).
const m = await import(new URL("__MONEY__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}

// Подпись баланса: разделитель тысяч (ru-RU, неразрывный пробел) + « Cr».
eq('label 10000', m.moneyLabel(10000), '10\u00a0000 Cr');
eq('label 0', m.moneyLabel(0), '0 Cr');
eq('label millions', m.moneyLabel(1234567), '1\u00a0234\u00a0567 Cr');
eq('label string', m.moneyLabel('x'), '—');
eq('label null', m.moneyLabel(null), '—');
eq('label nan', m.moneyLabel(NaN), '—');

// Сумма со знаком: + / типографский минус U+2212, разделитель тысяч.
eq('sign plus', m.moneySign(10000), '+10\u00a0000 Cr');
eq('sign minus', m.moneySign(-300), '−300 Cr');
eq('sign minus big', m.moneySign(-1234567), '−1\u00a0234\u00a0567 Cr');
eq('sign null', m.moneySign(null), '—');

// Подписи типов: открытый список, неизвестное не ломает UI (§5.2).
eq('kind seed', m.moneyKindLabel('admin_seed'), 'Стартовый капитал');
eq('kind lock', m.moneyKindLabel('escrow_lock'), 'Залог по контракту');
eq('kind release', m.moneyKindLabel('escrow_release'), 'Выплата по контракту');
eq('kind return', m.moneyKindLabel('escrow_return'), 'Возврат залога');
eq('kind work', m.moneyKindLabel('contract_work_earn'), 'Заработок по подряду');
eq('kind unknown', m.moneyKindLabel('mint'), 'Операция по счёту');
eq('kind empty', m.moneyKindLabel(undefined), 'Операция по счёту');

// Дата — общий игровой календарь (UTC, год +1000).
eq('date', m.moneyDate('2026-09-23T14:05:00Z'), '23.09.3026, 14:05');
eq('date broken', m.moneyDate('nope'), '—');

// Пустой список — человекочитаемое состояние.
eq('items empty', m.moneyItemsHtml([]).includes('Операций пока нет'), true);
eq('items null', m.moneyItemsHtml(null).includes('Операций пока нет'), true);

// Строка истории: тип + дата слева, сумма со знаком справа.
const row = m.moneyRowHtml({ delta: 10000, kind: 'admin_seed', occurred_at: '2026-09-23T14:05:00Z' });
eq('row kind', row.includes('Стартовый капитал'), true);
eq('row amount', row.includes('+10\u00a0000 Cr'), true);
eq('row date', row.includes('23.09.3026, 14:05'), true);
eq('items one', m.moneyItemsHtml([{ delta: 1, kind: 'escrow_lock' }]).includes('money-row'), true);

// Битые данные не рисуются как NaN.
const bad = m.moneyRowHtml({});
eq('bad no NaN', bad.includes('NaN'), false);
eq('bad amount', bad.includes('—'), true);

// Цвета gain/loss (§5.2): плюс — зелёный, минус — красный.
eq('gain class', m.moneyRowHtml({ delta: 5 }).includes('money-gain'), true);
eq('loss class', m.moneyRowHtml({ delta: -5 }).includes('money-loss'), true);

eq('initMoney export', typeof m.initMoney, 'function');
eq('moneyLabel export', typeof m.moneyLabel, 'function');

console.log('MONEY_BLOCK_OK');
`
