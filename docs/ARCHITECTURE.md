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
| `TravelManager.flights` | `RWMutex` + проверка `current == flight`; БД-строка `player_flights` (97a) — удаляет ТОЛЬКО актуальный полёт и ПОСЛЕ onArrival (гвард «новый полёт уже стартовал → не удалять») |
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
internal/regionprofile/                  — профили регионов (59a): каталог классов, интенсивность, привязка планет к региону
internal/audit/                          — движок аудита (Run[T])
internal/audit/planet/                   — 43 правила проверки планет
internal/resource/                       — 6 категорий ресурсов; универсальный слой (94a): каталог 20 ресурсов + шаблоны хемотипов + вывод окон расы + проверки; каталог 111 реальных веществ + проверка различимости (идея 2026-09-18 §4б: real.go/distance.go)
internal/names/                          — генераторы имён
internal/handlers/                       — HTTP-хендлеры
internal/models/                         — модели БД
internal/repository/                     — репозитории
internal/auth/                           — JWT, middleware
internal/travel/                         — TravelManager (полёты; персистентность 97a — таблица player_flights)
internal/config/                         — загрузка конфига из env
config/planet_archetypes.json            — архетипы планет
config/compatibility_defaults.json       — дефолты матрицы совместимости
config/anomalies/                        — библиотека аномалий (22 файла)
config/region_profiles/                  — каталог классов профилей регионов (59a, 12 файлов)
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
| Генераторы миров | `internal/generator/galaxy/` (спектральные веса, типы систем/объектов, модификаторы — спека `99.2.4`; параметры компаньонов — `35b` §3; сортировка «главная = самая массивная», пакет кратной на 3 — `35c`; масса — `29a` §4м; подкрутка весов под расу-дома — `race_tuning.go`, `99.2.22` §3.4) |
| Генераторы планет | `internal/generator/planet/` (mean-модель числа планет §5.2; ветка экзотики — `exotic.go` §5.3; P-ветка циркумбинарных — `circumbinary.go` `35b` §4.1; подкрутка входов каскада под расу-дома — `race_tuning.go`, `99.2.22` §3.3–§4: сдвиг орбиты/состава/давления/возраста/поверхности, двухслойная мягкость s) |
| Общие звёздные константы | `internal/astro/` (светимость по классу — единый источник для galaxy и planet; `35b` §3) |
| Профили регионов (59a) | `internal/regionprofile/` (каталог классов `config/region_profiles/`, интенсивность 0/1/2, `NearestRegionIndex` — привязка планет к региону); ролл в `buildRegions` (`galaxy.go`), применение к звёздам — `galaxy.go`, к планетам — `planet/` (число планет, гиганты, веса полосы, ресурсы); колонки `regions.profile`/`profile_intensity` (миграция `000036`) |
| Расы (99.2.21, 99.2.24) | `internal/races/` (каталог `config/races.json` — 60 карточек: 50 био + 10 роботов 51–60 со слоем `robotic` — энергия+сырьё вместо 13 биоосей, `territory: "conditions"`, флаги механик-кандидатов; валидация §16: сумма-100 к роботам не применяется, robotic-блок валидируется по enum; `RaceSuitable` — пригодность планеты для расы, `HumansSuitable` — пригодность для людей, 65a; подкрутка генератора под расу-дома — `tuning.go`, `99.2.22` §3.3: вывод ручек из окон карточки, кэш при загрузке каталога); раздача территорий — `internal/generator/galaxy/races.go` (агломеративная кластеризация до 60 групп + shuffle через g.rng, `regions.race_id`, миграция `000038`); поселения рас — `internal/generator/settlement/races.go` (отдельный проход: доминанта кластера + подселение соседа на выбросах, `settlements.race_id`, миграция `000039`; параметры — `chance`/`neighbor_chance`/`population` из пресета `config/race_settlement.json`, `race_preset.go`, 65a); ручка — `internal/handlers/admin_race_settlements.go` (`POST /admin/generate-race-settlements`); пригодность для людей (Settleable, тег inhabited) — `races.HumansSuitable` вместо пресета поселений (65a, старый генератор `/admin/generate-settlements` скрыт) |
| Система описаний | `internal/generator/planet/descriptions_*.go` |
| Композиция планет | `internal/generator/planet/composition_*.go` |
| Физика температуры | `internal/generator/planet/physics.go` |
| Ядро планеты | `internal/generator/planet/core.go` |
| Классификация | `internal/generator/planet/classify.go` |
| Газовые гиганты | `internal/generator/planet/planet_data_gas.go`, `gas_giant_physics.go` |
| Аудит | `internal/audit/` + `internal/audit/planet/` (45 правил; экзотика — `checks_exotic.go`) |
| Ресурсы | `internal/resource/` (6 категорий; универсальный слой 94a — `layer.go`/`chemotypes.go`/`diet.go`/`checks.go`: каталог 20 ресурсов, 13 шаблонов хемотипов, вывод окон расы из consumption, проверки §8/§13; каталог-витрина 111 реальных веществ — `real.go` (RealResource: 10 осей + T_melt/T_boil + семейство, категории 6), проверка различимости — `distance.go` (MaxAxisDiff/Colliding/ResolvedByT/DistinctCount, порог «отличие ≥ 10 по оси», идея 2026-09-18 §4б); вкладка «Ресурсы» — подвкладка «Реальные вещества»: 4 режима (Один ресурс/Все форматы/Сравнить/Список) × 6 форматов профиля (`web/static/js/admin/realResources.js` — радар-паутина canvas, бары, матрица, термометры с окнами, ярлыки-градации §9.1.5, профиль-полоска), окна: товары — демо в JS (реестр §5.2.2 не зафиксирован), расы — реальные хемотипы (`GET /admin/resources` → `chemotypes`, 13 шаблонов из chemotypes.go) |
| Имена | `internal/names/` |
| HTTP-хендлеры | `internal/handlers/` |
| Конфиг генерации (99.2.3) | `internal/handlers/admin_generation_config.go` (`GET/PUT /admin/generation/config`; ключи `star_weights`/`planet_means`/`stellar_mass_ranges` + подкрутка под расу-дома `race_tuning_softness`/`race_cluster_planet_count_mult`, `99.2.22` §4.3), `admin_regenerate_planets.go` (`POST /admin/regenerate-planets`) |
| Фильтр миров для карты | `internal/handlers/filter_worlds_handler.go` |
| Мир по ID | `internal/handlers/world_handlers.go` |
| Очистка вселенной | `internal/handlers/admin_universe.go` |
| Раздел «Пользователи» | `internal/handlers/admin_users.go` |
| Модели | `internal/models/` (экзотика — `stellar_mods.go`; конфиг — `generation_config.go`) |
| Репозитории | `internal/repository/` |
| Полёты игрока (идея 97a) | `internal/travel/manager.go` (TravelManager: in-memory map + горутина на полёт; персистентность — `FlightStore`-интерфейс, `Restore(now, worldExists, onArrival)` при старте: прошлый `arrive_at` → прибытие сразу, будущий → перерегистрация с оригинальными StartTime/Duration и остатком `waitFor`, битый мир → не восстанавливать + Delete); `internal/repository/player_flight_repository.go` (Upsert/Delete/ListAll, таблица `player_flights`, миграция `000044`); модель — `internal/models/player_flight.go`; обвязка — `cmd/server/main.go` (Restore до старта HTTP) |
| NPC-агенты (спека `20a.1`) | `internal/models/npc_agent.go`, `internal/repository/npc_repository.go` (этап 1: модель, курсорные batch-выборки, Insert/Delete/GetByID/Update); `internal/npc/` (этап 2: `manager.go` — планировщик, `agent_cache.go` — in-memory кэш агентов для позиций (идея 26c A2: один ListAll при старте/инвалидации, инкремент стартами/прибытиями тика, dirty-флаг от внешних мутаций), `worldgrid.go` — выбор маршрута по сетке миров, `position_cache.go` — snapshot позиций, `settings.go` — лимиты §2.4; этап 5: `notification_batch.go` — буфер прибытий с дросселем §2.2.C); ручки (этап 4): `internal/handlers/admin_npc.go` (CRUD), `npc_settings.go` (настройки менеджера), `npc_positions.go` (позиции для карты); уведомления (этап 5): `internal/handlers/ws_notifier.go` (batch → `WSHub.Broadcast`), `websocket_hub.go` (`Broadcast`) |
| JWT + роли | `internal/auth/jwt.go`, `internal/auth/middleware.go`, `internal/auth/admin_auth.go` |
| Визуал кораблей (спека `61b`) | реестр/маппинг/палитра — `internal/models/ship_sprites.go` (Go-константы: 21 PNG, маппинг legacy `ship_icon`→PNG, палитра 9 + «Оригинал»); клиент — `web/static/js/map/ship_sprites.js` (перекраска hue+destination-in, прелоадер, агенты {спрайт,цвет} от id), `map_render.js`/`npc_agents.js` (рендер); PNG — `web/static/sprites/*.png` (21 ассет); `PUT /me/ship-color` (`internal/handlers/user_handlers.go`) |
| Модель корабля, оборудование, радар (спека `77a`) | модели — `internal/models/ship.go` (ShipModel/EquipmentItem, радиусы 800/200, TTL знания 7 дней), `internal/models/knowledge.go` (PlanetKnowledge); каталог оборудования — `internal/ship/` (`catalog.go` — in-memory из БД + дефолты, `radar.go` — радиус по установленному радару); знание о планетах — `internal/repository/knowledge_repository.go` (Get/Upsert, KnownWorldIDs — «зажжённые» системы, ScanSystem — ленивый прогон сканера); серверная видимость — `internal/handlers/visibility.go` (круг радара, позиция игрока с интерполяцией полёта), `planet_visibility.go` (скрытие деталей планет за знанием), `players_positions.go` (`GET /api/players/positions`); применение — `filter_worlds_handler.go` (гибрид «звёздное поле»), `planet_handler.go` (403 вне радиуса), `npc_positions.go`/`npc_search.go` (фильтр агентов), `travel_handlers.go` (валидация цели); клиент — `web/static/js/map/map_render.js` (граница радара + переключалка §9.1, точки-огоньки, чужие игроки), `events.js` (тултип «вне зоны детальной видимости»), `modal/tabs.js` (заглушка «нет данных — купить отчёт»); миграции `000040` (таблицы), `000042` (радиус radar_1 400→800) |
| Балансировщик кривых R(X) (спека `99.2.17`) | модель/математика + дефолты — `internal/economy/settlement/balancer_curve.go` (bendTransform/evaluateCurve/default*Curve); in-memory store `RWMutex` — `internal/economy/settlement/balancer_store.go` (SegmentNode/ComponentCurve/GetCurve/SetCurve/ResetCurve/SampleCurve, дефолты при рестарте); пресеты кривых (итерация 7) — `internal/economy/settlement/balancer_presets.go` (JSON-файл `config/balancer_presets.json`, env `BALANCER_PRESETS_FILE`, атомарная запись tmp+rename, «сохранил = применил», default не удаляется); ручки — `internal/handlers/admin_balancer.go` (GET/PUT curve, reset, server sample, etalons, presets/apply/reset-default; валидация 422, 404); мини-R (`change_components.go`) читают store на каждом вызове; клиент — `web/static/js/admin/balancer.js` + `balancerCanvas.js` (вкладка «Балансировка», регистрация в `tabs.js`/`admin.html`) |
| Расовые R-кривые (спека `99.2.23`, Вариант Б — материализация) | store + доступ — `internal/economy/settlement/race_balancer.go` (RaceCurves/RaceRecord, GetRaceActiveCurves — для R-модели; SetRaceCurve/SetRaceReproduction/RegenerateRaceFactory/ResetRaceToFactory/RaceStatusList — для админки; CardHash/FactoryHash/RaceCurvesEqual; клонирование); генерация из карточки — `internal/economy/settlement/race_derive.go` (deriveRaceCurves §3.3, shift*Curve, applyResilienceScale); файл — `internal/economy/settlement/race_balancer_file.go` (LoadRaceBalancer §4.4 — авто-инициализация рас без записи, атомарная запись tmp+rename, битый JSON → лог; файл `config/race_balancer.json`, env `RACE_BALANCER_FILE`); валидация — `internal/economy/settlement/race_balancer_validate.go` (validateRaceRecord/validateRaceCurves/validateRaceCurve §3.1); расовый путь R-модели — `internal/economy/settlement/change_components.go` (ChangeComponents диспетчеризует по `PlanetInput.RaceID`: NULL/"humans" — глобальный store, иначе — active-кривые расы; нетто = reproduction·(1−k)·R_ест), `death.go` (DeathCause по active-кривым); гварды переполнения — `mortality.go` (MaxPopulation = 1e300 в Population, MaxInt4Population = 2^31−1 — кламп записи в `internal/repository/economy_repository.go`); ручки — `internal/handlers/admin_race_balancer.go` (7 эндпоинтов `/admin/race-balancer/*`: curve GET/PUT, reproduction, generate, reset-factory, factory, status); близнецы с расой — `internal/generator/planet/twin.go` (TwinGroup.RaceID, TwinSpec.Axis), `internal/handlers/admin_hypothesis.go` (гейт Race.Suitable, race_id в INSERT, отчёт групп с числами R); клиент — `web/static/js/admin/balancer.js` (селектор расы, reproduction, кнопки, оверлей заводской) + `balancerCanvas.js` (пунктирный оверлей factory) + `hypothesis.js` (дропдаун расы на группу) |
| Миграции | `migrations/` |
| Архетипы планет | `config/planet_archetypes.json` |
| Матрица дефолтов | `config/compatibility_defaults.json` |
| Аномалии | `config/anomalies/<code>.json` |
| Описания планет | `config/descriptions/<type>/<openings\|closings>_NN.json` |
| Фронтенд | `web/` |
| Карта миров | `web/static/js/map/` |
| Студия товаров (99a.1) | `cmd/goods-studio/` — отдельный бинарь `zorion-goods-studio.exe` (порт 8799, конфиг `config/goods/studio.json`, состояние `goods_data/state.json` вне git): модель — `model/` (категории/товары/слоты/статусы, атомарная запись tmp+rename с ретраем); граф — `graph/` (тир = 1 + max(тиры слотов), ресурс = 0; циклы — DFS достижимости; нормализация имён); каталоги — `catalog/` (импорт `internal/resource` `LayerCatalog()`/`RealCatalog()` на чтение, 20+111 ресурсов; импорт каталогов по кнопке: top-каталог `top_catalog.json` schema 1 — `ParseTopCatalog`/`ApplyTopCatalog` (99a.2), деревья Т8 `trees_t8.json` schema 2 — `ParseTreesCatalog`/`ApplyTreesCatalog` (дедуп общей базы s_* по id, резолв good по ref/id, resource по имени, сверка тиров — предупреждение)); ИИ — `ai/` (HTTP к локальному opencode: `POST {url}/session` → `POST /session/{id}/message` с телом `{model, agent:"", parts:[{type:"text",text:prompt}]}` — `agent:""` обходит `default_agent` из opencode.json, иначе отвечает менеджер, а не модель; ответ — текст из `parts`; промпт «заполнить комплектующие» §7.2; категория составляющей — строго из реестра категорий студии (ID или имя, фолбека на родителя нет, невалидная → пустая + предупреждение в отчёте); локальный фильтр бана/исключённого + вставка по слотам §7.4); валидаторы — `validate/` («каких ресурсов нет» по слою 20+витрине 111, неполные цепочки, ссылки на не-согласованных, страховки); экспорт — `export/` (только согласованные, complexity=тир, quantity=1, resources+warnings); HTTP — `handlers/` (API §9, одна активная ИИ-генерация TryStart, состояние под RWMutex); UI — `web/index.html` (canvas-граф рядами по тиру, zoom/drag, слоты под карточкой товара — компактные ячейки-приёмники: пустой — пунктир «пусто», заполненный — имя составляющей текстом (карточка составляющей на графе одна, дублей нет), drag&drop из палитры на слот канваса, клик по слоту = выбор составляющей, контекстное меню правого клика, «добавить родителя» (обратное «Используется в:» — POST слот + PUT заполнить, откат при 409), поиск в палитре ресурсов со счётчиком, «неиспользуемые», переключатель «забаненные», авто-обновление) |
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