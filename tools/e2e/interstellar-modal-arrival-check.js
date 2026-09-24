// tools/e2e/interstellar-modal-arrival-check.js
// Focused browser check for bug 2026-09-24: a system modal opened DURING an
// interstellar flight stayed in "composite" mode after arrival, so "Лететь"
// hit /travel with destination -> server 400 "Вы уже в этой системе".
//
// The update is event-driven: the map catches interstellar arrival in
// map/animation.js -> loadUserData -> checkCompositeArrival (data.js) and asks
// the open modal to refresh. The real flight would take minutes, so /me and
// /api/worlds/{id}/planets are stubbed (Playwright route) with a synthetic
// flight + system state; the map animation loop and checkCompositeArrival run
// unmodified.
//
// Run: node interstellar-modal-arrival-check.js   (BASE_URL overrides)
// ASCII output only (Windows PowerShell cp866 breaks Cyrillic).
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');

const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

const results = [];
function report(stepName, status, detail) {
  results.push({ stepName, status, detail });
  console.log(`[${stepName}] ${status}${detail ? ' - ' + detail : ''}`);
}

async function register() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'e2e_arr_' + Date.now() + '_' + attempt;
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password: 'e2e-pass-' + Date.now() }),
    });
    if (res.status === 201) return (await res.json()).token;
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
  console.log('BASE_URL: ' + BASE_URL);

  let token;
  try {
    token = await register();
    report('1/6 setup token', 'PASS', 'OK');
  } catch (err) {
    report('1/6 setup token', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const auth = { Authorization: 'Bearer ' + token };
  const meRes = await fetch(BASE_URL + '/me', { headers: auth });
  const realMe = await meRes.json();
  const currentId = realMe.current_world_id;
  if (!currentId) { report('2/6 setup data', 'FAIL', 'no current_world_id'); return finish(1); }

  // Planet body shape (real server data). We stub the target system's planets
  // with this shape, flipping in_own_system/my_position per phase.
  const planetsRes = await fetch(BASE_URL + '/api/worlds/' + currentId + '/planets', { headers: auth });
  const realPlanets = planetsRes.ok ? await planetsRes.json() : { planets: [] };

  // Target star: any other single star from the map filter (fallback: current).
  let targetId = currentId, targetName = realMe.current_world_name || 'Target', targetSpec = 'G';
  try {
    const wRes = await fetch(BASE_URL + '/worlds/' + currentId, { headers: auth });
    if (wRes.ok) {
      const w = (await wRes.json()).world || {};
      const px = w.coord_x || 0, py = w.coord_y || 0;
      const R = 4000;
      const params = new URLSearchParams({
        x_min: (px - R).toFixed(3), x_max: (px + R).toFixed(3),
        y_min: (py - R).toFixed(3), y_max: (py + R).toFixed(3), cell: '40',
      });
      const fRes = await fetch(BASE_URL + '/api/worlds/filter?' + params.toString(), { headers: auth });
      if (fRes.ok) {
        const clusters = await fRes.json();
        const other = clusters.find(c => c.cnt === 1 && c.sid && c.sid !== currentId);
        if (other) { targetId = other.sid; targetName = other.sname || targetName; targetSpec = other.sspec || targetSpec; }
      }
    }
  } catch (e) { /* keep current world as target */ }
  report('2/6 setup data', 'PASS', 'target=' + targetId + ' planets=' + (realPlanets.planets || []).length);

  const exe = findExecutable();
  if (!exe) { report('3/6 setup browser', 'FAIL', 'no Chrome/Edge found'); return finish(1); }
  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('3/6 setup browser', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));

  // phase: 'map' (real traffic) | 'flight' (stub: interstellar active) | 'arrived'
  let phase = 'map';
  let flight = null;

  await page.route('**/me', async (route) => {
    if (phase === 'map') return route.continue();
    // 'flight': still en route to target. 'arrived': current world = target,
    // no flight (server ArrivalHandler already moved the player).
    const body = phase === 'arrived'
      ? Object.assign({}, realMe, { current_world_id: targetId, flight: null, current_position: null,
                                    pending_destination: null })
      : Object.assign({}, realMe, { flight: flight, current_position: null });
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
  await page.route('**/api/worlds/' + targetId + '/planets', async (route) => {
    if (phase === 'map') return route.continue();
    const base = Object.assign({}, realPlanets);
    if (phase === 'flight') {
      base.in_own_system = false;
      base.my_position = null;
    } else {
      base.in_own_system = true;
      base.my_position = { status: 'orbit', object_type: 'star', object_id: targetId };
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(base) });
  });

  // readState — the exact module state the modal buttons read (dynamic import
  // of the served ES module, no duplication of logic).
  const readState = () => page.evaluate(async () => {
    const m = await import('/static/js/modal/state.js');
    return {
      mode: m.flightModeForSystem(),
      interstellar: !!m.modalState.interstellarFlight,
      inOwnSystem: !!m.modalState.inOwnSystem,
      posStatus: m.modalState.myPosition ? m.modalState.myPosition.status : null,
    };
  });
  const callRefresh = () => page.evaluate(async () => {
    const m = await import('/static/js/modal/index.js');
    m.refreshPlanets();
  });
  const setFlightState = (opts) => page.evaluate(async (o) => {
    const m = await import('/static/js/modal/state.js');
    m.modalState.interstellarFlight = o.flight;
    m.modalState.interstellarFlightName = o.name || null;
    m.modalState.inOwnSystem = false;
    m.modalState.myPosition = null;
  }, opts);
  // armMapFlight — make the map animation loop think an interstellar flight just
  // finished (elapsed >= duration), so its real arrival branch runs
  // (loadUserData -> checkCompositeArrival). No server flight needed.
  const armMapFlight = (target) => page.evaluate(async (t) => {
    const cfg = await import('/static/js/map/config.js');
    cfg.state.isFlying = true;
    cfg.state.flyStartTime = Date.now() - 5000;
    cfg.state.flyDuration = 1;
    cfg.state.flyFrom = { id: 'src', name: 'Src' };
    cfg.state.flyTo = { id: t, name: 'Dst' };
  }, target);
  const closeModal = () => page.evaluate(async () => {
    const m = await import('/static/js/modal/index.js');
    m.closeModal();
  });
  const waitArrival = async (timeoutMs) => {
    const t0 = Date.now();
    const timeline = [];
    while (Date.now() - t0 < timeoutMs) {
      const st = await readState();
      timeline.push(`${((Date.now() - t0) / 1000).toFixed(1)}s:${st.mode}${st.interstellar ? '+fl' : ''}${st.inOwnSystem ? '+own' : ''}${st.posStatus ? ':' + st.posStatus : ''}`);
      if (!st.interstellar && st.mode === 'intra') return { ok: true, timeline, final: st };
      await page.waitForTimeout(250);
    }
    return { ok: false, timeline, final: await readState() };
  };

  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(800);

    // --- Step 4: open modal DURING interstellar flight -> composite mode ---
    phase = 'flight';
    flight = { from: currentId, to: targetId, start_time: Date.now(), duration: 60, start_x: 0, start_y: 0 };
    await page.evaluate((o) => {
      window.openSystemModal(o.id, o.name, o.spec, null, null, { stype: 'star', systype: 'single', smods: {}, x: 0, y: 0, hasEngine: true });
    }, { id: targetId, name: targetName, spec: targetSpec });
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForTimeout(1200);
    const s4 = await readState();
    const ok4 = s4.mode === 'composite' && s4.interstellar && !s4.inOwnSystem;
    report('4/6 modal in flight = composite', ok4 ? 'PASS' : 'FAIL',
      `mode=${s4.mode} interstellar=${s4.interstellar} inOwnSystem=${s4.inOwnSystem} pos=${s4.posStatus}`);
    if (!ok4) return finish(1);

    // --- Step 5: "Обновить" while still flying does NOT clear (req 4) ---
    await callRefresh();
    await page.waitForTimeout(800);
    const s5 = await readState();
    const ok5 = s5.mode === 'composite' && s5.interstellar;
    report('5/6 refresh mid-flight stays composite', ok5 ? 'PASS' : 'FAIL',
      `mode=${s5.mode} interstellar=${s5.interstellar}`);
    if (!ok5) return finish(1);

    // --- Step 6a: arrival event (map) updates the OPEN modal, no manual action ---
    // Arm the real map arrival branch: map/animation.js -> loadUserData ->
    // checkCompositeArrival (no composite signs) -> refreshPlanets on open modal.
    phase = 'arrived';
    await armMapFlight(targetId);
    const arr = await waitArrival(20000);
    report('6/6a map arrival event = intra', arr.ok ? 'PASS' : 'FAIL',
      `mode=${arr.final.mode} interstellar=${arr.final.interstellar} inOwnSystem=${arr.final.inOwnSystem} | ${arr.timeline.join(' ')}`);
    if (!arr.ok) return finish(1);
    await page.screenshot({ path: 'artifacts/interstellar-arrival-intra.png' });

    // --- Step 6b: manual "Обновить" after arrival also clears (req 3) ---
    await setFlightState({
      flight: { from: currentId, to: targetId, start_time: Date.now() - 8000, duration: 10, start_x: 0, start_y: 0 },
      name: targetName,
    });
    const pre6b = await readState();
    await callRefresh();
    await page.waitForTimeout(900);
    const s6b = await readState();
    const ok6b = pre6b.mode === 'composite' && pre6b.interstellar &&
      s6b.mode === 'intra' && !s6b.interstellar && s6b.posStatus === 'orbit';
    report('6/6b manual refresh after arrival = intra', ok6b ? 'PASS' : 'FAIL',
      `pre=${pre6b.mode}${pre6b.interstellar ? '+fl' : ''} -> mode=${s6b.mode} interstellar=${s6b.interstellar} pos=${s6b.posStatus}`);
    if (!ok6b) return finish(1);

    // --- Step 6c: simple flight, modal CLOSED -> no auto-open ---
    await closeModal();
    await page.waitForTimeout(400);
    const closed = await page.evaluate(() => !document.getElementById('system-modal-overlay'));
    await page.evaluate(() => { try { sessionStorage.removeItem('compositeRoute'); } catch (e) {} });
    await armMapFlight(targetId);
    await page.waitForTimeout(6000);
    const reopened = await page.evaluate(() => !!document.getElementById('system-modal-overlay'));
    const ok6c = closed && !reopened;
    report('6/6c simple flight, closed modal stays closed', ok6c ? 'PASS' : 'FAIL',
      `closedBefore=${closed} reopened=${reopened}`);
    if (!ok6c) return finish(1);

    await page.screenshot({ path: 'artifacts/interstellar-arrival-intra.png' });

    const errs = pageErrors.slice();
    report('page errors', errs.length === 0 ? 'PASS' : 'FAIL', errs.length ? errs.join(' | ') : '0 errors');
    if (errs.length) return finish(1);
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
