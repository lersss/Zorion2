// tools/e2e/dashboard-property-check.js
// Browser smoke test for the dashboard "Property" tab + dashboard->map deeplink
// (spec 2026-09-26-собственность-игрока-в-дашборде §6/§7.2, T14/T15).
//
// Passes:
//   * real API: empty property -> empty-state text, tab opens, no JS errors;
//   * populated UI via request interception (there is NO player-facing "own"
//     handle by design: owner is set by admin): row render, address, knowledge
//     badge, "Посмотреть" href;
//   * "Перелететь" button (ЧК2): active href carries &fly=1; with no engine the
//     button is a disabled span with tooltip and no href (§8.4);
//   * deeplink: click -> /map?system=..&planet=..&wname=.., popup opens for a
//     cached world, auto-open markers cleared, URL wiped via replaceState;
//   * F5 after deeplink does NOT reopen the popup;
//   * deeplink to an invisible/unknown system -> toast, no popup.
// Console output is ASCII (Windows PowerShell cp866 breaks Cyrillic).
//
// Run: node dashboard-property-check.js   (BASE_URL env overrides)
// Exit code 0 = ALL PASS, 1 = any FAIL.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
// Unknown-world probe (step 6): /worlds/<id> 404 is expected and handled (toast),
// so its browser network-error console line is not a clean-console failure.
const INVISIBLE_ID = '00000000-0000-4000-8000-000000000000';

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
    const username = 'property_' + Date.now() + '_' + attempt;
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
    report('1/8 register/login', 'PASS', 'user ' + creds.username);
  } catch (err) {
    report('1/8 register/login', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  // Player's world + a planet: the mocked property points there, so the world is
  // in the map cache and the deeplink popup opens (cached-world branch, §7.2).
  const auth = { Authorization: 'Bearer ' + token };
  const meRes = await fetch(BASE_URL + '/me', { headers: auth });
  const me = await meRes.json();
  const worldId = me && me.current_world_id;
  let worldName = 'Sysat';
  let planetId = '';
  if (worldId) {
    try {
      const wRes = await fetch(BASE_URL + '/worlds/' + worldId, { headers: auth });
      if (wRes.ok) { const w = await wRes.json(); worldName = (w.world || w).name || worldName; }
      const pRes = await fetch(BASE_URL + '/api/worlds/' + worldId + '/planets', { headers: auth });
      if (pRes.ok) { const pl = await pRes.json(); if (pl && pl.planets && pl.planets[0]) planetId = pl.planets[0].id; }
    } catch (e) { /* keep defaults */ }
  }
  const propertyItem = {
    kind: 'settlement',
    id: 's1',
    name: 'Люди',
    subtitle: 'Поселение',
    planet_id: planetId,
    planet_name: 'Кеплер-3 b',
    world_id: worldId || 'w1',
    world_name: worldName,
    knowledge: { mode: 'snapshot', at: '2026-09-23T12:00:00Z', fresh: true },
  };

  const exe = findExecutable();
  if (!exe) { report('setup browser', 'FAIL', 'no Chrome/Edge found'); return finish(1); }
  console.log('browser: ' + exe.name);

  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('setup browser', 'FAIL', 'launch: ' + String(err && err.message ? err.message : err));
    return finish(1);
  }

  const context = await browser.newContext({ viewport: { width: 1200, height: 900 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  const consoleErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));
  page.on('console', (m) => {
    if (m.type() === 'error') {
      const loc = m.location();
      consoleErrors.push(m.text() + (loc && loc.url ? ' @' + loc.url : ''));
    }
  });
  page.on('dialog', (d) => { d.accept().catch(() => {}); });

  try {
    // --- Step 2: real API, tab exists, empty property ---
    await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('.tab-btn[data-tab="tab-property"]', { timeout: 15000 });
    await page.click('.tab-btn[data-tab="tab-property"]');
    await page.waitForTimeout(600);

    const empty = await page.evaluate(() => ({
      active: document.getElementById('tab-property').classList.contains('active'),
      text: (document.getElementById('tab-property') || {}).textContent || '',
      hasBtn: !!document.querySelector('.tab-btn[data-tab="tab-property"]'),
      rows: document.querySelectorAll('.property-row').length,
    }));
    const ok2 = empty.hasBtn && empty.active && empty.rows === 0 &&
      empty.text.includes('У вас пока нет собственности');
    report('2/8 tab empty (real API)', ok2 ? 'PASS' : 'FAIL', JSON.stringify(empty));
    if (!ok2) return finish(1);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'dashboard-property-empty.png') });

    // --- Step 3: populated UI via interception (no player-facing own-handle) ---
    // Engine gate (§8.4): pass /me through and pin the engine flag so the
    // "Перелететь" button state is deterministic (the real starter loadout is not
    // guaranteed here); mockEngine=false flips it off for step 3b.
    let mockEngine = true;
    await page.route('**/me', async (route) => {
      const response = await route.fetch();
      let json = null;
      try { json = await response.json(); } catch (e) { /* non-JSON: passthrough */ }
      if (json && typeof json === 'object') {
        if (mockEngine) {
          json.ship_catalog = [{ id: 'e2e_engine', type: 'engine' }];
          json.equipment = Object.assign({}, json.equipment || {}, { engine: 'e2e_engine' });
        } else {
          json.ship_catalog = [];
          json.equipment = {};
        }
        await route.fulfill({ status: response.status(), contentType: 'application/json', body: JSON.stringify(json) });
        return;
      }
      await route.fulfill({ response });
    });
    await page.route('**/me/property', async (route) => {
      await route.fulfill({
        status: 200, contentType: 'application/json',
        body: JSON.stringify({ items: [propertyItem] }),
      });
    });
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.click('.tab-btn[data-tab="tab-property"]');
    await page.waitForSelector('.property-row', { timeout: 15000 });
    await page.waitForTimeout(300);

    const listed = await page.evaluate(() => {
      const row = document.querySelector('.property-row');
      const link = document.querySelector('.property-view-btn');
      const fly = document.querySelector('.property-fly-btn');
      return {
        rows: document.querySelectorAll('.property-row').length,
        text: row ? row.textContent : '',
        href: link ? link.getAttribute('href') : '',
        flyHref: fly ? fly.getAttribute('href') : '',
        flyDisabled: !!(fly && fly.classList.contains('is-disabled')),
      };
    });
    const ok3 = listed.rows === 1 &&
      listed.text.includes('Люди') && listed.text.includes('Поселение') &&
      listed.text.includes(worldName + ' › Кеплер-3 b') &&
      listed.text.includes('Данные на') && listed.text.includes('Посмотреть') &&
      listed.text.includes('Перелететь') && !listed.flyDisabled &&
      listed.href.startsWith('/map?system=' + encodeURIComponent(propertyItem.world_id)) &&
      listed.flyHref.includes('&fly=1');
    report('3/8 row render + view/fly href', ok3 ? 'PASS' : 'FAIL',
      `rows=${listed.rows} href=${listed.href} flyHref=${listed.flyHref} flyDisabled=${listed.flyDisabled}`);
    if (!ok3) return finish(1);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'dashboard-property-list.png') });

    // --- Step 3b: no engine -> "Перелететь" is a disabled span, no href (§8.4) ---
    mockEngine = false;
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.click('.tab-btn[data-tab="tab-property"]');
    await page.waitForSelector('.property-row', { timeout: 15000 });
    await page.waitForTimeout(300);
    const noEngine = await page.evaluate(() => {
      const fly = document.querySelector('.property-fly-btn');
      return {
        disabled: !!(fly && fly.classList.contains('is-disabled')),
        tag: fly ? fly.tagName : '',
        href: fly ? fly.getAttribute('href') : null,
        title: fly ? fly.getAttribute('title') : '',
        view: !!document.querySelector('.property-view-btn'),
      };
    });
    const ok3b = noEngine.disabled && noEngine.tag === 'SPAN' && noEngine.href === null &&
      noEngine.title.includes('Двигатель не установлен') && noEngine.view;
    report('3b/8 fly disabled without engine', ok3b ? 'PASS' : 'FAIL', JSON.stringify(noEngine));
    if (!ok3b) return finish(1);
    mockEngine = true;

    // --- Step 4: deeplink dashboard -> map ---
    // Seed stale auto-open markers (C3): the explicit deeplink must suppress the
    // auto-openers and clear both markers.
    await page.evaluate(() => {
      sessionStorage.setItem('compositeRoute', 'stale-world');
      sessionStorage.setItem('beltReturn', 'stale-world');
    });
    await page.click('.property-view-btn');
    await page.waitForURL('**/map**', { timeout: 30000 });
    // Wait for the deeplink popup (initMap: loadClusters -> handleDashboardDeepLink).
    await page.waitForSelector('#system-modal-overlay', { timeout: 30000 });
    await page.waitForTimeout(500);

    const linked = await page.evaluate(() => ({
      path: location.pathname,
      search: location.search,
      overlay: !!document.getElementById('system-modal-overlay'),
      toastError: !!document.querySelector('.toast-error'),
      composite: sessionStorage.getItem('compositeRoute'),
      belt: sessionStorage.getItem('beltReturn'),
    }));
    const ok4 = linked.path === '/map' && linked.search === '' && linked.overlay &&
      !linked.toastError && linked.composite === null && linked.belt === null;
    report('4/8 deeplink popup + url wiped', ok4 ? 'PASS' : 'FAIL', JSON.stringify(linked));
    if (!ok4) return finish(1);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'deeplink-popup.png') });

    // --- Step 5: F5 does not reopen the popup (URL already wiped) ---
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(1500);
    const afterReload = await page.evaluate(() => ({
      path: location.pathname,
      overlay: !!document.getElementById('system-modal-overlay'),
    }));
    const ok5 = afterReload.path === '/map' && !afterReload.overlay;
    report('5/8 F5 no reopen', ok5 ? 'PASS' : 'FAIL', JSON.stringify(afterReload));
    if (!ok5) return finish(1);

    // --- Step 6: deeplink to an unknown/invisible system -> toast, no popup (§7.2) ---
    await page.goto(BASE_URL + '/map?system=' + INVISIBLE_ID + '&wname=Ghost',
      { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(2500);
    const invisible = await page.evaluate(() => {
      const toast = document.querySelector('.toast-error .toast-msg');
      return {
        path: location.pathname,
        search: location.search,
        overlay: !!document.getElementById('system-modal-overlay'),
        toast: toast ? toast.textContent : '',
      };
    });
    const ok6 = invisible.path === '/map' && invisible.search === '' && !invisible.overlay &&
      invisible.toast.includes('система вне зоны видимости');
    report('6/8 deeplink invisible -> toast', ok6 ? 'PASS' : 'FAIL', JSON.stringify(invisible));
    if (!ok6) return finish(1);

    // --- Step 7: console clean ---
    // favicon.ico (browser's own request) and the expected /worlds/<unknown> 404
    // of the step-6 probe are not app errors.
    const errs = pageErrors.concat(consoleErrors.filter(t =>
      !t.includes('favicon.ico') && !t.includes(INVISIBLE_ID)));
    const ok7 = errs.length === 0;
    report('7/8 console clean', ok7 ? 'PASS' : 'FAIL',
      `errors=${errs.length}` + (errs.length ? ' first: ' + errs[0] : ''));
    if (!ok7) return finish(1);

    // --- Step 8: load error keeps the page alive (§6.2, T12) ---
    await page.route('**/me/property', async (route) => {
      await route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"boom"}' });
    });
    await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded' });
    await page.click('.tab-btn[data-tab="tab-property"]');
    await page.waitForTimeout(600);
    const errored = await page.evaluate(() => ({
      path: location.pathname,
      text: (document.getElementById('tab-property') || {}).textContent || '',
      alive: !!document.querySelector('.tab-btn[data-tab="tab-property"]'),
    }));
    const ok8 = errored.path !== '/login-page' && errored.alive &&
      errored.text.includes('Не удалось загрузить собственность');
    report('8/8 load error keeps page alive', ok8 ? 'PASS' : 'FAIL', JSON.stringify(errored));
    if (!ok8) return finish(1);
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
