# AGENTS.md — единственная точка входа для агентов

> Этот файл opencode подгружает в контекст автоматически. Дополнительно на старте
> нужен только `STATUS.md`. Больше ничего не читай, пока не потребует задача.
> Остальные доки — по требованию, список в разделе 7.
> Начало сессии: команда `/start` (`.opencode/command/start.md`).
>
> Правило доков: **один факт — одно место.** Не дублируй сюда то, что есть в другом файле.

**Обновлено:** 2026-09-11
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

Go 1.21+ · PostgreSQL 15+ · Redis 7+ · Vanilla JS (ES-модули) + Canvas 2D · JWT HS256 · gorilla/websocket · testify · Docker/Amvera.

**Локально Go есть** (проверено: `go1.27.0 windows/amd64`). PostgreSQL в `C:\pgsql\pgsql\bin`, слушает `127.0.0.1:5432`. Redis на `6379`.

```powershell
$env:DATABASE_URL = "postgres://zorion:zorion123@127.0.0.1:5432/zorion?sslmode=disable"
$env:REDIS_URL    = "redis://localhost:6379"
$env:JWT_SECRET   = "минимум-32-символа"
$env:ADMIN_PASSWORD = "надёжный-пароль"
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

Сборка бинаря: `go build -o zorion-server.exe cmd/server/main.go`

---

## 3. Структура проекта

```
cmd/server/main.go       — точка входа
config/                  — archetypes, compatibility, anomalies (22), descriptions (11 типов)
internal/
  auth/                  — JWT + middleware
  config/                — env
  generator/galaxy/      — генерация галактики (Poisson disk)
  generator/planet/      — планеты, композиция, физика, ядро, описания
  generator/faction/     — фракции
  audit/  audit/planet/  — движок Run[T] + 43 правила
  handlers/              — HTTP
  models/  repository/   — модели и доступ к БД
  resource/              — ресурсы (6 категорий, 9 свойств)
  names/  travel/        — имена, полёты
  core/                  — черновики, не подключены
migrations/              — SQL + неподключённый раннер (см. §5)
pkg/                     — черновики (simulation/batch, worldgen)
web/                     — HTML + static/{css,js,sprites}
  static/js/map/         — карта (Canvas, кластеры)
  static/js/modal/       — модалка системы
  static/js/admin/       — админка
```

Полная карта файлов — `docs/ARCHITECTURE.md` §2.

---

## 4. Правила работы

1. **Баги приоритетнее фич.**
2. **Не менять файлы без подтверждения.** Предложить → дождаться «го» → писать.
3. **Один вопрос за раз.**
4. **Следовать стилю соседних файлов**, использовать существующие библиотеки.
5. **Комментарии только по запросу.**
6. **Не коммитить секреты, не логировать ключи.**
7. **Одна правка — один коммит.** Стейджить явными путями, **никогда `git add -A`**.
8. **Файл > 200–300 строк — разделить** на логические.
9. **Имена файлов в разных папках не должны совпадать** (исключения: `main.go`, `go.mod`, `index.html`). Известная коллизия: `web/static/js/config.js` и `web/static/js/map/config.js`.
10. **Над проектом могут работать несколько агентов.** Перед правкой перечитывай файл — твоё представление о дереве устаревает. Git ведёт только один агент.
11. **Думать до кода.** Предположения проговаривать явно в ответе, а не закапывать в реализацию.
12. **Минимально жизнеспособное решение.** Никаких спекулятивных функций «про запас».
13. **Хирургические изменения.** Не трогать код, которого задача не касается.
14. **Критерий успеха — до начала работы.** Сформулировать проверяемый критерий и итерировать, пока он не выполнен.

---

## 5. БД и миграции

**Состояние (2026-09-11):** 7 users, 100 000 worlds, 319 573 planets, 1389 МБ.

Таблицы: `worlds`, `planets` (JSONB `data`), `locations`, `users`, `assignments`, `factions`, `events`, `production_units`, `settlements`, `factories`, `goods_batches`, `planet_resources`, `compatibility_matrix`.

**Миграции 001–011 применены** (009 и 011 проверены в БД напрямую 2026-09-11), кроме `005_economy_tables.sql` — пропущена, пустая.

**Открытое решение:** `migrations/migrations.go` содержит `Apply(db)`, но `cmd/server/main.go` его **не вызывает** — автоприменение написано и не подключено. Пока миграции применяются руками через psql/DBeaver. Решить: подключать или убрать.

---

## 6. API — группы маршрутов

| Группа | Авторизация | Примеры |
|---|---|---|
| Публичные | — | `/health`, `/status`, `/register`, `/login`, `/api/planet-image` |
| Игровые | `Authorization: Bearer <JWT>` | `/me`, `/worlds`, `/worlds/{id}`, `/api/worlds/{id}/planets`, `/api/worlds/filter`, `/api/contracts`, `/travel`, `/ws` |
| Админка | `X-Admin-Password` | `/admin/worlds`, `/admin/generate*`, `/admin/clear`, `/admin/stats`, `/admin/audit`, `/admin/compatibility` |

Точные пути смотри в `internal/handlers/`.

---

## 7. Доки по требованию

Открывай **только если задача о них**:

| Файл | Когда |
|---|---|
| `STATUS.md` | Всегда вместе с этим файлом |
| `docs/ARCHITECTURE.md` | Конкурентность, карта кода, разбор инцидентов |
| `docs/gamedesign/02_worlds.md` | Генерация миров, спектры |
| `docs/gamedesign/03_planets.md` | Архетипы, масса, ядро, температура, 11 типов |
| `docs/gamedesign/04_composition.md` | 16 форм, 17 типов недр, спутники, матрица |
| `docs/gamedesign/05_economy.md` | Ресурсы, товары, заводы, энергия |
| `docs/gamedesign/06_factions.md` | Фракции, войны, контракты |
| `docs/gamedesign/07_ui.md` | Карта, карточка планеты, админка |
| `docs/gamedesign/08_roadmap.md` | Долгосрочный бэклог |
| `docs/DESCRIPTIONS_WORK.md` | Описания планет, список тегов |
| `CHANGELOG.md` | **Агентам не нужен.** Только при релизе |

---

## 8. Ловушки проекта (проверено на практике)

- **`TRUNCATE ... CASCADE` запрещён.** Работает на уровне таблиц, а не строк, и игнорирует `ON DELETE SET NULL`. Однажды снёс таблицу `users` с паролями целиком. Только `TRUNCATE` без CASCADE + явный список + временное снятие FK. См. `internal/handlers/admin_universe.go`, `clearUniverseTx`, разбор в `ARCHITECTURE.md` §4.1.
- **`/admin/stats` кэшируется.** Отдал 318 257 планет при 319 573 в БД. Для точных цифр — SQL, не эндпоинт.
- **У `worlds` единственный индекс `worlds_pkey`.** Фильтр карты идёт `Seq Scan` по 100k строк, 95% отбрасывается. Держится на page cache. См. `STATUS.md` B7.
- **`config/descriptions/README.md` содержит чужой текст** — описывает формат `config/anomalies/`. Не ориентируйся на него.
- **`.env` есть в истории git** (добавлен `5253b8c`, удалён `44e3cfc`). Содержит дев-креды, включая `admin123`.
- **`internal/core/`, `pkg/`** — черновики, не подключены. Не трогай без причины.
