// tools/e2e/qa99a3-smoke.js — быстрый смоук 99a.3 (фокус-подграф, справочник, попап).
// Run: node qa99a3-smoke.js  (требует запущенную студию на 8799)
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = process.env.BASE_URL || 'http://127.0.0.1:8799';
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const exe = [...CHROME_PATHS, ...EDGE_PATHS].find(p => existsSync(p));
if (!exe) { console.log('no browser'); process.exit(1); }

const results = [];
const report = (n, ok, d) => { results.push(ok ? 'PASS' : 'FAIL'); console.log(`[${n}] ${ok ? 'PASS' : 'FAIL'}${d ? ' - ' + d : ''}`); };

const browser = await chromium.launch({ executablePath: exe, headless: true });
const page = await browser.newPage({ viewport: { width: 1400, height: 900 } });
// чистый старт: очистить localStorage один раз (при reload не трогать — проверка сохранения фильтров)
await page.addInitScript(() => {
  if (!sessionStorage.getItem('qa_ls_cleared')) { localStorage.clear(); sessionStorage.setItem('qa_ls_cleared', '1'); }
});
const pageErrors = [];
page.on('pageerror', e => pageErrors.push(String(e && e.stack ? e.stack : e)));
const waitFor = (fn, t = 8000, d = 'cond') => page.waitForFunction(fn, null, { timeout: t }).catch(() => { throw new Error('timeout: ' + d); });

try {
  await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 15000, 'load');
  await page.waitForTimeout(500);

  // 1. фокус-режим по умолчанию: пустое состояние + бейдж
  const focusEmpty = await page.evaluate(() => document.getElementById('focusEmptyOverlay').style.display === 'flex');
  const badge = await page.evaluate(() => document.getElementById('modeBadge').textContent);
  report('1 focus default + empty overlay', focusEmpty && badge.includes('фокус'), `overlay=${focusEmpty} badge="${badge}"`);

  // 2. топбар: поиск/фильтр/ресурсы/бан/обратные/btnTop скрыты в фокусе
  const hidden = await page.evaluate(() => ({
    search: document.getElementById('search').style.display === 'none',
    cat: document.getElementById('catFilter').style.display === 'none',
    res: document.getElementById('showResources').parentElement.style.display === 'none',
    banned: document.getElementById('showBanned').parentElement.style.display === 'none',
    rev: document.getElementById('reverseEdges').parentElement.style.display === 'none',
    top: document.getElementById('btnTop').style.display === 'none',
  }));
  report('2 topbar hidden in focus', Object.values(hidden).every(Boolean), JSON.stringify(hidden));

  // 3. справочник: карточки есть, счётчик
  const sprav = await page.evaluate(() => ({
    title: document.getElementById('spravTitle').textContent,
    cards: document.querySelectorAll('#spravList .card').length,
  }));
  report('3 sprav list', sprav.cards > 0 && sprav.title.includes('· ' + sprav.cards), `title="${sprav.title}" cards=${sprav.cards}`);

  // 4. клик по карточке справочника = выбор + подграф (попап НЕ открывается)
  const firstId = await page.evaluate(() => {
    const card = document.querySelector('#spravList .card');
    return card ? card.getAttribute('ondblclick').match(/'([^']+)'/)[1] : null;
  });
  await page.click('#spravList .card >> nth=0');
  await page.waitForTimeout(400);
  const afterClick = await page.evaluate(() => ({
    selected: selected,
    popupOpen: document.getElementById('detailPopup').style.display === 'flex',
    focusEmptyGone: document.getElementById('focusEmptyOverlay').style.display === 'none',
    graphCards: (() => { const L = computeLayout(focusSubset(), selected); return L.goods.length; })(),
  }));
  report('4 sprav click selects + subgraph', afterClick.selected === firstId && !afterClick.popupOpen && afterClick.focusEmptyGone && afterClick.graphCards > 0, JSON.stringify(afterClick));

  // 5. ⓘ = попап без смены выбора
  const selBefore = await page.evaluate(() => selected);
  await page.locator('#spravList .card').nth(1).locator('.info').click();
  await page.waitForTimeout(300);
  const popupState = await page.evaluate(() => ({
    open: document.getElementById('detailPopup').style.display === 'flex',
    popupGoodId: popupGoodId,
    selected: selected,
    hasTierInput: !!document.getElementById('tierInput'),
    hasSlots: document.querySelectorAll('#popupBody .slot').length,
  }));
  report('5 info opens popup w/o selection change', popupState.open && popupState.popupGoodId !== selBefore && popupState.selected === selBefore, JSON.stringify(popupState));

  // 6. Esc закрывает попап, выбор сохраняется
  await page.keyboard.press('Escape');
  await page.waitForTimeout(200);
  const afterEsc = await page.evaluate(() => ({
    popupClosed: document.getElementById('detailPopup').style.display === 'none',
    selected: selected,
  }));
  report('6 Esc closes popup, selection kept', afterEsc.popupClosed && afterEsc.selected === selBefore, JSON.stringify(afterEsc));

  // 7. «всё дерево»: элементы возвращаются, бейдж меняется
  await page.check('#fullTree');
  await page.waitForTimeout(300);
  const fullTree = await page.evaluate(() => ({
    badge: document.getElementById('modeBadge').textContent,
    search: document.getElementById('search').style.display !== 'none',
    res: document.getElementById('showResources').parentElement.style.display !== 'none',
    top: document.getElementById('btnTop').style.display !== 'none',
    graphCards: (() => { const L = computeLayout(); return L.goods.length; })(),
  }));
  report('7 full tree mode', fullTree.badge.includes('всё дерево') && fullTree.search && fullTree.res && fullTree.top && fullTree.graphCards > 0, JSON.stringify(fullTree));
  await page.uncheck('#fullTree');

  // 8. пилюли тиров (99a.3-ui §5.3): пусто = весь тир 0 скрыт (ресурсы И тир-0 товары);
//    {0} = ресурсы + тир-0 товары; «все» = все тиры
  const expectedT1 = await page.evaluate(() => state.goods.filter(g => g.tier >= 1).length);
  const pills0 = await page.evaluate(() => ({
    count: document.querySelectorAll('#spravList .card').length,
    resBadges: document.querySelectorAll('#spravList .b-resource').length,
    pills: [...document.querySelectorAll('#tierFilters .pill')].map(p => p.textContent.trim()),
    noT0: !document.getElementById('fT0'),
  }));
  report('8a tier pills, empty hides tier 0', pills0.count === expectedT1 && pills0.resBadges === 0 && pills0.pills.length > 0 && pills0.noT0, JSON.stringify(pills0));
  await page.evaluate(() => {
    const pill = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === '0');
    if (pill) pill.querySelector('input').click();
  });
  await page.waitForTimeout(200);
  const pills0on = await page.evaluate(() => ({
    count: document.querySelectorAll('#spravList .card').length,
    resBadges: document.querySelectorAll('#spravList .b-resource').length,
    pillOn: document.querySelector('#tierFilters .pill.on') ? document.querySelector('#tierFilters .pill.on').textContent.trim() : null,
  }));
  report('8b tier 0 pill filters', pills0on.resBadges === 131 && pills0on.count >= 131 && pills0on.pillOn === '0', JSON.stringify(pills0on));
  await page.evaluate(() => {
    const pill = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === '0');
    if (pill) pill.querySelector('input').click();
  });
  await page.waitForTimeout(200);
  const pills0off = await page.evaluate(() => ({
    count: document.querySelectorAll('#spravList .card').length,
    resBadges: document.querySelectorAll('#spravList .b-resource').length,
    pillOn: document.querySelector('#tierFilters .pill.on') ? document.querySelector('#tierFilters .pill.on').textContent.trim() : null,
  }));
  report('8c tier 0 pill toggles off', pills0off.count === expectedT1 && pills0off.resBadges === 0 && pills0off.pillOn === null, JSON.stringify(pills0off));

  // 8d-8f. пилюля «все»: пусто (263) → все (394) → сброс (263) → все (394)
  await page.evaluate(() => {
    const all = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === 'все');
    if (all) all.querySelector('input').click();
  });
  await page.waitForTimeout(200);
  const allReset = await page.evaluate(() => ({
    count: document.querySelectorAll('#spravList .card').length,
    allOn: document.querySelector('#tierFilters .pill.on') ? document.querySelector('#tierFilters .pill.on').textContent.trim() : null,
  }));
  report('8d all pill selects all tiers', allReset.count === 394 && allReset.allOn === 'все', JSON.stringify(allReset));
  await page.evaluate(() => {
    const all = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === 'все');
    if (all) all.querySelector('input').click();
  });
  await page.waitForTimeout(200);
  const allSelected = await page.evaluate(() => ({
    count: document.querySelectorAll('#spravList .card').length,
    onPills: [...document.querySelectorAll('#tierFilters .pill.on')].map(p => p.textContent.trim()),
  }));
  report('8e all pill resets to empty', allSelected.count === expectedT1 && allSelected.onPills.length === 0, JSON.stringify(allSelected));
  await page.evaluate(() => {
    const all = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === 'все');
    if (all) all.querySelector('input').click();
  });
  await page.waitForTimeout(200);
  const allReset2 = await page.evaluate(() => ({
    count: document.querySelectorAll('#spravList .card').length,
    onPills: [...document.querySelectorAll('#tierFilters .pill.on')].map(p => p.textContent.trim()),
  }));
  report('8f all pill selects all again', allReset2.count === 394 && allReset2.onPills.includes('все') && allReset2.onPills.length > 2, JSON.stringify(allReset2));

  // 8g. фильтры справочника сохраняются при перезагрузке (создатель 2026-09-19)
  await page.evaluate(() => {
    // сброс (после 8f все тиры выбраны) → затем выбрать {0,3}
    const all = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === 'все');
    if (all) all.querySelector('input').click();
    ['0', '3'].forEach(t => {
      const pill = [...document.querySelectorAll('#tierFilters .pill')].find(p => p.textContent.trim() === t);
      if (pill) pill.querySelector('input').click();
    });
    document.getElementById('fUnused').click();
    const s = document.getElementById('spravSearch');
    s.value = 'QA_zzz';
    s.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await page.waitForTimeout(300);
  await page.reload({ waitUntil: 'domcontentloaded' });
  await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 15000, 'reload');
  await page.waitForTimeout(500);
  const restored = await page.evaluate(() => ({
    tierOn: [...document.querySelectorAll('#tierFilters .pill.on')].map(p => p.textContent.trim()),
    fUnused: document.getElementById('fUnused').checked,
    search: document.getElementById('spravSearch').value,
    count: document.querySelectorAll('#spravList .card').length,
  }));
  report('8g filters persist after reload', restored.tierOn.includes('0') && restored.tierOn.includes('3') && restored.fUnused && restored.search === 'QA_zzz' && restored.count === 0, JSON.stringify(restored));
  // сброс фильтров (чистота для шага 9)
  await page.evaluate(() => { localStorage.clear(); location.reload(); });
  await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 15000, 'reload 2');
  await page.waitForTimeout(500);

  // 8h. фильтр по категории в справочнике: выбор фильтрует, пусто = все, сохраняется при reload
  const catId = await page.evaluate(() => state.categories[0].id);
  const allCount = await page.evaluate(() => document.querySelectorAll('#spravList .card').length);
  await page.selectOption('#spravCat', catId);
  await page.waitForTimeout(300);
  const catFiltered = await page.evaluate(() => ({
    count: document.querySelectorAll('#spravList .card').length,
    title: document.getElementById('spravTitle').textContent,
  }));
  const catExpected = await page.evaluate((cid) => state.goods.filter(g => g.tier >= 1 && g.category === cid).length, catId);
  report('8h cat filter narrows list', catFiltered.count === catExpected && catFiltered.count < allCount && catFiltered.title.includes('· ' + catFiltered.count), JSON.stringify(catFiltered));
  await page.reload({ waitUntil: 'domcontentloaded' });
  await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 15000, 'reload 3');
  await page.waitForTimeout(500);
  const catRestored = await page.evaluate(() => ({
    value: document.getElementById('spravCat').value,
    count: document.querySelectorAll('#spravList .card').length,
  }));
  report('8i cat filter persists after reload', catRestored.value === catId && catRestored.count === catExpected, JSON.stringify(catRestored));
  // сброс (чистота для шага 9)
  await page.evaluate(() => { localStorage.clear(); location.reload(); });
  await waitFor(() => document.getElementById('loadingOverlay').style.display === 'none', 15000, 'reload 4');
  await page.waitForTimeout(500);

  // 9. тир-поле в попапе: ввод → PUT → бейджи обновляются (на товаре, не ресурсе)
  const goodId = await page.evaluate(() => {
    const g = state.goods.find(x => x.kind === 'good');
    return g ? g.id : null;
  });
  if (!goodId) { report('9 tier test', false, 'no good in state'); }
  else {
    await page.evaluate((id) => { selected = id; renderAll(); }, goodId);
    await page.waitForTimeout(200);
    const tierBefore = await page.evaluate(() => {
      const g = state.goods.find(x => x.id === selected);
      return { tier: g.tier, computed: g.tier_computed };
    });
    await page.evaluate((id) => { openPopup(id); }, goodId);
    await page.waitForTimeout(300);
    const popupOpen2 = await page.evaluate(() => document.getElementById('detailPopup').style.display === 'flex');
    report('9a dblclick opens popup', popupOpen2, 'popup open');
    const tierInput = await page.evaluate(() => {
      const inp = document.getElementById('tierInput');
      return inp ? { exists: true, value: inp.value } : { exists: false };
    });
    report('9b tier input present', tierInput.exists, JSON.stringify(tierInput));
    await page.fill('#tierInput', '9');
    await page.dispatchEvent('#tierInput', 'change');
    await page.waitForTimeout(800);
    const tierAfter = await page.evaluate(() => {
      const g = state.goods.find(x => x.id === selected);
      return { tier: g.tier, override: g.tier_override, warn: !!document.querySelector('.tier-warn') };
    });
    report('9c tier override applied', tierAfter.tier === 9 && tierAfter.override === 9, JSON.stringify(tierAfter));
    // очистка → null
    await page.fill('#tierInput', '');
    await page.dispatchEvent('#tierInput', 'change');
    await page.waitForTimeout(800);
    const tierCleared = await page.evaluate(() => {
      const g = state.goods.find(x => x.id === selected);
      return { tier: g.tier, override: g.tier_override };
    });
    report('9d tier cleared -> computed', tierCleared.override === null && tierCleared.tier === tierBefore.tier, JSON.stringify(tierCleared));
    await page.keyboard.press('Escape');
  }

  // 10. нет JS-ошибок
  report('10 no page JS errors', pageErrors.length === 0, pageErrors.length ? pageErrors.join(' | ').slice(0, 300) : 'clean');
} catch (err) {
  report('UNCAUGHT', false, String(err && err.message ? err.message : err));
}

await browser.close();
const fails = results.filter(r => r === 'FAIL').length;
console.log(`\nTOTAL: ${results.length} steps, ${fails} FAIL`);
process.exit(fails ? 1 : 0);