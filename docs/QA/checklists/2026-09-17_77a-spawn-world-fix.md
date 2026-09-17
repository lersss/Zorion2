# QA-прогон: баг-фикс 77a «стартовый мир для нового игрока» (000041)

**Дата:** 2026-09-17
**Тестировщик:** @tester
**Вектор:** Игрок (баг 🔴, найден создателем после сдачи 77a)

## Контекст

Новый игрок при регистрации получал `current_world_id = NULL` → не видел карту
(видимость 77a требует позицию). Фикс: `PickSpawnWorld` в `world_repository.go`
(мир с поселением humans ближайший к центру; фолбэк — ближайший вообще; миров
нет — NULL), вызов в `Register` до INSERT, бэкфилл-миграция 000041 для
существующих игроков (только NULL, admin/skycomposer не тронуты).

## Прогнал

- `go build ./...` — OK
- `go vet ./...` — OK
- `go test ./...` — OK (все пакеты зелёные)
- `/health` — 200 OK
- Сервер: `go run` (PID 4540, старт 21:59:03) — ПОСЛЕ правок (21:46–21:47), B-DB-5 соблюдён
- БД (psql): миграция 000041 в `schema_migrations` (верх), `role='player' AND current_world_id IS NULL` = 0

## Сценарии

| Кейс | Что проверено | Результат |
|---|---|---|
| Миграция 000041 | В `schema_migrations` (000041 — верх); 58 игроков, 0 с NULL; 18 admin/skycomposer, 8 с NULL — не тронуты (И7) | ✅ |
| Бэкфилл | NULL-игроки получили Benyolob (ближайший к центру мир с humans, dist2 752000875); не-NULL не перезаписаны (qa77a_18575 на Yelal — фолбэк, назначен до фикса; tessssssst на Epog) | ✅ |
| Регистрация (живая) | POST /register → 201, токен; `/me`: `current_world_id=52defc84` (Benyolob, has_humans=t), `ship_model_id=starter`, `equipment={radar:radar_1,scanner:scanner_1,engine:null}`, `radar_radius=400` | ✅ |
| Гибрид видимости (77a §5.1) | `/api/worlds/filter` для нового игрока: в радиусе 400 — 2 полных кластера (Benyolob, Extenis — cnt/sid/sname/sspec), за радаром — 9 точек-огоньков (только cx/cy/x/y, И11) | ✅ |
| Полёт из стартового мира | POST /travel → 202, `from=52defc84` (Benyolob), `start_x/y` = координаты Benyolob, duration 75; прибытие → current_world_id = Extenis (тоже humans) | ✅ |
| И7 (admin/skycomposer) | Статически: `applyPlayerVisibility` только для `role=player` (filter_worlds_handler.go:164); автотест `TestFilterWorldsAdminSeesAll` (visibility_handlers_test.go:151) | ✅ |
| И11 (за-радарная звезда) | Точки-огоньки без sname/sspec/stemp — только координаты (живой ответ) | ✅ |
| Юнит-тесты | `TestPickSpawnWorldHumansPreferred/FallbackClosest/NoWorlds` (world_repository_test.go), `TestRegisterAssignsSpawnWorld/NoWorlds/FallbackClosestWorld` (auth_handlers_test.go:254-294) | ✅ |
| docs/DB.md | 000041 описана (строка 99-105) | ✅ |

## Вердикт

**ПРОЙДЕНО.** Все пункты ТЗ (1–5) — ок. Оговорок нет.

## Предложение в чек-лист

- Новый кейс (зона auth): «Регистрация нового игрока → current_world_id назначен
  (мир с humans или фолбэк), стартовая комплектация starter/radar_1/scanner_1,
  карта видна (гибрид: полные кластеры в радиусе + точки-огоньки за ним)» —
  регрессионный на баг 77a. Кандидат в Ядро (поломка = нельзя играть).
- Кейс B-AUTH-4 дополнить: «/register при отсутствии миров в БД → 201, current_world_id=NULL, без падения».