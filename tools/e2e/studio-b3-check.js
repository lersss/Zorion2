// tools/e2e/studio-b3-check.js — перепроверка фикса B3 (ререндер после мутаций слотов)
// Пункты: 1) галка на унаследованном → «переопределён» сразу; 2) «−» → «унаследован» сразу;
// 3) «+ слот» → строка сразу; 4) дерево/список синхронно; 5) попап не мигает при авто-опросе;
// 6) K11 обратный кейс (хинт «база скрыта»).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const STUDIO_USER = process.env.STUDIO_USER || '';
const STUDIO_PASS = process.env.STUDIO_PASS || '';
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

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
  const r = await fetch(BASE_URL + url, { method, headers, body: body ? JSON.stringify(body) : undefined });
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
  if (!STUDIO_USER || !STUDIO_PASS) { report('setup creds', 'FAIL', 'env not set'); return finish(1); }

  const login = await api('POST', '/login', { username: STUDIO_USER, password: STUDIO_PASS }, false);
  if (login.status !== 200 || !login.data || !login.data.token) { report('login', 'FAIL', 'status=' + login.status); return finish(1); }
  token = login.data.token;
  report('login', 'PASS', 'role=' + (login.data.user && login.data.user.role));

  // подготовка: убрать тестовые слоты (Фабрика, топливо=8) F4/F1 и universal-переопределения, слот cat=1
  const st0 = (await api('GET', '/studio/api/state')).data;
  const testSlots = st0.producer_slots.filter(s => s.parent_id === 2 &&
    ((s.category_id === 8 && s.race_family) || (s.category_id === 1)));
  for (const s of testSlots) await api('DELETE', '/studio/api/slots/' + s.id);
  const base8 = st0.producer_slots.find(s => s.parent_id === 2 && s.category_id === 8 && !s.race_family);
  if (base8 && base8.hidden) await api('PUT', '/studio/api/slots/' + base8.id, { hidden: false });
  // универсальная запись cat=8 для проверки списка (С4: база-слот существует)
  let qaTop = st0.producer_types.find(p => p.name === 'QA_Топливо_универс');
  if (!qaTop) qaTop = (await api('POST', '/studio/api/producers', { name: 'QA_Топливо_универс', kind: 'goods', category_id: 8, parent_id: 2 })).data;
  report('setup data', 'PASS', `base8=${base8 && base8.id} qaTop=${qaTop.id}`);

  const exe = findExecutable();
  if (!exe) { report('setup browser', 'FAIL', 'no Chrome/Edge'); return finish(1); }
  try { browser = await chromium.launch({ executablePath: exe.path, headless: true }); }
  catch (err) { report('setup browser', 'FAIL', String(err && err.message ? err.message : err)); return finish(1); }
  const context = await browser.newContext({ viewport: { width: 1400, height: 900 } });
  await context.addInitScript((t) => localStorage.setItem('adminToken', t), token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.stack ? err.stack : err)));
  const waitFor = (fn, timeout = 8000, desc = 'condition', arg) =>
    page.waitForFunction(fn, arg, { timeout }).catch(() => { throw new Error('timeout waiting: ' + desc); });

  const rowText = () => page.evaluate(() => {
    const rows = [...document.querySelectorAll('#popupBody .pslot')];
    const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
    return top ? top.textContent.replace(/\s+/g, ' ').trim() : 'NOT FOUND';
  });
  const rowChk = () => page.evaluate(() => {
    const rows = [...document.querySelectorAll('#popupBody .pslot')];
    const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
    return top ? top.querySelector('input[type=checkbox]').checked : null;
  });
  const clickRowChk = () => page.evaluate(() => {
    const rows = [...document.querySelectorAll('#popupBody .pslot')];
    const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
    if (top) top.querySelector('input[type=checkbox]').click();
  });
  const clickRowMinus = () => page.evaluate(() => {
    const rows = [...document.querySelectorAll('#popupBody .pslot')];
    const top = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'топливо');
    if (top) { const b = top.querySelector('button'); if (b) b.click(); }
  });

  try {
    await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 10000, 'spinner');
    await page.click('#branchProdFactory'); // раздел «Фабрики»: скрипт работает с типом 2 (спека 2026-09-25)
    await waitFor(() => state.producer_slots.length > 0, 8000, 'slots loaded');

    // ============ 1. Галка на унаследованном (F4, база visible) → «переопределён» сразу ============
    await page.evaluate(() => { setProdRaceLevel('family'); setProdRaceFamily('F4'); renderAll(); });
    await page.waitForTimeout(300);
    await page.evaluate(() => openProdNodePopup('type:2'));
    await page.waitForTimeout(300);
    const p1a = { text: await rowText(), chk: await rowChk() };
    await clickRowChk(); // создать переопределение F4 hidden
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 8 && x.race_family === 'F4'); return s && s.hidden === true; }, 8000, 'F4 slot hidden created');
    await page.waitForTimeout(300);
    const p1b = { text: await rowText(), chk: await rowChk() };
    // повторный клик → PUT (свой слот) → hidden=false
    await clickRowChk();
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 8 && x.race_family === 'F4'); return s && s.hidden === false; }, 8000, 'F4 slot hidden=false via PUT');
    await page.waitForTimeout(300);
    const p1c = { text: await rowText(), chk: await rowChk() };
    const p1ok = p1a.text.includes('унаследован') && p1a.chk === false &&
      p1b.text.includes('переопределён') && p1b.chk === true &&
      p1c.text.includes('переопределён') && p1c.chk === false;
    report('B3-1 checkbox inherited → override + PUT', p1ok ? 'PASS' : 'FAIL', `before=${JSON.stringify(p1a)} afterPOST=${JSON.stringify(p1b)} afterPUT=${JSON.stringify(p1c)}`);

    // ============ 2. «− слот» переопределения → «унаследован» сразу ============
    await clickRowMinus();
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 8 && x.race_family === 'F4'); return !s; }, 8000, 'F4 override removed');
    await page.waitForTimeout(300);
    const p2 = { text: await rowText(), chk: await rowChk() };
    const p2ok = p2.text.includes('унаследован') && p2.chk === false;
    report('B3-2 minus → inherited immediately', p2ok ? 'PASS' : 'FAIL', JSON.stringify(p2));

    // ============ 3. «+ слот категории» → строка появляется сразу ============
    await page.evaluate(() => {
      const sel = document.getElementById('prodNewSlotCat');
      if (sel) sel.value = '1'; // Минералы (resource, вне набора)
    });
    await page.click('#popupBody button:has-text("+ слот")');
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 1 && x.race_family === 'F4'); return !!s; }, 8000, 'F4 slot cat=1 created');
    await page.waitForTimeout(300);
    const p3 = await page.evaluate(() => {
      const rows = [...document.querySelectorAll('#popupBody .pslot')];
      const min = rows.find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent.trim() === 'Минералы');
      return { found: !!min, text: min ? min.textContent.replace(/\s+/g, ' ').trim() : '' };
    });
    const p3ok = p3.found && p3.text.includes('переопределён');
    report('B3-3 plus slot appears immediately', p3ok ? 'PASS' : 'FAIL', JSON.stringify(p3));
    await page.keyboard.press('Escape');

    // ============ 4. Дерево и список синхронно (скрытая категория исчезает/затемняется) ============
    // скрыть базу топливо на universal (через редактор) → дерево: invite:2:8 исчезает; список: .card.hidden
    await page.evaluate(() => { setProdRaceLevel('universal'); renderAll(); });
    await page.waitForTimeout(300);
    await page.evaluate(() => openProdNodePopup('type:2'));
    await page.waitForTimeout(300);
    await clickRowChk(); // скрыть базу топливо
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 8 && !x.race_family); return s && s.hidden === true; }, 8000, 'base8 hidden');
    await page.waitForTimeout(300);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    const p4tree = await page.evaluate(() => { const L = prodTreeLayout(); return { invite: !!L.pos['invite:2:8'] }; });
    // список: без галки запись не видна; с галкой — .card.hidden
    await page.uncheck('#prodShowHidden');
    await page.waitForTimeout(300);
    const p4off = await page.evaluate(() => [...document.querySelectorAll('#spravList .card')].some(c => c.textContent.includes('QA_Топливо_универс')));
    await page.check('#prodShowHidden');
    await page.waitForTimeout(300);
    const p4on = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      const top = cards.find(c => c.textContent.includes('QA_Топливо_универс'));
      return { found: !!top, hiddenCls: top ? top.classList.contains('hidden') : false };
    });
    const p4ok = !p4tree.invite && !p4off && p4on.found && p4on.hiddenCls;
    report('B3-4 tree+sprav sync', p4ok ? 'PASS' : 'FAIL', `tree=${JSON.stringify(p4tree)} off=${p4off} on=${JSON.stringify(p4on)}`);

    // ============ 5. Попап не закрывается и не мигает при авто-опросе (3с) ============
    await page.evaluate(() => openProdNodePopup('type:2'));
    await page.waitForTimeout(300);
    const p5a = { text: await rowText(), popupOpen: await page.evaluate(() => document.getElementById('detailPopup').style.display === 'flex') };
    await page.waitForTimeout(3500); // авто-опрос fetchState (3с)
    const p5b = { text: await rowText(), popupOpen: await page.evaluate(() => document.getElementById('detailPopup').style.display === 'flex') };
    const p5ok = p5a.popupOpen && p5b.popupOpen && p5a.text === p5b.text;
    report('B3-5 popup stable on autopoll', p5ok ? 'PASS' : 'FAIL', `before=${JSON.stringify(p5a)} after3.5s=${JSON.stringify(p5b)}`);
    await page.keyboard.press('Escape');

    // ============ 6. K11: база скрыта + переопределение visible на F4 → хинт «база скрыта» ============
    // база топливо уже скрыта (п.4). На F4 создать переопределение visible (снять галку у унаследованного)
    await page.evaluate(() => { setProdRaceLevel('family'); setProdRaceFamily('F4'); renderAll(); });
    await page.waitForTimeout(300);
    await page.evaluate(() => openProdNodePopup('type:2'));
    await page.waitForTimeout(300);
    const k11a = { chk: await rowChk() };
    await clickRowChk(); // снять галку → создать F4-переопределение visible
    await waitFor(() => { const s = state.producer_slots.find(x => x.parent_id === 2 && x.category_id === 8 && x.race_family === 'F4'); return s && s.hidden === false; }, 8000, 'F4 override visible');
    await page.waitForTimeout(300);
    const k11b = { text: await rowText() };
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    const k11tree = await page.evaluate((subId) => { const L = prodTreeLayout(); return { inviteF4: !!L.pos['invite:2:8'], cardF4: !!L.pos[subId] }; }, 'sub:' + qaTop.id);
    await page.evaluate(() => { setProdRaceLevel('universal'); renderAll(); });
    await page.uncheck('#prodShowHidden'); // галка была включена с B3-4 — скрытая категория без галки не рисуется
    await page.waitForTimeout(300);
    const k11uni = await page.evaluate((subId) => { const L = prodTreeLayout(); return { inviteUni: !!L.pos['invite:2:8'], cardUni: !!L.pos[subId] }; }, 'sub:' + qaTop.id);
    const k11ok = k11a.chk === true && k11b.text.includes('переопределён') && k11b.text.includes('база скрыта') &&
      (k11tree.inviteF4 || k11tree.cardF4) && !k11uni.inviteUni && !k11uni.cardUni;
    report('B3-6 K11 reverse case + hint', k11ok ? 'PASS' : 'FAIL', `chk=${k11a.chk} after=${JSON.stringify(k11b)} f4=${JSON.stringify(k11tree)} uni=${JSON.stringify(k11uni)}`);

    // консоль
    const toast = await page.evaluate(() => document.getElementById('report').textContent);
    const toastErr = /ошибк|Ошибк|невалид|500/i.test(toast);
    report('console clean', (pageErrors.length === 0 && !toastErr) ? 'PASS' : 'FAIL', `pageErrors=${pageErrors.length} toast="${toast.slice(0, 80)}"`);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'studio-b3-check.png') });
    report('screenshot', 'PASS', 'studio-b3-check.png');
  } catch (err) {
    report('UNCAUGHT', 'FAIL', String(err && err.message ? err.message : err));
  }

  // ============ cleanup: вернуть к исходному (48 слотов, 25 типов) ============
  try {
    const st = (await api('GET', '/studio/api/state')).data;
    const testSlots = st.producer_slots.filter(s => s.parent_id === 2 &&
      ((s.category_id === 8 && s.race_family) || (s.category_id === 1)));
    for (const s of testSlots) await api('DELETE', '/studio/api/slots/' + s.id);
    const base8 = st.producer_slots.find(s => s.parent_id === 2 && s.category_id === 8 && !s.race_family);
    if (base8 && base8.hidden) await api('PUT', '/studio/api/slots/' + base8.id, { hidden: false });
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