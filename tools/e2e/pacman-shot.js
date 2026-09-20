// tools/e2e/pacman-shot.js
// Full pacman visual proof: (1) register a player, open the map, (2) generate
// worlds via admin, (3) start a slow pacman, (4) wait for the banner via WS
// (late-client activation fix), (5) zoom out and take a screenshot series.
// Run: node pacman-shot.js   (ASCII output only).
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

// Admin JWT for pacman_smoke (dev-only; generated 2026-09-20).
const ADMIN_TOKEN = process.env.ADMIN_TOKEN || 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiYTUyZTMxMjAtMWJhMS00Mzg5LTk3OWUtMDkzZTM1NDJkNzVkIiwicm9sZSI6ImFkbWluIiwiZXhwIjoxNzg5OTQzNDM0LCJpYXQiOjE3ODk4NTcwMzR9.BKKAuOG8ZgD1JWyaoEUn8i155_6vXwXNVvNLy_1iBiI';
const ADMIN_AUTH = { Authorization: 'Bearer ' + ADMIN_TOKEN };

const SHOTS = parseInt(process.env.PACMAN_SHOTS || '8', 10);
const INTERVAL_MS = parseInt(process.env.PACMAN_INTERVAL_MS || '4000', 10);
const WORLD_COUNT = parseInt(process.env.PACMAN_WORLDS || '4000', 10);
const WPS = parseInt(process.env.PACMAN_WPS || '100', 10);

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
async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'e2e_' + Date.now() + '_' + attempt;
    const password = 'e2e-pass-' + Date.now();
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (res.status === 201) { const d = await res.json(); return { username, token: d.token }; }
    if (res.status === 409) continue;
    throw new Error('register HTTP ' + res.status + ': ' + (await res.text()));
  }
  throw new Error('register: collision after 3 attempts');
}
async function admin(path, options) {
  const res = await fetch(BASE_URL + path, { ...options, headers: { ...ADMIN_AUTH, ...(options && options.headers) } });
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch (e) { /* not json */ }
  return { status: res.status, json, text };
}
async function waitJob(job, timeoutMs) {
  const t0 = Date.now();
  while (Date.now() - t0 < timeoutMs) {
    const { json } = await admin('/admin/generate-status?job=' + job);
    if (!json) { await sleep(1000); continue; }
    if (json.status === 'done' || json.status === 'error' || json.status === 'canceled') return json;
    await sleep(2000);
  }
  return null;
}
function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  let token = null;
  try {
    const creds = await registerAndLogin();
    token = creds.token;
    report('1/7 register/login', 'PASS', 'user ' + creds.username);
  } catch (err) {
    report('1/7 register/login', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  // --- Generate worlds if the galaxy is empty ---
  let worldStatus = 'generated';
  const stats = await admin('/admin/stats');
  if (stats.json && (stats.json.worlds || 0) === 0) {
    const g = await admin('/admin/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        world_count: WORLD_COUNT, cluster_count: 30, map_size: 8000, min_dist: 150,
        cluster_radius: 1200, cluster_spacing: 200, outlier_percent: 30, shape: 'blob',
      }),
    });
    if (g.status !== 202 && g.status !== 200) {
      report('2/7 generate worlds', 'FAIL', 'HTTP ' + g.status + ': ' + g.text);
      return finish(1);
    }
    const fin = await waitJob('generate_universe', 180000);
    if (!fin || fin.status !== 'done') {
      report('2/7 generate worlds', 'FAIL', 'job not done: ' + JSON.stringify(fin));
      return finish(1);
    }
    worldStatus = 'generated ' + (fin.processed || WORLD_COUNT);
  } else {
    worldStatus = 'existing ' + (stats.json ? stats.json.worlds : '?');
  }
  report('2/7 generate worlds', 'PASS', worldStatus);

  const exe = findExecutable();
  if (!exe) { report('3/7 browser', 'FAIL', 'no Chrome/Edge'); return finish(1); }
  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('3/7 browser', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));

  // WS frame log (diagnostics: any ws event + which pacman_* frames arrive).
  const wsFrames = [];
  const wsEvents = [];
  page.on('websocket', (ws) => {
    wsEvents.push('ws-open ' + ws.url());
    ws.on('open', () => wsEvents.push('ws-OPEN'));
    ws.on('close', () => wsEvents.push('ws-CLOSE'));
    ws.on('socketerror', () => wsEvents.push('ws-ERROR'));
    ws.on('framereceived', (f) => {
      const s = String(f.payload || '');
      wsFrames.push(s.slice(0, 100));
    });
  });
  const consoleMsgs = [];
  page.on('console', (msg) => { consoleMsgs.push(msg.type() + ': ' + msg.text().slice(0, 120)); });

  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(3000); // let the WS connect (startNPCLoop)

    report('4/7 map open', 'PASS', 'canvas ok, wsFrames so far: ' + wsFrames.length);

    // --- Start the pacman (slow), or reuse an already-running one ---
    const st = await admin('/admin/generate-status?job=pacman');
    let started = false;
    if (st.json && st.json.status === 'running') {
      started = true;
      report('5/7 pacman start', 'PASS', 'already running (' + (st.json.processed || 0) + '/' + (st.json.total || 0) + ')');
    } else {
      const p = await admin('/admin/pacman/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ worlds_per_second: WPS, batch_size: 100, trajectory: 'spiral' }),
      });
      if (p.status === 409 && p.text.includes('eating')) {
        started = true;
        report('5/7 pacman start', 'PASS', 'already eating (409)');
      } else if (p.status !== 202 && p.status !== 200) {
        report('5/7 pacman start', 'FAIL', 'HTTP ' + p.status + ': ' + p.text);
        return finish(1);
      } else {
        started = true;
        report('5/7 pacman start', 'PASS', 'HTTP ' + p.status + ' ' + (p.text || ''));
      }
    }

    // --- Wait for the banner (late-client activation fix) ---
    let bannerSeen = false;
    try {
      await page.waitForFunction(() => {
        const b = document.getElementById('pacman-banner');
        return !!b && b.style.display !== 'none';
      }, { timeout: 60000 });
      bannerSeen = true;
    } catch (e) { /* timeout */ }
    report('6/7 banner', bannerSeen ? 'PASS' : 'FAIL',
      'wsEvents=' + wsEvents.join(';') + ' wsFrames=' + wsFrames.length +
      (wsFrames.length ? ' first: ' + wsFrames[0] : '') +
      ' console=' + consoleMsgs.length + (consoleMsgs.length ? ' first: ' + consoleMsgs[0] : '') +
      ' pageErrors=' + pageErrors.length);

    // --- Zoom out and shoot ---
    await page.mouse.move(640, 400);
    for (let i = 0; i < 8; i++) { await page.mouse.wheel(0, 800); await page.waitForTimeout(120); }
    await page.waitForTimeout(1500);

    let anyPacmanPx = 0;
    for (let i = 1; i <= SHOTS; i++) {
      await page.waitForTimeout(INTERVAL_MS);
      const shot = path.join(ARTIFACTS_DIR, 'pacman_' + i + '.png');
      await page.screenshot({ path: shot });
      const yellow = await page.evaluate(() => {
        const c = document.getElementById('mapCanvas');
        if (!c) return -1;
        const ctx = c.getContext('2d');
        const img = ctx.getImageData(0, 0, c.width, c.height).data;
        let n = 0;
        for (let p = 0; p < img.length; p += 4) {
          if (img[p] > 200 && img[p + 1] > 150 && img[p + 2] < 120) n++;
        }
        return n;
      });
      if (yellow > anyPacmanPx) anyPacmanPx = yellow;
      console.log(`[7/7 shot ${i}/${SHOTS}] ${shot.split(path.sep).pop()} yellowPx=${yellow}`);
    }
    report('7/7 shots', 'PASS', 'shots=' + SHOTS + ' maxYellowPx=' + anyPacmanPx + ' wsFrames=' + wsFrames.length);
  } catch (err) {
    report('run', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const failed = results.filter(r => r.status === 'FAIL');
  console.log('');
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'DONE'));
  return finish(failed.length ? 1 : 0);
}

main();