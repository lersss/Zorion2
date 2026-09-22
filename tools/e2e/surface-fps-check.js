// tools/e2e/surface-fps-check.js
// Independent @tester check for idea 2026-09-21 §4 point 4: chunk cache cost
// while sprinting. Lands a real user on a live soft planet, holds sprint+right
// for 2.5 s and measures rAF frame pacing (fps + long frames / hitches) while
// the terrain scrolls and new chunks are generated.
// NOTE: headless Chrome is not the creator's GPU; only frame *drop/hitch*
// signal is meaningful, not the absolute fps.
// Run: node surface-fps-check.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const PASSWORD = 'e2e-fps-' + Date.now();

const results = [];
function report(step, ok, detail) { results.push({ step, ok, detail }); console.log(`[${step}] ${ok ? 'PASS' : 'FAIL'} - ${detail}`); }
function findExecutable() { for (const p of CHROME_PATHS) if (existsSync(p)) return p; for (const p of EDGE_PATHS) if (existsSync(p)) return p; return null; }
const sleep = (ms) => new Promise(r => setTimeout(r, ms));

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
  const line = psqlRun(sql, 'surface-fps-resolve.sql').split(/\r?\n/).map((s) => s.trim()).filter(Boolean).pop();
  if (!line) throw new Error('no live soft planet');
  const parts = line.split('|');
  return { world: parts[0], planet: parts[1] };
}

async function setupUser(world, planet) {
  let token = null, username = null;
  for (let i = 0; i < 3; i++) {
    username = 'e2e_fps_' + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
    if (res.status === 201) { token = (await res.json()).token; break; }
    if (res.status !== 409) throw new Error('register HTTP ' + res.status);
  }
  if (!token) throw new Error('register failed');
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  psqlRun(`UPDATE users SET current_world_id='${world}', current_position=NULL WHERE id='${me.id}';\n`, 'surface-fps-setup.sql');
  const f = await fetch(BASE_URL + '/api/intrasystem-flight', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token }, body: JSON.stringify({ object_type: 'planet', object_id: planet }) });
  const fj = await f.json();
  await sleep(Math.max(0, (fj.arrive_at || 0) - Date.now()) + 2000);
  const me2 = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  if (!me2.current_position || me2.current_position.object_id !== planet) throw new Error('not on orbit');
  return { token, username };
}

let browser = null;
async function finish(code) { if (browser) await browser.close().catch(() => {}); writeFileSync(path.join(ARTIFACTS_DIR, 'surface-fps-results.json'), JSON.stringify(results, null, 2)); process.exit(code); }

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER FOUND'); await finish(1); }
  const { world, planet } = resolveSoftPlanet();
  console.log('world/planet: ' + world + ' / ' + planet);
  const user = await setupUser(world, planet);
  console.log('user: ' + user.username);
  browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { try { localStorage.setItem('token', t); } catch (e) {} }, user.token);
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  try {
    await page.goto(`${BASE_URL}/surface.html?planet=${planet}`, { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#briefing', { state: 'visible', timeout: 25000 });
    await page.click('#brief-continue');
    await page.waitForSelector('#hud', { state: 'visible', timeout: 10000 });
    await page.waitForTimeout(1200);

    // Start a frame sampler in the page.
    await page.evaluate(() => {
      window.__fp = { n: 0, t0: performance.now(), long: 0, last: performance.now(), maxGap: 0 };
      const loop = () => {
        const now = performance.now();
        const gap = now - window.__fp.last; window.__fp.last = now;
        if (gap > 22) window.__fp.long++;
        if (gap > window.__fp.maxGap) window.__fp.maxGap = gap;
        window.__fp.n++;
        requestAnimationFrame(loop);
      };
      requestAnimationFrame(loop);
    });
    await page.keyboard.down('ArrowRight');
    await page.keyboard.down('ShiftLeft');
    await page.waitForTimeout(2500);
    await page.keyboard.up('ShiftLeft');
    await page.keyboard.up('ArrowRight');
    const fp = await page.evaluate(() => {
      const d = (performance.now() - window.__fp.t0) / 1000;
      return { fps: window.__fp.n / d, frames: window.__fp.n, long: window.__fp.long, maxGap: window.__fp.maxGap };
    });
    const dist = await page.evaluate(() => document.getElementById('hud-distance').textContent);
    report('F1 sprint: no long-frame hitches (>22 ms)',
      fp.long === 0,
      `fps=${fp.fps.toFixed(1)} frames=${fp.frames} longFrames=${fp.long} maxGap=${fp.maxGap.toFixed(1)}ms dist=${dist}`);
    report('F2 sprint moved the player (chunks streamed)', parseInt(dist, 10) > 5, 'distance="' + dist + '"');
    report('F3 no JS errors in sprint', errors.length === 0, errors.slice(0, 2).join(' | '));
    // cleanup: leave so the temp user is off-planet
    await page.click('#ship-call-btn').catch(() => {});
    const failed = results.filter(r => !r.ok).length;
    console.log('SUMMARY: ' + results.filter(r => r.ok).length + ' PASS / ' + failed + ' FAIL');
    await finish(failed ? 1 : 0);
  } catch (e) {
    console.log('EXCEPTION: ' + e.message + '\n' + e.stack);
    await finish(1);
  }
}
main();
