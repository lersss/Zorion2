// tools/e2e/qa-studio-popup-fit.js — независимый прогон @tester (2026-09-24):
// правка web/studio.html — высота попапа детали #detailPopup переведена с
// 92vh/88vh на calc(100% - 16px)/calc(100% - 12px), чтобы попап не выходил за окно.
// Реальный браузер (playwright-core + системный Chrome). Вход в студию — adminToken
// в localStorage (образец: qa-i1-studio.js).
// Run: node qa-studio-popup-fit.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const PASSWORD = 'qa-popfit-' + Date.now();
const GOOD_NAME = 'Пища';
const GOOD_TIER = 1;

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
    username = 'qa_popfit_' + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
    if (res.status === 201) { tok = (await res.json()).token; break; }
    if (res.status !== 409) throw new Error('register HTTP ' + res.status);
  }
  if (!tok) throw new Error('register failed');
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + tok } })).json();
  psqlRun(`UPDATE users SET role='admin' WHERE id='${me.id}';\n`, 'qa-popfit-role.sql');
  const lr = await fetch(BASE_URL + '/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
  if (lr.ok) tok = (await lr.json()).token;
  return { token: tok, username, uid: me.id };
}

let browser = null;
const consoleProblems = []; // общий сбор по всем контекстам (item 7)
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

async function openStudio(vp, tag) {
  const context = await browser.newContext({ viewport: { width: vp.w, height: vp.h } });
  await context.addInitScript((t) => { try { localStorage.setItem('adminToken', t); } catch (e) {} }, token);
  const page = await context.newPage();
  attachDiagnostics(page, tag);
  await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForFunction(() => { const o = document.getElementById('loadingOverlay'); return o && o.style.display === 'none'; }, null, { timeout: 15000 });
  await page.waitForFunction(() => typeof state !== 'undefined' && Array.isArray(state.goods) && state.goods.length > 0, null, { timeout: 10000 });
  return { context, page };
}

// Открыть попап товара «Пища» реальным двойным кликом по карточке справочника.
// Список пуст без «тир + галка» — включаем фильтр через штатные хендлеры UI,
// затем dblclick по настоящей карточке (ondblclick=spravDblClick).
async function openPishchaCard(page) {
  const found = await page.evaluate((tier) => {
    const g = (state.goods || []).find(x => x.name === 'Пища');
    document.getElementById('fUsed').checked = true;
    document.getElementById('fUnused').checked = true;
    toggleTier(tier, true);
    return g ? { id: g.id, tier: g.tier } : null;
  }, GOOD_TIER);
  if (!found) return { ok: false, reason: 'товар «Пища» не найден в state.goods' };
  const handle = await page.evaluateHandle((name) => {
    const cards = [...document.querySelectorAll('#spravList .card')];
    return cards.find(c => {
      const nm = c.querySelector('.nm');
      if (!nm) return false;
      return nm.textContent.replace(/[ⓘ?×\s]/g, '') === name;
    }) || null;
  }, GOOD_NAME);
  const el = handle.asElement();
  if (!el) return { ok: false, reason: 'карточка «Пища» не отрисована в #spravList' };
  await el.dblclick();
  await page.waitForFunction(() => {
    const e = document.getElementById('detailPopup');
    return e && e.style.display !== 'none' && document.querySelector('#popupBody .pblock');
  }, null, { timeout: 10000 });
  await page.waitForTimeout(400);
  return { ok: true, id: found.id };
}

function measurePopup(page) {
  return page.evaluate(() => {
    const p = document.getElementById('detailPopup');
    const r = p.getBoundingClientRect();
    const head = document.getElementById('popupHead').getBoundingClientRect();
    const close = document.getElementById('popupClose').getBoundingClientRect();
    const body = document.getElementById('popupBody');
    const main = document.getElementById('main').getBoundingClientRect();
    return {
      top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), height: Math.round(r.height),
      innerW: window.innerWidth, innerH: window.innerHeight,
      mainTop: Math.round(main.top), mainBottom: Math.round(main.bottom),
      title: document.getElementById('popupTitle').textContent,
      headTop: Math.round(head.top), headBottom: Math.round(head.bottom), headH: Math.round(head.height),
      closeW: Math.round(close.width), closeH: Math.round(close.height), closeBottom: Math.round(close.bottom),
      bodyClientH: body.clientHeight, bodyScrollH: body.scrollHeight,
      hasDel: !!document.querySelector('#popupBody button.del'),
    };
  });
}

async function item456(page, vp, tag) {
  // --- item 4: шапка/скролл/кнопки ---
  const m = await measurePopup(page);
  const headOk = m.headTop >= 0 && m.headBottom <= m.innerH && m.headTop >= m.top - 1 && m.headBottom <= m.bottom + 1 && m.closeW > 0 && m.closeH > 0;
  const titleOk = m.title === GOOD_NAME;
  const scrolls = m.bodyScrollH > m.bodyClientH;
  const scrolled = await page.evaluate(() => {
    const body = document.getElementById('popupBody');
    body.scrollTop = body.scrollHeight;
    const del = document.querySelector('#popupBody button.del');
    if (!del) return { hasDel: false };
    const r = del.getBoundingClientRect();
    const br = body.getBoundingClientRect();
    return { hasDel: true, delTop: Math.round(r.top), delBottom: Math.round(r.bottom), bodyTop: Math.round(br.top), bodyBottom: Math.round(br.bottom), innerH: window.innerHeight };
  });
  const delReachable = scrolled.hasDel && scrolled.delBottom <= scrolled.innerH + 1 && scrolled.delBottom <= scrolled.bodyBottom + 1 && scrolled.delTop >= scrolled.bodyTop - 1;
  report(`4 шапка+скролл+кнопки [${tag}]`,
    headOk && titleOk && scrolls && delReachable,
    `title="${m.title}" headTop=${m.headTop} headBottom=${m.headBottom} close=${m.closeW}x${m.closeH} innerH=${m.innerH}; bodyScrollH=${m.bodyScrollH} > bodyClientH=${m.bodyClientH} => scrolls=${scrolls}; delBottom=${scrolled.delBottom} bodyBottom=${scrolled.bodyBottom} innerH=${scrolled.innerH} => delReachable=${delReachable}`);

  // --- item 5: перетаскивание за шапку ---
  const before = await page.evaluate(() => { const r = document.getElementById('detailPopup').getBoundingClientRect(); return { top: Math.round(r.top), left: Math.round(r.left) }; });
  const headBox = await page.evaluate(() => {
    const r = document.getElementById('popupHead').getBoundingClientRect();
    // точка в шапке, но левее крестика
    return { x: Math.round(r.left + 40), y: Math.round(r.top + r.height / 2) };
  });
  await page.mouse.move(headBox.x, headBox.y);
  await page.mouse.down();
  await page.mouse.move(headBox.x + 70, headBox.y + 45, { steps: 8 });
  await page.mouse.up();
  await page.waitForTimeout(150);
  const after = await page.evaluate(() => { const r = document.getElementById('detailPopup').getBoundingClientRect(); return { top: Math.round(r.top), left: Math.round(r.left) }; });
  const moved = Math.abs(after.left - before.left) >= 30 && Math.abs(after.top - before.top) >= 20;
  report(`5 перетаскивание за шапку [${tag}]`, moved,
    `before=(${before.left},${before.top}) after=(${after.left},${after.top}) d=(${after.left - before.left},${after.top - before.top})`);
  return { m };
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

  const vps = [
    { w: 1280, h: 800, item: '1', tag: '1280x800' },
    { w: 1400, h: 900, item: '2', tag: '1400x900' },
    { w: 1920, h: 1080, item: '3', tag: '1920x1080' },
  ];

  let firstPage = null, firstContext = null;
  for (const vp of vps) {
    const { context, page } = await openStudio(vp, vp.tag);
    const opened = await openPishchaCard(page);
    if (!opened.ok) {
      report(`${vp.item} попап в окне ${vp.tag}`, false, opened.reason);
      await context.close();
      continue;
    }
    const m = await measurePopup(page);
    const inWindow = m.top >= 0 && m.bottom <= m.innerH;
    const inMain = m.top >= m.mainTop - 1 && m.bottom <= m.mainBottom + 1;
    report(`${vp.item} попап целиком в окне ${vp.tag}`, inWindow && inMain,
      `top=${m.top} bottom=${m.bottom} height=${m.height} innerH=${m.innerH} (innerW=${m.innerW}); mainTop=${m.mainTop} mainBottom=${m.mainBottom} inMain=${inMain}`);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, `qa-studio-popup-${vp.tag}.png`) });
    if (vp.item === '1') { firstPage = page; firstContext = context; }
    else await context.close();
  }

  // items 4/5 — на самом тесном окне 1280x800 (там риск обрезки максимален)
  if (firstPage) {
    await item456(firstPage, { w: 1280, h: 800 }, '1280x800');
    await firstContext.close();
    firstPage = null; firstContext = null;
  } else {
    report('4 шапка+скролл+кнопки', false, 'нет открытой страницы 1280x800');
    report('5 перетаскивание за шапку', false, 'нет открытой страницы 1280x800');
  }

  // --- item 6: регресс — карточка постройки, «Категории», «Эффекты» ---
  // свежая страница 1280x800 (без перенесённой перетаскиванием позиции попапа)
  {
    const { context, page } = await openStudio({ w: 1280, h: 800 }, 'regress');
    await page.evaluate(() => { document.getElementById('branchProducers').click(); });
    await page.waitForFunction(() => typeof state !== 'undefined' && Array.isArray(state.producer_types) && state.producer_types.length > 0, null, { timeout: 10000 });
    await page.evaluate(() => openProdPopup(state.producer_types[0].id));
    await page.waitForFunction(() => { const e = document.getElementById('detailPopup'); return e && e.style.display !== 'none'; }, null, { timeout: 10000 });
    await page.waitForTimeout(300);
    const mp = await measurePopup(page);
    const prodOk = mp.top >= 0 && mp.bottom <= mp.innerH;
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popup-producer.png') });
    report('6a карточка постройки в окне', prodOk,
      `title="${mp.title}" top=${mp.top} bottom=${mp.bottom} innerH=${mp.innerH}`);

    await page.evaluate(() => { closePopup(); openCatPopup(); });
    await page.waitForFunction(() => { const e = document.getElementById('catPopup'); return e && e.style.display !== 'none'; }, null, { timeout: 5000 });
    await page.waitForTimeout(200);
    const mc = await page.evaluate(() => { const r = document.getElementById('catPopup').getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), innerH: window.innerHeight }; });
    report('6b попап «Категории» в окне', mc.top >= 0 && mc.bottom <= mc.innerH, `top=${mc.top} bottom=${mc.bottom} innerH=${mc.innerH}`);

    await page.evaluate(() => { closeCatPopup(); openEffectPopup(); });
    await page.waitForFunction(() => { const e = document.getElementById('effPopup'); return e && e.style.display !== 'none'; }, null, { timeout: 5000 });
    await page.waitForTimeout(200);
    const me = await page.evaluate(() => { const r = document.getElementById('effPopup').getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), innerH: window.innerHeight }; });
    report('6c попап «Эффекты» в окне', me.top >= 0 && me.bottom <= me.innerH, `top=${me.top} bottom=${me.bottom} innerH=${me.innerH}`);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-studio-popup-effects.png') });
    await context.close();
  }

  // --- item 7: консоль/сеть ---
  report('7 консоль без JS-ошибок и ответов >=400', consoleProblems.length === 0,
    'problems=' + consoleProblems.length + (consoleProblems.length ? ' :: ' + consoleProblems.slice(0, 3).join(' | ') : ''));

  // cleanup
  try { psqlRun(`DELETE FROM users WHERE id='${admin.uid}';\n`, 'qa-popfit-deluser.sql'); } catch (e) {}

  const fails = results.filter(r => !r.ok);
  console.log(`\nTOTAL: ${results.length} steps, ${fails.length} FAIL`);
  return finish(fails.length ? 1 : 0);
}

main().catch(async (e) => { console.error('UNCAUGHT', e); await finish(1); });
