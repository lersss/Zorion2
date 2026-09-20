// tools/e2e/admin-stop-btn-check.js
// Independent tester run: red stop button (#genStopBtn) next to "Generation"
// heading (idea 2026-09-20). P1-P5 hard checklist:
//   P1 no jobs -> button hidden
//   P2 start pacman (1 wps) -> button visible <=3s, progress shown
//   P3 F5 -> button + progress restored, status running, processed grows, no dup job
//   P4 click stop -> confirm -> button hidden <=3s, status canceled, result text,
//      polling stops (no more generate-status?job=pacman requests)
//   P5 restart after stop works, stop again (fast cycle)
// Run: node admin-stop-btn-check.js   (ASCII output only).
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

const ADMIN_TOKEN = process.env.ADMIN_TOKEN;
if (!ADMIN_TOKEN) {
  console.error('ADMIN_TOKEN env required: dev JWT-токен админа (см. tools/e2e/pacman-shot.js / README.md)');
  process.exit(1);
}
const ADMIN_AUTH = { Authorization: 'Bearer ' + ADMIN_TOKEN };

const ALL_JOBS = ['generate_universe', 'generate_planets', 'generate_factions', 'generate_resources',
  'generate_race_settlements', 'regenerate_planets', 'generate_npc', 'hypothesis', 'pacman'];

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
async function admin(path, options) {
  const res = await fetch(BASE_URL + path, { ...options, headers: { ...ADMIN_AUTH, ...(options && options.headers) } });
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch (e) { /* not json */ }
  return { status: res.status, json, text };
}
async function jobStatus(job) {
  const { json } = await admin('/admin/generate-status?job=' + job);
  return json || {};
}
function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });

  // --- Precondition: no running jobs ---
  const running = [];
  for (const j of ALL_JOBS) {
    const s = await jobStatus(j);
    if (s.status === 'running') running.push(j + ':' + (s.processed || 0));
  }
  if (running.length) {
    report('PRE', 'FAIL', 'running jobs at start: ' + running.join(', '));
    return finish(1);
  }
  report('PRE', 'PASS', 'no running jobs at start');

  const exe = findExecutable();
  if (!exe) { report('BROWSER', 'FAIL', 'no Chrome/Edge'); return finish(1); }
  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('BROWSER', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => {
    localStorage.setItem('adminToken', t);
    localStorage.setItem('adminActiveTab', 'tab-generation');
  }, ADMIN_TOKEN);
  const page = await context.newPage();

  const pageErrors = [];
  const consoleErrors = [];
  const badResponses = []; // {status, url} for HTTP >= 400
  const pacmanPollRequests = []; // {t, url}
  const allRequests = []; // {t, method, url}
  const dialogs = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));
  page.on('console', (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text().slice(0, 200)); });
  page.on('response', (res) => {
    if (res.status() >= 400) badResponses.push({ status: res.status(), url: res.url().replace(BASE_URL, '') });
  });
  page.on('request', (req) => {
    const u = req.url();
    if (u.includes('generate-status?job=pacman')) pacmanPollRequests.push({ t: Date.now(), url: u });
    if (u.includes('/admin/')) allRequests.push({ t: Date.now(), method: req.method(), url: u.replace(BASE_URL, '') });
  });
  page.on('dialog', (d) => { dialogs.push(d.message()); d.accept(); });

  const shot = (name) => page.screenshot({ path: path.join(ARTIFACTS_DIR, name) }).catch(() => {});

  try {
    // ================= P1 =================
    await page.goto(BASE_URL + '/admin', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#genStopBtn', { state: 'attached', timeout: 15000 });
    await page.waitForTimeout(2500); // let restoreJobStates finish (9 status queries)

    const p1 = await page.evaluate(() => {
      const btn = document.getElementById('genStopBtn');
      const cancelBtns = ['cancelUniverseBtn', 'cancelPlanetsBtn', 'cancelFactionsBtn',
        'cancelResourcesBtn', 'cancelRaceSettlementsBtn', 'cancelRegenerateBtn',
        'cancelHypothesisBtn', 'cancelPacmanBtn'];
      const visibleCancels = cancelBtns.filter(id => {
        const el = document.getElementById(id);
        return el && getComputedStyle(el).display !== 'none';
      });
      return {
        stopDisplay: btn ? getComputedStyle(btn).display : 'MISSING',
        visibleCancels,
      };
    });
    const p1ok = p1.stopDisplay === 'none' && p1.visibleCancels.length === 0;
    report('P1', p1ok ? 'PASS' : 'FAIL',
      'genStopBtn display=' + p1.stopDisplay + ' visibleCancelBtns=' + JSON.stringify(p1.visibleCancels));
    if (!p1ok) return finish(1);
    await shot('p1-no-jobs.png');

    // ================= P2 =================
    await page.fill('#pacmanSpeed', '1');
    await page.click('#pacmanStartBtn');
    // wait for API running
    let st = {};
    for (let i = 0; i < 20; i++) {
      st = await jobStatus('pacman');
      if (st.status === 'running') break;
      await sleep(500);
    }
    if (st.status !== 'running') {
      report('P2', 'FAIL', 'pacman did not become running: ' + JSON.stringify(st));
      return finish(1);
    }
    // within <=3s the red button must be visible + progress shown
    let p2 = null;
    for (let i = 0; i < 6; i++) {
      p2 = await page.evaluate(() => {
        const btn = document.getElementById('genStopBtn');
        const prog = document.getElementById('pacmanProgress');
        return {
          stopDisplay: btn ? getComputedStyle(btn).display : 'MISSING',
          progressDisplay: prog ? getComputedStyle(prog).display : 'MISSING',
          progressText: document.getElementById('pacmanProgressText') ? document.getElementById('pacmanProgressText').textContent : '',
        };
      });
      if (p2.stopDisplay !== 'none' && p2.progressDisplay !== 'none') break;
      await sleep(500);
    }
    const p2ok = p2.stopDisplay !== 'none' && p2.progressDisplay !== 'none';
    report('P2', p2ok ? 'PASS' : 'FAIL',
      'status=' + st.status + ' processed=' + (st.processed || 0) +
      ' genStopBtn=' + p2.stopDisplay + ' pacmanProgress=' + p2.progressDisplay +
      ' text="' + p2.progressText + '"');
    if (!p2ok) return finish(1);
    await shot('p2-running.png');

    // ================= P3 (F5) =================
    const processedBeforeReload = st.processed || 0;
    await page.reload({ waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#genStopBtn', { state: 'attached', timeout: 15000 });
    // wait for restore: button visible + progress shown + text updated by poll
    let p3 = null;
    for (let i = 0; i < 12; i++) {
      p3 = await page.evaluate(() => {
        const btn = document.getElementById('genStopBtn');
        const prog = document.getElementById('pacmanProgress');
        return {
          stopDisplay: btn ? getComputedStyle(btn).display : 'MISSING',
          progressDisplay: prog ? getComputedStyle(prog).display : 'MISSING',
          progressText: document.getElementById('pacmanProgressText') ? document.getElementById('pacmanProgressText').textContent : '',
        };
      });
      if (p3.stopDisplay !== 'none' && p3.progressDisplay !== 'none' && /из \d+/.test(p3.progressText)) break;
      await sleep(500);
    }
    // API: running + processed grows (two samples)
    const s1 = await jobStatus('pacman');
    await sleep(2500);
    const s2 = await jobStatus('pacman');
    const p3ok = p3.stopDisplay !== 'none' && p3.progressDisplay !== 'none' &&
      s1.status === 'running' && s2.status === 'running' &&
      (s2.processed || 0) > (s1.processed || 0) &&
      (s1.processed || 0) >= processedBeforeReload;
    report('P3', p3ok ? 'PASS' : 'FAIL',
      'genStopBtn=' + p3.stopDisplay + ' progress=' + p3.progressDisplay + ' text="' + p3.progressText + '"' +
      ' processed beforeReload=' + processedBeforeReload + ' s1=' + (s1.processed || 0) + ' s2=' + (s2.processed || 0) +
      ' status=' + s1.status + '/' + s2.status +
      ' adminRequests=' + JSON.stringify(allRequests));
    if (!p3ok) return finish(1);
    await shot('p3-after-f5.png');

    // ================= P4 (stop via red button) =================
    const pollCountBefore = pacmanPollRequests.length;
    await page.click('#genStopBtn');
    await sleep(300); // let the confirm + POST fire
    // wait <=3s for button hidden
    let p4 = null;
    for (let i = 0; i < 6; i++) {
      p4 = await page.evaluate(() => {
        const btn = document.getElementById('genStopBtn');
        const prog = document.getElementById('pacmanProgress');
        return {
          stopDisplay: btn ? getComputedStyle(btn).display : 'MISSING',
          progressDisplay: prog ? getComputedStyle(prog).display : 'MISSING',
          resultText: document.getElementById('pacmanResult') ? document.getElementById('pacmanResult').textContent : '',
        };
      });
      if (p4.stopDisplay === 'none') break;
      await sleep(500);
    }
    const stCancel = await jobStatus('pacman');
    // polling must stop: no new pacman status requests for 4s after cancel
    const tCancel = Date.now();
    await sleep(4000);
    const newPolls = pacmanPollRequests.filter(r => r.t > tCancel).length;
    const p4ok = p4.stopDisplay === 'none' &&
      stCancel.status === 'canceled' &&
      /Остановлено/.test(p4.resultText) &&
      newPolls === 0;
    report('P4', p4ok ? 'PASS' : 'FAIL',
      'genStopBtn=' + p4.stopDisplay + ' progress=' + p4.progressDisplay +
      ' result="' + p4.resultText + '" apiStatus=' + stCancel.status +
      ' dialogs=' + dialogs.length + ' newPollsAfterCancel=' + newPolls +
      ' consoleErrors=' + consoleErrors.length + ' pageErrors=' + pageErrors.length +
      (consoleErrors.length ? ' consoleErr[0]="' + consoleErrors[0] + '"' : ''));
    if (!p4ok) return finish(1);
    await shot('p4-canceled.png');

    // ================= P5 (restart + stop again, fast cycle) =================
    await page.fill('#pacmanSpeed', '1');
    await page.click('#pacmanStartBtn');
    let st5 = {};
    for (let i = 0; i < 20; i++) {
      st5 = await jobStatus('pacman');
      if (st5.status === 'running') break;
      await sleep(500);
    }
    if (st5.status !== 'running') {
      report('P5', 'FAIL', 'restart did not become running: ' + JSON.stringify(st5));
      return finish(1);
    }
    // red button visible again
    let btn5 = null;
    for (let i = 0; i < 6; i++) {
      btn5 = await page.evaluate(() => {
        const b = document.getElementById('genStopBtn');
        return b ? getComputedStyle(b).display : 'MISSING';
      });
      if (btn5 !== 'none') break;
      await sleep(500);
    }
    if (btn5 === 'none') {
      report('P5', 'FAIL', 'red button not visible after restart (display=none)');
      return finish(1);
    }
    // stop again
    await page.click('#genStopBtn');
    await sleep(300);
    let p5 = null;
    for (let i = 0; i < 6; i++) {
      p5 = await page.evaluate(() => {
        const b = document.getElementById('genStopBtn');
        return b ? getComputedStyle(b).display : 'MISSING';
      });
      if (p5 === 'none') break;
      await sleep(500);
    }
    const st5c = await jobStatus('pacman');
    const p5ok = p5 === 'none' && st5c.status === 'canceled';
    report('P5', p5ok ? 'PASS' : 'FAIL',
      'restart status=' + st5.status + ' processed=' + (st5.processed || 0) +
      ' btnVisible=' + (btn5 !== 'none') + ' afterStop=' + p5 + ' apiStatus=' + st5c.status +
      ' dialogs=' + dialogs.length);
    if (!p5ok) return finish(1);
    await shot('p5-restarted-then-stopped.png');

  } catch (err) {
    report('RUN', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const failed = results.filter(r => r.status === 'FAIL');
  console.log('');
  if (consoleErrors.length) {
    console.log('CONSOLE ERRORS (' + consoleErrors.length + '):');
    consoleErrors.forEach((e, i) => console.log('  [' + i + '] ' + e));
  } else {
    console.log('CONSOLE ERRORS: none');
  }
  if (pageErrors.length) {
    console.log('PAGE ERRORS (' + pageErrors.length + '):');
    pageErrors.forEach((e, i) => console.log('  [' + i + '] ' + e));
  }
  if (badResponses.length) {
    console.log('BAD RESPONSES (' + badResponses.length + '):');
    badResponses.forEach((b, i) => console.log('  [' + i + '] ' + b.status + ' ' + b.url));
  }
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'DONE'));
  return finish(failed.length ? 1 : 0);
}

main();