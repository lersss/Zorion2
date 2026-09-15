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
как матрица совместимости).

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
  см. `18a_population_death.md`, §«Лог поселения»). Миграция `000023` (поля
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
- Миграции, вступающие в силу на старте, требуют перезапуска сервера
  (`AGENTS.md` §4 п.13).
- `VACUUM` внутрь миграции не положить — не работает внутри транзакции
  (`docs/PITFALLS.md`).