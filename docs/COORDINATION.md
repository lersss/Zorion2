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

---

*Пишется всеми по ролям (один писатель у каждого документа — AGENTS.md §4.23, владельцы — `docs/INDEX.md`).*