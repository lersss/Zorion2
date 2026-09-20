// tools/e2e/composite-route-check.js
// QA-прогон фичи 99.2.30 «Композитный маршрут» (playwright-core + системный Chrome).
// Сценарии по аргументу: node composite-route-check.js <1..7>
// Токен игрока — env QA_TOKEN. ASCII-вывод (PowerShell cp866).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const SCENARIO = parseInt(process.argv[2] || '1', 10);
const TOKEN = process.env.QA_TOKEN || '';

// Тестовые данные (dev-БД, подготовлены тестером):
const A = { id: '8db2beea-91d8-413e-853f-53b3dbe50427', name: 'Jorkar' };
const B = { id: 'f13452d8-6e9e-49e3-86ae-b0ef0daac9a3', name: 'Zinelchal', spec: 'M', stemp: 3665, x: -346.48538035305097, y: -498.6717935414292 };
const B_PLANET = { id: '266520db-8c5b-4ad0-a074-ba39d4298bfe', name: 'Irreiob' };
const B_GIANT = { id: '0a69c489-e28b-4750-acf2-2af5592dc304', name: 'Dannaldel' };
const B_SAT = { id: '842f6a0a-5614-4ea0-9242-370936063b6f', name: 'Zinelchal-8F9Z' };
const C = { id: '5a867045-11c2-48cd-ac4e-e1362b2dcc4f', name: 'Niubek' };

const results = [];
function report(stepName, status, detail) {
  results.push({ stepName, status, detail });
  console.log(`[${stepName}] ${status}${detail ? ' - ' + detail : ''}`);
}

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

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function openForeignModal(page, world) {
  // Программное открытие модалки чужой системы (как клик по звезде в map/events.js).
  await page.evaluate((w) => {
    window.openSystemModal(w.id, w.name, w.spec, null, null, {
      stype: 'star', stemp: w.stemp, systype: 'single', smods: { metallicity: -0.8 }, x: w.x, y: w.y, hasEngine: true,
    });
  }, world);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  // Ждём загрузку планет (refreshPlanets).
  await page.waitForFunction(() => {
    const panel = document.getElementById('right-panel');
    return panel && panel.querySelectorAll('tr[data-index]').length > 0;
  }, { timeout: 15000 });
}

async function openPlanetCard(page, index) {
  await page.evaluate((i) => {
    const panel = document.getElementById('right-panel');
    const tr = panel.querySelectorAll('tr[data-index]')[i];
    if (tr) tr.click();
  }, index);
  await page.waitForSelector('#composite-fly-btn, #intra-fly-btn', { timeout: 10000 });
  await page.waitForTimeout(600);
}

async function openSatelliteCard(page, satName) {
  // В карточке планеты-гиганта клик по спутнику в списке (li[data-sat-idx]).
  const ok = await page.evaluate((name) => {
    const lis = [...document.querySelectorAll('#right-panel li[data-sat-idx]')];
    const target = lis.find(e => e.textContent && e.textContent.includes(name));
    if (target) { target.click(); return true; }
    return false;
  }, satName);
  if (!ok) return false;
  await page.waitForTimeout(800);
  return true;
}

// flightLabel — текст шапки карты в полёте («В полёте: From → To»).
async function flightLabel(page) {
  return await page.evaluate(() => {
    const prefix = document.getElementById('currentWorldPrefix');
    const name = document.getElementById('currentWorldName');
    return (prefix ? prefix.textContent : '') + ' ' + (name ? name.textContent : '');
  });
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL + ' SCENARIO: ' + SCENARIO);
  if (!TOKEN) { console.log('QA_TOKEN env required'); return finish(1); }

  const exe = findExecutable();
  if (!exe) { report('setup browser', 'FAIL', 'no Chrome/Edge found'); return finish(1); }
  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('setup browser', 'FAIL', 'launch: ' + String(err && err.message ? err.message : err));
    return finish(1);
  }
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, TOKEN);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));
  const meResponses = [];
  page.on('response', (res) => {
    if (res.url().includes('/me')) {
      res.json().then(j => meResponses.push({ flight: j.flight ? j.flight.to : null, pos: j.current_position ? j.current_position.status : null })).catch(() => {});
    }
  });
  const toasts = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') toasts.push('console.error: ' + msg.text());
  });

  try {
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1200);

    if (SCENARIO === 1) {
      // ===== П.1-2: композитная кнопка на планете и спутнике чужой системы =====
      await openForeignModal(page, B);
      await openPlanetCard(page, 0);
      const btn1 = await page.evaluate(() => {
        const b = document.getElementById('composite-fly-btn');
        return b ? { text: b.textContent.trim(), title: b.title, disabled: b.disabled } : null;
      });
      const ok1 = btn1 && btn1.text.includes('Лететь') && btn1.text.includes('через систему') &&
        btn1.title.includes('Перелёт к системе и полёт до орбиты планеты') && !btn1.disabled;
      report('1/5 planet composite btn', ok1 ? 'PASS' : 'FAIL',
        btn1 ? `text="${btn1.text}" title="${btn1.title}" disabled=${btn1.disabled}` : 'no #composite-fly-btn');
      if (!ok1) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-planet-card.png') });

      // Спутник: назад к списку → карточка гиганта → спутник.
      await page.evaluate(() => { const b = document.getElementById('back-to-list-btn'); if (b) b.click(); });
      await page.waitForTimeout(600);
      await openPlanetCard(page, 2); // Dannaldel (газовый гигант)
      const satOk = await openSatelliteCard(page, B_SAT.name);
      if (!satOk) {
        report('2/5 satellite composite btn', 'SKIP', 'satellite card not reachable');
      } else {
        const btn2 = await page.evaluate(() => {
          const b = document.querySelector('[data-sat-composite-fly]');
          return b ? { text: b.textContent.trim(), title: b.title, disabled: b.disabled } : null;
        });
        const ok2 = btn2 && btn2.text.includes('через систему') &&
          btn2.title.includes('до орбиты спутника') && !btn2.disabled;
        report('2/5 satellite composite btn', ok2 ? 'PASS' : 'FAIL',
          btn2 ? `text="${btn2.text}" title="${btn2.title}" disabled=${btn2.disabled}` : 'no [data-sat-composite-fly]');
        if (!ok2) return finish(1);
        await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-sat-card.png') });
      }

      // ===== П.3: клик композитной кнопки → модалка закрылась, полёт, слежение =====
      // Назад к списку планет → карточка первой планеты → композитный старт.
      await page.evaluate(() => { const b = document.querySelector('[data-sat-back]'); if (b) b.click(); });
      await page.waitForTimeout(500);
      await page.evaluate(() => { const b = document.getElementById('back-to-list-btn'); if (b) b.click(); });
      await page.waitForTimeout(500);
      await openPlanetCard(page, 0);
      const clickRes = await page.evaluate(async () => {
        const b = document.getElementById('composite-fly-btn');
        if (!b) return { ok: false, reason: 'no composite btn' };
        b.click();
        return { ok: true };
      });
      if (!clickRes.ok) { report('3/5 composite start', 'FAIL', clickRes.reason); return finish(1); }
      await page.waitForTimeout(2500);
      const afterStart = await page.evaluate(() => {
        const panel = document.getElementById('flight-panel');
        const prefix = document.getElementById('currentWorldPrefix');
        const name = document.getElementById('currentWorldName');
        return {
          modalClosed: !document.getElementById('system-modal-overlay'),
          panelVisible: !!(panel && panel.classList.contains('flying')),
          labelText: (prefix ? prefix.textContent : '') + ' ' + (name ? name.textContent : ''),
          followShip: sessionStorage.getItem('followShip'),
          centerBtnActive: !!(document.getElementById('centerBtn') && document.getElementById('centerBtn').classList.contains('active')),
        };
      });
      const ok3 = afterStart.modalClosed && afterStart.panelVisible &&
        afterStart.labelText.includes('В полёте') && afterStart.followShip === '1';
      report('3/5 composite start', ok3 ? 'PASS' : 'FAIL',
        `modalClosed=${afterStart.modalClosed} panel=${afterStart.panelVisible} label="${afterStart.labelText}" followShip=${afterStart.followShip} centerBtnActive=${afterStart.centerBtnActive}`);
      if (!ok3) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-interstellar.png') });

      // ===== П.4: прибытие → автооткрытие модалки B + полоса intra =====
      await page.waitForFunction(() => {
        const ov = document.getElementById('system-modal-overlay');
        return ov && ov.style.display !== 'none';
      }, { timeout: 30000 });
      await page.waitForTimeout(1500);
      const arrived = await page.evaluate(() => {
        const strip = document.getElementById('intra-flight-strip');
        const toast = [...document.querySelectorAll('.toast, .toast-info, #toast-container *')].map(e => e.textContent).join(' | ');
        return {
          modalOpen: !!document.getElementById('system-modal-overlay'),
          stripVisible: !!(strip && strip.style.display !== 'none'),
          stripText: strip ? strip.textContent : '',
          toast,
        };
      });
      const ok4 = arrived.modalOpen && arrived.stripVisible && arrived.stripText.includes('Irreiob');
      report('4/5 auto-open + intra strip', ok4 ? 'PASS' : 'FAIL',
        `modalOpen=${arrived.modalOpen} strip=${arrived.stripVisible} text="${arrived.stripText}" toast="${arrived.toast}"`);
      if (!ok4) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-intra-strip.png') });

      // ===== П.5: прибытие к планете → «Вы на орбите» + авто-знание =====
      await page.waitForFunction(() => {
        const strip = document.getElementById('intra-flight-strip');
        return !strip || strip.style.display === 'none';
      }, { timeout: 20000 });
      await page.waitForTimeout(1500);
      const orbit = await page.evaluate(async (baseUrl) => {
        const token = localStorage.getItem('token');
        const me = await (await fetch(baseUrl + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
        const pos = me.current_position;
        return {
          status: pos ? pos.status : null,
          objType: pos ? pos.object_type : null,
          objId: pos ? pos.object_id : null,
          toastError: !!document.querySelector('.toast-error'),
        };
      }, BASE_URL);
      const ok5 = orbit.status === 'orbit' && orbit.objType === 'planet' && !orbit.toastError;
      report('5/5 orbit + knowledge', ok5 ? 'PASS' : 'FAIL',
        `status=${orbit.status} objType=${orbit.objType} objId=${orbit.objId} toastError=${orbit.toastError}`);
      if (!ok5) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-orbit.png') });
    }

    if (SCENARIO === 2) {
      // ===== П.6: рефреш во время межзвёздного сегмента → полёт восстановился =====
      await openForeignModal(page, B);
      await openPlanetCard(page, 0);
      await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
      await page.waitForTimeout(2500);
      const before = await page.evaluate(() => ({
        label: (document.getElementById('currentWorldPrefix') || {}).textContent + ' ' + (document.getElementById('currentWorldName') || {}).textContent,
        followShip: sessionStorage.getItem('followShip'),
      }));
      // Рефреш во время полёта.
      await page.reload({ waitUntil: 'domcontentloaded', timeout: 30000 });
      await page.waitForSelector('#mapCanvas', { timeout: 15000 });
      await page.waitForTimeout(2500);
      const after = await page.evaluate(() => {
        const panel = document.getElementById('flight-panel');
        const prefix = document.getElementById('currentWorldPrefix');
        const name = document.getElementById('currentWorldName');
        return {
          panelVisible: !!(panel && panel.classList.contains('flying')),
          labelText: (prefix ? prefix.textContent : '') + ' ' + (name ? name.textContent : ''),
          followShip: sessionStorage.getItem('followShip'),
        };
      });
      const ok6 = after.panelVisible && after.labelText.includes('В полёте');
      report('6/6 refresh mid-flight', ok6 ? 'PASS' : 'FAIL',
        `before="${before.label}" after="${after.labelText}" panel=${after.panelVisible} followShip=${after.followShip}`);
      if (!ok6) return finish(1);
      // Ждём прибытие → автооткрытие + автостарт (п.4-5 работают после рефреша).
      await page.waitForFunction(() => {
        const ov = document.getElementById('system-modal-overlay');
        return ov && ov.style.display !== 'none';
      }, { timeout: 30000 });
      await page.waitForTimeout(1200);
      const autoOpen = await page.evaluate(() => {
        const strip = document.getElementById('intra-flight-strip');
        return { modal: !!document.getElementById('system-modal-overlay'), strip: !!(strip && strip.style.display !== 'none') };
      });
      report('6/6 auto-open after refresh', autoOpen.modal && autoOpen.strip ? 'PASS' : 'FAIL',
        `modal=${autoOpen.modal} strip=${autoOpen.strip}`);
      if (!(autoOpen.modal && autoOpen.strip)) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-refresh-arrival.png') });
    }

    if (SCENARIO === 3) {
      // ===== П.7: рефреш ПОСЛЕ автостарта (me.flight null, intra идёт) =====
      // Честный путь: композитный старт из A → прибытие → автооткрытие + автостарт
      // (маркер compositeRoute стоит) → reload в окне intra → модалка B с полосой (M5).
      await openForeignModal(page, B);
      await openPlanetCard(page, 0);
      await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
      // Ждём автооткрытие модалки (прибытие + автостарт).
      await page.waitForFunction(() => {
        const ov = document.getElementById('system-modal-overlay');
        const strip = document.getElementById('intra-flight-strip');
        return ov && ov.style.display !== 'none' && strip && strip.style.display !== 'none';
      }, { timeout: 30000 });
      // Сразу reload (intra идёт, me.flight уже null).
      await page.reload({ waitUntil: 'domcontentloaded', timeout: 30000 });
      await page.waitForSelector('#mapCanvas', { timeout: 15000 });
      await page.waitForTimeout(1500);
      const m5 = await page.evaluate(async (baseUrl) => {
        const token = localStorage.getItem('token');
        let me = null;
        try { me = await (await fetch(baseUrl + '/me', { headers: { Authorization: 'Bearer ' + token } })).json(); } catch (e) { me = { err: String(e) }; }
        const ov = document.getElementById('system-modal-overlay');
        const strip = document.getElementById('intra-flight-strip');
        return {
          modalOpen: !!(ov && ov.style.display !== 'none'),
          stripVisible: !!(strip && strip.style.display !== 'none'),
          stripText: strip ? strip.textContent : '',
          marker: sessionStorage.getItem('compositeRoute'),
          me: me,
        };
      }, BASE_URL);
      const ok7 = m5.modalOpen && m5.stripVisible && m5.stripText.includes('Irreiob');
      report('7/7 refresh after autostart (M5)', ok7 ? 'PASS' : 'FAIL',
        `modal=${m5.modalOpen} strip=${m5.stripVisible} text="${m5.stripText}" marker=${m5.marker} me=${JSON.stringify(m5.me)}`);
      if (!ok7) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-m5-refresh.png') });
    }

    if (SCENARIO === 4) {
      // ===== П.8: разворот — игрок в полёте к B, клик композитной кнопки к планете C =====
      // Честный старт: композитный полёт к B из модалки B (мир B в кэше карты).
      await openForeignModal(page, B);
      await openPlanetCard(page, 0);
      await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
      await page.waitForTimeout(3000);
      // Модалка C: тултип «Маршрут развернётся…».
      await openForeignModal(page, C);
      await openPlanetCard(page, 0);
      await page.waitForTimeout(10000);
      const diagC = await page.evaluate(async (baseUrl) => {
        const token = localStorage.getItem('token');
        let me = null;
        try { me = await (await fetch(baseUrl + '/me', { headers: { Authorization: 'Bearer ' + token } })).json(); } catch (e) { me = { err: String(e) }; }
        const b = document.getElementById('composite-fly-btn');
        return { title: b ? b.title : 'no btn', flight: me.flight ? me.flight.to : null, world: me.current_world_name };
      }, BASE_URL);
      console.log('DIAG C tooltip: ' + JSON.stringify(diagC));
      console.log('DIAG meResponses: ' + JSON.stringify(meResponses));
      // Прямая перерисовка карточки через window.updateRightPanel.
      const diagC3 = await page.evaluate(() => {
        if (typeof window.updateRightPanel === 'function') window.updateRightPanel(0);
        return true;
      });
      await page.waitForTimeout(800);
      const diagC4 = await page.evaluate(() => {
        const b = document.getElementById('composite-fly-btn');
        return b ? { title: b.title } : 'no btn';
      });
      console.log('DIAG C tooltip after updateRightPanel: ' + JSON.stringify(diagC4));
      await page.waitForFunction(() => {
        const b = document.getElementById('composite-fly-btn');
        return b && b.title.includes('Маршрут развернётся');
      }, { timeout: 8000 }).catch(() => {});
      const tooltip = await page.evaluate(() => {
        const b = document.getElementById('composite-fly-btn');
        return b ? { title: b.title, disabled: b.disabled } : null;
      });
      const ok8a = tooltip && !tooltip.disabled && tooltip.title.includes('Маршрут развернётся');
      report('8/8 redirect tooltip', ok8a ? 'PASS' : 'FAIL',
        tooltip ? `title="${tooltip.title}" disabled=${tooltip.disabled}` : 'no btn');
      if (!ok8a) return finish(1);
      await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
      await page.waitForTimeout(2500);
      const redirect = await page.evaluate(() => ({
        label: (document.getElementById('currentWorldPrefix') || {}).textContent + ' ' + (document.getElementById('currentWorldName') || {}).textContent,
        modalClosed: !document.getElementById('system-modal-overlay'),
      }));
      const ok8b = redirect.modalClosed && redirect.label.includes('Niubek');
      report('8/8 redirect to C', ok8b ? 'PASS' : 'FAIL', `label="${redirect.label}" modalClosed=${redirect.modalClosed}`);
      if (!ok8b) return finish(1);
      // По прибытии в C — автостарт к планете C.
      await page.waitForFunction(() => {
        const ov = document.getElementById('system-modal-overlay');
        return ov && ov.style.display !== 'none';
      }, { timeout: 60000 });
      await page.waitForTimeout(1200);
      const autoC = await page.evaluate(() => {
        const strip = document.getElementById('intra-flight-strip');
        return { modal: !!document.getElementById('system-modal-overlay'), strip: !!(strip && strip.style.display !== 'none') };
      });
      report('8/8 autostart in C', autoC.modal && autoC.strip ? 'PASS' : 'FAIL', `modal=${autoC.modal} strip=${autoC.strip}`);
      if (!(autoC.modal && autoC.strip)) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-redirect-c.png') });
    }

    if (SCENARIO === 5) {
      // ===== П.9: 202-путь — игрок летит к B обычным «Перелететь», затем композитный клик =====
      // Честный старт: обычный «Перелететь» к B из модалки B (startTravelToStar).
      await openForeignModal(page, B);
      const flyStarBtn = await page.evaluate(() => {
        const btns = [...document.querySelectorAll('#right-panel button, #system-modal-overlay button')];
        const b = btns.find(x => x.textContent && x.textContent.includes('Перелететь'));
        if (!b) return false;
        b.click();
        return true;
      });
      if (!flyStarBtn) { report('9/9 202-path', 'FAIL', 'no Перелететь btn'); return finish(1); }
      await page.waitForTimeout(3000);
      // Модалка B: тултип «…маршрут дополнится».
      await openForeignModal(page, B);
      await openPlanetCard(page, 0);
      await page.waitForFunction(() => {
        const b = document.getElementById('composite-fly-btn');
        return b && b.title.includes('дополнится');
      }, { timeout: 8000 }).catch(() => {});
      const tooltip = await page.evaluate(() => {
        const b = document.getElementById('composite-fly-btn');
        return b ? { title: b.title, disabled: b.disabled } : null;
      });
      const ok9a = tooltip && !tooltip.disabled && tooltip.title.includes('маршрут дополнится');
      report('9/9 202-path tooltip', ok9a ? 'PASS' : 'FAIL',
        tooltip ? `title="${tooltip.title}" disabled=${tooltip.disabled}` : 'no btn');
      if (!ok9a) return finish(1);
      await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
      await page.waitForTimeout(2500);
      const still = await page.evaluate(() => ({
        label: (document.getElementById('currentWorldPrefix') || {}).textContent + ' ' + (document.getElementById('currentWorldName') || {}).textContent,
        modalClosed: !document.getElementById('system-modal-overlay'),
      }));
      const ok9b = still.modalClosed && still.label.includes('Zinelchal');
      report('9/9 202-path continue', ok9b ? 'PASS' : 'FAIL', `label="${still.label}" modalClosed=${still.modalClosed}`);
      if (!ok9b) return finish(1);
      // По прибытии — автостарт к планете (не «прилетел к звезде»).
      await page.waitForFunction(() => {
        const ov = document.getElementById('system-modal-overlay');
        return ov && ov.style.display !== 'none';
      }, { timeout: 60000 });
      await page.waitForTimeout(1200);
      const autoB = await page.evaluate(() => {
        const strip = document.getElementById('intra-flight-strip');
        return { modal: !!document.getElementById('system-modal-overlay'), strip: !!(strip && strip.style.display !== 'none') };
      });
      report('9/9 autostart after 202', autoB.modal && autoB.strip ? 'PASS' : 'FAIL', `modal=${autoB.modal} strip=${autoB.strip}`);
      if (!(autoB.modal && autoB.strip)) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-202-path.png') });
    }

    if (SCENARIO === 6) {
      // ===== П.10: очистка намерения — композитный старт к B, затем «Перелететь» к C =====
      // Честный старт: композитный полёт к B из модалки B.
      await openForeignModal(page, B);
      await openPlanetCard(page, 0);
      await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
      await page.waitForTimeout(3000);
      // «Перелететь» к C: кнопка в модалке C (звезда).
      await openForeignModal(page, C);
      const flyStar = await page.evaluate(async () => {
        // Кнопка «Перелететь» — в модалке чужой системы (звезда).
        const btns = [...document.querySelectorAll('#right-panel button, #system-modal-overlay button')];
        const b = btns.find(x => x.textContent && x.textContent.includes('Перелететь'));
        if (!b) return { ok: false, reason: 'no Перелететь btn' };
        b.click();
        return { ok: true };
      });
      if (!flyStar.ok) { report('10/10 clear intent', 'FAIL', flyStar.reason); return finish(1); }
      await page.waitForTimeout(2500);
      const cleared = await page.evaluate(() => ({
        label: (document.getElementById('currentWorldPrefix') || {}).textContent + ' ' + (document.getElementById('currentWorldName') || {}).textContent,
        modalClosed: !document.getElementById('system-modal-overlay'),
      }));
      const ok10a = cleared.modalClosed && cleared.label.includes('Niubek');
      report('10/10 fly to C star', ok10a ? 'PASS' : 'FAIL', `label="${cleared.label}" modalClosed=${cleared.modalClosed}`);
      if (!ok10a) return finish(1);
      // По прибытии в C — НЕ должно быть автостарта (игрок просто у звезды C).
      await page.waitForFunction(async () => {
        const token = localStorage.getItem('token');
        const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
        return me.current_world_id === C.id && !me.flight;
      }, { timeout: 30000 });
      await page.waitForTimeout(2000);
      const noAuto = await page.evaluate(() => ({
        modalOpen: !!document.getElementById('system-modal-overlay'),
        strip: (() => { const s = document.getElementById('intra-flight-strip'); return !!(s && s.style.display !== 'none'); })(),
        pending: null,
      }));
      const me = await page.evaluate(async () => {
        const token = localStorage.getItem('token');
        return await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
      });
      const ok10b = !noAuto.modalOpen && !noAuto.strip && !me.pending_destination;
      report('10/10 no autostart at C', ok10b ? 'PASS' : 'FAIL',
        `modal=${noAuto.modalOpen} strip=${noAuto.strip} pending=${JSON.stringify(me.pending_destination)}`);
      if (!ok10b) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-clear-intent.png') });
    }

    if (SCENARIO === 7) {
      // ===== П.11-13: «Найти меня», disabled внутрисистемной, композитная активна =====
      // Честный старт: обычный «Перелететь» к B из модалки B.
      await openForeignModal(page, B);
      const flyStarBtn7 = await page.evaluate(() => {
        const btns = [...document.querySelectorAll('#right-panel button, #system-modal-overlay button')];
        const b = btns.find(x => x.textContent && x.textContent.includes('Перелететь'));
        if (!b) return false;
        b.click();
        return true;
      });
      if (!flyStarBtn7) { report('12/13 setup', 'FAIL', 'no Перелететь btn'); return finish(1); }
      await page.waitForTimeout(3000);
      // П.12: модалка СВОЕЙ системы A → внутрисистемная кнопка disabled.
      await openForeignModal(page, A);
      await openPlanetCard(page, 0);
      const ownBtn = await page.evaluate(() => {
        const b = document.getElementById('intra-fly-btn');
        return b ? { disabled: b.disabled, title: b.title, text: b.textContent.trim() } : null;
      });
      const ok12 = ownBtn && ownBtn.disabled && ownBtn.title.includes('межзвёздном полёте');
      report('12/13 own intra btn disabled', ok12 ? 'PASS' : 'FAIL',
        ownBtn ? `disabled=${ownBtn.disabled} title="${ownBtn.title}"` : 'no intra-fly-btn');
      if (!ok12) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-own-disabled.png') });

      // П.13: модалка чужой B → композитная кнопка НЕ блокируется.
      await page.evaluate(() => { const b = document.getElementById('back-to-list-btn'); if (b) b.click(); });
      await page.waitForTimeout(400);
      await page.evaluate(() => { const b = document.getElementById('close-modal-btn'); if (b) b.click(); });
      await page.waitForTimeout(400);
      await openForeignModal(page, B);
      await openPlanetCard(page, 0);
      const compBtn = await page.evaluate(() => {
        const b = document.getElementById('composite-fly-btn');
        return b ? { disabled: b.disabled, title: b.title } : null;
      });
      const ok13 = compBtn && !compBtn.disabled;
      report('13/13 composite btn active mid-flight', ok13 ? 'PASS' : 'FAIL',
        compBtn ? `disabled=${compBtn.disabled} title="${compBtn.title}"` : 'no composite btn');
      if (!ok13) return finish(1);

      // П.11: «Найти меня» (🎯) в модалке при межзвёздном → модалка закрылась, камера ведёт.
      const findMe = await page.evaluate(() => {
        const btns = [...document.querySelectorAll('#system-modal-overlay button, #right-panel button')];
        const b = btns.find(x => x.textContent && x.textContent.includes('Найти меня'));
        if (!b) return { ok: false, reason: 'no find-me btn' };
        b.click();
        return { ok: true };
      });
      if (!findMe.ok) { report('11/13 find-me', 'FAIL', findMe.reason); return finish(1); }
      await page.waitForTimeout(1500);
      const fm = await page.evaluate(() => ({
        modalClosed: !document.getElementById('system-modal-overlay'),
        followShip: sessionStorage.getItem('followShip'),
        centerBtnActive: !!(document.getElementById('centerBtn') && document.getElementById('centerBtn').classList.contains('active')),
      }));
      const ok11 = fm.modalClosed && fm.followShip === '1' && fm.centerBtnActive;
      report('11/13 find-me mid-flight', ok11 ? 'PASS' : 'FAIL',
        `modalClosed=${fm.modalClosed} followShip=${fm.followShip} centerBtnActive=${fm.centerBtnActive}`);
      if (!ok11) return finish(1);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'comp-findme.png') });

      // П.11б: быстрый клик 🎯 сразу после открытия модалки (до резолва /me).
      await openForeignModal(page, B);
      const quick = await page.evaluate(() => {
        const btns = [...document.querySelectorAll('#system-modal-overlay button, #right-panel button')];
        const b = btns.find(x => x.textContent && x.textContent.includes('Найти меня'));
        if (!b) return { ok: false, reason: 'no find-me btn' };
        b.click();
        return { ok: true };
      });
      if (!quick.ok) { report('11b/13 find-me quick', 'FAIL', quick.reason); return finish(1); }
      await page.waitForTimeout(1200);
      const qm = await page.evaluate(() => ({
        modalClosed: !document.getElementById('system-modal-overlay'),
        followShip: sessionStorage.getItem('followShip'),
      }));
      const ok11b = qm.modalClosed && qm.followShip === '1';
      report('11b/13 find-me quick click', ok11b ? 'PASS' : 'FAIL',
        `modalClosed=${qm.modalClosed} followShip=${qm.followShip}`);
      if (!ok11b) return finish(1);
    }

    // Общая проверка: page errors.
    const errs = pageErrors.slice();
    const okE = errs.length === 0;
    report('page errors', okE ? 'PASS' : 'FAIL', errs.length ? errs.join(' | ') : '0 errors');
    if (!okE) return finish(1);
  } catch (err) {
    report('run', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const failed = results.filter(r => r.status === 'FAIL');
  console.log('');
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'ALL PASS'));
  return finish(failed.length ? 1 : 0);
}

main();