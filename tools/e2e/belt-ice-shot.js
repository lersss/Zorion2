// tools/e2e/belt-ice-shot.js
// Живой кадр сцены добычи в поясе с подключёнными ЛЕДЯНЫМИ спрайтами
// (арт-ТЗ art_belt_asteroids.md §10, подэтап 4c): рисуем реальный мир BeltWorld
// и реальный drawScene (тот же код, что в /belt.html) на offscreen-канвас,
// прогреваем спрайты через preloadSprites(). Серверное состояние не нужно —
// мир клиентский, детерминирован от seed.
//
// Кадры:
//   artifacts/belt-ice-sprites.png  — смешанный пояс (камень + лёд), луч по льду;
//   artifacts/belt-ice-compare.png  — сравнение «лёд | камень» в игровом размере.
//
// Запуск: cd tools/e2e; node belt-ice-shot.js
// Переменные: BASE_URL (default http://localhost:8080).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const OUT_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean).find((p) => existsSync(p));

let browser = null;
async function finish(c) { if (browser) await browser.close().catch(() => {}); process.exit(c); }

async function main() {
  if (!CHROME) { console.error('Chrome/Edge not found'); return finish(2); }
  mkdirSync(OUT_DIR, { recursive: true });
  browser = await chromium.launch({ executablePath: CHROME, headless: true });
  const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
  const errors = [];
  page.on('pageerror', (e) => { errors.push(e.message); console.log('PAGEERROR:', e.message); });
  await page.goto(BASE_URL + '/health', { waitUntil: 'domcontentloaded' });

  // 1. Смешанный пояс: реальный мир + drawScene, цель — ледяная жила (луч/искры).
  const stats = await page.evaluate(async () => {
    const C = await import('/static/js/belt/belt_config.js');
    const W = await import('/static/js/belt/belt_world.js');
    const R = await import('/static/js/belt/belt_render.js');
    await R.preloadSprites();
    const mk = () => ({ forward: false, back: false, left: false, right: false, brake: false, space: false, mouseDown: false, aim: null, aimWorld: null });
    // Смешанный пояс (переходная зона): лёд 0.28 / железо 0.20 → ~58% ледяных жил.
    const w = new W.BeltWorld(424242, { ice: 0.28, iron: 0.20, iceAvailable: true });
    for (let i = 0; i < 10; i++) w.update(1 / 60, mk());
    // Цель — ближайшая ледяная жила; нос в неё (холодный луч).
    let target = null, bd = Infinity;
    for (const a of w.asteroids) {
      if (a.res !== 'ice') continue;
      const d = Math.hypot(a.x - w.ship.x, a.y - w.ship.y);
      if (d < bd) { bd = d; target = a; }
    }
    if (target) w.ship.heading = Math.atan2(target.y - w.ship.y, target.x - w.ship.x);

    const cv = document.createElement('canvas');
    cv.id = 'ice-shot-canvas';
    cv.width = 1600; cv.height = 900;
    cv.style.cssText = 'position:fixed; inset:0; z-index:99999;';
    document.body.appendChild(cv);
    const ctx = cv.getContext('2d');
    R.drawScene(ctx, w, { x: w.ship.x, y: w.ship.y }, 1600, 900, {
      drilling: !!target, target, depleted: false, depletedRes: { iron: false, ice: false },
      shipSprite: null, shipOrient: { angle: 0, flip: false }, offBelt: false, now: 1000,
      lightX: -0.55, lightY: -0.55,
    });
    return {
      iceVeins: w.asteroids.filter((a) => a.res === 'ice').length,
      ironVeins: w.asteroids.filter((a) => a.res === 'iron').length,
      iceDebris: w.debris.filter((d) => d.res === 'ice').length,
      iceSprite: !!R.getSprite(C.ICE_SPRITES[0]),
      iceVeinSprite: !!R.getSprite(C.ICE_VEIN_SPRITES[0]),
      iceDebrisSprite: !!R.getSprite(C.ICE_DEBRIS_SPRITES[0]),
    };
  });
  console.log('[mixed] ' + JSON.stringify(stats));
  const el = await page.$('#ice-shot-canvas');
  await el.screenshot({ path: path.join(OUT_DIR, 'belt-ice-sprites.png') });
  console.log('[shot] ' + path.join(OUT_DIR, 'belt-ice-sprites.png'));

  // 2. Сравнение «лёд | камень» в игровом размере: тот же drawScene, тела в ряд.
  await page.evaluate(async () => {
    const C = await import('/static/js/belt/belt_config.js');
    const W = await import('/static/js/belt/belt_world.js');
    const R = await import('/static/js/belt/belt_render.js');
    await R.preloadSprites();
    const w = new W.BeltWorld(7, { ice: 0.5, iron: 0.5, iceAvailable: true });
    const mkVein = (res, x, y, sprite, pattern) => ({
      x, y, vx: 0, vy: 0, r: 90, rot: 0, rotSpeed: 0, vein: true, res, sprite,
      veinPattern: pattern, veinRot: 0, veinScale: 1, veinRich: 1,
      shape: [1, 1, 1, 1, 1, 1, 1, 1], glints: [{ x: 0, y: 0, r: 3 }], drill: 0,
    });
    w.asteroids.length = 0;
    w.debris.length = 0;
    w.ship.x = -100000; w.ship.y = -100000; // корабль вне кадра — чистое сравнение
    // Ряд льда (верх) и камня (низ), по 3 формы, руда есть.
    for (let i = 0; i < 3; i++) w.asteroids.push(mkVein('ice', -260 + i * 260, -140, i, i));
    for (let i = 0; i < 3; i++) w.asteroids.push(mkVein('iron', -260 + i * 260, 160, i, i));
    const cv = document.getElementById('ice-shot-canvas');
    const ctx = cv.getContext('2d');
    R.drawScene(ctx, w, { x: 0, y: 0 }, 1600, 900, {
      drilling: false, target: null, depleted: false, depletedRes: { iron: false, ice: false },
      shipSprite: null, shipOrient: { angle: 0, flip: false }, offBelt: false, now: 1000,
      lightX: -0.55, lightY: -0.55,
    });
  });
  await el.screenshot({ path: path.join(OUT_DIR, 'belt-ice-compare.png') });
  console.log('[shot] ' + path.join(OUT_DIR, 'belt-ice-compare.png'));

  console.log('errors=' + errors.length);
  return finish(errors.length ? 1 : 0);
}
main().catch((e) => { console.error('belt-ice-shot error:', e); finish(1); });
