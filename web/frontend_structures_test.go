// web/frontend_structures_test.go
// Node-тест вкладки «Строения» карточки планеты (спека
// 2026-09-24-постройка-структур-на-планете §10.1/Р14): видимость вкладки,
// подпись типа строения (столица), список всех buildings с пометками
// «не производит» / «производство ещё не реализовано», имя владельца
// (NULL → «NPC (без владельца)»), экранирование серверных строк. Паттерн — как
// в frontend_contracts_test.go: node исполняет чистый модуль (DOM не нужен).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStructuresTabInNode — modal/structures.js.
func TestStructuresTabInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модуля")
	}
	root := repoRoot(t)
	stateURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "state.js"))
	structURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "structures.js"))
	script := strings.Replace(structuresTabScript, "__STATE__", stateURL, 1)
	script = strings.Replace(script, "__STRUCTURES__", structURL, 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка вкладки «Строения» упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "STRUCTURES_TAB_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const structuresTabScript = `
globalThis.window = globalThis;
globalThis.localStorage = { getItem: () => null, setItem(){}, removeItem(){} };

const state = await import(new URL("__STATE__").href);
const s = await import(new URL("__STRUCTURES__").href);

function assert(cond, msg) { if (!cond) throw new Error(msg); }

// Подпись типа строения: известный ключ — человекочитаемо, неизвестный — как
// есть, пусто — «—».
assert(s.buildingTypeLabel('capital') === 'Столица', 'label capital');
assert(s.buildingTypeLabel('producer') === 'producer', 'label unknown passthrough');
assert(s.buildingTypeLabel('') === '—' && s.buildingTypeLabel(null) === '—', 'label empty');

// Видимость вкладки: админ — всегда; игроку — по наличию блока buildings
// (в none сервер обнуляет buildings → вкладки нет).
state.modalState.role = 'admin';
assert(s.structuresTabVisible({}) === true, 'admin: вкладка всегда');
state.modalState.role = 'player';
assert(s.structuresTabVisible({ buildings: null }) === false, 'player: нет блока → нет вкладки');
assert(s.structuresTabVisible({}) === false, 'player: поля нет → нет вкладки');
assert(s.structuresTabVisible({ buildings: [] }) === true, 'player: пустой список → вкладка есть');
assert(s.structuresTabVisible({ buildings: [{ id: 'b1' }] }) === true, 'player: список → вкладка есть');
state.modalState.role = 'player';

// Пусто → «Строений нет».
assert(s.renderStructures({ buildings: [] }).includes('Строений нет'), 'пусто');
assert(s.renderStructures({}).includes('Строений нет'), 'нет поля');

// Столица: type_name пуст → buildingTypeLabel; producer_type_id null →
// «не производит»; владелец-фракция красится цветом из planet.factions.
const planet = {
    factions: [{ id: 'f1', name: 'Люди', color: '#ff6b6b' }],
    buildings: [
        { id: 'b1', building_type: 'capital', owner_type: 'faction', owner_id: 'f1', owner_name: 'Люди', producer_type_id: null },
        { id: 'b2', building_type: 'producer', type_name: 'Лаборатория', owner_type: 'player', owner_id: 'u1', owner_name: 'Вася', producer_type_id: 152 },
        { id: 'b3', building_type: 'producer', type_name: 'Форпост', owner_type: 'player', owner_id: 'u2', owner_name: '', producer_type_id: 153 },
    ],
};
const html = s.renderStructures(planet);
assert(html.includes('Строения (3)'), 'счётчик всех построек (Р14)');
assert(html.includes('Столица') && html.includes('не производит'), 'столица: имя и пометка');
assert(html.includes('Лаборатория') && html.includes('производство ещё не реализовано'), 'строение: имя и пометка');
assert(html.includes('Владелец: Люди') && html.includes('Владелец: Вася'), 'владельцы');
assert(html.includes('NPC (без владельца)'), 'пустой владелец → NPC');
assert(html.includes('#ff6b6b'), 'цвет владельца-фракции из planet.factions');
assert(html.includes('data-building-toggle="b1"') && html.includes('data-building-details="b1"'), 'тоггл деталей');

// CSS-инъекция в цвете владельца-фракции: невалидный цвет → нейтральный.
const evilColorPlanet = {
    factions: [{ id: 'f9', name: 'Злые', color: 'red; background:url(x)' }],
    buildings: [
        { id: 'b9', building_type: 'producer', type_name: 'Фабрика', owner_type: 'faction', owner_id: 'f9', owner_name: 'Злые', producer_type_id: 9 },
    ],
};
const evilColorHtml = s.renderStructures(evilColorPlanet);
assert(!evilColorHtml.includes('red; background:url(x)'), 'CSS-инъекция цвета не прошла');
assert(evilColorHtml.includes('background:#64748b'), 'невалидный цвет → нейтральный');

// XSS: серверные type_name/owner_name экранируются.
const xss = s.renderStructures({ buildings: [
    { id: 'x', building_type: 'producer', type_name: '<img src=x onerror=alert(1)>', owner_name: '<b>boom</b>', producer_type_id: 1 },
] });
assert(!xss.includes('<img src=x onerror=alert(1)>'), 'xss type_name как тег');
assert(xss.includes('&lt;img src=x onerror=alert(1)&gt;'), 'xss type_name экранирован');
assert(!xss.includes('<b>boom</b>') && xss.includes('&lt;b&gt;boom&lt;/b&gt;'), 'xss owner_name экранирован');

console.log('STRUCTURES_TAB_OK');
`
