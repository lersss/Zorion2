// web/frontend_knowledge_tabs_test.go
// Node-тест клиентской карточки планеты — режимы знания (спека 2026-09-23-
// орбита-планеты-присутствие-и-снимок §5.1/§6.1/§6.2, этап Э4/T15–T17):
// knowledgeMode/knowledgeStripHtml (живое/память/устарело/скан/нет),
// renderSettlements (эффекты только в presence; в snapshot нет эффектов, лога и
// стрелки тренда), управление «купить отчёт» только по can_buy_report, вкладка
// «Фракции» — нейтральное пустое состояние. Паттерн — frontend_modal_sizes_test.go:
// node исполняет модуль с минимальной заглушкой DOM (tabs.js тянет ui/toast.js).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestKnowledgeTabsInNode — knowledgeMode/knowledgeStripHtml/renderSettlements в modal/tabs.js.
func TestKnowledgeTabsInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модуля")
	}
	root := repoRoot(t)
	tabsURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "tabs.js"))
	stateURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "state.js"))
	script := strings.Replace(knowledgeTabsScript, "__TABS__", tabsURL, 1)
	script = strings.Replace(script, "__STATE__", stateURL, 1)

	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка карточки знания упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "KNOWLEDGE_TABS_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const knowledgeTabsScript = `
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

const state = await import(new URL("__STATE__").href);
const tabs = await import(new URL("__TABS__").href);

function assert(cond, msg) { if (!cond) throw new Error(msg); }

// ---------- knowledgeMode: режим с сервера (§5.1), none — знания нет ----------
state.modalState.role = 'player';
assert(tabs.knowledgeMode({ knowledge: { mode: 'presence' } }) === 'presence', 'mode presence');
assert(tabs.knowledgeMode({ knowledge: { mode: 'snapshot' } }) === 'snapshot', 'mode snapshot');
assert(tabs.knowledgeMode({ knowledge: { mode: 'scan' } }) === 'scan', 'mode scan');
assert(tabs.knowledgeMode({}) === 'none', 'mode none (нет знания)');
assert(tabs.knowledgeMode({ knowledge: {} }) === 'scan', 'knowledge без mode → scan (как раньше)');
state.modalState.role = 'admin';
assert(tabs.knowledgeMode({ knowledge: { mode: 'snapshot' } }) === 'admin', 'admin — режим admin');
state.modalState.role = 'skycomposer';
assert(tabs.knowledgeMode({}) === 'admin', 'skycomposer — режим admin');
state.modalState.role = 'player';

// ---------- knowledgeStripHtml: плашки §6.1 ----------
const presStrip = tabs.knowledgeStripHtml({ knowledge: { mode: 'presence' } });
assert(presStrip.includes('● Свежие данные'), 'presence: ● Свежие данные');
assert(!presStrip.includes('3026') && !presStrip.includes('20'), 'presence: даты нет');
assert(presStrip.includes('#4ade80'), 'presence: зелёный');

const snapStrip = tabs.knowledgeStripHtml({ knowledge: { mode: 'snapshot', snapshot_at: '2026-09-23T14:05:00Z', snapshot_fresh: true } });
assert(snapStrip.includes('📷 Данные на 23.09.3026, 14:05'), 'snapshot: дата UTC+1000');
assert(!snapStrip.includes('устарело'), 'snapshot свежий: без «устарело»');
assert(snapStrip.includes('#94a3b8'), 'snapshot: серый');

const snapOld = tabs.knowledgeStripHtml({ knowledge: { mode: 'snapshot', snapshot_at: '2026-09-01T00:00:00Z', snapshot_fresh: false } });
assert(snapOld.includes('устарело'), 'snapshot старый: « · устарело»');
assert(snapOld.includes('#fbbf24'), 'snapshot старый: янтарный');

const scanStrip = tabs.knowledgeStripHtml({ knowledge: { mode: 'scan', scanned_at: '2026-09-23T14:05:00Z', fresh: true } });
assert(scanStrip.includes('🔍 Данные сканера на 23.09.3026, 14:05'), 'scan: строка сканера с датой');
assert(!scanStrip.includes('устарело'), 'scan свежий: без «устарело»');
const scanOld = tabs.knowledgeStripHtml({ knowledge: { mode: 'scan', scanned_at: '2026-09-01T00:00:00Z', fresh: false } });
assert(scanOld.includes('устарело') && scanOld.includes('#fbbf24'), 'scan старый: янтарный + устарело');

const noneStrip = tabs.knowledgeStripHtml({});
assert(noneStrip.includes('Нет данных — купить отчёт'), 'none: текст заглушки');
assert(noneStrip.includes('dashed'), 'none: пунктирная серая');

// admin — плашек нет.
state.modalState.role = 'admin';
assert(tabs.knowledgeStripHtml({ knowledge: { mode: 'presence' } }) === '', 'admin: плашек не рисуем');
assert(tabs.knowledgeStripHtml({ knowledge: { mode: 'snapshot', snapshot_at: '2026-09-23T14:05:00Z' } }) === '', 'admin: плашек нет (snapshot)');
state.modalState.role = 'player';

// ---------- renderSettlements: присутствие — полная карточка ----------
const settlement = {
    id: 's1', race_name: 'Люди', population: 100, stability: 60, r_per_sec: 0.5,
    branches: [{ id: 'b1', recipe_name: 'Пища', output: [{ good_id: 1, good_name: 'Еда', amount: 5 }] }],
    effects: [{ name: 'Голод', impact: 'population_rate', state: 'active' }],
    log: [{ occurred_at: '2026-09-23T14:05:00Z', cause: 'hunger' }]
};
const presPlanet = { id: 'p1', knowledge: { mode: 'presence' }, settlements: [settlement] };
let html = tabs.renderSettlements(presPlanet);
assert(html.includes('● Свежие данные'), 'presence: плашка живого');
assert(html.includes('Раса:'), 'presence: карточка поселения');
assert(html.includes('Эффекты') && html.includes('Голод'), 'presence: эффекты игроку видны');
assert(html.includes('влияет на население'), 'presence: подпись impact');
assert(html.includes('действует'), 'presence: состояние active');
assert(html.includes('💀 Вымерло') && html.includes('23.09.3026, 14:05'), 'presence: лог с датой');
assert(html.includes('Население убывает'), 'presence: стрелка тренда есть');
assert(html.includes('Пища'), 'presence: ветки видны');
assert(!html.includes('data-branch-create-form'), 'presence: админ-форм веток нет');
assert(!html.includes('data-effect-load-form'), 'presence: админ-формы эффектов нет');

// ---------- renderSettlements: снимок — без эффектов/лога/тренда ----------
const snapPlanet = { id: 'p1', knowledge: { mode: 'snapshot', snapshot_at: '2026-09-23T14:05:00Z', snapshot_fresh: true }, settlements: [settlement] };
html = tabs.renderSettlements(snapPlanet);
assert(html.includes('📷 Данные на 23.09.3026, 14:05'), 'snapshot: плашка памяти с датой');
assert(html.includes('Раса:'), 'snapshot: карточка поселения');
assert(html.includes('Пища'), 'snapshot: ветки (замороженные) видны');
assert(!html.includes('Эффекты'), 'snapshot: блока эффектов нет');
assert(!html.includes('влияет на население'), 'snapshot: impact не показан');
assert(!html.includes('💀 Вымерло'), 'snapshot: лога нет');
assert(!html.includes('Население убывает') && !html.includes('Население растёт'), 'snapshot: стрелки тренда нет');
const snapOldPlanet = { id: 'p1', knowledge: { mode: 'snapshot', snapshot_at: '2026-09-01T00:00:00Z', snapshot_fresh: false }, settlements: [settlement] };
assert(tabs.renderSettlements(snapOldPlanet).includes('устарело'), 'snapshot: устаревшая память помечена');

// ---------- renderSettlements: scan / none — прежние тексты ----------
const scanPlanet = { id: 'p1', knowledge: { mode: 'scan', scanned_at: '2026-09-23T14:05:00Z', fresh: true, settlements_count: 2 } };
html = tabs.renderSettlements(scanPlanet);
assert(html.includes('🔍 Данные сканера на 23.09.3026, 14:05'), 'scan: плашка');
assert(html.includes('Поселения (2)') && html.includes('Детали поселений — купить отчёт'), 'scan: прежний текст деталей');
html = tabs.renderSettlements({ id: 'p1' });
assert(html.includes('Нет данных — купить отчёт'), 'none: прежний текст');

// ---------- управление «купить отчёт» (§5.3/T17) ----------
assert(tabs.buyReportControlHtml({ can_buy_report: true }).includes('data-buy-report'), 'buy: элемент при can_buy_report');
assert(tabs.buyReportControlHtml({ can_buy_report: false }) === '', 'buy: нет при false');
assert(tabs.buyReportControlHtml({}) === '', 'buy: нет без поля');
assert(!tabs.renderSettlements({ id: 'p1' }).includes('data-buy-report'), 'none без флага: элемента нет');
assert(tabs.renderSettlements({ id: 'p1', can_buy_report: true }).includes('data-buy-report'), 'none с флагом: элемент есть');
assert(tabs.renderSettlements({ id: 'p1', knowledge: { mode: 'scan', scanned_at: '2026-09-23T14:05:00Z', fresh: true, settlements_count: 1 }, can_buy_report: true }).includes('data-buy-report'), 'scan с флагом: элемент есть');
assert(!tabs.renderSettlements(scanPlanet).includes('data-buy-report'), 'scan без флага: элемента нет');

// Клик по заглушке: запроса на покупку не уходит, только подпись.
let clicked = null;
const fakeBtn = { addEventListener(ev, cb) { if (ev === 'click') clicked = cb; } };
const statusEl = { textContent: '' };
const fakeContainer = {
    querySelector(sel) {
        if (sel === '[data-buy-report]') return fakeBtn;
        if (sel === '[data-buy-report-status]') return statusEl;
        return null;
    }
};
globalThis.fetch = () => { throw new Error('покупка не должна ходить на сервер'); };
tabs.initBuyReport({ can_buy_report: true }, fakeContainer);
assert(typeof clicked === 'function', 'buy: обработчик навешен');
clicked();
assert(statusEl.textContent === 'Покупка пока недоступна', 'buy: точный текст подписи');

// ---------- вкладка «Фракции»: нейтральное пустое состояние (§6.2 п.7) ----------
const factionsHtml = tabs.renderFactions({ id: 'p1', knowledge: { mode: 'scan' }, factions: [] });
assert(factionsHtml.includes('Фракции не отмечены'), 'фракции: нейтральный текст');
assert(!factionsHtml.includes('В отчёте сканера'), 'фракции: устаревший текст снят');
assert(!factionsHtml.includes('сканера'), 'фракции: без упоминания сканера');

console.log('KNOWLEDGE_TABS_OK');
`
