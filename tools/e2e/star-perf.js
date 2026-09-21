// tools/e2e/star-perf.js
// Star-render performance probe (idea 2026-09-22 "внешний вид звёзд", creator
// requirement: honest answer whether the sprite look is cheaper than the vector
// look at a large number of stars). Registers a user, opens the real map, injects
// N synthetic single-star clusters that all fall inside the viewport and measures
// BOTH the full draw() frame time and the star-only drawStar() time for the two
// real modes: 'eye' (vector) and 'sprite' (pre-baked halo).
//
// Why synthetic clusters: a real galaxy seldom packs 3000 single stars into one
// viewport, so N cannot be reached from live data. The starfield/names/regions
// overhead is shared by both modes, so the star-only number isolates the render.
//
// Why synchronous loops: JS is single-threaded, so while the measurement for-loop
// runs no requestAnimationFrame (twinkle/ignite/NPC) can fire - frames do not mix.
//
// Run: node star-perf.js   (BASE_URL env overrides the default)
// Output: artifacts/star-perf.json + console table.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ROOT = path.dirname(fileURLToPath(import.meta.url));
const OUT_FILE = path.join(ROOT, 'artifacts', 'star-perf.json');

const N_LIST = [300, 1000, 3000];
const FRAMES = 30;
const VIEW_W = 1280, VIEW_H = 800;

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'starperf_' + Date.now() + '_' + attempt;
    const password = 'starperf-pass-' + Date.now();
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (res.status === 201) { const d = await res.json(); return { username, token: d.token }; }
    if (res.status === 409) continue;
    throw new Error('register HTTP ' + res.status + ': ' + (await res.text()));
  }
  throw new Error('register: name collision');
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

// measure - runs in the page. Builds N on-screen clusters, then times FRAMES
// full draw() calls and FRAMES star-only drawStar() sweeps, for one preset.
async function measure(page, mode, n) {
  return page.evaluate(async (a) => {
    const cfg = await import('/static/js/map/config.js');
    const mr = await import('/static/js/map/map_render.js');
    const sr = await import('/static/js/map/star_render.js');
    const st = cfg.state;

    // Steady state: no ignite animation, no hover/player markers in the way.
    sr.setStarVisualToggle('starVisualIgnite', false);
    sr.setStarVisualPreset(a.mode);
    st.scale = 0.8;          // > nameDisplayThreshold 0.5, > regionDisplayThreshold 0.6 -> stars, no regions
    st.offsetX = 0;
    st.offsetY = 0;
    st.currentWorldId = null;
    st.hoveredWorldId = null;
    st.focusWorldId = null;
    st.playerPositions = [];
    st.npcPositions = [];
    st.isFlying = false;

    const W = st.canvasWidth, H = st.canvasHeight;
    const cols = Math.max(1, Math.ceil(Math.sqrt(a.n * (W / H))));
    const rows = Math.max(1, Math.ceil(a.n / cols));
    const SPECS = ['O', 'B', 'A', 'F', 'G', 'K', 'M'];
    const clusters = [];
    let id = 0;
    for (let r = 0; r < rows && id < a.n; r++) {
      for (let cix = 0; cix < cols && id < a.n; cix++) {
        const px = (cix + 0.5) * (W / cols);
        const py = (r + 0.5) * (H / rows);
        clusters.push({
          sid: 'perf-' + id,
          sname: 'P' + id,
          sspec: SPECS[id % SPECS.length],
          stemp: 5000,
          stype: 'star',
          systype: 'single',
          x: (px - st.offsetX) / st.scale,
          y: (py - st.offsetY) / st.scale,
          cnt: 1,
          smods: {},
        });
        id++;
      }
    }
    st.clusters = clusters;
    st.starSingles = a.n;

    // Warm-up (fills never-drawn paths, sprite bake) - not measured.
    for (let f = 0; f < 3; f++) mr.draw();
    const opts = sr.starVisualOptions();
    for (const c of clusters) {
      sr.drawStar(cfg.elements.ctx, c, 0, 0, mr.clusterScreenRadius(c), opts);
    }

    const frameMs = [];
    for (let f = 0; f < a.frames; f++) {
      const t0 = performance.now();
      mr.draw();
      frameMs.push(performance.now() - t0);
    }

    const ctx = cfg.elements.ctx;
    const starMs = [];
    for (let f = 0; f < a.frames; f++) {
      const t0 = performance.now();
      for (const c of clusters) {
        const px = c.x * st.scale + st.offsetX;
        const py = c.y * st.scale + st.offsetY;
        sr.drawStar(ctx, c, px, py, mr.clusterScreenRadius(c), opts);
      }
      starMs.push(performance.now() - t0);
    }

    const med = (arr) => { const s = arr.slice().sort((x, y) => x - y); return s[Math.floor(s.length / 2)]; };
    const avg = (arr) => arr.reduce((x, y) => x + y, 0) / arr.length;
    return {
      n: a.n, mode: a.mode,
      frameMedian: med(frameMs), frameAvg: avg(frameMs), frameMin: Math.min(...frameMs), frameMax: Math.max(...frameMs),
      starMedian: med(starMs), starAvg: avg(starMs), starMin: Math.min(...starMs), starMax: Math.max(...starMs),
      singles: st.starSingles,
    };
  }, { mode, n, frames: FRAMES });
}

async function main() {
  mkdirSync(path.dirname(OUT_FILE), { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);

  let creds;
  try { creds = await registerAndLogin(); console.log('[register/login] PASS user ' + creds.username); }
  catch (err) { console.log('[register/login] FAIL ' + (err && err.message ? err.message : err)); return finish(1); }

  const exe = findExecutable();
  if (!exe) { console.log('[browser] FAIL no Chrome/Edge found'); return finish(1); }
  console.log('browser: ' + exe.name);
  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: VIEW_W, height: VIEW_H } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, creds.token);
  const page = await context.newPage();

  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));

  const rows = [];
  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1000);

    for (const mode of ['eye', 'sprite']) {
      for (const n of N_LIST) {
        const r = await measure(page, mode, n);
        rows.push(r);
        console.log(`[${mode} n=${n}] frame median ${r.frameMedian.toFixed(2)} ms (${(1000 / r.frameMedian).toFixed(0)} FPS) | star-only median ${r.starMedian.toFixed(2)} ms`);
      }
    }
  } catch (err) {
    console.log('[run] FAIL ' + (err && err.message ? err.message : err));
    return finish(1);
  }

  const report = { baseUrl: BASE_URL, browser: exe.name, frames: FRAMES, viewport: { w: VIEW_W, h: VIEW_H }, rows, pageErrors };
  writeFileSync(OUT_FILE, JSON.stringify(report, null, 2));
  console.log('');
  console.log('mode   n     frameMed  frameFPS  starMed   starFPS   sprite/vector(star)');
  for (const n of N_LIST) {
    const v = rows.find(r => r.n === n && r.mode === 'eye');
    const s = rows.find(r => r.n === n && r.mode === 'sprite');
    if (!v || !s) continue;
    const ratio = v.starMedian > 0 ? (s.starMedian / v.starMedian).toFixed(2) : 'n/a';
    console.log(`vector ${String(n).padEnd(5)} ${v.frameMedian.toFixed(2).padStart(8)} ${(1000 / v.frameMedian).toFixed(0).padStart(8)} ${v.starMedian.toFixed(2).padStart(9)} ${(1000 / v.starMedian).toFixed(0).padStart(8)}`);
    console.log(`sprite ${String(n).padEnd(5)} ${s.frameMedian.toFixed(2).padStart(8)} ${(1000 / s.frameMedian).toFixed(0).padStart(8)} ${s.starMedian.toFixed(2).padStart(9)} ${(1000 / s.starMedian).toFixed(0).padStart(8)}   x${ratio}`);
  }
  console.log('');
  console.log('wrote ' + OUT_FILE);
  console.log('pageErrors=' + pageErrors.length + (pageErrors.length ? ' first: ' + pageErrors[0] : ''));
  return finish(pageErrors.length ? 1 : 0);
}

main();
