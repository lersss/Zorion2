# Прогон 2026-09-18 — e2e-смоук (tools/e2e, браузерный)

**Задача:** проверить новый инструмент `tools/e2e/map-check.js` (playwright-core + системный
Chrome/Edge) как конечный пользователь — браузерный смоук карты: регистрация/логин → карта без
JS-ошибок → клик по звезде без тост-ошибки/редиректа → restricted-карточка за-радарной звезды →
скриншоты.

**Сервер:** dev на 8080, `/health` → OK (до прогона).

## Вывод смоука

```
BASE_URL: http://localhost:8080
[1/5 register/login] PASS - user e2e_1789687254930_0
browser: Chrome (C:\Program Files\Google\Chrome\Application\chrome.exe)
[2/5 map loads] PASS - path=/map filter=200 canvasPx=5143 pageErrors=0
[3/5 click star] PASS - star=Glaan cnt=1 modal=true toastError=false path=/map
[4/5 restricted card] PASS - star=Ucpelal dist=6988 overlay=true text=true toastError=false path=/map
[5/5 screenshots] PASS - map.png=true restricted-card.png=true (C:\Zorion2\tools\e2e\artifacts\map.png) (C:\Zorion2\tools\e2e\artifacts\restricted-card.png)

RESULT: ALL PASS
EXIT CODE: 0
```

`npm.cmd i` — up to date, 0 vulnerabilities (зависимости уже стояли).

## Скриншоты

- `tools/e2e/artifacts/map.png` (44 КБ, 18.09 6:20:56) и `restricted-card.png` (59 КБ, 6:20:59) —
  созданы прогоном, не пустые. **Визуально не подтверждены:** модель тестера не поддерживает
  чтение изображений. Содержимое подтверждено косвенно: шаг 4 PASS (overlay + текст «Система вне
  зоны видимости» в right-panel), шаг 2 PASS (canvasPx=5143 — на канвасе что-то нарисовано).

## Вердикт: инструмент готов к использованию

Прогон стабилен (ALL PASS, exit 0), закрывает «слепую зону» статической проверки: реальный
браузер, ловит JS-ошибки (pageerror), тост-ошибки, редиректы, разлогин. Честные SKIP (нет
за-радарной звезды — SKIP, не ложный PASS). ASCII-вывод не ломается в cp866. Браузеры не
скачиваются (системный Chrome найден).

## Предложения (не обязательные, через @developer)

1. **Шаг 4 вызывает `window.openSystemModal(sid, name, spec)` БЕЗ starInfo** (`map-check.js:280`) —
   проверяется только 403-ветка, но НЕ передача starInfo из `map/events.js` (реальный клик).
   Регрессия «starInfo не доходит из кластера» смоуком не ловится (заголовок будет «Ucpelal (—, —)»).
   Предложение: передавать starInfo 6-м аргументом как events.js (`{stype, stemp, systype, smods, x, y}`)
   и/или добавить проверку заголовка с координатами.
2. **SKIP-порог шага 4 — `RADAR_RADIUS_MIN = 200`** (`map-check.js:31`), а у игрока радар 800
   (radar_1). Если самая дальняя звезда в радиусе 5000 окажется на dist ∈ (200, 800) — тест пойдёт
   в ветку FAIL (модалка откроется с планетами, не restricted). На текущих данных не сработало
   (dist=6988), но порог лучше брать из `/me` `radar_radius`.
3. **Проверка содержимого restricted-карточки** — сейчас только текст заглушки в right-panel.
   Можно добавить: заголовок содержит имя звезды, нет вкладок планет, minimap нарисован.
4. **Реальный клик по за-радарной звезде** (как шаг 3, но по дальней) вместо программного вызова —
   ближе к пользовательскому сценарию (клик → events.js → openSystemModal со starInfo).

## Не проверено

- Визуальное содержимое скриншотов (модель не читает изображения) — создателю/человеку глянуть
  `artifacts/map.png` и `restricted-card.png`.
- Edge-fallback (Chrome найден первым) — не проверялся.