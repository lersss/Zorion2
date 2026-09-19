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
| `cmd/goods-studio/web/index.html` | @developer (деревья Т8) | коммит деревьев Т8 |
| `cmd/goods-studio/catalog/trees.go` | @developer (деревья Т8) | коммит деревьев Т8 |
| `cmd/goods-studio/catalog/trees_test.go` | @developer (деревья Т8) | коммит деревьев Т8 |
| `cmd/goods-studio/catalog/topcatalog.go` | @developer (деревья Т8) | коммит деревьев Т8 |
| `cmd/goods-studio/handlers/state.go` | @developer (деревья Т8) | коммит деревьев Т8 |
| `cmd/goods-studio/handlers/handlers_test.go` | @developer (деревья Т8) | коммит деревьев Т8 |
| `docs/ARCHITECTURE.md` | @developer (деревья Т8) | коммит деревьев Т8 |
| `cmd/goods-studio/ai/apply.go` | @developer (99a.1) | коммит 99a.1 |
| `cmd/goods-studio/ai/prompt.go` | @developer (99a.1) | коммит 99a.1 |
| `cmd/goods-studio/ai/client.go` | @developer (99a.1) | коммит 99a.1 |
| `cmd/goods-studio/ai/ai_test.go` | @developer (99a.1) | коммит 99a.1 |
| `cmd/goods-studio/handlers/state.go` | @developer (99a.1) | коммит 99a.1 |
| `tools/e2e/goods-studio-check.js` | @developer (99a.1) | коммит 99a.1 |
| `cmd/goods-studio/model/types.go` | @developer (99a.3) | коммит 99a.3 |
| `cmd/goods-studio/graph/tier.go` | @developer (99a.3) | коммит 99a.3 |
| `cmd/goods-studio/graph/tier_test.go` | @developer (99a.3) | коммит 99a.3 |
| `cmd/goods-studio/handlers/state.go` | @developer (99a.3) | коммит 99a.3 |
| `cmd/goods-studio/handlers/handlers_test.go` | @developer (99a.3) | коммит 99a.3 |
| `cmd/goods-studio/export/export.go` | @developer (99a.3) | коммит 99a.3 |
| `cmd/goods-studio/export/export_test.go` | @developer (99a.3) | коммит 99a.3 |
| `cmd/goods-studio/web/index.html` | @developer (99a.3) | коммит 99a.3 |
| `tools/e2e/qa99a3-smoke.js` | @developer (99a.3) | коммит 99a.3 |
| `docs/ARCHITECTURE.md` | @developer (99a) | коммит 99a |
| `docs/PITFALLS.md` | @developer (99a) | коммит 99a |
| `.gitignore` | @developer (99a) | коммит 99a |
| `migrations/000045_create_goods_catalog.sql` | @developer (перенос студии A) | коммит iterA |
| `internal/goodsstudio/` (model/graph/validate/seed) | @developer (перенос студии A) | коммит iterA |
| `internal/repository/goods_repository.go` (+test) | @developer (перенос студии A) | коммит iterA |
| `internal/handlers/studio_handlers.go` (+test) | @developer (перенос студии A) | коммит iterA |
| `cmd/server/main.go` | @developer (перенос студии A) | коммит iterA |
| `cmd/goods-studio/*` (импорты на internal/goodsstudio) | @developer (перенос студии A) | коммит iterA |
| `web/studio.html` | @developer (перенос студии A) | коммит iterA |
| `docs/DB.md` | @developer (перенос студии A) | коммит iterA |
| `docs/ARCHITECTURE.md` | @developer (перенос студии A) | коммит iterA |
| `docs/PITFALLS.md` | @developer (перенос студии A) | коммит iterA |

---

*Пишется всеми по ролям (один писатель у каждого документа — AGENTS.md §4.23, владельцы — `docs/INDEX.md`).*