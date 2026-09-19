// tools/e2e/goods-studio-check.js
// Browser test for the Goods Studio UI — REWRITTEN for 99a.3 (2026-09-19):
// фокус-подграф, справочник (#spravList), попап-деталь (#detailPopup),
// тир-оверрайд (#tierInput), подгрузка списка (#bulkText), «всё дерево»
// (#fullTree). Старые селекторы (#detail/#resList/#unusedList/#bannedPanel)
// удалены — их UI больше нет (99a.3 §8.1/§10.3).
//
// Run: node goods-studio-check.js   (BASE_URL env overrides default 127.0.0.1:8799)
// Requires: goods studio running (go run cmd/goods-studio/main.go), Chrome/Edge.
// The test creates QA_* goods via the UI/API and deletes them at the end.
// Console output is ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, rmSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8799').replace(/\/+$/, '');
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

async function api(method, url, body) {
  const r = await fetch(BASE_URL + url, {
    method,
    headers: { 'Content-Type': 'application/json' },
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

  // --- preflight: server up, state sane ---
  let pre = null;
  try { pre = await api('GET', '/api/state'); } catch (e) {}
  if (!pre || pre.status !== 200) {
    report('preflight /api/state', 'FAIL', 'server not reachable');
    return finish(1);
  }
  const resIds = pre.data.goods.filter(g => g.kind === 'resource').map(g => g.id);
  const catId = pre.data.categories[0] ? pre.data.categories[0].id : null;
  const catId2 = pre.data.categories[1] ? pre.data.categories[1].id : null;
  report('preflight /api/state', 'PASS', `goods=${pre.data.goods.length} cats=${pre.data.categories.length} resources=${resIds.length}`);

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
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.stack ? err.stack : err)));

  const waitFor = (fn, timeout = 8000, desc = 'condition', arg) =>
    page.waitForFunction(fn, arg, { timeout }).catch(() => { throw new Error('timeout waiting: ' + desc); });

  try {
    // ============ Step 1: loading overlay (failure path) ============
    await page.route('**/api/state', (route) => route.abort());
    await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitFor(() => document.getElementById('loadingOverlay').style.display === 'flex', 8000, 'loading overlay on failure');
    report('1 loading overlay (failure path)', 'PASS', 'spinner shows when first /api/state fails');
    await page.unroute('**/api/state');
    await page.reload({ waitUntil: 'domcontentloaded' });
    await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 10000, 'overlay hides after reload');
    report('1b overlay hides after reload', 'PASS', 'fresh load with working network clears the overlay');

    // ============ Step 2: empty registry overlay (route-injected state) ============
    await page.route('**/api/state', (route) => route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ categories: [], goods: [], banned: [], unused: [], warnings: [], model: '', generating: false, report: [], auto_refresh_ms: 3000 }),
    }));
    await page.reload({ waitUntil: 'domcontentloaded' });
    await waitFor(() => document.getElementById('emptyOverlay').style.display === 'flex', 8000, 'empty overlay');
    const emptyCta = await page.evaluate(() => {
      const b = document.querySelector('#emptyOverlay button');
      return b && b.textContent.includes('+ товар');
    });
    report('2 empty registry + CTA', emptyCta ? 'PASS' : 'FAIL', 'overlay + CTA button present');
    await page.unroute('**/api/state');
    await page.reload({ waitUntil: 'domcontentloaded' });
    await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 10000, 'reload done');

    // ============ Step 3: focus mode default + empty overlay + badge ============
    const focusEmpty = await page.evaluate(() => document.getElementById('focusEmptyOverlay').style.display === 'flex');
    const badge = await page.evaluate(() => document.getElementById('modeBadge').textContent);
    report('3 focus default + empty overlay', (focusEmpty && badge.includes('фокус')) ? 'PASS' : 'FAIL', `overlay=${focusEmpty} badge="${badge}"`);

    // ============ Step 4: topbar hidden in focus mode ============
    const hidden = await page.evaluate(() => ({
      search: document.getElementById('search').style.display === 'none',
      cat: document.getElementById('catFilter').style.display === 'none',
      res: document.getElementById('showResources').parentElement.style.display === 'none',
      banned: document.getElementById('showBanned').parentElement.style.display === 'none',
      rev: document.getElementById('reverseEdges').parentElement.style.display === 'none',
      top: document.getElementById('btnTop').style.display === 'none',
    }));
    report('4 topbar hidden in focus', Object.values(hidden).every(Boolean) ? 'PASS' : 'FAIL', JSON.stringify(hidden));

    // ============ Step 5: sprav — default empty, filter turns it on ============
    // Новая семантика (создатель 2026-09-19): ничего не выбрано в фильтрах → список пуст
    const spravEmpty = await page.evaluate(() => ({
      title: document.getElementById('spravTitle').textContent,
      cards: document.querySelectorAll('#spravList .card').length,
      note: document.querySelector('#spravList .empty-note') ? document.querySelector('#spravList .empty-note').textContent : '',
    }));
    report('5a sprav default empty', (spravEmpty.cards === 0 && spravEmpty.title.includes('· 0') && spravEmpty.note.includes('ничего не выбрано')) ? 'PASS' : 'FAIL', JSON.stringify(spravEmpty));
    await page.evaluate(() => toggleAllTiers()); // «все»-пилюля: выбрать все тиры
    await page.check('#fUsed'); // показ требует «тир + галка» (создатель 2026-09-19) — включаем «используемые»
    await page.waitForTimeout(200);
    const spravOn = await page.evaluate(() => ({
      title: document.getElementById('spravTitle').textContent,
      cards: document.querySelectorAll('#spravList .card').length,
    }));
    report('5b sprav fills on filter', (spravOn.cards > 0 && spravOn.title.includes('· ' + spravOn.cards)) ? 'PASS' : 'FAIL', `title="${spravOn.title}" cards=${spravOn.cards}`);

    // ============ Step 6: sprav click selects + subgraph, popup NOT opened ============
    const firstId = await page.evaluate(() => {
      const card = document.querySelector('#spravList .card');
      return card ? card.getAttribute('ondblclick').match(/'([^']+)'/)[1] : null;
    });
    await page.click('#spravList .card >> nth=0');
    await page.waitForTimeout(400);
    const afterClick = await page.evaluate(() => ({
      selected: selected,
      popupOpen: document.getElementById('detailPopup').style.display === 'flex',
      focusEmptyGone: document.getElementById('focusEmptyOverlay').style.display === 'none',
      graphCards: (() => { const L = computeLayout(focusSubset(), selected); return L.goods.length; })(),
    }));
    report('6 sprav click selects + subgraph', (afterClick.selected === firstId && !afterClick.popupOpen && afterClick.focusEmptyGone && afterClick.graphCards > 0) ? 'PASS' : 'FAIL', JSON.stringify(afterClick));

    // ============ Step 7: info (ⓘ) opens popup WITHOUT selection change ============
    const selBefore = await page.evaluate(() => selected);
    await page.locator('#spravList .card').nth(1).locator('.info').click();
    await page.waitForTimeout(300);
    const popupState = await page.evaluate(() => ({
      open: document.getElementById('detailPopup').style.display === 'flex',
      popupGoodId: popupGoodId,
      selected: selected,
    }));
    report('7 info opens popup w/o selection change', (popupState.open && popupState.popupGoodId !== selBefore && popupState.selected === selBefore) ? 'PASS' : 'FAIL', JSON.stringify(popupState));

    // ============ Step 8: Esc closes popup, selection kept ============
    await page.keyboard.press('Escape');
    await page.waitForTimeout(200);
    const afterEsc = await page.evaluate(() => ({
      popupClosed: document.getElementById('detailPopup').style.display === 'none',
      selected: selected,
    }));
    report('8 Esc closes popup, selection kept', (afterEsc.popupClosed && afterEsc.selected === selBefore) ? 'PASS' : 'FAIL', JSON.stringify(afterEsc));

    // ============ Step 9: double-click on sprav card = selection + popup ============
    await page.locator('#spravList .card').nth(2).dblclick();
    await page.waitForTimeout(300);
    const dbl = await page.evaluate(() => ({
      open: document.getElementById('detailPopup').style.display === 'flex',
      popupGoodId: popupGoodId,
      selected: selected,
    }));
    report('9 dblclick sprav = select + popup', (dbl.open && dbl.popupGoodId === dbl.selected) ? 'PASS' : 'FAIL', JSON.stringify(dbl));
    await page.keyboard.press('Escape');

    // ============ Step 10: tier override in popup (input 7 -> warn, clear -> reset) ============
    // create a QA good with a known computed tier (leaf = 0), open popup, set tier 7
    const tierGood = (await api('POST', '/api/goods', { name: 'QA_Тир', category_id: catId })).data;
    await waitFor((gid) => state.goods.some(g => g.id === gid), 8000, 'QA_Тир in state', tierGood.id);
    await page.evaluate((id) => { selected = id; openPopup(id); renderAll(); }, tierGood.id);
    await page.waitForTimeout(300);
    const tierBefore = await page.evaluate(() => {
      const g = state.goods.find(x => x.id === selected);
      return { tier: g.tier, computed: g.tier_computed, override: g.tier_override };
    });
    await page.fill('#tierInput', '7');
    await page.dispatchEvent('#tierInput', 'change');
    await waitFor((gid) => {
      const g = state.goods.find(x => x.id === gid);
      return g && g.tier_override === 7;
    }, 8000, 'tier override 7 persisted', tierGood.id);
    const tierSet = await page.evaluate(() => {
      const g = state.goods.find(x => x.id === selected);
      return { tier: g.tier, override: g.tier_override, warn: !!document.querySelector('.tier-warn'), warnText: document.querySelector('.tier-warn') ? document.querySelector('.tier-warn').textContent : '' };
    });
    report('10a tier input 7 -> override', (tierSet.tier === 7 && tierSet.override === 7 && tierSet.warn && tierSet.warnText.includes('отличается')) ? 'PASS' : 'FAIL', JSON.stringify(tierSet));
    // clear -> reset to computed
    await page.fill('#tierInput', '');
    await page.dispatchEvent('#tierInput', 'change');
    await waitFor((gid) => {
      const g = state.goods.find(x => x.id === gid);
      return g && g.tier_override === null;
    }, 8000, 'tier override cleared', tierGood.id);
    const tierCleared = await page.evaluate(() => {
      const g = state.goods.find(x => x.id === selected);
      return { tier: g.tier, override: g.tier_override };
    });
    report('10b tier clear -> computed', (tierCleared.override === null && tierCleared.tier === tierBefore.tier) ? 'PASS' : 'FAIL', JSON.stringify(tierCleared));
    await page.keyboard.press('Escape');

    // ============ Step 11: bulk load (textarea -> create, toast report with line numbers) ============
    await page.click('#bulkToggle');
    await page.waitForTimeout(200);
    const bulkOpen = await page.evaluate(() => document.getElementById('bulkBlock').style.display !== 'none');
    await page.fill('#bulkText', 'QA_Балк1 | ' + (await api('GET', '/api/state')).data.categories.find(c => c.id === catId).name + '\n\nQA_Балк2 | НетТакойКатегории\nQA_Балк1 | ' + (await api('GET', '/api/state')).data.categories.find(c => c.id === catId).name);
    await page.click('#btnBulk');
    await waitFor(() => document.getElementById('report').style.display === 'block', 8000, 'bulk toast');
    const bulkToast = await page.evaluate(() => document.getElementById('report').textContent);
    const bulkState = (await api('GET', '/api/state')).data;
    const bulkCreated = bulkState.goods.filter(g => g.name === 'QA_Балк1');
    const bulkOk = bulkOpen && bulkToast.includes('Создано 1') && bulkToast.includes('строка 3') && bulkToast.includes('категория не найдена') && bulkToast.includes('строка 4') && bulkToast.includes('уже есть') && bulkCreated.length === 1 && bulkCreated[0].status === 'draft' && bulkCreated[0].recipe.length === 1 && bulkCreated[0].recipe[0].quantity === 1;
    report('11 bulk create + report', bulkOk ? 'PASS' : 'FAIL', `toast="${bulkToast.replace(/\n/g, ' | ').slice(0, 200)}" created=${bulkCreated.length}`);
    const bulkTextCleared = await page.evaluate(() => document.getElementById('bulkText').value === '');
    report('11b bulk textarea cleared', bulkTextCleared ? 'PASS' : 'FAIL', 'textarea emptied after create');

    // ============ Step 12: drag&drop sprav card into popup slot ============
    // QA_Балк1 has 1 empty slot; open its popup, drag a visible sprav card into the slot
    const bulkGood = bulkCreated[0];
    await page.evaluate((id) => { selected = id; openPopup(id); renderAll(); }, bulkGood.id);
    await page.waitForTimeout(300);
    const slotCount = await page.evaluate(() => document.querySelectorAll('#popupBody .slot').length);
    const resName = (await api('GET', '/api/state')).data.goods.find(g => g.id === resIds[0]).name;
    // показ требует «тир + галка» (создатель 2026-09-19); «вода» (resIds[0]) не используется
    // ни в одном рецепте → включаем «неиспользуемые» (сняв «используемые» от шага 5b), тиры «все» уже выбраны
    await page.uncheck('#fUsed');
    await page.check('#fUnused');
    await page.waitForTimeout(200);
    // программный drag (DataTransfer) — устойчивее мышиного dragTo: авто-обновление
    // перерисовывает список, мышиный drag теряет элемент; поиск карточки регистронезависимый
    const dragSent = await page.evaluate(({ rid }) => {
      const card = [...document.querySelectorAll('#spravList .card')].find(c => c.textContent.toLowerCase().includes(rid.toLowerCase()));
      const slot = document.querySelector('#popupBody .slot');
      if (!card || !slot) return false;
      const dt = new DataTransfer();
      card.dispatchEvent(new DragEvent('dragstart', { dataTransfer: dt, bubbles: true }));
      slot.dispatchEvent(new DragEvent('dragover', { dataTransfer: dt, bubbles: true, cancelable: true }));
      slot.dispatchEvent(new DragEvent('drop', { dataTransfer: dt, bubbles: true, cancelable: true }));
      return true;
    }, { rid: resName });
    if (!dragSent) report('12 drag sprav -> popup slot', 'FAIL', 'source card or slot not found in DOM');
    await waitFor((gid) => {
      const g = state.goods.find(x => x.id === gid);
      return g && g.recipe && g.recipe[0] && g.recipe[0].good_id;
    }, 8000, 'slot filled by drag', bulkGood.id);
    const dragOk = await page.evaluate(() => {
      const s = document.querySelector('#popupBody .slot .sname');
      return s && s.textContent.length > 0;
    });
    report('12 drag sprav -> popup slot', (dragSent && slotCount === 1 && dragOk) ? 'PASS' : 'FAIL', `slots=${slotCount} filled=${dragOk}`);
    // вернуть фильтры к состоянию шага 5b (тиры «все» + «используемые»)
    await page.uncheck('#fUnused');
    await page.check('#fUsed');
    await page.keyboard.press('Escape'); // close popup so canvas clicks are not intercepted

    // ============ Step 13: focus subgraph — click parent/child on canvas rebuilds ============
    // QA_Родитель (parent) with QA_Балк1 in its slot; select QA_Балк1 -> subgraph has parent;
    // click parent card on canvas -> selected = parent, subgraph rebuilds around it
    const parentGood = (await api('POST', '/api/goods', { name: 'QA_Родитель', category_id: catId })).data;
    await api('PUT', `/api/goods/${parentGood.id}/slots/0`, { good_id: bulkGood.id });
    await waitFor((ids) => ids.every(id => state.goods.some(g => g.id === id)), 8000, 'parent in state', [parentGood.id]);
    await page.evaluate((id) => { selected = id; renderAll(); }, bulkGood.id);
    await page.waitForTimeout(300);
    const subgraph = await page.evaluate(({ cid, pid }) => {
      const L = computeLayout(focusSubset(), selected);
      const ids = L.goods.map(g => g.id);
      return { hasChild: ids.includes(cid), hasParent: ids.includes(pid), count: ids.length };
    }, { cid: bulkGood.id, pid: parentGood.id });
    report('13a subgraph = selected + parent', (subgraph.hasChild && subgraph.hasParent) ? 'PASS' : 'FAIL', JSON.stringify(subgraph));
    // click parent card on canvas: сначала центрируем вид на родителе (устраняет
    // флак позиции от авто-центрирований прошлых кликов по справочнику), затем реальный клик
    const parentPos = await page.evaluate((pid) => {
      const L = computeLayout(focusSubset(), selected);
      const p = L.pos[pid];
      const rect = document.getElementById('graph').getBoundingClientRect();
      if (!p) return null;
      panX = rect.width / 2 - (p.x + 95) * zoom;
      panY = rect.height / 2 - (p.y + 43) * zoom;
      renderAll();
      return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
    }, parentGood.id);
    if (parentPos) {
      await page.mouse.click(parentPos.x, parentPos.y);
      await page.waitForTimeout(300);
      const afterParentClick = await page.evaluate(({ cid, pid }) => {
        const L = computeLayout(focusSubset(), selected);
        const ids = L.goods.map(g => g.id);
        return { selected: selected, hasChild: ids.includes(cid), hasParent: ids.includes(pid), popupOpen: document.getElementById('detailPopup').style.display === 'flex' };
      }, { cid: bulkGood.id, pid: parentGood.id });
      report('13b click parent on canvas = rebuild', (afterParentClick.selected === parentGood.id && afterParentClick.hasChild && afterParentClick.hasParent && !afterParentClick.popupOpen) ? 'PASS' : 'FAIL', JSON.stringify(afterParentClick));
    } else {
      report('13b click parent on canvas = rebuild', 'SKIP', 'parent card position not found');
    }

    // ============ Step 14: full tree mode ============
    await page.check('#fullTree');
    await page.waitForTimeout(300);
    const fullTree = await page.evaluate(() => ({
      badge: document.getElementById('modeBadge').textContent,
      search: document.getElementById('search').style.display !== 'none',
      res: document.getElementById('showResources').parentElement.style.display !== 'none',
      top: document.getElementById('btnTop').style.display !== 'none',
      graphCards: (() => { const L = computeLayout(); return L.goods.length; })(),
    }));
    report('14 full tree mode', (fullTree.badge.includes('всё дерево') && fullTree.search && fullTree.res && fullTree.top && fullTree.graphCards > 0) ? 'PASS' : 'FAIL', JSON.stringify(fullTree));
    await page.uncheck('#fullTree');
    await page.waitForTimeout(200);

    // ============ Step 15: no JS errors ============
    report('15 no page JS errors', pageErrors.length === 0 ? 'PASS' : 'FAIL', pageErrors.length ? pageErrors.join(' | ').slice(0, 300) : 'clean');

    // screenshots
    await page.evaluate((id) => { selected = id; renderAll(); }, bulkGood.id);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'goods-studio-99a3.png') });

  } catch (err) {
    report('UNCAUGHT', 'FAIL', String(err && err.message ? err.message : err));
  }

  // ============ cleanup: delete QA_ goods ============
  try {
    const st = (await api('GET', '/api/state')).data;
    const qa = st.goods.filter(g => g.name.startsWith('QA_'));
    for (const g of qa) await api('DELETE', `/api/goods/${g.id}`);
    report('cleanup', 'PASS', `deleted ${qa.length} QA_ goods`);
  } catch (e) {
    report('cleanup', 'FAIL', String(e && e.message ? e.message : e));
  }

  const fails = results.filter(r => r.status === 'FAIL');
  const skips = results.filter(r => r.status === 'SKIP');
  console.log(`\nTOTAL: ${results.length} steps, ${fails.length} FAIL, ${skips.length} SKIP`);
  return finish(fails.length ? 1 : 0);
}

main();