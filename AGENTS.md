# AGENTS.md — единственная точка входа для агентов

> Подгружается автоматически. Дополнительно на старте нужен только `STATUS.md` —
> больше ничего не читай, пока не потребует задача. Карта всех доков —
> `docs/INDEX.md` (этот файл не дублирует её). Правило доков: **один факт —
> одно место.** Начало сессии: команда `/start` (`.opencode/command/start.md`).

**Обновлено:** 2026-09-13
**Module path:** `zorion`
**Remote:** `github.com/lersss/Zorion2`

---

## 0. Главное правило: игра многопользовательская

Несколько игроков работают на одном сервере одновременно. Это влияет на **любую** правку.

- Любая `map`, доступная из двух горутин — под `sync.Mutex` / `RWMutex`.
  Go **убивает процесс** на `concurrent map writes`, `recover` не спасает.
- Общие `*rand.Rand` не потокобезопасны — локальный `rand.New(...)` на вызов.
- Проверка + действие атомарно (`TryStart`, не `if running { Start() }`).
- `gorilla/websocket` — только одна горутина на запись в соединение.

Что защищено, что нет — `docs/ARCHITECTURE.md` §0.

---

## 1. Стек и запуск

Go 1.21+ · PostgreSQL 15+ · Redis 7+ · Vanilla JS (ES-модули) + Canvas 2D · JWT HS256 · gorilla/websocket · testify · go-sqlmock (dev-тесты БД) · Docker/Amvera.

**Локально Go есть** (проверено: `go1.27.0 windows/amd64`). PostgreSQL в `C:\pgsql\pgsql\bin`, слушает `127.0.0.1:5432`. Redis на `6379`.

```powershell
$env:DATABASE_URL = "postgres://zorion:zorion123@127.0.0.1:5432/zorion?sslmode=disable"
$env:REDIS_URL    = "redis://localhost:6379"
$env:JWT_SECRET   = "минимум-32-символа"
$env:ADMIN_PASSWORD = "admin123"
$env:TICK_INTERVAL_SEC = "3"
go run cmd/server/main.go
```

`JWT_SECRET` обязателен — без него `log.Fatal`.

---

## 2. Команды и definition of done

Правка **не считается готовой**, пока не прошли все три:

```powershell
go build ./...
go vet ./...
go test -race ./...
```

`-race` — основной инструмент против риска из раздела 0. Гоняй его, а не рассуждай о гонках.

> **Локальное исключение (до версии 1.0):** `-race` требует cgo, а в PATH нет gcc
> (`CGO_ENABLED=0`). Локально DoD = `go build` + `go vet` + `go test ./...`;
> `-race` вернётся вместе с CI после 1.0. Решение игрока 2026-09-12, см. `STATUS.md` §4.

Сборка бинаря: `go build -o zorion-server.exe cmd/server/main.go`

---

## 3. Структура проекта

```
cmd/server/main.go       — точка входа
config/                  — archetypes, compatibility, anomalies, descriptions
internal/
  auth/                  — JWT + middleware
  config/                — env
  generator/galaxy/      — генерация галактики (Poisson disk)
  generator/planet/      — планеты, композиция, физика, ядро, описания
  generator/faction/     — фракции
  audit/  audit/planet/  — движок Run[T] + 43 правила
  economy/settlement/    — пересчёт населения (лениво, без тика)
  handlers/              — HTTP
  models/  repository/   — модели и доступ к БД
  resource/              — ресурсы
  names/  travel/        — имена, полёты
  mapcache/              — карта из снапшота в памяти
  core/                  — черновики, не подключены
migrations/              — SQL (см. docs/DB.md)
pkg/                     — черновики (simulation, worldgen)
web/                     — HTML + static/{css,js,sprites}
  static/js/map/         — карта (Canvas, кластеры)
  static/js/modal/       — модалка системы
  static/js/admin/       — админка
```

Назначение каждого пакета — в его `doc.go`. Карта файлов кода — `docs/ARCHITECTURE.md` §2. БД и миграции — `docs/DB.md`.

---

## 4. Правила работы

1. **Баги приоритетнее фич.**
2. **Правки файлов — только по явному «го».**
   Перед любой правкой — короткое предложение: что меняю, зачем, что затрону. Ждёшь ответа. «Го» — это явное подтверждение именно этого предложения.
   - Директива («сделай X»), вопрос, описание проблемы, молчание — не «го» на детали.
   - Один «го» покрывает только предложенный объём; всё сверх него — снова предложение и «го».
   - **Сомневаешься, есть ли «го», — его нет.**
   - **Не понимаешь, зачем это делаешь, — не делай, а спроси.**
   - **Есть решение лучше — предложи его, хотя бы один раз.**
3. **Один вопрос за раз.**
4. **Следовать стилю соседних файлов**, использовать существующие библиотеки.
5. **Комментарии только по запросу.**
6. **Не коммитить секреты, не логировать ключи.**
7. **Одна правка — один коммит.** Стейджить явными путями, **никогда `git add -A`**.
8. **Файл > 200–300 строк — разделить** на логические.
9. **Имена файлов в разных папках не должны совпадать** (исключения: `main.go`, `go.mod`, `index.html`). Известная коллизия: `web/static/js/config.js` и `web/static/js/map/config.js`.
10. **Над проектом могут работать несколько агентов.** Перед правкой перечитывай файл. Git ведёт только один агент.
11. **Как писать код — `docs/AGENT_RULES.md`** (подгружается автоматически, открывать не нужно).
12. **TDD: на каждую фичу — автоматический тест** (допустимо написать до кода): до фичи красный, после — зелёный.
13. **Сервер перезапускать самому.** После правок рантайма (Go, миграции) — перезапустить dev-сервер и проверить `/health`. Статика отдаётся с диска с `noCache` — рестарт не нужен.

---

## 5. API — группы маршрутов

| Группа | Авторизация | Примеры |
|---|---|---|
| Публичные | — | `/health`, `/status`, `/register`, `/login`, `/api/planet-image` |
| Игровые | `Authorization: Bearer <JWT>` | `/me`, `/worlds`, `/worlds/{id}`, `/api/worlds/{id}/planets`, `/api/worlds/filter`, `/api/entities/search`, `/api/contracts`, `/travel`, `/ws` |
| Админка | `X-Admin-Password` | `/admin/worlds`, `/admin/generate*`, `/admin/clear`, `/admin/stats`, `/admin/audit`, `/admin/compatibility`, `/admin/tests` |

Точные пути смотри в `internal/handlers/`.

---

## 6. Доки — карта в `docs/INDEX.md`

- `STATUS.md` — всегда вместе с этим файлом.
- Остальные — только по требованию через `docs/INDEX.md` (один нужный файл, а не всё подряд).
- БД и миграции — `docs/DB.md`. Ловушки проекта — `docs/PITFALLS.md`.
- `CHANGELOG.md` — **агентам не нужен**, только при релизе.