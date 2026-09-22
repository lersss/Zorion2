// tools/e2e/portfolio-shots.js
// Portfolio showcase frames for the public repo landing page (idea 2026-09-22
// "витрина проекта на GitHub", stage 2). ONE pass, five REAL in-game frames,
// 1600x900 each, no admin/studio UI, no debug overlays/toasts/tooltips:
//   01-galaxy-map.png  map /map overview (stars, clusters, default graphics)
//   02-system.png      system modal, single "Объекты" section (planets + belts)
//   03-planet.png      planet card with the big v4 planet image (orbit view)
//   04-surface.png     /surface walk: side view, weather, biome, HUD
//   05-contracts.png   planet card "Задания/Контракты" tab (living economy board)
//
// Flow: register a player -> put them in a dev world that has BOTH a belt and a
// soft life planet (SQL, the same setup path surface-walk-check.js uses) -> map
// frame -> own-system modal -> fly to the soft planet (API) -> planet card ->
// right-click "Высадиться" -> surface frame -> back to orbit -> contracts tab.
//
// Idempotency: at the start the script deletes its OWN artifacts from previous
// runs (test accounts /^pf/, their contracts + escrow accounts/knowledge and so
// their orbit positions) through tools/db.ps1 - other players' rows are never
// touched. Two runs in a row therefore produce the same frame numbers: frame 05
// shows exactly 3 contracts and a small stable "Корабли на орбите" count.
//
// Frame 04 acceptance (manager, iteration 2): day phase "День", a "live" biome
// (forest/jungle/steppe/ocean/savanna/lakes/...), and non-muffling weather (no
// dust storm/fog/ash/hail). The landing biome is a server-side random draw per
// landing, so the script re-lands (leave + land via the public API, page reload)
// up to MAX_LAND_TRIES times; no-match is an honest FAIL, never a game-code hack.
//
// Output: docs/media/*.png (repo) + tools/e2e/artifacts/portfolio-shots-results.json.
// Run: node portfolio-shots.js   (BASE_URL env overrides the default; server must be up)
// ASCII console output on purpose (Windows PowerShell cp866 breaks Cyrillic).
//
// Limitation (documented in the report): the contracts board is empty by design
// on the dev DB. To make the "Living economy" frame honest, a SECOND player is
// flown to the same orbit and posts three travel contracts there via the public
// game API - the board then shows offers by another player. That is gameplay,
// not an admin job (AGENTS.md §4.16 unaffected).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ROOT = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(ROOT, '..', '..');
const MEDIA_DIR = path.join(REPO_ROOT, 'docs', 'media');
const ARTIFACTS_DIR = path.join(ROOT, 'artifacts');
const VIEW_W = 1600, VIEW_H = 900;
const PASSWORD = 'pf-shots-' + Date.now();

const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

const results = [];
function report(step, status, detail) {
  results.push({ step, status, detail });
  console.log(`[${step}] ${status}${detail ? ' - ' + detail : ''}`);
}
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}
const sleep = (ms) => new Promise(r => setTimeout(r, ms));

// psqlRun — psql with SQL from a file, tuples-only (PITFALLS: unicode args via
// -c break on Windows; write a .sql file and read stdout). Used for the read-only
// target lookup; writes/cleanup go through tools/db.ps1 (dbRun below).
function psqlRun(sql, filename) {
  const sqlPath = path.join(ARTIFACTS_DIR, filename);
  writeFileSync(sqlPath, sql, 'utf8');
  return String(execFileSync('cmd', ['/c', process.env.PSQL || 'C:\\pgsql\\pgsql\\bin\\psql.exe',
    '-h', '127.0.0.1', '-U', 'zorion', '-d', 'zorion', '-t', '-A', '-f', sqlPath],
    { env: { ...process.env, PGPASSWORD: process.env.PGPASSWORD || 'zorion123' } })).trim();
}

// dbRun — run SQL through the project's tools/db.ps1 wrapper (encoding,
// multi-line SQL, ON_ERROR_STOP=1, PGPASSWORD are already handled inside).
// SQL is passed as a UTF-8 file to survive cp866/cp1251 consoles (PITFALLS).
function dbRun(sql, filename) {
  const sqlPath = path.join(ARTIFACTS_DIR, filename);
  writeFileSync(sqlPath, sql, 'utf8');
  const wrapper = path.join(REPO_ROOT, 'tools', 'db.ps1');
  return String(execFileSync('powershell', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', wrapper, '-File', sqlPath],
    { encoding: 'utf8' }));
}

// cleanupTestArtifacts — make the run idempotent: drop every artifact of the
// PREVIOUS runs of this script, so two runs in a row produce the same frame
// numbers (contracts count, ships on orbit). Scope is strictly this script's own
// test accounts (usernames matching /^pf/, i.e. pf_<ts>_<n> and pfprobe_<ts>_<n>);
// other players' rows are never touched. Order matters: player_planet_knowledge
// has FK NO ACTION to users (000040), so it goes before the users; contracts have
// no FK to users (author_id is a plain string), so they are deleted explicitly by
// author; escrow rows live in accounts/money_operations (no FK).
function cleanupTestArtifacts() {
  const pf = `(SELECT id FROM users WHERE username ~ '^pf')`;
  const sql = `-- portfolio-shots idempotency cleanup (only this script's test accounts)
DELETE FROM money_operations WHERE owner_type = 'player' AND owner_id IN ${pf};
DELETE FROM accounts WHERE owner_type = 'player' AND owner_id IN ${pf};
DELETE FROM contract_log WHERE contract_id IN (SELECT id FROM contracts WHERE author_type = 'player' AND author_id IN ${pf});
DELETE FROM contract_requirements WHERE contract_id IN (SELECT id FROM contracts WHERE author_type = 'player' AND author_id IN ${pf});
DELETE FROM contracts WHERE author_type = 'player' AND author_id IN ${pf};
DELETE FROM player_planet_knowledge WHERE user_id IN ${pf};
DELETE FROM users WHERE username ~ '^pf';
`;
  return dbRun(sql, 'portfolio-cleanup.sql');
}

// resolveTarget — a dev world that has BOTH a belt (so the unified "Объекты"
// section shows planets + belts) and a comfortable "soft" life planet (so the
// surface walk is survivable): >=3 biomes, temperature/pressure in the suit's
// comfort window, life=true. Prefer lush dominants (forest/jungle/lakes/ocean)
// for a readable frame, then lowest radioactivity. Dynamic on purpose: hardcoded
// UUIDs rot when the universe is regenerated (surface-walk-check.js note).
function resolveTarget() {
  const sql = `WITH cand AS (
    SELECT p.world_id, p.id AS planet_id, p.data->>'surface_dominant' AS dom,
           COALESCE((p.data->'core'->>'radioactivity')::float, 999) AS rad
    FROM planets p
    WHERE jsonb_typeof(p.data->'biomes') = 'array'
      AND jsonb_array_length(p.data->'biomes') >= 3
      AND (p.data->>'temperature')::float BETWEEN 263 AND 313
      AND COALESCE((p.data->'atmosphere_data'->>'pressure_atm')::float, 0) BETWEEN 0.5 AND 3.0
      AND COALESCE((p.data->>'life')::bool, false) = true
      AND EXISTS (SELECT 1 FROM system_belts b WHERE b.world_id = p.world_id)
  )
  SELECT world_id, planet_id FROM cand
  ORDER BY CASE dom WHEN 'леса' THEN 0 WHEN 'джунгли' THEN 1 WHEN 'озёра_реки' THEN 2
                    WHEN 'океаны' THEN 3 ELSE 9 END,
           rad ASC
  LIMIT 1;
`;
  const line = psqlRun(sql, 'portfolio-resolve.sql').split(/\r?\n/).map(s => s.trim()).filter(Boolean).pop();
  if (!line) throw new Error('no world with both a belt and a soft life planet');
  const [world, planet] = line.split('|');
  return { world, planet };
}

async function registerAndLogin() {
  for (let i = 0; i < 3; i++) {
    const username = 'pf_' + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password: PASSWORD }),
    });
    if (res.status === 201) { const d = await res.json(); return { username, token: d.token }; }
    if (res.status !== 409) throw new Error('register HTTP ' + res.status + ': ' + (await res.text()));
  }
  throw new Error('register: name collision');
}

// setupPlayerAt — register a fresh player and put them into the dev world via
// SQL (the only sane way to choose a spawn: registration picks a random world).
async function setupPlayerAt(world) {
  const creds = await registerAndLogin();
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + creds.token } })).json();
  psqlRun(`UPDATE users SET current_world_id = '${world}', current_position = NULL WHERE id = '${me.id}';\n`, 'portfolio-setup.sql');
  return { username: creds.username, token: creds.token, id: me.id };
}

// publishBoardContracts — a SECOND player is flown to the same orbit and posts
// travel contracts there, so the board frame shows offers by another player
// (a living economy), not the viewer's own list. Public game API only.
async function publishBoardContracts(world, planetId, destWorld, specs) {
  const merchant = await setupPlayerAt(world);
  const auth = { Authorization: 'Bearer ' + merchant.token };
  const fRes = await fetch(BASE_URL + '/api/intrasystem-flight', {
    method: 'POST', headers: { 'Content-Type': 'application/json', ...auth },
    body: JSON.stringify({ object_type: 'planet', object_id: planetId }),
  });
  if (!fRes.ok) throw new Error('merchant flight HTTP ' + fRes.status + ': ' + (await fRes.text()));
  const f = await fRes.json();
  await sleep(Math.max(0, new Date(f.arrive_at).getTime() - Date.now()) + 2000);
  const published = [];
  for (const s of specs) {
    const res = await fetch(BASE_URL + '/api/contracts', {
      method: 'POST', headers: { 'Content-Type': 'application/json', ...auth },
      body: JSON.stringify({
        planet_id: planetId, type: 'travel', title: s.title, description: s.desc, reward: s.reward,
        payload: { from_world_id: world, dest_world_id: destWorld, dest_planet_id: null },
      }),
    });
    published.push(res.status);
  }
  return { merchant: merchant.username, published };
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  writeFileSync(path.join(ARTIFACTS_DIR, 'portfolio-shots-results.json'), JSON.stringify(results, null, 2));
  process.exit(code);
}

// shot — full-viewport screenshot into docs/media (the frame files).
async function shot(page, file, label) {
  const p = path.join(MEDIA_DIR, file);
  await page.screenshot({ path: p });
  report(label, 'PASS', file);
}

// frameGalaxyView — overview frame: whole galaxy fitted to the screen (zoom
// control's "Галактика" fit = state.minZoom) centred on the region centroid, so
// regions, cluster bubbles and 10000 worlds are all in one shot. Graphics preset
// stays the player default (nothing forced). Returns frame facts for the log.
async function frameGalaxyView(page) {
  const facts = await page.evaluate(async () => {
    const cfg = await import('/static/js/map/config.js');
    const mr = await import('/static/js/map/map_render.js');
    const data = await import('/static/js/map/data.js');
    const st = cfg.state;
    const regions = st.regions || [];
    let cx = 0, cy = 0;
    for (const r of regions) { cx += r.x; cy += r.y; }
    if (regions.length) { cx /= regions.length; cy /= regions.length; }
    st.scale = st.minZoom; // galaxy fit (same value the zoom-out button reaches)
    st.offsetX = st.canvasWidth / 2 - cx * st.scale;
    st.offsetY = st.canvasHeight / 2 - cy * st.scale;
    await data.loadClusters();
    mr.draw();
    return { scale: st.scale, regions: regions.length,
             clusters: (st.clusters || []).length,
             singles: (st.clusters || []).filter(c => c.cnt === 1).length };
  });
  return facts;
}

// openOwnSystem — click the player's own star on the map. Recenters with the 🎯
// button, zooms in so the own star is a single cluster (cnt===1 -> click opens
// the modal instead of zooming a cluster), clicks the canvas centre and waits
// for the objects table. Returns the modal's worldId for a sanity check.
async function openOwnSystem(page) {
  const info = await page.evaluate(async () => {
    const cfg = await import('/static/js/map/config.js');
    const mr = await import('/static/js/map/map_render.js');
    const st = cfg.state;
    // Player's world coords from /me + /worlds (same as map navigation).
    const token = localStorage.getItem('token');
    const auth = { Authorization: 'Bearer ' + token };
    const me = await (await fetch('/me', { headers: auth })).json();
    const w = await (await fetch('/worlds/' + me.current_world_id, { headers: auth })).json();
    const ww = w.world || w;
    const cx = ww.coord_x, cy = ww.coord_y;
    // Deep zoom: neighbours fall outside the 40px cluster cell, own star singled out.
    st.scale = 3;
    st.offsetX = st.canvasWidth / 2 - cx * st.scale;
    st.offsetY = st.canvasHeight / 2 - cy * st.scale;
    await (await import('/static/js/map/data.js')).loadClusters();
    mr.draw();
    const canvas = document.getElementById('mapCanvas');
    const rect = canvas.getBoundingClientRect();
    const clientX = rect.left + (st.canvasWidth / 2) * (rect.width / st.canvasWidth);
    const clientY = rect.top + (st.canvasHeight / 2) * (rect.height / st.canvasHeight);
    return { clientX, clientY, worldId: me.current_world_id, ownSid: ww.id };
  });
  await page.mouse.click(info.clientX, info.clientY);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(2500); // textures + list render
  const state = await page.evaluate(async () => {
    const m = await import('/static/js/modal/state.js');
    const panel = document.getElementById('right-panel');
    const rows = panel ? panel.querySelectorAll('tr[data-index]').length : 0;
    return { worldId: m.modalState.worldId, rows, belts: (m.modalState.belts || []).length, toast: !!document.querySelector('.toast-error') };
  });
  return { ...info, ...state };
}

// openPlanetCard — reopen the modal (fresh my_position), click the row of the
// given planet id, wait for the general tab. Returns the tab buttons present.
async function openPlanetCard(page, planetId, wantOrbitImage) {
  await page.keyboard.press('Escape');
  await page.waitForTimeout(400);
  const info = await page.evaluate(async () => {
    const cfg = await import('/static/js/map/config.js');
    const mr = await import('/static/js/map/map_render.js');
    const st = cfg.state;
    const token = localStorage.getItem('token');
    const auth = { Authorization: 'Bearer ' + token };
    const me = await (await fetch('/me', { headers: auth })).json();
    const w = await (await fetch('/worlds/' + me.current_world_id, { headers: auth })).json();
    const ww = w.world || w;
    st.scale = 3;
    st.offsetX = st.canvasWidth / 2 - ww.coord_x * st.scale;
    st.offsetY = st.canvasHeight / 2 - ww.coord_y * st.scale;
    await (await import('/static/js/map/data.js')).loadClusters();
    mr.draw();
    const canvas = document.getElementById('mapCanvas');
    const rect = canvas.getBoundingClientRect();
    return {
      clientX: rect.left + (st.canvasWidth / 2) * (rect.width / st.canvasWidth),
      clientY: rect.top + (st.canvasHeight / 2) * (rect.height / st.canvasHeight),
    };
  });
  await page.mouse.click(info.clientX, info.clientY);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(2000);
  const res = await page.evaluate(async (pid) => {
    const m = await import('/static/js/modal/state.js');
    const panel = document.getElementById('right-panel');
    const planets = m.modalState.planets || [];
    const idx = planets.findIndex(p => p.id === pid);
    if (idx < 0) return { ok: false, reason: 'planet not in modal (' + planets.length + ')' };
    const tr = panel.querySelector('tr[data-index="' + idx + '"]') || panel.querySelector('tr[data-index]');
    if (!tr) return { ok: false, reason: 'no planet row' };
    tr.click();
    await new Promise(r => setTimeout(r, 1500));
    const tabs = [...panel.querySelectorAll('.tab-btn')].map(b => b.dataset.tab);
    return { ok: true, tabs, toast: !!document.querySelector('.toast-error') };
  }, planetId);
  if (wantOrbitImage && res.ok) {
    // Orbit view image: wait until the big PNG is decoded (up to ~12s).
    await page.waitForFunction(() => {
      const img = document.querySelector('.orbit-view-body img');
      return !!img && img.complete && img.naturalWidth > 0;
    }, { timeout: 12000 }).catch(() => {});
  }
  return res;
}

// landOnPlanet — right-click the planet on the modal canvas, click "Высадиться",
// wait for the surface page + briefing, continue to the walk. Mirrors the real
// context-menu path used by surface-walk-check.js (layout.computeLayout).
async function landOnPlanet(page, planetId) {
  const prep = await page.evaluate(async (softId) => {
    const layout = await import('/static/js/modal/layout.js');
    const m = await import('/static/js/modal/state.js');
    const planets = m.modalState.planets || [];
    const idx = planets.findIndex(p => p.id === softId);
    if (idx < 0) return { ok: false, reason: 'planet not in modal' };
    const w = m.modalState.canvasWidth, h = m.modalState.canvasHeight;
    const l = layout.computeLayout(planets, m.modalState.starRadius, w, h);
    const pose = layout.getPlanetPose(l, planets[idx], idx, performance.now());
    const canvas = document.querySelector('#system-modal-overlay canvas');
    const rect = canvas.getBoundingClientRect();
    const mx = pose.x * m.modalState.zoom + m.modalState.offsetX + (m.modalState.followOffsetX || 0);
    const my = pose.y * m.modalState.zoom + m.modalState.offsetY + (m.modalState.followOffsetY || 0);
    canvas.dispatchEvent(new MouseEvent('contextmenu', {
      clientX: rect.left + mx * (rect.width / w), clientY: rect.top + my * (rect.height / h),
      bubbles: true, cancelable: true, button: 2,
    }));
    await new Promise(r => setTimeout(r, 300));
    const menu = document.getElementById('star-context-menu');
    const item = menu ? [...menu.querySelectorAll('div')].find(d => d.textContent.includes('Высадиться')) : null;
    return { ok: !!item, menuText: menu ? menu.textContent.replace(/\s+/g, ' ').trim() : '' };
  }, planetId);
  if (!prep.ok) return prep;

  const navPromise = page.waitForURL('**/surface.html*', { timeout: 20000 });
  await page.evaluate(() => {
    const menu = document.getElementById('star-context-menu');
    const item = [...menu.querySelectorAll('div')].find(d => d.textContent.includes('Высадиться'));
    item.click();
  });
  await navPromise;
  await page.waitForSelector('#briefing', { state: 'visible', timeout: 25000 });
  await page.waitForTimeout(500);
  await page.click('#brief-continue');
  await page.waitForSelector('#hud', { state: 'visible', timeout: 15000 });
  // Wait out the preloader if it lingers (surface-walk-check blocker note).
  await page.waitForFunction(() => {
    const el = document.getElementById('loading');
    return !el || getComputedStyle(el).display === 'none';
  }, { timeout: 8000 }).catch(() => {});
  return { ok: true };
}

// readSurfaceHud — HUD facts of the walk (biome / day phase / weather, plus
// overlay checks). The landing biome (hence the surface seed -> day phase) is
// RANDOM per landing (surface_handlers.pickWalkBiome), so frame 04 retries
// landings until the walk matches the acceptance rules below.
async function readSurfaceHud(page) {
  return page.evaluate(() => {
    const t = (id) => (document.getElementById(id) ? document.getElementById(id).textContent.replace(/\s+/g, ' ').trim() : '');
    return {
      biome: t('hud-biome'), env: t('hud-env'), weather: t('hud-weather'),
      loadingVisible: (() => { const el = document.getElementById('loading'); return !!el && getComputedStyle(el).display !== 'none'; })(),
      hud: (() => { const el = document.getElementById('hud'); return !!el && el.style.display !== 'none'; })(),
      adminWeatherRow: !!document.getElementById('hud-weather-admin'),
      toast: (() => { const el = document.getElementById('toast'); return !!el && el.style.display !== 'none'; })(),
    };
  });
}

// Frame-04 acceptance rules (manager, iteration 2):
//   time = "День"; weather not muffling (no dust storm / fog / ash / hail);
//   biome is "alive" (forest / jungle / steppe / ocean / savanna / lakes / ...).
// Only the HUD labels are read - no game code is touched.
const isDay = (env) => env === 'День';
const isMufflingWeather = (w) => /туман|мгл|бур|пепел|град|круп|игл|метель/i.test(w || '');
const isLiveBiome = (b) => /лес|джунгл|степ|луг|океан|саванн|озёр|озер|рек|болот|риф/i.test(b || '');
const landingAccepted = (s) => isDay(s.env) && !isMufflingWeather(s.weather) && isLiveBiome(s.biome);

// leaveAndLand — return to orbit and land again via the public API (the same
// two calls the surface client makes). A fresh land re-rolls the biome, hence
// the day phase and weather. Used for frame-04 retries (the very first landing
// still goes through the real "Высадиться" menu in landOnPlanet).
async function leaveAndLand(page, planetId) {
  await page.evaluate(async (pid) => {
    const t = localStorage.getItem('token');
    const h = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + t };
    await fetch('/api/surface/leave', { method: 'POST', headers: h });
    await fetch('/api/surface/land', { method: 'POST', headers: h, body: JSON.stringify({ planet_id: pid }) });
  }, planetId);
}

// reloadSurfaceWalk — reload /surface.html for the current position and reach the
// walk again (briefing -> Continue -> HUD, preloader waited out).
async function reloadSurfaceWalk(page) {
  await page.reload({ waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#surface-canvas', { timeout: 15000 });
  const brief = await page.waitForSelector('#briefing', { state: 'visible', timeout: 6000 })
    .then(() => true).catch(() => false);
  if (brief) await page.click('#brief-continue').catch(() => {});
  await page.waitForSelector('#hud', { state: 'visible', timeout: 15000 });
  await page.waitForFunction(() => {
    const el = document.getElementById('loading');
    return !el || getComputedStyle(el).display === 'none';
  }, { timeout: 8000 }).catch(() => {});
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  mkdirSync(MEDIA_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL + '  viewport: ' + VIEW_W + 'x' + VIEW_H);

  // --- Step 0: cleanup of previous runs, then player + target world ---
  let creds, target;
  try {
    cleanupTestArtifacts();
    target = resolveTarget();
    creds = await setupPlayerAt(target.world);
    report('0 setup', 'PASS', 'user=' + creds.username + ' world=' + target.world + ' planet=' + target.planet);
  } catch (err) {
    report('0 setup', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const exe = findExecutable();
  if (!exe) { report('0 setup', 'FAIL', 'no Chrome/Edge found'); return finish(1); }
  console.log('browser: ' + exe.name + ' (' + exe.path + ')');

  browser = await chromium.launch({ executablePath: exe.path, headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: VIEW_W, height: VIEW_H } });
  await context.addInitScript((t) => { try { localStorage.setItem('token', t); } catch (e) {} }, creds.token);
  const page = await context.newPage();

  const pageErrors = [];
  const consoleErrors = [];
  const badResponses = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  page.on('response', (r) => { if (r.status() >= 400) badResponses.push(r.status() + ' ' + r.url()); });

  try {
    // =====================================================================
    // Frame 1: galaxy map. Whole-galaxy overview (zoom fit) so regions, star
    // clusters and the 10000-world scale read at a glance. Default graphics
    // preset - nothing forced.
    // =====================================================================
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1800); // regions + stars ignite
    const mapFacts = await frameGalaxyView(page);
    await page.mouse.move(0, 0);     // no hover tooltip in frame
    await page.waitForTimeout(1600);
    await shot(page, '01-galaxy-map.png', '1/5 galaxy map');
    console.log('  map: scale=' + mapFacts.scale.toFixed(5) + ' regions=' + mapFacts.regions +
      ' clusters=' + mapFacts.clusters + ' singles=' + mapFacts.singles);
    report('1/5 galaxy map', mapFacts.clusters > 100 ? 'PASS' : 'FAIL',
      `clusters=${mapFacts.clusters} singles=${mapFacts.singles} regions=${mapFacts.regions}`);

    // =====================================================================
    // Frame 2: own system modal - single "Объекты" section (planets + belts).
    // =====================================================================
    const sys = await openOwnSystem(page);
    const ok2 = sys.rows > 0 && !sys.toast && (!sys.worldId || sys.worldId === sys.ownSid);
    report('2/5 system modal', ok2 ? 'PASS' : 'FAIL',
      `rows=${sys.rows} belts=${sys.belts} worldId=${sys.worldId} own=${sys.ownSid} toast=${sys.toast}`);
    if (!ok2) return finish(1);
    await page.mouse.move(0, 0);
    await page.waitForTimeout(300);
    await shot(page, '02-system.png', '2/5 system modal');
    console.log('  system: rows=' + sys.rows + ' belts=' + sys.belts);

    // =====================================================================
    // Fly to the soft planet's orbit (API). Then a SECOND player ("merchant")
    // is flown to the same orbit and posts three travel contracts there, so the
    // board frame reads as a living economy. Public game API, not an admin job.
    // =====================================================================
    const fly = await page.evaluate(async ({ baseUrl, planetId }) => {
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };
      const fRes = await fetch(baseUrl + '/api/intrasystem-flight', {
        method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
        body: JSON.stringify({ object_type: 'planet', object_id: planetId }),
      });
      if (!fRes.ok) return { ok: false, reason: 'flight HTTP ' + fRes.status + ': ' + (await fRes.text()) };
      const f = await fRes.json();
      return { ok: true, arrive_at: f.arrive_at };
    }, { baseUrl: BASE_URL, planetId: target.planet });
    if (!fly.ok) { report('3/5 fly to orbit', 'FAIL', fly.reason); return finish(1); }
    await page.waitForTimeout(Math.max(0, new Date(fly.arrive_at).getTime() - Date.now()) + 2500);

    const orbit = await page.evaluate(async ({ baseUrl, planetId }) => {
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };
      const me = await (await fetch(baseUrl + '/me', { headers: auth })).json();
      const pos = me.current_position || {};
      // A real neighbouring system as the contract destination (longer window).
      let dest = me.current_world_id;
      try {
        const w = await (await fetch(baseUrl + '/worlds/' + me.current_world_id, { headers: auth })).json();
        const ww = w.world || w;
        const R = 400;
        const p = new URLSearchParams({
          x_min: (ww.coord_x - R).toFixed(1), x_max: (ww.coord_x + R).toFixed(1),
          y_min: (ww.coord_y - R).toFixed(1), y_max: (ww.coord_y + R).toFixed(1), cell: '40',
        });
        const cl = await (await fetch(baseUrl + '/api/worlds/filter?' + p, { headers: auth })).json();
        const near = cl.filter(c => c.cnt === 1 && c.sid && c.sid !== me.current_world_id);
        if (near.length) dest = near[near.length - 1].sid; // farthest in range
      } catch (e) { /* keep own world */ }
      return { orbit: pos.status === 'orbit' && pos.object_id === planetId, pos, dest };
    }, { baseUrl: BASE_URL, planetId: target.planet });

    const specs = [
      { title: 'Доставка продовольствия', desc: 'Провизия для колонии — срочно.', reward: 850 },
      { title: 'Разведка пояса', desc: 'Свежие данные о составе кольца.', reward: 1400 },
      { title: 'Пассажирский рейс', desc: 'Инженер и оборудование на борту.', reward: 2200 },
    ];
    let board = { merchant: '-', published: [] };
    try {
      board = await publishBoardContracts(target.world, target.planet, orbit.dest, specs);
    } catch (err) {
      console.log('  merchant publish FAILED: ' + (err && err.message ? err.message : err));
    }
    report('3/5 fly to orbit', orbit.orbit ? 'PASS' : 'FAIL',
      `pos=${JSON.stringify(orbit.pos)} dest=${orbit.dest} merchant=${board.merchant} published=[${board.published.join(',')}]`);

    // =====================================================================
    // Frame 3: planet card with the big v4 planet image (orbit view).
    // =====================================================================
    const card = await openPlanetCard(page, target.planet, true);
    const ok3 = card.ok && card.tabs.includes('general') && !card.toast;
    report('4/5 planet card', ok3 ? 'PASS' : 'FAIL', card.ok ? `tabs=[${card.tabs.join(',')}] toast=${card.toast}` : card.reason);
    if (!ok3) return finish(1);
    await page.mouse.move(0, 0);
    await page.waitForTimeout(300);
    await shot(page, '03-planet.png', '3/5 planet card');

    // =====================================================================
    // Frame 4: surface walk. First landing goes through the real "Высадиться"
    // menu; if it does not match the acceptance rules (День + живой биом +
    // non-muffling weather), we re-land via the public API and reload the page,
    // bounded to MAX_LAND_TRIES attempts. If nothing matches - honest FAIL (no
    // game-code hacks, per the manager's instruction).
    // =====================================================================
    const MAX_LAND_TRIES = 25;
    let land = null, surf = null, attempts = 0;
    for (let attempt = 1; attempt <= MAX_LAND_TRIES; attempt++) {
      attempts = attempt;
      if (attempt === 1) {
        land = await landOnPlanet(page, target.planet);
        if (!land.ok) break;
      } else {
        await leaveAndLand(page, target.planet);
        await reloadSurfaceWalk(page);
      }
      await page.waitForTimeout(1600);
      surf = await readSurfaceHud(page);
      const ok = landingAccepted(surf);
      console.log('  landing ' + attempt + ': ' + surf.biome + ' · ' + surf.env + ' · ' + surf.weather +
        ' -> ' + (ok ? 'ACCEPT' : 'retry'));
      if (ok) break;
    }
    if (!land || !land.ok) {
      report('5/5 surface walk', 'FAIL', (land && land.reason) ? land.reason : JSON.stringify(land));
      return finish(1);
    }
    if (!landingAccepted(surf)) {
      report('5/5 surface walk', 'FAIL',
        `no accepted landing in ${attempts} tries (last: ${surf.biome} · ${surf.env} · ${surf.weather})`);
      return finish(1);
    }
    await page.waitForTimeout(600);
    await page.keyboard.down('ArrowRight');
    await page.waitForTimeout(1200);
    await page.keyboard.up('ArrowRight');
    await page.waitForTimeout(1200);
    surf = await readSurfaceHud(page);
    const ok4 = surf.hud && !surf.loadingVisible && !surf.adminWeatherRow && !surf.toast && landingAccepted(surf);
    report('5/5 surface walk', ok4 ? 'PASS' : 'FAIL',
      `tries=${attempts} biome="${surf.biome}" env="${surf.env}" weather="${surf.weather}" ` +
      `hud=${surf.hud} loading=${surf.loadingVisible} adminWeatherRow=${surf.adminWeatherRow} toast=${surf.toast}`);
    if (!ok4) return finish(1);
    await shot(page, '04-surface.png', '4/5 surface walk');

    // =====================================================================
    // Frame 5: contracts board on the planet card ("Задания/Контракты" tab).
    // Back to orbit (Вызвать корабль -> /map), reopen the card, open the tab.
    // =====================================================================
    await page.click('#ship-call-btn');
    await page.waitForURL('**/map**', { timeout: 15000 }).catch(() => {});
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1500);

    const card2 = await openPlanetCard(page, target.planet, false);
    if (!card2.ok) { report('6/5 contracts board', 'FAIL', card2.reason); return finish(1); }
    const boardFrame = await page.evaluate(async () => {
      const panel = document.getElementById('right-panel');
      const btn = panel.querySelector('.tab-btn[data-tab="contracts"]');
      if (!btn) return { ok: false, reason: 'no contracts tab' };
      btn.click();
      let rows = 0;
      for (let i = 0; i < 40 && rows === 0; i++) {
        await new Promise(r => setTimeout(r, 200));
        rows = panel.querySelectorAll('[data-contract-take]').length;
      }
      const content = panel.querySelector('#tab-content');
      const text = content ? content.textContent : '';
      // Ships-on-orbit line + board header count, for the idempotency check.
      const shipsEl = [...panel.querySelectorAll('div')].find(d => /Корабли на орбите/.test(d.textContent));
      const shipsMatch = shipsEl ? shipsEl.textContent.match(/Корабли на орбите:\s*(\d+)/) : null;
      const headMatch = text.match(/КОНТРАКТЫ\s*\((\d+)\)/i);
      return {
        ok: true, rows, hasPublish: !!content.querySelector('[data-contract-publish]'),
        noData: text.includes('Нет данных'), toast: !!document.querySelector('.toast-error'),
        ships: shipsMatch ? Number(shipsMatch[1]) : 0,
        boardCount: headMatch ? Number(headMatch[1]) : rows,
      };
    });
    const ok5 = boardFrame.ok && boardFrame.rows > 0 && !boardFrame.toast && !boardFrame.noData;
    report('6/5 contracts board', ok5 ? 'PASS' : 'FAIL',
      boardFrame.ok ? `contracts=${boardFrame.boardCount} ships=${boardFrame.ships} publish=${boardFrame.hasPublish} noData=${boardFrame.noData} toast=${boardFrame.toast}` : boardFrame.reason);
    if (!ok5) return finish(1);
    await page.mouse.move(0, 0);
    await page.waitForTimeout(400);
    await shot(page, '05-contracts.png', '5/5 contracts board');

    // --- hygiene ---
    const jsErrors = consoleErrors.filter(t => !/Failed to load resource/i.test(t));
    const onlyFavicon = badResponses.every(b => b.includes('favicon.ico'));
    const ok6 = pageErrors.length === 0 && jsErrors.length === 0 && onlyFavicon;
    report('7/5 hygiene', ok6 ? 'PASS' : 'FAIL',
      `pageErrors=${pageErrors.length} consoleErrors=${jsErrors.length} bad=[${badResponses.join(' | ')}]` +
      (pageErrors.length ? ' first=' + pageErrors[0] : ''));
  } catch (err) {
    report('run', 'FAIL', String(err && err.message ? err.message : err) + '\n' + (err && err.stack ? err.stack : ''));
    return finish(1);
  }

  const failed = results.filter(r => r.status === 'FAIL');
  console.log('');
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'ALL PASS'));
  return finish(failed.length ? 1 : 0);
}

main();
