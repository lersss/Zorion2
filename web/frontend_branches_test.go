// web/frontend_branches_test.go
// Node-тест клиентского блока ветки/эффектов поселения (спека 2026-09-22-
// поселение-ветка-буферы-переработка §6/T13 + эффекты-снабжения §6/T14):
// модуль modal/branches.js исполняется в Node без DOM/сети на верхнем уровне;
// вход виден только админу, выход — всем; эффекты — только админу; админ-формы
// присутствуют. Паттерн — как в frontend_deposits_test.go.
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBranchesRenderInNode — branchBlockHtml/branchesBlockHtml в modal/branches.js.
func TestBranchesRenderInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(branchesScript, "__BR__", root+"/web/static/js/modal/branches.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка блока ветки упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "BRANCHES_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const branchesScript = `
const br = await import(new URL("__BR__").href);

function assert(cond, msg) { if (!cond) throw new Error(msg); }

const b = {
    id: 'b1', recipe_id: 69, recipe_name: 'Пища', complexity: 1,
    output: [{ good_id: 378, good_name: 'Пища', amount: 30 }],
    input: [{ good_id: 359, good_name: 'Мясо', amount: 97 }],
    produced: 12, eaten: 5, eaten_rate: 0.25
};

// Выход виден всем; вход — только админу.
const admin = br.branchBlockHtml(b, true);
assert(admin.includes('Пища'), 'admin: имя рецепта');
assert(admin.includes('Мясо') && admin.includes('97'), 'admin: вход виден');
const player = br.branchBlockHtml(b, false);
assert(player.includes('Пища'), 'player: выход виден');
assert(!player.includes('Мясо'), 'player: вход скрыт');

// Петля потребления (итерация 4 §6/T13): «остаток (осадок)», «сделано» /
// «съедено» за последний проход и «скорость поедания» (ед/сек).
assert(admin.includes('остаток (осадок)'), 'admin: выход = остаток (осадок)');
assert(admin.includes('сделано') && admin.includes('12'), 'admin: сделано за проход');
assert(admin.includes('съедено') && admin.includes('5'), 'admin: съедено за проход');
assert(admin.includes('скорость поедания') && admin.includes('0.25') && admin.includes('ед/сек'), 'admin: скорость поедания');
assert(player.includes('сделано') && player.includes('съедено'), 'player: петля видна и игроку');
assert(!/всего/i.test(admin), 'нет слова «всего» — «съедено» за проход, не накопительно');

// Блок всех веток: заголовок + форма создания с id поселения; у player без
// веток — пусто.
const all = br.branchesBlockHtml([b], true, 's1');
assert(all.includes('Ветки (1)'), 'заголовок');
assert(all.includes('data-branch-create-form="s1"'), 'админ-форма создания');
assert(all.includes('data-branch-input-form="b1"'), 'админ-форма входа');
assert(br.branchesBlockHtml([], false, 's1') === '', 'player без веток — пусто');

// Эффекты поселения (спека 2026-09-22-эффекты-снабжения §6/T14): витрина
// только админу — тип/источник (позиция), нагрузка, порог, состояние, сила/w
// + админ-форма ручной нагрузки.
const effects = [
    { effect_type_id: 1, source_position: 'продовольствие', load: 30, threshold: 24, rate: 1e-7, w: 0.8, enabled: true, curve: 'hunger' }
];
const effAdmin = br.effectsBlockHtml(effects, true, 's1');
assert(effAdmin.includes('Эффекты (1)'), 'эффекты: заголовок с числом');
assert(effAdmin.includes('продовольствие'), 'эффекты: источник (позиция)');
assert(effAdmin.includes('нагрузка') && effAdmin.includes('30'), 'эффекты: нагрузка');
assert(effAdmin.includes('порог') && effAdmin.includes('24'), 'эффекты: порог');
assert(effAdmin.includes('включён'), 'эффекты: состояние включён');
assert(effAdmin.includes('сила') && effAdmin.includes('w'), 'эффекты: сила и w');
assert(effAdmin.includes('data-effect-load-form="s1"'), 'эффекты: админ-форма нагрузки');
assert(effAdmin.includes('data-effect-load-set'), 'эффекты: кнопка «задать нагрузку»');
assert(br.effectsBlockHtml(effects, false, 's1') === '', 'player не видит эффекты');
assert(br.effectsBlockHtml([], true, 's1').includes('Эффектов нет'), 'эффекты: пусто у админа');

// Снятый эффект: состояние «снят».
const off = br.effectsBlockHtml([{ effect_type_id: 2, load: 1, threshold: 24, enabled: false }], true, 's1');
assert(off.includes('снят'), 'эффекты: состояние снят');

console.log('BRANCHES_OK');
`
