// tools/e2e/qa99a3-topbar-filters.js — проверка переноса фильтров в топбар.
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

// 1. поиск/галки справочника — в фиксированной шапке панели (.sprav-head), не в шапке страницы
const inPanelHead = await p.evaluate(() => {
  const head = document.querySelector('#sprav .sprav-head');
  return ['spravSearch', 'fUsed', 'fUnused', 'fBanned'].every(id => head.contains(document.getElementById(id)));
});
const notInHeadbar = await p.evaluate(() => {
  const hb = document.getElementById('headbar');
  return !hb.contains(document.getElementById('spravSearch')) && !document.getElementById('segSprav');
});
report('1 filters in panel head', inPanelHead && notInHeadbar, `inPanelHead=${inPanelHead} notInHeadbar=${notInHeadbar}`);

// 2. поиск по справочнику в топбаре фильтрует
  const expectedT1 = await p.evaluate(() => state.goods.filter(g => g.tier >= 1).length);
  await p.fill('#spravSearch', 'QA_zzz_nomatch');
  await p.waitForTimeout(200);
  const noMatch = await p.evaluate(() => document.querySelectorAll('#spravList .card').length);
  await p.fill('#spravSearch', '');
  await p.waitForTimeout(200);
  const allBack = await p.evaluate(() => document.querySelectorAll('#spravList .card').length);
  report('2 topbar search filters', noMatch === 0 && allBack === expectedT1, `noMatch=${noMatch} all=${allBack} expected=${expectedT1}`);

// 3. скроллится только список; шапка (заголовок/addrow/bulkToggle) фиксирована
const scrollOk = await p.evaluate(() => {
  const list = document.getElementById('spravList');
  list.scrollTop = 2000;
  const rr = document.getElementById('right').getBoundingClientRect();
  const cards = [...document.querySelectorAll('#spravList .card')];
  const visible = cards.some(c => { const cr = c.getBoundingClientRect(); return cr.top >= rr.top - 1 && cr.bottom <= rr.bottom + 1; });
  const head = document.querySelector('.sprav-head');
  const hr = head.getBoundingClientRect();
  const headVisible = hr.top >= rr.top - 1 && hr.bottom <= rr.bottom + 1;
  return { scrolled: list.scrollTop > 0, anyCardVisible: visible, headVisible: headVisible, listScrollable: list.scrollHeight > list.clientHeight };
});
report('3 list scrolls, head fixed', scrollOk.scrolled && scrollOk.anyCardVisible && scrollOk.headVisible && scrollOk.listScrollable, JSON.stringify(scrollOk));

// 4. «+ товар» в панели создаёт и выбирает
const catId = await p.evaluate(() => state.categories[0].id);
await p.fill('#spravNewName', 'QA_Topbar_Test');
await p.selectOption('#spravNewCat', catId);
await p.click('#btnSpravAdd');
await p.waitForTimeout(800);
const created = await p.evaluate(() => {
  const g = state.goods.find(x => x.name === 'QA_Topbar_Test');
  return g ? { id: g.id, selected: selected === g.id } : null;
});
report('4 + товар in panel', created && created.selected, JSON.stringify(created));

// 5. подгрузка списка (bulk) работает
await p.click('#bulkToggle');
await p.waitForTimeout(200);
await p.fill('#bulkText', 'QA_Bulk_Topbar | топливо\nQA_Bulk_Bad | nonexistent');
await p.click('#btnBulk');
await p.waitForTimeout(800);
const bulk = await p.evaluate(() => {
  const g = state.goods.find(x => x.name === 'QA_Bulk_Topbar');
  const textareaCleared = document.getElementById('bulkText').value === '';
  return { created: !!g, cleared: textareaCleared };
});
report('5 bulk from panel', bulk.created && bulk.cleared, JSON.stringify(bulk));

// 6. нет JS-ошибок
report('6 no page JS errors', errs.length === 0, errs.length ? errs.join(' | ').slice(0, 300) : 'clean');

// cleanup
const st = await p.evaluate(() => fetch('/api/state').then(r => r.json()));
for (const g of st.goods.filter(x => x.name.startsWith('QA_'))) {
  await p.evaluate((id) => fetch('/api/goods/' + id, { method: 'DELETE' }), g.id);
}
await b.close();
const fails = results.filter(r => r === 'FAIL').length;
console.log(`\nTOTAL: ${results.length} steps, ${fails} FAIL`);
process.exit(fails ? 1 : 0);