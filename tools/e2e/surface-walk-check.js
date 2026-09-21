// tools/e2e/surface-walk-check.js
// E2E-смоук флоу высадки (спека 2026-09-21): ПКМ по планете в модалке → «Высадиться»
// → прелоадер → прогулка (мир рисуется, ходьба/прыжок, HUD) → «Вызвать корабль» → орбита.
// Игрок ставится в систему Notelden (SQL) и летит на орбиту мягкой планеты Huszephxan
// (API) — иначе свежий игрок спавнится в мире без планет.
// Run: node surface-walk-check.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const PASSWORD = 'e2e-walk-' + Date.now();

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

// psqlRun — psql c SQL из файла (PITFALLS: unicode-аргументы через -c ломаются;
// пишем .sql-файл, читаем stdout). Возвращает stdout (tuples-only).
function psqlRun(sql, filename) {
  const sqlPath = path.join(ARTIFACTS_DIR, filename);
  writeFileSync(sqlPath, sql, 'utf8');
  return String(execFileSync('cmd', ['/c', process.env.PSQL || 'C:\\pgsql\\pgsql\\bin\\psql.exe',
    '-h', '127.0.0.1', '-U', 'zorion', '-d', 'zorion', '-t', '-A', '-f', sqlPath],
    { env: { ...process.env, PGPASSWORD: process.env.PGPASSWORD || 'zorion123' } })).trim();
}

// resolveSoftPlanet — динамический резолв живой мягкой планеты (хвост e2e идеи
// 2026-09-21): мир + планета с ≥3 биомами, в вилках комфорта скафандра и с
// минимальной радиоактивностью. Хардкод UUID устаревал при перегенерации БД.
function resolveSoftPlanet() {
  const sql = `SELECT p.world_id, p.id FROM planets p
WHERE jsonb_typeof(p.data->'biomes') = 'array' AND jsonb_array_length(p.data->'biomes') >= 3
  AND (p.data->>'temperature')::float BETWEEN 263 AND 313
  AND COALESCE((p.data->'atmosphere_data'->>'pressure_atm')::float, 0) BETWEEN 0.5 AND 3.0
  AND COALESCE((p.data->>'life')::bool, false) = true
ORDER BY COALESCE((p.data->'core'->>'radioactivity')::float, 999) ASC LIMIT 1;
`;
  const line = psqlRun(sql, 'surface-resolve.sql').split(/\r?\n/).map((s) => s.trim()).filter(Boolean).pop();
  if (!line) throw new Error('не найдена живая мягкая планета (SQL вернул пусто)');
  const parts = line.split('|');
  return { world: parts[0], planet: parts[1] };
}

async function setupUser(world, planet, role) {
  let token = null, username = null;
  for (let i = 0; i < 3; i++) {
    username = 'e2e_walk_' + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
    if (res.status === 201) { token = (await res.json()).token; break; }
    if (res.status !== 409) throw new Error('register HTTP ' + res.status);
  }
  if (!token) throw new Error('register failed');
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  const uid = me.id;
  const roleSql = role ? `, role='${role}'` : '';
  const sql = `UPDATE users SET current_world_id='${world}', current_position=NULL${roleSql} WHERE id='${uid}';\n`;
  psqlRun(sql, 'surface-setup.sql');
  // Смена роли в БД не меняет уже выписанный JWT — перелогин за свежим токеном.
  if (role) {
    const lr = await fetch(BASE_URL + '/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
    if (lr.ok) token = (await lr.json()).token;
  }
  // внутрисистемный полёт на орбиту мягкой планеты
  const f = await fetch(BASE_URL + '/api/intrasystem-flight', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token }, body: JSON.stringify({ object_type: 'planet', object_id: planet }) });
  const fj = await f.json();
  const waitMs = Math.max(0, (fj.arrive_at || 0) - Date.now()) + 2000;
  await sleep(waitMs);
  const me2 = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  if (!me2.current_position || me2.current_position.object_id !== planet) throw new Error('not on orbit: ' + JSON.stringify(me2.current_position));
  return { token, username };
}

let browser = null;
async function finish(code) { if (browser) await browser.close().catch(() => {}); writeFileSync(path.join(ARTIFACTS_DIR, 'surface-walk-results.json'), JSON.stringify(results, null, 2)); process.exit(code); }

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);
  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER FOUND'); await finish(1); }
  console.log('browser: ' + exe);

  const resolved = resolveSoftPlanet();
  const WORLD = resolved.world, SOFT = resolved.planet;
  console.log('world/planet: ' + WORLD + ' / ' + SOFT);

  const setup = await setupUser(WORLD, SOFT);
  console.log('user: ' + setup.username);

  browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { try { localStorage.setItem('token', t); } catch (e) {} }, setup.token);
  const page = await context.newPage();

  const pageErrors = [];
  const consoleErrors = [];
  const redirects = [];
  const badResponses = [];
  const skyImageHits = [];
  page.on('response', (r) => { if (r.status() >= 400) badResponses.push(r.status() + ' ' + r.url()); });
  page.on('response', (r) => { const u = r.url(); if (u.includes('/api/planet-image')) skyImageHits.push(r.status() + ' ' + u); });
  page.on('pageerror', (e) => pageErrors.push(String(e)));
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  page.on('framenavigated', (fr) => { if (fr === page.mainFrame() && fr.url().includes('/login-page')) redirects.push(fr.url()); });

  try {
    // --- открыть карту ---
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => { const el = document.getElementById('loading'); return !el || el.style.display === 'none'; }, { timeout: 30000 });
    await page.waitForTimeout(1200);

    // --- клик по своей звезде -> модалка системы ---
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
      for (const c of clusters) { const px = c.x * scale + offsetX, py = c.y * scale + offsetY; const d = (px - cx) ** 2 + (py - cy) ** 2; if (d < bestD) { bestD = d; best = { x: px, y: py, sid: c.sid, name: c.sname }; } }
      if (!best) return { ok: false, reason: 'no clusters' };
      return { ok: true, clickX: rect.left + best.x * (rect.width / cw), clickY: rect.top + best.y * (rect.height / ch), star: best };
    }, BASE_URL);
    if (!clickTarget.ok) { report('10a открыть модалку системы', false, clickTarget.reason); await finish(1); }
    await page.mouse.click(clickTarget.clickX, clickTarget.clickY);
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForTimeout(2000);
    const modalOpen = await page.evaluate(() => !!document.querySelector('#system-modal-overlay') && !!document.querySelector('#system-modal-overlay canvas'));
    report('10a открыть модалку системы', modalOpen, 'overlay+canvas=' + modalOpen);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'surface-modal.png') });

    // --- ПКМ по планете -> пункт «Высадиться» ---
    const menuInfo = await page.evaluate(async (softId) => {
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
      const text = menu ? menu.textContent : '';
      const item = menu ? [...menu.querySelectorAll('div')].find(d => d.textContent.includes('Высадиться')) : null;
      return { ok: !!item, text, hasLetsPojt: text.includes('Лететь') || false, client: { clientX: Math.round(clientX), clientY: Math.round(clientY) } };
    }, SOFT);
    report('10b ПКМ по планете -> пункт «Высадиться»', menuInfo.ok, 'menu="' + (menuInfo.text || menuInfo.reason || '').replace(/\s+/g, ' ').trim() + '"');

    // Обычный игрок: админского выбора биома и списка биомов нет (идея 2026-09-21).
    const playerNoPicker = !/выбор биома/.test(menuInfo.text || '') && /биом \?/.test(menuInfo.text || '');
    report('10b2 обычный игрок: нет выбора биома (список скрыт)', playerNoPicker, 'menu="' + (menuInfo.text || '').replace(/\s+/g, ' ').trim() + '"');

    // --- прелоадер: замедлим land, чтобы поймать оверлей ---
    await page.route('**/api/surface/land', async (route) => { await sleep(1200); await route.continue(); });
    const navPromise = page.waitForURL('**/surface.html*', { timeout: 15000 });
    await page.evaluate((softId) => {
      const menu = document.getElementById('star-context-menu');
      const item = [...menu.querySelectorAll('div')].find(d => d.textContent.includes('Высадиться'));
      item.click();
    }, SOFT);
    await navPromise;
    const loadingSeen = await page.waitForSelector('#loading', { state: 'visible', timeout: 8000 }).then(() => true).catch(() => false);
    report('10c прелоадер высадки показан', loadingSeen, 'loading visible=' + loadingSeen);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'surface-loading.png') });

    await page.waitForSelector('#briefing', { state: 'visible', timeout: 20000 });
    const brief = await page.evaluate(() => document.getElementById('briefing-content').textContent.replace(/\s+/g, ' ').trim().slice(0, 200));
    report('10d брифинг высадки отрисован', brief.length > 10, brief);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'surface-briefing.png') });

    await page.unroute('**/api/surface/land');
    // БЛОКЕР: прелоадер #loading не скрывается перед брифингом и перехватывает
    // клики (проверяем) — жмём «Продолжить» программно, чтобы проверить прогулку.
    const loadingBlocks = await page.evaluate(() => {
      const el = document.getElementById('loading');
      return getComputedStyle(el).display !== 'none' && getComputedStyle(el).pointerEvents !== 'none';
    });
    report('10d2 прелоадер скрыт после брифинга', !loadingBlocks, '#loading display=' + (loadingBlocks ? 'виден (перехватывает клики)' : 'скрыт'));
    await page.click('#brief-continue');
    await page.waitForSelector('#hud', { state: 'visible', timeout: 10000 });
    await page.waitForTimeout(1500);

    // Обычный игрок: админской строки переключателя погоды нет ВООБЩЕ.
    const playerNoWeather = await page.evaluate(() => !document.getElementById('hud-weather-admin'));
    report('10h обычный игрок: нет строки погоды (админ)', playerNoWeather, 'hud-weather-admin отсутствует=' + playerNoWeather);

    // --- мир рисуется: разнообразие цветов канваса ---
    const worldDrawn = await page.evaluate(() => {
      const c = document.getElementById('surface-canvas');
      const ctx = c.getContext('2d');
      const d = ctx.getImageData(0, 0, c.width, c.height).data;
      const set = new Set();
      for (let i = 0; i < d.length; i += 4 * 97) set.add((d[i] << 16) | (d[i + 1] << 8) | d[i + 2]);
      return { colors: set.size, w: c.width, h: c.height };
    });
    report('10e мир рисуется (canvas не одноцветный)', worldDrawn.colors > 10, 'uniqColors=' + worldDrawn.colors);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'surface-play.png') });

    // --- 10e2: тело неба грузит реальную текстуру (идея §8.4) ---
    const skyTexOk = skyImageHits.some(h => h.startsWith('200') && h.includes(SOFT));
    report('10e2 небо: текстура планеты загружена (200 /api/planet-image)', skyTexOk,
      'hits=' + JSON.stringify(skyImageHits.slice(0, 4)));

    // --- HUD hp ---
    const hpText = await page.evaluate(() => document.getElementById('hp-text').textContent);
    report('11 HUD показывает здоровье', /100\s*\/\s*100/.test(hpText), 'hp-text="' + hpText + '"');

    // --- ходьба ---
    await page.keyboard.down('ArrowRight');
    await page.waitForTimeout(1600);
    await page.keyboard.up('ArrowRight');
    const dist = await page.evaluate(() => document.getElementById('hud-distance').textContent);
    const distM = parseInt(dist, 10);
    report('10f персонаж идёт (счётчик пути растёт)', distM > 1, 'distance="' + dist + '"');

    // --- прыжок: визор игрока (#38bdf8) выше базовой позиции ---
    const visorY = () => page.evaluate(() => {
      const c = document.getElementById('surface-canvas');
      const ctx = c.getContext('2d');
      const cx = Math.floor(c.width / 2), cy = Math.floor(c.height / 2);
      const x0 = Math.max(0, cx - 250), y0 = Math.max(0, cy - 350);
      const w = Math.min(c.width - x0, 500), h = Math.min(c.height - y0, 700);
      const d = ctx.getImageData(x0, y0, w, h).data;
      let sum = 0, n = 0;
      for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
        const i = (y * w + x) * 4;
        if (d[i] === 56 && d[i + 1] === 189 && d[i + 2] === 248) { sum += y0 + y; n++; }
      }
      return n ? sum / n : -1;
    });
    // --- прыжок: сравнить стабильность визора в покое и его подъём при прыжке ---
    // Порог дрожи визора в покое — экранные px. Рендер прогулки масштабируется
    // ZOOM (surface_config.js): мировой дрожь ~1 px даёт ~ZOOM экранных, поэтому
    // базовые 2 px (калибровка на 1×) умножаем на фактический ZOOM.
    const ZOOM = await page.evaluate(async () => (await import('/static/js/surface/surface_config.js')).ZOOM);
    await page.waitForTimeout(900); // vx -> 0, камера стабилизировалась
    const idle = [];
    for (let i = 0; i < 12; i++) { await page.waitForTimeout(25); idle.push(await visorY()); }
    const idleValid = idle.filter(v => v > 0);
    const idleMin = idleValid.length ? Math.min(...idleValid) : -1;
    const idleMax = idleValid.length ? Math.max(...idleValid) : -1;
    const idleRange = idleMax - idleMin;
    await page.keyboard.down('Space');
    const samples = [];
    for (let i = 0; i < 50; i++) { await page.waitForTimeout(16); samples.push(await visorY()); }
    await page.keyboard.up('Space');
    const valid = samples.filter(v => v > 0);
    const jumpMin = valid.length ? Math.min(...valid) : -1;
    const rise = idleMin - jumpMin;
    report('10g персонаж прыгает (визор поднимается)', idleMin > 0 && jumpMin > 0 && idleRange <= 2 * ZOOM && rise > 3,
      'idleY=' + idleMin.toFixed(1) + ' idleRange=' + idleRange.toFixed(1) + ' max=' + (2 * ZOOM).toFixed(1) + ' jumpMinY=' + jumpMin.toFixed(1) + ' rise=' + rise.toFixed(1));

    // --- 14: hp не проседает от ходьбы/прыжков (мягкая планета) ---
    const hpAfter = await page.evaluate(() => document.getElementById('hp-text').textContent);
    report('14 hp не проседает от падения/жидкости', hpAfter === hpText, 'before="' + hpText + '" after="' + hpAfter + '"');

    // --- 11: кнопка вызова возвращает на орбиту ---
    await page.click('#ship-call-btn');
    await page.waitForURL('**/map**', { timeout: 15000 }).catch(() => {});
    await page.waitForTimeout(1000);
    const afterUrl = page.url();
    const meAfter = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + setup.token } })).json();
    const onOrbit = meAfter.current_position && meAfter.current_position.status === 'orbit' && meAfter.current_position.object_id === SOFT;
    report('11b «Вызвать корабль» вернул на орбиту', onOrbit, 'url=' + afterUrl + ' pos=' + JSON.stringify(meAfter.current_position));

    // --- 12: сессия без ошибок ---
    const toastErr = await page.evaluate(() => !!document.querySelector('.toast-error'));
    // «Failed to load resource» — сетевая ошибка ресурса (обычно favicon.ico), не JS-ошибка.
    const jsConsoleErrors = consoleErrors.filter(t => !/Failed to load resource/i.test(t));
    const onlyFavicon = badResponses.every(b => b.includes('favicon.ico'));
    const ok12 = pageErrors.length === 0 && jsConsoleErrors.length === 0 && redirects.length === 0 && !toastErr && onlyFavicon;
    report('12 нет JS-ошибок/тостов/разлогина', ok12,
      'pageErrors=' + pageErrors.length + ' jsConsoleErrors=' + jsConsoleErrors.length + ' redirects=' + redirects.length + ' toastError=' + toastErr +
      ' badResponses=[' + badResponses.join(' | ') + ']' +
      (pageErrors.length ? ' firstPageError=' + pageErrors[0] : '') + (jsConsoleErrors.length ? ' firstConsole=' + jsConsoleErrors[0] : ''));
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'surface-after-leave.png') });

    const failed = results.filter(r => !r.ok).length;
    console.log('SUMMARY: ' + results.filter(r => r.ok).length + ' PASS / ' + failed + ' FAIL');
    await finish(failed ? 1 : 0);
  } catch (e) {
    console.log('EXCEPTION: ' + e.message + '\n' + e.stack);
    await finish(1);
  }
}
main();
