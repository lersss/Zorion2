# tools/e2e — браузерный смоук (playwright-core)

Живой браузерный смоук для `@tester`: карта, клик по звезде, restricted-карточка
за-радарной системы. Закрывает «слепую зону» статической проверки клиента —
регрессии вроде toast-ошибки при клике по звезде (77a) или разлогина ловились
раньше только создателем вручную.

Браузеры **не скачиваются**: используется системный Chrome (fallback — Edge)
через `playwright-core` с `executablePath`.

## Запуск

```powershell
cd tools/e2e
npm.cmd i          # один раз: ставит playwright-core (devDependency)
node map-check.js
```

Переменные окружения:

- `BASE_URL` — адрес сервера (default `http://localhost:8080`).
- `CHROME_PATH` / `EDGE_PATH` — переопределить путь к браузеру (если он
  установлен не в стандартное место).

Требования: Node 18+ (используется глобальный `fetch`), запущенный dev-сервер
(`/health` → OK), системный Chrome или Edge.

## Что проверяет `map-check.js`

1. **Регистрация/логин** — уникальный юзер `e2e_<timestamp>` → `POST /register`
   → JWT.
2. **Карта `/map`** — канвас появился, кластеры загружены (`/api/worlds/filter`
   ответил 200), на канвасе что-то нарисовано, нет JS-ошибок
   (`page.on('pageerror')`), нет редиректа на `/login-page`.
3. **Клик по звезде** у центра канваса — нет `.toast-error` (регрессия 77a
   «Cannot read properties of undefined (reading 'star_type')»), токен жив,
   нет редиректа. Открылась ли модалка — информационно.
4. **Restricted-карточка** за-радарной звезды — программный
   `window.openSystemModal(sid, name, spec, null, null, starInfo)` (starInfo из
   кластера фильтра — как путь клика в `map/events.js`) → оверлей
   `#system-modal-overlay` с текстом «Система вне зоны видимости» и координатами
   в заголовке (подтверждает, что starInfo дошёл), без `.toast-error` и
   редиректа. Порог «за радаром» — `radar_radius` из `/me` (стартовая
   комплектация radar_1 → 800); если в данных нет звезды дальше радара —
   честный SKIP.
5. **Скриншоты** — `tools/e2e/artifacts/map.png`, `restricted-card.png`.

Exit code: `0` — все шаги PASS (SKIP допустим), `1` — есть FAIL.

## Как добавить шаг

В `map-check.js` после шага 4 добавьте блок вида:

```js
// --- Step N: что проверяем ---
const result = await page.evaluate(() => { /* проверка в браузере */ });
const ok = /* условие */;
report('N/5 название', ok ? 'PASS' : 'FAIL', 'детали для диагностики');
if (!ok) return finish(1);
```

- Проверки в браузере — через `page.evaluate` (доступ к DOM, `localStorage`,
  `fetch` с токеном из `localStorage.token`).
- Вывод — `report()` (латиницей/ASCII: PowerShell cp866 ломает кириллицу).
- `results` собирает итог для exit code; `finish(1)` прерывает прогон с FAIL.