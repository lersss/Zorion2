// tools/e2e/qa-i23-settlement-view.js
//
// E2E приёмки витрины поселения И2.3 (спека
// 2026-09-23-стадии-поселения-и-скорость-производства §8.2/§11.3):
//   2) API-контракт (админ): у поселения объект stage (enter/exit/next_enter),
//      у веток поля rate/take;
//   3) карточка планеты «Usmo» (админ) → вкладка «Поселения»: строка «Стадия:»,
//      блок веток, без pageerror/toast-error/редиректа;
//   4) фикстура (Аутпост, население 500, ветка рецепта 69) → блок «Арифметика»,
//      «скорость (число пары)», «забираем»;
//   5) игрок (presence): арифметика видна, входного буфера нет;
//   6) масштаб gs_unitScale: меняется ТОЛЬКО «скорость (число пары)».
//
// Фикстура — tools/e2e/fixtures/i23-stage-view-{setup,resolve}.sql.
// Chrome/Edge — системные (playwright-core). Вывод ASCII.
// Run: node qa-i23-settlement-view.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const E2E_DIR = path.dirname(fileURLToPath(import.meta.url));
const ARTIFACTS_DIR = path.join(E2E_DIR, 'artifacts');
const FIXTURES_DIR = path.join(E2E_DIR, 'fixtures');
const SETUP_SQL = path.join(FIXTURES_DIR, 'i23-stage-view-setup.sql');
const RESOLVE_SQL = path.join(FIXTURES_DIR, 'i23-stage-view-resolve.sql');
const FIX_SETTLEMENT = 'e0000000-0000-4000-8000-0000000000a1';
const USMO_SETTLEMENT = 'f6d7a337-cdb5-4392-8bef-43e67e553fa5';
const USMO_WORLD = '6431e653-5d20-4ca1-8abc-8c5e06f86598';
const ADMIN_PREFIX = 'e2e_i23a_';
const PLAYER_PREFIX = 'e2e_i23p_';
const PASSWORD = 'e2e-i23-' + Date.now();
const PSQL = process.env.PSQL || 'C:\\pgsql\\pgsql\\bin\\psql.exe';

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

const results = [];
function report(step, ok, detail) {
  results.push({ step, ok, detail });
  console.log(`[${step}] ${ok ? 'PASS' : 'FAIL'} - ${detail}`);
}
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function psqlRun(sql, filename, { tuples = false } = {}) {
  const sqlPath = path.join(os.tmpdir(), filename);
  writeFileSync(sqlPath, sql, 'utf8');
  const args = ['/c', PSQL, '-h', '127.0.0.1', '-U', 'zorion', '-d', 'zorion', '-v', 'ON_ERROR_STOP=1'];
  if (tuples) args.push('-t', '-A');
  args.push('-f', sqlPath);
  return String(execFileSync('cmd', args, {
    env: { ...process.env, PGPASSWORD: process.env.PGPASSWORD || 'zorion123' },
  })).trim();
}

let browser = null;
const cleanupFns = [];
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  for (const fn of cleanupFns.reverse()) {
    try { fn(); } catch (e) { console.log('cleanup: ' + String(e)); }
  }
  process.exit(code);
}

// register — регистрация пользователя, возвращает {token, id}.
async function register(prefix) {
  for (let i = 0; i < 3; i++) {
    const username = prefix + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password: PASSWORD }),
    });
    if (res.status === 201) {
      const token = (await res.json()).token;
      const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
      return { token, id: me.id, username };
    }
    if (res.status !== 409) throw new Error('register HTTP ' + res.status);
  }
  throw new Error('register failed');
}

// openSettlementCard — открыть карточку планеты, вкладку «Поселения».
async function openSettlementCard(page, token, worldID, planetID) {
  // Сброс карты перед открытием: модалка уже могла быть открыта на другой
  // системе (повторный openSystemModal не перерисовывает правую панель).
  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForFunction(() => typeof window.openSystemModal === 'function', { timeout: 20000 }).catch(() => {});
  await sleep(1200);
  const info = await page.evaluate(async ({ base, wid, pid, tok }) => {
    const auth = { Authorization: 'Bearer ' + tok };
    const wRes = await fetch(base + '/worlds/' + wid, { headers: auth });
    const w = await wRes.json(); const ww = w.world || w;
    const pRes = await fetch(base + '/api/worlds/' + wid + '/planets', { headers: auth });
    const p = await pRes.json();
    const idx = p.planets.findIndex((x) => x.id === pid);
    return { x: ww.coord_x, y: ww.coord_y, name: ww.name || 'W', idx };
  }, { base: BASE_URL, wid: worldID, pid: planetID, tok: token });
  if (info.idx < 0) return { ok: false, reason: 'planet not found', info };
  const hasModalFn = await page.evaluate(() => typeof window.openSystemModal === 'function');
  if (!hasModalFn) return { ok: false, reason: 'openSystemModal missing', info };
  await page.evaluate((s) => {
    window.openSystemModal(s.wid, s.name, '', null, null, { x: s.x, y: s.y });
  }, { wid: worldID, name: info.name, x: info.x, y: info.y });
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await sleep(1200);
  const rowClicked = await page.evaluate((idx) => {
    const row = document.querySelector(`#right-panel tr[data-index="${idx}"]`);
    if (!row) return false;
    row.click();
    return true;
  }, info.idx);
  if (!rowClicked) return { ok: false, reason: 'row click failed', info };
  await sleep(1000);
  const tabClicked = await page.evaluate(() => {
    const btn = document.querySelector('button.tab-btn[data-tab="settlements"]');
    if (!btn) return false;
    btn.click();
    return true;
  });
  if (!tabClicked) return { ok: false, reason: 'settlements tab missing', info };
  await sleep(1200);
  return { ok: true, info };
}

// reRenderSettlements — перерисовать вкладку (после смены localStorage).
async function reRenderSettlements(page) {
  await page.evaluate(() => {
    const g = document.querySelector('button.tab-btn[data-tab="general"]');
    if (g) g.click();
  });
  await sleep(500);
  await page.evaluate(() => {
    const s = document.querySelector('button.tab-btn[data-tab="settlements"]');
    if (s) s.click();
  });
  await sleep(900);
}

async function panelLines(page) {
  return page.evaluate(() => {
    const panel = document.getElementById('right-panel');
    if (!panel) return [];
    return panel.innerText.split('\n').map((l) => l.trim()).filter(Boolean);
  });
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });

  cleanupFns.push(() => psqlRun(
    `DELETE FROM player_planet_knowledge WHERE user_id IN (SELECT id FROM users WHERE username LIKE '${ADMIN_PREFIX}%' OR username LIKE '${PLAYER_PREFIX}%');\n` +
    `DELETE FROM users WHERE username LIKE '${ADMIN_PREFIX}%' OR username LIKE '${PLAYER_PREFIX}%';\n`,
    'e2e_i23_cleanup.sql'));
  cleanupFns.push(() => psqlRun(readFileSync(RESOLVE_SQL, 'utf8'), 'e2e_i23_resolve_run.sql'));

  // ===== Аккаунты =====
  const admin = await register(ADMIN_PREFIX);
  psqlRun(`UPDATE users SET role='admin' WHERE id='${admin.id}';\n`, 'e2e_i23_admin.sql');
  const lr = await fetch(BASE_URL + '/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: admin.username, password: PASSWORD }),
  });
  let adminToken = admin.token;
  if (lr.ok) adminToken = (await lr.json()).token;
  const player = await register(PLAYER_PREFIX);
  const playerToken = player.token;

  // ===== [2] API-контракт (админ) =====
  const api = await (await fetch(BASE_URL + '/api/worlds/' + USMO_WORLD + '/planets', {
    headers: { Authorization: 'Bearer ' + adminToken },
  })).json();
  const usmo = (api.planets || []).find((p) => (p.settlements || []).some((s) => s.id === USMO_SETTLEMENT));
  const usmoS = usmo ? usmo.settlements.find((s) => s.id === USMO_SETTLEMENT) : null;
  const usmoB69 = usmoS ? (usmoS.branches || []).find((b) => b.recipe_id === 69) : null;
  console.log('API usmo stage=' + JSON.stringify(usmoS && usmoS.stage));
  console.log('API usmo branches=' + JSON.stringify((usmoS && usmoS.branches || []).map((b) => ({
    recipe_id: b.recipe_id, rate: b.rate, take: b.take, deposit_share: b.deposit_share, not_in_stage_set: b.not_in_stage_set,
  }))));
  const apiStageOK = !!usmoS && !!usmoS.stage &&
    typeof usmoS.stage.enter === 'number' && usmoS.stage.next_enter > 0;
  report('api-stage-view', apiStageOK,
    `settlement=${USMO_SETTLEMENT} stage=${JSON.stringify(usmoS && usmoS.stage)} type_name=${usmoS && usmoS.type_name}`);
  // Ветки Usmo (тип 152): у пары rate = NULL → «не объявлено» (поля rate/take
  // нет — не применимо). Контракт «rate/take где применимо» проверяем на
  // фикстуре (тип 148 × рецепт 69, rate = 10000) ниже.
  report('api-usmo-branch-nil-rate', !!usmoB69 && usmoB69.rate === undefined && usmoB69.take === undefined,
    `recipe69 rate=${JSON.stringify(usmoB69 && usmoB69.rate)} take=${JSON.stringify(usmoB69 && usmoB69.take)} (152×69 rate=NULL)`);

  // ===== Фикстура =====
  const setupOut = psqlRun(readFileSync(SETUP_SQL, 'utf8'), 'e2e_i23_setup_run.sql');
  const fixPlanet = ((setupOut.match(/I23_PLANET= (\S+)/) || [])[1] || '').replace(/['"]/g, '');
  const fixWorld = ((setupOut.match(/I23_WORLD= (\S+)/) || [])[1] || '').replace(/['"]/g, '');
  console.log('fixture planet=' + fixPlanet + ' world=' + fixWorld);
  if (!fixPlanet || !fixWorld) { console.log('FIXTURE NOT CREATED'); return finish(1); }
  // Игрок — присутствие: current_world + позиция «орбита планеты».
  psqlRun(
    `UPDATE users SET current_world_id='${fixWorld}', ` +
    `current_position='{"status":"orbit","object_type":"planet","object_id":"${fixPlanet}","level":"orbit"}'::jsonb ` +
    `WHERE id='${player.id}';\n`, 'e2e_i23_presence.sql');

  // ===== [2b] API-контракт на фикстуре (rate/take применимы) =====
  const fapi = await (await fetch(BASE_URL + '/api/worlds/' + fixWorld + '/planets', {
    headers: { Authorization: 'Bearer ' + adminToken },
  })).json();
  const fPlanet = (fapi.planets || []).find((p) => p.id === fixPlanet);
  const fS = fPlanet ? (fPlanet.settlements || []).find((s) => s.id === FIX_SETTLEMENT) : null;
  const fB = fS ? (fS.branches || []).find((b) => b.recipe_id === 69) : null;
  console.log('API fixture stage=' + JSON.stringify(fS && fS.stage) + ' population=' + (fS && fS.population));
  console.log('API fixture branch69=' + JSON.stringify(fB && { rate: fB.rate, take: fB.take, not_in_stage_set: fB.not_in_stage_set, deposit_share: fB.deposit_share }));
  const fixApiOK = !!fS && !!fS.stage && typeof fS.stage.enter === 'number' &&
    fS.stage.next_enter === 1000 && !!fB && typeof fB.rate === 'number' && fB.rate === 10000 &&
    Array.isArray(fB.take) && fB.take.length > 0;
  report('api-fixture-branch-fields', fixApiOK,
    `stage=${JSON.stringify(fS && fS.stage)} rate=${fB && fB.rate} take=${JSON.stringify(fB && fB.take)} pop=${fS && fS.population}`);

  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER'); return finish(1); }
  browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });

  // ===== [3] Админ — карточка «Usmo» =====
  const adminCtx = await browser.newContext({ viewport: { width: 1400, height: 950 } });
  await adminCtx.addInitScript((t) => {
    try { localStorage.setItem('token', t); localStorage.setItem('adminToken', t); } catch (e) {}
  }, adminToken);
  const adminPage = await adminCtx.newPage();
  const adminErrors = [];
  adminPage.on('pageerror', (e) => adminErrors.push(String(e.message || e)));
  await adminPage.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await adminPage.waitForSelector('#mapCanvas', { timeout: 15000 });
  await adminPage.waitForFunction(() => typeof window.openSystemModal === 'function', { timeout: 20000 }).catch(() => {});
  await adminPage.waitForTimeout(1500);

  const openUsmo = await openSettlementCard(adminPage, adminToken, USMO_WORLD, usmo ? usmo.id : '');
  if (!openUsmo.ok) { report('admin-usmo-card', false, openUsmo.reason); }
  else {
    const lines = await panelLines(adminPage);
    const state = await adminPage.evaluate(() => ({
      toastError: !!document.querySelector('.toast-error'),
      path: location.pathname,
    }));
    const hasStage = lines.some((l) => l.startsWith('Стадия:'));
    const stageLine = lines.find((l) => l.startsWith('Стадия:')) || '';
    const hasBranches = lines.some((l) => /^ветки \(/i.test(l));
    const ok = hasStage && hasBranches && !state.toastError &&
      state.path !== '/login-page' && adminErrors.length === 0;
    report('admin-usmo-card', ok,
      `stage=${hasStage} branches=${hasBranches} toast=${state.toastError} path=${state.path} pageErrors=${adminErrors.length} :: ${stageLine}`);
    await adminPage.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-i23-usmo-admin.png') });
  }

  // ===== [4] Админ — фикстура (полный луп) =====
  const openFix = await openSettlementCard(adminPage, adminToken, fixWorld, fixPlanet);
  if (!openFix.ok) { report('admin-fixture-arithmetic', false, openFix.reason); }
  else {
    const lines = await panelLines(adminPage);
    const arHeader = lines.some((l) => /арифметика \(на текущем населении, ед\/сутки\)/i.test(l));
    const arLine = lines.find((l) => /производим/.test(l) && /потребляем/.test(l)) || '';
    const rateLine = lines.find((l) => l.startsWith('скорость (число пары):')) || '';
    const takeLine = lines.find((l) => l.startsWith('забираем:')) || '';
    const stageLine = lines.find((l) => l.startsWith('Стадия:')) || '';
    const ok = arHeader && /продовольствие: производим/.test(arLine) &&
      /(сверх|дефицит)/.test(arLine) && !!rateLine && !!takeLine;
    report('admin-fixture-arithmetic', ok,
      `header=${arHeader} ar="${arLine}" rate="${rateLine}" take="${takeLine}" stage="${stageLine}"`);
    await adminPage.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-i23-fixture-admin.png') });
    console.log('FIXTURE LINES:\n' + lines.filter((l) => /Арифметика|производим|скорость \(число|забираем|Стадия|из залежи|не в наборе/.test(l)).join('\n'));

    // ===== [6] Масштаб =====
    await adminPage.evaluate(() => localStorage.setItem('gs_unitScale', 'billion'));
    await reRenderSettlements(adminPage);
    let l1 = await panelLines(adminPage);
    const rateBillion = (l1.find((l) => l.startsWith('скорость (число пары):')) || '');
    const takeBillion = (l1.find((l) => l.startsWith('забираем:')) || '');
    const arBillion = (l1.find((l) => /производим/.test(l) && /потребляем/.test(l)) || '');
    await adminPage.evaluate(() => localStorage.setItem('gs_unitScale', 'person'));
    await reRenderSettlements(adminPage);
    let l2 = await panelLines(adminPage);
    const ratePerson = (l2.find((l) => l.startsWith('скорость (число пары):')) || '');
    const takePerson = (l2.find((l) => l.startsWith('забираем:')) || '');
    const arPerson = (l2.find((l) => /производим/.test(l) && /потребляем/.test(l)) || '');
    const rateChanged = rateBillion !== ratePerson && rateBillion !== '' && ratePerson !== '';
    const unitChanged = /10⁹/.test(rateBillion) && /1 чел/.test(ratePerson);
    const othersSame = takeBillion === takePerson && arBillion === arPerson;
    report('scale-toggle', rateChanged && unitChanged && othersSame,
      `billion="${rateBillion}" person="${ratePerson}" rateChanged=${rateChanged} unitChanged=${unitChanged} ` +
      `takeSame=${takeBillion === takePerson} arSame=${arBillion === arPerson}`);
    await adminPage.evaluate(() => localStorage.setItem('gs_unitScale', 'billion'));
  }

  // ===== [5] Игрок (presence) =====
  const playerCtx = await browser.newContext({ viewport: { width: 1400, height: 950 } });
  await playerCtx.addInitScript((t) => {
    try { localStorage.setItem('token', t); } catch (e) {}
  }, playerToken);
  const playerPage = await playerCtx.newPage();
  const playerErrors = [];
  playerPage.on('pageerror', (e) => playerErrors.push(String(e.message || e)));
  await playerPage.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await playerPage.waitForSelector('#mapCanvas', { timeout: 15000 });
  await playerPage.waitForFunction(() => typeof window.openSystemModal === 'function', { timeout: 20000 }).catch(() => {});
  await playerPage.waitForTimeout(1500);
  const openPlayer = await openSettlementCard(playerPage, playerToken, fixWorld, fixPlanet);
  if (!openPlayer.ok) { report('player-presence-arithmetic', false, openPlayer.reason); }
  else {
    const lines = await panelLines(playerPage);
    const arHeader = lines.some((l) => /арифметика \(на текущем населении, ед\/сутки\)/i.test(l));
    const hasInputMarker = lines.some((l) => /вход \(виден только админу\)/i.test(l));
    const hasRate = lines.some((l) => l.startsWith('скорость (число пары):'));
    const ok = arHeader && !hasInputMarker && playerErrors.length === 0;
    report('player-presence-arithmetic', ok,
      `arHeader=${arHeader} rate=${hasRate} adminInputMarker=${hasInputMarker} pageErrors=${playerErrors.length}`);
    await playerPage.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-i23-fixture-player.png') });
    console.log('PLAYER LINES:\n' + lines.filter((l) => /Арифметика|производим|скорость \(число|забираем|Вход \(виден/.test(l)).join('\n'));
  }

  const failed = results.filter((r) => !r.ok).length;
  console.log('RESULT: ' + (failed ? 'FAIL' : 'PASS') + ' (' + results.length + ' checks, ' + failed + ' failed)');
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.log('RUN FAIL ' + String(e)); finish(1); });
