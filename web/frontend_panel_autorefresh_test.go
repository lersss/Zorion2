// web/frontend_panel_autorefresh_test.go
// Node-тест логики отложенного авто-обновления карточки планеты (баг №48:
// ручное «🔄 Обновить» при фокусе в редактируемом поле давало второй запрос —
// отложенный тик не сбрасывался, а уход фокуса из удаляемого при перерисовке
// поля применял его). Решающая проверка вынесена в чистую функцию
// resolveDeferredRefresh (panel.js) и покрыта здесь. Паттерн —
// frontend_knowledge_tabs_test.go: node исполняет модуль с минимальной
// заглушкой DOM (panel.js тянет tabs.js → ui/toast.js и т.п.).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPanelAutoRefreshInNode — resolveDeferredRefresh в modal/panel.js.
func TestPanelAutoRefreshInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := repoRoot(t)
	panelURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "panel.js"))
	script := strings.Replace(panelAutoRefreshScript, "__PANEL__", panelURL, 1)

	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка автообновления панели упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "PANEL_AUTOREFRESH_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const panelAutoRefreshScript = `
globalThis.window = globalThis;
globalThis.localStorage = { getItem: () => null, setItem(){}, removeItem(){} };
globalThis.document = {
    getElementById: () => null,
    createElement: () => ({ style: {}, setAttribute(){}, appendChild(){}, addEventListener(){} }),
    head: { appendChild(){} },
    body: { appendChild(){} },
    addEventListener(){}, removeEventListener(){},
    documentElement: { style: {} },
    querySelectorAll: () => [],
    querySelector: () => null,
};

const p = await import(new URL("__PANEL__").href);

function eq(name, got, want) {
    if (got !== want) throw new Error(name + ': ' + got + ' != ' + want);
}

const state = { autoRefreshDeferred: false };

// Тик при фокусе на редактируемом поле правой панели — откладываем и копим флаг.
eq('tick busy defers', p.resolveDeferredRefresh(state, 'tick', true), 'defer');
eq('flag set while busy', state.autoRefreshDeferred, true);

// Фокус ушёл из панели — применяем отложенное один раз, флаг снят.
eq('focusout applies deferred', p.resolveDeferredRefresh(state, 'focusout', false), 'refresh');
eq('flag cleared after apply', state.autoRefreshDeferred, false);
// Повторный focusout уже ничего не применяет (флаг одноразовый).
eq('focusout no pending', p.resolveDeferredRefresh(state, 'focusout', false), 'none');

// Переход фокуса внутри панели обновление не запускает, флаг живёт.
state.autoRefreshDeferred = true;
eq('focusout inside panel none', p.resolveDeferredRefresh(state, 'focusout', true), 'none');
eq('flag kept while focus inside', state.autoRefreshDeferred, true);

// Тик без фокуса на контроле — обновляем сразу, флаг снят.
eq('tick free refreshes', p.resolveDeferredRefresh(state, 'tick', false), 'refresh');
eq('flag cleared on free tick', state.autoRefreshDeferred, false);

// --- Баг №48: ручное «Обновить» при отложенном тике ---
state.autoRefreshDeferred = false;
eq('tick busy again', p.resolveDeferredRefresh(state, 'tick', true), 'defer');
eq('flag pending before manual', state.autoRefreshDeferred, true);
// Игрок жмёт «🔄 Обновить»: свежие данные придут этим запросом.
eq('manual refreshes', p.resolveDeferredRefresh(state, 'manual'), 'refresh');
eq('manual clears deferred', state.autoRefreshDeferred, false);
// Перерисовка удалила сфокусированное поле → делегированный focusout:
// второго запроса быть не должно.
eq('no second request after manual', p.resolveDeferredRefresh(state, 'focusout', false), 'none');

console.log('PANEL_AUTOREFRESH_OK');
`
