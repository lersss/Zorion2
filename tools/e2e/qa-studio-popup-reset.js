// tools/e2e/qa-studio-popup-reset.js — независимый прогон @tester (2026-09-24):
// правка web/studio.html — resetPopupPos() в openPopup/openProdPopup/openItemPopup
// очищает inline left/top/transform, чтобы попап детали открывался по центру
// области (а не в оставленной drag-ом позиции). Реальный браузер
// (playwright-core + системный Chrome), вход — adminToken в localStorage.
// Run: node qa-studio-popup-reset.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const PASSWORD = 'qa-popreset-' + Date.now();
const VP = { w: 1280, h: 800 }; // окно из прошлой находки (bottom=1142 > 800)
const GOOD_A = 'Пища';

const results = [];
function report(step, ok, detail) {
  results.push({ step, ok, detail });
  console.log(`[${step}] ${ok ? 'PASS' : 'FAIL'} - ${detail}`);
}
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}
function psqlRun(sql, filename) {
  const sqlPath = path.join(ARTIFACTS_DIR, filename);
  writeFileSync(sqlPath, sql, 'utf8');
  return String(execFileSync('cmd', ['/c', process.env.PSQL || 'C:\\pgsql\\pgsql\\bin\\psql.exe',
    '-h', '127.0.0.1', '-U', 'zorion', '-d', 'zorion', '-t', '-A', '-f', sqlPath],
    { env: { ...process.env, PGPASSWORD: process.env.PGPASSWORD || 'zorion123', PGCLIENTENCODING: 'UTF8' } })).trim();
}

let token = '';
async function registerAdmin() {
  let username = null, tok = null;
  for (let i = 0; i < 3; i++) {
    username = 'qa_popreset_' + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
    if (res.status === 201) { tok = (await res.json()).token; break; }
    if (res.status !== 409) throw new Error('register HTTP ' + res.status);
  }
  if (!tok) throw new Error('register failed');
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + tok } })).json();
  psqlRun(`UPDATE users SET role='admin' WHERE id='${me.id}';\n`, 'qa-popreset-role.sql');
  const lr = await fetch(BASE_URL + '/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
  if (lr.ok) tok = (await lr.json()).token;
  return { token: tok, username, uid: me.id };
}

let browser = null;
const consoleProblems = [];
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}
function attachDiagnostics(page, tag) {
  page.on('pageerror', (err) => consoleProblems.push(tag + ' pageerror: ' + String(err && err.stack ? err.stack : err).slice(0, 300)));
  page.on('console', (m) => { if (m.type() === 'error') { let loc = ''; try { loc = m.location() && m.location().url ? ' @ ' + m.location().url : ''; } catch (e) {} consoleProblems.push(tag + ' console.error: ' + m.text().slice(0, 200) + loc); } });
  page.on('response', (r) => { if (r.status() >= 400) consoleProblems.push(tag + ' HTTP ' + r.status() + ' ' + r.url()); });
  page.on('requestfailed', (r) => consoleProblems.push(tag + ' REQFAIL ' + r.url() + ' ' + (r.failure() && r.failure().errorText)));
}
async function openStudio(tag) {
  const context = await browser.newContext({ viewport: { width: VP.w, height: VP.h } });
  await context.addInitScript((t) => { try { localStorage.setItem('adminToken', t); } catch (e) {} }, token);
  const page = await context.newPage();
  attachDiagnostics(page, tag);
  await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForFunction(() => { const o = document.getElementById('loadingOverlay'); return o && o.style.display === 'none'; }, null, { timeout: 15000 });
  await page.waitForFunction(() => typeof state !== 'undefined' && Array.isArray(state.goods) && state.goods.length > 0, null, { timeout: 10000 });
  return { context, page };
}

// Показать все товары: обе галки + все тиры (иначе список пуст/частичен).
async function ensureAllGoods(page) {
  await page.evaluate(() => {
    document.getElementById('fUsed').checked = true;
    document.getElementById('fUnused').checked = true;
    tierSel.clear();
    spravTiers().forEach(t => tierSel.add(t));
    saveTierSel();
    renderSprav();
  });
}
async function cardHandle(page, name) {
  const h = await page.evaluateHandle((name) => {
    const cards = [...document.querySelectorAll('#spravList .card')];
    return cards.find(c => {
      const nm = c.querySelector('.nm');
      if (!nm) return false;
      return nm.textContent.replace(/[ⓘ?×\s]/g, '') === name;
    }) || null;
  }, name);
  return h.asElement();
}
// Реальное открытие карточки товара двойным кликом (как игрок).
async function openGoodCard(page, name) {
  const el = await cardHandle(page, name);
  if (!el) return { ok: false, reason: 'карточка «' + name + '» не отрисована в #spravList' };
  await el.dblclick();
  await page.waitForFunction(() => {
    const e = document.getElementById('detailPopup');
    return e && e.style.display !== 'none' && document.querySelector('#popupBody .pblock');
  }, null, { timeout: 10000 });
  await page.waitForTimeout(350);
  return { ok: true };
}

function measure(page) {
  return page.evaluate(() => {
    const p = document.getElementById('detailPopup');
    const r = p.getBoundingClientRect();
    const main = document.getElementById('main').getBoundingClientRect();
    const cw = document.getElementById('canvasWrap').getBoundingClientRect();
    return {
      top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right),
      w: Math.round(r.width), h: Math.round(r.height),
      cx: Math.round(r.left + r.width / 2), cy: Math.round(r.top + r.height / 2),
      mainTop: Math.round(main.top), mainBottom: Math.round(main.bottom),
      mainCx: Math.round(main.left + main.width / 2), mainCy: Math.round(main.top + main.height / 2),
      cwLeft: Math.round(cw.left), cwRight: Math.round(cw.right), cwTop: Math.round(cw.top), cwBottom: Math.round(cw.bottom),
      innerW: window.innerWidth, innerH: window.innerHeight,
      styleLeft: p.style.left, styleTop: p.style.top, styleTransform: p.style.transform,
      title: document.getElementById('popupTitle').textContent,
    };
  });
}
function inWindow(m) { return m.top >= 0 && m.bottom <= m.innerH && m.left >= 0 && m.right <= m.innerW; }
function centeredInMain(m) { return Math.abs(m.cx - m.mainCx) <= 2 && Math.abs(m.cy - m.mainCy) <= 2; }
function fmt(m) {
  return `top=${m.top} bottom=${m.bottom} left=${m.left} right=${m.right} h=${m.h} w=${m.w} inner=${m.innerW}x${m.innerH} center=(${m.cx},${m.cy}) mainCenter=(${m.mainCx},${m.mainCy})`;
}

// Перетащить попап за шапку к правому-нижнему краю окна.
async function dragToBottomRight(page) {
  const g = await page.evaluate(() => {
    const b = document.getElementById('detailPopup').getBoundingClientRect();
    const h = document.getElementById('popupHead').getBoundingClientRect();
    return { left: Math.round(b.left), top: Math.round(b.top), right: Math.round(b.right), bottom: Math.round(b.bottom),
      headX: Math.round(h.left + 40), headY: Math.round(h.top + h.height / 2), innerW: window.innerWidth, innerH: window.innerHeight };
  });
  const dx = Math.max(40, (g.innerW - 10) - g.right);   // гарантированно вправо
  const dy = Math.max(0, (g.innerH - 10) - g.bottom);   // вниз, если есть место
  await page.mouse.move(g.headX, g.headY);
  await page.mouse.down();
  await page.mouse.move(g.headX + dx, g.headY + dy, { steps: 10 });
  await page.mouse.up();
  await page.waitForTimeout(180);
  const a = await page.evaluate(() => {
    const b = document.getElementById('detailPopup').getBoundingClientRect();
    const p = document.getElementById('detailPopup');
    return { left: Math.round(b.left), top: Math.round(b.top), right: Math.round(b.right), bottom: Math.round(b.bottom),
      styleLeft: p.style.left, styleTop: p.style.top, styleTransform: p.style.transform };
  });
  return { before: { left: g.left, top: g.top, right: g.right, bottom: g.bottom }, after: a, dx: a.left - g.left, dy: a.top - g.top };
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);
  const admin = await registerAdmin();
  token = admin.token;
  report('setup admin', !!token, 'username=' + admin.username);

  const exe = findExecutable();
  if (!exe) { report('browser', false, 'no Chrome/Edge'); await finish(1); }
  browser = await chromium.launch({ executablePath: exe, headless: true });

  const { context, page } = await openStudio('main');
  await ensureAllGoods(page);
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popreset-01-list.png') });

  // ---- item 1: открыть «Товары» → карточку «Пища» → по центру, в окне ----
  const o1 = await openGoodCard(page, GOOD_A);
  let m1 = null;
  if (o1.ok) {
    m1 = await measure(page);
    report('1 попап по центру и в окне', inWindow(m1) && centeredInMain(m1), `title="${m1.title}" ${fmt(m1)} inWindow=${inWindow(m1)} centered=${centeredInMain(m1)}`);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popreset-02-item1.png') });
  } else {
    report('1 попап по центру и в окне', false, o1.reason);
  }

  // ---- item 2: перетащить к правому-нижнему краю ----
  let drag = null;
  if (m1) {
    drag = await dragToBottomRight(page);
    const moved = Math.abs(drag.dx) >= 30 || Math.abs(drag.dy) >= 20;
    report('2 перетаскивание смещает попап', moved,
      `before=(${drag.before.left},${drag.before.top}) after=(${drag.after.left},${drag.after.top}) d=(${drag.dx},${drag.dy}) inlineLeft="${drag.after.styleLeft}" inlineTop="${drag.after.styleTop}"`);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popreset-03-dragged.png') });
  } else {
    report('2 перетаскивание смещает попап', false, 'нет открытого попапа');
  }

  // ---- item 3: закрыть крестиком → открыть ту же карточку → снова по центру ----
  if (m1) {
    await page.click('#popupClose');
    await page.waitForFunction(() => document.getElementById('detailPopup').style.display === 'none', null, { timeout: 5000 });
    const o3 = await openGoodCard(page, GOOD_A);
    if (o3.ok) {
      const m3 = await measure(page);
      const same = Math.abs(m3.left - m1.left) <= 2 && Math.abs(m3.top - m1.top) <= 2 && Math.abs(m3.right - m1.right) <= 2 && Math.abs(m3.bottom - m1.bottom) <= 2;
      report('3 повторное открытие снова по центру', same && inWindow(m3) && centeredInMain(m3),
        `reopen top=${m3.top} bottom=${m3.bottom} left=${m3.left} right=${m3.right} (было top=${m1.top} left=${m1.left}); equals=${same} inWindow=${inWindow(m3)} centered=${centeredInMain(m3)} inlineLeft="${m3.styleLeft}" inlineTop="${m3.styleTop}"`);
      await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popreset-04-reopened.png') });
    } else {
      report('3 повторное открытие снова по центру', false, o3.reason);
    }
  } else {
    report('3 повторное открытие снова по центру', false, 'нет исходных координат');
  }

  // ---- item 4: перетащить → НЕ закрывая открыть другую карточку товара ----
  {
    const drag2 = await dragToBottomRight(page);
    const otherName = await page.evaluate((a) => {
      const cards = [...document.querySelectorAll('#spravList .card')];
      for (const c of cards) {
        const nm = c.querySelector('.nm');
        if (!nm) continue;
        const name = nm.textContent.replace(/[ⓘ?×\s]/g, '');
        if (name && name !== a) return name;
      }
      return null;
    }, GOOD_A);
    let m4 = null, how = 'dblclick';
    if (otherName) {
      // попап перекрывает часть правой панели — открываем через штатный хендлер
      // карточки (ondblclick=spravDblClick), диспатчим событие на самой карточке.
      const el = await cardHandle(page, otherName);
      if (el) {
        await page.evaluate((node) => node.dispatchEvent(new MouseEvent('dblclick', { bubbles: true })), el);
        how = 'dispatchEvent(ondblclick)';
        await page.waitForFunction((nm) => document.getElementById('popupTitle').textContent === nm, otherName, { timeout: 5000 }).catch(() => {});
        await page.waitForTimeout(300);
        m4 = await measure(page);
      }
    }
    const ok4 = !!m4 && inWindow(m4) && centeredInMain(m4) && m4.title === otherName;
    report('4 другое открытие — снова по центру', ok4,
      `drag2 d=(${drag2.dx},${drag2.dy}); открыт товар "${otherName}" via ${how}; title="${m4 ? m4.title : '-'}" ${m4 ? fmt(m4) : 'нет замера'} inWindow=${m4 ? inWindow(m4) : '-'} centered=${m4 ? centeredInMain(m4) : '-'} inlineLeft="${m4 ? m4.styleLeft : '-'}"`);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popreset-05-other.png') });
  }

  // ---- item 5: регресс — постройка, «Категории», «Эффекты» ----
  {
    await page.evaluate(() => { closePopup(); });
    await page.evaluate(() => { document.getElementById('branchProducers').click(); });
    await page.waitForFunction(() => typeof state !== 'undefined' && Array.isArray(state.producer_types) && state.producer_types.length > 0, null, { timeout: 10000 });
    await page.evaluate(() => openProdPopup(state.producer_types[0].id));
    await page.waitForFunction(() => document.getElementById('detailPopup').style.display !== 'none', null, { timeout: 10000 });
    await page.waitForTimeout(350);
    const mp = await measure(page);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popreset-06-producer.png') });

    await page.evaluate(() => { closePopup(); openCatPopup(); });
    await page.waitForFunction(() => document.getElementById('catPopup').style.display !== 'none', null, { timeout: 5000 });
    await page.waitForTimeout(200);
    const mc = await page.evaluate(() => { const r = document.getElementById('catPopup').getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), innerW: window.innerWidth, innerH: window.innerHeight }; });
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popreset-07-cat.png') });

    await page.evaluate(() => { closeCatPopup(); openEffectPopup(); });
    await page.waitForFunction(() => document.getElementById('effPopup').style.display !== 'none', null, { timeout: 5000 });
    await page.waitForTimeout(200);
    const me = await page.evaluate(() => { const r = document.getElementById('effPopup').getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), innerW: window.innerWidth, innerH: window.innerHeight }; });
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popreset-08-eff.png') });

    const prodOk = mp.top >= 0 && mp.bottom <= mp.innerH && mp.left >= 0 && mp.right <= mp.innerW;
    const catOk = mc.top >= 0 && mc.bottom <= mc.innerH && mc.left >= 0 && mc.right <= mc.innerW;
    const effOk = me.top >= 0 && me.bottom <= me.innerH && me.left >= 0 && me.right <= me.innerW;
    const dragOk = !!drag && (Math.abs(drag.dx) >= 30 || Math.abs(drag.dy) >= 20);
    report('5 регресс: drag/постройка/категории/эффекты', dragOk && prodOk && catOk && effOk,
      `drag=${dragOk} (item2 d=(${drag ? drag.dx : '-'},${drag ? drag.dy : '-'})); producer "${mp.title}" top=${mp.top} bottom=${mp.bottom} left=${mp.left} right=${mp.right} inner=${mp.innerW}x${mp.innerH} ok=${prodOk}; catPopup top=${mc.top} bottom=${mc.bottom} left=${mc.left} right=${mc.right} ok=${catOk}; effPopup top=${me.top} bottom=${me.bottom} left=${me.left} right=${me.right} ok=${effOk}`);
  }

  await context.close();

  // ---- item 6: консоль/сеть ----
  const bad = consoleProblems.filter(s => !/favicon\.ico/.test(s));
  report('6 консоль без JS-ошибок и ответов >=400', bad.length === 0,
    'problems=' + bad.length + (bad.length ? ' :: ' + bad.slice(0, 4).join(' | ') : '') + ' (favicon-404 отфильтрован: ' + (consoleProblems.length - bad.length) + ')');

  try { psqlRun(`DELETE FROM users WHERE id='${admin.uid}';\n`, 'qa-popreset-deluser.sql'); } catch (e) {}

  const fails = results.filter(r => !r.ok);
  console.log(`\nTOTAL: ${results.length} steps, ${fails.length} FAIL`);
  return finish(fails.length ? 1 : 0);
}

main().catch(async (e) => { console.error('UNCAUGHT', e); await finish(1); });
