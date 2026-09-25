// tools/e2e/route-check.js
// Real browser e2e smoke for the accelerator mini-game "Route plotting"
// (main spec docs/specs/2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md
// §9 steps 1-8; UI spec docs/specs/2026-09-25-маршрут-мини-игра-интерфейс.md §6.6).
//
// Mobile viewport 390x844, system Chrome/Edge via playwright-core (no browser
// download), throwaway user via /register (QA_TOKEN overrides the login).
//
// Run:  cd tools/e2e; npm.cmd i; node route-check.js
// Env:  BASE_URL (default http://localhost:8080), QA_TOKEN, CHROME_PATH, EDGE_PATH.
//
// Output is ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic). Step 7
// (server restart mid-flight) is an explicit SKIP: restarting the shared dev
// server would break every other agent's session.
//
// Exit code: 0 = PASS/SKIP only, 1 = any FAIL.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const QA_TOKEN = process.env.QA_TOKEN || '';
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');

// System browsers: Chrome first, Edge as fallback (env overrides allowed).
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

// Acceptance range for the segment shrink after one boost (spec §8): a +10%..+50%
// speed bonus means newRem/remaining = 1/(1+bonus) in [2/3, 10/11] -> decrease
// 9%..33%. A little slack covers the few seconds between offer and submit.
const DECREASE_MIN = 0.08;
const DECREASE_MAX = 0.35;

const VIEWPORT = { width: 390, height: 844 };
const MIN_TOUCH = 44; // safe mobile touch target (UI spec §5)

// computeView is duplicated from web/static/js/route/route_config.js (field
// [0,1]^2 -> letterbox square in CSS px). Keep in sync: a mismatch here would
// draw the path outside the real field and the server would reject it.
function computeView(vw, vh) {
  const avail = Math.max(120, vh - 150 - 190);
  const size = Math.max(120, Math.min(vw - 24, avail));
  return { x0: (vw - size) / 2, y0: 150 + Math.max(0, (avail - size) / 2), size };
}

const results = [];
function report(name, status, detail) {
  results.push({ name, status });
  console.log(name + ': ' + status + (detail ? ' - ' + detail : ''));
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

async function api(p, token, opts = {}) {
  const headers = Object.assign({ Authorization: 'Bearer ' + token }, opts.headers || {});
  try {
    const res = await fetch(BASE_URL + p, Object.assign({}, opts, { headers }));
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (e) { data = null; }
    return { status: res.status, data };
  } catch (e) {
    return { status: 0, data: null, error: String(e && e.message ? e.message : e) };
  }
}

// registerAndLogin — unique throwaway user (map-check.js precedent). There is no
// self-delete endpoint, so the user is left behind but clearly prefixed.
async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'e2e_route_' + Date.now() + '_' + attempt;
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

// clusterWorlds — single-star clusters from /api/worlds/filter around origin.
async function clusterWorlds(token, origin) {
  const R = 7000;
  const params = new URLSearchParams({
    x_min: (origin.x - R).toFixed(2), x_max: (origin.x + R).toFixed(2),
    y_min: (origin.y - R).toFixed(2), y_max: (origin.y + R).toFixed(2),
    cell: '40',
  });
  const r = await api('/api/worlds/filter?' + params.toString(), token);
  if (r.status !== 200 || !Array.isArray(r.data)) return null;
  return r.data;
}

// findFarWorld — closest single world at distance [minD, maxD] from origin,
// excluding ids. Null when nothing suitable exists (caller reports SKIP).
function findFarWorld(clusters, origin, exclude, minD, maxD) {
  let best = null;
  for (const c of clusters) {
    if (c.cnt !== 1 || !c.sid || exclude.has(c.sid)) continue;
    const d = Math.hypot(c.x - origin.x, c.y - origin.y);
    if (d < minD || d > maxD) continue;
    if (!best || d < best.dist) best = { sid: c.sid, name: c.sname || 'star', x: c.x, y: c.y, dist: d };
  }
  return best;
}

// greedyOrder — nearest-neighbour beacon order (same heuristic as
// route_config.greedyRouteLen). Visiting every beacon is what the server checks;
// the order only improves quality.
function greedyOrder(start, beacons) {
  const left = beacons.slice();
  const order = [];
  let cur = start;
  while (left.length) {
    let bi = 0, bd = Infinity;
    left.forEach((b, i) => {
      const d = Math.hypot(b.x - cur.x, b.y - cur.y);
      if (d < bd) { bd = d; bi = i; }
    });
    const b = left.splice(bi, 1)[0];
    order.push(b);
    cur = b;
  }
  return order;
}

// tap — one Pointer Events click on the canvas (pointerdown + pointerup at the
// same point). The game fixes a vertex on pointerup; the first tap must land in
// the START capture radius.
async function tap(page, x, y) {
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.up();
  await page.waitForTimeout(45);
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL + (QA_TOKEN ? ' (QA_TOKEN)' : ' (throwaway register)'));

  // ---------------- STEP 1: login + start a long interplanetary flight ----------------
  let token = null;
  let username = '';
  if (QA_TOKEN) {
    token = QA_TOKEN;
    username = '(QA_TOKEN)';
  } else {
    try {
      const creds = await registerAndLogin();
      token = creds.token;
      username = creds.username;
    } catch (err) {
      report('STEP 1', 'FAIL', 'register: ' + String(err && err.message ? err.message : err));
      return finish(1);
    }
  }

  let me = null;
  const meRes = await api('/me', token);
  if (meRes.status !== 200 || !meRes.data) {
    report('STEP 1', 'FAIL', 'GET /me HTTP ' + meRes.status);
    return finish(1);
  }
  me = meRes.data;
  if (!me.current_world_id) {
    report('STEP 1', 'FAIL', 'no current_world_id');
    return finish(1);
  }

  const wRes = await api('/worlds/' + me.current_world_id, token);
  const fromWorld = wRes.data && (wRes.data.world || wRes.data);
  if (!fromWorld || typeof fromWorld.coord_x !== 'number') {
    report('STEP 1', 'FAIL', 'cannot read current world coords');
    return finish(1);
  }
  const origin = { x: fromWorld.coord_x, y: fromWorld.coord_y };

  const clusters = await clusterWorlds(token, origin);
  if (!clusters) {
    report('STEP 1', 'FAIL', 'GET /api/worlds/filter failed');
    return finish(1);
  }
  // Distance >= 900 px at starter speed_factor 0.3 -> ~270 s, i.e. remaining >=
  // min_remaining_offer_s (180). Upper bound keeps the field at 3 beacons.
  const target1 = findFarWorld(clusters, origin, new Set([me.current_world_id]), 900, 5000);
  if (!target1) {
    report('STEP 1', 'SKIP', 'no single star at 900..5000 px from the spawn world');
    return finish(0);
  }

  const travel1 = await api('/travel', token, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ world_id: target1.sid }),
  });
  const accel1 = travel1.data && travel1.data.accelerator;
  const ok1 = travel1.status === 202 && accel1 && accel1.available === true && accel1.active === false;
  report('STEP 1', ok1 ? 'PASS' : 'FAIL',
    'user=' + username + ' target=' + target1.name + ' dist=' + target1.dist.toFixed(0) +
    ' dur=' + (travel1.data && travel1.data.duration) + 's available=' + (accel1 && accel1.available) +
    ' active=' + (accel1 && accel1.active) + ' reason=' + (accel1 && accel1.reason));
  if (!ok1) return finish(1);

  // ---------------- Browser setup ----------------
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

  const context = await browser.newContext({ viewport: VIEWPORT });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));

  try {
    // ---------------- STEP 2: route.html renders the field, no result leak ----------------
    await page.goto(BASE_URL + '/route.html', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('hud');
      return el && el.style.display !== 'none';
    }, { timeout: 30000 });
    await sleep(900); // settle the render loop

    const state2 = await page.evaluate(() => {
      const canvas = document.getElementById('route-canvas');
      const img = canvas.getContext('2d').getImageData(0, 0, canvas.width, canvas.height).data;
      let nonEmpty = 0;
      for (let i = 3; i < img.length; i += 4) if (img[i] > 0) nonEmpty++;
      const hud = document.getElementById('hud');
      return {
        path: location.pathname,
        nonEmpty,
        cw: canvas.width,
        ch: canvas.height,
        hudText: hud ? hud.textContent : '',
        resultShown: document.getElementById('result').style.display === 'flex',
      };
    });

    const leaked = [];
    if (/\d+\s*сек/i.test(state2.hudText)) leaked.push('seconds');
    if (/\d+\s*мин/i.test(state2.hudText)) leaked.push('minutes');
    if (/\bETA\b/i.test(state2.hudText)) leaked.push('eta');
    if (/\bbonus\b/i.test(state2.hudText)) leaked.push('bonus');
    if (/\bq\b/i.test(state2.hudText)) leaked.push('q');
    if (/Скорость\s*\+/i.test(state2.hudText)) leaked.push('speed+');
    if (/\d+\s*%/.test(state2.hudText)) leaked.push('percent');
    if (/Осталось/.test(state2.hudText)) leaked.push('remaining');

    const errs2 = pageErrors.slice();
    const ok2 = state2.path !== '/login-page' && state2.nonEmpty > 0 &&
      !state2.resultShown && leaked.length === 0 && errs2.length === 0;
    report('STEP 2', ok2 ? 'PASS' : 'FAIL',
      'canvas=' + state2.cw + 'x' + state2.ch + ' px=' + state2.nonEmpty +
      ' resultShown=' + state2.resultShown + ' leak=' + (leaked.join(',') || 'none') +
      ' pageErrors=' + errs2.length + (errs2.length ? ' first: ' + errs2[0] : ''));
    if (!ok2) return finish(1);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-field.png') });

    // ---------------- STEP 8: mobile viewport, safe-area, touch targets ----------------
    const geom = await page.evaluate(() => ({ vw: window.innerWidth, vh: window.innerHeight }));
    const view = computeView(geom.vw, geom.vh);
    const rect = await page.$eval('#route-canvas', (c) => {
      const r = c.getBoundingClientRect();
      return { left: r.left, top: r.top };
    });
    const fieldInside = view.x0 >= 0 && view.y0 >= 0 &&
      view.x0 + view.size <= geom.vw + 1 && view.y0 + view.size <= geom.vh + 1;

    const targets = await page.evaluate((ids) => {
      const out = {};
      for (const id of ids) {
        const el = document.getElementById(id);
        const r = el ? el.getBoundingClientRect() : null;
        out[id] = r ? Math.round(r.height) : -1;
      }
      return out;
    }, ['boost', 'undo', 'reset', 'to-map']);
    const targetsOk = Object.keys(targets).every((k) => targets[k] >= MIN_TOUCH);

    // Drawing works: tap START, then the first beacon -> at least one beacon captured.
    const offerForGeom = await api('/api/accelerator/offer', token);
    const field8 = offerForGeom.data && offerForGeom.data.field;
    let drawOk = false;
    let drawDetail = 'no offer field';
    if (field8) {
      const sx = (p) => rect.left + view.x0 + p.x * view.size;
      const sy = (p) => rect.top + view.y0 + p.y * view.size;
      const beacon0 = (field8.nodes || []).find((n) => n.type === 'beacon');
      await tap(page, sx(field8.start), sy(field8.start));
      if (beacon0) await tap(page, sx(beacon0), sy(beacon0));
      await sleep(120);
      const after = await page.$eval('#beacon-count', (el) => el.textContent);
      drawOk = /Маяки\s+[1-9]/.test(after);
      drawDetail = 'beacon-count="' + after.trim() + '"';
      await page.click('#reset');
      await sleep(120);
      const cleared = await page.$eval('#beacon-count', (el) => el.textContent);
      if (drawOk && !/Маяки\s+0/.test(cleared)) { drawOk = false; drawDetail += ' reset="' + cleared.trim() + '"'; }
    }
    const ok8 = fieldInside && targetsOk && drawOk;
    report('STEP 8', ok8 ? 'PASS' : 'FAIL',
      'viewport=' + geom.vw + 'x' + geom.vh + ' field=' + Math.round(view.x0) + ',' + Math.round(view.y0) +
      ' size=' + Math.round(view.size) + ' inside=' + fieldInside +
      ' targets=' + JSON.stringify(targets) + ' draw=' + drawDetail);
    if (!ok8) return finish(1);

    // ---------------- STEP 3: draw a valid path, submit via UI, verify shrink + active ----------------
    // Fresh offer: field + fingerprint + remaining just before the submit.
    const offerRes = await api('/api/accelerator/offer', token);
    const offer = offerRes.data;
    if (offerRes.status !== 200 || !offer || !offer.field) {
      report('STEP 3', 'FAIL', 'offer HTTP ' + offerRes.status + ' reason=' + (offer && offer.reason));
      return finish(1);
    }
    const field = offer.field;
    const beforeRemaining = offer.remaining_s;
    const beacons = (field.nodes || []).filter((n) => n.type === 'beacon');
    const fieldPath = [{ x: field.start.x, y: field.start.y }]
      .concat(greedyOrder(field.start, beacons).map((b) => ({ x: b.x, y: b.y })))
      .concat([{ x: field.finish.x, y: field.finish.y }]);

    // Real UI drawing: one Pointer Events tap per vertex (beacons are far enough
    // apart that MIN_STEP never drops one).
    const sx = (p) => rect.left + view.x0 + p.x * view.size;
    const sy = (p) => rect.top + view.y0 + p.y * view.size;
    for (const p of fieldPath) await tap(page, sx(p), sy(p));
    await sleep(250);

    const gateReady = await page.$eval('#boost', (b) => !b.disabled);
    let uiSubmit = false;
    let statusBoost = 0;
    let boostReason = '';
    let afterRemaining = null;
    if (gateReady) {
      const respWait = page.waitForResponse((r) => r.url().includes('/api/accelerator/boost'), { timeout: 8000 }).catch(() => null);
      await page.click('#boost');
      const resp = await respWait;
      if (resp) {
        statusBoost = resp.status();
        const d = await resp.json().catch(() => null);
        if (statusBoost === 200 && d) { uiSubmit = true; afterRemaining = d.remaining_s; }
        else boostReason = (d && d.reason) || '';
      }
    }
    if (!uiSubmit) {
      // Fallback per task: the path itself is valid; if the UI plumbing did not
      // submit, apply through the API and mark UI-submit as not covered.
      const fb = await api('/api/accelerator/boost', token, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ fingerprint: offer.fingerprint, path: fieldPath }),
      });
      statusBoost = fb.status;
      if (fb.status === 200 && fb.data) afterRemaining = fb.data.remaining_s;
      else boostReason = (fb.data && fb.data.reason) || '';
    }

    const meAfter = await api('/me', token);
    const activeAfter = !!(meAfter.data && meAfter.data.accelerator && meAfter.data.accelerator.active);
    const decrease = (typeof afterRemaining === 'number' && beforeRemaining > 0)
      ? (beforeRemaining - afterRemaining) / beforeRemaining : null;
    const shrinkOk = decrease !== null && decrease >= DECREASE_MIN && decrease <= DECREASE_MAX;
    const ok3 = afterRemaining !== null && shrinkOk && activeAfter;
    report('STEP 3', ok3 ? 'PASS' : 'FAIL',
      'uiSubmit=' + uiSubmit + (uiSubmit ? '' : ' (UI-submit NOT covered: fallback API)') +
      ' pathPoints=' + fieldPath.length + ' gate=' + gateReady +
      ' remaining ' + beforeRemaining + 's -> ' + afterRemaining + 's decrease=' +
      (decrease === null ? 'n/a' : Math.round(decrease * 100) + '%') +
      ' active=' + activeAfter + ' boostHTTP=' + statusBoost + ' reason=' + (boostReason || '-'));
    if (!ok3) return finish(1);

    if (uiSubmit) {
      await sleep(500);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-result.png') });
    }

    // ---------------- STEP 4: repeat in the same segment is refused ----------------
    await page.goto(BASE_URL + '/route.html', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('reason');
      return el && el.style.display === 'flex';
    }, { timeout: 20000 });
    const reason4 = await page.evaluate(() => ({
      title: document.getElementById('reason-title').textContent,
      note: document.getElementById('reason-note').textContent,
      loadingHidden: document.getElementById('loading').style.display === 'none',
    }));
    const titleOk = reason4.title.indexOf('Ускорение уже действует') >= 0;
    const ok4 = titleOk && activeAfter && reason4.loadingHidden;
    report('STEP 4', ok4 ? 'PASS' : 'FAIL',
      'reasonTitleMatch=' + titleOk + ' note=' + (reason4.note ? 'yes' : 'no') +
      ' loadingHidden=' + reason4.loadingHidden);
    if (!ok4) return finish(1);

    // ---------------- STEP 6: back on the map the boosted segment + button state ----------------
    // Run before STEP 5: a reversal would reset `active`.
    const pageMap = await context.newPage();
    pageMap.on('pageerror', (err) => pageErrors.push('map: ' + String(err && err.message ? err.message : err)));
    let ok6 = false;
    let step6Detail = '';
    try {
      await pageMap.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
      await pageMap.waitForSelector('#flight-panel.flying', { timeout: 30000 });
      await pageMap.waitForFunction(() => {
        const b = document.getElementById('flight-boost-btn');
        return b && b.textContent.indexOf('Ускорение уже действует') >= 0;
      }, { timeout: 30000 });
      const btn6 = await pageMap.$eval('#flight-boost-btn', (b) => ({ text: b.textContent, disabled: b.disabled }));
      const meMap = await api('/me', token);
      const flight = meMap.data && meMap.data.flight;
      const startChanged = flight && flight.start_time !== (travel1.data && travel1.data.start_time);
      const shorter = flight && typeof flight.duration === 'number' &&
        flight.duration < (travel1.data && travel1.data.duration) &&
        (afterRemaining === null || Math.abs(flight.duration - afterRemaining) <= 1);
      ok6 = btn6.disabled === true && startChanged === true && shorter === true;
      step6Detail = 'btnDisabled=' + btn6.disabled + ' btnTextMatch=' + (btn6.text.indexOf('Ускорение уже действует') >= 0) +
        ' startChanged=' + startChanged + ' dur=' + (flight && flight.duration) +
        ' (was ' + (travel1.data && travel1.data.duration) + ', boosted ' + afterRemaining + ')';
      await pageMap.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-map-boost.png') });
    } catch (err) {
      step6Detail = 'map/button: ' + String(err && err.message ? err.message : err);
    }
    report('STEP 6', ok6 ? 'PASS' : 'FAIL', step6Detail);
    if (!ok6) return finish(1);
    await pageMap.close();

    // ---------------- STEP 5: reversal (new /travel) resets `active` ----------------
    // Other far world (different from both the current target and the origin).
    const target2 = findFarWorld(clusters, origin, new Set([me.current_world_id, target1.sid]), 900, 6000);
    if (!target2) {
      report('STEP 5', 'SKIP', 'no second single star for a reversal; button-ready-after-cooldown not covered');
    } else {
      const travel2 = await api('/travel', token, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ world_id: target2.sid }),
      });
      const meRev = await api('/me', token);
      const activeRev = !!(meRev.data && meRev.data.accelerator && meRev.data.accelerator.active);
      const offerRev = await api('/api/accelerator/offer', token);
      const revReason = (offerRev.data && offerRev.data.reason) || ('HTTP ' + offerRev.status);
      const ok5 = travel2.status === 202 && activeRev === false;
      // "Button available again after the cooldown" is not reachable here: the
      // cooldown is 25 min, so offer stays refused with `cooldown`/`too_short`.
      report('STEP 5', ok5 ? 'PASS' : 'FAIL',
        'reversal=' + target2.name + ' travelHTTP=' + travel2.status + ' active=' + activeRev +
        ' offerReason=' + revReason + ' [button-ready-after-cooldown NOT covered: cooldown 25 min]');
      if (!ok5) return finish(1);
    }

    // ---------------- STEP 7: server restart mid-flight (explicit SKIP) ----------------
    report('STEP 7', 'SKIP', 'server restart mid-flight would drop the shared dev server (97a row check done by Go tests)');
  } catch (err) {
    report('run', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  // ---------------- page errors across the whole run ----------------
  report('pageerrors', pageErrors.length === 0 ? 'PASS' : 'FAIL',
    pageErrors.length === 0 ? 'none' : pageErrors.length + ' error(s), first: ' + pageErrors[0]);

  const failed = results.filter((r) => r.status === 'FAIL').length;
  const passed = results.filter((r) => r.status === 'PASS').length;
  const skipped = results.filter((r) => r.status === 'SKIP').length;
  console.log('');
  console.log('RESULT: ' + (failed ? 'FAIL (' + failed + ')' : 'ALL PASS') +
    ' - PASS=' + passed + ' SKIP=' + skipped + ' FAIL=' + failed);
  return finish(failed ? 1 : 0);
}

main();
