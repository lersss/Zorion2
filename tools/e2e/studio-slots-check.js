// tools/e2e/studio-slots-check.js
// Browser test: студия — слоты родителя (переработанная фича, спека
// 2026-09-21-студия-скрытые-категории-строений, вторая волна), ветка
// «Производители». Проверяет: дерево из слотов (K7), скрытие пустого слота
// с галки карточки родителя (K8), заглушку «нет заводов · скрыта» (K9),
// редактор слотов (K10), обратный кейс «база скрыта + переопределение» (K11),
// select категорий по уровню записи (K12), список справа (K13), F5/zoom/pan
// (K14), регресс items/energy и приглашений (K15), консоль (K16).
//
// Run: node studio-slots-check.js
//   BASE_URL env overrides default http://127.0.0.1:8080
//   STUDIO_USER / STUDIO_PASS — учётка admin/skycomposer
// Requires: game server running on 8080, миграции 000052/000053 применены.
// Данные: база Фабрики = 13 товарных категорий (cat 7..19), слоты 48.
// Скрипт создаёт/правит слоты (Фабрика, топливо=8) на уровнях universal/F1
// и удаляет их в cleanup (возврат к 48 слотам).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const STUDIO_USER = process.env.STUDIO_USER || '';
const STUDIO_PASS = process.env.STUDIO_PASS || '';
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');

const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

const results = [];
function report(stepName, status, detail) {
  results.push({ stepName, status, detail });
  console.log(`[${stepName}] ${status}${detail ? ' - ' + detail : ''}`);
}

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

let token = '';
async function api(method, url, body, auth = true) {
  const headers = { 'Content-Type': 'application/json' };
  if (auth && token) headers['Authorization'] = 'Bearer ' + token;
  const r = await fetch(BASE_URL + url, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  let data = null;
  try { data = await r.json(); } catch (e) {}
  return { status: r.status, data };
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);
  if (!STUDIO_USER || !STUDIO_PASS) {
    report('setup creds', 'FAIL', 'STUDIO_USER/STUDIO_PASS env not set');
    return finish(1);
  }

  const login = await api('POST', '/login', { username: STUDIO_USER, password: STUDIO_PASS }, false);
  if (login.status !== 200 || !login.data || !login.data.token) {
    report('login', 'FAIL', 'POST /login status=' + login.status);
    return finish(1);
  }
  token = login.data.token;
  report('login', 'PASS', 'role=' + (login.data.user && login.data.user.role));

  // --- подготовка: убрать тестовые слоты (Фабрика, топливо=8) с прошлых прогонов ---
  const st0 = (await api('GET', '/studio/api/state')).data;
  const cleanupSlots = st0.producer_slots.filter(s => s.parent_id === 2 && s.category_id === 8 && s.race_family);
  for (const s of cleanupSlots) await api('DELETE', '/studio/api/slots/' + s.id);
  // мусор прерванного прогона: универсальный слот «Минералы» (cat=1, resource)
  // у Фабрики неканоничен (сид даёт только товарные категории) — иначе K7
  // увидит 14 категорий вместо 13. Канон-каталог не трогаем.
  const junkSlots = st0.producer_slots.filter(s => s.parent_id === 2 && s.category_id === 1 && !s.race_family);
  for (const s of junkSlots) await api('DELETE', '/studio/api/slots/' + s.id);
  // канон: все базовые универсальные слоты Фабрики видимы (прерванные прогоны
  // могли включить у них hidden — иначе K7 видит не 13 категорий)
  const baseHidden = st0.producer_slots.filter(s => s.parent_id === 2 && !s.race_family && s.hidden);
  for (const s of baseHidden) await api('PUT', '/studio/api/slots/' + s.id, { hidden: false });
  const base8 = st0.producer_slots.find(s => s.parent_id === 2 && s.category_id === 8 && !s.race_family);
  report('setup data', 'PASS', `base8=${base8 && base8.id} cleaned=${cleanupSlots.length} junk=${junkSlots.length} baseUnhidden=${baseHidden.length}`);

  const exe = findExecutable();
  if (!exe) { report('setup browser', 'FAIL', 'no Chrome/Edge found'); return finish(1); }
  console.log('browser: ' + exe.name + ' (' + exe.path + ')');

  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('setup browser', 'FAIL', 'launch: ' + String(err && err.message ? err.message : err));
    return finish(1);
  }
  const context = await browser.newContext({ viewport: { width: 1400, height: 900 } });
  await context.addInitScript((t) => localStorage.setItem('adminToken', t), token);
  const page = await context.newPage();
  const pageErrors = [];
  const apiRequests = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.stack ? err.stack : err)));
  page.on('request', (req) => {
    if (req.url().includes('/studio/api/')) apiRequests.push(req.method() + ' ' + req.url().replace(BASE_URL, ''));
  });

  const waitFor = (fn, timeout = 8000, desc = 'condition', arg) =>
    page.waitForFunction(fn, arg, { timeout }).catch(() => { throw new Error('timeout waiting: ' + desc); });

  // хелпер: узлы дерева (id → есть/нет)
  const treeHas = (ids) => page.evaluate((ids) => {
    const L = prodTreeLayout();
    const out = {};
    for (const id of ids) out[id] = !!L.pos[id];
    return out;
  }, ids);

  try {
    // ============ загрузка + ветка «Производители» ============
    await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 10000, 'spinner removed');
    await page.click('#branchProdFactory'); // раздел «Фабрики»: слоты типа 2 (спека 2026-09-25)
    await waitFor(() => document.getElementById('segProdRace').style.display !== 'none', 5000, 'producers branch');
    await waitFor(() => state.producer_slots.length > 0, 8000, 'slots loaded');

    // ============ K7: дерево из слотов (13 категорий базы, не «всё подряд») ============
    const k7 = await page.evaluate(() => {
      const L = prodTreeLayout();
      const fab = L.root && (function find(n){ if (n.id === 'type:2') return n; for (const c of n.children||[]) { const r = find(c); if (r) return r; } return null; })(L.root);
      const cats = fab ? fab.children.map(c => c.label) : [];
      const hasInvite = fab ? fab.children.some(c => c.kind === 'invite') : false;
      return { catCount: cats.length, cats: cats.slice(0, 20), hasInvite };
    });
    // категория вне набора (Минералы=1) не рисуется на universal
    const k7b = await treeHas(['invite:2:1', 'sub:17']);
    const k7ok = k7.catCount === 13 && k7.hasInvite && !k7b['invite:2:1'];
    report('K7 tree from slots', k7ok ? 'PASS' : 'FAIL', `cats=${k7.catCount} invite=${k7.hasInvite} minerals=${k7b['invite:2:1']}`);

    // ============ K8: скрытие пустого слота с галки карточки родителя ============
    // на F1: категория «топливо» (8) унаследована (база) → галка «скрытая» → POST переопределение
    await page.evaluate(() => { setProdRaceLevel('family'); setProdRaceFamily('F1'); renderAll(); });
    await page.waitForTimeout(300);
    await page.evaluate(() => openProdNodePopup('type:2')); // карточка Фабрики
    await page.waitForTimeout(300);
    const k8a = await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
      return {
        hasSlotEditor: !!document.querySelector('#popupBody .prod-slots-list'),
        rowFound: !!top,
        inherited: top ? top.textContent.includes('унаследован') : null,
        chk: top ? top.querySelector('input[type=checkbox]').checked : null,
      };
    });
    // кликнуть галку «скрытая» у топлива
    await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
      if (top) top.querySelector('input[type=checkbox]').click();
    });
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 8 && x.race_family === 'F1'); return s && s.hidden === true; }, 8000, 'F1 slot hidden=true created');
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    // на F1 категория топливо скрыта — ни карточки, ни приглашения
    const k8f1 = await treeHas(['invite:2:8', 'sub:32']);
    // на universal — видна (база не тронута)
    await page.evaluate(() => { setProdRaceLevel('universal'); renderAll(); });
    await page.waitForTimeout(300);
    const k8uni = await treeHas(['invite:2:8']);
    const k8ok = k8a.hasSlotEditor && k8a.rowFound && k8a.inherited === true && k8a.chk === false &&
      !k8f1['invite:2:8'] && !k8f1['sub:32'] && k8uni['invite:2:8'];
    report('K8 hide empty slot from parent card', k8ok ? 'PASS' : 'FAIL',
      `editor=${k8a.hasSlotEditor} inherited=${k8a.inherited} f1=${JSON.stringify(k8f1)} uni=${JSON.stringify(k8uni)}`);

    // ============ K9: галка «показать скрытые» на F1 → заглушка «нет заводов · скрыта» ============
    await page.evaluate(() => { setProdRaceLevel('family'); setProdRaceFamily('F1'); renderAll(); });
    await page.check('#prodShowHidden');
    await page.waitForTimeout(300);
    const k9 = await page.evaluate(() => {
      const L = prodTreeLayout();
      const stub = L.root && (function find(n){ if (n.id === 'hstub:2:8') return n; for (const c of n.children||[]) { const r = find(c); if (r) return r; } return null; })(L.root);
      return { hasStub: !!L.pos['hstub:2:8'], kind: stub ? stub.kind : null, label: stub ? stub.label : null };
    });
    // клик по заглушке → карточка родителя (Фабрика)
    await page.evaluate(() => openProdNodePopup('hstub:2:8'));
    await page.waitForTimeout(300);
    const k9b = await page.evaluate(() => ({
      popupTitle: document.getElementById('popupTitle').textContent,
      hasSlotEditor: !!document.querySelector('#popupBody .prod-slots-list'),
    }));
    const k9ok = k9.hasStub && k9.kind === 'hiddenStub' && k9b.popupTitle === 'Фабрика' && k9b.hasSlotEditor;
    report('K9 hidden stub + click to parent', k9ok ? 'PASS' : 'FAIL', `stub=${JSON.stringify(k9)} popup=${JSON.stringify(k9b)}`);
    await page.keyboard.press('Escape');

    // ============ K10: редактор слотов — пометки, «−» переопределения без модалки ============
    // сейчас на F1: топливо — переопределение (hidden=true). Снять галку → PUT (собственный слот)
    await page.evaluate(() => openProdNodePopup('type:2'));
    await page.waitForTimeout(300);
    const k10a = await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
      return { override: top ? top.textContent.includes('переопределён') : null, chk: top ? top.querySelector('input[type=checkbox]').checked : null };
    });
    // «−» у переопределения — без модалки (возврат к «унаследован»)
    await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
      if (top) { const btn = top.querySelector('button'); if (btn) btn.click(); }
    });
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 8 && x.race_family === 'F1'); return !s; }, 8000, 'F1 override removed');
    await page.waitForTimeout(400); // дождаться перерисовки попапа после fetchState
    const k10b = await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
      return { inherited: top ? top.textContent.includes('унаследован') : null, modalOpen: document.getElementById('modalOverlay').style.display === 'flex' };
    });
    // «−» у базового слота с заводами → модалка → 409-тост
    await page.evaluate(() => { setProdRaceLevel('universal'); renderAll(); });
    await page.waitForTimeout(300);
    await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const prod = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'продовольствие');
      if (prod) { const btn = prod.querySelector('button'); if (btn) btn.click(); }
    });
    await waitFor(() => document.getElementById('modalOverlay').style.display === 'flex', 5000, 'confirm modal');
    const k10c = await page.evaluate(() => ({ modalOpen: document.getElementById('modalOverlay').style.display === 'flex' }));
    await page.click('#modalBtns button[data-primary]');
    await waitFor(() => document.getElementById('report').textContent.includes('сначала удалите заводы'), 8000, '409 toast');
    const k10d = await page.evaluate(() => document.getElementById('report').textContent);
    // «+ слот категории» — добавляет категорию вне набора (Минералы=1) на universal
    await page.evaluate(() => {
      const sel = document.getElementById('prodNewSlotCat');
      if (sel) { sel.value = '1'; }
    });
    await page.click('#popupBody button:has-text("+ слот")');
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 1 && !x.race_family); return !!s; }, 8000, 'universal slot cat=1 created');
    const k10ok = k10a.override === true && k10a.chk === true && k10b.inherited === true && !k10b.modalOpen &&
      k10c.modalOpen && k10d.includes('сначала удалите заводы');
    report('K10 slot editor', k10ok ? 'PASS' : 'FAIL',
      `override=${k10a.override} afterMinus=${JSON.stringify(k10b)} modal=${k10c.modalOpen} toast409=${k10d.includes('сначала удалите заводы')}`);
    await page.keyboard.press('Escape');

    // ============ K11: обратный кейс — база скрыта + переопределение visible на F1 ============
    // скрыть базу (Фабрика, топливо=8, universal) через галку на universal
    await page.evaluate(() => openProdNodePopup('type:2'));
    await page.waitForTimeout(300);
    await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
      if (top && !top.querySelector('input[type=checkbox]').checked) top.querySelector('input[type=checkbox]').click();
    });
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 8 && !x.race_family); return s && s.hidden === true; }, 8000, 'base8 hidden=true');
    await page.keyboard.press('Escape');
    // на F1 создать переопределение visible (снять галку у унаследованного скрытого)
    await page.evaluate(() => { setProdRaceLevel('family'); setProdRaceFamily('F1'); renderAll(); });
    await page.waitForTimeout(300);
    await page.evaluate(() => openProdNodePopup('type:2'));
    await page.waitForTimeout(300);
    const k11a = await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
      return { chk: top ? top.querySelector('input[type=checkbox]').checked : null };
    });
    await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
      if (top && top.querySelector('input[type=checkbox]').checked) top.querySelector('input[type=checkbox]').click();
    });
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 8 && x.race_family === 'F1'); return s && s.hidden === false; }, 8000, 'F1 override visible');
    await page.waitForTimeout(400); // перерисовка попапа — хинт «база скрыта» появляется после создания переопределения
    const k11hint = await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
      return { hint: top ? top.textContent.includes('база скрыта') : null, override: top ? top.textContent.includes('переопределён') : null };
    });
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    // на F1 категория видна (приглашение), на universal — скрыта (без галки ничего)
    const k11f1 = await treeHas(['invite:2:8']);
    await page.evaluate(() => { setProdRaceLevel('universal'); renderAll(); });
    await page.waitForTimeout(300);
    const k11uni = await treeHas(['invite:2:8']);
    const k11ok = k11a.chk === true && k11hint.hint === true && k11hint.override === true && k11f1['invite:2:8'] && !k11uni['invite:2:8'];
    report('K11 reverse case base hidden + override', k11ok ? 'PASS' : 'FAIL',
      `chk=${k11a.chk} hint=${JSON.stringify(k11hint)} f1=${JSON.stringify(k11f1)} uni=${JSON.stringify(k11uni)}`);

    // ============ K12: select категорий в попапе записи по уровню записи ============
    // универсальная запись «продовольствие» (cat=7) при уровне UI «Семейство F4»
    // видит универсальные категории (базу), не F4-набор. Берём существующую
    // запись (жёсткий id=51 устарел; новую не создаём — уникальность подтипа).
    const stK12 = (await api('GET', '/studio/api/state')).data;
    let qaFood = stK12.producer_types.find(p => p.parent_id === 2 && p.category_id === 7 && !p.race_family && !p.race);
    if (!qaFood) qaFood = (await api('POST', '/studio/api/producers', { name: 'QA_Еда', kind: 'goods', category_id: 7, parent_id: 2 })).data;
    await page.evaluate(() => fetchState());
    await waitFor((id) => state.producer_types.some(p => p.id === id), 8000, 'record cat7 in state', qaFood.id);
    await page.evaluate(() => { setProdRaceLevel('family'); setProdRaceFamily('F4'); renderAll(); });
    await page.waitForTimeout(300);
    await page.evaluate((id) => openProdPopup(id), qaFood.id);
    await page.waitForTimeout(300);
    const k12 = await page.evaluate(() => {
      const sel = document.querySelector('#popupBody .dcat');
      const opts = sel ? [...sel.options].map(o => o.textContent) : [];
      return { hasCatSelect: !!sel, optCount: opts.length, hasFood: opts.includes('продовольствие'), hasOrganics: opts.includes('Органика') };
    });
    const k12ok = k12.hasCatSelect && k12.hasFood && !k12.hasOrganics;
    report('K12 category select by record level', k12ok ? 'PASS' : 'FAIL', JSON.stringify(k12));
    await page.keyboard.press('Escape');

    // ============ K13: список справа ============
    // скрытая категория «топливо» (база hidden, K11): создадим универсальную запись cat=8
    // (С4: слот базы существует) → без галки не видна, с галкой — видна с бейджем
    const qaTop = (await api('POST', '/studio/api/producers', { name: 'QA_Топливо_универс', kind: 'goods', category_id: 8, parent_id: 2 })).data;
    await waitFor((id) => state.producer_types.some(p => p.id === id), 8000, 'QA_Топливо_универс in state', qaTop.id);
    await page.evaluate(() => { setProdRaceLevel('universal'); renderAll(); });
    await page.uncheck('#prodShowHidden');
    await page.waitForTimeout(300);
    const k13off = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      return { hasTop: cards.some(c => c.textContent.includes('QA_Топливо_универс')) };
    });
    await page.check('#prodShowHidden');
    await page.waitForTimeout(300);
    const k13on = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      const top = cards.find(c => c.textContent.includes('QA_Топливо_универс'));
      return { hasTop: !!top, badge: top ? top.textContent.includes('скрыта') : false };
    });
    // клик по записи списка → существующий узел
    await page.evaluate(() => { const el = [...document.querySelectorAll('#spravList .card')].find(c => c.textContent.includes('QA_Топливо_универс')); if (el) el.click(); });
    await page.waitForTimeout(300);
    const k13click = await page.evaluate(() => {
      const L = prodTreeLayout();
      return { selected, nodeInTree: !!L.pos[selected] };
    });
    const k13ok = !k13off.hasTop && k13on.hasTop && k13on.badge && k13click.nodeInTree;
    report('K13 sprav filter', k13ok ? 'PASS' : 'FAIL',
      `off=${JSON.stringify(k13off)} on=${JSON.stringify(k13on)} click=${JSON.stringify(k13click)}`);

    // ============ K14: F5 + zoom/pan + no server calls ============
    const lsBefore = await page.evaluate(() => localStorage.getItem('gs_prodShowHidden'));
    await page.reload({ waitUntil: 'domcontentloaded' });
    await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 10000, 'spinner after reload');
    await page.click('#branchProdFactory'); // раздел «Фабрики»: слоты типа 2 (спека 2026-09-25)
    await waitFor(() => document.getElementById('segProdRace').style.display !== 'none', 5000, 'branch after reload');
    const k14a = await page.evaluate(() => ({ checked: document.getElementById('prodShowHidden').checked, ls: localStorage.getItem('gs_prodShowHidden') }));
    const apiBefore = apiRequests.length;
    await page.evaluate(() => { zoom = 2.5; panX = 123; panY = -45; });
    await page.uncheck('#prodShowHidden');
    await page.waitForTimeout(300);
    const k14b = await page.evaluate(() => ({ zoom, panX, panY }));
    await page.check('#prodShowHidden');
    await page.waitForTimeout(300);
    const apiAfter = apiRequests.length;
    const k14ok = k14a.checked === true && k14a.ls === '1' &&
      (k14b.zoom === 2.5 && k14b.panX === 123 && k14b.panY === -45) && (apiAfter === apiBefore);
    report('K14 F5 + zoom/pan + no server', k14ok ? 'PASS' : 'FAIL',
      `afterF5=${JSON.stringify(k14a)} zoompan=${JSON.stringify(k14b)} apiCalls=${apiAfter - apiBefore}`);

    // ============ K15: регресс items/energy + приглашение создаёт завод ============
    // лаборатории (items) — в своём разделе «Лаборатории» (спека 2026-09-25),
    // проверяем их на полотне этого раздела
    await page.click('#branchProdLab');
    await page.waitForTimeout(300);
    const k15a = await page.evaluate(() => {
      const L = prodTreeLayout();
      return { lab6: !!L.pos['sub:6'], lab7: !!L.pos['sub:7'], lab8: !!L.pos['sub:8'] };
    });
    await page.click('#branchProdFactory');
    await page.waitForTimeout(300);
    // приглашение видимого слота создаёт завод (С4 проходит): берём видимую
    // категорию без записей — «оружие» (17); у «химикатов» (10) запись уже есть
    await page.evaluate(() => { setProdRaceLevel('universal'); renderAll(); });
    await page.waitForTimeout(300);
    await page.evaluate(() => openProdNodePopup('invite:2:17'));
    await page.waitForTimeout(300);
    const k15b = await page.evaluate(() => {
      const inp = document.getElementById('prodCreateName');
      return { modalOpen: document.getElementById('modalOverlay').style.display === 'flex', name: inp ? inp.value : '' };
    });
    await page.fill('#prodCreateName', 'QA_Завод_оружия');
    await page.click('#modalBtns button[data-primary]');
    await waitFor(() => { const p = state.producer_types.find(x => x.name === 'QA_Завод_оружия'); return !!p; }, 8000, 'factory created');
    const k15ok = k15a.lab6 && k15a.lab7 && k15a.lab8 && k15b.modalOpen && k15b.name.includes('оружие');
    report('K15 regression items + invite create', k15ok ? 'PASS' : 'FAIL',
      `labs=${JSON.stringify(k15a)} invite=${JSON.stringify(k15b)}`);

    // ============ K16: 0 pageerror / 0 тост-ошибок ============
    const toast = await page.evaluate(() => document.getElementById('report').textContent);
    const toastErr = /ошибк|Ошибк|невалид|500/i.test(toast);
    const k16 = pageErrors.length === 0 && !toastErr;
    report('K16 no page errors / no toast errors', k16 ? 'PASS' : 'FAIL',
      `pageErrors=${pageErrors.length} toast="${toast.replace(/\n/g, ' | ').slice(0, 120)}"`);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'studio-slots-check.png') });
    report('screenshot', 'PASS', 'studio-slots-check.png');

  } catch (err) {
    report('UNCAUGHT', 'FAIL', String(err && err.message ? err.message : err));
  }

  // ============ cleanup: вернуть к исходному (48 слотов, 25 типов) ============
  try {
    const st = (await api('GET', '/studio/api/state')).data;
    // удалить тестовые слоты (Фабрика, топливо=8) уровня F1 и universal-переопределения, слот cat=1
    const testSlots = st.producer_slots.filter(s => s.parent_id === 2 &&
      ((s.category_id === 8 && s.race_family) || (s.category_id === 1 && !s.race_family)));
    for (const s of testSlots) await api('DELETE', '/studio/api/slots/' + s.id);
    // вернуть базу топливо visible
    const base8 = st.producer_slots.find(s => s.parent_id === 2 && s.category_id === 8 && !s.race_family);
    if (base8 && base8.hidden) await api('PUT', '/studio/api/slots/' + base8.id, { hidden: false });
    // удалить тестовые записи QA_
    const qa = st.producer_types.filter(p => p.name.startsWith('QA_'));
    for (const p of qa) await api('DELETE', '/studio/api/producers/' + p.id);
    report('cleanup', 'PASS', `slots=${testSlots.length} qa=${qa.length}`);
  } catch (e) {
    report('cleanup', 'FAIL', String(e && e.message ? e.message : e));
  }

  const fails = results.filter(r => r.status === 'FAIL');
  console.log(`\nTOTAL: ${results.length} steps, ${fails.length} FAIL`);
  return finish(fails.length ? 1 : 0);
}

main();