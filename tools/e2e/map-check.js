// tools/e2e/map-check.js
// Browser smoke test for the Zorion map (playwright-core + system Chrome/Edge).
// Catches client regressions that static review misses: star-click toast-error
// (77a "Cannot read properties of undefined (reading 'star_type')"), login
// redirect, restricted card for out-of-radar systems.
//
// Run: node map-check.js   (BASE_URL env overrides the default)
// Steps: 1 register/login, 2 map loads, 3 click star, 4 restricted card,
//        5 screenshots. Exit code 0 = all PASS (SKIP allowed), 1 = any FAIL.
// Console output is ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');

// System browsers: Chrome first, Edge as fallback (env overrides allowed).
const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

// Radar radius fallback (models.RadarRadiusMin, spec 77a §4.2): used only when
// /me does not report radar_radius. The real threshold is read from /me below.
const RADAR_RADIUS_FALLBACK = 200;

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

// Step 1: register a unique user, get a JWT. Retry on 409 (name collision).
async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'e2e_' + Date.now() + '_' + attempt;
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
    if (res.status === 409) continue; // name taken - try another
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

  // --- Step 1: register/login ---
  let token = null;
  try {
    const creds = await registerAndLogin();
    token = creds.token;
    report('1/5 register/login', 'PASS', 'user ' + creds.username);
  } catch (err) {
    report('1/5 register/login', 'FAIL', String(err && err.message ? err.message : err));
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
  // Inject the token into localStorage before any page script runs.
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));
  let filterStatus = null;
  page.on('response', (res) => {
    if (res.url().includes('/api/worlds/filter')) filterStatus = res.status();
  });

  try {
    // --- Step 2: map loads ---
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    // Clusters are loaded when the map hides #loading (map/data.js loadClusters).
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1000); // settle for the render loop

    const mapState = await page.evaluate(() => {
      const c = document.getElementById('mapCanvas');
      const ctx = c.getContext('2d');
      const img = ctx.getImageData(0, 0, c.width, c.height).data;
      let nonEmpty = 0;
      for (let i = 3; i < img.length; i += 4) if (img[i] > 0) nonEmpty++;
      return {
        path: location.pathname,
        hasToken: !!localStorage.getItem('token'),
        canvas: { w: c.width, h: c.height, nonEmpty },
      };
    });

    const errs = pageErrors.slice();
    const ok2 = mapState.path === '/map' && mapState.hasToken &&
      filterStatus === 200 && mapState.canvas.nonEmpty > 0 && errs.length === 0;
    report('2/5 map loads', ok2 ? 'PASS' : 'FAIL',
      `path=${mapState.path} filter=${filterStatus} canvasPx=${mapState.canvas.nonEmpty} pageErrors=${errs.length}` +
      (errs.length ? ' first: ' + errs[0] : ''));
    if (!ok2) return finish(1);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'map.png') });

    // --- Step 3: click a star near the canvas center ---
    // The map keeps clusters/viewport in a module-scoped `state` (not on
    // window), so we reconstruct the viewport: saved one from sessionStorage,
    // or (fresh load) centered on the player's world at scale 1.0
    // (map/navigation.js centerOnAgent), then issue the same /api/worlds/filter
    // query the map uses (map/data.js buildUrl, CLUSTER_CELL_PX=40).
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

      // Nearest cluster to the canvas center.
      const cx = cw / 2, cy = ch / 2;
      let best = null, bestD = Infinity;
      for (const c of clusters) {
        const px = c.x * scale + offsetX;
        const py = c.y * scale + offsetY;
        const d = (px - cx) * (px - cx) + (py - cy) * (py - cy);
        if (d < bestD) { bestD = d; best = { x: px, y: py, sid: c.sid, cnt: c.cnt, name: c.sname }; }
      }
      if (!best) return { ok: false, reason: 'no clusters in viewport' };

      // Client coords: canvas CSS size may differ from its attribute size.
      const clickX = rect.left + best.x * (rect.width / cw);
      const clickY = rect.top + best.y * (rect.height / ch);
      return { ok: true, clickX, clickY, star: best, viewport: { scale, offsetX, offsetY } };
    }, BASE_URL);

    if (!clickTarget.ok) {
      report('3/5 click star', 'SKIP', clickTarget.reason);
    } else {
      await page.mouse.click(clickTarget.clickX, clickTarget.clickY);
      await page.waitForTimeout(2000); // let the modal fetch/render settle
      const after = await page.evaluate(() => ({
        path: location.pathname,
        hasToken: !!localStorage.getItem('token'),
        toastError: !!document.querySelector('.toast-error'),
        modalOpen: !!document.getElementById('system-modal-overlay'),
      }));
      const ok3 = after.path !== '/login-page' && after.hasToken && !after.toastError;
      report('3/5 click star', ok3 ? 'PASS' : 'FAIL',
        `star=${clickTarget.star.name || clickTarget.star.sid} cnt=${clickTarget.star.cnt} modal=${after.modalOpen} toastError=${after.toastError} path=${after.path}`);
      if (!ok3) return finish(1);
    }

    // --- Step 4: restricted card for an out-of-radar star ---
    // Close any modal left from step 3 (Escape = closeModal in modal/index.js).
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);

    // Radar radius from /me (spec 77a §4.2): starting equipment radar_1 -> 800.
    // A star is "out of visibility" when dist > radar_radius (planet_handler.go).
    let radarRadius = RADAR_RADIUS_FALLBACK;
    try {
      const meRes = await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } });
      if (meRes.ok) {
        const me = await meRes.json();
        if (typeof me.radar_radius === 'number' && me.radar_radius > 0) radarRadius = me.radar_radius;
      }
    } catch (e) { /* keep fallback */ }

    const farStar = await page.evaluate(async (baseUrl) => {
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };

      // Player position (spec 77a §4.2): current_world_id coords.
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

      // Wide query around the player; pick the farthest single star.
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
      return { ok: true, star: {
        sid: best.sid, name: best.sname || 'Star', spec: best.sspec || '',
        x: best.x, y: best.y, dist: bestD,
        stype: best.stype, stemp: best.stemp, systype: best.systype, smods: best.smods,
      } };
    }, BASE_URL);

    if (!farStar.ok) {
      report('4/5 restricted card', 'SKIP', farStar.reason);
    } else if (farStar.star.dist <= radarRadius) {
      report('4/5 restricted card', 'SKIP',
        `farthest star ${farStar.star.dist.toFixed(0)} units <= radar ${radarRadius} - nothing out of visibility on this data`);
    } else {
      // window.openSystemModal is the global fallback (modal/index.js). Pass
      // starInfo exactly like the map click path (map/events.js handleCanvasClick):
      // the 403 branch renders the star card from it (coords in the title).
      await page.evaluate((s) => {
        window.openSystemModal(s.sid, s.name, s.spec, null, null, {
          stype: s.stype,
          stemp: s.stemp,
          systype: s.systype,
          smods: s.smods,
          x: s.x,
          y: s.y,
        });
      }, farStar.star);
      await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
      await page.waitForTimeout(500);
      const restricted = await page.evaluate(() => {
        const panel = document.getElementById('right-panel');
        const title = document.querySelector('#system-modal-overlay h2');
        return {
          overlay: !!document.getElementById('system-modal-overlay'),
          text: panel ? panel.textContent.includes('Система вне зоны видимости') : false,
          title: title ? title.textContent : '',
          toastError: !!document.querySelector('.toast-error'),
          path: location.pathname,
        };
      });
      // Title format (modal/index.js renderModal): "Name (x, y)" - coords come
      // from starInfo, so this proves the starInfo path of the click worked.
      const expectedTitle = '(' + Math.trunc(farStar.star.x) + ', ' + Math.trunc(farStar.star.y) + ')';
      const titleHasCoords = restricted.title.includes(expectedTitle);
      const ok4 = restricted.overlay && restricted.text && titleHasCoords &&
        !restricted.toastError && restricted.path !== '/login-page';
      report('4/5 restricted card', ok4 ? 'PASS' : 'FAIL',
        `star=${farStar.star.name} dist=${farStar.star.dist.toFixed(0)} radar=${radarRadius} overlay=${restricted.overlay} text=${restricted.text} titleCoords=${titleHasCoords} toastError=${restricted.toastError} path=${restricted.path}`);
      if (!ok4) return finish(1);

      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'restricted-card.png') });
    }

    // --- Step 5: screenshots ---
    const mapPng = path.join(ARTIFACTS_DIR, 'map.png');
    const restrictedPng = path.join(ARTIFACTS_DIR, 'restricted-card.png');
    const haveMap = existsSync(mapPng);
    const haveRestricted = existsSync(restrictedPng);
    const step4 = results.find(r => r.stepName === '4/5 restricted card');
    const step4Skipped = step4 && step4.status === 'SKIP';
    const ok5 = haveMap && (haveRestricted || step4Skipped);
    report('5/5 screenshots', ok5 ? 'PASS' : 'FAIL',
      `map.png=${haveMap} restricted-card.png=${haveRestricted}` +
      (haveMap ? ' (' + mapPng + ')' : '') +
      (haveRestricted ? ' (' + restrictedPng + ')' : ''));
    if (!ok5) return finish(1);
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