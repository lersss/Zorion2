# Архитектура Zorion

> Технический документ. Отвечает на вопросы: **как устроен код, что где лежит,
> какие правила конкурентности**. Дополняет `PROMPT.md` (правила работы с ассистентом)
> и `STATUS.md` (где мы сейчас).
>
> Обновляется по мере изменения структуры кода. Не дублирует GDD.

**Последнее обновление:** 2026-09-10

---

## 0. КЛЮЧЕВОЕ ПРАВИЛО: игра многопользовательская

**Zorion — игра с одновременной работой нескольких игроков на одном сервере.**
Об этом надо помнить при **любой** правке кода. Нагрузка одного игрока — не показатель,
проблемы начинаются, когда двое (или больше) делают что-то одновременно.

### Что это значит на практике

- **Любая map, к которой могут обратиться две горутины — под мьютексом.**
  Go убивает процесс при `concurrent map writes`, recover не спасает.
- **Любой синглтон** (`sync.Once`, глобальные managers) — потенциальный источник гонок.
- **Общие рандомы** (`*rand.Rand` на уровне пакета) не потокобезопасны.
  Мьютекс или локальный `rand.New(...)` на каждый вызов.
- **Сторонние библиотеки** могут быть не потокобезопасны (gorilla/websocket —
  только одна горутина на запись в соединение).
- **Проверка + действие** должны быть атомарными (`TryStart` вместо `if running + Start`).
- **Долгие операции** (генерация, bcrypt) увеличивают окно гонки.

### Что уже защищено

| Компонент | Защита |
|-----------|--------|
| `PlanetGenerator.cache/cacheOrder/rand` | `sync.Mutex` |
| `WebSocketHub.clients` + запись в conn | Глобальный `RWMutex` + per-connection мьютекс |
| `NPCManager.positions` (snapshot карты) | `atomic.Pointer` (lock-free чтение) |
| `NPCManager.settings` (лимиты §2.4) | `RWMutex` (тик читает, админ-ручка пишет) |
| `NPCManager.gridPtr` (сетка миров) | `atomic.Pointer` (тик пишет, хендлер читает `RandomWorld`/`WorldName`) |
| `NotificationBatch` (буфер прибытий) | `sync.Mutex` (тик менеджера добавляет, таймер WSNotifier отправляет) |
| `TravelManager.flights` | `RWMutex` + проверка `current == flight` |
| `StatusManager.jobs` | `RWMutex` + `TryStart` |
| `CompatibilityMatrix` (кеш) | `RWMutex` |
| `archetypeCache` | Только чтение после инициализации |
| `jwtSecret` | `RWMutex` + `InitJWTSecret` |
| `descriptionsManager` (описания планет) | `RWMutex` |
| `balancerCurveStore` (кривые балансировщика, спека 99.2.17) | `RWMutex`: мини-R читают на КАЖДОМ вызове (GetCurve возвращает копию), админка пишет редко |
| `balancerPresetState` (пресеты кривых, 99.2.17 итерация 7) | Та же `RWMutex` `balancerCurveStore.mu`: store и файл меняются согласованно; запись файла — атомарная (tmp + rename) под Lock |
| `*sql.DB` (везде) | Потокобезопасен «из коробки» |

### Что ещё не защищено (известные долги)

- `Register` в `auth_handlers.go` — race между `GetByUsername` и `Create`
  (БД защищает через `UNIQUE`, но возвращается 500 вместо 409).
- `planet_image.go` — кэш пишется, но не читается. Когда добавим чтение —
  обязательно под мьютексом.
- **Кеш аномалий** (когда появится Go-код для чтения `config/anomalies/`) —
  под `RWMutex`, только чтение после инициализации. Загружать один раз при старте,
  при ошибке чтения JSON — валить сервер явно (`log.Fatal`).
- Не проверено: `internal/generator/faction/faction.go`,
  `internal/generator/galaxy/galaxy.go` (при параллельной генерации).

### Как проверять при разработке

- Перед мержем **любой** правки, где появляется `var x = map[...]` на уровне пакета —
  спросить: «а если два игрока одновременно?».
- Перед мержем **любой** правки с `go func()` — подумать о разделяемом состоянии.
- Раз в N задач — перечитывать `go vet -race` (когда появится CI).
- При первом баге «сервер упал у игрока» — в первую очередь смотреть на map и горутины.

---

## 1. Структура проекта

```
cmd/server/main.go                       — точка входа
internal/generator/galaxy/               — генерация миров (Пуассон + кластеры)
internal/generator/planet/               — планеты, композиция, физика, ядро, описания
internal/generator/planet/descriptions_*.go — система описаний планет (6 файлов)
internal/audit/                          — движок аудита (Run[T])
internal/audit/planet/                   — 43 правила проверки планет
internal/resource/                       — 6 категорий ресурсов
internal/names/                          — генераторы имён
internal/handlers/                       — HTTP-хендлеры
internal/models/                         — модели БД
internal/repository/                     — репозитории
internal/auth/                           — JWT, middleware
internal/travel/                         — TravelManager (полёты)
internal/config/                         — загрузка конфига из env
config/planet_archetypes.json            — архетипы планет
config/compatibility_defaults.json       — дефолты матрицы совместимости
config/anomalies/                        — библиотека аномалий (22 файла)
config/descriptions/                     — библиотека описаний планет (11 типов)
migrations/                              — SQL-миграции (numbered, up/down)
web/                                     — фронтенд (HTML, CSS, JS, ES-модули)
web/static/js/map/                       — модули карты (canvas, кластеризация)
docs/gamedesign/                         — GDD (семейство доков, см. `AGENTS.md` §7)
```

---

## 2. Ключевые файлы и где что лежит

| Что | Где |
|-----|-----|
| Точка входа | `cmd/server/main.go` |
| Генераторы миров | `internal/generator/galaxy/` (спектральные веса, типы систем/объектов, модификаторы — спека `99.2.4`; параметры компаньонов — `35b` §3; сортировка «главная = самая массивная», пакет кратной на 3 — `35c`; масса — `29a` §4м) |
| Генераторы планет | `internal/generator/planet/` (mean-модель числа планет §5.2; ветка экзотики — `exotic.go` §5.3; P-ветка циркумбинарных — `circumbinary.go` `35b` §4.1) |
| Общие звёздные константы | `internal/astro/` (светимость по классу — единый источник для galaxy и planet; `35b` §3) |
| Система описаний | `internal/generator/planet/descriptions_*.go` |
| Композиция планет | `internal/generator/planet/composition_*.go` |
| Физика температуры | `internal/generator/planet/physics.go` |
| Ядро планеты | `internal/generator/planet/core.go` |
| Классификация | `internal/generator/planet/classify.go` |
| Газовые гиганты | `internal/generator/planet/planet_data_gas.go`, `gas_giant_physics.go` |
| Аудит | `internal/audit/` + `internal/audit/planet/` (45 правил; экзотика — `checks_exotic.go`) |
| Ресурсы | `internal/resource/` |
| Имена | `internal/names/` |
| HTTP-хендлеры | `internal/handlers/` |
| Конфиг генерации (99.2.3) | `internal/handlers/admin_generation_config.go` (`GET/PUT /admin/generation/config`), `admin_regenerate_planets.go` (`POST /admin/regenerate-planets`) |
| Фильтр миров для карты | `internal/handlers/filter_worlds_handler.go` |
| Мир по ID | `internal/handlers/world_handlers.go` |
| Очистка вселенной | `internal/handlers/admin_universe.go` |
| Раздел «Пользователи» | `internal/handlers/admin_users.go` |
| Модели | `internal/models/` (экзотика — `stellar_mods.go`; конфиг — `generation_config.go`) |
| Репозитории | `internal/repository/` |
| NPC-агенты (спека `20a.1`) | `internal/models/npc_agent.go`, `internal/repository/npc_repository.go` (этап 1: модель, курсорные batch-выборки, Insert/Delete/GetByID/Update); `internal/npc/` (этап 2: `manager.go` — планировщик, `agent_cache.go` — in-memory кэш агентов для позиций (идея 26c A2: один ListAll при старте/инвалидации, инкремент стартами/прибытиями тика, dirty-флаг от внешних мутаций), `worldgrid.go` — выбор маршрута по сетке миров, `position_cache.go` — snapshot позиций, `settings.go` — лимиты §2.4; этап 5: `notification_batch.go` — буфер прибытий с дросселем §2.2.C); ручки (этап 4): `internal/handlers/admin_npc.go` (CRUD), `npc_settings.go` (настройки менеджера), `npc_positions.go` (позиции для карты); уведомления (этап 5): `internal/handlers/ws_notifier.go` (batch → `WSHub.Broadcast`), `websocket_hub.go` (`Broadcast`) |
| JWT + роли | `internal/auth/jwt.go`, `internal/auth/middleware.go`, `internal/auth/admin_auth.go` |
| Визуал кораблей (спека `99.2.15`) | `config/ship_visual.json` (палитра/акценты/зоны/стиль); генератор — `internal/generator/ship/` (`generator.go` + `generator_{hull,nose,wings,engine,tail}.go`, `assembly.go` — `AssemblyFromSeed`); данные — `internal/models/{ship_part,ship_visual}.go`, `internal/repository/ship_repository.go` (CRUD `ship_parts` + `users.ship_visual`), `internal/repository/ship_catalog.go` (каталог в памяти: immutable snapshot + `atomic.Pointer`, §3.4); ручки — `internal/handlers/ship_handlers.go` (`GET /api/ship-parts` из памяти, O(1)), `internal/handlers/admin_ship_parts.go` (вкладка «Корабли»: генерация/удаление/перегенерация + данные предпросмотра И9); клиент — `web/static/js/map/ship_render.js` (сборка схемы + кэш спрайтов), `web/static/js/admin/ships.js` (вкладка) |
| Балансировщик кривых R(X) (спека `99.2.17`) | модель/математика + дефолты — `internal/economy/settlement/balancer_curve.go` (bendTransform/evaluateCurve/default*Curve); in-memory store `RWMutex` — `internal/economy/settlement/balancer_store.go` (SegmentNode/ComponentCurve/GetCurve/SetCurve/ResetCurve/SampleCurve, дефолты при рестарте); пресеты кривых (итерация 7) — `internal/economy/settlement/balancer_presets.go` (JSON-файл `config/balancer_presets.json`, env `BALANCER_PRESETS_FILE`, атомарная запись tmp+rename, «сохранил = применил», default не удаляется); ручки — `internal/handlers/admin_balancer.go` (GET/PUT curve, reset, server sample, etalons, presets/apply/reset-default; валидация 422, 404); мини-R (`change_components.go`) читают store на каждом вызове; клиент — `web/static/js/admin/balancer.js` + `balancerCanvas.js` (вкладка «Балансировка», регистрация в `tabs.js`/`admin.html`) |
| Миграции | `migrations/` |
| Архетипы планет | `config/planet_archetypes.json` |
| Матрица дефолтов | `config/compatibility_defaults.json` |
| Аномалии | `config/anomalies/<code>.json` |
| Описания планет | `config/descriptions/<type>/<openings\|closings>_NN.json` |
| Фронтенд | `web/` |
| Карта миров | `web/static/js/map/` |
| GDD | `docs/gamedesign/` |
| Текущий статус | `STATUS.md` |
| История | `CHANGELOG.md` |

---

## 3. Описания планет — архитектура подсистемы

Более детально — в `docs/DESCRIPTIONS_WORK.md`. Здесь — только карта файлов.

**Файлы `internal/generator/planet/descriptions_*.go`:**

| Файл | Что |
|------|-----|
| `descriptions_types.go` | `Opening`, `Closing`, `DescriptionContext`, `descriptionsManager` |
| `descriptions_tags.go` | Вычисление тегов из контекста планеты |
| `descriptions_load.go` | Автопоиск JSON-файлов при старте, загрузка в память |
| `descriptions_pick.go` | Фильтрация по тегам и выбор по хэшу (FNV-1a) |
| `descriptions_manager.go` | Точка входа `GenerateDescription`, склейка, fallback |
| `descriptions_mapping.go` | Маппинг «геймдизайнерский тип → папка» |

**Инициализация:** `main.go` вызывает `planet.LoadDescriptionsGlobal("config/descriptions")`.
При ошибке — `log.Fatal`, сервер не стартует.

**Где вызывается:** `planet_data_generate.go`, `planet_data_gas.go` — при генерации
планеты. Результат пишется в JSON планеты, в поле `description`.

---

## 4. Миграции БД

Папка `migrations/`. Формат: `NNN_name.up.sql` / `NNN_name.down.sql`.

**Важно:** миграции **не применяются автоматически**. Их выполняет пользователь
вручную через psql/DBeaver. Поэтому в SQL-файлах используется `IF NOT EXISTS`
там, где это возможно — на случай, если уже применено.

Список применённых миграций — в `STATUS.md` (раздел «Схема БД»).

### 4.1. Очистка таблиц с FK — только TRUNCATE без CASCADE

**Правило:** при массовой очистке таблиц, на которые ссылаются другие таблицы,
использовать `TRUNCATE` **без** `CASCADE` + явно перечислять все зависимые таблицы.

**Почему:** `TRUNCATE ... CASCADE` работает на уровне **таблиц**, а не строк.
Правило `ON DELETE SET NULL` / `ON DELETE CASCADE` у FK **не применяется** —
если таблица формально ссылается на очищаемую, она попадает в CASCADE целиком,
независимо от содержимого.

**Пример из проекта (`ClearUniverse`):**
- На `worlds` ссылаются `locations`, `assignments`, `planets` (прямо)
  и ещё 7 таблиц косвенно (через `locations` и `planets`).
- На `worlds` также ссылается `users` (через `current_world_id`),
  но её **нельзя** удалять.
- `TRUNCATE worlds CASCADE` снёс бы `users` целиком — что и случилось.
- Итоговый подход: `UPDATE users SET current_world_id = NULL`
  → `ALTER TABLE users DROP CONSTRAINT ...`
  → `TRUNCATE worlds, locations, planets, ...` (без CASCADE)
  → `ALTER TABLE users ADD CONSTRAINT ...` — всё в транзакции.

**Файл:** `internal/handlers/admin_universe.go`, функция `clearUniverseTx`.

**Если появится новая таблица с FK на любую из очищаемых** — `TRUNCATE`
упадёт с ошибкой `cannot truncate a table referenced in a foreign key constraint`.
Тогда добавь её в константу `truncateTables`.

---

## 5. Фронтенд — структура карты

```
web/static/js/
├── config.js              — глобальный CONFIG (настройки карты, UI)
├── main.js                — точка входа карты, init
├── filters.js             — панель фильтров
├── search.js              — поиск объектов (фокус на карте: центр+зум, кольцо, модалка)
├── modal/                 — модалка системы (tabs, panel, index, state, events)
└── map/
    ├── config.js          — state и elements
    ├── data.js            — загрузка кластеров, /me, loadUserData, handleUnauthorized
    ├── events.js          — hover, click, pan, zoom
    ├── map_render.js      — draw, отрисовка кластеров и одиночных звёзд
    ├── npc_agents.js      — NPC-агенты (спека 20a.1 §7): опрос /api/npc/positions,
    │                        иконки (ромб, видимость с nameDisplayThreshold),
    │                        клик → мини-панель, WS npc_arrivals_batch → тост
    ├── navigation.js      — centerOnAgent
    ├── animation.js       — animationLoop (только во время полёта)
    ├── starfield.js       — фон «звёздное небо» (спека 30c.1): offscreen-тайл, параллакс
    └── utils.js           — worldToCanvas, getStarColor
```

Админка — `web/static/js/admin/`: вкладка «NPC» — `npc.js` (спека 20a.1 §6, регистрация в `tabs.js`/`main.js`).

**Как работает карта (серверная кластеризация):**
- Клиент присылает `x_min/x_max/y_min/y_max/cell` в `/api/worlds/filter`.
- Сервер делает `GROUP BY` по ячейкам, отдаёт 100–5000 кластеров.
- Клиент рисует кружки с числами (кластеры) и звёзды (одиночные миры).
- При zoom/pan — debounced перезапрос (180 мс).

**Поиск объектов (`web/static/js/search.js`):**
- `GET /api/entities/search?q=...&limit=...` (JWT) — точный поиск без учёта регистра
  по трём таблицам: `worlds` (звезда), `planets` (+join `worlds`), спутники
  (`CROSS JOIN LATERAL jsonb_array_elements(p.data->'satellites')`). См.
  `internal/handlers/search_handler.go`.
- Фокус: центрирование + зум 6× + зелёное кольцо на звезде (`state.focusWorldId`).
  Планета → модалка системы с выбором планеты; спутник → модалка с автовыбором
  спутника (`openSystemModal(worldId, name, spectral, {planetId?, satelliteId?})`).

**Известная коллизия имён:** `web/static/js/config.js` и `web/static/js/map/config.js`.
Правило «имена файлов в разных папках не должны совпадать» зафиксировано
в `PROMPT.md`, но на фронте пока не применено. TODO: переименовать `map/config.js`
в `map/state.js`.

---

*Обновляется при изменении структуры кода или правил конкурентности.*