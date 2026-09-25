# QA-прогон: браузерный смоук студии после выноса блока контента — 2026-09-25

> Независимый живой прогон @tester. Задача: из монолита `web/studio.html` вынесен блок
> «экспорт/импорт снимка контента» в `web/static/js/studio/content_transfer.js`,
> подключённый тэгом `<script src=...>` **перед** инлайновым скриптом. Проверка —
> только живым браузером (playwright-core + системный Chrome), статики недостаточно.
>
> **Рабочее дерево, коммита нет.** Правки: `web/studio.html` (минус 136 строк, плюс
> `<script src>` и пустая строка), новый `web/static/js/studio/content_transfer.js`.
> `docs/QA_CHECKLIST.md` не трогаю (писатель — @manager).

**Дата:** 2026-09-25
**Сервер dev:** `http://127.0.0.1:8080`, `/health` = OK. Статика с диска (`noCache`),
рестарт не требовался. `GET /static/js/studio/content_transfer.js` → **200**,
`text/javascript; charset=utf-8` (8073 Б).
**Браузер:** системный Chrome через `playwright-core` (headless), viewport 1400×900.
**Временная учётка:** `qa_ct_*` (роль `skycomposer`) — создана регистрацией + `UPDATE users`.
**Импорт («Проверить»/«Применить») НЕ запускался; генерация не запускалась.** Экспорт
(клик по `#btnExportContent`) выполнен по чек-листу — серверный роут пишет снимок
`content/catalog*.json`, это не БД-мутация контента.

---

## Прогнал

```
cd tools/e2e; npm.cmd i (playwright-core уже стоял)
node tmp-qa-content-transfer.js      → все пункты PASS, exit 0
node qa-i1-studio.js                 → A8 (браузер + консоль) PASS, exit 1 из-за устаревшей
                                        предпосылки A6 (рецепт id=71 в БД отсутствует)
```

`goods-studio-server-check.js` **неприменим** и не гонялся: (1) требует
`STUDIO_USER/STUDIO_PASS` (admin/skycomposer) — пароля существующих учёток нет,
`SKYCOMPOSER_BOOTSTRAP_*` в `.env` не заданы; (2) его шаг 8 (fill) запускает ИИ-генерацию
в каталоге — задача это прямо запрещает. Взят `qa-i1-studio.js`: сам себе выдаёт admin-токен,
браузерный смоук `/studio` с `page.on('pageerror')` и печатью JS-ошибок, без генерации/импорта.

---

## Жёсткий чек-лист (5 пунктов)

| # | Вердикт | Факт (одной строкой) |
|---|---|---|
| **1** | **PASS** | `/studio` грузится: `pageErrors=0`, JS-`console.error=0`; единственный console-`error` — `404 /favicon.ico` (внешний шум, не JS), `loadingOverlay`=none, `studioLoginOverlay`≠flex. Все сетевые ответы 200 (в т.ч. `/static/js/studio/content_transfer.js`). |
| **2** | **PASS** | Обе кнопки в DOM и кликабельны: `#btnExportContent` (disabled=false), `#btnImportContent` (после экспорта disabled=false, title «Импорт контента (полная замена): сначала проверка (сухой про…)»). `typeof window.exportContent/importContent/refreshImportStatus === 'function'` — все три глобальны. |
| **3** | **PASS (с оговоркой)** | Выбранный смоук `qa-i1-studio.js`: A8 (браузер, `/studio`, `page.on('pageerror')`) — **PASS**, `errors=0 badResponses=[]`, скриншот `artifacts/qa-i1-studio.png`. Единственный FAIL скрипта — A6 «пара нет → 409», получено 404: рецепт id=71 в БД **отсутствует** (в `recipes` есть только 69 и 98) → сервер корректно отдаёт 404 «нет рецепта». Устаревшая предпосылка данных скрипта, к выносу блока отношения не имеет. |
| **4** | **PASS** | Клик «экспорт контента» даёт отчёт: `#report` = `Экспорт: сохранено 322 записей, файл catalog-20260925.json`; скачивание `catalog-20260925.json`; после экспорта `refreshImportStatus` (функция из вынесенного файла) оживил импорт — `#btnImportContent.disabled=false`. Функция реально вызвалась. |
| **5** | **PASS** | Граф/дерево и разделы живы: товары — фокус-режим без выбора показывает ожидаемый `focusEmptyOverlay` (канвас пуст законно); клик `#branchProdColony` → `branch=producers`, `colonyOn=true`, `goodsOn=false`, дерево построек нарисовано (ink=626); возврат на `#branchGoods` работает. |

**Итог:** 5/5 PASS. **СМОУК ПРОЙДЕН.**

**JS-ошибок/тостов-ошибок — нет.** Дословно единственное сообщение консоли:
`Failed to load resource: the server responded with a status of 404 (Not Found)`
(источник — `GET /favicon.ico`, проверено curl: `/favicon.ico → 404`, `/static/js/studio/content_transfer.js → 200`).

---

## Артефакты

- Скрипт прогона: `tools/e2e/tmp-qa-content-transfer.js` (временный, не коммичен).
- Скриншоты: `tools/e2e/artifacts/studio-content-transfer.png` (студия после клика экспорта),
  `tools/e2e/artifacts/qa-i1-studio.png` (смоук `qa-i1-studio.js`).
- Журнал: этот файл.

## Найденное (не чинил)

1. **Устаревшая предпосылка `tools/e2e/qa-i1-studio.js:21`** — `RECIPE_FREE = 71`
   («рецепт есть, но к 148 не привязан → 409»). В текущей БД рецепта 71 нет (есть 69, 98),
   поэтому `PUT /studio/api/producers/148/recipes/71` корректно отдаёт 404, а не 409,
   и скрипт выходит с `exit 1`. Правка тестовых данных/ожидания — не код игры. Для @manager.
2. **`/favicon.ico` → 404** во всех браузерных прогонах (шум в `console.error`).
   Не относится к правке; если мешает другим смоукам — завести отдельно.

## Чек-лист непроверяемого

- **Импорт «Проверить»/«Применить»** — не проверялся: задача запрещает применение
  (боевая/общая БД). Кнопка `#btnImportContent` и её title зафиксированы, но сама
  цепочка POST `/studio/api/content/import` живьём не гонялась. Закрыть — на изолированной
  БД (как в журнале `2026-09-25_студия-импорт-контента.md`).
- **`goods-studio-server-check.js` целиком** — неприменим (см. выше). Покрытие его зон
  (CRUD справочника, ИИ-fill, copy-universal) осталось непроверенным в этом прогоне.
- **Поведение при старом/битом кэше статики** — не воспроизводил (статика `noCache`).
- **Долгий тренд/UX глазами игрока** — вкусовое, вне тестера.
