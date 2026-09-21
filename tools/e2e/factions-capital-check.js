// tools/e2e/factions-capital-check.js
// Browser smoke for the "faction capitals" feature (spec
// 2026-09-21-фабрики-релиз-2-столицы-фракций, §8 test 8): admin opens a planet
// card, the "Factions" tab shows a faction and its capital row (🏛 Столица),
// and a click expands the inline details block with the owner.
//
// Run: ADMIN_TOKEN=<dev admin JWT> node factions-capital-check.js
// (BASE_URL env overrides the default; token minted per tools/e2e/README.md).
// Exit code 0 = all PASS (SKIP allowed), 1 = any FAIL. Console output is ASCII
// on purpose (Windows PowerShell cp866 breaks Cyrillic).
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

const ADMIN_TOKEN = process.env.ADMIN_TOKEN;
if (!ADMIN_TOKEN) {
  console.error('ADMIN_TOKEN env required: dev admin JWT (see tools/e2e/README.md).');
  process.exit(1);
}

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

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);

  const exe = findExecutable();
  if (!exe) { report('setup browser', 'FAIL', 'no Chrome/Edge found'); return finish(1); }
  console.log('browser: ' + exe.name + ' (' + exe.path + ')');

  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('setup browser', 'FAIL', 'launch: ' + String(err && err.message ? err.message : err));
    return finish(1);
  }

  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => {
    localStorage.setItem('token', t);
    localStorage.setItem('adminToken', t);
  }, ADMIN_TOKEN);
  const page = await context.newPage();

  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));

  try {
    // --- Step 1: open the map (admin token already in localStorage) ---
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1000);
    report('1/4 map open', 'PASS', 'canvas ready');

    // --- Step 2: open own system modal, click a planet row ---
    const opened = await page.evaluate(async (baseUrl) => {
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };
      const meRes = await fetch(baseUrl + '/me', { headers: auth });
      if (!meRes.ok) return { ok: false, reason: '/me HTTP ' + meRes.status };
      const me = await meRes.json();
      const wid = me && me.current_world_id;
      if (!wid) return { ok: false, reason: 'no current world (generate universe + planets + settlements first)' };

      const wRes = await fetch(baseUrl + '/worlds/' + wid, { headers: auth });
      if (!wRes.ok) return { ok: false, reason: '/worlds HTTP ' + wRes.status };
      const w = await wRes.json();
      const ww = w.world || w;

      if (typeof window.openSystemModal !== 'function') return { ok: false, reason: 'openSystemModal missing' };
      window.openSystemModal(wid, ww.name, ww.spectral_class, null, null, {
        stype: ww.star_type, stemp: ww.temperature, systype: ww.system_type,
        smods: ww.stellar_mods, x: ww.coord_x, y: ww.coord_y,
      });
      return { ok: true, wid };
    }, BASE_URL);
    if (!opened.ok) { report('2/4 system modal', 'SKIP', opened.reason); return finish(0); }

    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForTimeout(1500);
    const planetClicked = await page.evaluate(async () => {
      const panel = document.getElementById('right-panel');
      const tr = panel && panel.querySelector('tr[data-index]');
      if (!tr) return false;
      tr.click();
      await new Promise(r => setTimeout(r, 1200));
      return true;
    });
    if (!planetClicked) { report('2/4 system modal', 'SKIP', 'no planet rows in the system'); return finish(0); }
    report('2/4 system modal', 'PASS', 'planet card open');

    // --- Step 3: Factions tab shows factions + capital row ---
    const tab = await page.evaluate(async () => {
      const panel = document.getElementById('right-panel');
      const btn = panel && panel.querySelector('[data-tab="factions"]');
      if (!btn) return { ok: false, reason: 'factions tab button missing' };
      btn.click();
      await new Promise(r => setTimeout(r, 400));
      const text = panel.textContent;
      const toggles = panel.querySelectorAll('[data-faction-toggle]');
      const capitalRow = [...panel.querySelectorAll('div')].some(d => d.textContent.includes('Столица'));
      const emptyScanner = text.includes('В отчёте сканера фракции не значились');
      const noData = text.includes('Нет данных');
      return { ok: true, toggles: toggles.length, capitalRow, emptyScanner, noData };
    });
    if (!tab.ok) { report('3/4 factions tab', 'FAIL', tab.reason); return finish(1); }
    if (tab.toggles === 0) {
      // Honest SKIP: no factions on this planet (region without settled planets)
      // or a player-less knowledge state. Not a failure of the feature.
      report('3/4 factions tab', 'SKIP',
        `no faction cards (emptyScanner=${tab.emptyScanner} noData=${tab.noData})`);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'factions-capital-empty.png') });
      return finish(0);
    }
    const ok3 = tab.capitalRow;
    report('3/4 factions tab', ok3 ? 'PASS' : 'FAIL',
      `factions=${tab.toggles} capitalRow=${tab.capitalRow}`);
    if (!ok3) return finish(1);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'factions-capital.png') });

    // --- Step 4: click a faction -> inline details block with the owner ---
    const expanded = await page.evaluate(async () => {
      const panel = document.getElementById('right-panel');
      const toggle = panel.querySelector('[data-faction-toggle]');
      if (!toggle) return { ok: false, reason: 'no faction toggle' };
      const id = toggle.getAttribute('data-faction-toggle');
      const details = panel.querySelector('[data-faction-details="' + id + '"]');
      if (!details) return { ok: false, reason: 'no details block for faction ' + id };
      const before = getComputedStyle(details).display;
      toggle.click();
      await new Promise(r => setTimeout(r, 200));
      const after = getComputedStyle(details).display;
      return { ok: true, before, after, owner: details.textContent.includes('Владелец:') };
    });
    const ok4 = expanded.ok && expanded.before === 'none' && expanded.after !== 'none' && expanded.owner;
    report('4/4 expand details', ok4 ? 'PASS' : 'FAIL',
      `before=${expanded.before} after=${expanded.after} owner=${expanded.owner}` +
      (expanded.ok ? '' : ' reason=' + expanded.reason));
    if (!ok4) return finish(1);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'factions-capital-details.png') });
  } catch (err) {
    report('run', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  // --- Step 5: no toast errors, no login redirect (spec point 6) ---
  const toastErrors = await page.locator('.toast-error').count();
  const finalUrl = page.url();
  const loggedOut = finalUrl.includes('/login-page');
  report('5/5 no toast errors', toastErrors === 0 ? 'PASS' : 'FAIL', 'toast-error count=' + toastErrors);
  report('5/5 no login redirect', !loggedOut ? 'PASS' : 'FAIL', 'url=' + finalUrl);
  if (toastErrors > 0 || loggedOut) return finish(1);

  const errs = pageErrors.slice();
  const okErrors = errs.length === 0;
  report('no page errors', okErrors ? 'PASS' : 'FAIL', errs.length ? errs.join(' | ') : '0 errors');
  if (!okErrors) return finish(1);

  const failed = results.filter(r => r.status === 'FAIL');
  console.log('');
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'ALL PASS'));
  return finish(failed.length ? 1 : 0);
}

main();
