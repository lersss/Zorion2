// tools/e2e/belt-mining-check.js
// e2e-смоук сцены добычи в поясе (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §12.2, ревизия 4 — пункты 8–15):
// управление «два стика» (доворот носа к курсору, стрейф, плавная тяга),
// бурение по наведению носа, Space = ЛКМ, ПКМ подавлена, бесконечный вдоль
// кольца пояс (стриминг/кап) и рассеивание поперёк за край.
//
// Механика проверяется на реальных модулях сцены (import из served /static),
// без серверного состояния: детерминированный BeltWorld + ввод. Если заданы
// QA_TOKEN и QA_BELT_ID (игрок в поясе) — дополнительно открывается живая
// сцена web/belt.html и проверяется отсутствие pageerror/HUD/легенда.
//
// Запуск: cd tools/e2e; npm.cmd i; node belt-mining-check.js
// Переменные: BASE_URL (default http://localhost:8080), QA_TOKEN, QA_BELT_ID.
// ASCII-вывод (PowerShell cp866).
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const TOKEN = process.env.QA_TOKEN || '';
const BELT_ID = process.env.QA_BELT_ID || '';
const CHROME = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean).find((p) => existsSync(p));

let browser = null;
async function finish(c) { if (browser) await browser.close().catch(() => {}); process.exit(c); }

const results = [];
function report(name, ok, detail) {
  results.push({ name, ok: !!ok });
  console.log(`[${name}] ${ok ? 'PASS' : 'FAIL'}${detail ? ' - ' + detail : ''}`);
}

// Проверки механики (§12.2 п.8–15) — в контексте страницы, на реальных модулях.
async function mechanics(page) {
  return page.evaluate(async () => {
    const C = await import('/static/js/belt/belt_config.js');
    const W = await import('/static/js/belt/belt_world.js');
    const I = await import('/static/js/belt/belt_input.js');
    const U = await import('/static/js/belt/belt_ui.js');
    const out = [];
    const rec = (name, ok, detail) => out.push({ name, ok: !!ok, detail: detail || '' });
    const mk = () => ({ forward: false, back: false, left: false, right: false, brake: false, space: false, mouseDown: false, aim: null, aimWorld: null });
    const dt = 1 / 60;
    const norm = (a) => { while (a > Math.PI) a -= 2 * Math.PI; while (a < -Math.PI) a += 2 * Math.PI; return a; };
    const target = Math.PI / 2;

    // 8. Мышь / доворот: плавно, за кадр не равен цели, за ~0.5 с — на цели.
    {
      const w = new W.BeltWorld(101);
      const inp = mk();
      const h0 = w.ship.heading;
      inp.aimWorld = { x: w.ship.x, y: w.ship.y + 1000 };
      w.update(dt, inp);
      const h1 = w.ship.heading;
      for (let i = 0; i < 30; i++) { inp.aimWorld = { x: w.ship.x, y: w.ship.y + 1000 }; w.update(dt, inp); }
      const h2 = w.ship.heading;
      rec('08 mouse-turn-gradual',
        Math.abs(norm(h1 - h0)) > 1e-4 && Math.abs(norm(h1 - target)) > 0.1 && Math.abs(norm(h2 - target)) < 0.06,
        `h1=${h1.toFixed(3)} h2=${h2.toFixed(3)} target=${target.toFixed(3)}`);
    }

    // 9. Стрейф: D сдвигает вбок, heading не меняется.
    {
      const w = new W.BeltWorld(102);
      const inp = mk();
      inp.right = true;
      for (let i = 0; i < 30; i++) { inp.aimWorld = { x: w.ship.x + 1000, y: w.ship.y }; w.update(dt, inp); }
      rec('09 strafe', Math.abs(w.ship.y) > 5 && Math.abs(w.ship.heading) < 0.01,
        `y=${w.ship.y.toFixed(2)} heading=${w.ship.heading.toFixed(4)}`);
    }

    // 10. Плавная тяга: разгон нарастает, сброс не мгновенный.
    {
      const w = new W.BeltWorld(103);
      const inp = mk();
      inp.forward = true;
      w.update(dt, inp);
      const v1 = Math.hypot(w.ship.vx, w.ship.vy);
      for (let i = 0; i < 59; i++) w.update(dt, inp);
      const v60 = Math.hypot(w.ship.vx, w.ship.vy);
      inp.forward = false;
      w.update(dt, inp);
      const thr1 = w.ship.throttle;
      for (let i = 0; i < 30; i++) w.update(dt, inp);
      rec('10 throttle-smooth', v1 < v60 * 0.2 && thr1 > 0.8 && w.ship.throttle < 0.05,
        `v1=${v1.toFixed(2)} v60=${v60.toFixed(1)} thrAfter1=${thr1.toFixed(3)} thrEnd=${w.ship.throttle.toFixed(3)}`);
    }

    // 11. Бурение по наведению: цель — жила под лучом носа (изолированная жила).
    {
      const w = new W.BeltWorld(104);
      w.asteroids.length = 0;
      const vein = { x: 500, y: 0, vx: 0, vy: 0, r: 50, rot: 0, rotSpeed: 0, vein: true, shape: [1, 1, 1, 1, 1, 1, 1, 1], glints: [], drill: 0 };
      w.asteroids.push(vein);
      w.ship.x = 500 - (50 + 20); w.ship.y = 0; w.ship.heading = 0; w.ship.angVel = 0;
      const hitA = w.rayVein();
      const aimedA = w.aimedVein();
      w.ship.heading = Math.PI; // нос в пустоту
      const hitB = w.rayVein();
      w.ship.heading = 0; // жила под лучом, но дальше EXTRACT_RADIUS
      w.ship.x = 500 - (50 + C.EXTRACT_RADIUS + 200);
      const hitC = w.rayVein();
      const aimedC = w.aimedVein();
      rec('11 drill-aim', hitA === vein && aimedA === vein && !hitB && hitC === vein && !aimedC,
        `aimed=${!!aimedA} noseVoid=${!!hitB} farHit=${!!hitC} farAimed=${!!aimedC}`);
    }

    // 12 + 13. Space = ЛКМ; ПКМ подавлена (contextmenu на канвасе).
    {
      const canvas = document.createElement('canvas');
      document.body.appendChild(canvas);
      const inp = mk();
      I.bindInput(inp, { canvas });
      window.dispatchEvent(new KeyboardEvent('keydown', { code: 'Space', bubbles: true, cancelable: true }));
      const spaceDown = inp.space;
      window.dispatchEvent(new KeyboardEvent('keyup', { code: 'Space' }));
      canvas.dispatchEvent(new MouseEvent('mousedown', { button: 0, bubbles: true }));
      const lmbDown = inp.mouseDown;
      canvas.dispatchEvent(new MouseEvent('mouseup', { button: 0, bubbles: true }));
      const s1 = I.drillHeld({ space: true });
      const s2 = I.drillHeld({ mouseDown: true });
      rec('12 space-equals-lmb', spaceDown && lmbDown && s1 && s2 && !I.drillHeld({}),
        `space=${spaceDown} lmb=${lmbDown}`);
      const ev = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
      const notCanceled = canvas.dispatchEvent(ev);
      rec('13 rmb-suppressed', !notCanceled && ev.defaultPrevented, `defaultPrevented=${ev.defaultPrevented}`);
      canvas.remove();
    }

    // 14. Вдоль кольца: тела не кончаются, число в памяти не растёт (кап).
    {
      const w = new W.BeltWorld(105);
      w.ship.x = 20000;
      w.update(dt, mk());
      const near1 = w.asteroids.filter((a) => Math.hypot(a.x - w.ship.x, a.y - w.ship.y) <= C.BELT_STREAM_RADIUS).length;
      const cap1 = w.asteroids.length <= C.BELT_ASTEROID_CAP;
      w.ship.x = 60000;
      w.update(dt, mk());
      const near2 = w.asteroids.filter((a) => Math.hypot(a.x - w.ship.x, a.y - w.ship.y) <= C.BELT_STREAM_RADIUS).length;
      rec('14 infinite-along-ring', near1 > 0 && near2 > 0 && cap1 && w.asteroids.length <= C.BELT_ASTEROID_CAP,
        `near@20k=${near1} near@60k=${near2} mem=${w.asteroids.length} cap=${C.BELT_ASTEROID_CAP}`);
    }

    // 15. Поперёк за край: камней нет, стены/отскока нет; возврат — тела снова.
    {
      const w = new W.BeltWorld(106);
      w.ship.x = 0;
      w.ship.y = C.BELT_HALF_WIDTH + C.BELT_STREAM_RADIUS + 600;
      w.ship.vy = 60;
      w.update(dt, mk());
      const near = w.asteroids.filter((a) => Math.hypot(a.x - w.ship.x, a.y - w.ship.y) <= C.BELT_STREAM_RADIUS).length;
      const vyBeyond = w.ship.vy;
      const noWall = vyBeyond > 0;
      w.ship.y = 0; w.ship.vy = 0;
      w.update(dt, mk());
      const back = w.asteroids.length;
      rec('15 edge-dissipates', near === 0 && noWall && back > 0,
        `beyond=${near} vy=${vyBeyond.toFixed(1)} back=${back}`);
    }

    // 16. Кап жил реально режет память: с малым капом путь _trim исполняется
    // (при дефолтных 120 фактическая плотность ~20-40 тел до капа не доходит).
    {
      const w = new W.BeltWorld(107);
      const smallCap = 6; // заведомо меньше плотности стрима вокруг корабля
      w._layers.a.cap = smallCap;
      w.ship.x = 30000;
      w.update(dt, mk());
      const n = w.asteroids.length;
      rec('16 cap-trim-load', n > 0 && n <= smallCap,
        `mem=${n} cap=${smallCap} (default=${C.BELT_ASTEROID_CAP})`);
    }

    // 17. Повторный вход с тем же seed: тот же мир (тела совпадают), другой seed — другой.
    {
      const key = (list) => list.map((b) => `${b.x.toFixed(3)}:${b.y.toFixed(3)}:${b.r.toFixed(3)}`).sort().join('|');
      const a = new W.BeltWorld(2026); a.ship.x = 12345; a.update(dt, mk());
      const b = new W.BeltWorld(2026); b.ship.x = 12345; b.update(dt, mk());
      const c = new W.BeltWorld(2027); c.ship.x = 12345; c.update(dt, mk());
      const ka = key(a.asteroids), kb = key(b.asteroids), kc = key(c.asteroids);
      rec('17 seed-determinism', a.asteroids.length > 0 && ka === kb && ka !== kc,
        `n=${a.asteroids.length} sameSeed=${ka === kb} otherSeed=${ka !== kc}`);
    }

    // 18. Декор и пыль гаснут по МИРОВОЙ координате вместе с камнями (§5.1/§8.2):
    // генератор слоя не выпускает тело за мировую полосу (world y = layer y / parallax),
    // независимо от cull — проверяем сами ячейки.
    {
      const w = new W.BeltWorld(108);
      let overDecor = 0, overDust = 0, nDecor = 0, nDust = 0;
      const eps = 1e-6;
      for (let cx = 0; cx < 20; cx++) {
        for (let cy = -4; cy <= 4; cy++) {
          const d = w._genDebris(cx, cy);
          const u = w._genDustPatch(cx, cy);
          nDecor += d.length; nDust += u.length;
          for (const b of d) if (Math.abs(b.y / C.PARALLAX_DECOR) > C.BELT_HALF_WIDTH + eps) overDecor++;
          for (const b of u) if (Math.abs(b.y / C.PARALLAX_DUST) > C.BELT_HALF_WIDTH + eps) overDust++;
        }
      }
      rec('18 decor-dust-world-band', nDecor > 0 && nDust > 0 && overDecor === 0 && overDust === 0,
        `decor=${nDecor} overDecor=${overDecor} dust=${nDust} overDust=${overDust}`);
    }

    // 19. Легенда: у «мышь» и «ЛКМ» свои ключи — ЛКМ подсвечивает только себя,
    // «мышь» — по наведению (input.aim). Разметка belt.html не дублирует ключ.
    {
      const wrap = document.createElement('div');
      wrap.innerHTML = '<span class="ctrl-key" data-key="mouse"></span>'
        + '<span class="ctrl-key" data-key="lmb"></span>'
        + '<span class="ctrl-key" data-key="forward"></span>';
      document.body.appendChild(wrap);
      const on = (k) => {
        const el = wrap.querySelector('.ctrl-key[data-key="' + k + '"]');
        return !!el && el.classList.contains('active');
      };
      U.updateControls(mk());
      const none = !on('mouse') && !on('lmb') && !on('forward');
      U.updateControls(Object.assign(mk(), { mouseDown: true }));
      const lmbOnly = on('lmb') && !on('mouse');
      U.updateControls(Object.assign(mk(), { aim: { x: 1, y: 1 } }));
      const mouseOnly = on('mouse') && !on('lmb');
      U.updateControls(Object.assign(mk(), { forward: true }));
      const fwdOnly = on('forward') && !on('mouse') && !on('lmb');
      wrap.remove();
      const html = await (await fetch('/belt.html')).text();
      const lmbChip = html.indexOf('data-key="lmb"') >= 0;
      const mouseChips = (html.match(/data-key="mouse"/g) || []).length;
      rec('19 legend-chips', none && lmbOnly && mouseOnly && fwdOnly && lmbChip && mouseChips === 1,
        `none=${none} lmbOnly=${lmbOnly} mouseOnly=${mouseOnly} fwdOnly=${fwdOnly} lmbChip=${lmbChip} mouseChips=${mouseChips}`);
    }

    return out;
  });
}

async function main() {
  if (!CHROME) { console.error('Chrome/Edge not found'); return finish(2); }
  browser = await chromium.launch({ executablePath: CHROME, headless: true });
  const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
  page.on('pageerror', (e) => console.log('PAGEERROR:', e.message));

  // Страница того же origin (модули сцены доступны через /static).
  await page.goto(BASE_URL + '/health', { waitUntil: 'domcontentloaded' }).catch(() => {});
  const checks = await mechanics(page);
  for (const c of checks) report(c.name, c.ok, c.detail);
  const mechFailed = checks.some((c) => !c.ok);

  // Живая сцена (опционально): требует токен и игрока в поясе.
  if (TOKEN && BELT_ID) {
    const live = await browser.newPage({ viewport: { width: 1600, height: 900 } });
    const liveErrors = [];
    live.on('pageerror', (e) => { liveErrors.push(e.message); console.log('PAGEERROR:', e.message); });
    await live.addInitScript((t) => localStorage.setItem('token', t), TOKEN);
    await live.goto(BASE_URL + '/belt.html?belt=' + BELT_ID, { waitUntil: 'domcontentloaded' });
    await live.waitForTimeout(3500);
    const state = await live.evaluate(() => {
      const canvas = document.getElementById('belt-canvas');
      const hud = document.getElementById('hud');
      const legend = document.querySelector('.hud-controls');
      return {
        canvas: !!canvas,
        hudVisible: !!hud && hud.style.display !== 'none',
        legend: !!legend && legend.textContent.indexOf('ЛКМ') >= 0 && !!legend.querySelector('.ctrl-key[data-key="forward"]'),
      };
    });
    report('live scene (canvas+HUD+legend)', state.canvas && state.hudVisible && state.legend && liveErrors.length === 0,
      `canvas=${state.canvas} hud=${state.hudVisible} legend=${state.legend} errors=${liveErrors.length}`);
    await live.close();
  } else {
    console.log('[live scene] SKIP - QA_TOKEN/QA_BELT_ID not set');
  }

  const failed = mechFailed || results.some((r) => !r.ok);
  console.log(`\n${results.filter((r) => r.ok).length}/${results.length} PASS`);
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.error('belt-mining-check error:', e); finish(1); });
