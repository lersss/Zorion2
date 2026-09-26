// tools/e2e/route-check.js
// Real browser e2e smoke for the accelerator mini-game "Route plotting" on the
// v9 board (UI spec docs/specs/2026-09-25-маршрут-мини-игра-интерфейс.md §9).
//
// Flow: register -> start a long interplanetary flight -> /route.html board ->
// legend popup (§4.9) -> tap a sector + scan (ping -1) -> draw a 4-connected cell
// path (START -> all beacons -> FINISH) by pointer drag -> two-tap confirm ->
// boost -> result.
// A separate step stubs the boost response with a negative bonus to prove the
// "Перелёт стал длиннее" / negative-percent UI (spec §4.7/§9 step 4).
//
// Mobile viewport 390x844, system Chrome/Edge via playwright-core (no browser
// download), throwaway user via /register (QA_TOKEN overrides the login).
//
// Run:  cd tools/e2e; npm.cmd i; node route-check.js
// Env:  BASE_URL (default http://localhost:8080), QA_TOKEN, CHROME_PATH, EDGE_PATH.
//
// Output is ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic).
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

const VIEWPORT = { width: 390, height: 844 };
const MIN_TOUCH = 44; // safe mobile touch target (UI spec §6)

// computeView is duplicated from web/static/js/route/route_config.js (letterbox
// square in CSS px). Keep in sync: a mismatch would draw the path outside the
// real board and the server would reject it. top/bottom reserve HUD areas.
function computeView(vw, vh, n) {
  const top = 118;
  const bottom = 232;
  const avail = Math.max(120, vh - top - bottom);
  const size = Math.max(120, Math.min(vw - 24, avail));
  return { x0: (vw - size) / 2, y0: top + Math.max(0, (avail - size) / 2), size, n: n || 0 };
}

function cellCenter(view, cell) {
  const cs = view.size / view.n;
  const i = cell % view.n;
  const j = (cell / view.n) | 0;
  return { x: view.x0 + (i + 0.5) * cs, y: view.y0 + (j + 0.5) * cs };
}

// bfsPath - shortest 4-connected chain from `from` to `to` (all cells passable).
function bfsPath(n, from, to) {
  if (from === to) return [from];
  const prev = new Array(n * n).fill(-2);
  prev[from] = -1;
  const q = [from];
  for (let h = 0; h < q.length; h++) {
    const c = q[h];
    if (c === to) break;
    const i = c % n;
    const j = (c / n) | 0;
    const nb = [];
    if (i > 0) nb.push(c - 1);
    if (i < n - 1) nb.push(c + 1);
    if (j > 0) nb.push(c - n);
    if (j < n - 1) nb.push(c + n);
    for (const x of nb) if (prev[x] === -2) { prev[x] = c; q.push(x); }
  }
  if (prev[to] === -2) return null;
  const out = [];
  let c = to;
  while (c !== -1) { out.push(c); c = prev[c]; }
  return out.reverse();
}

// routeCells - START -> all beacons (nearest-first) -> FINISH, concatenated
// shortest paths (any 4-connected chain visiting all beacons is server-valid;
// the order only affects quality). Revisits are allowed by the server.
function routeCells(n, start, beacons, finish) {
  let chain = [start];
  let cur = start;
  const left = beacons.slice();
  while (left.length) {
    let bi = 0;
    let bp = null;
    let bl = Infinity;
    left.forEach((b, i) => {
      const p = bfsPath(n, cur, b);
      if (p && p.length < bl) { bl = p.length; bi = i; bp = p; }
    });
    const b = left.splice(bi, 1)[0];
    for (const c of (bp || []).slice(1)) chain.push(c);
    cur = b;
  }
  const fp = bfsPath(n, cur, finish);
  if (fp) for (const c of fp.slice(1)) chain.push(c);
  return chain;
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

// registerAndLogin - unique throwaway user (map-check.js precedent). There is no
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

// clusterWorlds - single-star clusters from /api/worlds/filter around origin.
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

// findFarWorld - closest single world at distance [minD, maxD] from origin,
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

// noLeak - keys of the forbidden pre-submit data found in the HUD text.
function noLeak(hudText) {
  const leaked = [];
  if (/\d+\s*сек/i.test(hudText)) leaked.push('seconds');
  if (/\d+\s*мин/i.test(hudText)) leaked.push('minutes');
  if (/\bETA\b/i.test(hudText)) leaked.push('eta');
  if (/\bbonus\b/i.test(hudText)) leaked.push('bonus');
  if (/\bq\b/.test(hudText)) leaked.push('q');
  if (/\d+\s*%/.test(hudText)) leaked.push('percent');
  if (/Скорость\s*[+−-]/i.test(hudText)) leaked.push('speed');
  if (/Осталось/.test(hudText)) leaked.push('remaining');
  if (/оптимум/i.test(hudText)) leaked.push('optimum');
  if (/итог/i.test(hudText)) leaked.push('verdict');
  return leaked;
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function viewAndRect(page, board) {
  const geom = await page.evaluate(() => ({ vw: window.innerWidth, vh: window.innerHeight }));
  const view = computeView(geom.vw, geom.vh, board.n);
  const rect = await page.$eval('#route-canvas', (c) => {
    const r = c.getBoundingClientRect();
    return { left: r.left, top: r.top };
  });
  return { view, rect, geom };
}

// dragPath - pointer drag through every cell of the chain (real UI drawing).
async function dragPath(page, rect, view, chain) {
  const pt = (cell) => {
    const p = cellCenter(view, cell);
    return { x: rect.left + p.x, y: rect.top + p.y };
  };
  const first = pt(chain[0]);
  await page.mouse.move(first.x, first.y);
  await page.mouse.down();
  for (const cell of chain) {
    const p = pt(cell);
    await page.mouse.move(p.x, p.y, { steps: 1 });
  }
  await page.mouse.up();
  await sleep(200);
}

// dispatchDrag - pointer events fired straight on the canvas (bypasses the popup
// hit-testing). Used to prove the board input is frozen while the legend is open:
// if the freeze works, the full valid path never registers.
async function dispatchDrag(page, rect, view, chain) {
  const points = chain.map((cell) => {
    const p = cellCenter(view, cell);
    return { x: rect.left + p.x, y: rect.top + p.y };
  });
  await page.evaluate((pts) => {
    const c = document.getElementById('route-canvas');
    const fire = (type, p, buttons) => c.dispatchEvent(new PointerEvent(type, {
      bubbles: true, cancelable: true, clientX: p.x, clientY: p.y,
      pointerId: 1, pointerType: 'mouse', buttons,
    }));
    fire('pointerdown', pts[0], 1);
    for (const p of pts) fire('pointermove', p, 1);
    fire('pointerup', pts[pts.length - 1], 0);
  }, points);
  await sleep(200);
}

async function waitHud(page) {
  await page.waitForFunction(() => {
    const el = document.getElementById('hud');
    return el && el.style.display !== 'none';
  }, { timeout: 30000 });
  await sleep(600);
}

async function waitGate(page) {
  return page.waitForFunction(() => {
    const b = document.getElementById('boost');
    return b && !b.disabled;
  }, { timeout: 10000 }).then(() => true).catch(() => false);
}

// twoTapBoost - two-tap confirmation (§4.4) with the boost response awaited.
// opts.midSubmit runs after the confirming tap, before the response is awaited -
// lets a test observe the "Прокладываю…" submitting state (§3/§7.3).
async function twoTapBoost(page, opts = {}) {
  const respP = page.waitForResponse((r) => r.url().includes('/api/accelerator/boost'), { timeout: 15000 }).catch(() => null);
  await page.click('#boost');
  await page.waitForFunction(() => {
    const b = document.getElementById('boost');
    return b && b.textContent.indexOf('Проложить?') >= 0;
  }, { timeout: 3000 }).catch(() => null);
  await page.click('#boost');
  if (opts.midSubmit) await opts.midSubmit();
  return respP;
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

  const meRes = await api('/me', token);
  if (meRes.status !== 200 || !meRes.data) {
    report('STEP 1', 'FAIL', 'GET /me HTTP ' + meRes.status);
    return finish(1);
  }
  const me = meRes.data;
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
  // min_remaining_offer_s (180). Upper bound keeps the field small.
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
    const offerRes = await api('/api/accelerator/offer', token);
    const offer = offerRes.data;
    if (offerRes.status !== 200 || !offer || !offer.board) {
      report('offer', 'FAIL', 'HTTP ' + offerRes.status + ' reason=' + (offer && offer.reason));
      return finish(1);
    }
    const board = offer.board;
    const chain = routeCells(board.n, board.start, board.beacons || [], board.finish);
    const chainValid = chain.length >= 2 && chain[0] === board.start &&
      chain[chain.length - 1] === board.finish &&
      (board.beacons || []).every((b) => chain.includes(b));
    console.log('board: n=' + board.n + ' beacons=' + (board.beacons || []).length +
      ' sectors=' + (board.sectors || []).length + ' mode=' + board.mode +
      ' chain=' + chain.length + ' valid=' + chainValid);

    // ---------------- STEP 2: board renders, no result leak ----------------
    await page.goto(BASE_URL + '/route.html', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitHud(page);
    const state2 = await page.evaluate(() => {
      const canvas = document.getElementById('route-canvas');
      const img = canvas.getContext('2d').getImageData(0, 0, canvas.width, canvas.height).data;
      let nonEmpty = 0;
      for (let i = 3; i < img.length; i += 4) if (img[i] > 0) nonEmpty++;
      return {
        path: location.pathname,
        nonEmpty,
        cw: canvas.width,
        ch: canvas.height,
        hudText: document.getElementById('hud').textContent,
        resultShown: document.getElementById('result').style.display === 'flex',
      };
    });
    const leaked2 = noLeak(state2.hudText);
    const errs2 = pageErrors.slice();
    const ok2 = state2.path !== '/login-page' && state2.nonEmpty > 0 &&
      !state2.resultShown && leaked2.length === 0 && errs2.length === 0;
    report('STEP 2 board', ok2 ? 'PASS' : 'FAIL',
      'canvas=' + state2.cw + 'x' + state2.ch + ' px=' + state2.nonEmpty +
      ' resultShown=' + state2.resultShown + ' leak=' + (leaked2.join(',') || 'none') +
      ' pageErrors=' + errs2.length + (errs2.length ? ' first: ' + errs2[0] : ''));
    if (!ok2) return finish(1);

    // Mobile geometry + touch targets.
    const { view, rect, geom } = await viewAndRect(page, board);
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
    report('STEP 2 mobile', (fieldInside && targetsOk) ? 'PASS' : 'FAIL',
      'viewport=' + geom.vw + 'x' + geom.vh + ' board=' + Math.round(view.x0) + ',' + Math.round(view.y0) +
      ' size=' + Math.round(view.size) + ' inside=' + fieldInside + ' targets=' + JSON.stringify(targets));
    if (!(fieldInside && targetsOk)) return finish(1);

    // ---------------- STEP 2b: legend popup (§4.9 / §9 step 6) ----------------
    const statusBefore = await page.$eval('#status-line', (el) => el.textContent);
    await page.click('#legend-toggle');
    await sleep(250);
    // 24 mini-figures = landmarks 3 + field 8 + sector(1) + sig row(3) + marks 9;
    // all four groups collapsed by default (§4.9/§9 step 6), no colour swatches.
    const legend = await page.evaluate(() => {
      const pop = document.getElementById('legend-popup');
      const close = document.getElementById('legend-close');
      const figs = Array.from(document.querySelectorAll('.legend-fig'));
      const groups = Array.from(document.querySelectorAll('.legend-group'));
      const bodies = groups.map((g) => g.querySelector('.legend-group-body'));
      let painted = 0;
      for (const cv of figs) {
        const d = cv.getContext('2d').getImageData(0, 0, cv.width, cv.height).data;
        for (let i = 3; i < d.length; i += 4) { if (d[i] > 0) { painted++; break; } }
      }
      return {
        shown: pop.style.display === 'flex',
        rows: figs.length,
        painted,
        groupCount: groups.length,
        collapsed: bodies.length === 4 && bodies.every((b) => b && b.hidden) &&
          groups.every((g) => g.querySelector('.legend-group-title').getAttribute('aria-expanded') === 'false'),
        closeH: close ? Math.round(close.getBoundingClientRect().height) : -1,
        swatches: document.querySelectorAll('.legend-swatch').length,
        text: pop.textContent,
      };
    });
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-legend.png') });

    // Expand the first collapsed group by tap; it must open with its positions.
    await page.click('.legend-group-title');
    await sleep(150);
    const expanded = await page.evaluate(() => {
      const title = document.querySelector('.legend-group-title');
      const body = document.querySelector('.legend-group-body');
      return {
        open: !body.hidden,
        aria: title.getAttribute('aria-expanded') === 'true',
        rowsInGroup: body.querySelectorAll('.legend-row').length,
      };
    });
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-legend-open.png') });

    const legendLeak = noLeak(legend.text);
    const legendOk = legend.shown && legend.rows === 24 && legend.painted === legend.rows &&
      legend.groupCount === 4 && legend.collapsed && legend.closeH >= MIN_TOUCH &&
      legend.swatches === 0 && legendLeak.length === 0 &&
      expanded.open && expanded.aria && expanded.rowsInGroup === 3;

    // Frozen input while open: dispatch the full valid path straight on the canvas.
    await dispatchDrag(page, rect, view, chain);
    const frozenState = await page.evaluate(() => ({
      boostDisabled: document.getElementById('boost').disabled,
      status: document.getElementById('status-line').textContent,
    }));
    const frozenOk = frozenState.boostDisabled && frozenState.status === statusBefore;

    // Close via the "X" button; board input works again, then clear the path.
    await page.click('#legend-close');
    await sleep(150);
    const popupHidden = await page.$eval('#legend-popup', (el) => el.style.display === 'none');
    await dragPath(page, rect, view, chain);
    const restoredOk = await waitGate(page);
    if (restoredOk) await page.click('#reset');
    await sleep(150);
    report('STEP 2b legend', (legendOk && frozenOk && popupHidden && restoredOk) ? 'PASS' : 'FAIL',
      'shown=' + legend.shown + ' rows=' + legend.rows +
      ' painted=' + legend.painted + ' groups=' + legend.groupCount + ' collapsed=' + legend.collapsed +
      ' open=' + expanded.open + ' inGroup=' + expanded.rowsInGroup + ' closeH=' + legend.closeH +
      ' swatch=' + legend.swatches + ' leak=' + (legendLeak.join(',') || 'none') +
      ' frozen=' + frozenOk + ' closed=' + popupHidden + ' restored=' + restoredOk);
    if (!(legendOk && frozenOk && popupHidden && restoredOk)) return finish(1);

    // ---------------- STEP 3: negative result UI (boost response stubbed) ----------------
    // The stub holds the response briefly so the test can observe the "Прокладываю…"
    // submitting state (§3/§7.3): button text + disabled while the request is in flight.
    await page.route('**/api/accelerator/boost', async (route) => {
      await sleep(700);
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ bonus: -0.12, remaining_s: 180 }),
      });
    });
    await dragPath(page, rect, view, chain);
    const negGate = await waitGate(page);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-board.png') });
    let submitState = { text: '', disabled: false };
    const respNeg = negGate ? await twoTapBoost(page, {
      midSubmit: async () => {
        await sleep(150);
        submitState = await page.evaluate(() => {
          const b = document.getElementById('boost');
          return { text: b ? b.textContent : '', disabled: b ? b.disabled : false };
        });
      },
    }) : null;
    const negData = respNeg ? await respNeg.json().catch(() => null) : null;
    await sleep(300);
    const negView = await page.evaluate(() => ({
      shown: document.getElementById('result').style.display === 'flex',
      title: document.getElementById('result-title').textContent,
      bonus: document.getElementById('result-bonus').textContent,
      klass: document.getElementById('result-class').textContent,
      remaining: document.getElementById('result-remaining').textContent,
      breakdownHidden: document.getElementById('result-breakdown').hidden === true,
    }));
    const submittingOk = submitState.text.indexOf('Прокладываю') >= 0 && submitState.disabled === true;
    const negOk = negGate && respNeg && respNeg.status() === 200 && negView.shown &&
      negView.title.indexOf('Перелёт стал длиннее') >= 0 &&
      negView.bonus.indexOf('\u2212' + '12 %') >= 0 &&
      negView.bonus.indexOf('перелёт удлинился') >= 0 &&
      negView.remaining.indexOf('Осталось') >= 0 && negView.breakdownHidden && submittingOk;
    report('STEP 3 negative', negOk ? 'PASS' : 'FAIL',
      'gate=' + negGate + ' http=' + (respNeg && respNeg.status()) + ' body=' + JSON.stringify(negData) +
      ' title="' + negView.title + '" bonus="' + negView.bonus + '" klass="' + negView.klass +
      '" remaining="' + negView.remaining + '" breakdownHidden=' + negView.breakdownHidden +
      ' submitting="' + submitState.text + '" disabled=' + submitState.disabled);
    if (!negOk) return finish(1);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-result-negative.png') });
    await page.unroute('**/api/accelerator/boost');

    // ---------------- STEP 3b: breakdown with factors (stubbed) ----------------
    // Proves the factor rows: groups error->gain->neutral, glyph+name, severity
    // pip (no digits) only on errors, "xN" case counts, unknown code ignored.
    await page.route('**/api/accelerator/boost', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          bonus: 0.34, remaining_s: 1200,
          breakdown: [
            { code: 'ping_destabilize', group: 'error', severity: 3, count: 1, cells: [42] },
            { code: 'mud_cost', group: 'error', severity: 1, count: 3, cells: [7, 8, 17] },
            { code: 'find_used', group: 'gain', severity: 0, count: 1, cells: [60] },
            { code: 'wall_cost', group: 'neutral', severity: 0, count: 2, cells: [11, 20] },
            { code: 'unknown_future_code', group: 'error', severity: 2, count: 5, cells: [] },
          ],
        }),
      });
    });
    await page.goto(BASE_URL + '/route.html', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitHud(page);
    await dragPath(page, rect, view, chain);
    const bdGate = await waitGate(page);
    await twoTapBoost(page);
    await sleep(300);
    const bd = await page.evaluate(() => {
      const el = document.getElementById('result-breakdown');
      const groups = Array.from(el.querySelectorAll('.bd-group'));
      const rows = Array.from(el.querySelectorAll('.bd-row'));
      const pips = Array.from(el.querySelectorAll('.bd-pips'));
      return {
        visible: el.hidden === false,
        clean: !!el.querySelector('.bd-clean'),
        groups: groups.map((g) => g.querySelector('.bd-group-title').textContent),
        rows: rows.length,
        rowText: rows.map((r) => r.textContent).join(' | '),
        pips: pips.length,
        pipsDigits: pips.some((p) => /\d/.test(p.textContent)),
        count: (el.textContent.match(/\u00d7/g) || []).length,
        text: el.textContent,
      };
    });
    const groupsOrder = bd.groups.join('>') === 'Ошибки>Находки>Не ошибка';
    const bdOk = bdGate && bd.visible && !bd.clean && groupsOrder && bd.rows === 4 &&
      bd.pips === 2 && !bd.pipsDigits && bd.count === 4 &&
      bd.rowText.indexOf('\u00d73') >= 0 && bd.rowText.indexOf('Сопротивление') >= 0 &&
      bd.rowText.indexOf('Помехи') >= 0 && bd.text.indexOf('unknown_future_code') < 0 &&
      !/\d+\s*%/.test(bd.text);
    report('STEP 3b breakdown', bdOk ? 'PASS' : 'FAIL',
      'gate=' + bdGate + ' visible=' + bd.visible + ' groups="' + bd.groups.join(',') +
      '" rows=' + bd.rows + ' pips=' + bd.pips + ' pipsDigits=' + bd.pipsDigits +
      ' count=' + bd.count + ' text="' + bd.text + '"');
    if (!bdOk) return finish(1);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-breakdown.png') });
    await page.unroute('**/api/accelerator/boost');

    // ---------------- STEP 3c: empty breakdown -> "Чистый курс" (stubbed) ----------------
    await page.route('**/api/accelerator/boost', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ bonus: 0.2, remaining_s: 900, breakdown: [] }),
      });
    });
    await page.goto(BASE_URL + '/route.html', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitHud(page);
    await dragPath(page, rect, view, chain);
    const cleanGate = await waitGate(page);
    await twoTapBoost(page);
    await sleep(300);
    const clean = await page.evaluate(() => {
      const el = document.getElementById('result-breakdown');
      return {
        visible: el.hidden === false,
        clean: !!el.querySelector('.bd-clean'),
        text: el.textContent,
        rows: el.querySelectorAll('.bd-row').length,
      };
    });
    const cleanOk = cleanGate && clean.visible && clean.clean && clean.rows === 0 &&
      clean.text.indexOf('Чистый курс') >= 0;
    report('STEP 3c clean course', cleanOk ? 'PASS' : 'FAIL',
      'gate=' + cleanGate + ' visible=' + clean.visible + ' clean=' + clean.clean +
      ' rows=' + clean.rows + ' text="' + clean.text + '"');
    if (!cleanOk) return finish(1);
    await page.unroute('**/api/accelerator/boost');

    // ---------------- STEP 4: real scan (ping -1) ----------------
    await page.goto(BASE_URL + '/route.html', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitHud(page);
    await dragPath(page, rect, view, chain);
    const realGate = await waitGate(page);

    const sector0 = (board.sectors && board.sectors[0]) || null;
    let scanOk = false;
    let scanDetail = 'no sector';
    if (sector0 && sector0.cells && sector0.cells.length) {
      const p = cellCenter(view, sector0.cells[0]);
      await page.mouse.click(rect.left + p.x, rect.top + p.y);
      await sleep(200);
      const cardBefore = await page.evaluate(() => {
        const el = document.getElementById('sector-card');
        return { shown: el.style.display !== 'none', text: el.textContent };
      });
      const scanRespP = page.waitForResponse((r) => r.url().includes('/api/accelerator/scan'), { timeout: 10000 }).catch(() => null);
      await page.click('#sector-card [data-act="scan"]');
      const scanResp = await scanRespP;
      const scanData = scanResp ? await scanResp.json().catch(() => null) : null;
      await sleep(250);
      const after = await page.evaluate(() => ({
        card: document.getElementById('sector-card').textContent,
        pings: document.getElementById('pings-count').textContent,
      }));
      const pingsLeft = scanData && scanData.pings_left;
      const content = scanData && scanData.content;
      const contentShown = !!(content && after.card.indexOf('вскрыт') >= 0);
      scanOk = scanResp && scanResp.status() === 200 && pingsLeft === offer.pings_left - 1 &&
        contentShown && (after.pings.indexOf('●') >= 0);
      scanDetail = 'cardBefore=' + cardBefore.shown + ' http=' + (scanResp && scanResp.status()) +
        ' pings ' + offer.pings_left + '->' + pingsLeft + ' content=' + content +
        ' contentShown=' + contentShown + ' hudPings="' + after.pings.trim() + '"';
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-scan.png') });
    }
    report('STEP 4 scan', scanOk ? 'PASS' : 'FAIL', scanDetail);
    if (!scanOk) return finish(1);

    // No leak after scan, still before submit.
    const hudBefore = await page.$eval('#hud', (el) => el.textContent);
    const leaked4 = noLeak(hudBefore);
    report('STEP 4 no-leak', leaked4.length === 0 ? 'PASS' : 'FAIL',
      'leak=' + (leaked4.join(',') || 'none'));
    if (leaked4.length) return finish(1);

    // ---------------- STEP 5: real two-tap submit -> result ----------------
    const resp = realGate ? await twoTapBoost(page) : null;
    const body = resp ? await resp.json().catch(() => null) : null;
    await sleep(400);
    const realView = await page.evaluate(() => ({
      shown: document.getElementById('result').style.display === 'flex',
      title: document.getElementById('result-title').textContent,
      bonus: document.getElementById('result-bonus').textContent,
      remaining: document.getElementById('result-remaining').textContent,
      breakdownHidden: document.getElementById('result-breakdown').hidden === true,
      breakdownText: document.getElementById('result-breakdown').textContent,
    }));
    const b = body ? Number(body.bonus) : NaN;
    const negB = b < 0;
    const titleOk = negB ? realView.title.indexOf('Перелёт стал длиннее') >= 0
      : realView.title.indexOf('Ускорение принято') >= 0;
    const signOk = isFinite(b) &&
      realView.bonus.indexOf((negB ? '\u2212' : '+') + Math.round(Math.abs(b) * 100) + ' %') >= 0;
    // Server always sends breakdown (array; may be empty -> "Чистый курс"): the
    // block must be revealed after a successful boost.
    const bdFromServer = Array.isArray(body && body.breakdown);
    const bdShown = realView.breakdownHidden === false;
    const submitOk = realGate && resp && resp.status() === 200 && realView.shown &&
      titleOk && signOk && realView.remaining.indexOf('Осталось') >= 0 && bdFromServer && bdShown;
    report('STEP 5 submit', submitOk ? 'PASS' : 'FAIL',
      'gate=' + realGate + ' uiSubmit=' + !!(resp && resp.status() === 200) +
      ' http=' + (resp && resp.status()) + ' body=' + JSON.stringify(body) +
      ' title="' + realView.title + '" bonusText="' + realView.bonus + '"' +
      ' bdFromServer=' + bdFromServer + ' bdShown=' + bdShown +
      ' bdText="' + realView.breakdownText + '"');
    if (!submitOk) return finish(1);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'route-result.png') });

    // Flight state on the server: the boost is applied and active.
    const meAfter = await api('/me', token);
    const activeAfter = !!(meAfter.data && meAfter.data.accelerator && meAfter.data.accelerator.active);
    report('STEP 6 applied', activeAfter ? 'PASS' : 'FAIL',
      'accelerator.active=' + activeAfter + ' duration=' + (meAfter.data && meAfter.data.flight && meAfter.data.flight.duration));
    if (!activeAfter) return finish(1);

    // ---------------- STEP 7: same segment -> offer refused (already_active) ----------------
    await page.goto(BASE_URL + '/route.html', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('reason');
      return el && el.style.display === 'flex';
    }, { timeout: 20000 });
    const reason7 = await page.evaluate(() => ({
      title: document.getElementById('reason-title').textContent,
      loadingHidden: document.getElementById('loading').style.display === 'none',
    }));
    const ok7 = reason7.title.indexOf('Ускорение уже действует') >= 0 && reason7.loadingHidden;
    report('STEP 7 already_active', ok7 ? 'PASS' : 'FAIL',
      'titleOk=' + (reason7.title.indexOf('Ускорение уже действует') >= 0) +
      ' loadingHidden=' + reason7.loadingHidden);
    if (!ok7) return finish(1);
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
