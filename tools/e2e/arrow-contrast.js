// tools/e2e/arrow-contrast.js
// Contrast check for the player arrow marker (2026-09-18 refinement):
// white body + dark stroke + dark shadow must read on stars of ANY color.
//
// Run: node arrow-contrast.js <CLASS> [--flight]
//   CLASS  - spectral class of the star the player stands on (G/K/M/B/A/...).
//            The tester sets the player's current_world_id via psql BEFORE
//            running (dev DB): UPDATE users SET current_world_id='<world_id>'
//            WHERE username='<user>';
//   --flight - also start a flight to the nearest single star and verify the
//            arrow is hidden while flying (do this LAST - the player stays
//            in flight afterwards).
// Env: ARROW_USER / ARROW_PASS - existing account (else a new one is
//      registered and its credentials printed for the psql step).
//
// Checks: P1 arrow visible + bobbing, P2 contrast CSS (white gradient fill,
// dark stroke #0f172a, dark circle #1e293b, double drop-shadow), P3a click
// through arrow opens modal, P3b pan keeps arrow synced, P3d hidden on small
// zoom, P3c hidden in flight, P4 no JS errors. Screenshot per class:
// artifacts/arrow-contrast-<CLASS>.png
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CLASS = (process.argv[2] || '').toUpperCase();
const DO_FLIGHT = process.argv.includes('--flight');

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

async function registerUser() {
  const username = 'arrow_' + Date.now();
  const password = 'arrow-pass-' + Date.now();
  const res = await fetch(BASE_URL + '/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  if (res.status !== 201) throw new Error('register HTTP ' + res.status + ': ' + (await res.text()));
  return { username, password };
}

async function login(username, password) {
  const res = await fetch(BASE_URL + '/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  if (res.status !== 200) throw new Error('login HTTP ' + res.status + ': ' + (await res.text()));
  const data = await res.json();
  return data.token;
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  if (!CLASS) {
    console.log('usage: node arrow-contrast.js <CLASS> [--flight]');
    return finish(1);
  }
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL + '  CLASS: ' + CLASS + (DO_FLIGHT ? '  FLIGHT: yes' : ''));

  // --- credentials ---
  let username = process.env.ARROW_USER;
  let password = process.env.ARROW_PASS;
  if (!username || !password) {
    try {
      const creds = await registerUser();
      username = creds.username;
      password = creds.password;
      console.log('NEW ACCOUNT: username=' + username + ' password=' + password);
      console.log('  -> set current_world_id via psql, then rerun with ARROW_USER/ARROW_PASS');
    } catch (err) {
      report('register', 'FAIL', String(err && err.message ? err.message : err));
      return finish(1);
    }
  }

  let token = null;
  try {
    token = await login(username, password);
    report('login', 'PASS', 'user ' + username);
  } catch (err) {
    report('login', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

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
  const consoleErrors = [];
  page.on('console', (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });

  try {
    // --- load map ---
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1500); // settle render loop + arrow position

    // --- P1: arrow visible over the player's star, bobbing ---
    const p1 = await page.evaluate(() => {
      const el = document.getElementById('player-arrow');
      const c = document.getElementById('mapCanvas');
      const rect = c.getBoundingClientRect();
      const svg = el ? el.querySelector('svg') : null;
      const t1 = svg ? getComputedStyle(svg).transform : null;
      return {
        exists: !!el,
        hidden: el ? el.hidden : null,
        left: el ? parseFloat(el.style.left) : null,
        top: el ? parseFloat(el.style.top) : null,
        canvasCenterX: rect.width / 2,
        canvasCenterY: rect.height / 2,
        transform1: t1,
      };
    });
    await page.waitForTimeout(600);
    const transform2 = await page.evaluate(() => {
      const el = document.getElementById('player-arrow');
      const svg = el ? el.querySelector('svg') : null;
      return svg ? getComputedStyle(svg).transform : null;
    });
    const bob = p1.transform1 && transform2 && p1.transform1 !== transform2;
    const centered = p1.exists && !p1.hidden &&
      Math.abs(p1.left - p1.canvasCenterX) < 3 && Math.abs(p1.top - p1.canvasCenterY) < 3;
    const ok1 = p1.exists && !p1.hidden && centered && bob;
    report('P1 arrow visible + bob', ok1 ? 'PASS' : 'FAIL',
      `exists=${p1.exists} hidden=${p1.hidden} pos=(${p1.left},${p1.top}) center=(${p1.canvasCenterX},${p1.canvasCenterY}) bob=${bob} (${p1.transform1} -> ${transform2})`);
    if (!ok1) return finish(1);

    // --- P2: contrast CSS + screenshot ---
    const p2 = await page.evaluate(async (baseUrl) => {
      const el = document.getElementById('player-arrow');
      const svg = el.querySelector('svg');
      const pathEl = svg.querySelector('path');
      const circleEl = svg.querySelector('circle');
      const grad = document.getElementById('playerArrowGrad');
      const stops = grad ? Array.from(grad.querySelectorAll('stop')).map(s => s.getAttribute('stop-color')) : [];
      const cs = getComputedStyle(el);
      const ps = getComputedStyle(pathEl);
      const circs = getComputedStyle(circleEl);
      // Player star spectral class from /worlds/{id}
      let spec = null;
      try {
        const meRes = await fetch(baseUrl + '/me', { headers: { Authorization: 'Bearer ' + localStorage.getItem('token') } });
        const me = await meRes.json();
        if (me && me.current_world_id) {
          const wRes = await fetch(baseUrl + '/worlds/' + me.current_world_id, { headers: { Authorization: 'Bearer ' + localStorage.getItem('token') } });
          if (wRes.ok) {
            const w = await wRes.json();
            const ww = w.world || w;
            spec = ww.spectral_class || null;
          }
        }
      } catch (e) { /* keep null */ }
      return {
        filter: cs.filter,
        pathFill: ps.fill,
        pathStroke: ps.stroke,
        pathStrokeWidth: ps.strokeWidth,
        circleFill: circs.fill,
        stops,
        spec,
      };
    }, BASE_URL);

    const expStroke = 'rgb(15, 23, 42)'; // #0f172a
    const expCircle = 'rgb(30, 41, 59)'; // #1e293b
    const ok2 = p2.pathFill.includes('playerArrowGrad') &&
      p2.pathStroke === expStroke &&
      parseFloat(p2.pathStrokeWidth) === 2 &&
      p2.circleFill === expCircle &&
      p2.stops.includes('#ffffff') && p2.stops.includes('#cbd5e1') &&
      (p2.filter.match(/drop-shadow/g) || []).length === 2;
    report('P2 contrast CSS', ok2 ? 'PASS' : 'FAIL',
      `fill=${p2.pathFill} stroke=${p2.pathStroke} w=${p2.pathStrokeWidth} circle=${p2.circleFill} stops=${JSON.stringify(p2.stops)} filter=${p2.filter} playerSpec=${p2.spec}`);
    if (!ok2) return finish(1);

    const shotPath = path.join(ARTIFACTS_DIR, 'arrow-contrast-' + CLASS + '.png');
    await page.screenshot({ path: shotPath });
    report('P2 screenshot', 'PASS', shotPath);

    // --- P3a: click through the arrow opens the star modal ---
    const canvasBox = await page.locator('#mapCanvas').boundingBox();
    await page.mouse.click(canvasBox.x + canvasBox.width / 2, canvasBox.y + canvasBox.height / 2);
    await page.waitForTimeout(2000);
    const afterClick = await page.evaluate(() => ({
      modal: !!document.getElementById('system-modal-overlay'),
      toastError: !!document.querySelector('.toast-error'),
      path: location.pathname,
    }));
    const ok3a = afterClick.modal && !afterClick.toastError && afterClick.path !== '/login-page';
    report('P3a click through arrow', ok3a ? 'PASS' : 'FAIL',
      `modal=${afterClick.modal} toastError=${afterClick.toastError} path=${afterClick.path}`);
    if (!ok3a) return finish(1);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(400);

    // --- P3b: pan +100/+100 keeps the arrow synced ---
    const before = await page.evaluate(() => {
      const el = document.getElementById('player-arrow');
      return { left: parseFloat(el.style.left), top: parseFloat(el.style.top), hidden: el.hidden };
    });
    await page.mouse.move(canvasBox.x + 300, canvasBox.y + 300);
    await page.mouse.down();
    await page.mouse.move(canvasBox.x + 400, canvasBox.y + 400, { steps: 5 });
    await page.mouse.up();
    await page.waitForTimeout(600);
    const after = await page.evaluate(() => {
      const el = document.getElementById('player-arrow');
      return { left: parseFloat(el.style.left), top: parseFloat(el.style.top), hidden: el.hidden };
    });
    const ok3b = !after.hidden &&
      Math.abs(after.left - before.left - 100) < 2 && Math.abs(after.top - before.top - 100) < 2;
    report('P3b pan sync', ok3b ? 'PASS' : 'FAIL',
      `before=(${before.left},${before.top}) after=(${after.left},${after.top}) delta=(${(after.left - before.left).toFixed(1)},${(after.top - before.top).toFixed(1)}) hidden=${after.hidden}`);
    if (!ok3b) return finish(1);

    // --- P3d: hidden on small zoom ---
    await page.mouse.move(canvasBox.x + canvasBox.width / 2, canvasBox.y + canvasBox.height / 2);
    for (let i = 0; i < 45; i++) {
      await page.mouse.wheel(0, 1000);
      await page.waitForTimeout(60);
    }
    await page.waitForTimeout(500);
    const smallZoom = await page.evaluate(() => {
      const el = document.getElementById('player-arrow');
      const zi = document.getElementById('zoom-info');
      return { hidden: el.hidden, zoomInfo: zi ? zi.textContent : null };
    });
    const ok3d = smallZoom.hidden === true;
    report('P3d hidden on small zoom', ok3d ? 'PASS' : 'FAIL',
      `hidden=${smallZoom.hidden} zoom=${smallZoom.zoomInfo}`);
    if (!ok3d) return finish(1);

    // --- P3c: hidden in flight (optional, run LAST) ---
    if (DO_FLIGHT) {
      await page.reload({ waitUntil: 'domcontentloaded', timeout: 30000 });
      await page.waitForSelector('#mapCanvas', { timeout: 15000 });
      await page.waitForFunction(() => {
        const el = document.getElementById('loading');
        return !el || el.style.display === 'none';
      }, { timeout: 30000 });
      await page.waitForTimeout(1500);

      const flight = await page.evaluate(async (baseUrl) => {
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
        const R = 3000;
        const params = new URLSearchParams({
          x_min: (px - R).toFixed(3), x_max: (px + R).toFixed(3),
          y_min: (py - R).toFixed(3), y_max: (py + R).toFixed(3),
          cell: '40',
        });
        const res = await fetch(baseUrl + '/api/worlds/filter?' + params.toString(), { headers: auth });
        if (!res.ok) return { ok: false, reason: 'filter HTTP ' + res.status };
        const clusters = await res.json();
        let best = null, bestD = Infinity;
        for (const c of clusters) {
          if (c.cnt !== 1 || !c.sid || c.sid === myWorldId) continue;
          const d = Math.hypot(c.x - px, c.y - py);
          if (d < bestD) { bestD = d; best = c; }
        }
        if (!best) return { ok: false, reason: 'no single star in range' };
        const tRes = await fetch(baseUrl + '/travel', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', ...auth },
          body: JSON.stringify({ world_id: best.sid }),
        });
        if (!tRes.ok) return { ok: false, reason: 'travel HTTP ' + tRes.status + ': ' + (await tRes.text()) };
        return { ok: true, target: best.sname || best.sid, dist: bestD };
      }, BASE_URL);

      if (!flight.ok) {
        report('P3c hidden in flight', 'SKIP', flight.reason);
      } else {
        await page.reload({ waitUntil: 'domcontentloaded', timeout: 30000 });
        await page.waitForSelector('#mapCanvas', { timeout: 15000 });
        await page.waitForFunction(() => {
          const el = document.getElementById('loading');
          return !el || el.style.display === 'none';
        }, { timeout: 30000 });
        await page.waitForTimeout(2500); // flight restore from /me is async
        const inFlight = await page.evaluate(() => {
          const el = document.getElementById('player-arrow');
          const panel = document.getElementById('flight-panel');
          return { hidden: el.hidden, flying: panel ? panel.classList.contains('flying') : false };
        });
        const ok3c = inFlight.hidden === true && inFlight.flying;
        report('P3c hidden in flight', ok3c ? 'PASS' : 'FAIL',
          `target=${flight.target} dist=${flight.dist.toFixed(0)} hidden=${inFlight.hidden} flightPanel=${inFlight.flying}`);
        if (!ok3c) return finish(1);
      }
    }

    // --- P4: console ---
    const errs = pageErrors.slice();
    // Network 404s (favicon.ico etc.) are resource-load errors, not JS errors;
    // they are reported separately below.
    const cErr = consoleErrors.filter(t => !t.includes('Failed to load resource'));
    const net404 = consoleErrors.filter(t => t.includes('Failed to load resource'));
    const ok4 = errs.length === 0 && cErr.length === 0;
    report('P4 no JS errors', ok4 ? 'PASS' : 'FAIL',
      `pageErrors=${errs.length} consoleErrors=${cErr.length}` +
      (errs.length ? ' first: ' + errs[0] : '') +
      (cErr.length ? ' first: ' + cErr[0] : '') +
      (net404.length ? ' (net404=' + net404.length + ' - favicon, pre-existing)' : ''));
    if (!ok4) return finish(1);
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