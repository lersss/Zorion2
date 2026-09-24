// tools/e2e/belt-sprites-shot.js
// Кадры из игры для приёмки спрайтов астероидов (подэтап 3c, арт-ТЗ
// art_belt_asteroids.md §4.4): (1) полёт среди камней, (2) бурение жилы
// (руда светится, искры), (3) тело с погашенной рудой / выработанный пояс.
// Стиль pacman-shot.js: playwright-core + системный Chrome, вход по токену.
//
// Запуск: cd tools/e2e; node belt-sprites-shot.js
// Переменные: BASE_URL (default http://localhost:8080), QA_TOKEN, QA_BELT_ID
// (игрок должен быть в поясе — иначе сцена не откроется). Кадры кладутся в
// ai_drafts/belt_asteroids/ingame/.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const TOKEN = process.env.QA_TOKEN || '';
const BELT_ID = process.env.QA_BELT_ID || '';
const OUT_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)),
  '..', '..', 'ai_drafts', 'belt_asteroids', 'ingame');
const CHROME = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean).find((p) => existsSync(p));

let browser = null;
async function finish(c) { if (browser) await browser.close().catch(() => {}); process.exit(c); }

async function main() {
  if (!TOKEN || !BELT_ID) { console.error('QA_TOKEN/QA_BELT_ID не заданы'); return finish(2); }
  if (!CHROME) { console.error('Chrome/Edge not found'); return finish(2); }
  mkdirSync(OUT_DIR, { recursive: true });

  browser = await chromium.launch({ executablePath: CHROME, headless: true });
  const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
  const errors = [];
  page.on('pageerror', (e) => { errors.push(e.message); console.log('PAGEERROR:', e.message); });
  await page.addInitScript((t) => localStorage.setItem('token', t), TOKEN);
  await page.goto(BASE_URL + '/belt.html?belt=' + BELT_ID, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(4000);

  const shot = async (name) => {
    const p = path.join(OUT_DIR, name + '.png');
    await page.screenshot({ path: p });
    console.log('[shot] ' + p);
  };

  // Диагностика: сколько тел рисуется спрайтом, сколько уходит в фолбэк
  // (нет файла/ошибка загрузки). Считаем по кэшу Image в belt_render.
  const spriteStats = async (label) => {
    const s = await page.evaluate(async () => {
      const R = await import('/static/js/belt/belt_render.js');
      const C = await import('/static/js/belt/belt_config.js');
      const w = window.__beltWorld;
      if (!w) return null;
      const ok = (name) => !!R.getSprite(name);
      let rockOk = 0, rockFallback = 0, debrisOk = 0, debrisFallback = 0, veinOk = 0, veinFallback = 0, iceOk = 0;
      for (const a of w.asteroids) {
        const rockName = (a.res === 'ice' && C.ICE_SPRITES.length)
          ? C.ICE_SPRITES[a.sprite % C.ICE_SPRITES.length] : C.ROCK_SPRITES[a.sprite];
        if (ok(rockName)) rockOk++; else rockFallback++;
        if (a.res === 'ice') iceOk++;
        const veinName = (a.res === 'ice' && C.ICE_VEIN_SPRITES.length)
          ? C.ICE_VEIN_SPRITES[a.veinPattern % C.ICE_VEIN_SPRITES.length] : C.VEIN_SPRITES[a.veinPattern];
        if (ok(veinName)) veinOk++; else veinFallback++;
      }
      for (const d of w.debris) {
        const dName = (d.res === 'ice' && C.ICE_DEBRIS_SPRITES.length)
          ? C.ICE_DEBRIS_SPRITES[d.sprite % C.ICE_DEBRIS_SPRITES.length] : C.DEBRIS_SPRITES[d.sprite];
        if (ok(dName)) debrisOk++; else debrisFallback++;
      }
      return { rockOk, rockFallback, debrisOk, debrisFallback, veinOk, veinFallback, iceOk };
    });
    if (s) {
      console.log(`[stats ${label}] rock sprite=${s.rockOk} fallback=${s.rockFallback} (ice=${s.iceOk})`
        + ` | debris sprite=${s.debrisOk} fallback=${s.debrisFallback}`
        + ` | vein sprite=${s.veinOk} fallback=${s.veinFallback}`);
    }
    return s;
  };

  // 1. Полёт среди камней: курсор в сторону, лёгкая тяга.
  await page.mouse.move(1200, 450);
  await page.keyboard.down('KeyW');
  await page.waitForTimeout(1200);
  await page.keyboard.up('KeyW');
  await page.waitForTimeout(400);
  await spriteStats('01_flight');
  await shot('01_flight');

  // 2. Бурение жилы: наводим нос на ближайшую жилу и держим ЛКМ.
  const aimed = await page.evaluate(() => {
    const w = window.__beltWorld;
    if (!w) return null;
    const s = w.ship;
    let best = null, bd = Infinity;
    for (const a of w.asteroids) {
      const d = Math.hypot(a.x - s.x, a.y - s.y);
      if (d < bd) { bd = d; best = a; }
    }
    if (!best) return null;
    // Ставим корабль у поверхности жилы и целимся в неё.
    const ang = Math.atan2(s.y - best.y, s.x - best.x);
    s.x = best.x + Math.cos(ang) * (best.r + 30);
    s.y = best.y + Math.sin(ang) * (best.r + 30);
    s.vx = 0; s.vy = 0;
    s.heading = Math.atan2(best.y - s.y, best.x - s.x);
    return { x: best.x, y: best.y, r: best.r };
  });
  if (aimed) {
    // Курсор в экранную точку жилы (камера следует за кораблём).
    await page.mouse.move(800, 450);
    await page.mouse.down();
    await page.waitForTimeout(2500);
    await spriteStats('02_drilling');
    await shot('02_drilling');
    await page.mouse.up();
  } else {
    console.log('[warn] __beltWorld недоступен — кадр бурения пропущен');
  }

  // 3. Погашенная руда: гасим drill у всех жил (визуальное истощение).
  await page.evaluate(() => {
    const w = window.__beltWorld;
    if (!w) return;
    for (const a of w.asteroids) a.drill = 1;
  });
  await page.waitForTimeout(600);
  await spriteStats('03_depleted_vein');
  await shot('03_depleted_vein');

  console.log('errors=' + errors.length);
  return finish(errors.length ? 1 : 0);
}
main().catch((e) => { console.error('belt-sprites-shot error:', e); finish(1); });
