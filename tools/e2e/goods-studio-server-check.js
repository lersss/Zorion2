// tools/e2e/goods-studio-server-check.js
// Browser test for the Goods Studio UI on the game server (/studio, iterB):
// авторизация JWT (localStorage adminToken), «+ ресурс», кириллица,
// OR-фильтры справочника, попап ресурса (поля сложности/тира нет), удаление.
// Модель рецептов (спека 2026-09-21-рецепт-сущность §5): состав адресуется
// recipe-роутами (/studio/api/recipes/{id}/components), сложность — свойство
// рецепта (PUT /studio/api/recipes/{id}); шаг 9 — copy-universal
// (POST /studio/api/producers/{id}/recipes/copy-universal).
// Итерация C (спека iterC §11 п.9): шаг fill — создание товара с пустым
// компонентом → POST fill → 202 → опрос state до generating=false → report
// непуст («Ошибка ИИ» — opencode в CI недоступен, детерминировано; если
// opencode локально запущен — proposals непуст, попап открывается, apply
// применяет принятое) → cleanup. Таймаут опроса ≥ OPENCODE_TIMEOUT_S + 15 c.
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
  const goodCatName = st.data.categories.find(c => c.kind === 'good').name;
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
    // форма «+ ресурс» живёт в попапе (ТЗ §7.6): кнопка в шапке → ввод → создать
    await page.click('#btnNewResOpen');
    await page.waitForSelector('#spravNewResName');
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
    // именно текст «уже есть» — отчёт-тост мог быть занят прошлым fill-отчётом.
    // Форма — в попапе (ТЗ §7.6): открываем заново, т.к. после успеха шага 2
    // окно закрылось. При ошибке 409 окно остаётся открытым (ТЗ §7.3).
    await page.click('#btnNewResOpen');
    await page.waitForSelector('#spravNewResName');
    await page.fill('#spravNewResName', 'QA_Ресурс_2');
    await page.selectOption('#spravNewResCat', String(resCatId));
    await page.click('#btnSpravAddRes');
    await waitFor(() => document.getElementById('report').textContent.includes('уже есть'), 8000, 'dup 409 toast');
    const dupToast = await page.evaluate(() => document.getElementById('report').textContent);
    report('3 cyrillic rename + dup 409', (renamed && dupToast.includes('уже есть')) ? 'PASS' : 'FAIL',
      `renamed=${renamed} toast="${dupToast.replace(/\n/g, ' | ').slice(0, 80)}"`);
    // ошибка оставляет попап открытым — закрываем его, overlay перехватил бы
    // клики следующих шагов по панели справа
    await page.keyboard.press('Escape');

    // ============ Step 4: OR-фильтры + обратимость скрытия ============
    await page.evaluate(() => { toggleAllTiers(); });
    await page.check('#fUnused');
    await page.waitForTimeout(300);
    const unusedList = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      return { count: cards.length, hasQA: cards.some(c => c.textContent.includes('QA_Ресурс_2')) };
    });
    await page.uncheck('#fUnused');
    // у товаров/ресурсов статуса и скрытия нет (спека 2026-09-21 §1.2) —
    // обратимость проверяем на записи-производителе: hidden=true → hidden=false
    const prodResp = await api('POST', '/studio/api/producers', { name: 'QA_Фабрика', kind: 'goods' });
    const prodId = prodResp.data && prodResp.data.id;
    const hiddenOnResp = await api('POST', `/studio/api/producers/${prodId}/hidden`, { hidden: true });
    const onState = ((await api('GET', '/studio/api/state')).data.producer_types || []).find(p => p.id === prodId);
    const hiddenOffResp = await api('POST', `/studio/api/producers/${prodId}/hidden`, { hidden: false });
    const offState = ((await api('GET', '/studio/api/state')).data.producer_types || []).find(p => p.id === prodId);
    await api('DELETE', `/studio/api/producers/${prodId}`);
    const hiddenOK = hiddenOnResp.status === 200 && onState && onState.hidden === true &&
      hiddenOffResp.status === 200 && offState && offState.hidden === false;
    report('4 OR-filters + producer hidden reversible', (unusedList.hasQA && hiddenOK) ? 'PASS' : 'FAIL',
      `unused=${unusedList.count} hidden=${onState && onState.hidden}->${offState && offState.hidden}`);

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

    // ============ Step 6: попап ресурса — поля сложности/тира нет ============
    // У ресурса рецепта нет (kind=resource), поэтому нет ни поля сложности,
    // ни тир-оверрайда; tier производный = 0 (спека §6.2).
    const res2 = (await api('POST', '/studio/api/goods', { name: 'QA_Ресурс_3', category_id: resCatId, kind: 'resource' })).data;
    await waitFor((id) => state.goods.some(g => g.id === id), 8000, 'QA_Ресурс_3 in state', res2.id);
    await page.evaluate((id) => { selected = id; openPopup(id); renderAll(); }, res2.id);
    await page.waitForTimeout(300);
    const resPopup = await page.evaluate(() => {
      const g = state.goods.find(x => x.id === selected);
      return {
        tier: g.tier, recipeId: g.recipe_id,
        hasComplexity: !!document.getElementById('complexityInput'),
        hasTierInput: !!document.getElementById('tierInput'),
      };
    });
    report('6 resource popup no complexity/tier field',
      (!resPopup.hasComplexity && !resPopup.hasTierInput && resPopup.tier === 0) ? 'PASS' : 'FAIL',
      JSON.stringify(resPopup));
    await page.keyboard.press('Escape');

    // ============ Step 6b: попап товара — сложность рецепта + состав через
    // recipe-роуты (спека §5: рецепт создаётся вместе с товаром; состав —
    // POST /studio/api/recipes/{id}/components) ============
    const parentGood = (await api('POST', '/studio/api/goods', { name: 'QA_Родитель', category_id: goodCatId })).data;
    await waitFor((id) => state.goods.some(g => g.id === id), 8000, 'QA_Родитель in state', parentGood.id);
    const parentRecipeId = await page.evaluate((id) => (state.goods.find(g => g.id === id) || {}).recipe_id, parentGood.id);
    report('6b good created with recipe', parentRecipeId ? 'PASS' : 'FAIL', 'recipe_id=' + parentRecipeId);
    // сложность рецепта через попап (PUT /studio/api/recipes/{id})
    await page.evaluate((id) => { selected = id; openPopup(id); renderAll(); }, parentGood.id);
    await page.waitForTimeout(300);
    await page.fill('#complexityInput', '5');
    await page.dispatchEvent('#complexityInput', 'change');
    await waitFor((id) => { const g = state.goods.find(x => x.id === id); return g && g.complexity === 5; }, 8000, 'complexity 5', parentGood.id);
    const cxState = await page.evaluate((id) => {
      const g = state.goods.find(x => x.id === id);
      const warn = document.querySelector('.tier-warn');
      return { complexity: g.complexity, tier: g.tier, warn: warn ? warn.textContent : '' };
    }, parentGood.id);
    report('6b complexity field', (cxState.complexity === 5 && cxState.tier === 5) ? 'PASS' : 'FAIL', JSON.stringify(cxState));
    // состав — пустой компонент через recipe-роут (замена снятого /goods/{id}/slots)
    const addComp = await api('POST', `/studio/api/recipes/${parentRecipeId}/components`);
    await waitFor((id) => {
      const g = state.goods.find(x => x.id === id);
      return g && (g.recipe || []).length >= 1 && (g.recipe || []).some(s => !s.good_id);
    }, 8000, 'empty component in state', parentGood.id);
    const compState = await page.evaluate((id) => {
      const g = state.goods.find(x => x.id === id);
      return { comps: (g.recipe || []).length, empty: (g.recipe || []).some(s => !s.good_id) };
    }, parentGood.id);
    report('6b recipe component route', (addComp.status === 200 && compState.comps >= 1 && compState.empty) ? 'PASS' : 'FAIL', JSON.stringify(compState));
    await page.keyboard.press('Escape');

    // ============ Step 6c: описание каталога (спека 2026-09-21-каталог-описание §11.3) ============
    // Без ИИ: создание QA_-записи → PUT description → state содержит текст →
    // PUT "" → пусто → удаление; bulk с тремя колонками → описание у записи.
    const descGood = (await api('POST', '/studio/api/goods', { name: 'QA_Описание', category_id: goodCatId })).data;
    await api('PUT', `/studio/api/goods/${descGood.id}`, { description: 'QA-описание каталога' });
    const descState = ((await api('GET', '/studio/api/state')).data.goods || []).find(g => g.id === descGood.id);
    await api('PUT', `/studio/api/goods/${descGood.id}`, { description: '' });
    const descCleared = ((await api('GET', '/studio/api/state')).data.goods || []).find(g => g.id === descGood.id);
    await api('DELETE', `/studio/api/goods/${descGood.id}`);
    const bulkResp = await api('POST', '/studio/api/goods/bulk', { lines: ['QA_Описание_балк | ' + goodCatName + ' | QA-описание из bulk'] });
    const bulkCreated = (bulkResp.data && bulkResp.data.created && bulkResp.data.created[0]) || null;
    const bulkState = bulkCreated ? ((await api('GET', '/studio/api/state')).data.goods || []).find(g => g.id === bulkCreated.id) : null;
    if (bulkCreated) await api('DELETE', `/studio/api/goods/${bulkCreated.id}`);
    const descOK = descState && descState.description === 'QA-описание каталога' &&
      descCleared && descCleared.description === '' &&
      bulkState && bulkState.description === 'QA-описание из bulk';
    report('6c catalog description (PUT + bulk 3 columns)', descOK ? 'PASS' : 'FAIL',
      `set="${descState && descState.description}" cleared="${descCleared && descCleared.description}" bulk="${bulkState && bulkState.description}"`);

    // ============ Step 7: no JS errors + screenshot ============
    report('7 no page JS errors', pageErrors.length === 0 ? 'PASS' : 'FAIL', pageErrors.length ? pageErrors.join(' | ').slice(0, 300) : 'clean');
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'goods-studio-server.png') });

    // ============ Step 8: fill (спека iterC §11 п.9) ============
    // Товар с пустым компонентом → POST fill → 202 → опрос state до
    // generating=false → report непуст. Детерминировано в CI: opencode
    // недоступен → «Ошибка ИИ», proposals пусты; локально с opencode —
    // proposals непуст, apply применяет принятое.
    const fillGood = (await api('POST', '/studio/api/goods', { name: 'QA_Fill', category_id: goodCatId })).data;
    await waitFor((id) => state.goods.some(g => g.id === id), 8000, 'QA_Fill in state', fillGood.id);
    // явно добавить пустой компонент перед fill (ревью iterC: не полагаемся на
    // состав от CreateGood — fill без пустых компонентов вернул бы 400);
    // recipe-роут вместо снятого POST /goods/{id}/slots (спека рецептов §5)
    const fillRecipeId = await page.evaluate((id) => (state.goods.find(g => g.id === id) || {}).recipe_id, fillGood.id);
    await api('POST', `/studio/api/recipes/${fillRecipeId}/components`);
    await waitFor((id) => {
      const g = state.goods.find(x => x.id === id);
      return g && (g.recipe || []).some(s => !s.good_id);
    }, 8000, 'empty component in state', fillGood.id);
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

    // ============ Step 9: copy-universal (спека рецептов §5, ТЗ §13) ============
    // Цель — конкретная фабрика kind=goods товарной категории; источник —
    // универсальные конкретные фабрики той же категории (parent_id NOT NULL,
    // race_family IS NULL, race IS NULL), кроме цели. Цель создаём семейной,
    // чтобы (parent, category, family) не конфликтовал с универсальным источником.
    const live = (await api('GET', '/studio/api/state')).data;
    const races = (await api('GET', '/studio/api/races')).data;
    const allFamilies = (races && races.families ? races.families : []).map(f => f.id);
    const catKind = (cid) => { const c = live.categories.find(x => x.id === cid); return c ? c.kind : null; };
    const hasRec = (pid) => live.producer_recipes.some(pr => pr.producer_type_id === pid);
    const freeFamily = (parentId, categoryId) =>
      allFamilies.find(f => !live.producer_types.some(p =>
        p.parent_id === parentId && p.category_id === categoryId && p.race_family === f && !p.race));
    const makeTarget = async (parentId, categoryId, tag) => {
      const fam = freeFamily(parentId, categoryId);
      if (!fam) return { id: null, err: 'no free family' };
      const t = await api('POST', '/studio/api/producers',
        { name: 'QA_Копия_' + tag + '_' + Date.now(), kind: 'goods', category_id: categoryId, parent_id: parentId, race_family: fam });
      return { id: t.data && t.data.id, err: t.status + ':' + JSON.stringify(t.data) };
    };

    // 9a: есть универсальный источник с рецептами → added>0; повтор → added:0/skipped>0
    const uniSrc = live.producer_types.find(p =>
      p.parent_id && p.kind === 'goods' && catKind(p.category_id) === 'good' && !p.race_family && !p.race && hasRec(p.id));
    let copyFirst = [], copyAgain = [];
    if (uniSrc) {
      const t = await makeTarget(uniSrc.parent_id, uniSrc.category_id, 'add');
      if (t.id) {
        const r1 = await api('POST', `/studio/api/producers/${t.id}/recipes/copy-universal`);
        const r2 = await api('POST', `/studio/api/producers/${t.id}/recipes/copy-universal`);
        copyFirst = [r1.status, r1.data && r1.data.added, r1.data && r1.data.skipped];
        copyAgain = [r2.status, r2.data && r2.data.added, r2.data && r2.data.skipped];
      } else {
        copyFirst = ['target-fail', t.err];
      }
    }
    report('9a copy-universal added>0 + idempotent',
      (copyFirst[0] === 200 && copyFirst[1] > 0 && copyAgain[0] === 200 && copyAgain[1] === 0 && copyAgain[2] > 0) ? 'PASS' : 'FAIL',
      `src=${uniSrc && uniSrc.id} first=${JSON.stringify(copyFirst)} again=${JSON.stringify(copyAgain)}`);

    // 9b: цель без универсального источника → 200 {added:0, skipped:0} (не ошибка)
    const uniNoRec = live.producer_types.find(p =>
      p.parent_id && p.kind === 'goods' && catKind(p.category_id) === 'good' && !p.race_family && !p.race && !hasRec(p.id));
    let emptyRes = null;
    if (uniNoRec) {
      const t = await makeTarget(uniNoRec.parent_id, uniNoRec.category_id, 'empty');
      if (t.id) {
        const r = await api('POST', `/studio/api/producers/${t.id}/recipes/copy-universal`);
        emptyRes = [r.status, r.data && r.data.added, r.data && r.data.skipped];
      } else {
        emptyRes = ['target-fail', t.err];
      }
    }
    report('9b copy-universal empty source -> 0/0',
      (emptyRes && emptyRes[0] === 200 && emptyRes[1] === 0 && emptyRes[2] === 0) ? 'PASS' : 'FAIL',
      `src=${uniNoRec && uniNoRec.id} resp=${JSON.stringify(emptyRes)}`);

    // 9c: 400 — не конкретная фабрика (тип без родителя) и ресурсная категория
    const nonConcrete = live.producer_types.find(p => p.kind === 'goods' && !p.parent_id);
    const resCatFac = live.producer_types.find(p => p.kind === 'goods' && p.parent_id && p.category_id && catKind(p.category_id) === 'resource');
    const rNon = nonConcrete ? await api('POST', `/studio/api/producers/${nonConcrete.id}/recipes/copy-universal`) : null;
    const rRes = resCatFac ? await api('POST', `/studio/api/producers/${resCatFac.id}/recipes/copy-universal`) : null;
    report('9c copy-universal 400 (non-concrete / resource cat)',
      (rNon && rNon.status === 400 && rRes && rRes.status === 400) ? 'PASS' : 'FAIL',
      `non=${rNon && rNon.status}(${nonConcrete && nonConcrete.id}) res=${rRes && rRes.status}(${resCatFac && resCatFac.id})`);

    // ============ Step 10: локальный ИИ-помощник — индикатор и ручки (спека
    // 2026-09-24-студия-управление-локальным-ии §8, E1–E5) ============
    // E1: в шапке есть #aiStatusText, текст — один из допустимых (начинается с «ИИ:»).
    const aiText = await page.evaluate(() => {
      const el = document.getElementById('aiStatusText');
      return el ? el.textContent.trim() : null;
    });
    const e1ok = !!aiText && aiText.indexOf('ИИ:') === 0;
    report('10a E1 AI indicator present', e1ok ? 'PASS' : 'FAIL', 'text="' + aiText + '"');

    // E2: GET /studio/api/ai/status → 200 и state из множества.
    const aiSt = await api('GET', '/studio/api/ai/status');
    const aiStates = ['stopped', 'starting', 'running', 'foreign'];
    const e2ok = aiSt.status === 200 && aiSt.data && aiStates.indexOf(aiSt.data.state) >= 0;
    report('10b E2 AI status endpoint', e2ok ? 'PASS' : 'FAIL',
      `status=${aiSt.status} state=${aiSt.data && aiSt.data.state} managed=${aiSt.data && aiSt.data.managed}`);

    // E3: при managed=false/foreign кнопка «запустить» выключена, а ручка
    // честно отвечает 409 человеческим текстом; страница не падает (процесс
    // не создаётся — клик по выключенной кнопке невозможен).
    const errBefore = pageErrors.length;
    if (aiSt.data && (aiSt.data.managed === false || aiSt.data.state === 'foreign')) {
      const btnDisabled = await page.evaluate(() => document.getElementById('btnAiStart').disabled);
      const raw = await fetch(BASE_URL + '/studio/api/ai/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
      });
      const rawBody = await raw.json().catch(() => null);
      const human = raw.status === 409 && rawBody && typeof rawBody.error === 'string' && rawBody.error.length > 0;
      report('10c E3 AI start guarded',
        (btnDisabled && human && pageErrors.length === errBefore) ? 'PASS' : 'FAIL',
        `btnDisabled=${btnDisabled} api=${raw.status} err="${rawBody && rawBody.error}"`);
    } else {
      report('10c E3 AI start guarded', 'SKIP', 'managed=true, state=' + (aiSt.data && aiSt.data.state));
    }

    // E4: индикатор обновляется без перезагрузки — после fetchState текст
    // строки совпадает с серверным detail (провод state → #aiStatusText).
    const e4 = await page.evaluate(async () => {
      await fetchState();
      const txt = document.getElementById('aiStatusText').textContent;
      return { txt, detail: (state.ai && state.ai.detail) || '' };
    });
    report('10d E4 AI state live', (e4.txt === e4.detail && e4.txt.length > 0) ? 'PASS' : 'FAIL',
      `txt="${e4.txt}"`);

    // E5: во время активного ИИ-прогона «остановить» выключена с подсказкой.
    const e5 = await page.evaluate(() => {
      const saved = {g: state.generating, d: state.desc_generating};
      state.generating = true; updateAIStatus();
      const stop = document.getElementById('btnAiStop');
      const res = {disabled: stop.disabled, title: stop.title};
      state.generating = saved.g; state.desc_generating = saved.d; updateAIStatus();
      return res;
    });
    report('10e E5 AI stop disabled during run',
      (e5.disabled && e5.title.indexOf('прогон') >= 0) ? 'PASS' : 'FAIL', JSON.stringify(e5));

  } catch (err) {
    report('UNCAUGHT', 'FAIL', String(err && err.message ? err.message : err));
  }

  // ============ cleanup: delete QA_ goods + QA_ producers ============
  try {
    const st2 = (await api('GET', '/studio/api/state')).data;
    const qaGoods = st2.goods.filter(g => g.name.startsWith('QA_'));
    for (const g of qaGoods) await api('DELETE', `/studio/api/goods/${g.id}`);
    const qaProds = st2.producer_types.filter(p => p.name.startsWith('QA_'));
    for (const p of qaProds) await api('DELETE', `/studio/api/producers/${p.id}`);
    report('cleanup', 'PASS', `deleted ${qaGoods.length} QA_ goods, ${qaProds.length} QA_ producers`);
  } catch (e) {
    report('cleanup', 'FAIL', String(e && e.message ? e.message : e));
  }

  const fails = results.filter(r => r.status === 'FAIL');
  const skips = results.filter(r => r.status === 'SKIP');
  console.log(`\nTOTAL: ${results.length} steps, ${fails.length} FAIL, ${skips.length} SKIP`);
  return finish(fails.length ? 1 : 0);
}

main();