// web/frontend_branches_test.go
// Node-тест клиентского блока ветки/эффектов поселения (спека 2026-09-22-
// поселение-ветка-буферы-переработка §6/T13 + эффекты-снабжения §6/T14):
// модуль modal/branches.js исполняется в Node без DOM/сети на верхнем уровне;
// вход виден только админу, выход — всем; эффекты — только админу; админ-формы
// присутствуют. Паттерн — как в frontend_deposits_test.go.
// Плюс витрина И2.3 (спека 2026-09-23-стадии-поселения §8.2/§11.3): строка
// ступени (stageRowHtml), блок арифметики по позициям (settlementArithmeticHtml)
// и новые поля ветки — число скорости/«забираем»/доля залежи/пометка «не в
// наборе стадии».
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

// Эффекты поселения (спека 2026-09-22-эффекты-снабжения §6/T14 + спека
// 2026-09-23-орбита-планеты-присутствие-и-снимок §5.4/§6.2 п.2): витрина
// админу — тип/источник (позиция), нагрузка, порог, состояние, сила/w
// + админ-форма ручной нагрузки; игроку — player-safe DTO {name, impact, state}
// без нагрузки/порога/силы/админ-форм.
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
assert(br.effectsBlockHtml([], true, 's1').includes('Эффектов нет'), 'эффекты: пусто у админа');

// Снятый эффект: состояние «снят».
const off = br.effectsBlockHtml([{ effect_type_id: 2, load: 1, threshold: 24, enabled: false }], true, 's1');
assert(off.includes('снят'), 'эффекты: состояние снят');

// Player-ветка (спека 2026-09-23 §5.4): только name/impact/state; силы,
// нагрузки, порога, источника и админ-форм нет; пусто — блока нет.
const playerEffects = [
    { name: 'Голод', impact: 'population_rate', state: 'active' }
];
const effPlayer = br.effectsBlockHtml(playerEffects, false, 's1');
assert(effPlayer.includes('Голод'), 'player: имя эффекта');
assert(effPlayer.includes('влияет на население'), 'player: подпись impact');
assert(effPlayer.includes('действует'), 'player: state=active → «действует»');
assert(!/нагрузк|порог|сила|силу/i.test(effPlayer), 'player: нет нагрузки/порога/силы');
assert(!effPlayer.includes('продовольствие'), 'player: нет источника');
assert(!effPlayer.includes('data-effect-load-form'), 'player: нет админ-формы');
assert(br.effectsBlockHtml([{ name: 'Голод', impact: 'population_rate', state: 'inactive' }], false, 's1').includes('не действует'), 'player: state=inactive → «не действует»');
assert(br.effectsBlockHtml([], false, 's1') === '', 'player: пустой список — блока нет');
assert(br.effectsBlockHtml([{ name: 'Прочее', impact: 'unknown_key', state: 'active' }], false, 's1').includes('unknown_key'), 'player: неизвестный impact показан как есть');

// Витрина арифметики ветки (спека 2026-09-23-стадии-поселения §11.3/§2.1):
// число скорости пары в масштабе единицы (rate хранится «ед/сутки/млрд»),
// «забираем» по компонентам (абсолютные ед/сутки с разрядами), доля залежи в
// процентах и пометка «не в наборе стадии» — форматирование/масштаб (подэтап B).
const b2 = {
    id: 'b2', recipe_id: 70, recipe_name: 'Сплав', complexity: 2,
    rate: 650, not_in_stage_set: true, deposit_share: 0.25,
    take: [{ good_id: 359, good_name: 'Мясо', per_day: 650 }, { good_id: 12, good_name: 'Вода', per_day: 1300 }],
    output: [], input: []
};
const admin2 = br.branchBlockHtml(b2, true);
assert(admin2.includes('скорость (число пары)') && admin2.includes('650'), 'ветка: число скорости пары');
assert(admin2.includes('ед/сутки / 10⁹'), 'ветка: единица скорости — дефолтный масштаб «на млрд»');
assert(admin2.includes('забираем') && admin2.includes('Мясо ×650') && admin2.includes('Вода ×1 300'), 'ветка: забираем по компонентам (разряды)');
assert(admin2.includes('из залежи') && admin2.includes('25%'), 'ветка: доля залежи в процентах');
assert(admin2.includes('рецепт не в наборе стадии (не производит)'), 'ветка: пометка не в наборе стадии');
// Объявленный ноль скорости — строка есть; отсутствие поля — строки нет;
// признак «не в наборе» не появляется сам по себе.
assert(br.branchBlockHtml({ id: 'b3', recipe_id: 71, rate: 0 }, true).includes('скорость (число пары)'), 'ветка: объявленный ноль скорости показан');
assert(!br.branchBlockHtml({ id: 'b4', recipe_id: 72 }, true).includes('скорость (число пары)'), 'ветка: нет числа — нет строки');
assert(!br.branchBlockHtml({ id: 'b5', recipe_id: 73 }, true).includes('не в наборе'), 'ветка: нет пометки без признака');
// Существующие строки не сломаны новой витриной.
assert(admin2.includes('за проход') && admin2.includes('скорость поедания'), 'ветка: старая петля на месте');

// Строка ступени (§11.3): имя + пороги (люди, абсолютные, с разрядами) +
// близость следующей. В снимке тип не замораживается, без имени типа — пусто.
const stageHtml = br.stageRowHtml({ id: 's1', type_name: 'Городок', population: 4000, stage: { enter: 1000, exit: 750, next_enter: 10000 } });
assert(stageHtml.includes('Стадия') && stageHtml.includes('Городок'), 'стадия: имя');
assert(stageHtml.includes('порог входа') && stageHtml.includes('1 000'), 'стадия: порог входа (разряды)');
assert(stageHtml.includes('порог выхода') && stageHtml.includes('750'), 'стадия: порог выхода');
assert(stageHtml.includes('до следующей ступени') && stageHtml.includes('6 000'), 'стадия: next_enter − population (разряды)');
// Пол: вход 0 не показываем, выход «не читается (пол)», следующей нет.
const floor = br.stageRowHtml({ id: 's2', type_name: 'Аутпост', population: 10, stage: { enter: 0, next_enter: 0 } });
assert(floor.includes('Аутпост'), 'стадия-пол: имя');
assert(!floor.includes('порог входа'), 'стадия-пол: вход 0 не показываем');
assert(floor.includes('порог выхода не читается (пол)'), 'стадия-пол: выход не читается');
assert(!floor.includes('до следующей'), 'стадия-пол: следующей нет');
// Только имя — без блока stage (тип вне ладдеры).
const named = br.stageRowHtml({ id: 's3', type_name: 'Городок' });
assert(named.includes('Городок') && !named.includes('порог'), 'стадия без порогов: только имя');
assert(br.stageRowHtml({ id: 's4' }) === '', 'нет имени типа — пусто');
assert(br.stageRowHtml(null) === '', 'нет поселения — пусто');

// Блок арифметики (§8.2): по позициям производим / потребляем / сверх
// (отрицательный net → «дефицит»), абсолютные ед/сутки с разрядами, масштаб не
// применяется; пусто / отсутствие / у player без arithmetic — блока нет.
const ar = br.settlementArithmeticHtml({
    id: 's1',
    arithmetic: [
        { position: 'продовольствие', produced_per_day: 12345, consumed_per_day: 600, net_per_day: 11745 },
        { position: 'вода', produced_per_day: 100, consumed_per_day: 400, net_per_day: -300 }
    ]
});
assert(ar.includes('Арифметика (на текущем населении, ед/сутки)'), 'арифметика: заголовок с подписью и единицей');
assert(ar.includes('продовольствие'), 'арифметика: позиция');
assert(ar.includes('производим') && ar.includes('12 345'), 'арифметика: производим (разряды)');
assert(ar.includes('потребляем') && ar.includes('600'), 'арифметика: потребляем');
assert(ar.includes('сверх') && ar.includes('+11 745'), 'арифметика: положительный net → «сверх +»');
assert(ar.includes('дефицит') && ar.includes('300'), 'арифметика: отрицательный net → «дефицит»');
assert(!ar.includes('-300'), 'арифметика: дефицит без знака минус');
assert(br.settlementArithmeticHtml({ id: 's1' }) === '', 'нет arithmetic — блока нет');
assert(br.settlementArithmeticHtml({ id: 's1', arithmetic: [] }) === '', 'пустой arithmetic — блока нет');
assert(br.settlementArithmeticHtml(null) === '', 'нет поселения — блока нет');

// Масштаб единицы (§2.1): применяется ТОЛЬКО к rate (хранимое «ед/сутки/млрд»).
// Нет localStorage (Node) → дефолт «на млрд»; globalThis.localStorage = 'person'
// → rate 650 превращается в 0.00000065. Забираем/пороги — абсолютные, без масштаба.
const scaleBranch = {
    id: 'bs', recipe_id: 80, rate: 650, deposit_share: 0.05,
    take: [{ good_id: 9, good_name: 'Руда', per_day: 1000 }]
};
const scaleDefault = br.branchBlockHtml(scaleBranch, false);
assert(scaleDefault.includes('скорость (число пары): <strong>650</strong> ед/сутки / 10⁹'), 'масштаб «на млрд» (дефолт): rate как есть');
assert(scaleDefault.includes('Руда ×1 000'), 'масштаб не применяется к забираем (абсолютные ед/сутки)');
assert(scaleDefault.includes('5%'), 'доля залежи 0.05 → 5%');

globalThis.localStorage = { getItem: () => 'person' };
const scalePerson = br.branchBlockHtml(scaleBranch, false);
assert(scalePerson.includes('скорость (число пары): <strong>0.00000065</strong> ед/сутки / 1 чел'), 'масштаб «на 1 чел»: rate 650 → 0.00000065');
assert(scalePerson.includes('Руда ×1 000'), 'масштаб «на 1 чел»: забираем не масштабируется');
delete globalThis.localStorage;

console.log('BRANCHES_OK');
`
