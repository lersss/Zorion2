// tools/e2e/belt-nearest-check.js
// e2e-проверка правки создателя 2026-09-23 («Надо лететь к ближайшей»): цель
// полёта к поясу — ближайшая точка осевой линии кольца к кораблю, а не
// канонический азимут hash(belt.id) (арт-док §9.3 п.1).
//
// Как проверяем (детерминированно, без ручной игры):
//   1) свежий игрок, его мир, реальный пояс мира (id — настоящий: POST полёта
//      должен пройти);
//   2) позицию корабля подставляем интерцептором ответа системы: орбита звезды
//      → точка корабля лежит на +X от звезды (азимут 0, без анимации);
//   3) пояс выбираем так, чтобы его канонический азимут был далеко от 0 —
//      тогда «каноническая цель» и «ближайшая» различимы;
//   4) до полёта: цель = канонический азимут (фолбэк «точка прибытия
//      неизвестна», §9.3 п.2 — маркер не исчезает);
//   5) после «Лететь»: цель = ближайшая точка (азимут корабля) и она
//      ОТЛИЧАЕТСЯ от канонической — при возврате к канонической цели тест
//      падает;
//   6) после закрытия/переоткрытия модалки цель не «прыгает» на каноническую
//      (один источник точки, §9.3 п.2).
// Плюс два кадра модалки: кольцо без обводки (правка 1) и оно же в ховере.
//
// Требует: поднятый dev-сервер (BASE_URL). Токен не нужен — игрок регистрируется.
// Запуск: cd tools/e2e; node belt-nearest-check.js
// ASCII-вывод (PowerShell cp866).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, rmSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
// Допуск сравнения азимутов (град): полёт и маркер считают точку одной функцией,
// расхождение — только округление/тайминг чтения.
const ANGLE_TOL_DEG = 2;
// Минимальная различимость «ближайшей» и «канонической» цели (град).
const MIN_DIFF_DEG = 10;

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

// Угол из радиан в градусы с нормализацией в (-180, 180].
function deg(rad) { return rad * 180 / Math.PI; }
function norm180(d) { let x = d % 360; if (x > 180) x -= 360; if (x <= -180) x += 360; return x; }

async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'e2e_belt_near_' + Date.now() + '_' + attempt;
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

// shot — устойчивая запись кадра (D1): артефакт прошлого прогона удаляем перед
// записью (на Windows `page.screenshot` падает «UNKNOWN: unknown error, open …»,
// если файл существует/залочен — например, открыт просмотрщиком) и повторяем
// попытку. Ошибка кадра НЕ бросает исключение: обрывается только свой пункт, а
// функциональные проверки зонда идут дальше.
async function shot(page, file, clip) {
  let err = '';
  for (let attempt = 0; attempt < 3; attempt++) {
    try { rmSync(file, { force: true }); } catch (e) { /* нет файла/занят — пробуем писать */ }
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

// beltState — состояние точки пояса глазами клиента: цель полёта (orbitalPoint),
// маркер (objectCanvasPos), канонический азимут и азимут корабля до старта.
async function beltState(page, beltId, shipPos) {
  return page.evaluate(async ({ beltId, shipPos }) => {
    const L = await import('/static/js/modal/layout.js');
    const R = await import('/static/js/modal/modal_render.js');
    const S = await import('/static/js/modal/state.js');
    const s = S.modalState;
    const belt = (s.belts || []).find(b => b.id === beltId);
    if (!belt) return { error: 'belt not in modalState' };
    const layout = L.computeLayout(s.planets, s.starRadius, s.canvasWidth, s.canvasHeight);
    const g = L.beltRing(layout, belt);
    const target = R.orbitalPoint(layout, s.planets, 'belt', beltId, performance.now());
    const marker = R.objectCanvasPos(layout, s.planets, 'belt', beltId, performance.now());
    const angleOf = (p) => Math.atan2(p.y - g.cy, p.x - g.cx);
    let shipAngle = null;
    if (shipPos) {
      const ship = R.orbitalPoint(layout, s.planets, shipPos.object_type, shipPos.object_id, performance.now());
      shipAngle = angleOf(ship);
    }
    return {
      targetAngle: angleOf(target),
      markerAngle: angleOf(marker),
      canonicalAngle: L.beltAngle(belt),
      shipAngle,
      cx: g.cx, cy: g.cy, radius: g.radius, half: g.half,
      zoom: s.zoom, canvasW: s.canvasWidth, canvasH: s.canvasHeight,
    };
  }, { beltId, shipPos });
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

  // Реальные пояса мира (id — настоящий: POST полёта обязан пройти).
  const planetsRes = await fetch(BASE_URL + '/api/worlds/' + creds.worldId + '/planets', {
    headers: { Authorization: 'Bearer ' + creds.token },
  });
  if (!planetsRes.ok) { console.error('planets HTTP ' + planetsRes.status); return finish(2); }
  const system = await planetsRes.json();
  const belts = (system.belts || []).filter(b => b && b.id);
  if (!belts.length) { console.error('в мире нет поясов — нечего проверять'); return finish(2); }
  const firstPlanet = (system.planets || []).find(p => p && p.id);

  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await context.addInitScript((t) => localStorage.setItem('token', t), creds.token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => { pageErrors.push(e.message); console.log('PAGEERROR:', e.message); });

  // Позиция корабля на момент открытия модалки: орбита звезды (точка на +X от
  // звезды, азимут 0, без анимации) — «ближайшая» и «каноническая» различимы.
  const shipPos = { status: 'orbit', object_type: 'star', object_id: creds.worldId };
  await page.route('**/api/worlds/*/planets*', async (route) => {
    const res = await route.fetch();
    let json = {};
    try { json = await res.json(); } catch (e) { json = {}; }
    json.my_position = shipPos;
    await route.fulfill({ json });
  });

  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForTimeout(1200);

    // Выбор пояса: канонический азимут дальше всего от азимута корабля (0) —
    // тогда «каноническая цель» и «ближайшая» гарантированно различимы.
    const canon = await page.evaluate(async (ids) => {
      const L = await import('/static/js/modal/layout.js');
      return ids.map(id => L.beltAngle({ id }));
    }, belts.map(b => b.id));
    let pick = null;
    for (let i = 0; i < belts.length; i++) {
      const d = Math.abs(norm180(deg(canon[i])));
      if (!pick || d > pick.diff) pick = { belt: belts[i], canonical: canon[i], diff: d };
    }
    console.log(`мир: ${creds.worldName}, поясов: ${belts.length}, выбран: ${pick.belt.name || pick.belt.id} ` +
      `(канон ${deg(pick.canonical).toFixed(1)}°, отличие от азимута корабля ${pick.diff.toFixed(1)}°)`);
    if (pick.diff < MIN_DIFF_DEG) {
      report('belt-choice', false, `канонический азимут слишком близок к азимуту корабля (${pick.diff.toFixed(1)}°)`);
      return finish(1);
    }
    const BELT_ID = pick.belt.id;

    // --- Модалка открыта: цель ДО полёта = канонический азимут (фолбэк) ---
    await page.evaluate((w) => {
      window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true });
    }, { id: creds.worldId, name: creds.worldName || 'QA-мир', spec: 'G' });
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForSelector('[data-belt-row]', { timeout: 10000 });
    await page.waitForTimeout(500);

    const before = await beltState(page, BELT_ID, shipPos);
    if (before.error) { report('before-flight-state', false, before.error); return finish(1); }
    report('canonical-fallback-before-flight',
      Math.abs(norm180(deg(before.targetAngle - before.canonicalAngle))) < 0.5 &&
      Math.abs(norm180(deg(before.shipAngle - before.canonicalAngle))) > MIN_DIFF_DEG,
      `target=${deg(before.targetAngle).toFixed(1)}° canonical=${deg(before.canonicalAngle).toFixed(1)}° ship=${deg(before.shipAngle).toFixed(1)}°`);

    // --- Кадры: кольцо без обводки (зум «по кольцу», затем ховер) ---
    const zoomed = await page.evaluate(({ beltId }) => {
      return (async () => {
        const L = await import('/static/js/modal/layout.js');
        const S = await import('/static/js/modal/state.js');
        const s = S.modalState;
        const belt = (s.belts || []).find(b => b.id === beltId);
        const layout = L.computeLayout(s.planets, s.starRadius, s.canvasWidth, s.canvasHeight);
        const g = L.beltRing(layout, belt);
        const need = Math.min(s.canvasWidth, s.canvasHeight) * 0.42 / (g.radius + g.half);
        s.zoom = Math.min(s.zoom, need);
        s.offsetX = s.canvasWidth / 2 - g.cx * s.zoom;
        s.offsetY = s.canvasHeight / 2 - g.cy * s.zoom;
        s.arrivalObject = null;
        s.followOffsetX = 0;
        s.followOffsetY = 0;
        s.followDirty = false;
        return true;
      })();
    }, { beltId: BELT_ID });
    if (!zoomed) { report('zoom-to-ring', false, 'не удалось приблизить кольцо'); return finish(1); }
    await page.waitForTimeout(400);
    const canvasShot = path.join(ARTIFACTS_DIR, 'belt-nearest-ring.png');
    const modalBox = await page.evaluate(() => {
      const c = document.getElementById('system-canvas');
      const r = c.getBoundingClientRect();
      return { x: Math.round(r.left), y: Math.round(r.top), width: Math.round(r.width), height: Math.round(r.height) };
    });
    const ringShot = await shot(page, canvasShot, modalBox);
    report('shot-ring-no-outline', ringShot.ok, ringShot.ok ? ringShot.path : (ringShot.error || ''));

    // Точка кольца (азимут 0) в клиентских координатах: канвасные px →
    // client (canvas.width ≠ rect.width при dpr), как в хит-тесте events.js.
    const ringPoint = await page.evaluate(async (beltId) => {
      const L = await import('/static/js/modal/layout.js');
      const S = await import('/static/js/modal/state.js');
      const s = S.modalState;
      const belt = (s.belts || []).find(b => b.id === beltId);
      const layout = L.computeLayout(s.planets, s.starRadius, s.canvasWidth, s.canvasHeight);
      const g = L.beltRing(layout, belt);
      const canvas = document.getElementById('system-canvas');
      const r = canvas.getBoundingClientRect();
      const pxCanvas = (g.cx + g.radius) * s.zoom + s.offsetX + (s.followOffsetX || 0);
      const pyCanvas = g.cy * s.zoom + s.offsetY + (s.followOffsetY || 0);
      return { x: r.left + pxCanvas * (r.width / canvas.width), y: r.top + pyCanvas * (r.height / canvas.height) };
    }, BELT_ID);
    let hovered = null;
    for (const dy of [0, 4, -4, 8, -8]) {
      await page.mouse.move(ringPoint.x, ringPoint.y + dy);
      await page.waitForTimeout(250);
      hovered = await page.evaluate(async () => {
        const S = await import('/static/js/modal/state.js');
        const h = S.modalState.hoveredObject;
        return h ? { type: h.type, id: h.id } : null;
      });
      if (hovered && hovered.type === 'belt') break;
    }
    const hoverShotFile = path.join(ARTIFACTS_DIR, 'belt-nearest-ring-hover.png');
    const hoverShot = await shot(page, hoverShotFile, modalBox);
    report('shot-ring-hover', !!hovered && hovered.type === 'belt' && hoverShot.ok,
      `hovered=${JSON.stringify(hovered)} ${hoverShot.ok ? hoverShot.path : (hoverShot.error || '')}`);

    // --- Полёт «Лететь» из ПКМ-меню пояса ---
    await page.click('[data-belt-row]', { button: 'right' });
    await page.waitForSelector('#star-context-menu', { timeout: 5000 });
    await page.locator('#star-context-menu div').filter({ hasText: 'Лететь' }).first().click();
    await page.waitForTimeout(1200);
    // Дальше — реальный ответ системы (позиция in_flight → orbit у пояса).
    await page.unroute('**/api/worlds/*/planets*').catch(() => {});

    const after = await beltState(page, BELT_ID, shipPos);
    if (after.error) { report('after-flight-state', false, after.error); return finish(1); }
    const dNearest = Math.abs(norm180(deg(after.targetAngle - after.shipAngle)));
    const dCanonical = Math.abs(norm180(deg(after.targetAngle - after.canonicalAngle)));
    report('nearest-target-after-flight',
      dNearest < ANGLE_TOL_DEG,
      `target=${deg(after.targetAngle).toFixed(1)}° ship=${deg(after.shipAngle).toFixed(1)}° diff=${dNearest.toFixed(2)}° (tol ${ANGLE_TOL_DEG}°)`);
    report('differs-from-canonical',
      dCanonical > MIN_DIFF_DEG,
      `target=${deg(after.targetAngle).toFixed(1)}° canonical=${deg(after.canonicalAngle).toFixed(1)}° diff=${dCanonical.toFixed(1)}° (min ${MIN_DIFF_DEG}°)`);
    report('marker-equals-target',
      Math.abs(norm180(deg(after.markerAngle - after.targetAngle))) < 0.01,
      `marker=${deg(after.markerAngle).toFixed(2)}° target=${deg(after.targetAngle).toFixed(2)}°`);

    // --- Один источник: закрытие/переоткрытие модалки цель не сдвигает ---
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    await page.evaluate((w) => {
      window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true });
    }, { id: creds.worldId, name: creds.worldName || 'QA-мир', spec: 'G' });
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForTimeout(700);
    const reopened = await beltState(page, BELT_ID, null);
    const dReopen = Math.abs(norm180(deg(reopened.targetAngle - after.targetAngle)));
    report('survives-modal-reopen',
      dReopen < 0.01,
      `after=${deg(after.targetAngle).toFixed(2)}° reopened=${deg(reopened.targetAngle).toFixed(2)}° diff=${dReopen.toFixed(3)}°`);

    // --- F5: точка прибытия неизвестна → канонический азимут (маркер жив) ---
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForTimeout(1200);
    await page.evaluate((w) => {
      window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true });
    }, { id: creds.worldId, name: creds.worldName || 'QA-мир', spec: 'G' });
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForTimeout(700);
    const reloaded = await beltState(page, BELT_ID, null);
    if (reloaded.error) { report('after-reload-state', false, reloaded.error); return finish(1); }
    report('canonical-after-reload',
      Math.abs(norm180(deg(reloaded.targetAngle - reloaded.canonicalAngle))) < 0.5,
      `target=${deg(reloaded.targetAngle).toFixed(1)}° canonical=${deg(reloaded.canonicalAngle).toFixed(1)}°`);

    report('no-page-errors', pageErrors.length === 0, `errors=${pageErrors.length}`);
  } catch (err) {
    report('run', false, String(err && err.message ? err.message : err));
  }

  const failed = results.filter(r => !r.ok).length;
  console.log(`\n${results.length - failed}/${results.length} PASS`);
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.error('belt-nearest-check error:', e); finish(1); });
