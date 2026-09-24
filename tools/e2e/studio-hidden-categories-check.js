// tools/e2e/studio-hidden-categories-check.js
// Browser test: студия — флаг «скрытая» у категорий строений (спека
// 2026-09-21-студия-скрытые-категории-строений), ветка «Производители».
// Проверяет: галку #prodShowHidden, три состояния узла (вариант б),
// попап-чекбокс «скрытая», фильтр списка справа, localStorage/F5,
// отсутствие сетевых запросов при смене галки, 0 pageerror/тост-ошибок.
//
// Run: node studio-hidden-categories-check.js
//   BASE_URL env overrides default http://127.0.0.1:8080
//   STUDIO_USER / STUDIO_PASS — учётка admin/skycomposer
// Requires: game server running on 8080, миграция 000052 применена.
// Данные для кейсов готовятся через API: id=40 «Фабрика топлива»
// (universal, hidden=false), id=43 «Фабрика топлива F1» (F1, hidden=true).
// Console output is ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic).
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
    report('setup creds', 'FAIL', 'STUDIO_USER/STUDIO_PASS env not set (admin/skycomposer account)');
    return finish(1);
  }

  const login = await api('POST', '/login', { username: STUDIO_USER, password: STUDIO_PASS }, false);
  if (login.status !== 200 || !login.data || !login.data.token) {
    report('login', 'FAIL', 'POST /login status=' + login.status);
    return finish(1);
  }
  token = login.data.token;
  report('login', 'PASS', 'role=' + (login.data.user && login.data.user.role));

  // --- подготовка данных через API (идемпотентно) ---
  const st0 = (await api('GET', '/studio/api/state')).data;
  let p40 = st0.producer_types.find(p => p.name === 'Фабрика топлива' && !p.race_family);
  let p43 = st0.producer_types.find(p => p.name === 'Фабрика топлива F1');
  if (!p40) {
    const r = await api('POST', '/studio/api/producers', { name: 'Фабрика топлива', kind: 'goods', category_id: 8, parent_id: 2, hidden: false });
    p40 = r.data;
  } else if (p40.hidden) {
    await api('PUT', '/studio/api/producers/' + p40.id, { hidden: false });
  }
  if (!p43) {
    const r = await api('POST', '/studio/api/producers', { name: 'Фабрика топлива F1', kind: 'goods', category_id: 8, parent_id: 2, race_family: 'F1', hidden: true });
    p43 = r.data;
  } else if (!p43.hidden) {
    await api('PUT', '/studio/api/producers/' + p43.id, { hidden: true });
  }
  const ID40 = p40.id, ID43 = p43.id;
  const NODE40 = 'sub:' + ID40, NODE43 = 'sub:' + ID43;
  report('setup data', 'PASS', `p40=${ID40} hidden=${p40.hidden} | p43=${ID43} hidden=${p43.hidden}`);

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

  try {
    // ============ загрузка студии + ветка «Производители» ============
    await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 10000, 'spinner removed');
    await page.click('#branchProdFactory'); // раздел «Фабрики»: скрипт работает с типом 2 (спека 2026-09-25)
    await waitFor(() => document.getElementById('segProdRace').style.display !== 'none', 5000, 'producers branch active');
    await waitFor(() => state.producer_types.length > 0, 8000, 'producer_types loaded');

    // ============ K7: галка в шапке, выключена, title ============
    const k7 = await page.evaluate(() => {
      const chk = document.getElementById('prodShowHidden');
      const seg = document.getElementById('segProdRace');
      return {
        exists: !!chk,
        checked: chk ? chk.checked : null,
        title: chk ? (chk.title || '') : '',
        nearRaceSeg: !!seg && !!chk && seg.parentElement === chk.parentElement,
      };
    });
    report('K7 checkbox in header', (k7.exists && k7.checked === false && k7.title.length > 0) ? 'PASS' : 'FAIL', JSON.stringify(k7));

    // ============ K8: вариант б — скрытая F1 прячет категорию на уровне F1 ============
    // universal: карточка ${NODE40} видна, ${NODE43} (F1) не применяется
    const treeUniversal = await page.evaluate(({ n40, n43 }) => {
      const L = prodTreeLayout();
      return { has40: !!L.pos[n40], has43: !!L.pos[n43], hasInvite: !!L.pos['invite:2:8'] };
    }, { n40: NODE40, n43: NODE43 });
    // family F1: категория топливо скрыта — ни карточки, ни приглашения
    await page.evaluate(() => { setProdRaceLevel('family'); setProdRaceFamily('F1'); renderAll(); });
    await page.waitForTimeout(300);
    const treeF1 = await page.evaluate(({ n40, n43 }) => {
      const L = prodTreeLayout();
      return { has40: !!L.pos[n40], has43: !!L.pos[n43], hasInvite: !!L.pos['invite:2:8'] };
    }, { n40: NODE40, n43: NODE43 });
    // обратно на universal: карточка снова видна
    await page.evaluate(() => { setProdRaceLevel('universal'); renderAll(); });
    await page.waitForTimeout(300);
    const treeUniversal2 = await page.evaluate(({ n40, n43 }) => {
      const L = prodTreeLayout();
      return { has40: !!L.pos[n40], has43: !!L.pos[n43], hasInvite: !!L.pos['invite:2:8'] };
    }, { n40: NODE40, n43: NODE43 });
    const k8 = (treeUniversal.has40 && !treeUniversal.has43 && !treeUniversal.hasInvite) &&
      (!treeF1.has40 && !treeF1.has43 && !treeF1.hasInvite) &&
      (treeUniversal2.has40 && !treeUniversal2.has43);
    report('K8 variant-b hide on F1', k8 ? 'PASS' : 'FAIL',
      `universal=${JSON.stringify(treeUniversal)} f1=${JSON.stringify(treeF1)} universal2=${JSON.stringify(treeUniversal2)}`);

    // ============ K9: галка на уровне F1 → только затемнённая скрытая ============
    await page.evaluate(() => { setProdRaceLevel('family'); setProdRaceFamily('F1'); renderAll(); });
    await page.check('#prodShowHidden');
    await page.waitForTimeout(300);
    const k9 = await page.evaluate(({ n40, n43 }) => {
      const L = prodTreeLayout();
      const n43n = L.root && findNode(L.root, n43);
      return {
        has43: !!L.pos[n43],
        has40: !!L.pos[n40],
        hasInvite: !!L.pos['invite:2:8'],
        n43hidden: n43n ? !!n43n.hidden : null,
      };
      function findNode(n, id) {
        if (n.id === id) return n;
        for (const c of n.children || []) { const r = findNode(c, id); if (r) return r; }
        return null;
      }
    }, { n40: NODE40, n43: NODE43 });
    report('K9 checkbox on F1 shows only hidden', (k9.has43 && k9.n43hidden === true && !k9.has40 && !k9.hasInvite) ? 'PASS' : 'FAIL', JSON.stringify(k9));

    // ============ K10: попап скрытой записи, чекбокс отмечен, снять → PUT → обычная ============
    await page.evaluate((id) => openProdNodePopup(id), NODE43);
    await page.waitForTimeout(300);
    const k10a = await page.evaluate(() => {
      const chk = document.getElementById('prodHiddenChk');
      return { popupOpen: document.getElementById('detailPopup').style.display === 'flex', chkExists: !!chk, chkChecked: chk ? chk.checked : null };
    });
    // снять чекбокс → saveProdHidden → PUT
    await page.uncheck('#prodHiddenChk');
    await waitFor((id) => { const p = state.producer_types.find(x => x.id === id); return p && p.hidden === false; }, 8000, 'p43 hidden=false after uncheck', ID43);
    const k10b = await page.evaluate(({ id43, n43node }) => {
      const p = state.producer_types.find(x => x.id === id43);
      const L = prodTreeLayout();
      const n43 = L.root && (function find(n){ if (n.id===n43node) return n; for (const c of n.children||[]) { const r=find(c); if (r) return r; } return null; })(L.root);
      return { hidden: p.hidden, nodeHidden: n43 ? !!n43.hidden : null, inTree: !!L.pos[n43node] };
    }, { id43: ID43, n43node: NODE43 });
    report('K10 popup + uncheck', (k10a.popupOpen && k10a.chkExists && k10a.chkChecked === true && k10b.hidden === false && k10b.nodeHidden === false && k10b.inTree) ? 'PASS' : 'FAIL',
      `popup=${JSON.stringify(k10a)} after=${JSON.stringify(k10b)}`);
    await page.keyboard.press('Escape');
    // вернуть hidden:true для K12
    await api('PUT', '/studio/api/producers/' + ID43, { hidden: true });
    await waitFor((id) => { const p = state.producer_types.find(x => x.id === id); return p && p.hidden === true; }, 8000, 'p43 hidden=true restored', ID43);

    // ============ K11: в попапе ТИПА и подтипа items чекбокса НЕТ ============
    await page.evaluate(() => openProdNodePopup('type:2')); // Фабрика (тип)
    await page.waitForTimeout(300);
    const k11a = await page.evaluate(() => ({ chkExists: !!document.getElementById('prodHiddenChk') }));
    await page.keyboard.press('Escape');
    await page.evaluate(() => openProdNodePopup('sub:7')); // Лаборатория космических технологий (items)
    await page.waitForTimeout(300);
    const k11b = await page.evaluate(() => ({ chkExists: !!document.getElementById('prodHiddenChk') }));
    await page.keyboard.press('Escape');
    report('K11 no checkbox for type/items', (!k11a.chkExists && !k11b.chkExists) ? 'PASS' : 'FAIL', `type=${JSON.stringify(k11a)} items=${JSON.stringify(k11b)}`);

    // ============ K12: список справа — фильтр видимости+скрытости ============
    // галка выключена (K9 её включала) — проверяем «без галки» честно
    await page.uncheck('#prodShowHidden');
    await page.waitForTimeout(200);
    // universal: F1-запись (43) не показывается; 40 видна
    await page.evaluate(() => { setProdRaceLevel('universal'); renderAll(); });
    await page.waitForTimeout(300);
    const spravUni = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      return { has40: cards.some(c => c.textContent.includes('Фабрика топлива') && !c.textContent.includes('F1')), has43: cards.some(c => c.textContent.includes('F1')) };
    });
    // family F1 без галки: скрытая 43 не видна (и 40 тоже — категория скрыта)
    await page.evaluate(() => { setProdRaceLevel('family'); setProdRaceFamily('F1'); renderAll(); });
    await page.waitForTimeout(300);
    const spravF1Off = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      return { has43: cards.some(c => c.textContent.includes('F1')), has40: cards.some(c => c.textContent.includes('Фабрика топлива') && !c.textContent.includes('F1')) };
    });
    // family F1 с галкой: 43 видна с пометкой «скрыта»
    await page.check('#prodShowHidden');
    await page.waitForTimeout(300);
    const spravF1On = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      const c43 = cards.find(c => c.textContent.includes('F1'));
      return { has43: !!c43, badge: c43 ? c43.textContent.includes('скрыта') : false, has40: cards.some(c => c.textContent.includes('Фабрика топлива') && !c.textContent.includes('F1')) };
    });
    // клик по записи списка → выбор существующего узла дерева
    await page.evaluate(() => { const el = [...document.querySelectorAll('#spravList .card')].find(c => c.textContent.includes('F1')); if (el) el.click(); });
    await page.waitForTimeout(300);
    const spravClick = await page.evaluate(() => {
      const L = prodTreeLayout();
      return { selected: selected, nodeInTree: !!L.pos[selected] };
    });
    const k12 = spravUni.has40 && !spravUni.has43 && !spravF1Off.has43 && !spravF1Off.has40 &&
      spravF1On.has43 && spravF1On.badge && !spravF1On.has40 && spravClick.selected === NODE43 && spravClick.nodeInTree;
    report('K12 sprav filter', k12 ? 'PASS' : 'FAIL',
      `uni=${JSON.stringify(spravUni)} f1off=${JSON.stringify(spravF1Off)} f1on=${JSON.stringify(spravF1On)} click=${JSON.stringify(spravClick)}`);

    // ============ K13: F5 переживает галку; смена галки не трогает zoom/pan и сервер ============
    // галка сейчас включена (K12) — проверим localStorage и F5
    const lsBefore = await page.evaluate(() => localStorage.getItem('gs_prodShowHidden'));
    await page.reload({ waitUntil: 'domcontentloaded' });
    await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 10000, 'spinner after reload');
    await page.click('#branchProdFactory'); // раздел «Фабрики»: скрипт работает с типом 2 (спека 2026-09-25)
    await waitFor(() => document.getElementById('segProdRace').style.display !== 'none', 5000, 'branch after reload');
    const k13a = await page.evaluate(() => ({ checked: document.getElementById('prodShowHidden').checked, ls: localStorage.getItem('gs_prodShowHidden') }));
    // смена галки: zoom/pan не сбрасываются, сервер не дёргается
    const apiBefore = apiRequests.length;
    await page.evaluate(() => { zoom = 2.5; panX = 123; panY = -45; });
    await page.uncheck('#prodShowHidden');
    await page.waitForTimeout(300);
    const k13b = await page.evaluate(() => ({ zoom: zoom, panX: panX, panY: panY }));
    await page.check('#prodShowHidden');
    await page.waitForTimeout(300);
    const apiAfter = apiRequests.length;
    const k13 = (k13a.checked === true && k13a.ls === '1') &&
      (k13b.zoom === 2.5 && k13b.panX === 123 && k13b.panY === -45) &&
      (apiAfter === apiBefore);
    report('K13 F5 + zoom/pan + no server calls', k13 ? 'PASS' : 'FAIL',
      `afterF5=${JSON.stringify(k13a)} zoompan=${JSON.stringify(k13b)} apiCalls=${apiAfter - apiBefore}`);

    // ============ K14: 0 pageerror / 0 тост-ошибок ============
    const toast = await page.evaluate(() => document.getElementById('report').textContent);
    const toastErr = /ошибк|Ошибк|невалид|400|409|500/i.test(toast);
    const k14 = pageErrors.length === 0 && !toastErr;
    report('K14 no page errors / no toast errors', k14 ? 'PASS' : 'FAIL',
      `pageErrors=${pageErrors.length} toast="${toast.replace(/\n/g, ' | ').slice(0, 120)}"`);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'studio-hidden-categories.png') });
    report('screenshot', 'PASS', 'studio-hidden-categories.png');

  } catch (err) {
    report('UNCAUGHT', 'FAIL', String(err && err.message ? err.message : err));
  }

  // ============ cleanup: удалить созданные тестовые записи ============
  try {
    let deleted = 0;
    for (const id of [ID40, ID43]) {
      if (id) { const r = await api('DELETE', '/studio/api/producers/' + id); if (r.status === 200) deleted++; }
    }
    report('cleanup', 'PASS', `deleted ${deleted} test producer_types`);
  } catch (e) {
    report('cleanup', 'FAIL', String(e && e.message ? e.message : e));
  }

  const fails = results.filter(r => r.status === 'FAIL');
  console.log(`\nTOTAL: ${results.length} steps, ${fails.length} FAIL`);
  return finish(fails.length ? 1 : 0);
}

main();