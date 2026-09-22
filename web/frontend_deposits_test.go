// web/frontend_deposits_test.go
// Node-тест клиентской группировки залежей поверхности (спека 2026-09-22-
// поселение-добыча-сырья-биома-ленивый-буфер §5.2/T13 + спека итерации 3
// §6/T14): сервер отдаёт deposits[] построчно, карточка планеты сводит пятна по
// ресурсу — число пятен, суммарный запас, диапазон богатства, счётчик
// выработанных (depleted). Паттерн — как в TestGraphicsOptionsInNode
// (frontend_graphics_test.go): модуль исполняется в Node (чистая функция, DOM не
// нужен).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDepositsGroupingInNode — groupDeposits() в modal/deposits.js.
func TestDepositsGroupingInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(depositsGroupingScript, "__DEP__", root+"/web/static/js/modal/deposits.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка группировки залежей упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "DEPOSITS_GROUPING_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const depositsGroupingScript = `
const dep = await import(new URL("__DEP__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}

// Пустой вход — пустая группа (карточка покажет «залежей нет»).
eq('empty', dep.groupDeposits([]), []);
eq('null', dep.groupDeposits(null), []);

// Два пятна одного ресурса + одно другого: группировка по good_id,
// суммарный запас и диапазон богатства (T13). Порядок групп — по первому
// появлению ресурса в ответе.
const rows = [
    { good_id: 1, good_name: 'вода-ресурс', stratum: 'surface', wealth: 0.2, amount: 500 },
    { good_id: 1, good_name: 'вода-ресурс', stratum: 'surface', wealth: 0.8, amount: 1500 },
    { good_id: 359, good_name: 'Мясо', stratum: 'surface', wealth: 0.5, amount: 1000 },
];
const g = dep.groupDeposits(rows);
eq('groups count', g.length, 2);
eq('water count', g[0].count, 2);
eq('water amount', g[0].amount, 2000);
eq('water wealth min', g[0].wealthMin, 0.2);
eq('water wealth max', g[0].wealthMax, 0.8);
eq('water name', g[0].good_name, 'вода-ресурс');
eq('meat count', g[1].count, 1);
eq('meat amount', g[1].amount, 1000);
eq('meat wealth min', g[1].wealthMin, 0.5);
eq('meat wealth max', g[1].wealthMax, 0.5);

// Выработанные пятна (amount <= 0) — только у админа (спека итерации 3 §6/п.26):
// группа считает счётчик depleted, но остаётся группой того же ресурса.
const withDepleted = [
    { good_id: 1, good_name: 'вода-ресурс', stratum: 'surface', wealth: 0.2, amount: 500 },
    { good_id: 1, good_name: 'вода-ресурс', stratum: 'surface', wealth: 0.8, amount: 0 },
    { good_id: 359, good_name: 'Мясо', stratum: 'surface', wealth: 0.5, amount: 1000 },
];
const gd = dep.groupDeposits(withDepleted);
eq('depleted count', gd[0].depleted, 1);
eq('depleted water count', gd[0].count, 2);
eq('active group depleted', gd[1].depleted, 0);
eq('active group count', gd[1].count, 1);

console.log('DEPOSITS_GROUPING_OK');
`
