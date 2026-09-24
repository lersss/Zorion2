// tools/e2e/market-check.js
// Browser smoke test for the planet-card "Shop" tab (module market, spec
// 2026-09-24-магазин-модулей-локальный-рынок §10). playwright-core + system
// Chrome/Edge. Catches client regressions static review misses: tab visibility
// by settlement knowledge, 4 offer cards, slot pick with trade-in recalculation,
// buy button gate when can_trade == false, and a successful buy updating the
// slot cell.
//
// The scenario (planet with a live settlement + player knowledge + player on
// orbit) is assembled against the dev DB through tools/db.ps1: the freshly
// registered throwaway user gets a player_planet_knowledge row (source='scan',
// settlements_count=1) and an orbit position on that planet, then the modal is
// driven through the real API. Steps that cannot be assembled are reported SKIP.
//
// Run: node market-check.js   (BASE_URL env overrides the default)
// Console output is ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic).
import { chromium } from 'playwright-core';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const HERE = path.dirname(fileURLToPath(import.meta.url));
const ARTIFACTS_DIR = path.join(HERE, 'artifacts');
const REPO_ROOT = path.resolve(HERE, '..', '..');
const DB_PS1 = path.join(REPO_ROOT, 'tools', 'db.ps1');

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

// sql — run SQL through the project wrapper (encoding/password/multi-line are
// handled there). Returns psql stdout; ASCII markers survive any code page.
function sql(query) {
  return execFileSync('powershell.exe',
    ['-ExecutionPolicy', 'Bypass', '-File', DB_PS1, '-Sql', query],
    { encoding: 'utf8' });
}

// sqlMarker — first value after "<MARKER>:" in the wrapper output (UUIDs/numbers
// are ASCII; header/pipes are ignored).
function sqlMarker(query, marker) {
  const out = sql(query);
  const m = out.match(new RegExp(marker + ':([^\\s|]+)'));
  return m ? m[1] : null;
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'e2e_market_' + Date.now() + '_' + attempt;
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

// openMarketTab — open the system modal for worldId, click the planet row at
// planetIndex, switch to the "Shop" tab, wait for offers to load.
async function openMarketTab(page, worldId, worldName, planetIndex) {
  await page.evaluate((args) => {
    window.openSystemModal(args.worldId, args.worldName, 'G', null, null, null);
  }, { worldId, worldName });
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForSelector('#right-panel tr[data-index]', { timeout: 15000 });
  await page.click(`#right-panel tr[data-index="${planetIndex}"]`);
  await page.waitForSelector('.tab-btn[data-tab="market"]', { timeout: 10000 });
  await page.click('.tab-btn[data-tab="market"]');
  await page.waitForSelector('[data-market-buy]', { timeout: 10000 });
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);

  // --- Step 1: register/login ---
  let token = null, userId = null, worldId = null, worldName = '';
  try {
    const creds = await registerAndLogin();
    token = creds.token;
    const meRes = await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } });
    if (!meRes.ok) throw new Error('/me HTTP ' + meRes.status);
    const me = await meRes.json();
    userId = me.id;
    worldId = me.current_world_id;
    worldName = me.current_world_name || 'World';
    report('1/6 register/login', 'PASS', 'user ' + creds.username);
  } catch (err) {
    report('1/6 register/login', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  // --- Step 2: assemble the scenario (planet + knowledge + orbit) ---
  let planetId = null, planetIndex = -1;
  try {
    planetId = sqlMarker(
      `SELECT 'PLANET:'||s.planet_id FROM settlements s JOIN planets p ON p.id = s.planet_id ` +
      `WHERE p.world_id = '${worldId}' AND s.population_exact > 0 LIMIT 1;`, 'PLANET');
    if (!planetId) {
      report('2/6 assemble scenario', 'SKIP', 'no planet with a live settlement in the current world');
      return finish(0);
    }
    sql(
      `INSERT INTO player_planet_knowledge (user_id, planet_id, data, scanned_at, source) ` +
      `VALUES ('${userId}', '${planetId}', jsonb_build_object('settlements_count', 1), NOW(), 'scan') ` +
      `ON CONFLICT (user_id, planet_id) DO UPDATE SET data = EXCLUDED.data, scanned_at = NOW(), source = 'scan';`);
    sql(
      `UPDATE users SET current_position = ` +
      `jsonb_build_object('level','orbit','status','orbit','object_type','planet','object_id','${planetId}') ` +
      `WHERE id = '${userId}';`);
    report('2/6 assemble scenario', 'PASS', 'planet ' + planetId + ' (knowledge scan, orbit)');
  } catch (err) {
    report('2/6 assemble scenario', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  // Planet index in the modal list = index in the endpoint's planets array.
  try {
    const res = await fetch(BASE_URL + '/api/worlds/' + worldId + '/planets', {
      headers: { Authorization: 'Bearer ' + token },
    });
    if (!res.ok) throw new Error('planets HTTP ' + res.status);
    const data = await res.json();
    const planets = Array.isArray(data.planets) ? data.planets : [];
    planetIndex = planets.findIndex(p => p.id === planetId);
    if (planetIndex < 0) {
      report('3/6 tab visible', 'SKIP', 'planet not in the world planets response');
      return finish(0);
    }
  } catch (err) {
    report('3/6 tab visible', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  // --- Browser setup ---
  const exe = findExecutable();
  if (!exe) {
    report('setup browser', 'FAIL', 'no Chrome/Edge found');
    return finish(1);
  }
  console.log('browser: ' + exe.name + ' (' + exe.path + ')');
  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('setup browser', 'FAIL', 'launch: ' + String(err && err.message ? err.message : err));
    return finish(1);
  }
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));

  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(500);

    // --- Step 3: tab visible by settlement knowledge ---
    await openMarketTab(page, worldId, worldName, planetIndex);
    const tabVisible = await page.evaluate(() => !!document.querySelector('.tab-btn[data-tab="market"]'));
    report('3/6 tab visible', tabVisible ? 'PASS' : 'FAIL', 'planetIndex=' + planetIndex);
    if (!tabVisible) return finish(1);

    // --- Step 4: 4 offer cards + 3 slot cells ---
    const counts = await page.evaluate(() => ({
      buys: document.querySelectorAll('[data-market-buy]').length,
      slots: document.querySelectorAll('[data-market-slot]').length,
    }));
    const ok4 = counts.buys === 4 && counts.slots === 3;
    report('4/6 offers and slots', ok4 ? 'PASS' : 'FAIL', `offers=${counts.buys} slots=${counts.slots}`);
    if (!ok4) return finish(1);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'market-tab.png') });

    // --- Step 5: slot pick recalculates trade-in (occupied vs empty) ---
    // Default slot "universal" holds the starter cargo module -> trade-in shown;
    // "universal2" is empty -> "без выкупа" and full price.
    await page.click('[data-market-slot="universal"]');
    const occupied = await page.evaluate(() => {
      const t = document.querySelector('#tab-content').textContent;
      return { tradein: t.includes('выкуп старого'), total: t.includes('1500') };
    });
    await page.click('[data-market-slot="universal2"]');
    const empty = await page.evaluate(() => {
      const t = document.querySelector('#tab-content').textContent;
      return { noTradein: t.includes('без выкупа'), full: t.includes('Итого: 3000') };
    });
    const ok5 = occupied.tradein && occupied.total && empty.noTradein && empty.full;
    report('5/6 slot recalc', ok5 ? 'PASS' : 'FAIL',
      `occupied(tradein=${occupied.tradein},total1500=${occupied.total}) ` +
      `empty(noTradein=${empty.noTradein},full3000=${empty.full})`);
    if (!ok5) return finish(1);

    // --- Step 6: buy updates the slot cell ---
    await page.click('[data-market-buy]');
    await page.waitForSelector('.toast-success', { timeout: 10000 });
    const afterBuy = await page.evaluate(() => {
      const cell = document.querySelector('[data-market-slot="universal2"]');
      return {
        text: cell ? cell.textContent : '',
        total: document.querySelector('#tab-content').textContent.includes('выкуп старого'),
      };
    });
    const ok6 = afterBuy.text.includes('выкуп старого') && afterBuy.total;
    report('6/6 buy updates cell', ok6 ? 'PASS' : 'FAIL',
      `cellOccupied=${afterBuy.text.includes('выкуп старого')} pageErrors=${pageErrors.length}`);
    if (!ok6) return finish(1);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'market-buy.png') });

    // --- Gate pass: can_trade == false (player off orbit) ---
    // Same knowledge (tab still visible), but position cleared -> buy disabled.
    try {
      sql(`UPDATE users SET current_position = NULL WHERE id = '${userId}';`);
      await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
      await page.waitForSelector('#mapCanvas', { timeout: 15000 });
      await page.waitForFunction(() => {
        const el = document.getElementById('loading');
        return !el || el.style.display === 'none';
      }, { timeout: 30000 });
      await page.waitForTimeout(500);
      await openMarketTab(page, worldId, worldName, planetIndex);
      const gated = await page.evaluate(() => ({
        note: document.querySelector('#tab-content').textContent.includes('Магазин доступен только с орбиты планеты'),
        disabled: !!document.querySelector('[data-market-buy][disabled]'),
      }));
      report('gate can_trade=false', (gated.note && gated.disabled) ? 'PASS' : 'FAIL',
        `note=${gated.note} disabled=${gated.disabled}`);
      if (!(gated.note && gated.disabled)) return finish(1);
    } catch (err) {
      report('gate can_trade=false', 'FAIL', String(err && err.message ? err.message : err));
      return finish(1);
    }
  } catch (err) {
    report('run', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  } finally {
    // Cleanup: drop the synthetic knowledge row (position already cleared).
    try { sql(`DELETE FROM player_planet_knowledge WHERE user_id = '${userId}';`); } catch (e) { /* best effort */ }
  }

  // A page-level JS error is exactly what this smoke exists to catch: it must
  // fail the run, not just be logged.
  if (pageErrors.length) {
    report('no page errors', 'FAIL', pageErrors.length + ': ' + pageErrors.slice(0, 3).join(' | '));
  }

  const failed = results.filter(r => r.status === 'FAIL');
  console.log('');
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'ALL PASS'));
  return finish(failed.length ? 1 : 0);
}

main();
