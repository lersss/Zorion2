// tools/e2e/qa-belt-leave.js
// QA-проба (@tester): кнопка выхода из мини-игры подписана «Вернуться на карту»,
// клик ведёт на /map, без pageerror и без редиректа на логин. ASCII-вывод.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const TOKEN = process.env.QA_TOKEN || '';
const BELT_ID = process.env.QA_BELT_ID || '';
const CHROME = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].find(p => existsSync(p));

let browser = null;
async function finish(c) { if (browser) await browser.close().catch(() => {}); process.exit(c); }

async function main() {
  if (!TOKEN || !BELT_ID) { console.error('QA_TOKEN/QA_BELT_ID не заданы'); return finish(2); }
  browser = await chromium.launch({ executablePath: CHROME, headless: true });
  const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
  const errors = [];
  page.on('pageerror', e => { errors.push(e.message); console.log('PAGEERROR:', e.message); });
  await page.addInitScript((t) => localStorage.setItem('token', t), TOKEN);
  await page.goto(BASE_URL + '/belt.html?belt=' + BELT_ID, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(3500);

  const label = await page.evaluate(() => {
    const b = document.getElementById('belt-leave-btn');
    return b ? b.textContent.trim() : null;
  });
  console.log('[leave-label]', JSON.stringify(label));
  const labelOk = label === 'Вернуться на карту';
  console.log(`[leave-label] ${labelOk ? 'PASS' : 'FAIL'} - "${label}"`);

  const urlBefore = page.url();
  // Клик по кнопке выхода.
  await page.click('#belt-leave-btn').catch(e => console.log('click err', e.message));
  await page.waitForTimeout(3500);
  const urlAfter = page.url();
  console.log('[leave-nav]', urlBefore, '->', urlAfter);
  const path = new URL(urlAfter).pathname;
  const navOk = path === '/map';
  const loginRedirect = /login/.test(urlAfter);
  console.log(`[leave-nav] ${(navOk && !loginRedirect) ? 'PASS' : 'FAIL'} - path=${path} errors=${errors.length}`);
  return finish((labelOk && navOk && !loginRedirect && errors.length === 0) ? 0 : 1);
}
main().catch(e => { console.error('error:', e); finish(1); });
