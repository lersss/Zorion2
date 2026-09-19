// tools/e2e/qa99a3-ctx-sprav.js — контекстное меню карточек справочника (ПКМ).
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

// helper: ПКМ по карточке с именем (список длинный — прокрутить к карточке)
const ctxCard = async (name) => {
  const pos = await p.evaluate((nm) => {
    const card = [...document.querySelectorAll('#spravList .card')].find(c => c.querySelector('.nm').textContent.includes(nm));
    if (!card) return null;
    card.scrollIntoView({ block: 'center' });
    const r = card.getBoundingClientRect();
    return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
  }, name);
  if (!pos) return null;
  await p.mouse.click(pos.x, pos.y, { button: 'right' });
  await p.waitForTimeout(300);
  return pos;
};

// 1. ПКМ по ресурсу → меню «Забанить»/«удалить» (ресурсы скрыты по умолчанию — включить пилюлю «0»)
await p.evaluate(() => {
  const pill = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === '0');
  if (pill) pill.querySelector('input').click();
});
await p.waitForTimeout(300);
const resName = await p.evaluate(() => {
  const g = state.goods.find(x => x.kind === 'resource' && x.status !== 'banned');
  return g ? g.name : null;
});
if (!resName) { report('1 ctx on resource', false, 'no unbanned resource'); }
else {
  await ctxCard(resName);
  const items = await p.evaluate(() => [...document.querySelectorAll('#ctxMenu button')].map(b => b.textContent));
  report('1 ctx on resource', items.includes('Забанить') && items.includes('удалить'), JSON.stringify(items));
  await p.keyboard.press('Escape');
  await p.waitForTimeout(200);
}
// вернуть пилюлю (чистота)
await p.evaluate(() => {
  const pill = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === '0');
  if (pill) pill.querySelector('input').click();
});
await p.waitForTimeout(200);

// 2. ПКМ по товару → меню; «Забанить» → статус banned, карточка приглушена
const goodName = await p.evaluate(() => {
  const g = state.goods.find(x => x.kind === 'good' && x.status !== 'banned');
  return g ? g.name : null;
});
if (!goodName) { report('2 ban good', false, 'no unbanned good'); }
else {
  await ctxCard(goodName);
  const items2 = await p.evaluate(() => [...document.querySelectorAll('#ctxMenu button')].map(b => b.textContent));
  report('2a ctx on good', items2.includes('Забанить') && items2.includes('удалить'), JSON.stringify(items2));
  await p.click('#ctxMenu button:has-text("Забанить")');
  await p.waitForTimeout(800);
  const banned = await p.evaluate((nm) => {
    const g = state.goods.find(x => x.name === nm);
    return g ? { status: g.status, cardDimmed: !!document.querySelector('#spravList .card.banned') } : null;
  }, goodName);
  report('2b ban good', banned && banned.status === 'banned', JSON.stringify(banned));
  // для забаненного — «Вернуть» (забаненные скрыты — включить галку)
  await p.check('#fBanned');
  await p.waitForTimeout(300);
  await ctxCard(goodName);
  const items3 = await p.evaluate(() => [...document.querySelectorAll('#ctxMenu button')].map(b => b.textContent));
  report('2c banned shows Вернуть', items3.includes('Вернуть') && !items3.includes('Забанить'), JSON.stringify(items3));
  await p.click('#ctxMenu button:has-text("Вернуть")');
  await p.waitForTimeout(800);
  await p.uncheck('#fBanned');
  await p.waitForTimeout(200);
  const unbanned = await p.evaluate((nm) => {
    const g = state.goods.find(x => x.name === nm);
    return g ? g.status : null;
  }, goodName);
  report('2d unban works', unbanned === 'draft', `status=${unbanned}`);
}
// далее работаем с тир-0 сущностями (новый товар-черновик, ресурсы) — включить пилюлю «0»
await p.evaluate(() => {
  const pill = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === '0');
  if (pill) pill.querySelector('input').click();
});
await p.waitForTimeout(300);

// 3. «Удалить» товара — модалка → удалён
const delName = 'QA_Ctx_Del_' + Date.now();
await p.evaluate(async (nm) => {
  const r = await fetch('/api/goods', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: nm, category_id: state.categories[0].id }) });
  return r.json();
}, delName);
await p.waitForFunction((nm) => state.goods.some(g => g.name === nm), delName, { timeout: 10000 });
await p.waitForTimeout(500); // renderSprav пересобрал список
await ctxCard(delName);
await p.click('#ctxMenu button:has-text("удалить")');
await p.waitForTimeout(300);
const delModal = await p.evaluate(() => document.getElementById('modalOverlay').style.display === 'flex');
await p.click('#modalBtns button:has-text("Удалить")');
await p.waitForTimeout(800);
const delGone = await p.evaluate((nm) => !state.goods.some(g => g.name === nm), delName);
report('3 delete good via ctx', delModal && delGone, `modal=${delModal} gone=${delGone}`);

// 4. «Удалить» ресурса → тост (403 read-only от сервера)
const resName2 = await p.evaluate(() => {
  const g = state.goods.find(x => x.kind === 'resource' && x.status !== 'banned');
  return g ? g.name : null;
});
if (!resName2) { report('4 delete resource', false, 'no unbanned resource'); }
else {
  await ctxCard(resName2);
  await p.click('#ctxMenu button:has-text("удалить")');
  await p.waitForTimeout(300);
  await p.click('#modalBtns button:has-text("Удалить")');
  await p.waitForTimeout(800);
  const toast = await p.evaluate(() => document.getElementById('report').textContent);
  const resStill = await p.evaluate((nm) => state.goods.some(g => g.name === nm), resName2);
  report('4 delete resource -> toast', resStill && toast.includes('read-only'), `still=${resStill} toast="${toast.slice(0, 80)}"`);
}

// 5. клик вне / Esc закрывают меню
await ctxCard(resName2);
const menuShown = await p.evaluate(() => document.getElementById('ctxMenu').style.display === 'block');
await p.mouse.click(10, 400);
await p.waitForTimeout(200);
const menuClosed = await p.evaluate(() => document.getElementById('ctxMenu').style.display === 'none');
report('5 click outside closes', menuShown && menuClosed, `shown=${menuShown} closed=${menuClosed}`);

// 6. drag&drop и клик не сломаны (карточка draggable, клик = выбор)
const dragOk = await p.evaluate(() => {
  const card = document.querySelector('#spravList .card');
  return card && card.getAttribute('draggable') === 'true' && card.getAttribute('onclick').includes('spravClick');
});
report('6 drag/click intact', dragOk, `dragOk=${dragOk}`);

// 7. нет JS-ошибок
report('7 no page JS errors', errs.length === 0, errs.length ? errs.join(' | ').slice(0, 300) : 'clean');

await b.close();
const fails = results.filter(r => r === 'FAIL').length;
console.log(`\nTOTAL: ${results.length} steps, ${fails} FAIL`);
process.exit(fails ? 1 : 0);