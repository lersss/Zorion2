// web/frontend_ui_state_test.go
// Node-тест чистых помощников неразрушающей перерисовки правой панели модалки
// (баг 2026-09-25: автообновление сбрасывало состояние панели): сохранение
// раскрытых инлайн-деталей и выбор слота магазина при перерисовке. Паттерн — как
// в frontend_structures_test.go: node исполняет чистый модуль (DOM не нужен).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestUiStateInNode — modal/ui_state.js.
func TestUiStateInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модуля")
	}
	root := repoRoot(t)
	uiURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "ui_state.js"))
	script := strings.Replace(uiStateScript, "__UI_STATE__", uiURL, 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка ui_state.js упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "UI_STATE_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const uiStateScript = `
const ui = await import(new URL("__UI_STATE__").href);

function eq(name, got, want) {
    const a = JSON.stringify(got), b = JSON.stringify(want);
    if (a !== b) throw new Error(name + ': ' + a + ' != ' + b);
}

// mergeDetailIds: присутствующие id получают текущее состояние (раскрыт/свёрнут),
// отсутствующие сохраняют прежнее (перерисовка другой вкладки их не теряет).
eq('merge add expanded', ui.mergeDetailIds(['b1'], [{ id: 'b2', expanded: true }]), ['b1', 'b2']);
eq('merge collapse', ui.mergeDetailIds(['b1', 'b2'], [{ id: 'b1', expanded: false }]), ['b2']);
eq('merge keeps absent', ui.mergeDetailIds(['b1'], []), ['b1']);
eq('merge no dup', ui.mergeDetailIds(['b1'], [{ id: 'b1', expanded: true }]), ['b1']);
eq('merge null stored', ui.mergeDetailIds(null, [{ id: 'b1', expanded: true }]), ['b1']);
eq('merge numeric coerced', ui.mergeDetailIds([1], [{ id: 2, expanded: true }]), ['1', '2']);
eq('merge skips null id', ui.mergeDetailIds(['b1'], [{ id: null, expanded: true }]), ['b1']);

// expandDetailIds: раскрываем только сохранённые и присутствующие.
eq('expand filter', ui.expandDetailIds(['b1', 'b3'], ['b1', 'b2']), ['b1']);
eq('expand none', ui.expandDetailIds([], ['b1']), []);
eq('expand empty present', ui.expandDetailIds(['b1'], []), []);
eq('expand numeric', ui.expandDetailIds([1], [1, 2]), [1]);

// resolveMarketSlot: сохранённый валиден — остаётся; иначе первый; нет ключей — null.
eq('slot valid', ui.resolveMarketSlot(['universal', 'universal2'], 'universal2'), 'universal2');
eq('slot invalid falls back', ui.resolveMarketSlot(['universal', 'universal2'], 'nope'), 'universal');
eq('slot null first', ui.resolveMarketSlot(['universal', 'universal2'], null), 'universal');
eq('slot no keys', ui.resolveMarketSlot([], 'universal'), null);
eq('slot null keys', ui.resolveMarketSlot(null, 'universal'), null);

// isEditableControl: редактируемые контролы (input/select/textarea/contenteditable)
// откладывают авто-обновление; кнопки/прочее — нет.
eq('editable input', ui.isEditableControl({ tagName: 'INPUT' }), true);
eq('editable select lowercase', ui.isEditableControl({ tagName: 'select' }), true);
eq('editable textarea', ui.isEditableControl({ tagName: 'TEXTAREA' }), true);
eq('editable contenteditable', ui.isEditableControl({ tagName: 'DIV', isContentEditable: true }), true);
eq('editable button', ui.isEditableControl({ tagName: 'BUTTON' }), false);
eq('editable div', ui.isEditableControl({ tagName: 'DIV' }), false);
eq('editable anchor', ui.isEditableControl({ tagName: 'A' }), false);
eq('editable null', ui.isEditableControl(null), false);

console.log('UI_STATE_OK');
`
