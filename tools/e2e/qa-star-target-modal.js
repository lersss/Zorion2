// tools/e2e/qa-star-target-modal.js (QA temporary probe, NOT for commit)
// Independent check of idea 2026-09-22 "stars as target / rings" fix:
//  - modal drawSystem: star = bright point + soft glow (not flat circle), hover
//    ring on the star works, planets/orbits readable;
//  - quantitative radial profile: no concentric rings (monotone falloff) for the
//    modal star and for the map star (vector + sprite hybrid R>34).
// Run: node qa-star-target-modal.js   (BASE_URL env overrides)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

const results = [];
function report(name, status, detail) {
  results.push({ name, status, detail });
  console.log(`[${name}] ${status}${detail ? ' - ' + detail : ''}`);
}
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}
async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'qastar_' + Date.now() + '_' + attempt;
    const password = 'qastar-pass-' + Date.now();
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

// radialProfile(ctx, cx, cy, rMax) - annular median luminance per integer radius.
const profileFn = `
function lum(d, i) { return 0.2126 * d[i] + 0.7152 * d[i+1] + 0.0722 * d[i+2]; }
function radialProfile(g, cx, cy, rMax, W, H) {
  const img = g.getImageData(0, 0, W, H).data;
  const out = [];
  for (let r = 0; r <= rMax; r++) {
    const vals = [];
    const n = Math.max(16, Math.round(2 * Math.PI * r));
    for (let k = 0; k < n; k++) {
      const a = 2 * Math.PI * k / n;
      const x = Math.round(cx + Math.cos(a) * r);
      const y = Math.round(cy + Math.sin(a) * r);
      if (x < 0 || y < 0 || x >= W || y >= H) continue;
      vals.push(lum(img, (y * W + x) * 4));
    }
    vals.sort((p, q) => p - q);
    out.push(vals.length ? vals[Math.floor(vals.length / 2)] : 0);
  }
  return out;
}
// ringScore: monotone falloff => ~0. Detects a "dark moat then brighter ring"
// (local min followed by a local max that rises by > riseAbs over the min).
function ringScore(prof, riseAbs) {
  let worstRise = 0, moatAt = -1;
  for (let i = 2; i < prof.length - 1; i++) {
    const isLocalMin = prof[i] <= prof[i-1] && prof[i] <= prof[i+1];
    if (!isLocalMin) continue;
    for (let j = i + 1; j < prof.length; j++) {
      if (prof[j] - prof[i] > worstRise) { worstRise = prof[j] - prof[i]; moatAt = i; }
    }
  }
  return { worstRise, moatAt, ring: worstRise > riseAbs };
}
`;

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);
  let creds;
  try { creds = await registerAndLogin(); report('register/login', 'PASS', creds.username); }
  catch (e) { report('register/login', 'FAIL', String(e.message || e)); return finish(1); }

  const exe = findExecutable();
  if (!exe) { report('browser', 'FAIL', 'no Chrome/Edge'); return finish(1); }
  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); localStorage.setItem('starVisualPreset', 'sprite'); }, creds.token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e.message || e)));

  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => { const el = document.getElementById('loading'); return !el || el.style.display === 'none'; }, { timeout: 30000 });
    await page.waitForTimeout(1000);

    // ---- open own system modal by clicking the star nearest the viewport centre ----
    const target = await page.evaluate(async (baseUrl) => {
      const canvas = document.getElementById('mapCanvas');
      const rect = canvas.getBoundingClientRect();
      const cw = canvas.width, ch = canvas.height;
      const auth = { Authorization: 'Bearer ' + localStorage.getItem('token') };
      const me = await (await fetch(baseUrl + '/me', { headers: auth })).json();
      let wx = 0, wy = 0;
      if (me.current_world_id) {
        const w = await (await fetch(baseUrl + '/worlds/' + me.current_world_id, { headers: auth })).json();
        const ww = w.world || w;
        wx = ww.coord_x; wy = ww.coord_y;
      }
      const scale = 1.0;
      const offsetX = cw / 2 - wx * scale, offsetY = ch / 2 - wy * scale;
      const inv = 1 / scale;
      const p = new URLSearchParams({
        x_min: (-offsetX * inv).toFixed(3), x_max: ((cw - offsetX) * inv).toFixed(3),
        y_min: (-offsetY * inv).toFixed(3), y_max: ((ch - offsetY) * inv).toFixed(3), cell: (40 / scale).toFixed(3),
      });
      const clusters = await (await fetch(baseUrl + '/api/worlds/filter?' + p, { headers: auth })).json();
      const cx = cw / 2, cy = ch / 2;
      let best = null, bestD = Infinity;
      for (const c of clusters) {
        const px = c.x * scale + offsetX, py = c.y * scale + offsetY;
        const d = (px - cx) * (px - cx) + (py - cy) * (py - cy);
        if (d < bestD) { bestD = d; best = { x: px, y: py, sid: c.sid, name: c.sname }; }
      }
      if (!best) return { ok: false, reason: 'no clusters' };
      return { ok: true, clickX: rect.left + best.x * (rect.width / cw), clickY: rect.top + best.y * (rect.height / ch), star: best };
    }, BASE_URL);
    if (!target.ok) { report('open modal', 'SKIP', target.reason); return finish(0); }
    await page.mouse.click(target.clickX, target.clickY);
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForTimeout(2500);

    const info = await page.evaluate(() => {
      const canvas = document.querySelector('#system-modal-overlay canvas');
      const r = canvas.getBoundingClientRect();
      return { rect: { x: r.x, y: r.y, w: r.width, h: r.height }, cw: canvas.width, ch: canvas.height, rows: document.querySelectorAll('#right-panel tr[data-index]').length };
    });
    report('modal-open', info.rect.w > 0 && info.rows > 0 ? 'PASS' : 'FAIL', `rows=${info.rows} canvas=${Math.round(info.rect.w)}x${Math.round(info.rect.h)}`);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-star-modal.png') });
    const starX = info.rect.x + info.rect.w / 2, starY = info.rect.y + info.rect.h / 2;
    await page.mouse.move(starX, starY);
    await page.waitForTimeout(400);
    const hover = await page.evaluate(async () => {
      const canvas = document.querySelector('#system-modal-overlay canvas');
      const st = await import('/static/js/modal/state.js');
      return { cursor: canvas.style.cursor, hovered: st.modalState.hoveredObject };
    });
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-star-modal-hover.png') });
    report('modal-hover-star', hover.cursor === 'pointer' && hover.hovered === 'star' ? 'PASS' : 'FAIL', JSON.stringify(hover));

    // hover ring pixel proof: canvas imageData with hover vs without (mouse to corner)
    const hoverDiff = await page.evaluate(async (a) => {
      const canvas = document.querySelector('#system-modal-overlay canvas');
      const g = canvas.getContext('2d');
      const W = canvas.width, H = canvas.height;
      const st = await import('/static/js/modal/state.js');
      const dpr = window.devicePixelRatio || 1;
      const cx = W / 2, cy = H / 2;
      const snap = () => g.getImageData(0, 0, W, H).data.slice();
      const st1 = st.modalState.hoveredObject;
      st.modalState.hoveredObject = 'star';
      // redraw happens on the modal raf loop; wait a frame
      await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
      const A = snap();
      st.modalState.hoveredObject = null;
      await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
      const B = snap();
      let diff = 0;
      for (let i = 0; i < A.length; i += 4) {
        const d = Math.abs(A[i] - B[i]) + Math.abs(A[i+1] - B[i+1]) + Math.abs(A[i+2] - B[i+2]);
        if (d > 8) diff++;
      }
      st.modalState.hoveredObject = st1;
      return { diff };
    }, {});
    report('modal-hover-ring-pixels', hoverDiff.diff > 50 ? 'PASS' : 'FAIL', `changedPx=${hoverDiff.diff}`);

    // ---- quantitative: modal star radial profile (no rings) ----
    const modalProf = await page.evaluate(async (pfSrc) => {
      eval(pfSrc);
      const mr = await import('/static/js/modal/modal_render.js');
      const st = await import('/static/js/modal/state.js');
      const cases = [
        { spec: 'G', color: '#ffd700' },
        { spec: 'O', color: '#cfe3ff' },
        { spec: 'L', color: '#b83b2b' },
      ];
      const out = [];
      for (const c of cases) {
        const cv = document.createElement('canvas');
        cv.width = 320; cv.height = 320;
        const g = cv.getContext('2d');
        const s = st.modalState;
        s.zoom = 1; s.offsetX = 0; s.offsetY = 0; s.followOffsetX = 0; s.followOffsetY = 0;
        s.starType = 'star'; s.systemType = 'single'; s.companion = ''; s.extraCompanions = [];
        s.myPosition = null; s.systemPlayers = []; s.spectralClass = c.spec; s.starColor = c.color;
        s.hoveredObject = null;
        await mr.drawSystem(cv, c.spec, [], 40, c.color, 320, 320);
        const prof = radialProfile(g, 160, 160, 120, 320, 320);
        const rs = ringScore(prof, 14);
        out.push({ spec: c.spec, center: prof[0].toFixed(0), r30: prof[30].toFixed(0), r60: prof[60].toFixed(0), r100: prof[100].toFixed(0), worstRise: rs.worstRise.toFixed(1), moatAt: rs.moatAt, ring: rs.ring });
      }
      return out;
    }, profileFn);
    const modalRings = modalProf.filter(p => p.ring).length;
    report('modal-profile-no-rings', modalRings === 0 ? 'PASS' : 'FAIL', JSON.stringify(modalProf));

    // ---- quantitative: map star (vector + sprite hybrid) radial profile ----
    const mapProf = await page.evaluate(async (pfSrc) => {
      eval(pfSrc);
      const sr = await import('/static/js/map/star_render.js');
      const cfg = await import('/static/js/map/config.js');
      sr.setStarVisualToggle('starVisualIgnite', false);
      sr.setStarVisualToggle('starVisualTwinkle', false);
      const cases = [];
      for (const mode of ['eye', 'sprite']) {
        for (const R of [20, 60]) {
          const cv = document.createElement('canvas');
          cv.width = 400; cv.height = 400;
          const g = cv.getContext('2d');
          g.fillStyle = '#020617'; g.fillRect(0, 0, 400, 400);
          const c = { sid: 'qa-fixed', sspec: 'G', stype: 'star', stemp: 5800 };
          sr.drawStar(g, c, 200, 200, R, { mode, crown: false, twinkle: false, ignite: false, additive: false, exotic: false, petals: false });
          const prof = radialProfile(g, 200, 200, Math.min(190, Math.round(R * 2.4)), 400, 400);
          const rs = ringScore(prof, 14);
          const at = (k) => prof[Math.min(prof.length - 1, k)].toFixed(0);
          cases.push({ mode, R, center: at(0), r10: at(10), r30: at(30), rMax: at(prof.length - 1), worstRise: rs.worstRise.toFixed(1), moatAt: rs.moatAt, ring: rs.ring });
        }
      }
      return cases;
    }, profileFn);
    const mapRings = mapProf.filter(p => p.ring).length;
    report('map-profile-no-rings', mapRings === 0 ? 'PASS' : 'FAIL', JSON.stringify(mapProf));

    report('console', pageErrors.length === 0 ? 'PASS' : 'FAIL', `pageErrors=${pageErrors.length}${pageErrors.length ? ' first: ' + pageErrors[0] : ''}`);
  } catch (e) {
    report('run', 'FAIL', String(e.message || e));
    return finish(1);
  }

  const failed = results.filter(r => r.status === 'FAIL');
  console.log('');
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'ALL PASS'));
  return finish(failed.length ? 1 : 0);
}

main();
