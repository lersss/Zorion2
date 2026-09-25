# docs/COORDINATION.md — реестр занятости файлов

> Зачем: несколько агентов работают параллельно (AGENTS.md §4.10). Перед правкой
> общего файла — проверь, не занят ли он; начал — отметь; закоммитил — сними метку.
> Мягкий локейт для **кода и обычных доков**. **Горячие доки** сюда не вносятся:
> их монопольный писатель — `@dispatcher` по построению (AGENTS.md §4.22).
> Кто и что пишет — таблица владельцев в `docs/INDEX.md` §«Владельцы доков».

## Как пользоваться

1. Перед правкой файла, который может править кто-то ещё, — посмотри таблицу ниже: есть метка → **не начинай**, доложи менеджеру (или создателю, если ты менеджер).
2. Начал править — добавь строку `| путь | @кто | коммит/дата |`.
3. Команда сама коммитит свой код (AGENTS.md §4.10) и снимает метку после коммита; горячие доки в реестр не вносятся (их писатель — @dispatcher).
4. **Миграции** — бронь номера через @manager до начала работы (актуальный номер — `docs/DB.md`); два вектора одну миграцию в одном релизе не добавляют (правило 46a §3.1, см. `docs/VECTORS.md`).
5. Живые джобы админки (пишут в БД) — по правилу AGENTS.md §4.24: перед стартом смотреть статус генерации, крутится чужой — ждать.

## Уже есть — НЕ дублировать (предупреждение @manager, 2026-09-21)

Параллельным задачам (в первую очередь «рецепты» 000055 и всему, что строится вокруг
фабрик/производства): эти сущности **уже существуют**, вторую такую же не заводить.

| Сущность | Что это | Где | Владелец/статус |
|---|---|---|---|
| `buildings` | **экземпляры построек** в мире (что стоит на планете), `owner_type`+`owner_id` | `migrations/000056_create_buildings.sql` | ✅ реализовано 2026-09-21, коммит `dfba218` |
| `factions` | NPC-фракции (имя/тип/`homeworld_id`/цвет) | `migrations/004_factions.sql`, `internal/generator/faction/faction.go` | реализовано; в реестре impact map её ещё не было — @manager добавит |
| `producer_types`, `items`, `producer_items`, `producer_slots`, рецепты (`producer_recipes`/`recipes`, 000055) | **справочник «что бывает»** (каталог студии), НЕ объекты в мире | студия `/studio`, БД каталога | параллельная задача рецептов |

**Правило связки:** «что бывает» — каталог студии; «что стоит и у кого» — `buildings`.
Строение цепляется к каталогу через будущую колонку `buildings.producer_type_id`
(отложена до этапа производства — не изобретать вторую связь/вторую таблицу
экземпляров). Привязка капитала: `building_type='capital'`, `owner_type='faction'`,
`owner_id=factions.id`, `planet_id=factions.homeworld_id`.

⚠️ **Не возрождать снесённый легаси:** `factories` (000018 → снесена 000034) и
`production_units` (001_init → снесена 000050) — это мёртвые таблицы, не модель.

⚠️ **Инцидент общего индекса, 2026-09-21.** Артефакты задачи «рецепты»
(`docs/specs/2026-09-21-рецепт-сущность-и-граф-фабрики.md`,
`docs/specs/2026-09-21-студия-товары-рецепты-ui.md`,
`docs/gamedesign/ideas/2026-09-21_фабрика-центральный-граф-товары-под-фабрику.md`,
`docs/QA/checklists/2026-09-21_рецепты-сущность.md`, `docs/QA_CHECKLIST.md`,
`docs/impact_map.json`, `docs/impact_map.md`) уехали в коммит `997d968` вместе
с задачей «столицы фракций» — параллельный агент поставил их в индекс между
проверкой и коммитом. **Файлы целы и закоммичены, переделывать/переносить не
нужно** — просто знайте, что подпись коммита не совпадает с содержимым.
Защита на будущее: коммитить через `git commit -- <пути>` (минует общий индекс).

---

## Занято сейчас

> Активные метки. Исторические (завершённые задачи) — в таблице ниже, **не блокируют**.

| Файл | Кто правит | До какого коммита |
|---|---|---|
| `cmd/art-studio/{postproc,generator,handlers}/*_test.go` (кроссплатформенные фейковые python вместо `.cmd` — helper-процесс) | @developer (краснота CI, вариант 2) | без коммита (гейты) |
| `migrations/000085_producer_sections.sql` — занята, закоммичена (разделы построек, коммит `2b32533`) | @developer | коммит `2b32533` |
| `migrations/000086_*.sql` — **номер забронирован** (внутреннее хранилище: `settlement_storage_cells`, `settlements.storage_size`, снятие `settlement_branch_buffers`); спека `docs/specs/2026-09-25-внутреннее-хранилище-и-рождение-заказов.md` (ЧК2а), миграция ещё не написана | @manager (бронь) | без коммита (дизайн-гейт) |
| `internal/models/economy.go` — занят (ЧК2а, подэтап 2а: тип `StorageCell`); `internal/repository/storage_cell_repository.go` (новый файл, в реестр не нужен) | @developer | без коммита (гейт) |
| ЧК2а подэтап 2б (переезд буферов ветки на ячейки): `migrations/000087_retire_branch_buffers.sql`, `internal/models/economy.go`, `internal/repository/{branch_repository,settlement_owner_pass,settlement_stage,knowledge_repository,content_import,storage_cell_repository,producer_repository}.go`, `internal/economy/settlement/{storage,needs}.go`, `internal/handlers/{admin_universe,admin_settlement_branches,admin_settlements,admin_race_settlements,planet_visibility}.go` (+ тесты) | @developer | без коммита (гейт) |
| ЧК2а подэтап 3а (автор-поселение): `migrations/000088_contract_author_settlement.sql`, `internal/models/contract.go`, `internal/repository/contract_repository.go` (+`contract_repository_test.go`), `internal/integration/contract_author_settlement_test.go`, `docs/{DB,ARCHITECTURE}.md`, `docs/pitfalls/db-shell.md` | @developer | без коммита (гейт) |
| ЧК2а подэтап 3б (доска и свежесть): `internal/repository/{contract_board_repository,planet_repo,settlement_owner_pass}.go` (+тесты), `internal/handlers/contract_handlers.go`, `internal/integration/contract_author_settlement_test.go` | @developer | без коммита (гейт) |
| ЧК2а подэтап UI (минимальный UI нового): `internal/models/economy.go`, `internal/repository/{settlement_owner_pass,planet_repo,producer_repository,branch_testhelpers_test,settlement_owner_cells_test}.go`, `internal/handlers/planet_visibility.go` (+`planet_visibility_arithmetic_test.go`, `planet_visibility_handler_modes_test.go`, `settlement_owner_testhelpers_test.go`, `studio_producer_rate_test.go`), `web/static/js/modal/{branches,tabs}.js`, `web/frontend_branches_test.go` | @developer | без коммита (гейт) |
| ЧК6.2 «Плавание и погружение» (спека `2026-09-25-мир-прогулки-вода`), **сдано на гейт**: `web/static/js/surface/{surface_config,surface_world,surface_player,surface_main,surface_render}.js`, новые `tools/surface-swim-check.mjs`, `tools/e2e/surface-swim-check.js`; доки `docs/{ARCHITECTURE}.md`, `docs/pitfalls/design.md` | @developer | без коммита (гейт) |

## Занято ранее (историческое, не блокирует — чистится @manager по мере надобности)

| Файл | Кто правит | До какого коммита |
|---|---|---|
| `internal/economy/settlement/{stage,arithmetic,units}.go`, `internal/repository/{settlement_owner_pass,settlement_stage,galaxy_population,branch_repository,economy_repository,planet_repo}.go` (+ тесты), `internal/handlers/{admin_settlement_branches,admin_settlement_settings,planet_visibility,planet_handler}.go` (+ тесты), `internal/models/{economy,generation_config}.go` | @developer (И2.1 + И2.2 эпика «Экономика поселения»: механика стадий, витрина арифметики, видимость игроку) | коммиты `25c3c1e`, `588d358` (2026-09-24) |
| `internal/generator/planet/cascade.go` / `planet_data.go` / `planet_data_generate.go` / `accretion_mass_test.go` / `giant_smoke_test.go` | @developer (этап 1 «протопылевое облако», 2026-09-21) | коммит dd37a8c |
| `internal/generator/planet/*` (cascade.go, planet_data.go, planet_data_generate.go, planet_data_belt.go, planet_data_batch.go, belt_test.go), `internal/models/planet.go`, `internal/handlers/admin_universe.go`, `internal/handlers/admin_regenerate_planets.go`, `migrations/000059_system_belts.sql` | @developer (пояса малых тел — этап 1а/1б, 2026-09-21/22) | коммит b24e358 |
| `internal/generator/planet/biome_*.go` (+seed.json, biome_test.go) | @developer (99.2.28) | коммит 99.2.28 |
| `internal/generator/planet/cascade.go` / `classify.go` / `composition_*.go` / `planet_data*.go` / `circumbinary.go` | @developer (99.2.28) | коммит 99.2.28 |
| `internal/models/planet.go` | @developer (99.2.28) | коммит 99.2.28 |
| `internal/repository/planet_repo.go` | @developer (99.2.28) | коммит 99.2.28 |
| `internal/audit/planet/checks_biomes.go` (+test), `rules.go` | @developer (99.2.28) | коммит 99.2.28 |
| `config/biome_catalog.json` | @developer (99.2.28) | коммит 99.2.28 |
| `cmd/server/main.go` (только блок загрузки справочника биомов) | @developer (99.2.28) | коммит 99.2.28 |
| `migrations/000046_intrasystem_flight.sql` | @developer (99.2.27) | коммит 99.2.27 |
| `internal/models/current_position.go` | @developer (99.2.27) | коммит 99.2.27 |
| `internal/models/player_intrasystem_flight.go` | @developer (99.2.27) | коммит 99.2.27 |
| `internal/models/user.go` | @developer (99.2.27) | коммит 99.2.27 |
| `internal/repository/player_intrasystem_flight_repository.go` (+test) | @developer (99.2.27) | коммит 99.2.27 |
| `internal/repository/user_repository.go` | @developer (99.2.27) | коммит 99.2.27 |
| `internal/repository/knowledge_repository.go` | @developer (99.2.27) | коммит 99.2.27 |
| `internal/repository/planet_repo.go` | @developer (99.2.27) | коммит 99.2.27 |
| `internal/travel/intrasystem_manager.go` (+test) | @developer (99.2.27) | коммит 99.2.27 |
| `internal/handlers/intrasystem_handlers.go` (+test) | @developer (99.2.27) | коммит 99.2.27 |
| `internal/handlers/players_positions.go` (+test) | @developer (99.2.27) | коммит 99.2.27 |
| `internal/handlers/travel_handlers.go` (+test) | @developer (99.2.27) | коммит 99.2.27 |
| `internal/handlers/auth_handlers.go` (+test) | @developer (99.2.27) | коммит 99.2.27 |
| `internal/handlers/planet_handler.go` | @developer (99.2.27) | коммит 99.2.27 |
| `internal/handlers/visibility_handlers_test.go` | @developer (99.2.27) | коммит 99.2.27 |
| `cmd/server/main.go` | @developer (99.2.27) | коммит 99.2.27 |
| `web/static/js/modal/*.js` (state/index/events/panel/tabs/modal_render) | @developer (99.2.27) | коммит 99.2.27 |
| `web/static/js/map/map_render.js` | @developer (99.2.27) | коммит 99.2.27 |
| `cmd/server/main.go` | @developer (97a) | коммит 97a |
| `internal/travel/manager.go` | @developer (97a) | коммит 97a |
| `internal/travel/manager_test.go` | @developer (97a) | коммит 97a |
| `internal/models/player_flight.go` | @developer (97a) | коммит 97a |
| `internal/repository/player_flight_repository.go` | @developer (97a) | коммит 97a |
| `internal/repository/player_flight_repository_test.go` | @developer (97a) | коммит 97a |
| `migrations/000044_create_player_flights.sql` | @developer (97a) | коммит 97a |
| `internal/handlers/auth_handlers_test.go` | @developer (97a) | коммит 97a |
| `internal/handlers/travel_handlers_test.go` | @developer (97a) | коммит 97a |
| `internal/handlers/visibility_test.go` | @developer (97a) | коммит 97a |
| `internal/handlers/visibility_handlers_test.go` | @developer (97a) | коммит 97a |
| `docs/DB.md` | @developer (97a) | коммит 97a |
| `docs/ARCHITECTURE.md` | @developer (97a) | коммит 97a |
| `docs/PITFALLS.md` | @developer (97a) | коммит 97a |
| `cmd/art-studio/config/config_types.go` | @developer (98a) | коммит 98a |
| `cmd/art-studio/config/load.go` | @developer (98a) | коммит 98a |
| `cmd/art-studio/config/load_test.go` | @developer (98a) | коммит 98a |
| `cmd/art-studio/generator/prompt.go` | @developer (98a) | коммит 98a |
| `cmd/art-studio/generator/prompt_test.go` | @developer (98a) | коммит 98a |
| `config/art/families.json` | @developer (98a) | коммит 98a |
| `docs/gamedesign/races/coastal.md` | @developer (98a) | коммит 98a |
| `docs/gamedesign/races/methane_plankton.md` | @developer (98a) | коммит 98a |
| `docs/gamedesign/races/deep_dwellers.md` | @developer (98a) | коммит 98a |
| `docs/gamedesign/races/cryo_swarms.md` | @developer (98a) | коммит 98a |
| `docs/gamedesign/races/mist_swarms.md` | @developer (98a) | коммит 98a |
| `docs/gamedesign/races/saltfolk.md` | @developer (98a) | коммит 98a |
| `cmd/art-studio/handlers/race_handlers.go` | @developer (98b) | коммит 98b |
| `cmd/art-studio/handlers/http.go` | @developer (98b) | коммит 98b |
| `cmd/art-studio/handlers/prompt_handlers_test.go` | @developer (98b) | коммит 98b |
| `cmd/art-studio/generator/race_job.go` | @developer (98b) | коммит 98b |
| `cmd/art-studio/generator/ref_job.go` | @developer (98b) | коммит 98b |
| `cmd/art-studio/web/index.html` | @developer (98b) | коммит 98b |
| `cmd/art-studio/handlers/race_handlers_test.go` | @developer (98b) | коммит 98b |
| `cmd/art-studio/config/ships.go` / `ship_dict.go` / `ship_section.go` / `ships_test.go` | @developer (ships-races-generator) | коммит ships-races-generator |
| `cmd/art-studio/generator/ship_prompt.go` / `ships_job.go` / `ship_prompt_test.go` / `ships_job_test.go` / `ships_pixel_test.go` | @developer (ships-races-generator) | коммит ships-races-generator |
| `cmd/art-studio/comfy/ship_workflow.go` / `ship_workflow_test.go` | @developer (ships-races-generator) | коммит ships-races-generator |
| `cmd/art-studio/handlers/ships_handlers.go` / `ships_patch.go` / `ships_rebuild.go` / `ships_handlers_test.go` | @developer (ships-races-generator) | коммит ships-races-generator |
| `cmd/art-studio/postproc/ship.go` | @developer (ships-races-generator) | коммит ships-races-generator |
| `cmd/art-studio/main.go` / `web/index.html` / `generator/worker.go` / `handlers/http.go` / `handlers/race_info.go` | @developer (ships-races-generator) | коммит ships-races-generator |
| `config/art/ships.json` / `config/art/ship_dict.json` | @developer (ships-races-generator) | коммит ships-races-generator |
| `tools/make_ship_silhouettes.py` | @developer (ships-races-generator) | коммит ships-races-generator |
| `tools/process_ship.py` | @developer (ships-races-generator, 98c: --canvas эскиз) | коммит ships-races-generator |
| `cmd/art-studio/web/index.html` (loadShipsList cache-busting) | @developer (баг превью-сетки кораблей) | коммит бага превью-сетки |
| `cmd/art-studio/config/config_types.go` (KnownCheckpoints) | @developer (селект чекпоинта SDXL) | коммит селекта чекпоинта |
| `cmd/art-studio/generator/worker.go` (SetCheckpoint/GetCheckpoint/checkpoint) | @developer (селект чекпоинта SDXL) | коммит селекта чекпоинта |
| `cmd/art-studio/generator/{race_job,ref_job,human_job,ships_job}.go` (checkpoint()) | @developer (селект чекпоинта SDXL) | коммит селекта чекпоинта |
| `cmd/art-studio/handlers/http.go` (/checkpoint) | @developer (селект чекпоинта SDXL) | коммит селекта чекпоинта |
| `cmd/art-studio/handlers/checkpoint_handlers_test.go` | @developer (селект чекпоинта SDXL) | коммит селекта чекпоинта |
| `cmd/art-studio/generator/checkpoint_test.go` | @developer (селект чекпоинта SDXL) | коммит селекта чекпоинта |
| `cmd/art-studio/web/index.html` (селект «Модель») | @developer (селект чекпоинта SDXL) | коммит селекта чекпоинта |
| `tools/make_ship_silhouettes.py` (выразительные формы 13 шт) | @developer (формы силуэтов) | коммит форм силуэтов |
| `cmd/art-studio/generator/ships_silhouette_forms_test.go` | @developer (формы силуэтов) | коммит форм силуэтов |
| `docs/PITFALLS.md` (запись про null-модули скрипта силуэтов) | @developer (формы силуэтов) | коммит форм силуэтов |
| `docs/gamedesign/11_production.md`, `13_tiers.md`, `13_settlements.md`, `10_exploration.md`, `05_economy.md`, `04_composition.md`, `09_resources.md`, `19_robots.md`, `01_concept.md`, `06_factions.md`, `07_ui.md`, `11_contracts.md`, `docs/GLOSSARY.md` | @designer (спека «Фабрика» §8 — GDD-правки) | коммит фабрики-§8 |
| `migrations/000049_add_users_pending_destination.sql` | @developer (99.2.30) | коммит 99.2.30 |
| `internal/models/pending_destination.go` (+user.go) | @developer (99.2.30) | коммит 99.2.30 |
| `internal/repository/player_intrasystem_flight_repository.go` (+test) | @developer (99.2.30) | коммит 99.2.30 |
| `internal/repository/user_repository.go` | @developer (99.2.30) | коммит 99.2.30 |
| `internal/handlers/travel_handlers.go` (+test) | @developer (99.2.30) | коммит 99.2.30 |
| `internal/handlers/auth_handlers.go` (+test) | @developer (99.2.30) | коммит 99.2.30 |
| `internal/handlers/admin_pacman.go` (+test) | @developer (99.2.30) | коммит 99.2.30 |
| `cmd/server/main.go` (блок Restore намерений) | @developer (99.2.30) | коммит 99.2.30 |
| `web/static/js/map/flight.js` / `data.js` / `config.js` / `animation.js` | @developer (99.2.30) | коммит 99.2.30 |
| `web/static/js/modal/panel.js` / `events.js` / `index.js` / `tabs.js` | @developer (99.2.30) | коммит 99.2.30 |
| `docs/DB.md` / `docs/ARCHITECTURE.md` / `docs/PITFALLS.md` (99.2.30) | @developer (99.2.30) | коммит 99.2.30 |
| `internal/generator/planet/planet_data*.go` / `circumbinary.go` / `region_profile_test.go` / `exotic_test.go` / `circumbinary_test.go` / `cascade_test.go` / `giant_distribution_test.go` (новый) | @developer (реализм распределения 2026-09-20) | коммит реализма распределения |
| `migrations/000051_producer_tree.sql` (новый) | @developer (дерево построек 2026-09-21) | коммит дерева построек |
| `internal/goodsstudio/seed_producers.go` (+test) | @developer (дерево построек 2026-09-21) | коммит дерева построек |
| `internal/repository/producer_repository.go` (+test) | @developer (дерево построек 2026-09-21) | коммит дерева построек |
| `internal/handlers/studio_handlers.go` (+tests) | @developer (дерево построек 2026-09-21) | коммит дерева построек |
| `cmd/server/main.go` (роут `/studio/api/races`) | @developer (дерево построек 2026-09-21) | коммит дерева построек |
| `web/studio.html` (дерево построек на канвасе) | @developer (дерево построек 2026-09-21) | коммит дерева построек |
| `docs/DB.md` / `docs/ARCHITECTURE.md` / `docs/PITFALLS.md` (дерево построек) | @developer (дерево построек 2026-09-21) | коммит дерева построек |
| `internal/models/ship_sprites.go` (+test) | @developer (проба расовых кораблей людей 2026-09-21) | коммит пробы |
| `internal/handlers/auth_handlers_test.go` | @developer (проба расовых кораблей людей 2026-09-21) | коммит пробы |
| `web/static/sprites/race_humans_*.png` | @developer (проба расовых кораблей людей 2026-09-21) | коммит пробы |
| `cmd/art-studio/handlers/ships_edit.go` (+`ships_handlers.go`, `ships_handlers_test.go`) | @developer (корабли рас: полный кадр + нос вправо 2026-09-21) | до коммита задачи |
| `cmd/art-studio/web/index.html` (кнопки ориентации «Корабли рас») | @developer (корабли рас: полный кадр + нос вправо 2026-09-21) | до коммита задачи |
| `tools/spike_ship_sprite_run.py`, `spike_ship_sprite_cut.py`, `spike_ship_sprite_bis.py` | @developer (корабли рас: полный кадр + нос вправо 2026-09-21) | до коммита задачи |
| `tools/e2e/ships-accept-check.js` (+ артефакты) | @developer (корабли рас: инструмент ручной приёмки 2026-09-21) | до коммита задачи |
| `cmd/art-studio/generator/ships_job.go` / `ship_prompt.go` / `worker.go` / `ship_filter.go` (+тесты) | @developer (перенос рецепта кораблей в студию 2026-09-21) | до коммита задачи |
| `cmd/art-studio/comfy/ship_workflow.go` (+тест), `postproc/ship.go` | @developer (перенос рецепта кораблей в студию 2026-09-21) | до коммита задачи |
| `cmd/art-studio/config/config_types.go` / `load_test.go` / `config/art/studio.json` (блок ships) | @developer (перенос рецепта кораблей в студию 2026-09-21) | до коммита задачи |
| `cmd/art-studio/handlers/ships_handlers.go` (+тест) | @developer (перенос рецепта кораблей в студию 2026-09-21) | до коммита задачи |
| `cmd/art-studio/web/index.html` (вкладка «Корабли рас»: txt2img + Hi-Res) | @developer (перенос рецепта кораблей в студию 2026-09-21) | до коммита задачи |
| `tools/ship_sprite_cut.py` (новый, промоут спайка) | @developer (перенос рецепта кораблей в студию 2026-09-21) | до коммита задачи |
| `docs/PITFALLS.md` (записи о ships.model и промоуте скрипта) | @developer (перенос рецепта кораблей в студию 2026-09-21) | до коммита задачи |

---

*Пишется всеми по ролям (один писатель у каждого документа — AGENTS.md §4.23, владельцы — `docs/INDEX.md`).*