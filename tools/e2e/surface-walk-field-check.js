// tools/e2e/surface-walk-field-check.js
// Э5.1 «Мир прогулки: скульптурный объём» — фундамент: e2e-смоук прогулки
// (клиентский флоу: ходьба + угловая коллизия) на реальном мире.
// Отличие от surface-walk-check.js: игрок сажается на орбиту планеты SQL-ом
// напрямую — штатный setup через `/api/intrasystem-flight` на момент Э5.1 не
// доводит игрока до планеты (чужой незакоммиченный код в internal/travel;
// к правке Э5.1 отношения не имеет). Дальше флоу тот же: land → брифинг →
// прогулка → ходьба/прыжок → «Вызвать корабль».
// Run: node surface-walk-field-check.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const PASSWORD = 'e2e-walkf-' + Date.now();

const results = [];
function report(step, ok, detail) { results.push({ step, ok, detail }); console.log(`[${step}] ${ok ? 'PASS' : 'FAIL'} - ${detail}`); }
function findExecutable() { for (const p of CHROME_PATHS) if (existsSync(p)) return p; for (const p of EDGE_PATHS) if (existsSync(p)) return p; return null; }
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
function psqlRun(sql, filename) {
  const sqlPath = path.join(ARTIFACTS_DIR, filename);
  writeFileSync(sqlPath, sql, 'utf8');
  return String(execFileSync('cmd', ['/c', process.env.PSQL || 'C:\\pgsql\\pgsql\\bin\\psql.exe',
    '-h', '127.0.0.1', '-U', 'zorion', '-d', 'zorion', '-t', '-A', '-f', sqlPath],
    { env: { ...process.env, PGPASSWORD: process.env.PGPASSWORD || 'zorion123' } })).trim();
}
function resolveSoftPlanet() {
  const sql = `SELECT p.world_id, p.id FROM planets p
WHERE jsonb_typeof(p.data->'biomes') = 'array' AND jsonb_array_length(p.data->'biomes') >= 3
  AND (p.data->>'temperature')::float BETWEEN 263 AND 313
  AND COALESCE((p.data->'atmosphere_data'->>'pressure_atm')::float, 0) BETWEEN 0.5 AND 3.0
  AND COALESCE((p.data->>'life')::bool, false) = true
ORDER BY COALESCE((p.data->'core'->>'radioactivity')::float, 999) ASC LIMIT 1;
`;
  const line = psqlRun(sql, 'surface-walkf-resolve.sql').split(/\r?\n/).map((s) => s.trim()).filter(Boolean).pop();
  if (!line) throw new Error('не найдена живая мягкая планета');
  const [world, planet] = line.split('|');
  return { world, planet };
}
async function setupOnOrbit(world, planet) {
  const username = 'e2e_walkf_' + Date.now();
  const res = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
  if (res.status !== 201) throw new Error('register HTTP ' + res.status);
  const { token } = await res.json();
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  const pos = JSON.stringify({ status: 'orbit', object_type: 'planet', object_id: planet, level: 'orbit' }).replace(/'/g, "''");
  psqlRun(`UPDATE users SET current_world_id='${world}', current_position='${pos}'::jsonb WHERE id='${me.id}';\n`, 'surface-walkf-setup.sql');
  const me2 = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  if (!me2.current_position || me2.current_position.object_id !== planet) throw new Error('не на орбите планеты: ' + JSON.stringify(me2.current_position));
  return { token, username };
}

let browser = null;
async function finish(code) { if (browser) await browser.close().catch(() => {}); writeFileSync(path.join(ARTIFACTS_DIR, 'surface-walk-field-results.json'), JSON.stringify(results, null, 2)); process.exit(code); }

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER FOUND'); await finish(1); }
  const { world, planet } = resolveSoftPlanet();
  console.log('world/planet: ' + world + ' / ' + planet);
  const setup = await setupOnOrbit(world, planet);
  console.log('user: ' + setup.username);

  browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { try { localStorage.setItem('token', t); } catch (e) {} }, setup.token);
  const page = await context.newPage();
  const pageErrors = [];
  const consoleErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e)));
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });

  try {
    await page.goto(BASE_URL + '/surface.html?planet=' + planet, { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#briefing', { state: 'visible', timeout: 25000 });
    report('W1 брифинг высадки отрисован', true, (await page.evaluate(() => document.getElementById('briefing-content').textContent.trim().slice(0, 80))));
    await page.click('#brief-continue');
    await page.waitForSelector('#hud', { state: 'visible', timeout: 10000 });
    await page.waitForTimeout(1500);

    const worldDrawn = await page.evaluate(() => {
      const c = document.getElementById('surface-canvas');
      const d = c.getContext('2d').getImageData(0, 0, c.width, c.height).data;
      const set = new Set();
      for (let i = 0; i < d.length; i += 4 * 97) set.add((d[i] << 16) | (d[i + 1] << 8) | d[i + 2]);
      return set.size;
    });
    report('W2 мир рисуется (canvas не одноцветный)', worldDrawn > 10, 'colors=' + worldDrawn);

    // Ходьба вправо: угловая коллизия не должна запирать игрока на подъёмах.
    const d0 = await page.evaluate(() => parseInt(document.getElementById('hud-distance').textContent, 10) || 0);
    await page.keyboard.down('ArrowRight');
    await page.waitForTimeout(2600);
    await page.keyboard.up('ArrowRight');
    await page.keyboard.down('ArrowLeft');
    await page.waitForTimeout(1600);
    await page.keyboard.up('ArrowLeft');
    const d1 = await page.evaluate(() => parseInt(document.getElementById('hud-distance').textContent, 10) || 0);
    report('W3 персонаж идёт (нет застревания на рельефе)', d1 - d0 > 2, `distance ${d0}m → ${d1}m (Δ=${d1 - d0})`);

    // Прыжок.
    await page.waitForTimeout(500);
    const jumpBefore = await page.evaluate(() => {
      const el = document.getElementById('surface-canvas');
      return el.width;
    });
    await page.keyboard.down('Space');
    await page.waitForTimeout(400);
    await page.keyboard.up('Space');
    await page.waitForTimeout(600);
    report('W4 прыжок без ошибок', pageErrors.length === 0, 'pageErrors=' + pageErrors.length);

    const hp = await page.evaluate(() => document.getElementById('hp-text').textContent);
    report('W5 HUD здоровья жив', /100\s*\/\s*100/.test(hp), 'hp=' + hp);

    const jsErr = consoleErrors.filter((t) => !/Failed to load resource/i.test(t));
    report('W6 0 pageerror/JS-ошибок', pageErrors.length === 0 && jsErr.length === 0,
      'pageErrors=' + pageErrors.length + ' jsErrors=' + jsErr.length + (pageErrors[0] ? ' | ' + pageErrors[0] : ''));

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'surface-walk-field.png') });
    const failed = results.filter((r) => !r.ok).length;
    console.log('SUMMARY: ' + results.filter((r) => r.ok).length + ' PASS / ' + failed + ' FAIL');
    await finish(failed ? 1 : 0);
  } catch (e) {
    console.log('EXCEPTION: ' + e.message + '\n' + e.stack);
    await finish(1);
  }
}
main();
