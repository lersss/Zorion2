// web/frontend_build_test.go
// Node-тест панели «Построить структуру» карточки планеты (спека
// 2026-09-24-постройка-структур-на-планете §5.2/§7/§11): чистые хелперы формы —
// опции типов (группы по class_name, серверный порядок, суффикс live=false),
// список владельца (подписи, цвет цветной точки, CSS-инъекция), заглушки поиска
// (мин. длина q ≥ 2, «Ничего не найдено»), разметка формы (подписи классов
// владельца, хинт расы) и экранирование серверных строк. Паттерн — как в
// frontend_structures_test.go: node исполняет чистый модуль (DOM не нужен).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildPanelInNode — чистые хелперы modal/build.js.
func TestBuildPanelInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модуля")
	}
	root := repoRoot(t)
	stateURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "state.js"))
	buildURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "build.js"))
	contractsURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "contracts.js"))
	script := strings.Replace(buildPanelScript, "__STATE__", stateURL, 1)
	script = strings.Replace(script, "__BUILD__", buildURL, 1)
	script = strings.Replace(script, "__CONTRACTS__", contractsURL, 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка панели «Построить» упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "BUILD_PANEL_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const buildPanelScript = `
globalThis.window = globalThis;
globalThis.localStorage = { getItem: () => null, setItem(){}, removeItem(){} };

const state = await import(new URL("__STATE__").href);
const build = await import(new URL("__BUILD__").href);
const contracts = await import(new URL("__CONTRACTS__").href);

function assert(cond, msg) { if (!cond) throw new Error(msg); }

// ---------- safeCssColor: только безопасные цветовые формы ----------
assert(contracts.safeCssColor('#abc') === '#abc', 'safeCssColor #rgb');
assert(contracts.safeCssColor('#a1b2c3') === '#a1b2c3', 'safeCssColor #rrggbb');
assert(contracts.safeCssColor('rgb(1, 2, 3)') === 'rgb(1, 2, 3)', 'safeCssColor rgb()');
assert(contracts.safeCssColor('rgba(1,2,3,0.5)') === 'rgba(1,2,3,0.5)', 'safeCssColor rgba()');
assert(contracts.safeCssColor('red') === '#64748b', 'safeCssColor имя → нейтральный');
assert(contracts.safeCssColor('red; background:url(x)') === '#64748b', 'safeCssColor CSS-инъекция → нейтральный');
assert(contracts.safeCssColor(null) === '#64748b', 'safeCssColor null → нейтральный');

// ---------- typeOptionsHtml: группы, порядок, суффикс, экранирование ----------
const types = [
    { id: 148, name: 'Поселение', class_id: 148, class_name: 'Поселение', target: 'settlement', live: true },
    { id: 152, name: 'Аутпост', class_id: 148, class_name: 'Поселение', target: 'settlement', live: true },
    { id: 200, name: 'Лаборатория', class_id: 190, class_name: 'Наука', target: 'building', live: false },
];
const opts = build.typeOptionsHtml(types);
assert(opts.includes('<optgroup label="Поселение">'), 'группа по class_name');
assert(opts.includes('<optgroup label="Наука">'), 'вторая группа');
assert(opts.indexOf('value="148">Поселение') < opts.indexOf('value="152">Аутпост'), 'серверный порядок внутри группы');
assert(opts.includes('value="152">Аутпост</option>'), 'live-тип без суффикса');
assert(!opts.includes('Аутпост — '), 'live-тип: суффикса нет');
assert(opts.includes('Лаборатория — производство ещё не реализовано</option>'), 'неживой тип: суффикс §7');
assert(build.typeOptionsHtml([]).includes('— выберите тип —'), 'пустой список — только placeholder');

const xssTypes = [{ id: 1, name: '<img src=x onerror=alert(1)>', class_id: 2, class_name: '<b>Класс</b>', live: true }];
const xssOpts = build.typeOptionsHtml(xssTypes);
assert(!xssOpts.includes('<img src=x onerror=alert(1)>'), 'type name не как тег');
assert(xssOpts.includes('&lt;img src=x onerror=alert(1)&gt;'), 'type name экранирован');
assert(!xssOpts.includes('<b>Класс</b>') && xssOpts.includes('&lt;b&gt;Класс&lt;/b&gt;'), 'class_name экранирован');

// ---------- ownerListHtml: подписи, подсветка, цвет, XSS ----------
const items = [
    { id: 'f1', name: 'Люди', subtitle: 'Корпорация', color: '#ff6b6b' },
    { id: 'f2', name: 'Звери', subtitle: 'Клан', color: '#00ff00' },
];
const list = build.ownerListHtml(items, 'f1');
assert(list.includes('data-owner-pick="f1"') && list.includes('data-owner-name="Люди"'), 'data-атрибуты кандидата');
assert(list.includes('Люди — Корпорация'), 'подпись фракции (имя — тип)');
assert(list.includes('background:#ff6b6b'), 'валидный цвет проходит');
assert(list.includes('background:#2a2a4a'), 'выбранная строка подсвечена');
assert(list.includes('Звери — Клан'), 'вторая строка');

// Пусто — «Ничего нет» (список фракций без кандидатов).
assert(build.ownerListHtml([], null).includes('Ничего нет'), 'пустой список → «Ничего нет»');

// XSS + CSS-инъекция: строки и цвет с сервера.
const evil = [{ id: '"><script>', name: '<b>boom</b>', subtitle: '</div><img src=x>', color: 'red; background-image:url(javascript:alert(1))' }];
const evilList = build.ownerListHtml(evil, null);
assert(!evilList.includes('<b>boom</b>') && evilList.includes('&lt;b&gt;boom&lt;/b&gt;'), 'owner name экранирован');
assert(!evilList.includes('<img src=x>'), 'subtitle экранирован');
assert(!evilList.includes('javascript:alert(1)') && !evilList.includes('red;'), 'CSS-инъекция цвета не прошла');
assert(evilList.includes('background:#64748b'), 'невалидный цвет → нейтральный');

// ---------- ownerSearchMessageHtml: мин. длина и пустой результат ----------
assert(build.ownerSearchMessageHtml('', 0).includes('Введите минимум 2 символа'), 'q пустой → подсказка');
assert(build.ownerSearchMessageHtml('a', 5).includes('Введите минимум 2 символа'), 'q 1 символ → подсказка');
assert(build.ownerSearchMessageHtml(' ab ', 0).includes('Ничего не найдено'), 'trim: 2 символа, пустой результат');
assert(build.ownerSearchMessageHtml('ab', 0).includes('Ничего не найдено'), 'пустой результат → «Ничего не найдено»');
assert(build.ownerSearchMessageHtml('ab', 3) === '', 'есть результаты → заглушки нет');

// ---------- formHtml: структура формы, подписи владельца, хинт расы ----------
const form = build.formHtml({
    types: types,
    default_owner: { owner_type: 'player', owner_id: 'u1', name: 'Вася' },
    default_race: { race_id: 'humans', race_name: 'Люди' },
    population_fallback: 1000,
});
assert(form.includes('id="build-type"') && form.includes('Аутпост'), 'селект типов внутри формы');
assert(form.includes('id="build-owner-type"'), 'селект класса владельца');
assert(form.includes('>Фракция</option>') && form.includes('>Игрок</option>') && form.includes('>Агент</option>'), 'подписи классов владельца');
assert(form.includes('value="player" selected'), 'префилл класса владельца из default_owner');
assert(form.includes('Раса: Люди (по региону планеты)'), 'хинт расы');
assert(form.includes('id="build-submit-btn"'), 'кнопка «Построить»');

// Нет расы — нейтральный хинт; нет default_owner — класс по умолчанию faction.
assert(build.formHtml({ types: [] }).includes('Раса: регион без расы — люди'), 'нет расы → люди');
assert(build.formHtml({ types: [] }).includes('value="faction" selected'), 'нет префилла → faction');

// XSS: race_name с сервера экранируется.
const evilForm = build.formHtml({ types: [], default_race: { race_name: '<img src=x onerror=alert(1)>' } });
assert(!evilForm.includes('<img src=x onerror=alert(1)>') && evilForm.includes('&lt;img src=x onerror=alert(1)&gt;'), 'race_name экранирован');

console.log('BUILD_PANEL_OK');
`
