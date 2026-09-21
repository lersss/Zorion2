// tools/e2e/star-visuals-shot.js
// Screenshots of the new star look prototype (idea 2026-09-22 "внешний вид звёзд",
// @tester run). Registers a user, opens the real map, forces a viewport at the
// name-display zoom (scale > 0.5) and captures each preset (eye/photo/sprite/
// crown) plus an exotic-star frame, a player-marker frame, and a synthetic class
// palette (palette.png: O B A F G K M L T Y in a row, default preset). It also
// proves the crown wreath animates (crown-anim: two frames differ).
//
// Why dynamic import: the map keeps its viewport/clusters in a module-scoped
// `state` (map/config.js), not on window. Importing the very same module URL the
// page already loaded returns the LIVE module instance, so we can set
// state.scale/offsetX/offsetY and call draw() - no fake data.
//
// Run: node star-visuals-shot.js   (BASE_URL env overrides the default)
// Output: tools/e2e/out/2026-09-22-stars/*.png
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ROOT = path.dirname(fileURLToPath(import.meta.url));
const OUT_DIR = path.join(ROOT, 'out', '2026-09-22-stars');

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const VIEW_W = 1280, VIEW_H = 800;

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
    const username = 'starshot_' + Date.now() + '_' + attempt;
    const password = 'starshot-pass-' + Date.now();
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

// In-page helper: import the LIVE map modules (same URL the page loaded, so the
// same module instance) and set the viewport, load clusters for it, redraw.
async function setViewport(page, args) {
  return page.evaluate(async (a) => {
    const cfg = await import('/static/js/map/config.js');
    const mr = await import('/static/js/map/map_render.js');
    const data = await import('/static/js/map/data.js');
    const st = cfg.state;
    st.scale = a.scale;
    st.offsetX = st.canvasWidth / 2 - a.cx * a.scale;
    st.offsetY = st.canvasHeight / 2 - a.cy * a.scale;
    await data.loadClusters();
    mr.draw();
    return {
      scale: st.scale,
      canvasW: st.canvasWidth, canvasH: st.canvasHeight,
      clusters: (st.clusters || []).length,
      singles: (st.clusters || []).filter(c => c.cnt === 1).length,
      offsetX: st.offsetX, offsetY: st.offsetY,
    };
  }, args);
}

async function setPreset(page, preset) {
  return page.evaluate(async (p) => {
    const sr = await import('/static/js/map/star_render.js');
    sr.setStarVisualPreset(p);
    const mr = await import('/static/js/map/map_render.js');
    mr.draw();
    return sr.starVisualOptions();
  }, preset);
}

async function main() {
  mkdirSync(OUT_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);

  let creds;
  try { creds = await registerAndLogin(); report('register/login', 'PASS', 'user ' + creds.username); }
  catch (err) { report('register/login', 'FAIL', String(err && err.message ? err.message : err)); return finish(1); }

  const exe = findExecutable();
  if (!exe) { report('browser', 'FAIL', 'no Chrome/Edge found'); return finish(1); }
  console.log('browser: ' + exe.name);
  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: VIEW_W, height: VIEW_H } });
  await context.addInitScript((t) => {
    localStorage.setItem('token', t);
    // Product default is now 'sprite' (creator decision 2026-09-22, gate 2).
    localStorage.setItem('starVisualPreset', 'sprite');
  }, creds.token);
  const page = await context.newPage();

  const pageErrors = [];
  const consoleMsgs = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));
  page.on('console', (m) => consoleMsgs.push(m.type() + ': ' + m.text()));

  const shots = {};
  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1200);

    // --- pick viewports from real clustered data (broad query near player) ---
    const pick = await page.evaluate(async (baseUrl) => {
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };
      const me = await (await fetch(baseUrl + '/me', { headers: auth })).json();
      let px = 0, py = 0, myId = me.current_world_id;
      if (myId) {
        const w = await (await fetch(baseUrl + '/worlds/' + myId, { headers: auth })).json();
        const ww = w.world || w;
        px = ww.coord_x; py = ww.coord_y;
      }
      const R = 6000;
      const p = new URLSearchParams({
        x_min: (px - R).toFixed(1), x_max: (px + R).toFixed(1),
        y_min: (py - R).toFixed(1), y_max: (py + R).toFixed(1), cell: '40',
      });
      const clusters = await (await fetch(baseUrl + '/api/worlds/filter?' + p, { headers: auth })).json();
      const singles = clusters.filter(c => c.cnt === 1 && typeof c.x === 'number');
      const EXO = ['black_hole', 'neutron', 'white_dwarf', 'protostar'];

      // Showcase: field with most bright O/B/A singles (petals) + exotic kinds.
      const scale = 0.8;
      const hx = (640 / scale), hy = (400 / scale);
      let best = null;
      for (const a of singles) {
        if (!['O', 'B', 'A'].includes((a.sspec || '').toUpperCase())) continue;
        const inFrame = singles.filter(c => Math.abs(c.x - a.x) <= hx && Math.abs(c.y - a.y) <= hy);
        const bright = inFrame.filter(c => ['O', 'B', 'A'].includes((c.sspec || '').toUpperCase())).length;
        const kinds = new Set(inFrame.filter(c => EXO.includes(c.stype)).map(c => c.stype)).size;
        const score = bright * 10 + kinds * 40 + Math.min(inFrame.length, 80);
        if (!best || score > best.score) best = { score, x: a.x, y: a.y, bright, kinds, singles: inFrame.length };
      }
      // If no O/B/A anywhere, fall back to densest single field.
      if (!best) {
        for (const a of singles) {
          const inFrame = singles.filter(c => Math.abs(c.x - a.x) <= hx && Math.abs(c.y - a.y) <= hy);
          if (!best || inFrame.length > best.singles) best = { score: inFrame.length, x: a.x, y: a.y, bright: 0, kinds: 0, singles: inFrame.length };
        }
      }

      // Exotic: most distinct exotic kinds in one frame (scale 0.9).
      const escale = 0.9, ehx = 640 / escale, ehy = 400 / escale;
      let exo = null;
      for (const a of singles.filter(c => EXO.includes(c.stype))) {
        const inFrame = singles.filter(c => EXO.includes(c.stype) && Math.abs(c.x - a.x) <= ehx && Math.abs(c.y - a.y) <= ehy);
        const kinds = new Set(inFrame.map(c => c.stype)).size;
        const score = kinds * 100 + inFrame.length;
        if (!exo || score > exo.score) exo = { score, x: a.x, y: a.y, kinds, cnt: inFrame.length, types: [...new Set(inFrame.map(c => c.stype))] };
      }

      // Companions (gate 3): a binary/multiple single star - a high-zoom frame
      // shows the glowing companion(s) next to the primary. Prefer 'multiple'
      // (two companions) and a bright primary for a clearer frame.
      let comp = null;
      for (const c of singles) {
        if (c.systype !== 'binary' && c.systype !== 'multiple') continue;
        const score = (c.systype === 'multiple' ? 2 : 1) + (['O', 'B', 'A'].includes((c.sspec || '').toUpperCase()) ? 1 : 0);
        if (!comp || score > comp.score) comp = { score, x: c.x, y: c.y, sid: c.sid, name: c.sname, systype: c.systype, sspec: c.sspec };
      }

      return {
        player: { x: px, y: py, id: myId },
        showcase: best, exotic: exo, companion: comp, totalSingles: singles.length,
      };
    }, BASE_URL);

    console.log('pick: showcase=' + JSON.stringify(pick.showcase) + ' exotic=' + JSON.stringify(pick.exotic));
    if (!pick.showcase) { report('pick viewport', 'FAIL', 'no singles in data'); return finish(1); }

    const shot = async (file, label) => {
      const p = path.join(OUT_DIR, file);
      await page.screenshot({ path: p });
      shots[label] = p;
      report(label, 'PASS', p);
      return p;
    };

    // --- Showcase frames: eye / photo / sprite at the same viewport ---
    const vp = await setViewport(page, { scale: 0.8, cx: pick.showcase.x, cy: pick.showcase.y });
    console.log('showcase viewport: ' + JSON.stringify(vp));

    for (const preset of ['eye', 'photo', 'sprite', 'crown']) {
      const opts = await setPreset(page, preset);
      await page.waitForTimeout(1700); // let the 900 ms ignite settle
      console.log('preset ' + preset + ' opts=' + JSON.stringify(opts));
      const st = await page.evaluate(async () => {
        const cfg = await import('/static/js/map/config.js');
        return { singles: cfg.state.starSingles, scale: cfg.state.scale };
      });
      console.log('  frame: scale=' + st.scale + ' singles=' + st.singles);
      await shot(preset + '.png', preset + '.png');
    }

    // --- Crown wreath proof (idea 2026-09-22, "Корона" D): the wreath rotates
    // and crossfades, so two frames with an interval must differ. Twinkle is
    // switched OFF to isolate the wreath (otherwise twinkle would also change
    // pixels). ---
    const crownAnim = await page.evaluate(async () => {
      const sr = await import('/static/js/map/star_render.js');
      const mr = await import('/static/js/map/map_render.js');
      const cfg = await import('/static/js/map/config.js');
      const c = document.getElementById('mapCanvas');
      const g = c.getContext('2d');
      const W = c.width, H = c.height;
      sr.setStarVisualPreset('crown');
      sr.setStarVisualToggle('starVisualTwinkle', false); // isolate the wreath
      await new Promise(r => setTimeout(r, 1300));
      mr.draw();
      const A = g.getImageData(0, 0, W, H).data.slice();
      await new Promise(r => setTimeout(r, 700));
      mr.draw();
      const B = g.getImageData(0, 0, W, H).data;
      let count = 0, maxd = 0;
      for (let i = 0; i < A.length; i += 4) {
        const d = Math.abs(A[i] - B[i]) + Math.abs(A[i + 1] - B[i + 1]) + Math.abs(A[i + 2] - B[i + 2]);
        if (d > 6) { count++; if (d > maxd) maxd = d; }
      }
      sr.setStarVisualToggle('starVisualTwinkle', true); // restore
      return { count, maxd, singles: cfg.state.starSingles, needAnim: sr.starNeedsAnim() };
    });
    console.log('crown-anim: ' + JSON.stringify(crownAnim));
    report('crown-anim', crownAnim.count > 0 ? 'PASS' : 'FAIL',
      `differingPx=${crownAnim.count} maxd=${crownAnim.maxd} singles=${crownAnim.singles} needAnim=${crownAnim.needAnim}`);

    // --- Exotic frame ---
    if (pick.exotic) {
      await setViewport(page, { scale: 0.9, cx: pick.exotic.x, cy: pick.exotic.y });
      await setPreset(page, 'eye');
      await page.waitForTimeout(1700);
      const st = await page.evaluate(async () => {
        const cfg = await import('/static/js/map/config.js');
        return { singles: cfg.state.starSingles };
      });
      console.log('exotic frame singles=' + st.singles + ' kinds=' + JSON.stringify(pick.exotic.types));
      await shot('exotic.png', 'exotic.png');
    } else {
      report('exotic.png', 'SKIP', 'no exotic single star found in range');
    }

    // --- Player-marker frame (arrow over own star, for readability check) ---
    if (pick.player && typeof pick.player.x === 'number') {
      await setViewport(page, { scale: 0.8, cx: pick.player.x, cy: pick.player.y });
      await setPreset(page, 'eye');
      await page.waitForTimeout(1700);
      const arrow = await page.evaluate(() => {
        const el = document.getElementById('player-arrow');
        return { exists: !!el, hidden: el ? el.hidden : null };
      });
      console.log('player-arrow: ' + JSON.stringify(arrow));
      await shot('markers-player.png', 'markers-player.png');
    }

    // --- Companion frame (gate 3): high zoom on a binary/multiple system so the
    // glowing companion star(s) (core + soft halo, not flat dots) are visible
    // next to the primary. Two shots: the new default 'sprite' and the vector
    // 'eye' for comparison. ---
    if (pick.companion) {
      await setViewport(page, { scale: 4, cx: pick.companion.x, cy: pick.companion.y });
      console.log('companion target: ' + JSON.stringify(pick.companion));
      for (const preset of ['sprite', 'eye']) {
        await setPreset(page, preset);
        await page.waitForTimeout(1700);
        const st = await page.evaluate(async () => {
          const cfg = await import('/static/js/map/config.js');
          return { scale: cfg.state.scale, singles: cfg.state.starSingles };
        });
        console.log('companion ' + preset + ': scale=' + st.scale + ' singles=' + st.singles);
        await shot('companions-' + preset + '.png', 'companions-' + preset + '.png');
      }
    } else {
      report('companions.png', 'SKIP', 'no binary/multiple single star in range');
    }

    // --- Check 9: hit-zone (click on star selects; click outside does not) ---
    const hit = await page.evaluate(() => (async () => {
      const cfg = await import('/static/js/map/config.js');
      const mr = await import('/static/js/map/map_render.js');
      const sr = await import('/static/js/map/star_render.js');
      const st = cfg.state;
      const canvas = document.getElementById('mapCanvas');
      const rect = canvas.getBoundingClientRect();
      const sx = (c) => c.x * st.scale + st.offsetX;
      const sy = (c) => c.y * st.scale + st.offsetY;
      // Pick an isolated single star well inside the viewport.
      let chosen = null, bestGap = -1;
      const singles = (st.clusters || []).filter(c => c.cnt === 1);
      for (const c of singles) {
        const cx = sx(c), cy = sy(c);
        if (cx < 120 || cy < 120 || cx > st.canvasWidth - 120 || cy > st.canvasHeight - 120) continue;
        let gap = Infinity;
        for (const o of st.clusters) {
          if (o === c) continue;
          gap = Math.min(gap, Math.hypot(sx(o) - cx, sy(o) - cy));
        }
        if (gap > bestGap) { bestGap = gap; chosen = c; }
      }
      if (!chosen) return { ok: false, reason: 'no isolated single star' };
      const R = mr.clusterScreenRadius(chosen);
      const hr = sr.starHitRadius(chosen, R);
      const vr = sr.starVisualRadius(chosen, R);
      const px = sx(chosen), py = sy(chosen);
      // Click direction away from the densest side: just use +y (down), verified gap is big.
      const toClient = (x, y) => ({ x: rect.left + x * (rect.width / st.canvasWidth), y: rect.top + y * (rect.height / st.canvasHeight) });
      const inside = toClient(px, py);
      // Effective click radius (map/events.js findClusterAt): max(hitRadius+3,
      // minDistForClick=30). "Outside the visible star" must clear BOTH that
      // floor and the visible halo, plus a margin.
      const clickR = Math.max(hr + 3, 30);
      const outsideDist = Math.max(vr, clickR) + 12;
      const outside = toClient(px, py + outsideDist);
      return {
        ok: true, sid: chosen.sid, name: chosen.sname, sspec: chosen.sspec, stype: chosen.stype,
        screen: { px, py }, R, vr, hr, outsideDist, inside, outside, gap: bestGap,
      };
    })());
    console.log('hit-target: ' + JSON.stringify(hit));
    if (hit.ok) {
      await page.mouse.click(hit.inside.x, hit.inside.y);
      await page.waitForTimeout(1500);
      const onStar = await page.evaluate(() => (async () => {
        const cfg = await import('/static/js/map/config.js');
        return { modal: !!document.getElementById('system-modal-overlay'), selected: cfg.state.selectedWorldId };
      })());
      await page.keyboard.press('Escape');
      await page.waitForTimeout(400);
      await page.mouse.click(hit.outside.x, hit.outside.y);
      await page.waitForTimeout(1200);
      const offStar = await page.evaluate(() => (async () => {
        const cfg = await import('/static/js/map/config.js');
        return { modal: !!document.getElementById('system-modal-overlay'), selected: cfg.state.selectedWorldId };
      })());
      const clickOk = onStar.modal && onStar.selected === hit.sid && !offStar.modal && !offStar.selected;
      report('click-hit-zone', clickOk ? 'PASS' : 'FAIL',
        `star=${hit.name}(${hit.sspec}/${hit.stype}) R=${hit.R.toFixed(1)} visual=${hit.vr.toFixed(1)} hit=${hit.hr.toFixed(1)} inside->modal=${onStar.modal} sel=${onStar.selected === hit.sid}; outside(+${hit.outsideDist.toFixed(0)}px)->modal=${offStar.modal} sel=${offStar.selected} gap=${hit.gap.toFixed(0)}`);
    } else {
      report('click-hit-zone', 'SKIP', hit.reason);
    }

    // --- Petals proof: eye vs photo with twinkle frozen (isolates petals) ---
    const petal = await page.evaluate(async () => {
      const sr = await import('/static/js/map/star_render.js');
      const mr = await import('/static/js/map/map_render.js');
      const cfg = await import('/static/js/map/config.js');
      const c = document.getElementById('mapCanvas');
      const g = c.getContext('2d');
      const delay = (ms) => new Promise(r => setTimeout(r, ms));
      const W = c.width, H = c.height;
      sr.setStarVisualPreset('eye');
      await delay(1300);
      cfg.state.starSingles = 500; // > TWINKLE_MAX_SINGLES -> twinkle off for this draw
      mr.draw();
      const A = g.getImageData(0, 0, W, H).data.slice();
      sr.setStarVisualPreset('photo');
      await delay(1300);
      cfg.state.starSingles = 500;
      mr.draw();
      const B = g.getImageData(0, 0, W, H).data;
      let count = 0, maxd = 0, minx = W, maxx = -1, miny = H, maxy = -1;
      for (let i = 0; i < A.length; i += 4) {
        const d = Math.abs(A[i] - B[i]) + Math.abs(A[i + 1] - B[i + 1]) + Math.abs(A[i + 2] - B[i + 2]);
        if (d > 6) {
          count++;
          if (d > maxd) maxd = d;
          const px = (i / 4) % W, py = Math.floor((i / 4) / W);
          if (px < minx) minx = px; if (px > maxx) maxx = px;
          if (py < miny) miny = py; if (py > maxy) maxy = py;
        }
      }
      const rect = c.getBoundingClientRect();
      return { count, maxd, bbox: [minx, miny, maxx, maxy], canvasTop: rect.top, rectW: rect.width, canvasW: W };
    });
    console.log('petals-diff: ' + JSON.stringify(petal));
    if (petal.count > 0) {
      const padX = 90, padY = 90;
      const clipX = Math.max(0, petal.bbox[0] - padX);
      const clipY = Math.max(0, petal.canvasTop + petal.bbox[1] - padY);
      const clipW = Math.min(VIEW_W - clipX, (petal.bbox[2] - petal.bbox[0]) + padX * 2 + 1);
      const clipH = Math.min(VIEW_H - clipY, (petal.bbox[3] - petal.bbox[1]) + padY * 2 + 1);
      const clip = { x: clipX, y: clipY, width: Math.max(40, clipW), height: Math.max(40, clipH) };
      await page.evaluate(async () => {
        const sr = await import('/static/js/map/star_render.js');
        const mr = await import('/static/js/map/map_render.js');
        const cfg = await import('/static/js/map/config.js');
        sr.setStarVisualPreset('eye'); cfg.state.starSingles = 500; mr.draw();
      });
      await page.screenshot({ path: path.join(OUT_DIR, 'eye-petals-region.png'), clip });
      await page.evaluate(async () => {
        const sr = await import('/static/js/map/star_render.js');
        const mr = await import('/static/js/map/map_render.js');
        const cfg = await import('/static/js/map/config.js');
        sr.setStarVisualPreset('photo'); cfg.state.starSingles = 500; mr.draw();
      });
      await page.screenshot({ path: path.join(OUT_DIR, 'photo-petals-region.png'), clip });
      report('petals-diff', 'PASS', `differingPx=${petal.count} maxd=${petal.maxd} bbox=${JSON.stringify(petal.bbox)} -> eye-petals-region.png / photo-petals-region.png`);
    } else {
      report('petals-diff', 'FAIL', 'photo preset rendered identically to eye with twinkle frozen');
    }

    // --- Palette frame: one synthetic star per spectral class (O B A F G K M
    // L T Y) in a row, drawn by the same drawStar in the DEFAULT preset, with the
    // class label as the star name. Purpose: the creator sees at a glance that
    // F/Y/L/T are different colours (idea 2026-09-22, "правка цвета и ядра").
    // Synthetic clusters go through the live state, then the real view is
    // restored. ---
    const palette = await page.evaluate(async () => {
      const cfg = await import('/static/js/map/config.js');
      const mr = await import('/static/js/map/map_render.js');
      const st = cfg.state;
      window.__paletteSaved = { clusters: st.clusters, scale: st.scale, offsetX: st.offsetX, offsetY: st.offsetY };
      const specs = ['O', 'B', 'A', 'F', 'G', 'K', 'M', 'L', 'T', 'Y'];
      const spacing = 120;
      st.clusters = specs.map((sp, i) => ({
        sid: 'palette-' + sp, sname: sp, sspec: sp, stemp: null, stype: 'star', systype: 'single',
        cnt: 1, x: 70 + i * spacing, y: 140,
      }));
      st.scale = 1;
      st.offsetX = 0;
      st.offsetY = 0;
      st.currentWorldId = null;
      st.hoveredWorldId = null;
      st.focusWorldId = null;
      st.regions = [];
      mr.draw();
      return { specs, canvasW: st.canvasWidth, canvasH: st.canvasHeight };
    });
    console.log('palette: ' + JSON.stringify(palette));
    await page.waitForTimeout(1700); // ignite settled -> labels fully opaque
    await shot('palette.png', 'palette.png');
    await page.evaluate(async () => {
      const cfg = await import('/static/js/map/config.js');
      const mr = await import('/static/js/map/map_render.js');
      const s = window.__paletteSaved;
      if (s) {
        cfg.state.clusters = s.clusters;
        cfg.state.scale = s.scale;
        cfg.state.offsetX = s.offsetX;
        cfg.state.offsetY = s.offsetY;
      }
      mr.draw();
    });

    // --- Check 10: console hygiene ---
    const allErrs = pageErrors.slice();
    const drawErr = consoleMsgs.filter(m => /draw is not defined/i.test(m));
    const nanErr = consoleMsgs.filter(m => /NaN/.test(m));
    const jsConsoleErr = consoleMsgs.filter(m => m.startsWith('error:') && !/Failed to load resource/i.test(m));
    const ok10 = allErrs.length === 0 && drawErr.length === 0 && nanErr.length === 0;
    report('console-hygiene', ok10 ? 'PASS' : 'FAIL',
      `pageErrors=${allErrs.length} draw-is-not-defined=${drawErr.length} NaN=${nanErr.length} consoleErrors=${jsConsoleErr.length}` +
      (allErrs.length ? ' first: ' + allErrs[0] : '') + (nanErr.length ? ' firstNaN: ' + nanErr[0] : ''));
  } catch (err) {
    report('run', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  console.log('');
  console.log('SHOTS:');
  for (const [k, v] of Object.entries(shots)) console.log('  ' + k + ' -> ' + v);
  const failed = results.filter(r => r.status === 'FAIL');
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'ALL OK'));
  return finish(failed.length ? 1 : 0);
}

main();
