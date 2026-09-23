// tools/e2e/belt-stones-check.js
// e2e-проверка вида камней кольца пояса (ТЗ визуального дизайна 2026-09-23):
// число камней — от ПРОТЯЖЁННОСТИ пояса (beltStoneCount), выработанный пояс —
// вдвое реже (пол 2 камня), детерминизм от сида; плюс кадры модалки (кольцо
// целиком, крупный план камней со светотенью, ховер).
//
// Числа проверяются на ЧИСТОЙ функции beltStoneCount (без отрисовки) — тест
// падает, если вернуть фиксированные 12–24 без учёта width_au.
//
// Требует: поднятый dev-сервер (BASE_URL). Токен не нужен — игрок регистрируется.
// Запуск: cd tools/e2e; node belt-stones-check.js
// ASCII-вывод (PowerShell cp866).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, rmSync } from 'node:fs';
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

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

const results = [];
function report(name, ok, detail) {
  results.push({ name, ok: !!ok });
  console.log(`[${name}] ${ok ? 'PASS' : 'FAIL'}${detail ? ' - ' + detail : ''}`);
}

// Устойчивая запись кадра: файл прошлого прогона удаляем (на Windows
// page.screenshot падает «UNKNOWN: unknown error, open …», если файл залочен),
// ошибка кадра не обрывает проверки.
async function shot(page, file, clip) {
  let err = '';
  for (let attempt = 0; attempt < 3; attempt++) {
    try { rmSync(file, { force: true }); } catch (e) { /* нет файла/занят */ }
    try {
      await page.screenshot({ path: file, clip });
      if (existsSync(file)) return { ok: true, path: file };
      err = 'файл не создан';
    } catch (e) {
      err = String(e && e.message ? e.message : e);
    }
    await page.waitForTimeout(200);
  }
  return { ok: false, path: file, error: err };
}

async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'e2e_belt_stones_' + Date.now() + '_' + attempt;
    const password = 'e2e-pass-' + Date.now();
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (res.status === 201) {
      const data = await res.json();
      const me = await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + data.token } });
      const profile = me.ok ? await me.json() : {};
      return {
        username,
        token: data.token,
        worldId: profile.current_world_id || '',
        worldName: profile.current_world_name || '',
      };
    }
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
  const exe = findExecutable();
  if (!exe) { console.error('Chrome/Edge not found'); return finish(2); }
  mkdirSync(ARTIFACTS_DIR, { recursive: true });

  let creds;
  try {
    creds = await registerAndLogin();
  } catch (err) {
    console.error('register/login: ' + err.message);
    return finish(2);
  }
  if (!creds.worldId) { console.error('у игрока нет current_world_id'); return finish(2); }

  const planetsRes = await fetch(BASE_URL + '/api/worlds/' + creds.worldId + '/planets', {
    headers: { Authorization: 'Bearer ' + creds.token },
  });
  if (!planetsRes.ok) { console.error('planets HTTP ' + planetsRes.status); return finish(2); }
  const system = await planetsRes.json();
  const belts = (system.belts || []).filter(b => b && b.id);
  if (!belts.length) { console.error('в мире нет поясов — нечего проверять'); return finish(2); }
  const belt = belts[0];

  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript((t) => localStorage.setItem('token', t), creds.token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => { pageErrors.push(e.message); console.log('PAGEERROR:', e.message); });

  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForTimeout(1000);

    // ===== Числа: beltStoneCount (чистая функция, без отрисовки) =====
    const nums = await page.evaluate(async (beltArg) => {
      const R = await import('/static/js/modal/modal_render.js');
      const count = R.beltStoneCount;
      const mk = (w, id, level) => ({ id, width_au: w, remaining_level: level });
      const band = (w, n = 40) => {
        const vals = [];
        for (let i = 0; i < n; i++) vals.push(count(mk(w, 'qa-band-' + i)));
        return vals;
      };
      const inRange = (vals, lo, hi) => vals.every(v => v >= lo && v <= hi);
      const mean = (vals) => vals.reduce((s, v) => s + v, 0) / vals.length;
      const b02 = band(0.2), b1 = band(1), b4 = band(4), b12 = band(12);
      // Выработанный: вдвое реже и не меньше 2 камней.
      const dep = [0.2, 1, 4, 12].flatMap(w => band(w, 20).map((v, i) =>
        [v, count(mk(w, 'qa-band-' + i, 'выработан'))]));
      const depOk = dep.every(([full, d]) => d === Math.max(2, Math.round(full * 0.5)));
      const depFloor = dep.every(([, d]) => d >= 2);
      // Детерминизм: один id — одно число; разные id — не константа.
      const same = new Set([0, 1, 2, 3, 4].map(() => count(mk(4, 'qa-det')))).size === 1;
      const distinct = new Set(band(4, 40)).size;
      // Нет данных о протяжённости — «средняя» полоса 10..16.
      const fallback = [undefined, null, 0, -1, NaN, 'abc'].map(v => count({ id: 'qa-fb', width_au: v }));
      return {
        b02, b1, b4, b12,
        bandsOk: inRange(b02, 6, 10) && inRange(b1, 10, 16) && inRange(b4, 16, 22) && inRange(b12, 22, 28),
        means: { b02: mean(b02), b1: mean(b1), b4: mean(b4), b12: mean(b12) },
        depOk, depFloor,
        same, distinct,
        fallbackOk: fallback.every(v => v >= 10 && v <= 16),
        real: count(beltArg),
      };
    }, belt);

    report('count-bands',
      nums.bandsOk,
      `width 0.2→[${Math.min(...nums.b02)}..${Math.max(...nums.b02)}] 1→[${Math.min(...nums.b1)}..${Math.max(...nums.b1)}] ` +
      `4→[${Math.min(...nums.b4)}..${Math.max(...nums.b4)}] 12→[${Math.min(...nums.b12)}..${Math.max(...nums.b12)}]`);
    report('count-grows-with-width',
      nums.means.b02 < nums.means.b1 && nums.means.b1 < nums.means.b4 && nums.means.b4 < nums.means.b12,
      `mean 0.2=${nums.means.b02.toFixed(2)} 1=${nums.means.b1.toFixed(2)} 4=${nums.means.b4.toFixed(2)} 12=${nums.means.b12.toFixed(2)}`);
    report('count-fallback-no-width',
      nums.fallbackOk, 'нет данных → 10..16');
    report('count-depleted-half-and-floor',
      nums.depOk && nums.depFloor, 'выработан = round(полный/2), пол 2');
    report('count-deterministic',
      nums.same && nums.distinct >= 3,
      `один id → одно число; разных значений на 40 id: ${nums.distinct}`);
    report('count-real-belt',
      nums.real >= 6 && nums.real <= 28,
      `пояс «${belt.name || belt.id}» width_au=${belt.width_au} → ${nums.real} камней`);

    // ===== Кадры: кольцо целиком, крупный план камней, ховер =====
    await page.evaluate((w) => {
      window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true });
    }, { id: creds.worldId, name: creds.worldName || 'QA-мир', spec: 'G' });
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForSelector('[data-belt-row]', { timeout: 10000 });
    await page.waitForTimeout(600);

    const modalBox = await page.evaluate(() => {
      const c = document.getElementById('system-canvas');
      const r = c.getBoundingClientRect();
      return { x: Math.round(r.left), y: Math.round(r.top), width: Math.round(r.width), height: Math.round(r.height) };
    });

    // Кадр 1: кольцо целиком (камни на «общем» зуме — как видит игрок).
    await page.evaluate(async (beltId) => {
      const L = await import('/static/js/modal/layout.js');
      const S = await import('/static/js/modal/state.js');
      const s = S.modalState;
      const b = (s.belts || []).find(x => x.id === beltId);
      const layout = L.computeLayout(s.planets, s.starRadius, s.canvasWidth, s.canvasHeight);
      const g = L.beltRing(layout, b);
      s.zoom = Math.min(s.zoom, Math.min(s.canvasWidth, s.canvasHeight) * 0.42 / (g.radius + g.half));
      s.offsetX = s.canvasWidth / 2 - g.cx * s.zoom;
      s.offsetY = s.canvasHeight / 2 - g.cy * s.zoom;
      s.arrivalObject = null;
      s.followOffsetX = 0;
      s.followOffsetY = 0;
    }, belt.id);
    await page.waitForTimeout(400);
    const ringShot = await shot(page, path.join(ARTIFACTS_DIR, 'belt-stones-ring.png'), modalBox);
    report('shot-ring', ringShot.ok, ringShot.ok ? ringShot.path : (ringShot.error || ''));

    // Кадр 2: крупный план камней со светотенью — камеру на найденный камень
    // (самый яркий пиксель в кольцевой зоне канваса).
    const closeup = await page.evaluate(async (beltId) => {
      const L = await import('/static/js/modal/layout.js');
      const S = await import('/static/js/modal/state.js');
      const s = S.modalState;
      const b = (s.belts || []).find(x => x.id === beltId);
      const layout = L.computeLayout(s.planets, s.starRadius, s.canvasWidth, s.canvasHeight);
      const g = L.beltRing(layout, b);
      const canvas = document.getElementById('system-canvas');
      const ctx = canvas.getContext('2d');
      const dpr = window.devicePixelRatio || 1;
      const img = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
      const cxc = g.cx * s.zoom + s.offsetX;
      const cyc = g.cy * s.zoom + s.offsetY;
      const rPx = g.radius * s.zoom;
      // Камень ищем по ПАЛИТРЕ тонов (а не «самый яркий пиксель» — им оказывались
      // орбита/планета): минимальное расстояние цвета до одного из тонов камня.
      const tones = [[107, 114, 128], [139, 147, 161], [69, 76, 88], [59, 65, 75],
        [148, 163, 184], [168, 179, 194], [124, 135, 152]];
      const toneDist = (r, gg, b) => tones.reduce((m, t) =>
        Math.min(m, Math.abs(r - t[0]) + Math.abs(gg - t[1]) + Math.abs(b - t[2])), 1e9);
      let best = null;
      // Кольцевая зона ±2.5 экранных px от осевой линии, шаг 1 px по канвасу.
      for (let x = 0; x < canvas.width; x += 1) {
        for (let y = 0; y < canvas.height; y += 1) {
          const d = Math.hypot(x / dpr - cxc, y / dpr - cyc);
          if (Math.abs(d - rPx) > 2.5) continue;
          const i = (y * canvas.width + x) * 4;
          const r = img[i], gc = img[i + 1], b = img[i + 2];
          if (r + gc + b < 120) continue; // фон
          const dist = toneDist(r, gc, b);
          if (!best || dist < best.dist) best = { dist, x: x / dpr, y: y / dpr };
        }
      }
      if (!best) return null;
      // Мировой угол найденного камня.
      const worldX = (best.x - s.offsetX) / s.zoom;
      const worldY = (best.y - s.offsetY) / s.zoom;
      const angle = Math.atan2(worldY - g.cy, worldX - g.cx);
      // Крупный план: мелкий камень ≈ 3–5 экранных px, «якорный» ≈ 6–9 px.
      s.zoom = 6 / Math.max(1e-6, g.half * 0.45);
      const sx = g.cx + Math.cos(angle) * g.radius;
      const sy = g.cy + Math.sin(angle) * g.radius;
      s.offsetX = s.canvasWidth / 2 - sx * s.zoom;
      s.offsetY = s.canvasHeight / 2 - sy * s.zoom;
      s.arrivalObject = null;
      s.followOffsetX = 0;
      s.followOffsetY = 0;
      return { angle, toneDist: best.dist, stonePx: 6 };
    }, belt.id);
    await page.waitForTimeout(400);
    const closeupShot = await shot(page, path.join(ARTIFACTS_DIR, 'belt-stones-closeup.png'), modalBox);
    report('shot-closeup', closeupShot.ok && !!closeup,
      `${closeupShot.ok ? closeupShot.path : (closeupShot.error || '')} angle=${closeup ? (closeup.angle * 180 / Math.PI).toFixed(1) : '—'}°`);

    // Кадр 2б: макро (камень ≈14 экранных px) — светотень видна крупно.
    if (closeup) {
      await page.evaluate(async ({ beltId, angle }) => {
        const L = await import('/static/js/modal/layout.js');
        const S = await import('/static/js/modal/state.js');
        const s = S.modalState;
        const b = (s.belts || []).find(x => x.id === beltId);
        const layout = L.computeLayout(s.planets, s.starRadius, s.canvasWidth, s.canvasHeight);
        const g = L.beltRing(layout, b);
        s.zoom = 14 / Math.max(1e-6, g.half * 0.45);
        const sx = g.cx + Math.cos(angle) * g.radius;
        const sy = g.cy + Math.sin(angle) * g.radius;
        s.offsetX = s.canvasWidth / 2 - sx * s.zoom;
        s.offsetY = s.canvasHeight / 2 - sy * s.zoom;
      }, { beltId: belt.id, angle: closeup.angle });
      await page.waitForTimeout(400);
      const macroShot = await shot(page, path.join(ARTIFACTS_DIR, 'belt-stones-macro.png'), modalBox);
      report('shot-macro', macroShot.ok, macroShot.ok ? macroShot.path : (macroShot.error || ''));
    }

    // Кадр 3: ховер — подсветка камней (курсор на камень в центре кадра).
    await page.mouse.move(modalBox.x + modalBox.width / 2, modalBox.y + modalBox.height / 2);
    await page.waitForTimeout(300);
    const hovered = await page.evaluate(async () => {
      const S = await import('/static/js/modal/state.js');
      const h = S.modalState.hoveredObject;
      return h ? { type: h.type, id: h.id } : null;
    });
    const hoverShot = await shot(page, path.join(ARTIFACTS_DIR, 'belt-stones-hover.png'), modalBox);
    report('shot-hover', hoverShot.ok && !!hovered && hovered.type === 'belt',
      `hovered=${JSON.stringify(hovered)} ${hoverShot.ok ? hoverShot.path : (hoverShot.error || '')}`);

    report('no-page-errors', pageErrors.length === 0, `errors=${pageErrors.length}`);
  } catch (err) {
    report('run', false, String(err && err.message ? err.message : err));
  }

  const failed = results.filter(r => !r.ok).length;
  console.log(`\n${results.length - failed}/${results.length} PASS`);
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.error('belt-stones-check error:', e); finish(1); });
