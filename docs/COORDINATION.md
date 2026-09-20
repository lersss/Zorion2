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

## Занято сейчас

| Файл | Кто правит | До какого коммита |
|---|---|---|
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

---

*Пишется всеми по ролям (один писатель у каждого документа — AGENTS.md §4.23, владельцы — `docs/INDEX.md`).*