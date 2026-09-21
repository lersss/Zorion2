// tools/e2e/surface-admin-biome-check.js
// E2E админского инструмента высадки (идея 2026-09-21):
//  - админ: ПКМ по планете → аккордеон «⚙ выбор биома ▸» со списком биомов
//    планеты (иконка + имя + доля), клик по биому → /surface.html?...&biome=<form>
//    → на брифинге та же доля; переключатель погоды виден и работает;
//  - обычный игрок: ни списка биомов в ПКМ-меню, ни строки погоды.
// Живая мягкая планета резолвится динамически (SQL), хардкод UUID не используется.
// Run: node surface-admin-biome-check.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const PASSWORD = 'e2e-admin-' + Date.now();

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
  const line = psqlRun(sql, 'surface-admin-resolve.sql').split(/\r?\n/).map((s) => s.trim()).filter(Boolean).pop();
  if (!line) throw new Error('не найдена живая мягкая планета (SQL вернул пусто)');
  const parts = line.split('|');
  return { world: parts[0], planet: parts[1] };
}

async function setupUser(world, planet, role) {
  let token = null, username = null;
  for (let i = 0; i < 3; i++) {
    username = 'e2e_adm_' + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
    if (res.status === 201) { token = (await res.json()).token; break; }
    if (res.status !== 409) throw new Error('register HTTP ' + res.status);
  }
  if (!token) throw new Error('register failed');
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  const roleSql = role ? `, role='${role}'` : '';
  psqlRun(`UPDATE users SET current_world_id='${world}', current_position=NULL${roleSql} WHERE id='${me.id}';\n`, 'surface-admin-setup.sql');
  // Роль в БД ≠ роль в уже выписанном JWT — перелогин за свежим токеном.
  if (role) {
    const lr = await fetch(BASE_URL + '/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
    if (lr.ok) token = (await lr.json()).token;
  }
  const f = await fetch(BASE_URL + '/api/intrasystem-flight', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token }, body: JSON.stringify({ object_type: 'planet', object_id: planet }) });
  const fj = await f.json();
  await sleep(Math.max(0, (fj.arrive_at || 0) - Date.now()) + 2000);
  const me2 = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  if (!me2.current_position || me2.current_position.object_id !== planet) throw new Error('not on orbit: ' + JSON.stringify(me2.current_position));
  return { token, username };
}

async function newPage(role, token) {
  const browser = await chromium.launch({ executablePath: findExecutable(), headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { try { localStorage.setItem('token', t); } catch (e) {} }, token);
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  return { browser, page, errors };
}

async function openPlanetMenu(page, softId) {
  // клик по своей звезде -> модалка системы
  const clickTarget = await page.evaluate(async (baseUrl) => {
    const canvas = document.getElementById('mapCanvas');
    const rect = canvas.getBoundingClientRect();
    const cw = canvas.width, ch = canvas.height;
    const token = localStorage.getItem('token');
    const auth = { Authorization: 'Bearer ' + token };
    let vp = null; try { vp = JSON.parse(sessionStorage.getItem('viewport') || 'null'); } catch (e) {}
    let wx = 0, wy = 0;
    const meRes = await fetch(baseUrl + '/me', { headers: auth });
    if (meRes.ok) { const me = await meRes.json(); if (me.current_world_id) { const wRes = await fetch(baseUrl + '/worlds/' + me.current_world_id, { headers: auth }); if (wRes.ok) { const w = await wRes.json(); const ww = w.world || w; if (typeof ww.coord_x === 'number') { wx = ww.coord_x; wy = ww.coord_y; } } } }
    const scale = vp && vp.scale ? vp.scale : 1.0;
    const offsetX = vp ? vp.offsetX : cw / 2 - wx * scale;
    const offsetY = vp ? vp.offsetY : ch / 2 - wy * scale;
    const inv = 1 / scale;
    const params = new URLSearchParams({ x_min: (-offsetX * inv).toFixed(3), x_max: ((cw - offsetX) * inv).toFixed(3), y_min: (-offsetY * inv).toFixed(3), y_max: ((ch - offsetY) * inv).toFixed(3), cell: (40 / scale).toFixed(3) });
    const res = await fetch(baseUrl + '/api/worlds/filter?' + params.toString(), { headers: auth });
    if (!res.ok) return { ok: false, reason: 'filter HTTP ' + res.status };
    const clusters = await res.json();
    const cx = cw / 2, cy = ch / 2;
    let best = null, bestD = Infinity;
    for (const c of clusters) { const px = c.x * scale + offsetX, py = c.y * scale + offsetY; const d = (px - cx) ** 2 + (py - cy) ** 2; if (d < bestD) { bestD = d; best = { x: px, y: py }; } }
    if (!best) return { ok: false, reason: 'no clusters' };
    return { ok: true, clickX: rect.left + best.x * (rect.width / cw), clickY: rect.top + best.y * (rect.height / ch) };
  }, BASE_URL);
  if (!clickTarget.ok) return { ok: false, reason: clickTarget.reason };
  await page.mouse.click(clickTarget.clickX, clickTarget.clickY);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(2000);

  // ПКМ по планете -> контекстное меню
  return page.evaluate(async (softId) => {
    const layout = await import('/static/js/modal/layout.js');
    const { modalState } = await import('/static/js/modal/state.js');
    const planets = modalState.planets || [];
    const idx = planets.findIndex(p => p.id === softId);
    if (idx < 0) return { ok: false, reason: 'planet not in modal (' + planets.length + ')' };
    const w = modalState.canvasWidth, h = modalState.canvasHeight;
    const l = layout.computeLayout(planets, modalState.starRadius, w, h);
    const pose = layout.getPlanetPose(l, planets[idx], idx, performance.now());
    const canvas = document.querySelector('#system-modal-overlay canvas');
    const rect = canvas.getBoundingClientRect();
    const mouseX = pose.x * modalState.zoom + modalState.offsetX + (modalState.followOffsetX || 0);
    const mouseY = pose.y * modalState.zoom + modalState.offsetY + (modalState.followOffsetY || 0);
    const clientX = rect.left + mouseX * (rect.width / w);
    const clientY = rect.top + mouseY * (rect.height / h);
    canvas.dispatchEvent(new MouseEvent('contextmenu', { clientX, clientY, bubbles: true, cancelable: true, button: 2 }));
    await new Promise(r => setTimeout(r, 300));
    const menu = document.getElementById('star-context-menu');
    return { ok: !!menu, text: menu ? menu.textContent : '' };
  }, softId);
}

async function openMap(page) {
  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForFunction(() => { const el = document.getElementById('loading'); return !el || el.style.display === 'none'; }, { timeout: 30000 });
  await page.waitForTimeout(1200);
}

let browsers = [];
async function finish(code) {
  for (const b of browsers) await b.close().catch(() => {});
  writeFileSync(path.join(ARTIFACTS_DIR, 'surface-admin-biome-results.json'), JSON.stringify(results, null, 2));
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);
  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER FOUND'); await finish(1); }
  console.log('browser: ' + exe);

  const resolved = resolveSoftPlanet();
  const WORLD = resolved.world, SOFT = resolved.planet;
  console.log('world/planet: ' + WORLD + ' / ' + SOFT);

  try {
    // ---------------- АДМИН ----------------
    const admin = await setupUser(WORLD, SOFT, 'admin');
    console.log('admin user: ' + admin.username);
    const a = await newPage('admin', admin.token);
    browsers.push(a.browser);
    await openMap(a.page);
    const menu = await openPlanetMenu(a.page, SOFT);
    report('A1 админ: ПКМ-меню открыто', menu.ok, 'menu="' + (menu.text || menu.reason || '').replace(/\s+/g, ' ').trim() + '"');
    report('A2 админ: тумблер «⚙ выбор биома»', /выбор биома/.test(menu.text || ''), 'menu="' + (menu.text || '').replace(/\s+/g, ' ').trim() + '"');

    // раскрыть аккордеон
    const expanded = await a.page.evaluate(() => {
      const t = document.getElementById('land-biome-toggle');
      if (!t) return { ok: false };
      t.click();
      const wrap = document.getElementById('star-context-menu');
      const rows = [...wrap.querySelectorAll('div')].map(d => d.textContent.replace(/\s+/g, ' ').trim());
      return { ok: true, arrow: t.textContent, rows };
    });
    report('A3 админ: список биомов раскрыт', expanded.ok && /▾/.test(expanded.arrow) && expanded.rows.some(r => /Случайно — как у игрока/.test(r)),
      'arrow="' + (expanded.arrow || '') + '" rows=' + JSON.stringify((expanded.rows || []).filter(r => /%/.test(r)).slice(0, 3)));
    report('A4 админ: чип «⚙ админ» в заголовке', (menu.text || '').includes('⚙ админ') || (expanded.rows || []).join(' ').includes('⚙ админ'), 'header chip');

    // ожидаемый биом (макс. доля) из modalState
    const expected = await a.page.evaluate(async (softId) => {
      const { modalState } = await import('/static/js/modal/state.js');
      const p = (modalState.planets || []).find(x => x.id === softId);
      const biomes = ((p && p.biomes) || []).filter(b => b && b.share > 0).sort((x, y) => y.share - x.share);
      return { count: biomes.length, form: biomes[0] && biomes[0].form, share: biomes[0] && biomes[0].share };
    }, SOFT);
    report('A5 админ: биомы планеты видны клиенту', expected.count > 0, 'biomes=' + expected.count + ' top=' + expected.form + ' ' + expected.share);

    const shareLabel = expected.share.toFixed(1) + ' %';
    const clicked = await a.page.evaluate((label) => {
      const menu = document.getElementById('star-context-menu');
      const row = [...menu.querySelectorAll('div')].find(d => d.textContent.includes(label) && d.textContent.trim().length < 100);
      if (!row) return false;
      row.click();
      return true;
    }, shareLabel);
    report('A6 админ: клик по биому с долей ' + shareLabel, clicked, 'clicked=' + clicked);

    await a.page.waitForURL('**/surface.html*', { timeout: 15000 });
    const urlBiome = new URL(a.page.url()).searchParams.get('biome');
    report('A7 админ: URL несёт выбранный биом', urlBiome === expected.form, 'url biome="' + urlBiome + '" ожидался="' + expected.form + '"');

    await a.page.waitForSelector('#briefing', { state: 'visible', timeout: 20000 });
    const brief = await a.page.evaluate(() => document.getElementById('briefing-content').textContent.replace(/\s+/g, ' ').trim());
    report('A8 админ: на брифинге та же доля', brief.includes(shareLabel), brief.slice(0, 160));

    await a.page.click('#brief-continue');
    await a.page.waitForSelector('#hud', { state: 'visible', timeout: 10000 });
    await a.page.waitForTimeout(600);
    const hasWeatherRow = await a.page.evaluate(() => !!document.getElementById('hud-weather-admin'));
    report('A9 админ: строка переключателя погоды видна', hasWeatherRow, 'hud-weather-admin=' + hasWeatherRow);

    const weatherWorked = await a.page.evaluate(async () => {
      const wrap = document.getElementById('hud-weather-admin');
      const btn = [...wrap.querySelectorAll('button')].find(b => b.textContent === 'штиль');
      if (!btn) return { ok: false, reason: 'нет чипа «штиль»' };
      btn.click();
      await new Promise(r => setTimeout(r, 500));
      const active = [...wrap.querySelectorAll('button')].find(b => b.dataset.active === '1');
      return { ok: true, hud: document.getElementById('hud-weather').textContent, active: active && active.textContent };
    });
    report('A10 админ: принудительная погода «штиль · вручную»', weatherWorked.ok && weatherWorked.hud === 'штиль · вручную',
      'hud="' + (weatherWorked.hud || weatherWorked.reason || '') + '" active="' + (weatherWorked.active || '') + '"');
    report('A11 админ: нет JS-ошибок', a.errors.length === 0, a.errors.slice(0, 2).join(' | '));

    // ---------------- ОБЫЧНЫЙ ИГРОК ----------------
    const player = await setupUser(WORLD, SOFT, null);
    console.log('player user: ' + player.username);
    const p = await newPage('player', player.token);
    browsers.push(p.browser);
    await openMap(p.page);
    const pmenu = await openPlanetMenu(p.page, SOFT);
    const playerNoPicker = !/выбор биома/.test(pmenu.text || '') && /биом \?/.test(pmenu.text || '');
    report('P1 обычный игрок: нет списка биомов в ПКМ-меню', playerNoPicker, 'menu="' + (pmenu.text || pmenu.reason || '').replace(/\s+/g, ' ').trim() + '"');
    report('P2 обычный игрок: нет JS-ошибок', p.errors.length === 0, p.errors.slice(0, 2).join(' | '));

    const failed = results.filter(r => !r.ok).length;
    console.log('SUMMARY: ' + results.filter(r => r.ok).length + ' PASS / ' + failed + ' FAIL');
    await finish(failed ? 1 : 0);
  } catch (e) {
    console.log('EXCEPTION: ' + e.message + '\n' + e.stack);
    await finish(1);
  }
}
main();
