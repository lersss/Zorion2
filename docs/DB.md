# docs/DB.md — база данных Zorion

> БД, таблицы, миграции. Правила работы с БД на практике — `docs/PITFALLS.md`.
> Текущее состояние дева/прода — `STATUS.md` (не дублировать сюда).

## Таблицы

`worlds`, `planets` (JSONB `data`), `locations`, `users`, `assignments`,
`factions`, `events`, `production_units`, `settlements`, `factories`,
`goods_batches`, `planet_resources`, `compatibility_matrix`, `regions`,
`settlement_log` (лог поселения, миграция `000024`), `npc_agents`
(NPC-агенты, миграция `000026`, спека `20a.1` §2.1), `generation_config`
(реестр конфигов генерации, миграция `000031`, спека `99.2.3` §3:
`key` TEXT PK + `payload` JSONB — паттерн «дефолты в коде + override в БД»,
как матрица совместимости), `player_flights` (активные полёты игроков,
миграция `000044`, идея 97a: одна запись на игрока, PK `user_id`),
`categories`/`goods`/`goods_slots` (каталог товаров и ресурсов студии,
миграция `000045`, спека `перенос-студии-товаров-iterA` §4: единая таблица
категорий — товарные + 6 системных ресурсных (составной FK
`goods(kind, category_id) → categories(kind, id)`), товары/ресурсы по `kind`,
слоты рецептов отдельной таблицей; каталог — контент, не данные вселенной:
ClearUniverse его не трогает).

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
  + CHECK `player`/`admin`/`skycomposer`), спека `99.2.14-role-model-admin-users.md` §2.
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
  `users.ship_icon` не меняется: теперь пишутся PNG-имена из реестра
  `ShipSprites`; старые SVG-значения маппятся при чтении (`ResolveShipIcon`).
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
- `000043` — двигатель — настоящий модуль (спека `91a-ship-section-dashboard.md`
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
  source/tier_override/banned_at/created_at, `name_norm` generated + UNIQUE
  (одно пространство имён), JSONB `props` только для kind=resource, составной
  FK `(kind, category_id) → categories(kind, id)` — категория соответствует
  kind на уровне БД), `goods_slots` (good_id FK ON DELETE CASCADE, pos,
  component_id FK ON DELETE SET NULL, quantity CHECK ≥ 1, reason,
  allow_resource, UNIQUE (good_id, pos), индекс по component_id — обратные
  рёбра). Сидер (`internal/goodsstudio/seed.go`) при первом старте сеет
  131 ресурс (слой 20 + витрина 111, approved/palette, props из каталога) +
  6 ресурсных + 13 товарных категорий; маркер — ключ `goods_catalog_seed`
  в `generation_config` (payload `{"applied_at", "resources", "categories"}`):
  повторные старты не перезаписывают правки студии, удалённый ресурс не
  возвращается (С1).
- Миграции, вступающие в силу на старте, требуют перезапуска сервера
  (`AGENTS.md` §4 п.13).
- `VACUUM` внутрь миграции не положить — не работает внутри транзакции
  (`docs/PITFALLS.md`).