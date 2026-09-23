// tools/e2e/ship-sprites-humans-check.js
// Probe (2026-09-21, обновлена 2026-09-23 под П1): three "humans" race ship
// sprites in the dashboard grid (tab "Внешний вид", #ship-sel). Registers a
// player, opens the look tab, checks the tile count equals the player's race
// pool from /me.ship_options (selector filters by race — neutral is NOT
// selectable, so tiles = humans pool = 12, not len(ship_options) = 13) and
// that the 3 humans ones rendered (img loaded), screenshots the grid. Console
// output is ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic).
// Run: node ship-sprites-humans-check.js   (BASE_URL overrides default)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const NEW_FILES = ['race_humans_starship.png', 'race_humans_cruiser.png', 'race_humans_carrier.png'];

const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}

async function register() {
  const username = 'e2e_ship_' + Date.now();
  const password = 'e2e-pass-' + Date.now();
  const res = await fetch(BASE_URL + '/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  if (res.status !== 201) throw new Error('register HTTP ' + res.status);
  return (await res.json()).token;
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const token = await register();
  // Ожидаемое число плиток — пул расы игрока из /me.ship_options (растёт при
  // импорте, И6). Селектор фильтрует по расе игрока (спека 2026-09-23 §6.5):
  // нейтральный корабль не выбирается, поэтому плиток 12, а не 13.
  let expected = 0;
  try {
    const meRes = await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } });
    const me = await meRes.json();
    const opts = Array.isArray(me.ship_options) ? me.ship_options : [];
    const race = me.race_id || 'humans';
    expected = opts.filter((o) => (o.race || '') === race).length;
  } catch (e) { /* пусто — ниже FAIL с диагностикой */ }
  if (expected <= 0) {
    console.log('RESULT: FAIL - /me.ship_options has no tiles for player race');
    return finish(1);
  }
  const exe = findExecutable();
  if (!exe) {
    console.log('RESULT: FAIL - no Chrome/Edge found');
    return finish(1);
  }
  browser = await chromium.launch({ executablePath: exe, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));
  const spriteStatus = {};
  page.on('response', (res) => {
    const u = res.url();
    for (const f of NEW_FILES) if (u.endsWith('/static/sprites/' + f)) spriteStatus[f] = res.status();
  });

  await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('.dashboard-tabs', { timeout: 15000 });
  await page.click('.tab-btn[data-tab="tab-look"]');
  await page.waitForSelector('#ship-sel [data-file]', { timeout: 15000 });
  await page.waitForFunction((n) => document.querySelectorAll('#ship-sel [data-file]').length === n, expected, { timeout: 15000 });
  await page
    .waitForFunction((files) =>
      files.every((f) => {
        const img = document.querySelector(`#ship-sel [data-file="${f}"] img`);
        return img && img.complete && img.naturalWidth > 0;
      }), NEW_FILES, { timeout: 15000 })
    .catch(() => {});

  const state = await page.evaluate((files) => {
    const count = document.querySelectorAll('#ship-sel [data-file]').length;
    const missing = files.filter((f) => !document.querySelector(`#ship-sel [data-file="${f}"]`));
    const loaded = files.filter((f) => {
      const img = document.querySelector(`#ship-sel [data-file="${f}"] img`);
      return img && img.complete && img.naturalWidth > 0;
    });
    return { count, missing, loaded };
  }, NEW_FILES);

  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'ship-sprites-humans.png'), fullPage: true });

  console.log('tiles: ' + state.count + ' (expected ' + expected + ')');
  console.log('missing: ' + JSON.stringify(state.missing));
  console.log('loaded previews: ' + JSON.stringify(state.loaded));
  console.log('spriteHTTP: ' + JSON.stringify(spriteStatus));
  console.log('pageErrors: ' + pageErrors.length);
  const ok = state.count === expected && state.missing.length === 0 && state.loaded.length === 3 && pageErrors.length === 0;
  console.log('RESULT: ' + (ok ? 'PASS' : 'FAIL'));
  return finish(ok ? 0 : 1);
}

main().catch((e) => {
  console.log('RESULT: FAIL - ' + (e && e.message ? e.message : e));
  finish(1);
});
