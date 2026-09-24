# docs/DB.md — база данных Zorion

> БД, таблицы, миграции. Правила работы с БД на практике — `docs/PITFALLS.md`.
> Текущее состояние дева/прода — `STATUS.md` (не дублировать сюда).

## Таблицы

`worlds`, `planets` (JSONB `data`), `locations`, `users`,
`factions` (NPC-фракции, одна на расу: `race_id`, миграция `000066`), `events`, `settlements`,
`goods_batches`, `planet_resources`, `compatibility_matrix`, `regions`,
`settlement_log` (лог поселения, миграция `000024`), `npc_agents`
(NPC-агенты, миграция `000026`, спека `20a.1` §2.1; `race_id` — миграция
`000074`, спека `2026-09-23-корабли-рас`), `generation_config`
(реестр конфигов генерации, миграция `000031`, спека `99.2.3` §3:
`key` TEXT PK + `payload` JSONB — паттерн «дефолты в коде + override в БД»,
как матрица совместимости), `player_flights` (активные полёты игроков,
миграция `000044`, идея 97a: одна запись на игрока, PK `user_id`),
`categories`/`goods` (каталог товаров и ресурсов студии,
миграция `000045`, спека `перенос-студии-товаров-iterA` §4: единая таблица
категорий — товарные + 6 системных ресурсных (составной FK
`goods(kind, category_id) → categories(kind, id)`), товары/ресурсы по `kind`;
каталог — контент, не данные вселенной:
ClearUniverse его не трогает), `recipes`/`recipe_components`/`producer_recipes`
(рецепт как сущность — выход-товар + состав + сложность и набор рецептов
фабрики, миграция `000055`, спека
`2026-09-21-рецепт-сущность-и-граф-фабрики` §3: `recipes` — один рецепт на
товар, `recipe_components` — замена `goods_slots`, `producer_recipes` — M:N
«конкретная фабрика ↔ рецепты»), `producer_types`/`items`/`producer_items`
(каталог типов производителей и предметов студии, миграция `000048`, спека
`2026-09-20-фабрики-сущность-производства` §3.1: BIGSERIAL-ключи как в
000045; `producer_types` — типы производителей (kind goods/items/energy,
`category_id` для kind=goods, `race_family`, вход/выход/параметры JSONB),
`items` — справочник ТИПОВ предметов («что бывает», экземпляры — в
инвентаре, не здесь), `producer_items` — связь «производитель предметов ↔
предметы»; каталог — контент, ClearUniverse не трогает), `buildings` (строения
на планете, миграция `000056`, спека `2026-09-21-фабрики-релиз-2-столицы-фракций`
§2: `id` UUID, `planet_id` FK → `planets` ON DELETE CASCADE, `building_type`
(свободный ключ, в первой итерации — `capital`), `owner_type` CHECK
(player/faction/agent) + `owner_id` (без FK: владелец полиморфный),
`created_at`/`updated_at`; частичный UNIQUE `(owner_type, owner_id) WHERE
building_type='capital'` — одна столица на фракцию; колонки
`producer_type_id`/`population`/`slots`/`status`/`data`/`name` §3.2 спеки фабрик
отложены до следующих итераций; чистится вместе с мирами; только столицы
  фракций, население/снабжение — потом; таблица — данные вселенной, ClearUniverse
  её TRUNCATE-ит), `accounts` (счёт актора — игрок/фракция/агент, миграция
  `000061`, спека `2026-09-22-деньги-и-эскроу` §3.1: PK `(owner_type, owner_id)` —
  один счёт на владельца, `owner_type` CHECK (player/faction/agent), `owner_id`
  UUID без FK (владелец полиморфный), `balance` BIGINT CHECK `>= 0`,
  `withdrawable` BIGINT — корзина «заработанное», CHECK `withdrawable <= balance`;
  счёта поселений/построек нет — кошелёк поселения лимит, а не счёт, залог платит
  владелец; кошелёк игрока переживает `ClearUniverse` — `accounts` НЕ в
  `truncateTables`), `money_operations` (журнал движений по счёту, миграция
  `000061`, спека §3.2: `delta`/`balance_after`/`kind` (открытый список:
  escrow_lock/release/return, contract_work_earn, admin_seed, позже
  mint/salary/transfer)/`contract_id` без FK (журнал переживает удаление
  контракта)/`occurred_at`; индекс `idx_money_operations_owner (owner_type,
  owner_id, occurred_at DESC)`), `contracts` (состояние контракта как сущности,
  миграция `000062`, спека `2026-09-22-контракт-модель-сущности` §4.1: `id`,
  `type` (открытый список, CHECK нет), `author_type` CHECK
  (player/faction/building/agent) + `author_id` (без FK — автор полиморфный),
  `publication_planet_id` UUID FK → `planets` ON DELETE CASCADE (место —
  планета, не мир), `title`/`description`, `payload` JSONB (нагрузка типа),
  `reward` BIGINT CHECK `>= 0`, `funding` CHECK (regular/contract_work),
  `escrow_amount`/`escrow_withdrawable` BIGINT CHECK `>= 0` (залог на контракте;
  `escrow_amount` не обнуляется при release/return; `escrow_withdrawable <=
  escrow_amount`), `escrow_kind` (`deposit`), `status` CHECK
  (draft/open/taken/completed/cancelled/expired), `visibility` CHECK
  (public/direct) + `direct_target_*`, `executor_type` CHECK (player/agent) +
  `executor_id` (NULL = не взят), `taken_at`/`expires_at`/`created_at`/
  `updated_at`; инварианты CHECK: прямой контракт без адресата, взятый без
  исполнителя, живой (open/taken) без залога; индексы §6.2 — частичные
  `idx_contracts_board (publication_planet_id, created_at DESC) WHERE status='open'`,
  `idx_contracts_expiry (expires_at) WHERE status IN ('open','taken')`,
  `idx_contracts_author`, `idx_contracts_executor WHERE executor_id IS NOT NULL`,
  `idx_contracts_direct_target WHERE visibility='direct'`), `contract_requirements`
  (требования-окно, миграция `000062`, §4.2: `id` BIGSERIAL, `contract_id` FK →
  `contracts` ON DELETE CASCADE, `pos` + `UNIQUE (contract_id, pos)`, `kind`
  (axis/goods/gear, открытый), `subject`, `op` CHECK (ge/le/eq/in),
  `threshold_num`/`threshold_text`/`quantity`; интервал = две односторонние
  строки; температура в K), `contract_log` (жизнь контракта, миграция `000062`,
  §4.3: `contract_id` **без FK** — лог переживает удаление контракта; `type`
  (published/taken/completed/cancelled/expired/failed/escrow_locked/
  escrow_released/escrow_returned, открытый); `actor_type`/`actor_id`; `data`
  JSONB (`escrow_returned.data.reason` = expired/cancelled/world_deleted);
  `occurred_at`; индекс `idx_contract_log_contract (contract_id, occurred_at
  DESC)`; структура по духу `settlement_log`, но без CASCADE). Залог: lock
  при публикации / release при выполнении / return при отмене/истечении/удалении
  — `internal/repository/contract_repository.go`. Колонка `npc_agents.owner_faction_id UUID NULL
  REFERENCES factions(id) ON DELETE SET NULL` (миграция `000061` §6) — владелец
  агента; NULL = не назначен.

`deposits` (залежь поверхности, спека `2026-09-22-поселение-добыча-сырья-биома-ленивый-буфер`,
итерация 1, миграция `000058`): планета + ресурс каталога + слой `surface` + богатство +
конечный запас, FK `goods` ON DELETE CASCADE. Выработанная залежь (`amount = 0`) скрыта от
игрока (`stripPlanetDetails`/`filterActiveDeposits`).

`system_belts` (пояс малых тел — объект системы, спека `2026-09-21-пояса-малых-тел-объект-системы`,
миграция `000059`): открытый `kind` (астероидный/Койпера/Оорта/обломочный/пылевое кольцо),
геометрия, FK `worlds` ON DELETE CASCADE; входит в `truncateTables`. Колонка
`iron_remaining DOUBLE PRECISION NULL CHECK (>= 0)` (миграция `000069`, этап 3 — добыча в
поясе): конечный запас пояса, `NULL` = «нет данных», `0` = «выработан», `> 0` = запас.
Вторая колонка `ice_remaining DOUBLE PRECISION NULL CHECK (>= 0)` (миграция `000078`,
расширение 2026-09-24 — второй ресурс пояса, лёд → «Вода неочищенная»): конечный запас
льда пояса; `NULL` **двусмыслен** — «нет данных о льде» **или** «льда в поясе нет»
(различие только через `composition.ice`), `0` = «выработан», `> 0` = запас.

`settlement_branches` / `settlement_branch_buffers` (ветка поселения, спека
`2026-09-22-поселение-ветка-буферы-переработка`, миграция `000064`): ветка = поселение ↔ рецепт +
чек-точка `processed_at`; буферы — вход/выход (ресурс → количество, `double`), FK `goods`
CASCADE. Итерация 3 (добыча из залежи) — без миграции (`deposits.amount`, `processed_at`).

`player_cargo` (содержимое трюма игрока, спека `2026-09-22-трюм-грузоподъёмность-корабля`,
миграция `000068`): `(user_id, good_id, quantity)` — FK `users`/`goods` ON DELETE CASCADE,
`quantity >= 0`, PK `(user_id, good_id)`; строки с нулём удаляются. Ёмкость трюма — врождённая
`ship_models.base_capacity` + грузовые модули `equipment.type='cargo'` (`params.capacity`) в
универсальном слоте. `player_cargo` **не** входит в `truncateTables` — состояние игрока,
переживает очистку вселенной (§3.5).

`contract_board_state` (чек-точка ленивой материализации доски планеты, спека
`2026-09-23-контракт-ленивая-доска-пакет-и-снабжение` §3.1, миграция `000071`):
`planet_id` UUID PK FK → `planets` ON DELETE CASCADE, `materialized_at`/`updated_at`
TIMESTAMPTZ `DEFAULT NOW()`; чек-точка **своя** — не `settlements.computed_at` и не
`settlement_branches.processed_at`. Строка спарсена: появляется только у планеты с нуждами.
Входит в `truncateTables` (иначе `TRUNCATE planets` падает). Той же миграцией — колонки
пакета `contracts.package_key` TEXT NULL / `contracts.share_index` INTEGER NULL (NULL —
контракт вне пакета) и частичные индексы: `uq_contracts_package_open_share
(package_key, share_index) WHERE status='open'`, `uq_contracts_package_taken_executor
(package_key, executor_id) WHERE status='taken'`, `idx_contracts_package_open`.

`effect_types` (каталог типов эффектов, спека
`2026-09-22-эффекты-снабжения-задержка-голод`, миграция `000070`): `impact` (открытый
набор), `params.curve` — **ссылка** на компоненту «Балансировки» (`recovery` в типе
НЕ хранится); не чистится (`truncateTables` его не содержит).

`active_effects` (состояние владельца, миграция `000070`): `load`/`load_at`
(нагрузка/базис), полиморфный владелец (`owner_settlement_id` CASCADE), источник —
`source_position` (позиция корзины, TEXT, без FK); **входит в `truncateTables`**.
Кривая силы и скаляр `recovery` — **не в БД**, а в in-memory store «Балансировки»
(`config/balancer_presets.json` хранит пресеты, включая `hunger`). Таблиц
потребности/позиции **нет** — спрос/покрытие/дефицит производны (позиция = категория
`categories`).

Удалены: `production_units` (легаси 000018-эпохи, снос миграцией `000050`,
спека `2026-09-20-фабрики` §11.6, решение создателя 3b.6.8), `factories`/
`goods_batches` (миграция `000034` — имя `factories` свободно), `assignments`
(муляж заданий, снос миграцией `000062` — заменён `contracts`; миграции
`000010`/`000035` его колонок — историческое, применены до `000062`).

Проектные масштабы для расчётов нагрузки: 100k миров, ~320k планет.

## Миграции

- Файлы в `migrations/`, применяются автоматически при старте: `cmd/server/main.go`
  вызывает `migrations.Apply(db)`, учёт в таблице `schema_migrations`. Руками
  накатывать больше не нужно.
- Если учёта в базе нет, а схема уже есть, `Apply` делает **baseline**: отмечает
  все файлы применёнными, ничего не выполняя. Иначе накат упал бы — `003`, `006`,
  `000009` и `000011` не идемпотентны. Сработало один раз на рабочей БД 2026-09-11.
- `005_economy_tables.sql` — пустой стаб, пропущен; экономические таблицы
  (`settlements`, `factories`, `goods_batches`) создаёт `000018_create_economy_tables.sql`.
- `000008` применена частично: `idx_planets_world_id` в БД есть, GIN
  `idx_planets_data` — нет.
- `000014` — индексы `LOWER(name)`; `000015` — таблица `regions` для карты.
- `000024` — лог поселения `settlement_log` (запись «Вымерло»: `type`,
  `occurred_at NOT NULL` — записи без даты не существует, `cause`; срез
  2026-09-14, концепция создателя: лог, не поля у поселения; бэкфилл отменён —
  см. `18b_settlement_log.md`, §«Лог поселения»). Миграция `000023` (поля
  `died_at`/`death_cause`) отменена до создания.
- `000025` — роль пользователя `users.role` (`TEXT NOT NULL DEFAULT 'player'`
  + CHECK `player`/`admin`/`skycomposer`), спека `docs/specs/_archive/99.2.14-role-model-admin-users.md` §2.
  Существующие учётки получили `player`; первый skycomposer создаётся
  бутстрапом на старте (env `SKYCOMPOSER_BOOTSTRAP_*`), см. §5 спеки.
- `000026` — таблица `npc_agents` (спека `20a.1` §2.1): id UUID PK, name,
  status (`idle`/`flying`/`observing` + CHECK), `current_world_id` NOT NULL,
  `from_world_id`/`target_world_id`/`depart_at`/`arrive_at` (кортеж полёта,
  только при `flying`), `notify_enabled` DEFAULT true, `last_observed_at`.
  Индекс `(status, id)` — по статусу + покрытие курсорной batch-выборки
  `WHERE status = $1 AND id > $2 ORDER BY id` (спека §2.2.A). FK на `worlds` —
  таблица добавлена в `truncateTables` (`admin_universe.go`).
- `000029` — индекс пагинации списка агентов (спека `26a.1` §5.3):
  `idx_npc_agents_created_id ON npc_agents (created_at DESC, id DESC)` —
  keyset-фильтр `(created_at, id) < (...)`, O(страница) при сотнях тысяч
  строк; аддитивен, на тик планировщика не влияет.
- `000030` — экзотические типы звёзд (спека `99.2.4` §3): `worlds.system_type`
  (`single`/`binary`/`multiple`), `worlds.star_type`
  (`star`/`white_dwarf`/`neutron`/`black_hole`/`protostar`), `worlds.stellar_mods`
  JSONB (фаза, переменность, подтипы, параметры двойной — открытый пакет;
  `35b` §2.1 добавляет ключи `companion_mass`/`companion_temp`/
  `companion_sep_au`/`extra_companions[]` — миграция не нужна).
  Дефолты миграции — существующие миры `single`/`star` без модификаторов;
  `spectral_class` у экзотики — NULL (колонка допускает NULL).
- `000031` — таблица `generation_config` (спека `99.2.3` §3): реестр конфигов
  генерации, ключи `star_weights` и `planet_means` (веса звёзд и средние
  числа планет; дефолты — спека `99.2.4` §4.1/§4.2/§5.2).
- `000032` — масса звезды `worlds.stellar_mass` (`DOUBLE PRECISION`, nullable;
  дополнение `29a` §4м). Старые миры без массы (NULL); новые заполняются
  генератором по диапазонам (конфиг `stellar_mass_ranges`, 99.2.3).
- `000033` — возраст системы `worlds.age` (`DOUBLE PRECISION`, млрд лет,
  nullable; спека `41a` §3.1). Заполняется генератором для остатков
  (ЧД/НЗ/WD, 2–13 млрд лет) и протозвёзд (1–10 млн лет); NULL у старых миров
  и обычных звёзд (возраст показывает фронтовый справочник по классу).
- `000036` — профиль региона (59a, спека `99.2.10` §10): `regions.profile`
  (`TEXT`, ключ класса из `config/region_profiles/`, NULL/пусто — фоновый
  регион ~25%) + `regions.profile_intensity` (`SMALLINT NOT NULL DEFAULT 0`,
  0/1/2: слабая/средняя/сильная). Служебные колонки: в API регионов
  (`/api/regions`) не выводятся (профиль — не ярлык, §11.7).
- `000037` — цвет перекраски спрайта корабля `users.ship_color` (`TEXT`,
  nullable; спека `61b` §3.3/§7): NULL = «Оригинал» (без перекраски), иначе
  hex из `ShipColorPalette` (9 хроматических, `internal/models/ship_sprites.go`).
  Валидация при записи — в `PUT /me/ship-color` (NULL/палитра, иначе 400).
  `users.ship_icon` не меняется: пишутся PNG-имена из расового реестра
  `RaceShipSprites` (легаси-имена и маппинг при чтении выпилены — миграция
  `000074`, спека `2026-09-23-корабли-рас…` §8).
- `000038` — доминантная раса территории `regions.race_id` (`TEXT`, nullable;
  спека `99.2.21` §7.1): NULL = территория без назначенной расы (до раздачи,
  легаси-вселенные). Заполняется генератором галактики (раздача 60 территорий
  по 60 расам каталога — 50 био + 10 роботов, `internal/generator/galaxy/races.go`).
- `000039` — раса поселения `settlements.race_id` (`TEXT`, nullable; спека
  `99.2.21` §2.3/§7.4): NULL = легаси/люди. Две расы на одной планете = два
  ряда `settlements` с разными `race_id` (правило §13.2.1 «на одной планете
  может быть несколько поселений»). Заполняется генератором поселений рас
  (`internal/generator/settlement/races.go`, отдельный проход от человеческого).
- `000040` — модель корабля, оборудование и радар — видимость игрока (спека
  `77a-ship-equipment-radar.md`, вектор «Игрок», 2026-09-17). Таблицы:
  `ship_models` (справочник моделей, стартовая `starter`), `equipment`
  (справочник оборудования: `radar_1` радиус 400, `scanner_1`), колонки
  `users.ship_model_id`/`users.equipment` (JSONB: `{"radar":"radar_1",
  "scanner":"scanner_1","engine":null}`), `player_planet_knowledge` (личный
  каталог знания о планетах, PK `(user_id, planet_id)`, протухание 7 дней —
  статус на чтении). Бутстрап: стартовые записи справочников + бэкфилл
  существующих игроков (стартовая модель + радар-1 + сканер-1).
- `000041` — стартовый мир для игроков без `current_world_id` (баг 77a:
  игрок с NULL не видит карту — видимость требует позицию; решение создателя
  2026-09-17 «давай его пока к людям кидать»). `UPDATE users SET
  current_world_id = <мир с поселением расы humans, ближайший к центру;
  фолбэк — ближайший к центру вообще> WHERE role='player' AND
  current_world_id IS NULL`. Миров нет — NULL не трогаем (EXISTS-гвард).
  admin/skycomposer не трогаем (И7).
- `000042` — увеличение дальности стартового радара в 2 раза (решение
  создателя 2026-09-17): `UPDATE equipment SET params = '{"radius": 800}'
  WHERE id = 'radar_1'` — радиус меняется у ВСЕХ игроков (radar_1 один на
  всех, включая бэкфилл 000040). Дефолты-страховка в коде:
  `models.RadarRadiusDefault = 800.0` (`internal/models/ship.go`),
  `defaultEquipment` в `internal/ship/catalog.go`.
- `000043` — двигатель — настоящий модуль (спека `docs/specs/_archive/91a-ship-section-dashboard.md`
  §7.1/§7.4, 2026-09-18): `engine_1` («Двигатель-1», params
  `{"speed_factor": 0.3}`) в каталог `equipment` (тип `engine` уже в CHECK
  77a §3.2) + бэкфилл существующих игроков: `jsonb_set(equipment, '{engine}',
  '"engine_1"')` где слот `engine` пуст/отсутствует. Стартовая комплектация
  обновлена в коде (`models.StarterEquipment`, `defaultEquipment` в
  `internal/ship/catalog.go` — дефолты-страховка); скорость полёта берётся из
  установленного двигателя (`ship.EngineSpeed`, значение 0.3 — константа 66a,
  меняется источник), полёт без двигателя запрещён для `role=player`
(`ship.HasEngine` → отказ `/travel`; админ/skycomposer — исключение, спека
   91a §6.1).
- `000044` — активный полёт игрока переживает рестарт сервера (идея 97a,
   2026-09-18): таблица `player_flights` (PK `user_id` REFERENCES `users(id)`
   ON DELETE CASCADE; `from_world_id`/`to_world_id` UUID NOT NULL — без FK на
   `worlds`, миры удаляются перегенерацией; `start_x`/`start_y` DOUBLE
   PRECISION — стартовая точка сегмента 61a (точка P); `start_time`/
   `arrive_at` TIMESTAMPTZ — абсолютные времена сегмента). Одна запись на
   игрока; редирект 61a заменяет сегмент (INSERT ON CONFLICT DO UPDATE,
   `internal/repository/player_flight_repository.go`); строка удаляется при
   прибытии (после onArrival), отмене и обработке Restore
   (`internal/travel/manager.go`).
- `000045` — каталог товаров и ресурсов студии (спека
  `перенос-студии-товаров-iterA` §4, 2026-09-19): `categories` (единая:
  товарные + 6 системных ресурсных, `kind` CHECK good/resource, `code` —
  только ресурсные, `is_system`, `name_norm` generated + UNIQUE (kind,
  name_norm), UNIQUE (kind, id) для составного FK), `goods` (kind/status/
  source/tier_override/banned_at/created_at, `description` TEXT NULL — описание
  каталога (миграция `000057`: игровое + рабочее различение записей, `NULL` =
  «описания нет»), `name_norm` generated + UNIQUE
  (одно пространство имён), JSONB `props` только для kind=resource — профиль
  реального вещества (оси + T_melt/T_boil + `family`), дискриминаторы props:
  `family` — признак real-ресурса (итерация B): витрина «Реальные вещества»
  (94a) читает real/real_summary/families из `goods` kind=resource c props ?
  'family'; `closes` — признак ресурса слоя 20 (итерация C): вкладка «Ресурсы»
  (слой/покрытие рас/gaps) читает из БД по props ? 'closes' (`?` — JSONB
  existence-оператор), составной
  FK `(kind, category_id) → categories(kind, id)` — категория соответствует
  kind на уровне БД), `goods_slots` (good_id FK ON DELETE CASCADE, pos,
  component_id FK ON DELETE SET NULL, quantity CHECK ≥ 1, reason,
  allow_resource, UNIQUE (good_id, pos), индекс по component_id — обратные
  рёбра). Сидер (`internal/goodsstudio/seed.go`) при первом старте сеет
  131 ресурс (слой 20 + витрина 111, approved/palette, props из каталога) +
  6 ресурсных + 13 товарных категорий (две товарные переименованы миграцией
  `000053`: «комплектующие»→«электроника», «детали»→«механика»); маркер — ключ
  `goods_catalog_seed`
  в `generation_config` (payload `{"applied_at", "resources", "categories"}`):
  повторные старты не перезаписывают правки студии, удалённый ресурс не
  возвращается (С1). Первичный источник каталога — сид 131 в БД; Go-каталог
  `internal/resource` — только сид и эталон (iterC 2026-09-20).
- `000046` — внутрисистемный полёт (спека `99.2.27-intrasystem-flight` §7.1,
  2026-09-20): `users.current_position` (точка стояния в системе, DOUBLE
  PRECISION x/y) + таблица `player_intrasystem_flights` (PK `user_id` — одна
  запись на игрока; `world_id`, `target_planet_id`, `arrive_at` абсолютное;
  без FK на worlds/planets — миры удаляются перегенерацией/пакманом).
  CHECK `from_type`/`to_type` включает `belt` (миграция `000065`, спека
  поясов этап 2 §5.3).
- `000047` — индексы `npc_agents(current_world_id/from_world_id/
  target_world_id)` (пакман, спека `2026-09-20-pacman-galaxy-wipe` §3.2/§9.6,
  2026-09-20): батчи пакмана удаляют агентов по трём колонкам — без индексов
  seq-scan 46к агентов = ~670 мс/батч (33 мин на 100к миров); с индексами
  критерий «100к ≤ 90 с» выполняется. Индексы с `IF NOT EXISTS` — безопасны
  для существующих БД.
- `000048` — каталог типов производителей и предметов (спека
  `2026-09-20-фабрики-сущность-производства` §3.1, 2026-09-20):
  `producer_types` (BIGSERIAL PK, name/name_norm UNIQUE, kind CHECK
  goods/items/energy, `category_id BIGINT NULL FK → categories(id)` для
  kind=goods, `race_family`, output/input/params JSONB, status), `items`
  (BIGSERIAL PK, name/name_norm UNIQUE, slot_type, unlocks/params JSONB,
  status), `producer_items` (PK (producer_type_id, item_id), FK ON DELETE
  CASCADE, requirements JSONB). Дельта `goods` (решение 3b.6.4): колонки
  `volume`/`weight` DOUBLE PRECISION NULL — данные каталога (механика
  грузов/трюма — будущая фича); approved-товар без веса/объёма не проходит
  валидацию (NULL-каталог запрещён, проверка в студии). Сид типов/
  предметов — Go (`internal/goodsstudio/seed_producers.go`, маркер
  `producer_catalog_seed` в `generation_config`), вызывается после
  `goodsstudio.Seed` (категории уже посеяны).
- `000049` — `users.pending_destination` JSONB (спека `99.2.30-composite-route`,
  2026-09-20): `ALTER TABLE users ADD COLUMN pending_destination JSONB` —
  намерение композитного маршрута; NULL = намерения нет; цели
  planet/satellite/companion (`companion` на границе транслируется во
  внутрисистемное `star` + синтетический id); очищается при
  прибытии/отмене/развороте/неактуальности, пакман чистит намерения
  съеденных миров.
- `000050` — снос легаси-таблицы `production_units` (эпоха 000018, спека
  `2026-09-20-фабрики` §11.6, решение создателя 3b.6.8 — СНЕСТИ): DROP
  TABLE; удалён `internal/repository/production_unit_repository.go`,
  `production_units` снят из `truncateTables` (`admin_universe.go`) и
  каскадов пакмана. Номер 000050 — как в спеке §11.6 (000049 занят чужой
  миграцией `pending_destination`; 000050 спеки был назначен под `buildings`
  релиза 2 — тот пойдёт отдельным номером позже (реализован как `000056` —
  первая итерация релиза 2, спека `2026-09-21-фабрики-релиз-2-столицы-фракций`).
- `000051` — дерево построек студии (спека
  `2026-09-21-студия-дерево-построек-канвас` §1.2/§1.3, 2026-09-21):
  `producer_types` + `parent_id BIGINT NULL FK → producer_types(id) ON DELETE
  RESTRICT` (базовый тип/подтип: подтип → тип-родитель; тип с подтипами не
  удаляется) и `race TEXT NULL` (второй уровень расовости: id расы из
  `config/races.json`; семейство расы — из `config/race_lore.json`
  (`F0`–`F9`|`robotic`); задана → `race_family` обязана быть задана и
  соответствовать). Data-миграция для существующих БД (сид `producer_catalog_seed`
  уже отработал): тип «Лаборатория» (kind=items, `ON CONFLICT DO NOTHING`),
  переименование трёх лабораторий по `name_norm` (пропуск если не найдено),
  `parent_id` трёх лабораторий → «Лаборатория» и «Фабрика продовольствия» →
  «Фабрика», сброс `category_id` «Добывающей платформы» в NULL (чистый тип
  уровня 3). Для свежих БД — обновлённый сид `seed_producers.go` (оба пути
  дают одинаковый результат).
- `000052` — слоты родителя (спека
  `2026-09-21-студия-скрытые-категории-строений` §1.2/§1.3, 2026-09-21,
  переписана: первая волна — колонка `hidden` у `producer_types`, снята):
  `DROP COLUMN IF EXISTS hidden` (фича не релизнута) + `CREATE TABLE
  producer_slots` (конфигурация категорий типа kind=goods: `parent_id`/
  `category_id` FK → CASCADE, `race_family`/`race` — уровень расовости,
  `hidden BOOL NOT NULL DEFAULT false`, `CHECK (race IS NULL OR race_family
  IS NOT NULL)`, `UNIQUE NULLS NOT DISTINCT (parent_id, category_id,
  race_family, race)` — NULL-safe уникальность на уровне БД). Data-миграция
  для существующих БД (путь 1): слоты под все существующие подтипы
  kind=goods (`SELECT DISTINCT ... ON CONFLICT DO NOTHING`) + базовый сид
  универсального уровня (Фабрика/Автофабрика × товарные категории, Платформа
  × ресурсные; по `name_norm` родителей, литералы нижнего регистра — коллация
  C). Для свежих БД — слоты в сиде `seed_producers.go` (путь 2, оба пути
  дают одинаковый результат). Инвариант С4: подтип kind=goods без применяемого
  слота — 400; удаление слота с заводами категории — 409 (RESTRICT).
- `000053` — переименование товарных категорий (идея
  `2026-09-21_переименование-категорий-электроника-механика`, решение создателя
  2026-09-21): data-миграция `UPDATE categories` — «комплектующие»→«электроника»,
  «детали»→«механика» (только `kind='good'`), `UPDATE goods` (товары спорных
  категорий переезжают на новые `category_id`), `UPDATE producer_types`
  (`jsonb_set(input, '{consumables}')` — корзина Автофабрики на новые имена;
  name_norm литералами нижнего регистра — коллация C). Применяется до сида: на
  свежей БД UPDATE холостые, сид создаёт новые имена; на существующей —
  переименовывает, маркерный сид пропускается.
- `000054` — снятие согласования каталога (спека
  `2026-09-21-студия-производители-лестница-предложения` §3, редакция v4; идея
  `2026-09-21_студия-производители-самая-конкретная-фабрика` §5): у
  `producer_types` статус-класс снят, возвращён единственный обратимый флаг
  `hidden BOOLEAN NOT NULL DEFAULT false` (`status='banned'` → `hidden=true`;
  К1 — скрытость записей-фабрик, снятая 000052, возвращается); у `goods`
  (товары и ресурсы) колонки `status` и `banned_at` и у `items` колонка
  `status` сняты **без замены флагом** — «убрать» такое можно только
  удалением; `goods.volume`/`weight` → `NOT NULL DEFAULT 1` (`NULL → 1`).
  `producer_slots.hidden` (000052) — вторая ось скрытости, не трогается.
- `000055` — рецепт как сущность (спека
  `2026-09-21-рецепт-сущность-и-граф-фабрики` §3, 2026-09-21): новые таблицы
  `recipes` (good_id FK CASCADE, UNIQUE (good_id), complexity INT NULL),
  `recipe_components` (замена `goods_slots`: recipe_id FK CASCADE, pos,
  component_id FK SET NULL, quantity, reason, allow_resource; UNIQUE
  (recipe_id, pos) + индекс по component_id), `producer_recipes` (PK
  (producer_type_id, recipe_id), M:N, индекс по recipe_id); data-перенос
  (рецепт на каждый товар, состав из `goods_slots`, привязка ко всем
  универсальным конкретным фабрикам категории); снос `goods_slots` и колонки
  `goods.tier_override`; таблицы рецептов — каталог-контент (в `truncateTables`
  не входят).
- `000056` — таблица `buildings` (первая итерация релиза 2 фабрик: строения
  + владелец-фракция, спека `2026-09-21-фабрики-релиз-2-столицы-фракций` §2,
  2026-09-21): `id`/`planet_id` FK CASCADE/`building_type`/
  `owner_type`+`owner_id` NOT NULL/`created_at`/`updated_at`; индексы
  `idx_buildings_planet_id` и частичный UNIQUE на столицу фракции. `buildings`
  добавлена в `truncateTables` (`admin_universe.go`) — иначе `TRUNCATE`
  падает на FK `buildings → planets`. Номер `000056`: `000055` занята
  рецептами студии (спека `2026-09-21-рецепт-сущность-и-граф-фабрики` §3);
  `000050` спеки фабрик был назначен под `buildings` релиза 2, дальше номер
  ушёл на другие релизы.
- `000057` — описание каталога студии (спека
  `2026-09-21-каталог-описание-товаров-и-ресурсов` §4, 2026-09-21): `ALTER
  TABLE goods ADD COLUMN description TEXT NULL` — одно поле для товаров и
  ресурсов (игровое описание + рабочее различение записей в студии); `NULL` =
  «описания нет» (пустая строка/пробелы на запись нормализуются в `NULL`, на
  чтении `NULL` → `""`). Бэкфилла нет — существующие 131 ресурс и созданные
  товары получают `NULL` (наполняет пакетный прогон ИИ). `props` ресурсов и
  `CHECK (goods.props)` не затрагиваются; каталог — контент, `ClearUniverse`
  его не трогает. Показ описания игроку — будущая задача (игрового экрана
  товаров нет).
- `000058` — залежи поверхности (спека
  `2026-09-22-поселение-добыча-сырья-биома-ленивый-буфер`, итерация 1 эпика «экономика
  поселения», 2026-09-22): таблица `deposits` (см. «Таблицы»); FK `goods` ON DELETE CASCADE.
  Генерация — из трёх пилотов каталога (резолв по `name_norm`); видимость — со знанием планеты.
- `000059` — пояса малых тел, этап 1 (спека
  `2026-09-21-пояса-малых-тел-объект-системы`, 2026-09-22): таблица `system_belts`
  (см. «Таблицы»), входит в `truncateTables`; очистка при перегенерации.
- `000064` — ветка поселения (спека `2026-09-22-поселение-ветка-буферы-переработка`,
  итерация 2 эпика «экономика поселения», 2026-09-22): таблицы `settlement_branches` /
  `settlement_branch_buffers` (см. «Таблицы»), каскады settlement/recipe/good. Итерация 3
  (добыча из залежи) миграции не добавляет.
- `000061` — деньги и эскроу, фундамент (спека `2026-09-22-деньги-и-эскроу`
  §3/§7, 2026-09-22): `accounts` (счёт актора player/faction/agent, PK
  `(owner_type, owner_id)`, `CHECK balance >= 0`, `CHECK withdrawable >= 0`,
  `CHECK withdrawable <= balance`), `money_operations` (журнал движений + индекс
  `idx_money_operations_owner`), `ALTER TABLE npc_agents ADD COLUMN
  owner_faction_id UUID NULL REFERENCES factions(id) ON DELETE SET NULL`
  (владелец-фракция агента, §6); бэкфилл — счёт каждому игроку
  (`PlayerBalanceSeed=10000`) и фракции (`FactionBalanceSeed=10^15`),
  `ON CONFLICT DO NOTHING` (идемпотентно). `accounts` НЕ входит в
  `truncateTables` (кошелёк игрока переживает очистку вселенной, §3.5);
  после `TRUNCATE` удаляются счета `owner_type='faction'` (фракции
  перегенерируются с новыми id; счета агентов — когда появятся, B2+). Залог/
  эскроу (lock/release/return) — этап B (`000062_contracts.sql`). Номер `000061`:
  бронь менеджера (деньги `000061`, контракты `000062`); `000060` пропущен
  (свободен).
- `000062` — контракт как сущность (спека `2026-09-22-контракт-модель-сущности`
  §4/§9, 2026-09-22): таблицы `contracts`, `contract_requirements`, `contract_log`
  (см. «Таблицы»), `DROP TABLE IF EXISTS assignments` (муляж, замена — `contracts`).
  `contracts`/`contract_requirements`/`contract_log`/`money_operations` входят в
  `truncateTables`; во всех путях удаления контрактов залог возвращается автору ДО
  удаления (§6.5, `ReturnEscrowForContractsTx`). Идёт после `000061_money.sql`.
- `000065` — `000065_belt_flight.sql` — пояс — цель внутрисистемного полёта:
  CHECK `player_intrasystem_flights.from_type/to_type` расширен значением
  `belt` (спека поясов этап 2
  `2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт` §5.3).
- `000066` — `000066_factions_race.sql` — фракции: одна на расу (идея
  `2026-09-22_фракции-одна-на-расу-со-столицей`): `ALTER TABLE factions ADD COLUMN
  IF NOT EXISTS race_id TEXT` + частичный UNIQUE `uq_factions_race (race_id)
  WHERE race_id IS NOT NULL` (NULL разрешён — легаси-фракции без расы). Генератор —
  `internal/generator/faction/faction.go` (`GenerateFactions`), столица — на родной
  планете расы (`EnsureCapitals`).
- `000068` — `000068_cargo_hold.sql` — трюм корабля (спека
  `2026-09-22-трюм-грузоподъёмность-корабля` §8): CHECK `equipment.type` расширен
  значением `cargo` + сид модуля `cargo_1` (`params.capacity=80`), колонка
  `ship_models.base_capacity` (врождённая ёмкость 20), бэкфилл универсального слота,
  таблица `player_cargo` (FK `users`/`goods` CASCADE, `quantity >= 0`,
  PK `(user_id, good_id)`). `player_cargo` — состояние игрока, **не** в
  `truncateTables` (переживает очистку вселенной).
- `000069` — `000069_belt_mining.sql` — добыча в поясе (спека
  `2026-09-22-пояса-малых-тел-этап-3-добыча` §4): колонка `system_belts.iron_remaining`
  `DOUBLE PRECISION NULL CHECK (iron_remaining >= 0)` — конечный запас пояса
  (`NULL` = «нет данных», `0` = «выработан», `> 0` = запас; бэкфилла нет). Новых
  таблиц нет: состояние захода — аддитивные ключи JSONB `users.current_position`.
- `000071` — `000071_contract_board.sql` — контракт как состояние: чек-точка
  ленивой доски + «пакет контрактов» (спека
  `2026-09-23-контракт-ленивая-доска-пакет-и-снабжение` §3.1/§4.2, Поставка 1/ЧК1):
  таблица `contract_board_state` (FK `planets` CASCADE, **в** `truncateTables`),
  колонки `contracts.package_key`/`share_index` (NULL — вне пакета), частичные
  уникальные индексы `uq_contracts_package_open_share` / `uq_contracts_package_taken_executor`
  и индекс `idx_contracts_package_open`. Идёт после `000062_contracts.sql`; номер
  подтверждён менеджером (`000070` зарезервирован спекой
  `2026-09-22-эффекты-снабжения-задержка-голод`).
- `000073` — `000073_planet_knowledge_cascade.sql` — B25: `player_planet_knowledge.planet_id`
  пересоздан как FK `planets(id)` **`ON DELETE CASCADE`** (был без каскада, `000040`) —
  знание о планете чистится при удалении планеты. Чинит падавшие пути удаления планет:
  `GeneratePlanets` (`clearPlanets`, `admin_universe.go`), `RegeneratePlanets`
  (`clearPlanetsOf`, `admin_regenerate_planets.go`), `DeleteWorld` (каскад
  `worlds → planets`, `admin_worlds.go`). Номер `000073` забронирован менеджером
  (`000072` — за спекой `2026-09-23-орбита-планеты-присутствие-и-снимок`).
- `000074` — `000074_race_ships.sql` — корабли рас: раса агента и игрока
  (спека `2026-09-23-корабли-рас-раса-агентов-и-игрока` §4.3, П1): `ALTER TABLE
  npc_agents ADD COLUMN IF NOT EXISTS race_id TEXT` (раса агента, NULL =
  нейтральный корабль), `ALTER TABLE users ADD COLUMN IF NOT EXISTS race_id TEXT
  NOT NULL DEFAULT 'humans'` (раса игрока, задел), смена DEFAULT `users.ship_icon`
  на `race_humans_starship.png` (член расового реестра `RaceShipSprites`); бэкфилл
  агентов — случайная раса из ДАННЫХ (`array_agg(DISTINCT race_id) FROM factions`,
  пул пуст → NULL), идемпотентно (`WHERE race_id IS NULL`); бэкфилл `users.ship_icon`
  — всё, что не расовый файл (`NOT LIKE 'race\_%'`), → людской корабль,
  идемпотентно. `VACUUM` не кладётся (не работает в транзакции). Номер `000074`
  забронирован менеджером.
- `000075` — `000075_default_settlement_type_id.sql` — базовый тип поселения по
  id (решение создателя 2026-09-23: резолв по id, не по имени — переименование
  «Обычное поселение» → «Городок» оставляло новые поселения без типа): бэкфилл
  ключа `generation_config.default_settlement_type_id` значением типа, реально
  используемого поселениями (на dev-БД — `148`); читает
  `repository.ResolveDefaultSettlementTypeID`. Свежая БД: поселений нет → ключ
  ставит Go-сид каталога (`goodsstudio.SeedProducers`). Идемпотентно
  (`ON CONFLICT DO NOTHING`). Номер `000075`: последний в дереве — `000074`
  (`000072` отсутствует).
- `000077` — `000077_settlement_stages_ladder.sql` — ладдера ступеней поселения
  (спека `2026-09-23-стадии-поселения-и-скорость-производства` §4.3): 7
  подтипов-ступеней класса «Поселение» (Аутпост → Посёлок → Городок → Город →
  Мегаполис → Метрополия → Экуменополис) с порогами `params.stage` (люди,
  `exit` = 75 % от `enter`; у пола «Аутпост» `exit` не читается — только
  `enter`). Базовая ступень = переименование существующего подтипа
  (`деревня`/`обычное поселение` → `аутпост`, id и ссылки
  `settlements.settlement_type_id` / `generation_config.default_settlement_type_id`
  сохраняются); «Город»/«Мегаполис» получают `stage` слиянием `||` (прочие
  ключи `params` целы); Посёлок/Городок/Метрополия/Экуменополис вставляются
  `ON CONFLICT (name_norm) DO NOTHING`. Идемпотентно; на свежей БД (миграции
  идут до Go-сида, `cmd/server/main.go`) вставка новых ступеней защищена
  `CROSS JOIN` по типу-родителю «Поселение» — нет родителя → пропуск (записи
  создаёт сид `goodsstudio.SeedProducers`). `name_norm` — литералы нижнего
  регистра (коллация `C`, `lower()` кириллицу не берёт). Номер `000077`
  забронирован менеджером (`docs/COORDINATION.md`).
- `000078` — `000078_belt_ice_remaining.sql` — второй ресурс пояса: лёд → «Вода
  неочищенная» (спека `2026-09-24-ледяные-астероиды-вода` §4, коммит `96d71fb`):
  колонка `system_belts.ice_remaining DOUBLE PRECISION NULL CHECK (ice_remaining >= 0)`
  — конечный запас льда пояса; `NULL` двусмыслен («нет данных о льде» **или** «льда
  в поясе нет» — различие через `composition.ice`), `0` = «выработан», `> 0` = запас.
  Бэкфилла нет — ленивая инициализация при входе в пояс; новых таблиц нет (состояние
  захода — аддитивный ключ `mined_ice` JSONB `users.current_position`). **Данные
  каталога (не миграция):** ресурс «Вода неочищенная» (`goods.id 426`, `kind='resource'`,
  `source='manual'`) — резолв по точному `name_norm`, **не сидер**: на проде завести
  вручную; рецепт «Вода неочищенная» → «Очищенная вода» (`goods.id 426 → 422`). Номер
  `000078` забронирован менеджером.
- `000079` — `000079_supply_goods_positions.sql` — потребление по товарам (спека
  `2026-09-24-потребление-по-товарам-и-альтернативы` §6.2, коммит `d8fd634`):
  (1) строка `effect_types` «Жажда» (`name_norm='жажда'`,
  `impact='population_rate'`, `params.curve='hunger'` — общая кривая с «Голодом»;
  `ON CONFLICT (name_norm) DO NOTHING`); (2) data-миграция `producer_types.params`
  на **всей ладдерe** класса «Поселение» (предикат `params ? 'stage'` +
  `parent_id` типа-родителя «Поселение» — по данным, не по именам; 7 ступеней,
  включая «Мегаполис»): `effects` = `{пища: голод, очищенная вода: жажда}`,
  `eat` = `{пища: 600, очищенная вода: 20000000}`, `eat_units`; ключи
  «вода»/«продовольствие» сняты. Вложенные объекты сливаются `||` +
  `jsonb_build_object` (урок `000070`), литералы `name_norm` нижнего регистра
  (коллация C, урок `000051`). **Схема не меняется.** Идемпотентно; номер
  `000079` забронирован менеджером.
- `000080` — `000080_cargo_hold_delta.sql` — трюм, дельта (спека
  `2026-09-22-трюм-грузоподъёмность-корабля` §20, решения создателя 2026-09-24):
  значение модуля `cargo_1` `params.capacity` **80 → 30 т** (старт **20 + 30 = 50 т**,
  глобально у всех, у кого модуль стоит) + `ship_models.starter.slots.universal`
  **1 → 3** (два новых слота пустые; ключи `universal`/`universal2`/`universal3`,
  бэкфилл `users.equipment` **не** делается — пустой слот = отсутствие ключа) +
  **обрезка перегруженных трюмов** до новой ёмкости (ёмкость = `ship_models.base_capacity`
  + Σ `params.capacity` установленных cargo-модулей; строки в порядке `good_id`,
  «хвост» удаляется, пограничная урезается, строки с нулём удаляются) — инвариант
  `used ≤ total` восстанавливается для **всех** записей, не только новых. Все шаги
  идемпотентны; порядок обязателен (capacity/slots **до** обрезки, иначе ёмкость
  считалась бы по старому значению). `player_cargo` — состояние игрока, по-прежнему
  **не** в `truncateTables` (И5, переживает очистку вселенной). Номер `000080`
  забронирован менеджером (`000078` — лёд в поясе, `000079` — потребление по
  товарам, см. выше).
- Миграции, вступающие в силу на старте, требуют перезапуска сервера
  (`AGENTS.md` §4 п.13).
- `VACUUM` внутрь миграции не положить — не работает внутри транзакции
  (`docs/PITFALLS.md`).