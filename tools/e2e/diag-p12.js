// Проверка модалки СВОЕЙ системы A при активном межзвёздном полёте (п.12).
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

async function main() {
  const browser = await chromium.launch({ executablePath: exe, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, TOKEN);
  const page = await context.newPage();
  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForFunction(() => {
    const el = document.getElementById('loading');
    return !el || el.style.display === 'none';
  }, { timeout: 30000 });
  await page.waitForTimeout(1200);

  // Старт композитного полёта к B.
  await openModal(page, B);
  await page.evaluate(() => { document.querySelectorAll('tr[data-index]')[0].click(); });
  await page.waitForSelector('#composite-fly-btn', { timeout: 10000 });
  await page.waitForTimeout(600);
  await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
  await page.waitForTimeout(3000);

  // Модалка СВОЕЙ системы A.
  await openModal(page, A);
  await page.evaluate(() => { document.querySelectorAll('tr[data-index]')[0].click(); });
  await page.waitForTimeout(800);
  const btns = await page.evaluate(() => {
    const panel = document.getElementById('right-panel');
    const all = panel ? [...panel.querySelectorAll('button')].map(b => ({ id: b.id, text: b.textContent.trim(), title: b.title, disabled: b.disabled })) : [];
    return all;
  });
  console.log('P12 modal A buttons: ' + JSON.stringify(btns));
  const myPos = await page.evaluate(async () => {
    const token = localStorage.getItem('token');
    const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
    return { pos: me.current_position, world: me.current_world_name, flight: me.flight ? me.flight.to : null };
  });
  console.log('P12 me: ' + JSON.stringify(myPos));

  await browser.close();
}

main().catch(e => { console.log('FATAL: ' + String(e && e.message ? e.message : e)); process.exit(1); });