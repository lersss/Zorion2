// Диагностика п.11-13: «Найти меня», disabled внутрисистемной, композитная активна.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const TOKEN = process.env.QA_TOKEN || '';

const CHROME_PATHS = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(existsSync);
const EDGE_PATHS = ['C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(existsSync);
const exe = CHROME_PATHS[0] || EDGE_PATHS[0];

const A = { id: '8db2beea-91d8-413e-853f-53b3dbe50427', name: 'Jorkar', spec: 'M', stemp: 3600, x: -529.02, y: -339.44 };
const B = { id: 'f13452d8-6e9e-49e3-86ae-b0ef0daac9a3', name: 'Zinelchal', spec: 'M', stemp: 3665, x: -472.02, y: -282.44 };

async function openModal(page, w) {
  await page.evaluate((x) => {
    window.openSystemModal(x.id, x.name, x.spec, null, null, { stype: 'star', stemp: x.stemp, systype: 'single', smods: {}, x: x.x, y: x.y, hasEngine: true });
  }, w);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForFunction(() => {
    const panel = document.getElementById('right-panel');
    return panel && panel.querySelectorAll('tr[data-index]').length > 0;
  }, { timeout: 15000 });
}

async function openPlanetCard(page, i) {
  await page.evaluate((idx) => { document.querySelectorAll('tr[data-index]')[idx].click(); }, i);
  await page.waitForSelector('#composite-fly-btn, #intra-fly-btn', { timeout: 10000 });
  await page.waitForTimeout(600);
}

async function main() {
  const browser = await chromium.launch({ executablePath: exe, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, TOKEN);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));

  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForFunction(() => {
    const el = document.getElementById('loading');
    return !el || el.style.display === 'none';
  }, { timeout: 30000 });
  await page.waitForTimeout(1200);

  // Старт композитного полёта к B (п.13: композитная активна при межзвёздном).
  await openModal(page, B);
  await openPlanetCard(page, 0);
  await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
  await page.waitForTimeout(3000);

  // П.12: модалка СВОЕЙ системы A → внутрисистемная кнопка disabled.
  await openModal(page, A);
  await openPlanetCard(page, 0);
  const ownBtn = await page.evaluate(() => {
    const b = document.getElementById('intra-fly-btn');
    return b ? { disabled: b.disabled, title: b.title, text: b.textContent.trim() } : null;
  });
  console.log('P12 own intra btn: ' + JSON.stringify(ownBtn));

  // П.13: модалка чужой B → композитная кнопка НЕ блокируется.
  await page.evaluate(() => { const b = document.getElementById('back-to-list-btn'); if (b) b.click(); });
  await page.waitForTimeout(400);
  await page.evaluate(() => { const b = document.getElementById('close-modal-btn'); if (b) b.click(); });
  await page.waitForTimeout(400);
  await openModal(page, B);
  await openPlanetCard(page, 0);
  const compBtn = await page.evaluate(() => {
    const b = document.getElementById('composite-fly-btn');
    return b ? { disabled: b.disabled, title: b.title } : null;
  });
  console.log('P13 composite btn mid-flight: ' + JSON.stringify(compBtn));

  // П.11: «Найти меня» (🎯) в модалке при межзвёздном → модалка закрылась, камера ведёт.
  const findMe = await page.evaluate(() => {
    const b = document.getElementById('modal-find-me-btn');
    if (!b) return { ok: false, reason: 'no #modal-find-me-btn' };
    b.click();
    return { ok: true };
  });
  console.log('P11 find-me click: ' + JSON.stringify(findMe));
  await page.waitForTimeout(1500);
  const fm = await page.evaluate(() => ({
    modalClosed: !document.getElementById('system-modal-overlay'),
    followShip: sessionStorage.getItem('followShip'),
    centerBtnActive: !!(document.getElementById('centerBtn') && document.getElementById('centerBtn').classList.contains('active')),
  }));
  console.log('P11 find-me result: ' + JSON.stringify(fm));

  // П.11б: быстрый клик 🎯 сразу после открытия модалки (до резолва /me).
  await openModal(page, B);
  const quick = await page.evaluate(() => {
    const b = document.getElementById('modal-find-me-btn');
    if (!b) return { ok: false, reason: 'no #modal-find-me-btn' };
    b.click();
    return { ok: true };
  });
  console.log('P11b quick click: ' + JSON.stringify(quick));
  await page.waitForTimeout(1200);
  const qm = await page.evaluate(() => ({
    modalClosed: !document.getElementById('system-modal-overlay'),
    followShip: sessionStorage.getItem('followShip'),
  }));
  console.log('P11b quick result: ' + JSON.stringify(qm));
  console.log('PAGEERRORS: ' + JSON.stringify(pageErrors));

  await browser.close();
}

main().catch(e => { console.log('FATAL: ' + String(e && e.message ? e.message : e)); process.exit(1); });