// tools/e2e/qa99a3-headbar-check.js — проверка структуры шапки (99a.3-ui §13).
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

// 1. статус в строке заголовка (h1 + modelInfo/genIndicator в .head-title)
const title = await p.evaluate(() => {
  const row = document.querySelector('.head-title');
  const h1 = row.querySelector('h1');
  const status = row.querySelector('.head-status');
  return {
    hasH1: !!h1 && h1.textContent.includes('Студия товаров'),
    hasModel: status.contains(document.getElementById('modelInfo')),
    hasGen: status.contains(document.getElementById('genIndicator')),
  };
});
report('1 status in title row', title.hasH1 && title.hasModel && title.hasGen, JSON.stringify(title));

// 2. сегменты: Режим графа / Действия (без подписи); «Категории» — кнопка #btnCatOpen; «Справочник» в шапке отсутствует
const segs = await p.evaluate(() => {
  const labels = [...document.querySelectorAll('.tool-seg .seg-label')].map(s => s.textContent.trim());
  const segActions = document.getElementById('segActions');
  return {
    labels: labels,
    hasGraph: !!document.getElementById('segGraph'),
    hasActions: !!segActions,
    actionsNoLabel: segActions && !segActions.querySelector('.seg-label'),
    noSegSprav: !document.getElementById('segSprav'),
    noSegSub: !document.querySelector('#segGraph .seg-sub'),
    noSegCat: !document.getElementById('segCat'),
    hasCatBtn: !!document.getElementById('btnCatOpen'),
  };
});
report('2 segments present', segs.hasGraph && segs.hasActions && segs.actionsNoLabel && segs.noSegSprav && segs.noSegSub && segs.noSegCat && segs.hasCatBtn && segs.labels.includes('Режим графа'), JSON.stringify(segs));

// 3. addbar удалён; «+ товар» только в справочнике
const noAddbar = await p.evaluate(() => {
  return !document.getElementById('newGoodName') && !document.getElementById('newGoodCat') && !document.getElementById('btnAddGood') && !document.getElementById('addbar');
});
const spravAdd = await p.evaluate(() => {
  const addrow = document.querySelector('#sprav .addrow');
  return addrow && addrow.contains(document.getElementById('spravNewName')) && addrow.contains(document.getElementById('btnSpravAdd'));
});
report('3 addbar removed, + товар in sprav', noAddbar && spravAdd, `noAddbar=${noAddbar} spravAdd=${spravAdd}`);

// 4. фокус-режим: «Режим графа» схлопывается до «всё дерево» (остальное скрыто)
const focusSeg = await p.evaluate(() => {
  const seg = document.getElementById('segGraph');
  const visible = [...seg.querySelectorAll('input,select,button')].filter(el => el.style.display !== 'none' && el.offsetParent !== null);
  return { visibleIds: visible.map(el => el.id), count: visible.length };
});
report('4 graph seg collapses in focus', focusSeg.count === 1 && focusSeg.visibleIds[0] === 'fullTree', JSON.stringify(focusSeg));

// 5. «всё дерево»: сегмент разворачивается
await p.check('#fullTree');
await p.waitForTimeout(300);
const fullSeg = await p.evaluate(() => {
  const seg = document.getElementById('segGraph');
  const visible = [...seg.querySelectorAll('input,select,button')].filter(el => el.style.display !== 'none' && el.offsetParent !== null);
  return { count: visible.length, hasSearch: visible.some(el => el.id === 'search'), hasTop: visible.some(el => el.id === 'btnTop') };
});
report('5 graph seg expands in full tree', fullSeg.count > 3 && fullSeg.hasSearch && fullSeg.hasTop, JSON.stringify(fullSeg));
await p.uncheck('#fullTree');

// 6. попап категорий: кнопка открывает, список со счётчиками, создание, переименование, удаление
await p.click('#btnCatOpen');
await p.waitForTimeout(300);
const catPopup = await p.evaluate(() => ({
  open: document.getElementById('catPopup').style.display === 'flex',
  rows: document.querySelectorAll('#catList .cat-row').length,
  hasAdd: !!document.getElementById('catNewName') && !!document.getElementById('catAddBtn'),
  counts: [...document.querySelectorAll('#catList .ccount')].map(c => c.textContent.trim()),
}));
report('6a cat popup opens with list', catPopup.open && catPopup.rows > 0 && catPopup.hasAdd, JSON.stringify(catPopup));
// создание категории (Enter)
const newCatName = 'QA_Кат_' + Date.now();
await p.fill('#catNewName', newCatName);
await p.press('#catNewName', 'Enter');
await p.waitForTimeout(600);
const created = await p.evaluate((nm) => {
  const rows = [...document.querySelectorAll('#catList .cat-row')];
  return rows.some(r => r.querySelector('.cname').textContent === nm);
}, newCatName);
report('6b create category', created, `created=${created}`);
// переименование (✏ → input → Enter)
await p.evaluate((nm) => {
  const row = [...document.querySelectorAll('#catList .cat-row')].find(r => r.querySelector('.cname').textContent === nm);
  if (row) row.querySelector('button[title="переименовать"]').click();
}, newCatName);
await p.waitForTimeout(200);
await p.fill('#catRenameInput', newCatName + '_ren');
await p.press('#catRenameInput', 'Enter');
await p.waitForTimeout(600);
const renamed = await p.evaluate((nm) => {
  const rows = [...document.querySelectorAll('#catList .cat-row')];
  return rows.some(r => r.querySelector('.cname').textContent === nm);
}, newCatName + '_ren');
report('6c rename category', renamed, `renamed=${renamed}`);
// удаление пустой (модалка подтверждения)
await p.evaluate((nm) => {
  const row = [...document.querySelectorAll('#catList .cat-row')].find(r => r.querySelector('.cname').textContent === nm);
  if (row) row.querySelector('.cat-del').click();
}, newCatName + '_ren');
await p.waitForTimeout(200);
const delModal = await p.evaluate(() => document.getElementById('modalOverlay').style.display === 'flex');
await p.click('#modalBtns button:has-text("Удалить")');
await p.waitForTimeout(600);
const deleted = await p.evaluate((nm) => {
  const rows = [...document.querySelectorAll('#catList .cat-row')];
  return !rows.some(r => r.querySelector('.cname').textContent === nm);
}, newCatName + '_ren');
report('6d delete empty category', delModal && deleted, `modal=${delModal} deleted=${deleted}`);
// Esc закрывает попап
await p.keyboard.press('Escape');
await p.waitForTimeout(200);
const catClosed = await p.evaluate(() => document.getElementById('catPopup').style.display === 'none');
report('6e Esc closes cat popup', catClosed, `closed=${catClosed}`);

// 7. нет JS-ошибок
report('7 no page JS errors', errs.length === 0, errs.length ? errs.join(' | ').slice(0, 300) : 'clean');

await b.close();
const fails = results.filter(r => r === 'FAIL').length;
console.log(`\nTOTAL: ${results.length} steps, ${fails} FAIL`);
process.exit(fails ? 1 : 0);