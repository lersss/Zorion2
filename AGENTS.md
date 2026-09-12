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

Go 1.21+ · PostgreSQL 15+ · Redis 7+ · Vanilla JS (ES-модули) + Canvas 2D · JWT HS256 · gorilla/websocket · testify · go-sqlmock (dev-тесты БД) · Docker/Amvera.

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

> **Локальное исключение (до версии 1.0):** `-race` требует cgo, а в PATH нет gcc
> (`CGO_ENABLED=0`). CI (единственное место, где `-race` реально гоняется) отложен.
> Поэтому локально DoD = `go build` + `go vet` + `go test ./...`; `-race` вернётся
> вместе с CI после 1.0. Решение игрока 2026-09-12, см. `STATUS.md` §4.

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
11. **Как писать код — `docs/AGENT_RULES.md`:** думать до кода, минимальное решение, хирургические правки, работа от критерия успеха. Файл подгружается автоматически через `opencode.json`, открывать вручную не нужно.
12. **Test-driven development: на каждую фичу — автоматический тест.** Тест пишется вместе с фичей (допустимо и раньше кода): до фичи красный, после — зелёный.

---

## 5. БД и миграции

**Состояние (2026-09-12):** 20 users, **1 world, 1 planet** — вселенная очищена,
в базе только пресет `prototype` (`web/static/js/admin/generation.js`) для
прототипа поселения. Прежние 100 000 миров и 318 701 планета снесены.
Проектные масштабы для расчётов нагрузки: 100k миров, ~320k планет.

Таблицы: `worlds`, `planets` (JSONB `data`), `locations`, `users`, `assignments`, `factions`, `events`, `production_units`, `settlements`, `factories`, `goods_batches`, `planet_resources`, `compatibility_matrix`, `regions`.

**Миграции 001–015 применены** (000014 — индексы `LOWER(name)`; 000015 — таблица `regions` для карты), кроме `005_economy_tables.sql` — пропущена, пустая; таблицы экономики (`settlements`, `factories`, `goods_batches`) создаёт `000018_create_economy_tables.sql` (были только вручную в dev, на чистой БД их не хватало). **000008 применена частично:** `idx_planets_world_id` в БД есть, GIN `idx_planets_data` — нет.

**Миграции применяются автоматически** при старте: `cmd/server/main.go` вызывает `migrations.Apply(db)`, учёт в таблице `schema_migrations`. Руками накатывать больше не нужно.

Если учёта в базе нет, а схема уже есть, `Apply` делает **baseline**: отмечает все файлы применёнными, ничего не выполняя. Иначе накат упал бы — `003`, `006`, `000009` и `000011` не идемпотентны. Сработало один раз на рабочей БД 2026-09-11.

---

## 6. API — группы маршрутов

| Группа | Авторизация | Примеры |
|---|---|---|
| Публичные | — | `/health`, `/status`, `/register`, `/login`, `/api/planet-image` |
| Игровые | `Authorization: Bearer <JWT>` | `/me`, `/worlds`, `/worlds/{id}`, `/api/worlds/{id}/planets`, `/api/worlds/filter`, `/api/entities/search`, `/api/contracts`, `/travel`, `/ws` |
| Админка | `X-Admin-Password` | `/admin/worlds`, `/admin/generate*`, `/admin/clear`, `/admin/stats`, `/admin/audit`, `/admin/compatibility`, `/admin/tests` |

Точные пути смотри в `internal/handlers/`.

---

## 7. Доки по требованию

Открывай **только если задача о них**:

| Файл | Когда |
|---|---|
| `STATUS.md` | Всегда вместе с этим файлом |
| `docs/AGENT_RULES.md` | **Открывать не нужно** — подгружается автоматически (`opencode.json`) |
| `docs/ARCHITECTURE.md` | Конкурентность, карта кода, разбор инцидентов |
| `docs/gamedesign/02_worlds.md` | Генерация миров, спектры |
| `docs/gamedesign/03_planets.md` | Архетипы, масса, ядро, температура, 11 типов |
| `docs/gamedesign/04_composition.md` | 16 форм, 17 типов недр, спутники, матрица |
| `docs/gamedesign/05_economy.md` | **Точка входа в экономику:** каркас, товары, качество, телеметрия, статус, открытые вопросы |
| `docs/gamedesign/06_factions.md` | Фракции, войны, контракты |
| `docs/gamedesign/07_ui.md` | Карта, карточка планеты, админка |
| `docs/gamedesign/09_resources.md` | Вещества, свойства, категории, имена, география веществ |
| `docs/gamedesign/10_exploration.md` | Месторождения и истощение, разведка и добыча, мини-игры, информация и связь |
| `docs/gamedesign/11_markets.md` | Заводы, энергия, содержание построек, биржа, контракты, охрана |
| `docs/gamedesign/12_society.md` | Роли игроков, владение, война, упадок |
| `docs/gamedesign/13_settlements.md` | Поселения, труд, потребности, стабильность, гибель |
| `docs/gamedesign/14_money.md` | Деньги: эмиссия, приватность балансов, расчёты |
| `docs/gamedesign/15_monetization.md` | Реальные деньги: подряды, ввод и вывод |
| `docs/gamedesign/99_roadmap.md` | Долгосрочный бэклог |
| `docs/DESCRIPTIONS_WORK.md` | Описания планет, список тегов |
| `CHANGELOG.md` | **Агентам не нужен.** Только при релизе |

---

## 8. Ловушки проекта (проверено на практике)

- **`TRUNCATE ... CASCADE` запрещён.** Работает на уровне таблиц, а не строк, и игнорирует `ON DELETE SET NULL`. Однажды снёс таблицу `users` с паролями целиком. Только `TRUNCATE` без CASCADE + явный список + временное снятие FK. См. `internal/handlers/admin_universe.go`, `clearUniverseTx`, разбор в `ARCHITECTURE.md` §4.1.
- **`/admin/stats` кэшируется.** Отдал 318 257 планет при 319 573 в БД. Для точных цифр — SQL, не эндпоинт.
- **После массовой генерации вселенной нужен `VACUUM (ANALYZE) planets`.** Без него карта видимости холодная, `Index Only Scan` фильтров карты даёт тысячи heap fetches: `has_life` 306 мс вместо 63. В миграцию не положить — `VACUUM` не работает внутри транзакции.
- **`config/descriptions/README.md` содержит чужой текст** — описывает формат `config/anomalies/`. Не ориентируйся на него.
- **`.env` есть в истории git** (добавлен `5253b8c`, удалён `44e3cfc`). Содержит дев-креды, включая `admin123`.
- **`GeneratePlanets` сам очищает старые планеты** (`DELETE FROM planets` в начале генерации, каскад безопасен). Баг B11 закрыт 2026-09-12; ручная чистка перед перегенерацией больше не нужна.
- **Кэш статистики после перегенерации.** `GenerateUniverse` кэширует «0 планет» (планет на тот момент нет — корректно), `GeneratePlanets` сбрасывает кэш в начале генерации и пересчитывает в конце (~20 сек на 318k планет) — в окне пересчёта вкладка статистики считает заново при каждом запросе. Кнопка «Обновить» (`?refresh=1`) сбрасывает кэш принудительно.
- ~~**Две таблицы ресурсов**~~ — снято 2026-09-12. Legacy-таблица `resources` удалена (коммит `f69d991`), вселенная очищена. `substances`/`deposits` (миграция `000017`) созданы и пусты; решено убить и пересоздать по итогам перевывода номенклатуры (`docs/gamedesign/99_roadmap.md` §99.2.5).
- **`internal/core/`, `pkg/`** — черновики, не подключены. Не трогай без причины.
