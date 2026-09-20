// tools/e2e/map-check-planet.js
// Planet-modal smoke for the "atmosphere & surface visualization" feature
// (spec 2026-09-20): small textures on the orbit canvas, biome icons in the
// planet card, "View from orbit" after arrival, stub for foreign systems.
// Run: node map-check-planet.js   (BASE_URL env overrides the default)
// Exit code 0 = all PASS (SKIP allowed), 1 = any FAIL. ASCII output on purpose.
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
    const username = 'e2ep_' + Date.now() + '_' + attempt;
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
    report('1/7 register/login', 'PASS', 'user ' + creds.username);
  } catch (err) {
    report('1/7 register/login', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

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
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));
  let planetImageCalls = 0;
  let planetImageStatuses = [];
  page.on('response', (res) => {
    if (res.url().includes('/api/planet-image')) {
      planetImageCalls++;
      planetImageStatuses.push(res.status());
    }
  });

  try {
    // --- Step 2: open map, click own star -> modal ---
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1000);

    const clickTarget = await page.evaluate(async (baseUrl) => {
      const canvas = document.getElementById('mapCanvas');
      const rect = canvas.getBoundingClientRect();
      const cw = canvas.width, ch = canvas.height;
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };

      let vp = null;
      try { vp = JSON.parse(sessionStorage.getItem('viewport') || 'null'); } catch (e) { vp = null; }

      let wx = 0, wy = 0;
      const meRes = await fetch(baseUrl + '/me', { headers: auth });
      if (meRes.ok) {
        const me = await meRes.json();
        if (me && me.current_world_id) {
          const wRes = await fetch(baseUrl + '/worlds/' + me.current_world_id, { headers: auth });
          if (wRes.ok) {
            const w = await wRes.json();
            const ww = w.world || w;
            if (typeof ww.coord_x === 'number') { wx = ww.coord_x; wy = ww.coord_y; }
          }
        }
      }

      const scale = vp && vp.scale ? vp.scale : 1.0;
      const offsetX = vp ? vp.offsetX : cw / 2 - wx * scale;
      const offsetY = vp ? vp.offsetY : ch / 2 - wy * scale;

      const inv = 1 / scale;
      const params = new URLSearchParams({
        x_min: (-offsetX * inv).toFixed(3),
        x_max: ((cw - offsetX) * inv).toFixed(3),
        y_min: (-offsetY * inv).toFixed(3),
        y_max: ((ch - offsetY) * inv).toFixed(3),
        cell: (40 / scale).toFixed(3),
      });
      const res = await fetch(baseUrl + '/api/worlds/filter?' + params.toString(), { headers: auth });
      if (!res.ok) return { ok: false, reason: 'filter HTTP ' + res.status };
      const clusters = await res.json();

      const cx = cw / 2, cy = ch / 2;
      let best = null, bestD = Infinity;
      for (const c of clusters) {
        const px = c.x * scale + offsetX;
        const py = c.y * scale + offsetY;
        const d = (px - cx) * (px - cx) + (py - cy) * (py - cy);
        if (d < bestD) { bestD = d; best = { x: px, y: py, sid: c.sid, cnt: c.cnt, name: c.sname }; }
      }
      if (!best) return { ok: false, reason: 'no clusters in viewport' };
      return { ok: true, clickX: rect.left + best.x * (rect.width / cw), clickY: rect.top + best.y * (rect.height / ch), star: best };
    }, BASE_URL);

    if (!clickTarget.ok) {
      report('2/7 open own modal', 'SKIP', clickTarget.reason);
    } else {
      await page.mouse.click(clickTarget.clickX, clickTarget.clickY);
      await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
      await page.waitForTimeout(2500); // let textures load
      const modal = await page.evaluate(() => {
        const panel = document.getElementById('right-panel');
        const rows = panel ? panel.querySelectorAll('tr[data-index]').length : 0;
        const canvas = document.querySelector('#system-modal-overlay canvas');
        return {
          overlay: !!document.getElementById('system-modal-overlay'),
          rows,
          canvas: !!canvas,
          toastError: !!document.querySelector('.toast-error'),
          path: location.pathname,
        };
      });
      const errs = pageErrors.slice();
      const ok2 = modal.overlay && modal.rows > 0 && modal.canvas && !modal.toastError &&
        modal.path !== '/login-page' && errs.length === 0;
      report('2/7 open own modal', ok2 ? 'PASS' : 'FAIL',
        `rows=${modal.rows} canvas=${modal.canvas} toastError=${modal.toastError} pageErrors=${errs.length}` +
        (errs.length ? ' first: ' + errs[0] : ''));
      if (!ok2) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'planet-modal.png') });
    }

    // --- Step 3: planet textures requested (small, 200) ---
    const ok3 = planetImageCalls > 0 && planetImageStatuses.every(s => s === 200);
    report('3/7 planet textures', ok3 ? 'PASS' : 'FAIL',
      `calls=${planetImageCalls} statuses=[${planetImageStatuses.join(',')}]`);
    if (!ok3) return finish(1);

    // --- Step 4: planet card - composition + biome icons ---
    const card = await page.evaluate(async () => {
      const panel = document.getElementById('right-panel');
      const tr = panel && panel.querySelector('tr[data-index]');
      if (!tr) return { ok: false, reason: 'no planet row' };
      tr.click();
      await new Promise(r => setTimeout(r, 1200));
      const icons = panel.querySelectorAll('img[data-biome-fallback]');
      const surfaceHeader = [...panel.querySelectorAll('p')].some(p => p.textContent.trim() === 'Поверхность');
      const imgs = [...icons].map(i => i.getAttribute('src'));
      return {
        ok: true,
        iconCount: icons.length,
        surfaceHeader,
        imgs: imgs.slice(0, 5),
        toastError: !!document.querySelector('.toast-error'),
      };
    });
    const ok4 = card.ok && card.iconCount > 0 && card.surfaceHeader && !card.toastError;
    report('4/7 planet card icons', ok4 ? 'PASS' : 'FAIL',
      `icons=${card.iconCount} surfaceHeader=${card.surfaceHeader} toastError=${card.toastError}` +
      (card.imgs && card.imgs.length ? ' sample=' + card.imgs[0] : '') +
      (card.ok ? '' : ' reason=' + card.reason));
    if (!ok4) return finish(1);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'planet-card.png') });

    // --- Step 5: fly to the planet, then "View from orbit" ---
    const fly = await page.evaluate(async (baseUrl) => {
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };
      // planet id from the open modal data: fetch the system planets again
      const meRes = await fetch(baseUrl + '/me', { headers: auth });
      const me = await meRes.json();
      const wid = me && me.current_world_id;
      if (!wid) return { ok: false, reason: 'no current world' };
      const pRes = await fetch(baseUrl + '/api/worlds/' + wid + '/planets', { headers: auth });
      if (!pRes.ok) return { ok: false, reason: 'planets HTTP ' + pRes.status };
      const data = await pRes.json();
      const planets = data.planets || data;
      if (!planets || !planets.length) return { ok: false, reason: 'no planets' };
      const pid = planets[0].id;
      const fRes = await fetch(baseUrl + '/api/intrasystem-flight', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
        body: JSON.stringify({ object_type: 'planet', object_id: pid }),
      });
      if (!fRes.ok) return { ok: false, reason: 'flight HTTP ' + fRes.status + ': ' + (await fRes.text()) };
      const f = await fRes.json();
      return { ok: true, pid, arrive_at: f.arrive_at };
    }, BASE_URL);

    if (!fly.ok) {
      report('5/7 orbit view', 'SKIP', fly.reason);
    } else {
      const waitMs = Math.max(0, new Date(fly.arrive_at).getTime() - Date.now()) + 2500;
      await page.waitForTimeout(waitMs);
      // Re-open the modal so my_position (orbit {planet}) is fresh.
      await page.keyboard.press('Escape');
      await page.waitForTimeout(400);
      await page.mouse.click(clickTarget.clickX, clickTarget.clickY);
      await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
      await page.waitForTimeout(1500);
      const orbit = await page.evaluate(async () => {
        const panel = document.getElementById('right-panel');
        const tr = panel && panel.querySelector('tr[data-index]');
        if (!tr) return { ok: false, reason: 'no planet row' };
        tr.click();
        await new Promise(r => setTimeout(r, 2000)); // big texture fetch
        const view = panel.querySelector('[data-orbit-view]');
        const img = view ? view.querySelector('img') : null;
        return {
          ok: true,
          view: !!view,
          imgLoaded: !!(img && img.complete && img.naturalWidth > 0),
          imgSrc: img ? img.getAttribute('src').slice(0, 40) : '',
          toastError: !!document.querySelector('.toast-error'),
        };
      });
      const ok5 = orbit.ok && orbit.view && orbit.imgLoaded && !orbit.toastError;
      report('5/7 orbit view', ok5 ? 'PASS' : 'FAIL',
        `view=${orbit.view} imgLoaded=${orbit.imgLoaded} toastError=${orbit.toastError}` +
        (orbit.ok ? '' : ' reason=' + orbit.reason));
      if (!ok5) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'orbit-view.png') });
    }

    // --- Step 6: restricted foreign modal - no planet-image calls ---
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    const before = planetImageCalls;
    const farStar = await page.evaluate(async (baseUrl) => {
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };
      const meRes = await fetch(baseUrl + '/me', { headers: auth });
      const me = await meRes.json();
      const myWorldId = me && me.current_world_id;
      let px = 0, py = 0;
      if (myWorldId) {
        const wRes = await fetch(baseUrl + '/worlds/' + myWorldId, { headers: auth });
        if (wRes.ok) {
          const w = await wRes.json();
          const ww = w.world || w;
          if (typeof ww.coord_x === 'number') { px = ww.coord_x; py = ww.coord_y; }
        }
      }
      const R = 5000;
      const params = new URLSearchParams({
        x_min: (px - R).toFixed(3), x_max: (px + R).toFixed(3),
        y_min: (py - R).toFixed(3), y_max: (py + R).toFixed(3),
        cell: '40',
      });
      const res = await fetch(baseUrl + '/api/worlds/filter?' + params.toString(), { headers: auth });
      if (!res.ok) return { ok: false, reason: 'filter HTTP ' + res.status };
      const clusters = await res.json();
      let best = null, bestD = -1;
      for (const c of clusters) {
        if (c.cnt !== 1 || !c.sid || c.sid === myWorldId) continue;
        const d = Math.hypot(c.x - px, c.y - py);
        if (d > bestD) { bestD = d; best = c; }
      }
      if (!best) return { ok: false, reason: 'no single star found in range' };
      return { ok: true, star: { sid: best.sid, name: best.sname || 'Star', spec: best.sspec || '', x: best.x, y: best.y, stype: best.stype, stemp: best.stemp, systype: best.systype, smods: best.smods } };
    }, BASE_URL);

    if (!farStar.ok) {
      report('6/7 restricted no-texture', 'SKIP', farStar.reason);
    } else {
      await page.evaluate((s) => {
        window.openSystemModal(s.sid, s.name, s.spec, null, null, {
          stype: s.stype, stemp: s.stemp, systype: s.systype, smods: s.smods, x: s.x, y: s.y,
        });
      }, farStar.star);
      await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
      await page.waitForTimeout(1500);
      const after = planetImageCalls;
      const restricted = await page.evaluate(() => {
        const panel = document.getElementById('right-panel');
        return {
          text: panel ? panel.textContent.includes('Система вне зоны видимости') : false,
          toastError: !!document.querySelector('.toast-error'),
        };
      });
      const ok6 = restricted.text && !restricted.toastError && after === before;
      report('6/7 restricted no-texture', ok6 ? 'PASS' : 'FAIL',
        `text=${restricted.text} toastError=${restricted.toastError} planetImageCalls=${before}->${after}`);
      if (!ok6) return finish(1);
    }

    // --- Step 7: no page errors overall ---
    const errs = pageErrors.slice();
    const ok7 = errs.length === 0;
    report('7/7 no page errors', ok7 ? 'PASS' : 'FAIL', errs.length ? errs.join(' | ') : '0 errors');
    if (!ok7) return finish(1);
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