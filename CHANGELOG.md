# Changelog

Все значимые изменения проекта Zorion.
Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
проект следует [Semantic Versioning](https://semver.org/lang/ru/).

---

## [Unreleased]

### Сессия 2026-09-11

**Исправлено**
- **B6 `ADMIN_PASSWORD`** — обязательный из env, `log.Fatal` при пустом
  (`internal/config/config.go`). Раньше дефолт `admin123` открывал админку,
  включая кнопку «Очистка вселенной».
- **B2 `Register`** — разбор `unique_violation` (23505) → 409 вместо 500.
- **B4** — запись «`compatibility_matrix` не создана» оказалась ошибочной:
  таблица в БД есть, миграция 009 применена.
- `planets.forEach is not a function` на мирах без планет (`planets` → `[]`).
- Убран отладочный вывод `[PCM-DEBUG]`.

**Добавлено**
- Формы поверхности спутников: реголит, ледяная кора, криовулканы, разломы,
  гейзеры (~1,5% планетарных).
- `alert()` в админке заменён на тосты (`web/static/js/ui/toast.js`).
- Блок «корабли / полёты / UI»: `web/static/sprites/` (22 SVG + `ships.json`),
  `map/flight.js`, `loader.js`, `modal/layout.js`, `noise.go`,
  миграция `000011` (`users.ship_icon`).
- Команда opencode `/start` (`.opencode/command/start.md`) — старт сессии:
  сводка статуса и вопрос о задаче.

**Изменено**
- Перегенерация вселенной: 100 000 миров, 319 573 планеты.
- **Доки сжаты:** `AGENTS.md` 392 → 158, `STATUS.md` 211 → 64. Стартовое чтение
  сессии 2 700 → 222 строки. Удалены `PROMPT.md` (правила уехали в `AGENTS.md`)
  и `docs/gamedesign/README.md`. Убраны дубли по 8 темам.
- Исправлены неверные факты в доках: миграции 009 и 011 применены (проверено
  в БД напрямую), Go есть локально (1.27.0), карта отвечает не «< 100 мс».
- `origin` переключён на `github.com/lersss/Zorion2`.

**Найдено, не исправлено**
- **B7** — у `worlds` единственный индекс `worlds_pkey`; фильтр карты идёт
  `Seq Scan` по 100k строк, `Rows Removed by Filter: 95496`.
- **B8** — `FilterWorlds` до 1,19 с при отдалённой камере; в изоляции тот же
  SQL 14–110 мс, причина не найдена.
- `/admin/stats` кэшируется: отдал 318 257 планет при 319 573 в БД.
- `migrations/migrations.go` содержит `Apply(db)`, но `main.go` его не вызывает.

---

### Добавлено

**Описания планет (фазы 1+2)**
- **Фаза 1 (скелет):** 8 типов × (30 openings + 30 closings) = 480 текстов:
  `volcanic`, `desert`, `earthlike`, `radioactive`, `organic`, `glass`, `metal`, `rocky`.
- **Ранее:** `gas_giant`, `icy`, `oceanic` — по 150+150 (уже в репозитории).
- **Итого:** 11 типов, ~705 зачинов, ~690 концовок.
- **Фаза 2 (Go-код):** новые файлы в `internal/generator/planet/`:
  - `descriptions_types.go` — структуры (`Opening`, `Closing`, `DescriptionContext`, `descriptionsManager`).
  - `descriptions_tags.go` — вычисление тегов из контекста планеты.
  - `descriptions_load.go` — автопоиск JSON-файлов при старте, загрузка в память.
  - `descriptions_pick.go` — фильтрация по тегам и выбор по хэшу (FNV-1a).
  - `descriptions_manager.go` — точка входа `GenerateDescription`, склейка, fallback.
  - `descriptions_mapping.go` — маппинг «геймдизайнерский тип → папка» (`TypeIce` → `icy`).
- Автопоиск: сервер сам сканирует `config/descriptions/<type>/`, подхватывает
  все `openings_*.json` и `closings_*.json`. Новые файлы — через `git push`, без правок кода.
- Детерминированный выбор по хэшу от `planet.id`. Fallback — 7 нейтральных вариантов.
- Интеграция: `planet_data_generate.go`, `planet_data_gas.go`, `main.go`.
- **Детали:** `docs/DESCRIPTIONS_WORK.md`.

**Серверная кластеризация карты**
- `filter_worlds_handler.go` переписан под SQL `GROUP BY` по ячейкам сетки.
- Клиент присылает `x_min/x_max/y_min/y_max/cell`, сервер отдаёт 100–5000 кластеров
  вместо 100 000 миров. Gzip-сжатие ответа.
- Фронт: `map_render.js` рисует кластеры (кружки с числами), `data.js` грузит
  их с debounce 180 мс, `events.js` — pan/zoom/click, `main.js`, `animation.js`, `navigation.js`.
- `minZoom: 0.02 → 0.001` — можно отдалить до всей галактики.
- **Результат:** карта на 100k миров отвечает за < 100 мс.

**Миграции БД**
- `000010_assignments_type_reward.up.sql` / `.down.sql` — добавить колонки
  `type` и `reward` в `assignments` (используются кодом, но отсутствовали в схеме).

**Безопасность**
- JWT-секрет читается из env `JWT_SECRET` через `auth.InitJWTSecret` в `main.go`.
  Валидация длины ≥ 32 байта. Без секрета сервер не стартует.

### Изменено

**Безопасность**
- `internal/auth/jwt.go` — секрет больше не хардкод, живёт под `RWMutex`.
- `internal/auth/jwt.go` — в `VerifyToken` добавлена проверка алгоритма
  (защита от подмены `alg: none`).
- `internal/config/config.go` — `JWT_SECRET` обязателен, `log.Fatal` при пустом.
- `cmd/server/main.go` — вызов `auth.InitJWTSecret` в начале `main()`.

**Классификатор планет (`classify.go`)**
- **Ледяная:** `T < 150` теперь требует `ShareOf(Glaciers) >= 30`. Раньше любая
  холодная планета становилась ледяной (перекос 28%).
- **Органик:** порог по сумме биосферных форм `>= 25%` вместо требования доминанты
  (тип был почти недостижим: 0,1%).
- **Стекло/металл:** пороги `15 → 10` (эти формы в архетипах редко доходят до 15%).

**Карта миров**
- `filter_worlds_handler.go` — облегчённые поля (убраны `created_at`, `updated_at`),
  gzip, таймеры в лог, `QueryContext`, фикс `superfluous WriteHeader`.
- `GetWorld` (`world_handlers.go`) — устойчивость к падениям locations/assignments:
  если вспомогательный запрос падает, ответ всё равно уходит. `sql.ErrNoRows` → 404, не 500.

**Описания планет**
- `planet_data_generate.go` — UUID генерится раньше (нужен для выбора описания),
  все три генератора заполняют `DescriptionContext`.
- `planet_data_gas.go` — газовый гигант тоже получает описание из библиотеки.
- Фикс бага с ключом `surface:` в `generateRadioactivePlanet` — заменено
  на явную переменную `dominantSurface`.

**Админка — очистка вселенной**
- `internal/handlers/admin_universe.go` — `ClearUniverse` и `GenerateUniverse`
  используют общую функцию `clearUniverseTx`. Подход: `UPDATE users SET current_world_id = NULL`
  → снятие FK у users → `TRUNCATE` без CASCADE по явному списку таблиц → возврат FK.
  Всё в одной транзакции.

**Безопасность — `ADMIN_PASSWORD` обязательный**
- Дефолт `admin123` убран: пустое env → `log.Fatal` (`internal/config/config.go`),
  по аналогии с `JWT_SECRET`.

**Register — 409 на гонке**
- Обработан `unique_violation` (23505): `errors.As` → `*pq.Error` → `409`
  «Имя пользователя уже занято». Раньше в гонке после pre-check падало 500.

**Фронт**
- Убраны 6 отладочных `[PCM-DEBUG]` `console.log` в `web/static/js/map/events.js`.
- `alert()` → тосты (`ui/toast.js`) в админке: `auth.js`, `generation.js`, `worlds.js`.

**Спутники — формы поверхности**
- Большинство спутников получают спутниковые формы: `реголит`, `ледяная_кора`,
  `криовулканы`, `тектонические_разломы`, `гейзерные_поля` (`composition_forms.go`,
  список `AllSatelliteSurfaceForms`).
- С вероятностью ~1.5% спутник — «планетарный»: обычные планетные формы поверхности.
- `generateSatelliteSurface` — диспетчер: планетная ветвь (`generateSatellitePlanetarySurface`)
  или типовая (`generateTypicalSatelliteSurface`); недра коррелируют с водой/льдом
  (`generateSatelliteSubterrain`); жизнь без параметра поверхности (`determineSatelliteLife`);
  `satelliteDescription` переписана (`planet_data_satellites_physics.go`).
- `web/static/js/modal/tabs.js` — иконки (`🪨🧊🌋〰️💨`) и цвета новых форм.
- Юнит-тесты (временные): доля планетарных спутников 1.520%, формы без планетных у типовых.

**Данные вселенной перегенерированы**
- После правок спутников: 100k миров, `mapSize=1 400 000` (координаты до ±700k —
  как на прежней карте), `minDist=150`, кластеры 20×1200, спейсинг 200, выбросы 30%.
- Итог: 100 000 миров, планеты 319 573. 12 963 мира без планет (по дизайну).
- **Статистика классификатора (факт):** ледяная 36.5% не-гигантов, стеклянная/
  металлическая 0% (порог 15 не снижен — решение: баланс не трогать).

### Исправлено

- **`Ошибка загрузки планет: planets.forEach is not a function`** — для миров без планет
  `GetPlanetsByWorld` возвращал nil-срез → JSON `"planets": null`; фронтенд
  (`index.js:48`) подставлял весь объект ответа вместо массива → краш.
  Хендлер теперь всегда отдаёт `[]`, фронт защищён через `Array.isArray`.
- **JWT-секрет — хардкод `your-secret-key`** → env. Критично, было в публичном репозитории.
- **`GetWorld` 500 при несуществующих locations/assignments** → устойчивость.
- **`column "type" does not exist`** в `assignmentRepo.GetByWorld` → миграция `000010`.
- **`ClearUniverse` (кнопка в админке) падала с `upstream request timeout`**
  на 100k миров. Причина: `DELETE FROM worlds` — долго, Amvera-прокси рвал соединение.
  Первая попытка фикса — `TRUNCATE ... CASCADE` — **привела к потере таблицы `users`**
  (CASCADE работает на уровне таблиц, не строк; `ON DELETE SET NULL` не учитывается).
  Итоговый фикс: `TRUNCATE` без CASCADE + явный список зависимых таблиц
  + временное снятие FK у `users`. Всё в транзакции, работает мгновенно,
  `users` не трогает. Файл: `internal/handlers/admin_universe.go`, `clearUniverseTx`.
- **`superfluous response.WriteHeader call`** в `filter_worlds_handler.go` —
  убран `http.Error` после начала записи ответа.
- **`.env` был в публичном репозитории.** Содержал локальный дев-конфиг
  (`127.0.0.1`, `zorion123`, `admin123`), не прод-секреты. Удалён из индекса,
  добавлен в `.gitignore`. В истории git остаётся — приемлемо.
- **`concurrent map writes`** — не в этой сессии, но фикс в `planet_image.go`
  (`sync.Mutex` на `cache`/`cacheOrder`/`rand`).

### Удалено

- `internal/generator/planet/planet_data_description.go` — старая `generateDescription`
  больше не вызывается. **Восстановлен** после ошибочного удаления: файл содержит
  функцию `clamp`, нужную `physics.go`. `generateDescription` в нём — мёртвый код,
  пусть лежит.

**Документация:**
- `docs/CONTEXT.md` — заменён на `STATUS.md` + `docs/ARCHITECTURE.md`.
- `docs/readme.gamedev` — устаревший сводный GDD, дубликат `docs/gamedesign/`.
- `faq_gamedesign.md` — устаревший FAQ, дубликат GDD.
- `docs/PROMPT.md` — перемещён в корень как `PROMPT.md`.

### В планах

**Аномалии как контент** (следующий крупный блок)
- Библиотека готова (22 JSON-файла в `config/anomalies/`).
- Осталось: Go-код чтения при старте, API, детекция на фронте, показ в карточке планеты.

**LLM-генерация описаний**
- Groq / YandexGPT при создании планеты, сохранение в БД.
- Fallback — библиотека `config/descriptions/` + библиотека аномалий.

**Тонкая настройка генерации**
- Все хардкод-константы в конфиг, редактируемый через админку.

**Расширение описаний**
- Добить до 150+150 популярные типы: `desert`, `volcanic`, `earthlike`.
- Ревизия `gas_giant`/`icy` (600 текстов) под актуальные теги.

**Безопасность и инфраструктура**
- Вкладка «Совместимость» в админке (frontend для готового backend).
- `compatibility_matrix` — миграция `009` не применена, таблицы нет.
- Модель энергии: планетарный рынок, микроконтракты.
- Миграция старых категорий ресурсов (`energy` → `fuel`).
- Рефакторинг остальных генераторов имён на `LocalizedName`.
- CI с `go vet`, `go test`, `go vet -race`.

---

## [0.4.0] — 2026-09-10

Обновление: **аудит планет**, **карточка планеты в UI**,
**фикс гонок конкурентности**, **балансировка биосферы в холоде**.

### Добавлено

**Аудит планет**
- Пакет `internal/audit` с дженерик-движком `Run[T]` — расширяемо на другие сущности.
- Подпакет `internal/audit/planet` — 43 правила проверки планет:
  - физика (T, M/R/ρ, суммы композиций, отрицательные проценты);
  - композиция vs T (джунгли/леса/луга/болота/рифы/лёд/лава/океаны);
  - композиция vs вода (океаны, биосфера);
  - атмосфера vs T (парник, кислород, метан, водород);
  - ядро (диапазоны, возраст, флаг `is_metallic`);
  - спутники (температура, масса);
  - жизнь и обитаемость;
  - `type` ↔ `surface_dominant`;
  - мусор в данных (NaN, Inf, нули, пустое имя).
- HTTP-хендлер `GET /admin/audit`.
- Английские коды проблем (`jungles_in_cold`, `lava_in_cold`, ...).
- Защита от паник в правилах (`safeCheck`).

**Вкладка «🔍 Аудит» в админке**
- Сводка (4 плитки): всего планет, с проблемами, всего проблем, время.
- Таблица кодов проблем (код, количество, severity).
- Таблица примеров (планета, код, описание).
- Кнопка «показать все» (50 → 200).

**Карточка планеты (UI)**
- Модалка разбита: `tabs.js` + `panel.js` + `index.js`.
- Новые поля: масса, размер, плотность, температура, ядро (тип, доля, активность, радиоактивность, возраст), композиция поверхности, композиция недр, спутники газовых гигантов.
- Цветная полоска + иконки для композиции поверхности и недр.
- Обрезка топ-7 форм с кнопкой «показать все».
- Список спутников у газовых гигантов.

**Редирект на логин**
- Автоматический редирект на `/login-page` при 401/403 (в `main.js`).
- Удаление токена из `localStorage` при 401.

**LLM-генерация (обсуждение в GDD)**
- Решено: генерировать описания планет и аномалий **при создании** планеты и сохранять в БД.
- Планируемые API: Groq (free tier) или YandexGPT.

### Изменено

- **`PlanetGenerator`** в `planet_image.go` защищён `sync.Mutex` (гонка `cache`/`cacheOrder`/`rand`).
- **`WebSocketHub`** переведён на `wsClient` с per-connection мьютексом (gorilla/websocket не потокобезопасна для записи).
- **`TravelManager`** — старый полёт не удаляет новый (проверка `current == flight`).
- **`StatusManager`** — новый метод `TryStart` (атомарная проверка + запуск).
- **`admin_universe.go`** — `TryStart` вместо `if + Start`, безопасный `recoverErr`, `ClearUniverse` блокируется при активной генерации.
- **`composition_modifiers.go`** — биосферные формы при `T < 250` умножаются на ×0.05 (было ×0.2–0.3). Абсолютные запреты: лава при `T < 500`, лёд при `T > 320`.
- **`computeSatelliteTemp`** — приливный нагрев снижен с 400 до 100, добавлен потолок `giantTemp + 50`.
- **`checks_physics.go`** — severity биосферных форм в холоде `high` → `low`; газовые гиганты исключены из `hydrogen_in_heat`.
- **`models.Planet`** расширена: `Density`, `Core`, `SurfaceComposition`, `SubterrainComposition`, `Satellites`, `Climate`, `SystemAge`, `Hydrosphere`, `Biosphere`, `Radioactive`.
- **`planet_repo.go`** — парсинг ~20 полей (было 9).

### Исправлено

- **`concurrent map writes`** в `PlanetGenerator` (паника всего сервера при двух одновременных запросах картинок).
- **Concurrent write** в `WebSocketHub` (запись в одно соединение из двух горутин).
- **Race** в `TravelManager` (старый полёт удалял новый).
- **Race** в `StatusManager` (проверка + старт были неатомарны).
- **Паника от `r.(string)`** в `recover` (падало на `runtime.Error`).
- **Джунгли/леса/луга/болота** при `T 220–250 K` — теперь ×0.05, редко (2% планет).
- **`satellite_hotter_than_giant`** — 584 случая → ~0.
- **`biosphere_without_water`** — 1089 случаев → ~0.

### Удалено

- Дублирующие файлы `internal/audit/parser.go`, `auditor.go`, `checks_physics.go`, `checks_consistency.go` (переехали в `internal/audit/planet/`).

---

## [0.3.0] — 2026-09-10

Обновление: **ядро планеты**, **плотность и размер через массу**,
**правильная формула температуры**, **реалистичные спектры**.

### Добавлено

- **Ядро планеты** (`core.go`): тип, доля массы, активность, радиоактивность, возраст.
- **Физика температуры** (`physics.go`): `T_eq = 278.7 × L^0.25 / sqrt(r)`, альбедо, парниковый эффект, вклад ядра.
- **Плотность и размер**: `R = (M/ρ)^(1/3)`.
- **Спектральные классы**: взвешенное распределение (O 0.5% → M 32%).
- **Классификация планет** по композиции (`classify.go`).
- **Статистика** с распределениями по формам, недрам, ядрам.

### Изменено

- Архетипы: `size_min/max` → `mass_min/max`.
- JSON планеты: добавлены `density`, `core`, `system_age`.
- Атмосферы расширены до 14 типов.

---

## [0.2.0] — 2026-09-10

Обновление: **композиция поверхности и недр**, **6 категорий ресурсов**,
**спутники газовых гигантов**, **билингвальные имена**.

### Добавлено

- 16 форм поверхности, 17 типов недр.
- Матрица совместимости (backend + кеш).
- Спутники газовых гигантов (3–10 штук).
- 6 категорий ресурсов, маппинг «форма → категория».
- Тип `LocalizedName` (кириллица + латиница).

---

## [0.1.0] — 2026-09-09

Первый MVP-релиз: генерация вселенной, планеты, модальное окно, текстуры, админка.

---

[Unreleased]: https://github.com/lersss/zorion/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/lersss/zorion/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/lersss/zorion/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/lersss/zorion/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/lersss/zorion/releases/tag/v0.1.0