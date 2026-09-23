// tools/e2e/layers-check.js
// e2e-смоук реестра слоёв интерфейса (спека 2026-09-23 «Слои интерфейса и
// стек попапов» §7 п.1–5): ошибка-алерт открывается ПОВЕРХ модалки системы и
// модалка остаётся; Esc/клик-вне закрывают только верхний слой; видимый тост
// не мешает Esc закрыть модалку и сам не гаснет; после закрытия меню в стеке
// не остаётся «призрачного» слоя; фикс «трюм полон» — клик «Добывать» при
// полном трюме остаётся на карте, модалка цела, состояние игрока не меняется.
//
// Требует: поднятый dev-сервер (BASE_URL). Токен не нужен — тест регистрирует
// свежего игрока сам (как map-check.js). Интерцепторы: /api/worlds/*/planets
// (в систему добавляется QA-пояс + позиция «в поясе») и /api/cargo (полный трюм).
//
// Запуск: cd tools/e2e; npm.cmd i; node layers-check.js
// Переменные: BASE_URL (default http://localhost:8080).
// ASCII-вывод (PowerShell cp866).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');

// QA-пояс и позиция игрока — инжектируются в ответ системы (детерминизм, без
// зависимости от того, есть ли в мире настоящие пояса у свежего игрока).
const BELT_ID = 'qa-belt-layers';

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

async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'e2e_layers_' + Date.now() + '_' + attempt;
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

// pageState — состояние слоёв и модалки «одним запросом» (z-index и наличие узлов).
async function pageState(page) {
  return page.evaluate(() => {
    const cs = (el, prop) => (el ? getComputedStyle(el)[prop] : null);
    const modal = document.getElementById('system-modal-overlay');
    const alertEl = document.querySelector('.ui-alert-overlay');
    const menu = document.getElementById('star-context-menu');
    const toasts = Array.from(document.querySelectorAll('#toast-container .toast'));
    return {
      modal: !!modal,
      modalZ: cs(modal, 'zIndex'),
      alert: !!alertEl,
      alertZ: cs(alertEl, 'zIndex'),
      alertTitle: (document.querySelector('.ui-alert-title') || {}).textContent || '',
      menu: !!menu,
      menuZ: cs(menu, 'zIndex'),
      toastCount: toasts.length,
      toastZ: cs(document.getElementById('toast-container'), 'zIndex'),
      path: location.pathname,
    };
  });
}

// topLayerState — верхний маршрутизируемый слой по мнению реестра.
async function topLayerState(page) {
  return page.evaluate(async () => {
    const L = await import('/static/js/ui/layers.js');
    const t = L.topLayer();
    return t ? { level: t.level, connected: !!t.el.isConnected, id: t.el.id || t.el.className } : null;
  });
}

async function openAlertViaModule(page, title) {
  await page.evaluate(async (t) => {
    const A = await import('/static/js/ui/alert.js');
    A.openAlert({ title: t, text: 'QA-текст' });
  }, title);
  await page.waitForSelector('.ui-alert-overlay', { timeout: 3000 });
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
  console.log('BASE_URL: ' + BASE_URL + ', world: ' + creds.worldId);

  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => localStorage.setItem('token', t), creds.token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => { pageErrors.push(e.message); console.log('PAGEERROR:', e.message); });

  // Интерцепторы: система с QA-поясом + позиция «в поясе»; трюм полон.
  let cargoServed = 0;
  await page.route('**/api/worlds/*/planets*', async (route) => {
    const res = await route.fetch();
    let json = {};
    try { json = await res.json(); } catch (e) { json = {}; }
    const belts = Array.isArray(json.belts) ? json.belts.slice() : [];
    if (!belts.some(b => b && b.id === BELT_ID)) {
      belts.push({
        id: BELT_ID, name: 'QA-пояс', kind: 'asteroid',
        radius_au: 2.5, width_au: 0.5, body_size_km: 5, mass: 0.001,
        belt_class: 'средний', remaining_level: 'истощается',
      });
    }
    json.belts = belts;
    json.my_position = { status: 'orbit', object_type: 'belt', object_id: BELT_ID };
    await route.fulfill({ json });
  });
  await page.route('**/api/cargo', async (route) => {
    cargoServed++;
    await route.fulfill({ json: { limits: { mass: { used: 12, total: 12 } }, items: [] } });
  });

  async function openModal() {
    await page.evaluate((w) => {
      window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true });
    }, { id: creds.worldId, name: 'QA-система', spec: 'G' });
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForSelector('[data-belt-row]', { timeout: 10000 });
    await page.waitForTimeout(400);
  }

  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForTimeout(1500);

    // --- Полосы: sanity (числа из одного места, alert выше модалки) ---
    const bands = await page.evaluate(async () => {
      const L = await import('/static/js/ui/layers.js');
      return L.LEVELS;
    });
    report('bands',
      bands.modal === 1000 && bands.menu === 1100 && bands.alert === 2000 && bands.toast === 10000,
      JSON.stringify(bands));

    // ===== (m) меню мира и тултип координат (map/events.js) =====
    const mapPoints = await page.evaluate(async () => {
      const cfg = await import('/static/js/map/config.js');
      const s = cfg.state;
      const canvas = document.getElementById('mapCanvas');
      if (!canvas) return null;
      const r = canvas.getBoundingClientRect();
      const fx = r.width / canvas.width;
      const fy = r.height / canvas.height;
      const singles = (s.clusters || []).filter(c => c.cnt === 1);
      let star = null;
      for (const c of singles) {
        const x = r.left + (c.x * s.scale + s.offsetX) * fx;
        const y = r.top + (c.y * s.scale + s.offsetY) * fy;
        if (x < r.left + 40 || x > r.right - 40 || y < r.top + 40 || y > r.bottom - 40) continue;
        star = { x, y, name: c.sname || '' };
        break;
      }
      // Пустая точка: подальше от любой одиночной звезды (для тултипа координат).
      let empty = null;
      for (let gx = 60; gx < r.width - 60 && !empty; gx += 40) {
        for (let gy = 60; gy < r.height - 60; gy += 40) {
          const x = r.left + gx, y = r.top + gy;
          const near = singles.some(c => {
            const sx = r.left + (c.x * s.scale + s.offsetX) * fx;
            const sy = r.top + (c.y * s.scale + s.offsetY) * fy;
            return Math.hypot(sx - x, sy - y) < 50;
          });
          if (!near) { empty = { x, y }; break; }
        }
      }
      return { star, empty };
    });
    if (mapPoints && mapPoints.star) {
      await page.mouse.click(mapPoints.star.x, mapPoints.star.y, { button: 'right' });
      await page.waitForTimeout(250);
      const worldMenu = await page.evaluate(() => {
        const m = document.getElementById('map-context-menu');
        return { menu: !!m, z: m ? getComputedStyle(m).zIndex : null };
      });
      const topWorldMenu = await topLayerState(page);
      report('m1 map-world-menu-layer',
        worldMenu.menu && Number(worldMenu.z) === bands.menu &&
        !!topWorldMenu && topWorldMenu.level === 'menu' && topWorldMenu.connected,
        `menu=${worldMenu.menu} z=${worldMenu.z} top=${JSON.stringify(topWorldMenu)}`);
      await page.keyboard.press('Escape');
      await page.waitForTimeout(200);
      const afterWorldEsc = await page.evaluate(() => !!document.getElementById('map-context-menu'));
      const topAfterWorldEsc = await topLayerState(page);
      report('m2 esc-closes-map-menu-no-ghost',
        !afterWorldEsc && topAfterWorldEsc === null,
        `menu=${afterWorldEsc} top=${JSON.stringify(topAfterWorldEsc)}`);
    } else {
      report('m1 map-world-menu-layer', false, 'нет одиночной звезды в кадре');
      report('m2 esc-closes-map-menu-no-ghost', false, 'нет одиночной звезды в кадре');
    }

    if (mapPoints && mapPoints.empty) {
      await page.mouse.click(mapPoints.empty.x, mapPoints.empty.y, { button: 'right' });
      await page.waitForTimeout(250);
      const coords = await page.evaluate(() => {
        const t = document.getElementById('coords-tooltip');
        return { exists: !!t, z: t ? getComputedStyle(t).zIndex : null };
      });
      const topCoords = await topLayerState(page);
      report('m3 coords-tooltip-passive',
        coords.exists && Number(coords.z) === bands.menu && topCoords === null,
        `exists=${coords.exists} z=${coords.z} top=${JSON.stringify(topCoords)}`);
      await page.mouse.click(mapPoints.empty.x + 30, mapPoints.empty.y + 30);
      await page.waitForTimeout(250);
      const coordsAfter = await page.evaluate(async () => {
        const L = await import('/static/js/ui/layers.js');
        return { exists: !!document.getElementById('coords-tooltip'), top: L.topLayer() };
      });
      report('m4 leftclick-hides-coords-tooltip',
        !coordsAfter.exists && coordsAfter.top === null,
        `exists=${coordsAfter.exists} top=${JSON.stringify(coordsAfter.top)}`);

      // Три цикла показа/скрытия: z тултипа не растёт — мёртвых записей в
      // стеке не остаётся (при «призраке» каждый цикл прибавлял бы +10).
      const cycleZ = [];
      for (let i = 0; i < 3; i++) {
        await page.mouse.click(mapPoints.empty.x, mapPoints.empty.y, { button: 'right' });
        await page.waitForTimeout(200);
        cycleZ.push(await page.evaluate(() => {
          const t = document.getElementById('coords-tooltip');
          return t ? Number(getComputedStyle(t).zIndex) : null;
        }));
        await page.mouse.click(mapPoints.empty.x + 30, mapPoints.empty.y + 30);
        await page.waitForTimeout(200);
      }
      report('m5 coords-tooltip-no-z-growth',
        cycleZ.every(z => z !== null && z >= bands.menu) && cycleZ[0] === cycleZ[1] && cycleZ[1] === cycleZ[2],
        `z=${JSON.stringify(cycleZ)}`);
    } else {
      report('m3 coords-tooltip-passive', false, 'нет пустой точки в кадре');
      report('m4 leftclick-hides-coords-tooltip', false, 'нет пустой точки в кадре');
    }

    await openModal();

    // ===== (f) фикс «трюм полон»: клик «Добывать» остаётся на карте =====
    // ПКМ по строке пояса → меню → «Добывать»: трюм полон (интерцептор /api/cargo).
    await page.click('[data-belt-row]', { button: 'right' });
    await page.waitForSelector('#star-context-menu', { timeout: 5000 });
    const beforePos = await page.evaluate(async () => {
      const m = await import('/static/js/modal/state.js');
      const p = m.modalState.myPosition || {};
      return { status: p.status, object_type: p.object_type, object_id: p.object_id };
    });
    await page.locator('#star-context-menu div').filter({ hasText: 'Добывать' }).first().click();
    await page.waitForSelector('.ui-alert-overlay', { timeout: 5000 });
    const fullHold = await pageState(page);
    const afterPos = await page.evaluate(async () => {
      const m = await import('/static/js/modal/state.js');
      const p = m.modalState.myPosition || {};
      return { status: p.status, object_type: p.object_type, object_id: p.object_id };
    });
    report('f1 hold-full-alert-over-modal',
      fullHold.alert && fullHold.modal && fullHold.alertTitle.indexOf('Трюм полон') >= 0,
      `alert=${fullHold.alert} modal=${fullHold.modal} title="${fullHold.alertTitle}"`);
    report('f2 stay-on-map',
      fullHold.path === '/map' && !fullHold.menu && cargoServed > 0,
      `path=${fullHold.path} menu=${fullHold.menu} cargoCalls=${cargoServed}`);
    report('f3 read-only-verdict',
      JSON.stringify(beforePos) === JSON.stringify(afterPos) && afterPos.status === 'orbit',
      JSON.stringify(afterPos));
    report('f4 alert-above-modal',
      Number(fullHold.alertZ) >= bands.alert && Number(fullHold.modalZ) === bands.modal,
      `alertZ=${fullHold.alertZ} modalZ=${fullHold.modalZ}`);

    // «Понятно» закрывает ТОЛЬКО алерт.
    await page.locator('.ui-alert-btn').first().click();
    await page.waitForTimeout(200);
    const afterOk = await pageState(page);
    report('f5 ok-closes-alert-only', !afterOk.alert && afterOk.modal, `alert=${afterOk.alert} modal=${afterOk.modal}`);

    // ===== (a) ошибка-алерт поверх модалки (через модуль) =====
    await openAlertViaModule(page, 'QA ошибка поверх модалки');
    const overModal = await pageState(page);
    report('a1 alert-over-modal',
      overModal.alert && overModal.modal && Number(overModal.alertZ) > Number(overModal.modalZ),
      `alertZ=${overModal.alertZ} modalZ=${overModal.modalZ}`);

    // ===== (b) Esc закрывает только верхний: алерт → потом модалку =====
    const selBefore = await page.evaluate(async () => {
      const m = await import('/static/js/modal/state.js');
      return m.modalState.selectedPlanetIndex;
    });
    await page.keyboard.press('Escape');
    await page.waitForTimeout(200);
    const esc1 = await pageState(page);
    const selAfter = await page.evaluate(async () => {
      const m = await import('/static/js/modal/state.js');
      return m.modalState.selectedPlanetIndex;
    });
    report('b1 esc-closes-alert-only',
      !esc1.alert && esc1.modal && selBefore === selAfter,
      `alert=${esc1.alert} modal=${esc1.modal} planetIdx=${selAfter}`);

    // ===== (d) клик вне закрывает только верхний =====
    await page.click('[data-belt-row]', { button: 'right' });
    await page.waitForSelector('#star-context-menu', { timeout: 5000 });
    await page.mouse.click(8, 8);
    await page.waitForTimeout(250);
    const outside = await pageState(page);
    report('d1 outside-closes-top-only',
      !outside.menu && outside.modal,
      `menu=${outside.menu} modal=${outside.modal}`);

    // ===== (e) без «призрачных» слоёв: повторный Esc адресуется модалке =====
    const topAfterOutside = await topLayerState(page);
    report('e1 no-ghost-layer',
      !!topAfterOutside && topAfterOutside.level === 'modal' && topAfterOutside.connected,
      JSON.stringify(topAfterOutside));

    await page.click('[data-belt-row]', { button: 'right' });
    await page.waitForSelector('#star-context-menu', { timeout: 5000 });
    await page.keyboard.press('Escape');
    await page.waitForTimeout(200);
    const afterEscMenu = await pageState(page);
    const topAfterEsc = await topLayerState(page);
    report('e2 esc-closes-menu-not-modal',
      !afterEscMenu.menu && afterEscMenu.modal && !!topAfterEsc &&
      topAfterEsc.level === 'modal' && topAfterEsc.connected,
      `menu=${afterEscMenu.menu} modal=${afterEscMenu.modal} top=${JSON.stringify(topAfterEsc)}`);

    // ===== (i) три уровня: меню поверх модалки, ошибка поверх меню =====
    await page.click('[data-belt-row]', { button: 'right' });
    await page.waitForSelector('#star-context-menu', { timeout: 5000 });
    await openAlertViaModule(page, 'QA ошибка поверх меню');
    const stack3 = await pageState(page);
    report('i1 stack-order-z',
      stack3.modal && stack3.menu && stack3.alert &&
      Number(stack3.alertZ) > Number(stack3.menuZ) && Number(stack3.menuZ) > Number(stack3.modalZ),
      `modal=${stack3.modalZ} menu=${stack3.menuZ} alert=${stack3.alertZ}`);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(200);
    const i2 = await pageState(page);
    report('i2 esc-alert-keeps-menu-and-modal',
      !i2.alert && i2.menu && i2.modal, `alert=${i2.alert} menu=${i2.menu} modal=${i2.modal}`);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(200);
    const i3 = await pageState(page);
    report('i3 esc-menu-keeps-modal', !i3.menu && !i3.alert && i3.modal, `menu=${i3.menu} modal=${i3.modal}`);

    // ===== (g) пассивный тултип звезды: свой цикл, Esc не перехватывает =====
    const hovered = await page.evaluate(() => {
      const c = document.getElementById('system-canvas');
      if (!c) return null;
      const r = c.getBoundingClientRect();
      return { cx: r.left + r.width / 2, cy: r.top + r.height / 2 };
    });
    let tooltipShown = false;
    if (hovered) {
      for (const d of [0, 16, -16, 32, -32, 48, -48]) {
        await page.mouse.move(hovered.cx + d, hovered.cy);
        await page.waitForTimeout(120);
        tooltipShown = await page.evaluate(() => {
          const t = document.getElementById('star-tooltip');
          return !!t && t.style.display !== 'none' && t.isConnected;
        });
        if (tooltipShown) break;
      }
    }
    // Уводим мышь — тултип гаснет, слой не остаётся «призраком»: Esc попадает в модалку.
    await page.mouse.move(4, 4);
    await page.waitForTimeout(200);
    const tooltipAfter = await page.evaluate(() => {
      const t = document.getElementById('star-tooltip');
      return { exists: !!t, visible: !!t && t.style.display !== 'none' };
    });
    const topAfterTooltip = await topLayerState(page);
    report('g1 star-tooltip-passive',
      tooltipShown && !tooltipAfter.visible &&
      !!topAfterTooltip && topAfterTooltip.level === 'modal' && topAfterTooltip.connected,
      `shown=${tooltipShown} visibleAfter=${tooltipAfter.visible} top=${JSON.stringify(topAfterTooltip)}`);

    // ===== (b2) второй Esc — модалку (+ (д): скрытый тултип не «призрак») =====
    // Один жест: если бы от тултипа остался мёртвый слой, Esc ушёл бы в него и
    // модалка осталась бы открытой — обе проверки падают.
    await page.keyboard.press('Escape');
    await page.waitForTimeout(250);
    const esc2 = await pageState(page);
    report('b2 esc-then-closes-modal', !esc2.modal && !esc2.alert, `modal=${esc2.modal}`);
    report('g2 ghost-layer-does-not-eat-esc', !esc2.modal, `modal=${esc2.modal}`);

    // ===== (h) дропдаун поиска сущностей — над модалкой, Esc закрывает его =====
    await openModal();
    // Запрос по имени своего мира — гарантированно непустой список (иначе
    // строка «Ничего не найдено»: z и перекрытие те же, но дропдаун не проверен).
    await page.fill('#entity-search', creds.worldName || 'a');
    await page.waitForSelector('#entity-search-results:not([hidden])', { timeout: 8000 });
    const drop = await page.evaluate(() => {
      const d = document.getElementById('entity-search-results');
      const r = d.getBoundingClientRect();
      const top = document.elementFromPoint(r.left + Math.min(8, r.width / 2), r.top + 4);
      return {
        z: getComputedStyle(d).zIndex,
        hitInside: !!top && (top === d || d.contains(top)),
        items: d.querySelectorAll('.entity-search-item').length,
      };
    });
    report('h1 dropdown-above-modal',
      Number(drop.z) >= bands.menu && drop.hitInside && drop.items > 0,
      `z=${drop.z} hitInside=${drop.hitInside} items=${drop.items}`);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(200);
    const dropEsc = await page.evaluate(() => ({
      hidden: document.getElementById('entity-search-results').hidden,
      modal: !!document.getElementById('system-modal-overlay'),
    }));
    report('h2 esc-closes-dropdown-only', dropEsc.hidden && dropEsc.modal,
      `hidden=${dropEsc.hidden} modal=${dropEsc.modal}`);

    // ===== (c) видимый тост не мешает Esc закрыть модалку и сам не гаснет =====
    await page.evaluate(async () => {
      const t = await import('/static/js/ui/toast.js');
      t.notifyError('QA тост — не гаснет по Esc', 0);
    });
    await page.waitForTimeout(150);
    const withToast = await pageState(page);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(250);
    const toastEsc = await pageState(page);
    // Реестр: после закрытия модалки остаётся только пассивный слой тоста
    // (hasOpen — есть запись, topLayer — маршрутизируемых нет).
    const toastLayers = await page.evaluate(async () => {
      const L = await import('/static/js/ui/layers.js');
      return { hasOpen: L.hasOpen(), top: L.topLayer() };
    });
    report('c toast-does-not-eat-esc',
      withToast.toastCount > 0 && !toastEsc.modal && toastEsc.toastCount > 0 &&
      Number(toastEsc.toastZ) === bands.toast &&
      toastLayers.hasOpen && toastLayers.top === null,
      `toastBefore=${withToast.toastCount} modalAfterEsc=${toastEsc.modal} toastAfter=${toastEsc.toastCount} toastZ=${toastEsc.toastZ} layers=${JSON.stringify(toastLayers)}`);

    // Крестик тоста — собственный: закрывает тост, а пустой контейнер
    // освобождает слой (в стеке не остаётся мёртвой записи).
    await page.click('#toast-container .toast-close');
    await page.waitForTimeout(450);
    const toastClosed = await page.evaluate(async () => {
      const L = await import('/static/js/ui/layers.js');
      return {
        toasts: document.querySelectorAll('#toast-container .toast').length,
        hasOpen: L.hasOpen(),
      };
    });
    report('c2 toast-close-frees-layer',
      toastClosed.toasts === 0 && !toastClosed.hasOpen,
      `toasts=${toastClosed.toasts} hasOpen=${toastClosed.hasOpen}`);

    // ===== (p) пакман: баннер и оверлей — пассивная полоса banner =====
    await page.evaluate(async () => {
      const m = await import('/static/js/map/pacman.js');
      m.handlePacmanMessage({ type: 'pacman_start', total: 5 });
    });
    await page.waitForTimeout(150);
    const banner = await page.evaluate(() => {
      const b = document.getElementById('pacman-banner');
      return { exists: !!b, visible: !!b && b.style.display !== 'none', z: b ? getComputedStyle(b).zIndex : null };
    });
    const topWithBanner = await topLayerState(page);
    report('p1 pacman-banner-passive',
      banner.exists && banner.visible && Number(banner.z) === bands.banner && topWithBanner === null,
      `visible=${banner.visible} z=${banner.z} top=${JSON.stringify(topWithBanner)}`);

    await openModal();
    await page.keyboard.press('Escape');
    await page.waitForTimeout(250);
    const bannerAfterEsc = await page.evaluate(() => {
      const b = document.getElementById('pacman-banner');
      return {
        visible: !!b && b.style.display !== 'none',
        modal: !!document.getElementById('system-modal-overlay'),
      };
    });
    report('p2 banner-does-not-eat-esc',
      !bannerAfterEsc.modal && bannerAfterEsc.visible,
      `modalAfterEsc=${bannerAfterEsc.modal} bannerVisible=${bannerAfterEsc.visible}`);

    const pacmanEnd = await page.evaluate(async () => {
      const m = await import('/static/js/map/pacman.js');
      m.handlePacmanMessage({ type: 'pacman_end', status: 'done', eaten_total: 5, total: 5 });
      const b = document.getElementById('pacman-banner');
      return { visible: !!b && b.style.display !== 'none' };
    });
    const topAfterPacman = await topLayerState(page);
    report('p3 banner-hide-no-ghost',
      !pacmanEnd.visible && topAfterPacman === null,
      `visible=${pacmanEnd.visible} top=${JSON.stringify(topAfterPacman)}`);

    // Оверлей «Галактика пуста» рисуется из drawPacman: 0 кластеров + нет мира.
    const emptyOverlay = await page.evaluate(async () => {
      const cfg = await import('/static/js/map/config.js');
      const P = await import('/static/js/map/pacman.js');
      const s = cfg.state;
      const saved = { clusters: s.clusters, worldId: s.currentWorldId, active: s.pacman.active };
      const ctx = document.getElementById('mapCanvas').getContext('2d');
      s.clusters = [];
      s.currentWorldId = null;
      s.pacman.active = false;
      P.drawPacman(ctx, s.canvasWidth || 800, s.canvasHeight || 600);
      const el = document.getElementById('pacman-empty-overlay');
      const shown = !!el && Number(getComputedStyle(el).zIndex) === 1200;
      s.clusters = saved.clusters;
      s.currentWorldId = saved.worldId;
      s.pacman.active = saved.active;
      P.drawPacman(ctx, s.canvasWidth || 800, s.canvasHeight || 600);
      return { shown, existsAfter: !!document.getElementById('pacman-empty-overlay') };
    });
    const topAfterEmpty = await topLayerState(page);
    report('p4 empty-galaxy-overlay-passive',
      emptyOverlay.shown && !emptyOverlay.existsAfter && topAfterEmpty === null,
      `shown=${emptyOverlay.shown} after=${emptyOverlay.existsAfter} top=${JSON.stringify(topAfterEmpty)}`);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'layers-check.png') });
    report('no-page-errors', pageErrors.length === 0, `errors=${pageErrors.length}`);
  } catch (err) {
    report('run', false, String(err && err.message ? err.message : err));
  }

  const failed = results.filter(r => !r.ok).length;
  console.log(`\n${results.length - failed}/${results.length} PASS`);
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.error('layers-check error:', e); finish(1); });
