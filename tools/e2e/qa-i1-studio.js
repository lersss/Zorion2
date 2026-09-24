// tools/e2e/qa-i1-studio.js — независимый прогон @tester, итерация И1 спеки
// «стадии поселения и скорость производства» (2026-09-23): число скорости на
// паре «постройка × рецепт», единица «ед/сутки/млрд», чтение миром, поля студии.
// Покрывает A6 (API студии), A7 (GET /studio/api/state), A8 (браузерный смоук).
// Временная admin-учётка — по образцу qa-e3-profile.mjs; каталог (params типа 148,
// rate пар) восстанавливается в конце.
// Run: node qa-i1-studio.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const PASSWORD = 'qa-i1-' + Date.now();
const TYPE_ID = 148;      // тип поселения («Деревня»), пара 148×{69,73}
const RECIPE_BOUND = 69;  // привязанный рецепт
const RECIPE_FREE = 71;   // рецепт есть, но к 148 не привязан → 409

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
function psqlRun(sql, filename) {
  const sqlPath = path.join(ARTIFACTS_DIR, filename);
  writeFileSync(sqlPath, sql, 'utf8');
  return String(execFileSync('cmd', ['/c', process.env.PSQL || 'C:\\pgsql\\pgsql\\bin\\psql.exe',
    '-h', '127.0.0.1', '-U', 'zorion', '-d', 'zorion', '-t', '-A', '-f', sqlPath],
    { env: { ...process.env, PGPASSWORD: process.env.PGPASSWORD || 'zorion123', PGCLIENTENCODING: 'UTF8' } })).trim();
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

async function registerAdmin() {
  let username = null, tok = null;
  for (let i = 0; i < 3; i++) {
    username = 'qa_i1_' + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
    if (res.status === 201) { tok = (await res.json()).token; break; }
    if (res.status !== 409) throw new Error('register HTTP ' + res.status);
  }
  if (!tok) throw new Error('register failed');
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + tok } })).json();
  psqlRun(`UPDATE users SET role='admin' WHERE id='${me.id}';\n`, 'qa-i1-role.sql');
  const lr = await fetch(BASE_URL + '/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
  if (lr.ok) tok = (await lr.json()).token;
  return { token: tok, username, uid: me.id };
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);

  const admin = await registerAdmin();
  token = admin.token;
  report('setup admin', !!token, 'username=' + admin.username);

  // Оригинал каталога — для восстановления после проверок.
  const origParamsText = psqlRun(`SELECT params::text FROM producer_types WHERE id=${TYPE_ID};\n`, 'qa-i1-orig.sql');
  const origParams = JSON.parse(origParamsText);
  const origRates = psqlRun(`SELECT producer_type_id||'|'||recipe_id||'|'||COALESCE(rate::text,'NULL') FROM producer_recipes WHERE producer_type_id IN (148,151) ORDER BY producer_type_id, recipe_id;\n`, 'qa-i1-origrate.sql');

  // ================= A6: API студии =================
  const r1 = await api('PUT', `/studio/api/producers/${TYPE_ID}/recipes/${RECIPE_BOUND}`, { rate: 650 });
  report('A6 rate=650 → 200', r1.status === 200, 'status=' + r1.status);

  const r2 = await api('PUT', `/studio/api/producers/${TYPE_ID}/recipes/${RECIPE_BOUND}`, { rate: -5 });
  report('A6 rate<0 → 422', r2.status === 422, 'status=' + r2.status);

  const r3 = await api('PUT', `/studio/api/producers/${TYPE_ID}/recipes/${RECIPE_FREE}`, { rate: 100 });
  report('A6 пара нет → 409', r3.status === 409, 'status=' + r3.status);

  const r4 = await api('PUT', `/studio/api/producers/999999/recipes/${RECIPE_BOUND}`, { rate: 100 });
  report('A6 нет типа → 404', r4.status === 404, 'status=' + r4.status);

  const r5 = await api('PUT', `/studio/api/producers/${TYPE_ID}/recipes/999999`, { rate: 100 });
  report('A6 нет рецепта → 404', r5.status === 404, 'status=' + r5.status);

  const r6 = await api('PUT', `/studio/api/producers/${TYPE_ID}`, { stage: { enter: 100, exit: 200 } });
  report('A6 exit>=enter → 422', r6.status === 422, 'status=' + r6.status);

  const r7 = await api('PUT', `/studio/api/producers/${TYPE_ID}`, { stage: { enter: 200, exit: 100 } });
  report('A6 exit<enter → 200', r7.status === 200, 'status=' + r7.status);

  // позиция «вода» в eat, но её нет в effects → 200 + предупреждение (не 422)
  const r8 = await api('PUT', `/studio/api/producers/${TYPE_ID}`, { eat: { 'вода': 700, 'продовольствие': 600 }, effects: { 'продовольствие': 'голод' } });
  const warned = r8.status === 200 && Array.isArray(r8.data && r8.data.warnings) && r8.data.warnings.some(w => /без эффекта/.test(w));
  report('A6 eat-без-эффекта → 200+warning', warned, 'status=' + r8.status + ' warnings=' + JSON.stringify(r8.data && r8.data.warnings));

  // ================= A7: GET /studio/api/state =================
  const st = (await api('GET', '/studio/api/state')).data;
  const pr = (st.producer_recipes || []).find(b => Number(b.producer_type_id) === TYPE_ID && Number(b.recipe_id) === RECIPE_BOUND);
  const allHaveRate = (st.producer_recipes || []).every(b => Object.prototype.hasOwnProperty.call(b, 'rate'));
  report('A7 producer_recipes.rate есть', allHaveRate && !!pr && Number(pr.rate) === 650, `allHaveRate=${allHaveRate} pairRate=${pr && pr.rate}`);
  const pt = (st.producer_types || []).find(p => Number(p.id) === TYPE_ID);
  const par = pt && pt.params || {};
  const paramsFull = par && par.eat && par.eat_units === 'per_day_per_billion' && par.stage && par.stage.enter === 200 && par.eat['вода'] === 700;
  report('A7 params целиком (eat/eat_units/stage)', !!paramsFull, 'params=' + JSON.stringify(par));

  // ================= A8: браузерный смоук студии =================
  const exe = findExecutable();
  if (!exe) { report('A8 browser', false, 'no Chrome/Edge'); }
  else {
    browser = await chromium.launch({ executablePath: exe, headless: true });
    const context = await browser.newContext({ viewport: { width: 1400, height: 1000 } });
    await context.addInitScript((t) => { try { localStorage.setItem('adminToken', t); } catch (e) {} }, token);
    const page = await context.newPage();
    const pageErrors = [];
    const badResponses = [];
    page.on('pageerror', (err) => pageErrors.push(String(err && err.stack ? err.stack : err)));
    page.on('console', (m) => { if (m.type() === 'error') { let loc = ''; try { loc = JSON.stringify(m.location()); } catch (e) {} pageErrors.push('console.error: ' + m.text() + ' @ ' + loc); } });
    page.on('response', (r) => { if (r.status() >= 400) badResponses.push(r.status() + ' ' + r.url()); });
    page.on('requestfailed', (r) => badResponses.push('FAILED ' + r.url() + ' ' + (r.failure() && r.failure().errorText)));
    try {
      await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
      await page.waitForFunction(() => { const o = document.getElementById('loadingOverlay'); return o && o.style.display === 'none'; }, null, { timeout: 15000 });
      await page.click('#branchProdColony'); // раздел построек (спека 2026-09-25): ветка «Производители» снята
      await page.waitForFunction(() => Array.isArray(state.producer_types) && state.producer_types.some(p => Number(p.id) === 148), null, { timeout: 10000 });
      await page.evaluate((id) => openProdPopup(id), TYPE_ID);
      await page.waitForFunction(() => { const e = document.getElementById('detailPopup'); return e && e.style.display !== 'none' && document.querySelector('#popupBody .pblock'); }, null, { timeout: 10000 });
      await page.waitForTimeout(800);

      const ui = await page.evaluate(() => {
        const body = document.getElementById('popupBody');
        const text = body.innerText || '';
        const rateInputs = [...body.querySelectorAll('input[onchange^="saveRecipeRate"]')];
        const eatNorm = [...body.querySelectorAll('input[placeholder="по умолчанию 600"]')];
        const details = body.querySelector('details.prod-json');
        return {
          text,
          rateValues: rateInputs.map(i => i.value),
          hasRateField: rateInputs.length > 0,
          hasDefault600: eatNorm.length > 0,
          rawJsonOpen: details ? details.open : null,
          hasRawJson: !!details,
        };
      });
      const hasProd = /Производит/.test(ui.text), hasCons = /Потребляет/.test(ui.text);
      report('A8 блоки Производит/Потребляет', hasProd && hasCons,
        `hasProd=${hasProd} hasCons=${hasCons} rateField=${ui.hasRateField} rateValues=${JSON.stringify(ui.rateValues)} textLen=${(ui.text || '').length}`);
      report('A8 пустое число = не производит', ui.hasRateField && ui.rateValues.includes('') && /без числа — не производит/.test(ui.text),
        `rateValues=${JSON.stringify(ui.rateValues)}`);
      report('A8 предупреждение «позиция без эффекта»', /без эффекта — не потребляется/.test(ui.text), '');
      report('A8 «по умолчанию 600» placeholder', ui.hasDefault600, '');
      report('A8 сырой JSON свёрнут', ui.hasRawJson && ui.rawJsonOpen === false, `hasRawJson=${ui.hasRawJson} open=${ui.rawJsonOpen}`);
      report('A8 консоль без JS-ошибок', pageErrors.length === 0, 'errors=' + pageErrors.length + (pageErrors.length ? ' :: ' + pageErrors[0].slice(0, 200) : '') + ' badResponses=' + JSON.stringify(badResponses));
      const shot = path.join(ARTIFACTS_DIR, 'qa-i1-studio.png');
      await page.screenshot({ path: shot });
      report('A8 скриншот', true, 'qa-i1-studio.png');
    } catch (err) {
      report('A8 UNCAUGHT', false, String(err && err.message ? err.message : err));
    }
  }

  // ================= Восстановление каталога =================
  try {
    // params типа 148 — через сырой путь (без признака eat_units-логики)
    await api('PUT', `/studio/api/producers/${TYPE_ID}`, { params: JSON.stringify(origParams) });
    for (const line of origRates.split(/\r?\n/).map(s => s.trim()).filter(Boolean)) {
      const [tid, rid, rate] = line.split('|');
      await api('PUT', `/studio/api/producers/${tid}/recipes/${rid}`, { rate: rate === 'NULL' ? null : Number(rate) });
    }
    const back = psqlRun(`SELECT params::text FROM producer_types WHERE id=${TYPE_ID};\n`, 'qa-i1-back.sql');
    report('cleanup restore', back.replace(/\s/g, '') === JSON.stringify(origParams).replace(/\s/g, ''), 'restored');
    psqlRun(`DELETE FROM users WHERE id='${admin.uid}';\n`, 'qa-i1-deluser.sql');
  } catch (e) {
    report('cleanup', false, String(e && e.message ? e.message : e));
  }

  const fails = results.filter(r => !r.ok);
  console.log(`\nTOTAL: ${results.length} steps, ${fails.length} FAIL`);
  return finish(fails.length ? 1 : 0);
}

main();
