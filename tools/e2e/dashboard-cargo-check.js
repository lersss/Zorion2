// tools/e2e/dashboard-cargo-check.js
// Browser smoke test for the dashboard "Cargo hold" block (spec
// 2026-09-22-трюм-грузоподъёмность-корабля §9.1/§9.3, subtask 3).
//
// Two passes:
//   * real API: empty hold -> bar "0 / 50 т", "Трюм пуст.", no "jettison all" btn;
//   * populated UI via request interception (there is NO player-facing "add"
//     handle by design, §9.2): row rendering, stored-XSS name escaped, bar
//     fill, per-row jettison POST {good_id, quantity}, "jettison all" POST {all:true}.
// Console output is ASCII (Windows PowerShell cp866 breaks Cyrillic).
//
// Run: node dashboard-cargo-check.js   (BASE_URL env overrides)
// Exit code 0 = ALL PASS, 1 = any FAIL.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
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

async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'cargo_' + Date.now() + '_' + attempt;
    const password = 'e2e-pass-' + Date.now();
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (res.status === 201) {
      const data = await res.json();
      return { username, token: data.token };
    }
    if (res.status === 409) continue;
    throw new Error('register HTTP ' + res.status + ': ' + (await res.text()));
  }
  throw new Error('register: name collision after 3 attempts');
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);

  let token = null;
  try {
    const creds = await registerAndLogin();
    token = creds.token;
    report('1/5 register/login', 'PASS', 'user ' + creds.username);
  } catch (err) {
    report('1/5 register/login', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const exe = findExecutable();
  if (!exe) {
    report('setup browser', 'FAIL', 'no Chrome/Edge found');
    return finish(1);
  }
  console.log('browser: ' + exe.name);

  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('setup browser', 'FAIL', 'launch: ' + String(err && err.message ? err.message : err));
    return finish(1);
  }

  const context = await browser.newContext({ viewport: { width: 1100, height: 900 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  const consoleErrors = [];
  const http404 = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));
  page.on('console', (m) => {
    if (m.type() === 'error') {
      const loc = m.location();
      consoleErrors.push(m.text() + (loc && loc.url ? ' @' + loc.url : ''));
    }
  });
  page.on('response', (res) => { if (res.status() === 404) http404.push(res.url()); });
  page.on('dialog', (d) => { d.accept().catch(() => {}); });

  try {
    // --- Step 2: real API, empty hold ---
    await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('.tab-btn[data-tab="tab-ship"]', { timeout: 15000 });
    await page.click('.tab-btn[data-tab="tab-ship"]');
    await page.waitForSelector('#cargo-items', { timeout: 15000 });
    await page.waitForTimeout(500);

    const empty = await page.evaluate(() => ({
      active: document.getElementById('tab-ship').classList.contains('active'),
      label: (document.getElementById('cargoBarLabel') || {}).textContent || '',
      fill: (document.getElementById('cargoBarFill') || {}).style ? document.getElementById('cargoBarFill').style.width : '',
      emptyText: (document.getElementById('cargo-items') || {}).textContent || '',
      allHidden: !!(document.getElementById('cargoJettisonAllBtn') || {}).hidden,
      rows: document.querySelectorAll('.cargo-row').length,
    }));
    const ok2 = empty.active && empty.label === '0 / 50 т' && empty.fill === '0%' &&
      empty.emptyText.includes('Трюм пуст') && empty.allHidden && empty.rows === 0;
    report('2/5 empty hold (real API)', ok2 ? 'PASS' : 'FAIL', JSON.stringify(empty));
    if (!ok2) return finish(1);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'dashboard-cargo-empty.png') });

    // --- Step 3: populated UI (interception; no add-handle by design) ---
    const XSS_NAME = '<img src=x onerror=alert(1)>';
    let mock = {
      limits: { mass: { used: 40, total: 50 } },
      items: [{ good_id: 21, name: XSS_NAME, kind: 'resource', quantity: 40, weight: 1, mass: 40 }],
    };
    const posts = [];
    await page.route('**/api/cargo/jettison', async (route) => {
      const body = JSON.parse(route.request().postData() || '{}');
      posts.push(body);
      if (body.all) {
        mock = { limits: { mass: { used: 0, total: 50 } }, items: [] };
      } else {
        const it = mock.items[0];
        const left = Math.max(0, it.quantity - body.quantity);
        mock = left <= 0
          ? { limits: { mass: { used: 0, total: 50 } }, items: [] }
          : { limits: { mass: { used: left, total: 50 } }, items: [{ ...it, quantity: left, mass: left }] };
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mock) });
    });
    await page.route('**/api/cargo', async (route) => {
      if (route.request().method() !== 'GET') return route.continue();
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mock) });
    });

    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.click('.tab-btn[data-tab="tab-ship"]');
    await page.waitForSelector('.cargo-row', { timeout: 15000 });
    await page.waitForTimeout(300);

    const populated = await page.evaluate((xssName) => {
      const nameEl = document.querySelector('.cargo-row-name');
      const metaEl = document.querySelector('.cargo-row-meta');
      return {
        label: document.getElementById('cargoBarLabel').textContent,
        fill: document.getElementById('cargoBarFill').style.width,
        nameText: nameEl ? nameEl.textContent : '',
        nameTagCount: document.querySelectorAll('.cargo-row-name img, .cargo-row-name script').length,
        meta: metaEl ? metaEl.textContent : '',
        qtyValue: (document.querySelector('.cargo-qty') || {}).value,
        allHidden: document.getElementById('cargoJettisonAllBtn').hidden,
        rows: document.querySelectorAll('.cargo-row').length,
        nameIsXssText: nameEl ? nameEl.textContent === xssName : false,
      };
    }, XSS_NAME);
    const ok3 = populated.label === '40 / 50 т' && populated.fill === '80%' &&
      populated.nameIsXssText && populated.nameTagCount === 0 &&
      populated.meta.includes('40 ед.') && populated.meta.includes('40 т') &&
      populated.qtyValue === '40' && populated.allHidden === false && populated.rows === 1;
    report('3/5 row render + escaped name', ok3 ? 'PASS' : 'FAIL', JSON.stringify({ ...populated, xssText: populated.nameText }));
    if (!ok3) return finish(1);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'dashboard-cargo-populated.png') });

    // Per-row jettison: set qty 15, click -> POST {good_id:21, quantity:15}.
    await page.fill('.cargo-qty', '15');
    await page.click('.cargo-jettison');
    await page.waitForTimeout(400);
    const afterRow = await page.evaluate(() => ({
      label: document.getElementById('cargoBarLabel').textContent,
      meta: (document.querySelector('.cargo-row-meta') || {}).textContent || '',
      status: (document.getElementById('cargo-status') || {}).textContent || '',
    }));
    const rowPost = posts[0] || {};
    const ok4 = rowPost.good_id === 21 && rowPost.quantity === 15 &&
      afterRow.label === '25 / 50 т' && afterRow.meta.includes('25 ед.') &&
      afterRow.status.includes('сброшен');
    report('4/5 per-row jettison POST', ok4 ? 'PASS' : 'FAIL',
      `post=${JSON.stringify(rowPost)} ui=${JSON.stringify(afterRow)}`);
    if (!ok4) return finish(1);

    // "Jettison all": POST {all:true}, hold becomes empty, button hides.
    await page.click('#cargoJettisonAllBtn');
    await page.waitForTimeout(400);
    const afterAll = await page.evaluate(() => ({
      label: document.getElementById('cargoBarLabel').textContent,
      emptyText: document.getElementById('cargo-items').textContent,
      allHidden: document.getElementById('cargoJettisonAllBtn').hidden,
    }));
    const allPost = posts[1] || {};
    const ok5 = allPost.all === true && afterAll.label === '0 / 50 т' &&
      afterAll.emptyText.includes('Трюм пуст') && afterAll.allHidden === true;
    report('5/5 jettison all POST', ok5 ? 'PASS' : 'FAIL',
      `post=${JSON.stringify(allPost)} ui=${JSON.stringify(afterAll)}`);
    if (!ok5) return finish(1);

    // Console clean (favicon.ico is the browser's own request, not the app).
    const errs = pageErrors.concat(consoleErrors.filter(t => !t.includes('favicon.ico')));
    const okClean = errs.length === 0;
    report('console clean', okClean ? 'PASS' : 'FAIL',
      `errors=${errs.length} http404=${JSON.stringify(http404)}` + (errs.length ? ' first: ' + errs[0] : ''));
    if (!okClean) return finish(1);
  } catch (err) {
    report('run', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const failed = results.filter(r => r.status === 'FAIL');
  console.log('');
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'ALL PASS'));
  return finish(failed.length ? 1 : 0);
}

main();
