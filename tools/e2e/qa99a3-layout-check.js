// tools/e2e/qa99a3-layout-check.js — проверка раскладки: колонка от верха, шапка не заходит, канвас слева.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';
const exe = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe', 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].find(p => existsSync(p));
const b = await chromium.launch({ executablePath: exe, headless: true });
const p = await b.newPage({ viewport: { width: 1400, height: 900 } });
await p.addInitScript(() => { if (!sessionStorage.getItem('qa_ls_cleared')) { localStorage.clear(); sessionStorage.setItem('qa_ls_cleared', '1'); } });
const errs = [];
p.on('pageerror', e => errs.push(String(e && e.stack ? e.stack : e)));
const results = [];
const report = (n, ok, d) => { results.push(ok ? 'PASS' : 'FAIL'); console.log(`[${n}] ${ok ? 'PASS' : 'FAIL'}${d ? ' - ' + d : ''}`); };

await p.goto('http://127.0.0.1:8799/', { waitUntil: 'domcontentloaded' });
await p.waitForFunction(() => document.getElementById('loadingOverlay').style.display === 'none', null, { timeout: 15000 });
await p.waitForTimeout(500);

// 1. правая колонка от верха страницы (top совпадает с верхом body/viewport)
const rightTop = await p.evaluate(() => {
  const r = document.getElementById('right').getBoundingClientRect();
  const b = document.body.getBoundingClientRect();
  return { rightTop: r.top, bodyTop: b.top, rightBottom: r.bottom, viewH: window.innerHeight };
});
report('1 right column from page top', Math.abs(rightTop.rightTop - rightTop.bodyTop) < 2 && rightTop.rightBottom >= rightTop.viewH - 2, JSON.stringify(rightTop));

// 2. шапка не заходит под колонку (правый край шапки <= левый край колонки)
const headbar = await p.evaluate(() => {
  const h = document.getElementById('headbar').getBoundingClientRect();
  const r = document.getElementById('right').getBoundingClientRect();
  return { headRight: h.right, rightLeft: r.left, headTop: h.top, headBottom: h.bottom };
});
report('2 headbar not under column', headbar.headRight <= headbar.rightLeft + 2, JSON.stringify(headbar));

// 3. канвас слева под шапкой, не заходит под колонку
const canvas = await p.evaluate(() => {
  const c = document.getElementById('canvasWrap').getBoundingClientRect();
  const r = document.getElementById('right').getBoundingClientRect();
  const h = document.getElementById('headbar').getBoundingClientRect();
  return { canvasRight: c.right, rightLeft: r.left, canvasTop: c.top, headBottom: h.bottom };
});
report('3 canvas left of column, below headbar', canvas.canvasRight <= canvas.rightLeft + 2 && canvas.canvasTop >= canvas.headBottom - 2, JSON.stringify(canvas));

// 4. поиск+галки в панели над списком, фильтруют в реальном времени
  const expectedT1 = await p.evaluate(() => state.goods.filter(g => g.tier >= 1).length);
  await p.fill('#spravSearch', 'QA_zzz_nomatch');
  await p.waitForTimeout(200);
  const noMatch = await p.evaluate(() => document.querySelectorAll('#spravList .card').length);
  await p.fill('#spravSearch', '');
  await p.waitForTimeout(200);
  const allBack = await p.evaluate(() => document.querySelectorAll('#spravList .card').length);
  report('4 search in panel filters live', noMatch === 0 && allBack === expectedT1, `noMatch=${noMatch} all=${allBack} expected=${expectedT1}`);

// 5. список скроллится, шапка панели прибита (поиск/галки видны при прокрутке)
const scrollHead = await p.evaluate(() => {
  const list = document.getElementById('spravList');
  list.scrollTop = list.scrollHeight;
  const rr = document.getElementById('right').getBoundingClientRect();
  const head = document.querySelector('.sprav-head');
  const hr = head.getBoundingClientRect();
  const search = document.getElementById('spravSearch').getBoundingClientRect();
  return { scrolled: list.scrollTop > 0, headVisible: hr.top >= rr.top - 1 && hr.bottom <= rr.bottom + 1, searchVisible: search.top >= rr.top - 1 && search.bottom <= rr.bottom + 1 };
});
report('5 list scrolls, panel head fixed', scrollHead.scrolled && scrollHead.headVisible && scrollHead.searchVisible, JSON.stringify(scrollHead));

// 6. нет JS-ошибок
report('6 no page JS errors', errs.length === 0, errs.length ? errs.join(' | ').slice(0, 300) : 'clean');

await b.close();
const fails = results.filter(r => r === 'FAIL').length;
console.log(`\nTOTAL: ${results.length} steps, ${fails} FAIL`);
process.exit(fails ? 1 : 0);