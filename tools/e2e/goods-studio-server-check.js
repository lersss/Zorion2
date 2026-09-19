// tools/e2e/goods-studio-server-check.js
// Browser test for the Goods Studio UI on the game server (/studio, iterB):
// авторизация JWT (localStorage adminToken), «+ ресурс», кириллица,
// OR-фильтры справочника, попап ресурса (тир/статусы), удаление.
// Итерация C (спека iterC §11 п.9): шаг fill — создание товара с пустым
// слотом → POST fill → 202 → опрос state до generating=false → report непуст
// («Ошибка ИИ» — opencode в CI недоступен, детерминировано; если opencode
// локально запущен — proposals непуст, попап открывается, apply применяет
// принятое) → cleanup. Таймаут опроса ≥ OPENCODE_TIMEOUT_S + 15 c.
// Старая студия (8799) — отдельный смоук goods-studio-check.js (до C).
//
// Run: node goods-studio-server-check.js
//   BASE_URL env overrides default http://127.0.0.1:8080
//   STUDIO_USER / STUDIO_PASS — учётка admin/skycomposer (обязательны,
//   секреты в файл не хардкодим; bootstrap — env SKYCOMPOSER_BOOTSTRAP_*)
//   CHROME_PATH / EDGE_PATH — как в старом смоуке
// Requires: game server running on 8080 with seeded catalog.
// The test creates QA_* goods via the UI/API and deletes them at the end.
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

  // --- login: POST /login → token (Bearer для API-шагов, localStorage для браузера) ---
  const login = await api('POST', '/login', { username: STUDIO_USER, password: STUDIO_PASS }, false);
  if (login.status !== 200 || !login.data || !login.data.token) {
    report('login', 'FAIL', 'POST /login status=' + login.status);
    return finish(1);
  }
  token = login.data.token;
  report('login', 'PASS', 'role=' + (login.data.user && login.data.user.role));

  // --- preflight: state с Bearer → 200; без Bearer → 401 (проверка авторизации) ---
  const st = await api('GET', '/studio/api/state');
  if (st.status !== 200) {
    report('preflight /studio/api/state', 'FAIL', 'status=' + st.status);
    return finish(1);
  }
  const anon = await api('GET', '/studio/api/state', undefined, false);
  report('preflight auth', (st.status === 200 && anon.status === 401) ? 'PASS' : 'FAIL',
    `with token=${st.status} without=${anon.status}`);
  const resCatId = st.data.categories.find(c => c.kind === 'resource').id;
  const goodCatId = st.data.categories.find(c => c.kind === 'good').id;
  const resCount = st.data.goods.filter(g => g.kind === 'resource').length;
  report('preflight /studio/api/state', 'PASS', `goods=${st.data.goods.length} cats=${st.data.categories.length} resources=${resCount}`);

  // очистка зависших proposals от прошлого прогона (иначе попап «Предложения
  // ИИ» авто-откроется и перехватит клики в шаге 3): товар жив → cancel;
  // удалён → apply (М2: 200 «товар не найден» + сброс proposals)
  if ((st.data.proposals || []).length > 0) {
    const pid = st.data.proposals_good_id;
    const goodExists = st.data.goods.some(g => g.id === pid);
    if (goodExists) {
      await api('POST', `/studio/api/goods/${pid}/fill/cancel`);
    } else {
      const accepted = st.data.proposals.map((p, i) => {
        const item = { i };
        if (p.kind === 'new') item.category_id = p.category_valid ? p.category_id : goodCatId;
        return item;
      });
      await api('POST', `/studio/api/goods/${pid}/fill/apply`, { accepted });
    }
    report('preflight stale proposals', 'PASS', `cleared ${st.data.proposals.length} proposals for good ${pid}`);
  }

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
  // токен в localStorage ДО открытия /studio (спека iterB §6)
  await context.addInitScript((t) => localStorage.setItem('adminToken', t), token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.stack ? err.stack : err)));

  const waitFor = (fn, timeout = 8000, desc = 'condition', arg) =>
    page.waitForFunction(fn, arg, { timeout }).catch(() => { throw new Error('timeout waiting: ' + desc); });

  try {
    // ============ Step 1: загрузка после логина ============
    await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 10000, 'spinner removed');
    const loginGone = await page.evaluate(() => document.getElementById('studioLoginOverlay').style.display !== 'flex');
    report('1 studio loads after login', loginGone ? 'PASS' : 'FAIL', 'login overlay hidden, spinner removed');

    // ============ Step 2: «+ ресурс» (UI) ============
    await page.fill('#spravNewResName', 'QA_Ресурс');
    await page.selectOption('#spravNewResCat', String(resCatId));
    await page.click('#btnSpravAddRes');
    await waitFor((name) => state.goods.some(g => g.name === name), 8000, 'QA_Ресурс in state', 'QA_Ресурс');
    const resState = await page.evaluate(() => {
      const g = state.goods.find(x => x.name === 'QA_Ресурс');
      return g ? { kind: g.kind, recipe: (g.recipe || []).length, id: g.id } : null;
    });
    report('2 + resource via UI', (resState && resState.kind === 'resource' && resState.recipe === 0) ? 'PASS' : 'FAIL', JSON.stringify(resState));

    // ============ Step 3: кириллица — переименование через попап + дубликат 409 ============
    await page.evaluate((id) => { selected = id; openPopup(id); renderAll(); }, resState.id);
    await page.waitForTimeout(300);
    await page.click('#popupBody .dname .rename');
    await page.fill('#renameInput', 'QA_Ресурс_2');
    await page.keyboard.press('Enter');
    await waitFor((name) => state.goods.some(g => g.name === name), 8000, 'renamed in state', 'QA_Ресурс_2');
    const renamed = await page.evaluate(() => state.goods.some(g => g.name === 'QA_Ресурс_2'));
    // дубликат имени через UI → 409-тост (api() показывает e.error); ждём
    // именно текст «уже есть» — отчёт-тост мог быть занят прошлым fill-отчётом
    await page.fill('#spravNewResName', 'QA_Ресурс_2');
    await page.selectOption('#spravNewResCat', String(resCatId));
    await page.click('#btnSpravAddRes');
    await waitFor(() => document.getElementById('report').textContent.includes('уже есть'), 8000, 'dup 409 toast');
    const dupToast = await page.evaluate(() => document.getElementById('report').textContent);
    report('3 cyrillic rename + dup 409', (renamed && dupToast.includes('уже есть')) ? 'PASS' : 'FAIL',
      `renamed=${renamed} toast="${dupToast.replace(/\n/g, ' | ').slice(0, 80)}"`);

    // ============ Step 4: OR-фильтры ============
    await page.evaluate(() => { toggleAllTiers(); });
    await page.check('#fUnused');
    await page.waitForTimeout(300);
    const unusedList = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      return { count: cards.length, hasQA: cards.some(c => c.textContent.includes('QA_Ресурс_2')) };
    });
    // бан + галка «забаненные» → забаненный виден приглушённым
    await api('POST', `/studio/api/goods/${resState.id}/status`, { status: 'banned' });
    await waitFor((id) => { const g = state.goods.find(x => x.id === id); return g && g.status === 'banned'; }, 8000, 'banned in state', resState.id);
    await page.check('#fBanned');
    await page.waitForTimeout(300);
    const bannedList = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      const qa = cards.find(c => c.textContent.includes('QA_Ресурс_2'));
      return { hasQA: !!qa, dimmed: qa ? qa.classList.contains('banned') : false };
    });
    // «Вернуть» в попапе забаненного → draft
    await page.evaluate((id) => { selected = id; openPopup(id); renderAll(); }, resState.id);
    await page.waitForTimeout(200);
    await page.click('#popupBody .btns button:has-text("Вернуть")');
    await waitFor((id) => { const g = state.goods.find(x => x.id === id); return g && g.status === 'draft'; }, 8000, 'unbanned to draft', resState.id);
    report('4 OR-filters', (unusedList.hasQA && bannedList.hasQA && bannedList.dimmed) ? 'PASS' : 'FAIL',
      `unused=${unusedList.count} bannedDimmed=${bannedList.dimmed}`);
    await page.uncheck('#fBanned');
    await page.uncheck('#fUnused');
    await page.keyboard.press('Escape');

    // ============ Step 5: удаление ресурса (N=0 → модалка 1 → DELETE → тост) ============
    await page.evaluate((id) => { selected = id; openPopup(id); renderAll(); }, resState.id);
    await page.waitForTimeout(200);
    await page.click('#popupBody .btns button:has-text("удалить")');
    await waitFor(() => document.getElementById('modalOverlay').style.display === 'flex', 5000, 'modal 1');
    await page.click('#modalBtns button[data-primary]');
    await waitFor((id) => !state.goods.some(g => g.id === id), 8000, 'deleted from state', resState.id);
    const toast = await page.evaluate(() => document.getElementById('report').textContent);
    report('5 delete resource', (toast.includes('Удалено') && toast.includes('0')) ? 'PASS' : 'FAIL',
      `toast="${toast.replace(/\n/g, ' | ').slice(0, 120)}"`);

    // ============ Step 6: попап ресурса — тир-поле, статусы ============
    const res2 = (await api('POST', '/studio/api/goods', { name: 'QA_Ресурс_3', category_id: resCatId, kind: 'resource' })).data;
    await waitFor((id) => state.goods.some(g => g.id === id), 8000, 'QA_Ресурс_3 in state', res2.id);
    await page.evaluate((id) => { selected = id; openPopup(id); renderAll(); }, res2.id);
    await page.waitForTimeout(300);
    await page.fill('#tierInput', '3');
    await page.dispatchEvent('#tierInput', 'change');
    await waitFor((id) => { const g = state.goods.find(x => x.id === id); return g && g.tier_override === 3; }, 8000, 'tier override 3', res2.id);
    const tierState = await page.evaluate(() => {
      const g = state.goods.find(x => x.id === selected);
      const warn = document.querySelector('.tier-warn');
      return { tier: g.tier, override: g.tier_override, warn: warn ? warn.textContent : '' };
    });
    await page.click('#popupBody .btns button:has-text("Согласовать")');
    await waitFor((id) => { const g = state.goods.find(x => x.id === id); return g && g.status === 'approved'; }, 8000, 'approved', res2.id);
    report('6 resource popup tier+status', (tierState.tier === 3 && tierState.override === 3 && tierState.warn.includes('отличается')) ? 'PASS' : 'FAIL', JSON.stringify(tierState));
    await page.keyboard.press('Escape');

    // ============ Step 6b: «добавить родителя» (BUG-1 iterB: номер слота из
    // state, не из ответа POST slots — контракт iterA возвращает {"id": id}) ============
    const parentGood = (await api('POST', '/studio/api/goods', { name: 'QA_Родитель', category_id: goodCatId })).data;
    await waitFor((id) => state.goods.some(g => g.id === id), 8000, 'QA_Родитель in state', parentGood.id);
    await page.evaluate((id) => { selected = id; openPopup(id); renderAll(); }, res2.id);
    await page.waitForTimeout(300);
    await page.click('#popupBody .btns button:has-text("добавить родителя")');
    await waitFor(() => document.getElementById('modalOverlay').style.display === 'flex', 5000, 'add parent modal');
    await page.click('#modalBody .pline:has-text("QA_Родитель")');
    await waitFor((ids) => {
      const p = state.goods.find(x => x.id === ids.pid);
      return p && (p.recipe || []).some(s => s.good_id === ids.cid);
    }, 8000, 'parent slot filled', { pid: parentGood.id, cid: res2.id });
    const parentState = await page.evaluate((pid) => {
      const p = state.goods.find(x => x.id === pid);
      return { slots: (p.recipe || []).length, filled: (p.recipe || []).some(s => s.good_id) };
    }, parentGood.id);
    report('6b add parent fills slot', (parentState.slots >= 1 && parentState.filled) ? 'PASS' : 'FAIL', JSON.stringify(parentState));
    await page.keyboard.press('Escape');

    // ============ Step 7: no JS errors + screenshot ============
    report('7 no page JS errors', pageErrors.length === 0 ? 'PASS' : 'FAIL', pageErrors.length ? pageErrors.join(' | ').slice(0, 300) : 'clean');
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'goods-studio-server.png') });

    // ============ Step 8: fill (спека iterC §11 п.9) ============
    // Товар с пустым слотом → POST fill → 202 → опрос state до
    // generating=false → report непуст. Детерминировано в CI: opencode
    // недоступен → «Ошибка ИИ», proposals пусты; локально с opencode —
    // proposals непуст, apply применяет принятое.
    const fillGood = (await api('POST', '/studio/api/goods', { name: 'QA_Fill', category_id: goodCatId })).data;
    await waitFor((id) => state.goods.some(g => g.id === id), 8000, 'QA_Fill in state', fillGood.id);
    // явно добавить пустой слот перед fill (ревью iterC: не полагаемся на
    // слоты от CreateGood — fill без пустых слотов вернул бы 400)
    await api('POST', `/studio/api/goods/${fillGood.id}/slots`);
    await waitFor((id) => {
      const g = state.goods.find(x => x.id === id);
      return g && (g.recipe || []).some(s => !s.good_id);
    }, 8000, 'empty slot in state', fillGood.id);
    const fillResp = await api('POST', `/studio/api/goods/${fillGood.id}/fill`);
    report('8 fill start', fillResp.status === 202 ? 'PASS' : 'FAIL', 'status=' + fillResp.status);
    // опрос state до завершения fill (через API, не страницу: страница
    // опрашивает раз в 3 с и может не успеть увидеть generating=true —
    // waitFor по !generating резолвился бы сразу на устаревшем state, ревью
    // iterC). Завершён = generating=false И (report непуст ИЛИ proposals
    // непуст) — BuildProposals всегда даёт одно из двух. Таймаут ≥
    // OPENCODE_TIMEOUT_S + 15 c (fill может висеть до таймаута клиента).
    let fillState = null;
    const fillDeadline = Date.now() + 135000;
    while (Date.now() < fillDeadline) {
      const s = (await api('GET', '/studio/api/state')).data;
      if (!s.generating && ((s.report || []).length > 0 || (s.proposals || []).length > 0)) {
        fillState = s;
        break;
      }
      await new Promise(r => setTimeout(r, 2000));
    }
    if (!fillState) {
      report('8 fill negative', 'FAIL', 'fill не завершился за 135 c (таймаут опроса)');
    } else {
      const hasError = fillState.report.some(l => l.includes('Ошибка ИИ'));
      if (fillState.proposals.length > 0) {
        // opencode локально: proposals непуст, попап открылся (авто-открытие
        // в fetchState), применяем принятое (kind=new — категория из
        // селекта/дефолта)
        const accepted = fillState.proposals.map((p, i) => {
          const item = { i };
          if (p.kind === 'new') item.category_id = p.category_valid ? p.category_id : goodCatId;
          return item;
        });
        const applyResp = await api('POST', `/studio/api/goods/${fillGood.id}/fill/apply`, { accepted });
        report('8 fill apply', applyResp.status === 200 ? 'PASS' : 'FAIL',
          `applied=${applyResp.data && applyResp.data.applied} proposals=${fillState.proposals.length}`);
      } else {
        report('8 fill negative', hasError ? 'PASS' : 'FAIL',
          `generating=${fillState.generating} report="${fillState.report.join(' | ').slice(0, 120)}"`);
      }
    }

  } catch (err) {
    report('UNCAUGHT', 'FAIL', String(err && err.message ? err.message : err));
  }

  // ============ cleanup: delete QA_ goods ============
  try {
    const st2 = (await api('GET', '/studio/api/state')).data;
    const qa = st2.goods.filter(g => g.name.startsWith('QA_'));
    for (const g of qa) await api('DELETE', `/studio/api/goods/${g.id}`);
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