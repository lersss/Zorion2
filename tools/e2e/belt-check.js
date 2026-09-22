// tools/e2e/belt-check.js
// QA-смоук этапа 2 поясов малых тел (спека
// 2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт §7.6).
// Сценарий: открыть модалку своей системы -> единая секция «Объекты» (пояса
// объединены с планетами 2026-09-22) видна (имя/радиус/масса) ->
// «🚀 Лететь» к поясу -> полоса полёта (модалка не закрылась) ->
// прибытие -> бейдж «вы в поясе»; состав появляется после скана/присутствия;
// для WD-мира значок «обломочный пояс» НЕ показывается (заменён секцией).
//
// Требует: поднятый dev-сервер (BASE_URL), QA_TOKEN игрока, мир с поясами.
// Переменные: BASE_URL, QA_TOKEN, QA_WORLD_ID, QA_WORLD_NAME, QA_WORLD_SPEC.
// ASCII-вывод (PowerShell cp866).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const TOKEN = process.env.QA_TOKEN || '';
const WORLD = {
  id: process.env.QA_WORLD_ID || '',
  name: process.env.QA_WORLD_NAME || 'QA-мир',
  spec: process.env.QA_WORLD_SPEC || 'G',
};

const results = [];
function report(stepName, status, detail) {
  results.push({ stepName, status, detail });
  console.log(`[${stepName}] ${status}${detail ? ' - ' + detail : ''}`);
}

const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  if (!TOKEN) { console.error('QA_TOKEN не задан'); return finish(2); }
  if (!WORLD.id) { console.error('QA_WORLD_ID не задан'); return finish(2); }
  const exe = findExecutable();
  if (!exe) { console.error('Chrome/Edge не найден'); return finish(2); }
  if (!existsSync(ARTIFACTS_DIR)) mkdirSync(ARTIFACTS_DIR, { recursive: true });

  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
  await page.addInitScript((t) => localStorage.setItem('token', t), TOKEN);

  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);

  // 1. Открыть модалку своей системы.
  await page.evaluate((w) => {
    window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true });
  }, WORLD);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(1500);

  // 2. Секция «Объекты» видна (пояса объединены с планетами, 2026-09-22).
  const beltsHeader = await page.evaluate(() => {
    const h = [...document.querySelectorAll('#right-panel h4')].find(e => e.textContent.startsWith('Объекты'));
    return h ? h.textContent : null;
  });
  if (!beltsHeader) { report('objects-section', 'FAIL', 'секция «Объекты» не найдена'); return finish(1); }
  report('objects-section', 'PASS', beltsHeader);

  // 3. Строка пояса: имя/радиус/масса.
  const beltRow = await page.evaluate(() => {
    const panel = document.getElementById('right-panel');
    const rows = [...panel.querySelectorAll('div')].filter(d => d.textContent.includes('радиус') && d.textContent.includes('масса'));
    return rows.length ? rows[0].textContent.replace(/\s+/g, ' ').trim() : null;
  });
  if (!beltRow) { report('belt-row', 'FAIL', 'строка пояса не найдена'); return finish(1); }
  report('belt-row', 'PASS', beltRow.slice(0, 120));

  // 4. Кнопка «🚀 Лететь» к поясу.
  const flyBtn = await page.$('[data-belt-fly]');
  if (!flyBtn) { report('belt-fly-btn', 'FAIL', 'кнопка полёта не найдена'); return finish(1); }
  report('belt-fly-btn', 'PASS', 'кнопка найдена');

  // 5. Старт полёта -> полоса полёта, модалка не закрылась.
  await flyBtn.click();
  await page.waitForTimeout(1200);
  const stripVisible = await page.evaluate(() => {
    const s = document.getElementById('intra-flight-strip');
    return !!s && s.style.display !== 'none';
  });
  const modalOpen = await page.evaluate(() => !!document.getElementById('system-modal-overlay'));
  if (!stripVisible || !modalOpen) {
    report('belt-flight-strip', 'FAIL', `strip=${stripVisible} modal=${modalOpen}`);
    return finish(1);
  }
  report('belt-flight-strip', 'PASS', 'полоса полёта видна, модалка открыта');

  // 6. Прибытие -> бейдж «вы в поясе» (ждём завершения полёта).
  await page.waitForFunction(() => {
    const panel = document.getElementById('right-panel');
    return panel && panel.textContent.includes('Вы в поясе');
  }, { timeout: 60000 }).catch(() => {});
  const badge = await page.evaluate(() => {
    const panel = document.getElementById('right-panel');
    return panel && panel.textContent.includes('Вы в поясе');
  });
  report('belt-arrival-badge', badge ? 'PASS' : 'FAIL', badge ? 'бейдж «Вы в поясе»' : 'бейдж не появился');

  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'belt-check.png') });
  const failed = results.some(r => r.status === 'FAIL');
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.error('belt-check error:', e); finish(1); });
