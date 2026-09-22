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
| Биомы и недра объектами (99.2.28) | справочник «что бывает» — `internal/generator/planet/biome_catalog.go` (store RWMutex + hot-reload, валидация инварианта 17, `SaveBiomeCatalog` — атомарная запись tmp+rename, `SeedBiomeCatalog` — сброс к сиду), `biome_catalog_seed.go` + `biome_catalog_seed.json` (встроенный сид 57 биомов + 17 недр + правила типов; рабочая копия — `config/biome_catalog.json`, грузится при старте, правится админкой); физика долей — `biome_physics.go` (вес = weight_base × категория-формула × гейты условий; слой 8 draft без RNG, слой 9 недра, слой 10 биомы; признаки атмосферы §0; вулканический индекс V; гейты недр читают `Conditions` — данные справочника, не switch); классификация типов — `classify.go` (интерпретатор правил справочника, без чисел); полосы климатов — `archetype.go` (RWMutex + `GetArchetypes`/`RebuildArchetypes`/`SaveArchetypes`, hot-reload для админки); ручки — `internal/handlers/admin_biome_catalog.go` (GET/PATCH `/admin/biome-catalog`, POST `/admin/biome-catalog/reset`, GET/PATCH `/admin/planet-archetypes`; правка — admin, чтение — admin + skycomposer); клиент — `web/static/js/admin/biomeCatalog.js` (карточка на вкладке «Основное», 6 подразделов, read-only для skycomposer) |
| Физика температуры | `internal/generator/planet/physics.go` |
| Ядро планеты | `internal/generator/planet/core.go` |
| Классификация | `internal/generator/planet/classify.go` |
| Газовые гиганты | `internal/generator/planet/planet_data_gas.go`, `gas_giant_physics.go` |
| Аудит | `internal/audit/` + `internal/audit/planet/` (48 правил; экзотика — `checks_exotic.go`; биомы 99.2.28 — `checks_biomes.go`: biosphere_without_conditions/empty_biomes/biome_not_in_catalog) |
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
| Композитный маршрут (спека `99.2.30`) | намерение «после прибытия в систему X лететь к объекту P» — `users.pending_destination` JSONB (миграция `000049`), модель `internal/models/pending_destination.go`; операции — `internal/repository/user_repository.go` (Get/List/Set/ClearPendingDestination; GetByIDWithPosition + намерение); дельта `/travel` — `internal/handlers/travel_handlers.go` (валидация destination §3.1; пояс, `object_type='belt'`, атомарная запись/очистка `CancelAtomicWithDestination` §3.2–3.4, 202-путь M2 §3.5, onArrival → `autostartIntra` §4.3–4.4, `RestorePendingDestinations` §4.5 — фаза 3 после Restore межзвёздных/внутрисистемных); `/me` + `pending_destination` — `internal/handlers/auth_handlers.go`; пакман чистит намерения съеденных миров — `internal/handlers/admin_pacman.go` (шаг 4.5, каст `(…->>'world_id')::uuid`); клиент — `web/static/js/map/flight.js` (startFlight + destination, returnToMapInFlight), `map/data.js` (автооткрытие по прибытии, триггеры A/B, маркер `compositeRoute`), `map/animation.js` (триггер A), `modal/panel.js`/`tabs.js` (композитная кнопка «🚀 Лететь · через систему»), `modal/events.js` (startCompositeFlight), `modal/index.js` (фикс «Найти меня» при межзвёздном — mapState.isFlying) |
| NPC-агенты (спека `20a.1`) | `internal/models/npc_agent.go`, `internal/repository/npc_repository.go` (этап 1: модель, курсорные batch-выборки, Insert/Delete/GetByID/Update); `internal/npc/` (этап 2: `manager.go` — планировщик, `agent_cache.go` — in-memory кэш агентов для позиций (идея 26c A2: один ListAll при старте/инвалидации, инкремент стартами/прибытиями тика, dirty-флаг от внешних мутаций), `worldgrid.go` — выбор маршрута по сетке миров, `position_cache.go` — snapshot позиций, `settings.go` — лимиты §2.4; этап 5: `notification_batch.go` — буфер прибытий с дросселем §2.2.C); ручки (этап 4): `internal/handlers/admin_npc.go` (CRUD), `npc_settings.go` (настройки менеджера), `npc_positions.go` (позиции для карты); уведомления (этап 5): `internal/handlers/ws_notifier.go` (batch → `WSHub.Broadcast`), `websocket_hub.go` (`Broadcast`) |
| JWT + роли | `internal/auth/jwt.go`, `internal/auth/middleware.go`, `internal/auth/admin_auth.go` |
| Визуал кораблей (спека `61b`) | реестр/маппинг/палитра — `internal/models/ship_sprites.go` (Go-константы: 21 PNG, маппинг legacy `ship_icon`→PNG, палитра 9 + «Оригинал»); клиент — `web/static/js/map/ship_sprites.js` (перекраска hue+destination-in, прелоадер, агенты {спрайт,цвет} от id), `map_render.js`/`npc_agents.js` (рендер); PNG — `web/static/sprites/*.png` (21 ассет); `PUT /me/ship-color` (`internal/handlers/user_handlers.go`) |
| Картинка планеты (спеки 2026-09-20 + 2026-09-21) | честный генератор — `internal/generator/planet/planet_image_v2.go` (+`postprocessing.go`, `rings.go`, `noise.go`, `atmosphere.go`; один генератор на типоразмеры 256/512): поверхность из биомов {form,share} → «поле высот + регионы» (fbm 3 октавы на сфере → 2–5 крупных масс, регионы по зонам высот как мягкий байас, бюджет долей ±30%, агрессивная рваность материков — согласованный доменный варп + взвешенный Вороной; контракт «биом → цвет» — база категории + сдвиги + опциональный `color` в каталоге); свет — единый вектор L(seed) (`lightVector`), приглушённый спекл ≤0.25 по типам, освещённые кольца (день/ночь + тень планеты), контракт постобработки `applyPostProcessing(img, size, fx PostFX, rng)`; атмосфера — режимы: `stub` (нейтральный силуэт, без блика/свечения), `honest` (производная из видимых параметров, И1), `full` (реальная: состав→дымка, давление→лимб, τ_IR→свечение, cloudFraction→облака; своя система игрока или админ); старый косметический генератор — `planet_image.go` (фолбэк старых миров без биомов); эндпоинт — `internal/handlers/planet_image_handler.go` (авторизованный `/api/planet-image?planet_id&size`, режим stub/honest/full по роли/своей системе/знанию, атмосферные входы только full), диск-кэш большой — `internal/handlers/planet_image_disk_cache.go` (PNG 512 в `{PLANET_IMAGE_CACHE_DIR}/{ImageGenVersion}_{sha256(planet_id|mode)}.png`, лимит 1000 LRU по mtime, битый файл → перегенерация, стартовый клин файлов без текущего префикса включая легаси); кэш-ключ in-memory `hash64(planet_id)|mode|size|ImageGenVersion` (v4), 1000 FIFO; клиент — `web/static/js/modal/textures.js` (fetch+blob, JWT, ключ id|size|own/known/unknown), `tabs.js` (большая «Вид с орбиты» при orbit {planet} или админ, иконки биомов PNG 22px + onerror-фолбэк); иконки — `web/static/sprites/biomes/*.png` (57/57, пачки 1+2) |
| Звук интерфейса и полёта (идея 2026-09-21) | `web/static/js/ui/sound.js` — звук интерфейса и полёта (Web Audio, файлы `web/static/audio/`, CC0 Kenney; активируется только картой — в админке молчит) |
| Модель корабля, оборудование, радар (спека `77a`) | модели — `internal/models/ship.go` (ShipModel/EquipmentItem, радиусы 800/200, TTL знания 7 дней), `internal/models/knowledge.go` (PlanetKnowledge); каталог оборудования — `internal/ship/` (`catalog.go` — in-memory из БД + дефолты, `radar.go` — радиус по установленному радару); знание о планетах — `internal/repository/knowledge_repository.go` (Get/Upsert, KnownWorldIDs — «зажжённые» системы, ScanSystem — ленивый прогон сканера); серверная видимость — `internal/handlers/visibility.go` (круг радара, позиция игрока с интерполяцией полёта), `planet_visibility.go` (скрытие деталей планет за знанием), `players_positions.go` (`GET /api/players/positions`; пояс, `object_type='belt'`); применение — `filter_worlds_handler.go` (гибрид «звёздное поле»), `planet_handler.go` (403 вне радиуса; пояс, `object_type='belt'`), `npc_positions.go`/`npc_search.go` (фильтр агентов), `travel_handlers.go` (валидация цели; пояс, `object_type='belt'`); клиент — `web/static/js/map/map_render.js` (граница радара + переключалка §9.1, точки-огоньки, чужие игроки), `events.js` (тултип «вне зоны детальной видимости»), `modal/tabs.js` (заглушка «нет данных — купить отчёт»); миграции `000040` (таблицы), `000042` (радиус radar_1 400→800) |
| Пояса малых тел — этап 2 (спека `2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт`) | `internal/handlers/planet_visibility.go` — `stripBeltDetails`/`applyBeltVisibility` (BeltView пояса для игрока, спека поясов этап 2 §4.2/§4.3); `internal/repository/planet_repo.go` — `GetBeltsByWorldID` (читающая ручка поясов) |
| Балансировщик кривых R(X) (спека `99.2.17`) | модель/математика + дефолты — `internal/economy/settlement/balancer_curve.go` (bendTransform/evaluateCurve/default*Curve); in-memory store `RWMutex` — `internal/economy/settlement/balancer_store.go` (SegmentNode/ComponentCurve/GetCurve/SetCurve/ResetCurve/SampleCurve, дефолты при рестарте); пресеты кривых (итерация 7) — `internal/economy/settlement/balancer_presets.go` (JSON-файл `config/balancer_presets.json`, env `BALANCER_PRESETS_FILE`, атомарная запись tmp+rename, «сохранил = применил», default не удаляется); ручки — `internal/handlers/admin_balancer.go` (GET/PUT curve, reset, server sample, etalons, presets/apply/reset-default; валидация 422, 404); мини-R (`change_components.go`) читают store на каждом вызове; клиент — `web/static/js/admin/balancer.js` + `balancerCanvas.js` (вкладка «Балансировка», регистрация в `tabs.js`/`admin.html`) |
| Расовые R-кривые (спека `99.2.23`, Вариант Б — материализация) | store + доступ — `internal/economy/settlement/race_balancer.go` (RaceCurves/RaceRecord, GetRaceActiveCurves — для R-модели; SetRaceCurve/SetRaceReproduction/RegenerateRaceFactory/ResetRaceToFactory/RaceStatusList — для админки; CardHash/FactoryHash/RaceCurvesEqual; клонирование); генерация из карточки — `internal/economy/settlement/race_derive.go` (deriveRaceCurves §3.3, shift*Curve, applyResilienceScale); файл — `internal/economy/settlement/race_balancer_file.go` (LoadRaceBalancer §4.4 — авто-инициализация рас без записи, атомарная запись tmp+rename, битый JSON → лог; файл `config/race_balancer.json`, env `RACE_BALANCER_FILE`); валидация — `internal/economy/settlement/race_balancer_validate.go` (validateRaceRecord/validateRaceCurves/validateRaceCurve §3.1); расовый путь R-модели — `internal/economy/settlement/change_components.go` (ChangeComponents диспетчеризует по `PlanetInput.RaceID`: NULL/"humans" — глобальный store, иначе — active-кривые расы; нетто = reproduction·(1−k)·R_ест), `death.go` (DeathCause по active-кривым); гварды переполнения — `mortality.go` (MaxPopulation = 1e300 в Population, MaxInt4Population = 2^31−1 — кламп записи в `internal/repository/economy_repository.go`); ручки — `internal/handlers/admin_race_balancer.go` (7 эндпоинтов `/admin/race-balancer/*`: curve GET/PUT, reproduction, generate, reset-factory, factory, status); близнецы с расой — `internal/generator/planet/twin.go` (TwinGroup.RaceID, TwinSpec.Axis), `internal/handlers/admin_hypothesis.go` (гейт Race.Suitable, race_id в INSERT, отчёт групп с числами R); клиент — `web/static/js/admin/balancer.js` (селектор расы, reproduction, кнопки, оверлей заводской) + `balancerCanvas.js` (пунктирный оверлей factory) + `hypothesis.js` (дропдаун расы на группу) |
| Миграции | `migrations/` |
| Архетипы планет | `config/planet_archetypes.json` |
| Справочник биомов (99.2.28) | `config/biome_catalog.json` (биомы/типы недр/правила типов/параметры токсичности; сид — `internal/generator/planet/biome_catalog_seed.json`) |
| Матрица дефолтов | `config/compatibility_defaults.json` |
| Аномалии | `config/anomalies/<code>.json` |
| Описания планет | `config/descriptions/<type>/<openings\|closings>_NN.json` |
| Фронтенд | `web/` |
| Карта миров | `web/static/js/map/` |
| Студия товаров (99a.1, перенесена в игровой сервер) | Студия — часть `cmd/server` (перенос A/B/C завершён 2026-09-20): `internal/goodsstudio/` (модель/граф/валидаторы/сид + `ai/` — ИИ-заполнение, env `OPENCODE_URL/MODEL/TIMEOUT_S/MAX_RETRIES`), роуты `/studio/api/*` (JWT admin/skycomposer), UI `web/studio.html` (статика с диска, noCache), общий auth `web/static/js/auth.js` (ядро + адаптеры `admin/auth.js`/`studio/auth.js`); `cmd/goods-studio` удалён (2026-09-20), `goods_data/`/`config/goods/` не существуют |
| Студия товаров на сервере (спека `перенос-студии-товаров-iterA`, итерация A) | `internal/goodsstudio/` — домен каталога (перенос чистых пакетов студии: `model/` — сущности, `graph/` — тиры/циклы/имена (EffectiveTier: оверрайд работает и для ресурсов, С3), `validate/` — 3 валидатора (ресурсы пропускаются по kind, критика №1; статусные `non_approved_ref`/`incomplete_chain` сняты при снятии согласования, 000054); `seed.go` — сидер при первом старте: 131 ресурс (слой 20 + витрина 111, palette, props JSONB) + 6 ресурсных + 13 товарных категорий, маркер `goods_catalog_seed` в `generation_config`); SQL-доступ — `internal/repository/goods_repository.go` (categories/goods/goods_slots; каждая мутация — транзакция с `pg_advisory_xact_lock`, снимок — REPEATABLE READ; DELETE с очисткой ссылок + `cleared_links`); HTTP — `internal/handlers/studio_handlers.go` (роуты `/studio/api/*` за `auth.AdminAuth`, admin/skycomposer; статика — `web/studio.html` публична, паттерн админки); регистрация — `cmd/server/main.go` (сидер после миграций, log.Fatal при ошибке) |
| Студия товаров на сервере (спека `перенос-студии-товаров-iterB-ui`, итерация B) | UI — `web/studio.html` (статика с диска, noCache): JWT-авторизация инлайн-паттерном adminToken (общий с админкой, localStorage), все запросы — на `/studio/api/*` (маппинг id → Number), «+ ресурс» (kind=resource, без слотов, тир-поле, статусы, удаление), второе подтверждение удаления при `cleared_links > 0`, OR-фильтры справочника (перенесены 1:1), кнопки перехода админка ↔ студия, пилюли тиров 0..8 всегда; витрина «Реальные вещества» (94a) — `internal/handlers/admin_resources.go` читает БД: real/real_summary/families из goods kind=resource c props ? 'family' (маппинг русских осей → DTO, id строка, category code, t_melt/t_boil K, sublimating производное, DistinctCount на БД-данных), `internal/repository/goods_repository.go` (+RealResources) |
| Фабрики — каталог производителей/предметов (спека `2026-09-20-фабрики-сущность-производства`, релиз 1) | Ветки студии «Производители»/«Предметы»: таблицы `producer_types`/`items`/`producer_items` (миграция `000048`, BIGSERIAL как 000045) + дельта `goods.volume/weight` (3b.6.4); сид — `internal/goodsstudio/seed_producers.go` (8 типов + 4 предмета + 4 связи, маркер `producer_catalog_seed`, после `goodsstudio.Seed` — категории уже посеяны); SQL — `internal/repository/producer_repository.go` (CRUD на том же `pg_advisory_xact_lock`); HTTP — `internal/handlers/studio_handlers.go` (+`/studio/api/producers*`, `/studio/api/items*`); UI — `web/studio.html` (разделы Товары/Производители/Предметы). Снос легаси `production_units` — миграция `000050`, удалён `internal/repository/production_unit_repository.go`, снят из `truncateTables` (`admin_universe.go`) |
| Дерево построек студии (спека `2026-09-21-студия-дерево-построек-канвас`) | Ветка «Производители» — дерево на канвасе (Строения → Классы → Типы → Подтипы): `producer_types` + `parent_id` (базовый тип/подтип, RESTRICT-удаление) и `race` (второй уровень расовости) — миграция `000051` + data-миграция (переименования лабораторий, «Лаборатория»-родитель, платформа без категории); сид — `internal/goodsstudio/seed_producers.go` (10 типов: +«Лаборатория», +«Фабрика продовольствия», новые имена лабораторий, `Parent`); SQL — `internal/repository/producer_repository.go` (валидация инвариантов §1.2: глубина 1, kind наследуется, категория только у подтипов kind=goods, уникальность подтипа, RESTRICT); HTTP — `internal/handlers/studio_handlers.go` (+`/studio/api/races` — семейства F1–F9+robotic и расы из `internal/races`); UI — `web/studio.html` (`renderProdTree`, переключатель расовости, попап создания подтипа, канвас в ветке producers) |
| Рецепт как сущность (спека `2026-09-21-рецепт-сущность-и-граф-фабрики`) | `internal/goodsstudio/`: `Good.Complexity`/`RecipeID`, `State.Bindings`, `graph.EffectiveTier` читает сложность, warning `unbound_recipe`; роуты студии — новые `/studio/api/recipes*` и `POST /studio/api/producers/{id}/recipes/copy-universal`, снятые `/studio/api/goods/{id}/slots*` и `/studio/api/goods/{id}/tier`; таблицы `recipes`/`recipe_components`/`producer_recipes` — миграция `000055` (замена `goods_slots`) |
| Строения — столицы фракций (спека `2026-09-21-фабрики-релиз-2-столицы-фракций`) | Таблица `buildings` (миграция `000056`, спека §2), `EnsureCapitals()` — `internal/generator/faction/faction.go` (+ вызов в `GenerateFactions`), `buildings` в `truncateTables` + порядок ветки `total == 0` — `internal/handlers/admin_universe.go` (C3), `PlanetFactions`/`PlanetBuildings` — `internal/models/planet.go`, `attachFactionsAndBuildings` — `internal/repository/planet_repo.go`, `stripPlanetDetails` — `internal/handlers/planet_visibility.go`, вкладка «Фракции» — `web/static/js/modal/tabs.js` |
| Фракции: одна на расу со столицей (идея `2026-09-22_фракции-одна-на-расу-со-столицей`) | `factions.race_id TEXT` + частичный UNIQUE `uq_factions_race (race_id) WHERE race_id IS NOT NULL` (миграция `000066`; NULL — легаси). Генератор — `internal/generator/faction/faction.go`: `GenerateFactions` берёт кандидатов из `settlements` (`race_id` непустой, `population > 0`), родная планета расы — поселение с наибольшим населением (tiebreak — меньший `planet_id`), имя — название расы (`internal/races`, fallback `race_id`), уже созданные расы пропускаются (идемпотентно); `total` джоба — `COUNT(DISTINCT race_id)` — `internal/handlers/admin_universe.go` |
| Описание каталога студии (спека `2026-09-21-каталог-описание-товаров-и-ресурсов`) | Поле `goods.description TEXT NULL` (миграция `000057`; `NULL` = нет описания, пусто/пробелы на запись → `NULL`, на чтение `NULL` → `""`) — одно поле для товаров и ресурсов (И1). Слои: `model.Good.Description`; `goods_repository.go` (`loadGoods` +`g.description`, `UpdateGood` +`description *string` с пределом 2000 рун, `BulkCreateGoods` — третья колонка «имя \| категория \| описание», `ApplyProposals` INSERT с описанием, новый `UpdateDescriptions` + `DescItem`); `internal/goodsstudio/ai/` (`desc.go` — `ParseDescriptionResponse`/`NormalizeDescription`/`MaxDescriptionRunes`, `prompt_desc.go` — `BuildDescriptionPrompt`, `Component/Proposal/ProposalItem.Description`, `Client.Ask` — generic вместо `FillComponents`). API: `PUT /studio/api/goods/{id}` (+`description`), новые `/studio/api/descriptions/{fill\|apply\|cancel}` (JWT admin/skycomposer); `StateView` +`desc_generating/desc_report/desc_proposals/desc_total/desc_done`; один ИИ-джоб студии одновременно (общий флаг И6, второй старт — 409). UI — `web/studio.html`: textarea описания в попапе записи, кнопка «предложить описание», галка «описание ИИ» в строке создания, третья колонка массовой подгрузки, попап «Описания ИИ» (`#descPopup`, порции по 10, принять/пропустить/отредактировать, отмена сохраняет предложения — И7) |
| Деньги и эскроу, фундамент (спека `2026-09-22-деньги-и-эскроу`, этап A) | Счёт актора + журнал движений: таблицы `accounts`/`money_operations` (миграция `000061`), `npc_agents.owner_faction_id` (владелец-фракция агента, §6). Модель — `internal/models/account.go` (`AccountOwnerPlayer/Faction/Agent`, `PlayerBalanceSeed=10000`, `FactionBalanceSeed=10^15`, `Account`/`MoneyOperation`). SQL — `internal/repository/account_repository.go` (`EnsureAccount` идемпотентно, `Debit`/`Credit` одним условным оператором — проверка+действие атомарно, `GetBalance`/`GetOperations`); регистрация в одной транзакции — `user_repository.go` `CreateWithAccount` (§3.4). HTTP — `internal/handlers/money_handlers.go` (`GET /me/money` — свой баланс + своя история, О-д4; `owner_id` из JWT, чужое не отдаётся), ленивая страховка счёта — `auth_handlers.go` (`/register` в той же tx, `/me` best-effort). Залог/эскроу (lock/release/return) — этап B; счёта поселений/построек нет (решение 11) |
| Контракт как сущность + эскроу (спеки `2026-09-22-контракт-модель-сущности` §4/§6, `2026-09-22-деньги-и-эскроу` §3–§4, `2026-09-22-контракт-перелёт-и-доска` §5) | Таблицы `contracts`/`contract_requirements`/`contract_log` (миграция `000062`), `DROP TABLE assignments` (снос муляжа `internal/models/assignment.go`, `internal/models/contracts.go`, `internal/repository/assignment_repository.go`). Модель — `internal/models/contract.go` (статусы, типы автора/исполнителя, типы лога). SQL — `internal/repository/contract_repository.go`: `Publish` (одна tx: lock залога со счёта автора через CTE `FOR UPDATE` + `escrow_withdrawable = LEAST(withdrawable, amount)`, вставка контракта/требований, `contract_log` published+escrow_locked, `money_operations('escrow_lock')`), `Take`/`Cancel`/`Complete` — атомарные flip'ы `UPDATE ... WHERE status=$old` (0 строк = гонка; `Complete` дополнительно требует `expires_at > NOW()` — просроченный `taken` закрывает `ExpireDue`), `ExpireDue` (ленивое истечение §6.3), `ReturnEscrowForContractsTx` (атомарный статусный flip §6.5 + возврат залога; пустая `ContractScope{}` = вся таблица), `ResolvePublicationPlanet` (место публикации по автору §3: faction → `homeworld_id`, building → `planet_id`; player резолвит хендлер по позиции, agent не поддержан), `resolvePayerAccountQ` (§4: player/faction напрямую, building → владелец, agent → фракция). HTTP — `internal/handlers/contract_handlers.go`: `GET /api/planets/{id}/contracts` (доска, гейт знанием планеты для player), `GET /api/contracts/mine`, `POST /api/contracts` (публикация игроком — только с планеты, О-п1), `POST /api/contracts/take|cancel`, `POST /admin/contracts` (остальные авторы + отладка; планета — по автору, не из тела). Возврат залога в путях удаления — `clearUniverseTx`/`clearPlanets`/`clearPlanetsOf`/`DeleteWorld`/пакман (`ReturnEscrowForContractsTx`); `accounts` не в `truncateTables`, счета фракций и агентов удаляются после TRUNCATE (§3.5). Фронт-вкладка доски и тип «перелёт» — B2/B3 |
| Перелёт как контракт — флоу игрока (спека `2026-09-22-контракт-перелёт-и-доска` §1, подэтап B2a) | `payload` перелёта (`from_world_id`/`dest_world_id`/`dest_planet_id`, валидация при публикации — `validateTravelPayload`); окно предложения `offer_window = flight_time_ref·10` от евклидова расстояния миров (`travelDistance`/`travelOfferWindow`, CoordX/CoordY) — `internal/handlers/contract_handlers.go`. На общей публикации сервер **гарантирует свойства типа**: авто-требование к двигателю `gear/speed_factor le contractTravelGearSpeedFactorRef` (§1.3; если публикатор задал своё `speed_factor` — уважается, `ensureTravelGearRequirement`) и `funding='regular'` (§6 R8, `15_monetization` §15.4.2: перелёт подрядом не бывает); не-перелёт не затронут. Срок перелёта **выводится**, клиентский `expires_at` игнорируется (§4.3). Проверка при взятии: `ship.HasEngine` (без двигателя игрок не летает, 91a §6.1 — то же правило, что в `/travel`) + `checkGearRequirements` (значение — `ship.EngineSpeed`); перебазирование срока `expires_at = now() + flight_time_ref·1.5` (`Take(..., expiresAt *time.Time)`, `takeContractSQL` — `COALESCE($5::timestamptz, expires_at)`; `from_world_id` с позицией берущего не сверяется — осознанное упрощение) — `internal/repository/contract_repository.go`. Закрытие по прибытии в двух точках: цель-система (`dest_planet_id IS NULL`) — `TravelHandlers.ArrivalHandler`; цель-планета — `NewIntraArrivalHandler` (вкл. Restore); для каждого контракта сперва истечение (`expireDueTx`), затем завершение (`completeTx`) — `CloseTravelArrivals` ведёт весь проход **одной транзакцией**. Тип `ContractTypeTravel` — `internal/models/contract.go`. NPC-агент-исполнитель — B2b, доска во фронте — B3 |
| Пояса малых тел — этап 1 (спека `2026-09-21-пояса-малых-тел-объект-системы`) | Таблица `system_belts` (миграция `000059`, открытый `kind`, в `truncateTables`); бюджет облака `M_диск` — общая масса (нормировка профиля `w_i = c_i/S₀`, шум `ζ` не нормируется) — `internal/generator/planet/cascade.go`; генерация поясов — `internal/generator/planet/planet_data_belt.go` (+`belt_test.go`); модель — `internal/models/planet.go` |
| Погода поверхности — вид и физика (спека `2026-09-22-погода-поверхности-вид-и-физика`) | `web/static/js/surface/surface_weather.js` — выбор явления из физики планеты (гейты по давлению/радиации из `suit`-пакета + веса), 6 явлений, 3 прохода между слоями |
| Второй пакет погоды поверхности (спека `2026-09-22-второй-пакет-погоды-поверхности` + appendix) | Слой суток — `web/static/js/surface/surface_environment.js`; примитивы новых явлений — `surface_weather_shapes.js`; правки `surface_config.js`/`surface_weather.js`/`surface_main.js`/`surface_ui.js`/`surface_render.js`, `web/surface.html`, `web/static/css/surface.css`; чеки — `tools/surface-weather-check.mjs`, `tools/surface-env-check.mjs` |
| Графика карты — вид звёзд и настройки игрока (идеи 2026-09-22) | `web/static/js/map/star_render.js` (наборы Глаз/Фото/Спрайт/Корона, дефолт «Спрайт»; ядро `CLASS_GLOW`/`coreStops` — общая рецептура карты и модалки), `map/star_presets.js` (дефолты наборов и эффектов — единый источник для карты и дашборда), `dashboard/graphics.js` (вкладка «⚙️ Графика» — настройки игрока в localStorage; карта только читает), `modal/modal_render.js`/`modal/layout.js` |
| Залежи поверхности (спека `2026-09-22-поселение-добыча-сырья-биома-ленивый-буфер`, итерация 1) | Таблица `deposits` (миграция `000058`: планета + ресурс каталога + слой surface + богатство + конечный запас, FK CASCADE). Генерация — `internal/generator/planet/deposit_generation.go`; SQL — `internal/repository/deposit_repository.go`; админ-ручка — `internal/handlers/admin_deposits.go`; модель — `internal/models/deposit.go`; видимость — `stripPlanetDetails`/`filterActiveDeposits` (`internal/handlers/planet_visibility.go`); клиент — `web/static/js/modal/deposits.js` |
| Ветка поселения (спека `2026-09-22-поселение-ветка-буферы-переработка`, итерация 2; добыча из залежи — итерация 3) | Таблицы `settlement_branches`/`settlement_branch_buffers` (миграция `000064`). Переработка — `internal/economy/settlement/branch.go` (линейная скорость `k·population/complexity`, `k = 2.78·10⁻⁸`, `MinPersistInterval` 30 мин); добыча из `deposits` (лок ветка → залежи) — `internal/repository/branch_repository.go`; HTTP — `internal/handlers/admin_settlement_branches.go`; модель — `internal/models/economy.go` (`SettlementBranch`/`BranchBufferEntry`); вход обнуляется `stripBranchInputs` (`planet_visibility.go`); клиент — `web/static/js/modal/branches.js`, `modal/deposits.js`/`tabs.js` |
| Ориентация спрайта корабля (спека `2026-09-21-угол-корабля-в-метаданных`) | Пара `(A, F)` в метаданных вместо поворота пикселей: реестр `ShipSprite.Angle`/`Flip` — `internal/models/ship_sprites.go`; показ — `web/static/js/map/ship_sprites.js`/`map_render.js` (`rotate(H + V·A)`, `scale(F ? −1 : 1, V)`, `V = cos H < 0 ? −1 : 1`), дашборд; студия — CSS-трансформ; импорт принятых — `tools/import_ship_sprites.ps1` (sha256 + гвард `orient_meta`) |
| Трюм — грузоподъёмность корабля (спека `2026-09-22-трюм-грузоподъёмность-корабля`) | Сервис трюма — `internal/cargo/`; ёмкость (врождённая `ship_models.base_capacity` + грузовые модули `equipment.type='cargo'` `params.capacity` в универсальном слоте) — `internal/ship`; содержимое — `player_cargo` (миграция `000068`); HTTP — `internal/handlers/cargo_handlers.go` (`GET /api/cargo`, `POST /api/cargo/jettison`); клиент — `web/static/js/dashboard/cargo.js` (блок «Трюм» в разделе «Корабль») |
| Эффекты снабжения — фундамент (спека `2026-09-22-эффекты-снабжения-задержка-голод`, этап 1) | Таблицы `effect_types` (каталог: `impact` — открытый набор, `params.curve` — ссылка на компоненту Балансировки; `recovery` в типе НЕ хранится) и `active_effects` (`load`/`load_at`, полиморфный владелец, `source_position` без FK) — миграция `000070`; `active_effects` в `truncateTables` (`admin_universe.go`). Модель — `internal/models/effect.go`; реестр impact + резолв `EffectRate` (неизвестный impact/curve → 0 + лог) — `internal/economy/settlement/effect_impact.go`; SQL/CRUD + счётчики привязок + админ-инструмент «задать load» — `internal/repository/effect_repository.go`; HTTP студии — `internal/handlers/studio_effects_handlers.go` (`/studio/api/effects*`); UI-попап «Эффекты» — `web/studio.html`; пилотная привязка по позиции — миграция `000070` + сид `internal/goodsstudio/seed_producers.go`. **Этап 2 (ядро расчёта):** слой потребности — `internal/economy/settlement/needs.go` (спрос `население×норма(позиция)`, покрытие — сумма источников (выход веток), дефицит → `w`, нагрузка `load` + сила `R(load(t))` кусочно); owner-проход «производство → потребность → население» — `internal/repository/settlement_owner_pass.go` (одна транзакция на поселение: `pg_advisory_xact_lock(hashtext(settlement_id))` → settlements/ветки/залежи `FOR UPDATE` → единый `now`, `load_at == processed_at == computed_at`; путь «в памяти» без записи); интеграция с R — `change_components.go` (`EnvComponents` + `ChangeComponents = EnvComponents + Σ R(AsOf)`), `recompute.go` (посегментно, без `ChangeComponents`), `death.go` (вклад эффекта → `DeathCause` = `hunger`); кривая `hunger` + скаляр `recovery` — `balancer_store.go`/`balancer_curve.go` (`defaultHungerCurve`, `ComponentScalar`, заводское `0.25`). Хвост `eaten` из `ProcessBranch` убран — буфер пишет слой потребности (`O0_b + batches_b − drawn_b`). **Этап 3 (настройки/витрина):** `hunger` открыта наружу — `ValidComponent` + диапазон `[0, 8760]`, `IsRaceComponent` (гейт расовых ручек, `SetRaceCurve`, `Sample?race_id=` → 422), скаляр `recovery` в GET/PUT/`reset`/пресетах (`SetCurveWithScalar`/`ResetCurveWithScalar`, `BalancerPreset.Recovery`, заводское `0.25`); админ-ручка `POST /admin/settlements/{id}/effects` + диспетчер пути — `internal/handlers/admin_settlement_effects.go`; клиент «Балансировки» (`web/static/js/admin/balancer.js`/`balancerCanvas.js`: поле recovery, маркер порога, исключение голода из расового режима) и витрина эффектов админа (`web/static/js/modal/branches.js`/`tabs.js`) |
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

Список миграций и применённых номеров — в `docs/DB.md` (§«Миграции»).

### 4.1. Очистка таблиц с FK — только TRUNCATE без CASCADE

**Правило:** при массовой очистке таблиц, на которые ссылаются другие таблицы,
использовать `TRUNCATE` **без** `CASCADE` + явно перечислять все зависимые таблицы.

**Почему:** `TRUNCATE ... CASCADE` работает на уровне **таблиц**, а не строк.
Правило `ON DELETE SET NULL` / `ON DELETE CASCADE` у FK **не применяется** —
если таблица формально ссылается на очищаемую, она попадает в CASCADE целиком,
независимо от содержимого.

**Пример из проекта (`ClearUniverse`):**
- На `worlds` ссылаются `locations`, `planets` (прямо), `contracts` — на
  `planets` (`ON DELETE CASCADE`), `contract_requirements` — на `contracts`
  (CASCADE); и ещё 8 таблиц косвенно (через `locations` и `planets`: `factions`,
  `settlements` (→ `active_effects`, миграция `000070`), `buildings` — `buildings`
  добавлена миграцией `000056`, `planets` ← `buildings` ON DELETE CASCADE).
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