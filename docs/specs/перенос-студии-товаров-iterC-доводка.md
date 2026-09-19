# Перенос студии товаров в игровой сервер — итерация C: доводка (fill, читатели БД, удаление легаси)

**Статус:** ✅ реализована 2026-09-20 (приёмка создателя «ок», e2e 14/14)
**Дата:** 2026-09-20
**Блок роадмапа:** `docs/gamedesign/99_roadmap.md` §99.2 (строку добавляет менеджер)
**Связи:** идея `docs/gamedesign/ideas/2026-09-19_перенос-студии-товаров-в-игру-БД.md`
(главный документ задачи; §15 хвосты на C, §17 решения создателя 2026-09-20);
спеки `перенос-студии-товаров-iterA-каркас-БД.md` (каркас: таблицы §4, контракт §7,
конкурентность §9, инварианты §12), `перенос-студии-товаров-iterB-ui.md` (UI /studio,
витрина real на БД §5, авторизация §4.1, скрытые кнопки C §4.6);
спеки `99a.1-goods-studio.md` (§7 ИИ-заполнение — эталон переноса), `99a.1-goods-studio-ui.md`,
`99a.3-goods-studio-focus-graph-spravochnik.md`; код `cmd/goods-studio/` (ai/, handlers/state.go
handleFill/buildFillPrompt/runFill, http.go tryStartFill/finishFill, web/index.html fillGood),
`internal/goodsstudio/` (model/graph/validate/seed), `internal/handlers/studio_handlers.go`,
`internal/handlers/admin_resources.go`, `internal/repository/goods_repository.go`,
`internal/resource/` (layer.go/checks.go/chemotypes.go — читатели), `web/studio.html`,
`web/static/js/admin/auth.js`, `web/frontend_test.go`, `docs/impact_map.json`.

---

## 1. Цель и скоуп

Завершить перенос студии товаров на игровой сервер (итерация C — доводка, решения
создателя 2026-09-20, идея §17): ИИ «заполнить комплектующие», перевод читателей
Go-каталога на БД, общий модуль авторизации, удаление старой студии.

**В скоупе C:**
1. **Fill** — перенос `cmd/goods-studio/ai/` в `internal/goodsstudio/ai/` (client/prompt/
   parse/apply + тесты), **двухфазный fill «предложи → подтверди в попапе»** (решение
   создателя 2026-09-20): `POST /studio/api/goods/{id}/fill` — асинхронный запуск ИИ
   (TryStart, статус generating, UI опрашивает state), ответ ИИ разбирается в
   **предложения (proposals)**, НЕ применяется; UI показывает попап «Предложения ИИ»;
   `POST /studio/api/goods/{id}/fill/apply` — применение только принятых пунктов в одной
   транзакции с advisory lock; `POST /studio/api/goods/{id}/fill/cancel` — сброс
   предложений. Честная ошибка «ИИ недоступен» при недоступности opencode, конфиг
   opencode (env), включение кнопки fill в `web/studio.html`.
2. **Экспорт — НЕ делаем** (решение создателя); вопрос §8 идеи (не-approved в экспорте)
   закрыт — неактуален.
3. **Импорт top-каталога/деревьев Т8 — ОТМЕНЁН** (решение создателя); `catalog/` удаляется
   со старой студией; `goods_data/top_catalog.json`, `trees_t8.json` в игру не переносятся.
4. **Удаление старой студии** — `cmd/goods-studio/` целиком + `goods_data/` + `config/goods/`
   + старый смоук `tools/e2e/goods-studio-check.js` (отдельным шагом в конце, после того
   как fill перенесён и работает).
5. **Перевод читателей Go-каталога на БД** — `admin_resources.go`: «Базовый слой» (20),
   покрытие рас, gaps — с `LayerCatalog()`/`CoveringResources()`/`CheckAllFed()` на чтение
   из БД (goods kind=resource c props), переиспользование чистых функций на БД-данных.
   Сидер (`seed.go`) остаётся на Go-каталоге (одноразовый, маркер `goods_catalog_seed`).
   forage/генератор полей — будущие потребители, НЕ в C. Проверки 94a
   (CheckMediocrity/CheckRanges/CheckBridgeLiveness) — в рантайме не вызываются, в тестах
   остаются на Go-каталоге (эталон сида).
6. **Общий модуль авторизации** — `web/static/js/auth.js` (ядро) + тонкие адаптеры
   админки и студии; без изменения поведения, только консолидация.
7. **Доки** — дельты (ARCHITECTURE, PITFALLS, INDEX, impact map, GLOSSARY, 99_roadmap,
   CHANGELOG, STATUS, specs/README, COORDINATION).

**НЕ в скоупе C:** схема substances/resources (99.2.5 — отдельный релиз); маппинг
категорий на игровую модель (99a.1 §17); forage и генератор полей на БД (будущие
потребители); хвост «контекстное меню ПКМ в списке справочника» (99a.3 — отдельная идея).

---

## 2. Разворот подходов — пространство решений (история итераций)

Перебрано до углубления (правило создателя: сначала обзор, потом числа):

| # | Развилка | Варианты | Цена | Выбор |
|---|---|---|---|---|
| 1 | **Конфиг opencode** | A. env-переменные (OPENCODE_URL/MODEL/TIMEOUT_S/MAX_RETRIES) с дефолтами из studio.json; B. сохранить `config/goods/studio.json` как источник; C. новый `config/opencode.json` | A: паттерн сервера (config.go — всё из env), дефолты = старые значения (url 127.0.0.1:3456, model opencode/deepseek-v4-flash, timeout 120, retries 2), на проде env не заданы — fill честно падает «ИИ недоступен»; config/goods/ удаляется со старой студией. B: файл живёт, но сервер не читает JSON-конфиги (кроме файлов-данных), два источника конфигурации. C: новый файл ради 4 значений — избыточно | **A** — env (§4) |
| 2 | **Статус fill (generating/report)** | A. in-memory в хендлере (mutex + bool + []string, TryStart); B. таблица в БД (fill_jobs) | A: поведение старой студии 1:1 (tryStartFill/finishFill, http.go:85–103), ноль схемы/миграции, отчёт — транзиентный статус последнего прогона (не лог — истории не нужно); рестарт сервера сбрасывает генерацию — приемлемо для dev-инструмента (старая студия: то же). B: персистентность не нужна (отчёт одноразовый), таблица ради одного факта — анти-паттерн «сущность с двумя экземплярами» | **A** — in-memory (§5.2) |
| 3 | **Применение ответа ИИ (apply)** | A. per-slot функция `ApplyProposals` (применяет выбранное подмножество по конкретным слотам; резолв имени/бана/цикла/галки — на свежем снимке в tx); B. последовательный `ApplyFill` (заполняет пустые слоты по порядку — не умеет подмножество: пропуск пункта сдвигает слоты) | A: подмножество и слоты сохраняются (пользователь принял пункты 0 и 2 — заполняются слоты 0 и 2, не 0 и 1); резолв повторяется в tx (свежий снимок — бан/цикл/дубликаты перепроверяются); тесты apply (17) адаптируются под per-slot-сигнатуру (семантика сохранена). B: несовместимо с «применить выбранное» | **A** — ApplyProposals (§5.4) |
| 4 | **Категория нового товара, если ИИ дал невалидную/пустую** | A. молчаливый фолбек на категорию родителя; B. пропуск создания + отчёт; C. **выбор в попапе** (дефолт — категория родителя, пометка «категория „Y" не найдена», можно выбрать любую существующую или пропустить) | A: неявное решение за пользователя (отвергнуто создателем). B: теряет контент. C: явный выбор до применения (в БД category_id NOT NULL — «без категории» невозможно); закрывает развилку — решение создателя 2026-09-20 | **C** — выбор в попапе (§5.4/§6) |
| 5 | **Читатели Go-каталога (слой 20)** | A. переиспользовать чистые функции `CoveringResources`/`CheckAllFed` на БД-данных (маппинг props → `[]*resource.Resource`); B. продублировать логику на БД-данных | A: один источник логики (checks.go), БД — источник данных (С1), маппинг ~30 строк (по образцу realResourceFromRow, iterB §5.3); шаблоны хемотипов (LayerTemplates) — модели, не контент — остаются в Go. B: дублирование алгоритмов окон/покрытия — дрейф | **A** — переиспользование (§8) |
| 6 | **Дискриминатор layer-ресурсов в БД** | A. `props ? 'closes'` (JSONB-оператор наличия ключа); B. `props ? 'bridge'`; C. `props IS NOT NULL AND NOT props ? 'family'` | A: `closes` есть у всех 20 layer (сид, seed.go layerProps), нет у real (realProps) и у пользовательских (props NULL) — точный признак. B: bridge есть только у 7 мостовых — 13 ядерных не попадут. C: включает пользовательские ресурсы с props (сейчас их нет, но схема не гарантирует) | **A** — `props ? 'closes'` (§8) |
| 7 | **Общий модуль авторизации** | A. ядро `web/static/js/auth.js` (id-независимые примитивы: токен/роль//me//login/401) + тонкие адаптеры `admin/auth.js` (те же экспорты, то же поведение) и `studio/auth.js` (window.*, id студии, bootstrap); B. единый параметризованный модуль + прямой импорт во все 13 файлов админки | A: логика — один раз в ядре; контракт экспортов admin/auth.js не меняется (13 файлов админки импортируют ./auth.js: 12 — `fetchWithAuth`/`getAdminToken`, main.js — `ensureAdminAuth` и др. — не трогаем); frontend_test.go — граф админки резолвится (admin/auth.js → ../auth.js); студия — модуль-адаптер вместо инлайн-копии (~80 строк удаляются). B: чище «единый модуль» буквально, но правка 13 файлов импортов + риск регрессий + граф теста меняется сильнее | **A** — ядро + адаптеры (§9) |
| 8 | **Судьба мёртвого кода model** (LoadState/SaveState/NextCategoryID/ResourceRef) | A. удалить вместе со старой студией (осиротевшие после удаления единственного потребителя); B. оставить | A: AGENTS.md §3 «удалять осиротевшее»; LoadState/SaveState/state_test.go/NextCategoryID/ResourceRef — только старая студия. B: мёртвый код в internal | **A** — удалить (§10) |
| 9 | **Старый смоук `goods-studio-check.js`** | A. удалить со старой студией; B. оставить | A: его цель (бинарь 8799) удаляется — смоук мёртв. B: висяк | **A** — удалить (§10) |
| 10 | **Поток fill: применить сразу vs предложи→подтверди** | A. применить сразу (старая студия: ответ ИИ → в слоты, отчёт); B. **двухфазный**: ИИ → предложения (proposals) → попап с вариантами по каждому пункту → «Применить выбранное»/«Отмена» | A: контент пишется без контроля (ИИ ошибся в категории/составе — править после). B: контроль до записи — создатель видит и выбирает (Создать/Заполнить/Пропустить/Изменить категорию); цена — второй запрос apply + попап; proposals транзиентны (in-memory, сброс отменой/повторным fill) | **B** — решение создателя 2026-09-20 (§5) |

Отклонённые варианты зафиксированы; повторно не пересматриваются.

---

## 3. Архитектура — куда что ложится

```
internal/goodsstudio/ai/          — НОВЫЙ (move из cmd/goods-studio/ai/, разв. 3):
  client.go                       — HTTP к opencode (как есть, контракт 99a.1 §7.5)
  prompt.go                       — BuildPrompt (как есть)
  prompt_fill.go                  — НОВЫЙ: BuildFillPrompt(st *model.State, goodID string)
                                    (перенос buildFillPrompt из handlers/state.go:763)
  parse.go                        — ParseFillResponse (как есть)
  proposals.go                    — НОВЫЙ: BuildProposals (разбор ответа в предложения,
                                    разв. 10) + ApplyProposals (per-slot применение
                                    подмножества, разв. 3)
  ai_test.go                      — move (23 теста: 7 parse/prompt без правок, 17 apply
                                    адаптируются под per-slot-семантику ApplyProposals)
                                    + prompt_fill_test.go + proposals_test.go
internal/handlers/studio_handlers.go    — ПРАВКА: fill/apply/cancel-хендлеры, статус
                                           генерации + proposals, Report в StateView
internal/repository/goods_repository.go — ПРАВКА: ApplyProposals (write-back), LayerResources
internal/handlers/admin_resources.go    — ПРАВКА: слой 20 из БД (разв. 5/6)
internal/config/config.go               — ПРАВКА: 4 поля opencode (env, разв. 1)
cmd/server/main.go                      — ПРАВКА: ai.Client, NewStudioHandlers(db, ai, model),
                                           роуты /studio/api/goods/{id}/fill[/apply|/cancel]
web/studio.html                         — ПРАВКА: кнопка fill + попап «Предложения ИИ»
                                           + отчёт + auth-модуль (§6/§9)
web/static/js/auth.js                   — НОВЫЙ: ядро авторизации (разв. 7)
web/static/js/studio/auth.js            — НОВЫЙ: адаптер студии
web/static/js/admin/auth.js             — ПРАВКА: тонкий адаптер (те же экспорты)
tools/e2e/goods-studio-server-check.js   — ПРАВКА: шаг fill (негатив) + apply (§11)
cmd/goods-studio/                       — УДАЛЯЕТСЯ (шаг 6, §10)
goods_data/, config/goods/, tools/e2e/goods-studio-check.js — УДАЛЯЮТСЯ
internal/goodsstudio/model/state.go, ids.go (частично), types.go (ResourceRef) — УДАЛЯЮТСЯ
```

**Перенос `ai/` (разв. 3):** move (не копия — AGENTS.md §4.9). Старая студия до удаления
продолжает собираться: правятся импорты `zorion/cmd/goods-studio/ai` →
`zorion/internal/goodsstudio/ai` в `cmd/goods-studio/{main.go, handlers/http.go,
handlers/state.go, handlers/handlers_test.go}` (паттерн iterA §3 — хирургическая правка).

**Почему apply в репозитории, а не в хендлере:** вставка в БД — I/O с транзакцией и
advisory lock (дисциплина iterA §9.1); хендлер — только оркестрация (TryStart, goroutine,
proposals, отчёт). Чистая логика (BuildProposals/ApplyProposals) остаётся в ai-пакете и не
знает о БД.

**Конкурентность (AGENTS.md §0):** каталог в БД; fill-статус + proposals — in-memory под
mutex (разв. 2); apply пишет через `repo.ApplyProposals` — одна транзакция с
`pg_advisory_xact_lock` (сериализация с другими мутациями каталога). `*rand.Rand` не
используется. TryStart — атомарный старт (не `if running { Start() }`).

---

## 4. Конфиг opencode (разв. 1)

В `internal/config/config.go` — 4 необязательных поля (паттерн BALANCER_PRESETS_FILE:
env с дефолтом; на проде не заданы — это нормально, fill честно падает):

| env | Дефолт (из `config/goods/studio.json`) | Поле Config |
|---|---|---|
| `OPENCODE_URL` | `http://127.0.0.1:3456` | `OpenCodeURL string` |
| `OPENCODE_MODEL` | `opencode/deepseek-v4-flash` | `OpenCodeModel string` |
| `OPENCODE_TIMEOUT_S` | `120` | `OpenCodeTimeout time.Duration` |
| `OPENCODE_MAX_RETRIES` | `2` | `OpenCodeMaxRetries int` |

`config/goods/studio.json` удаляется со старой студией (§10) — дефолты переносятся в код.
`auto_refresh_ms` из studio.json НЕ переносится: клиентский дефолт `|| 3000` уже закреплён
в B (С1, iterB §4.4).

---

## 5. Fill — «заполнить комплектующие» (двухфазный: предложи → подтверди, разв. 10)

Три эндпоинта (ветки в `GoodByID`, паттерн status/tier/slots; отдельные роуты не нужны —
в отличие от bulk, конфликта парсинга id нет):

| Метод/путь | Назначение | Ответ |
|---|---|---|
| `POST /studio/api/goods/{id}/fill` | запуск ИИ (асинхронно): ответ разбирается в **предложения**, НЕ применяется | 202 `{"started":"true"}` · 400 нет пустых слотов · 404 нет товара · 409 уже идёт генерация |
| `POST /studio/api/goods/{id}/fill/apply` | применить **принятые** предложения (одна транзакция с advisory lock) | 200 `{"applied": N}` · 400 пустой/битый accepted · 409 нет предложений / идёт генерация · товар удалён между фазами → **200 + отчёт «товар не найден»** (М2) |
| `POST /studio/api/goods/{id}/fill/cancel` | сбросить предложения (Отмена в попапе) | 200 · 404 нет товара · **409 идёт генерация — отмена после завершения** (М5) |

### 5.1. Фаза 1 — запуск ИИ: `POST /studio/api/goods/{id}/fill`

Поток (эталон: старая handleFill, state.go:714–740, + разв. 10):
1. `r.Method != POST` → 405.
2. Снимок каталога (`repo.Snapshot()`): товар не найден → 404; пустых слотов нет → 400
   «нет пустых слотов» (эталон: emptySlots, state.go:726).
3. Промпт: `ai.BuildFillPrompt(st, goodID)` на снимке (st — model.State из снимка).
4. `tryStartFill()` → false → 409 «уже идёт генерация» (TryStart, эталон http.go:86–95);
   **tryStartFill сбрасывает предыдущие proposals** (повторный fill — новые предложения).
5. `go runFill(id, prompt)` → 202 `{"started":"true"}`.

`runFill` (эталон state.go:743–759, изменён: разбор → proposals, НЕ применение):
- `raw, err := aiClient.FillComponents(prompt)` → err → `finishFill(["Ошибка ИИ: "+err])`
  (честная ошибка «ИИ недоступен» — решение создателя, вариант а);
- `comps, err := ai.ParseFillResponse(raw)` → err → `finishFill(["Мусор в ответе ИИ: "+err])`;
- `proposals, dropReport := ai.BuildProposals(st, goodID, comps)` на снимке → err →
  `finishFill(["Ошибка разбора: "+err])`;
- `setProposals(proposals, goodID)` + `finishFill(dropReport)` (дропы бана/ресурса/цикла —
  в отчёте сразу; proposals — в state для попапа).

### 5.2. Статус генерации + proposals (разв. 2) — in-memory

`StudioHandlers` получает поля (паттерн старой http.go:24–27):

```go
type StudioHandlers struct {
    repo   *repository.GoodsRepository
    ai     *ai.Client
    aiModel string

    fillMu         sync.Mutex
    fillGenerating bool
    fillReport     []string
    fillProposals  []ProposalView   // предложения последнего завершённого fill
    fillProposalsGoodID string      // товар, для которого предложения
}
```

`tryStartFill`/`finishFill` — копия старой логики (http.go:85–103) под `fillMu`;
`setProposals` — запись под `fillMu`; `clearProposals` — сброс (apply/cancel/новый fill).
`NewStudioHandlers(db, aiClient, model)` — сигнатура меняется (main.go).

**Природа сущности (архитектурный фильтр):** статус fill и proposals — транзиентное
состояние инструмента (не событие, не лог — истории прогонов не ведём), владелец — один
хендлер, тип один. In-memory — правильно; таблица была бы «сущностью ради одного факта».

### 5.3. State — report/generating/model/proposals

`StateView` (studio_handlers.go:70) — добавляются поля (аддитивно к iterA §7, существующие
не меняются):
- `Report []string` (`json:"report"`, как в старой StateView state.go:56);
- `Proposals []ProposalView` (`json:"proposals"`) — пустой массив, если предложений нет;
- `ProposalsGoodID string` (`json:"proposals_good_id,omitempty"`) — товар-цель.

`ProposalView` (в state):
```json
{"slot": 0, "name": "Сталь", "category": "металлы", "category_id": 5,
 "category_valid": true, "reason": "несущий каркас",
 "kind": "link", "link_id": "17", "link_name": "Сталь"}
```
- `slot` — pos пустого слота (0-based);
- `kind`: `"new"` — создать новый товар (имя не найдено в каталоге) / `"link"` — заполнить
  слот готовым товаром (совпадение по нормализованному имени);
- `category` — как ИИ назвал категорию (для пометки «не найдена»); `category_id` —
  резолв (int64) если валидна, иначе null; `category_valid` — производное;
- `link_id`/`link_name` — только для kind=link.

`buildStateView` (или State-хендлер) заполняет: `Model: h.aiModel`, `Generating:
h.fillGenerating`, `Report: копия(h.fillReport)`, `Proposals: копия(h.fillProposals)`,
`ProposalsGoodID: h.fillProposalsGoodID` (копии под fillMu — слайсы не мутировать извне).

Контракт iterA §7 по state не ломается: поля добавляются, существующие не меняются
(UI B-версии уже читает `state.generating`/`state.model` — line 379; report — возвращается,
как и обещано в B: «report вернётся в C вместе с fill», iterB §2 разв. 8).

### 5.4. Предложения и применение — `BuildProposals` / `repo.ApplyProposals`

**`ai.BuildProposals(st *model.State, goodID string, comps []Component) ([]Proposal, []string)`**
— чистая функция (фаза 1): для каждого компонента (в порядке пустых слотов):
- **лишние компоненты** (больше пустых слотов): строка в отчёт «ИИ вернул больше
  запрошенного: N лишних — пропущены», лишние отбрасываются (эталон ApplyFill
  apply.go:27–29, М7); меньше — «ИИ вернул меньше запрошенного: M из K — остальные
  слоты пустые» (информационно);
- резолв имени (`graph.NameIndex`): найдено → `kind=link` (link_id/link_name);
  не найдено → `kind=new` (category: `resolveCategory` → category_id/category_valid);
- **дропы на фазе разбора** (не показываются в попапе, строки в отчёт): совпадение с
  banned/excluded («ИИ предложил X: забаненное — пропущено»); ресурс в слот без галки
  «заполнять ресурсом»; цикл для link (`graph.WouldCreateCycle` на снимке — новый товар
  лист, цикла дать не может);
- возврат: actionable-предложения (kind=new/link) + отчёт дропов.

**`POST /studio/api/goods/{id}/fill/apply`** — `{accepted: [{i: 0, category_id: 5}, {i: 1}]}`:
- `i` — индекс в `fillProposals` (порядок массива); `category_id` — финальная категория
  для kind=new (обязателен для new — выбор попапа; для link игнорируется);
- валидация: proposals есть и `fillProposalsGoodID == id` (иначе 409 «нет предложений»);
  `generating` — 409; `accepted` непуст, индексы в диапазоне (400); `category_id` для
  kind=new — непустое число (400). **Существование категории НЕ проверяется в хендлере —
  только в транзакции под lock** (С2, TOCTOU: категория могла быть удалена между
  валидацией и INSERT);
- товар удалён между фазами: 404 НЕ возвращается — `repo.ApplyProposals` вернёт отчёт
  «товар не найден», ответ 200 (М2 — единообразно с таблицей §5);
- отчёт: строки «пропущено: X (не принято)» для отклонённых пользователем +
  результат `repo.ApplyProposals` + строка «Применено: N» (N — из ответа, М3);
  `finishFill(report)`; `clearProposals()`; ответ `{"applied": N}`.

**`repo.ApplyProposals(goodID int64, items []ai.ProposalItem) (applied int, report []string, error)`** —
одна транзакция с advisory lock (разв. 3, дисциплина iterA §9.1):
`ProposalItem = {Slot int, Name string, CategoryID string, Reason string, Kind string}`
(Kind — «new»/«link» из proposal; финальная категория уже выбрана попапом — резолв
категории на apply не нужен; Kind нужен для С1):

1. `beginMutation()` (pg_advisory_xact_lock — сериализация с другими мутациями каталога).
2. В tx: `loadCategories` + `loadGoods` + `loadSlots` (существующие хелперы) → собрать
   `model.State`: Categories `{ID: strconv(id), Name}`, Goods с Recipe (id — строки).
   Запомнить `origLen = len(goods)`.
3. `applied, report := ai.ApplyProposals(st, strconv.FormatInt(goodID, 10), items)` —
   мутирует копию (чистая функция; решение в tx под lock — снимок свежий, между решением
   и записью никто не вмешается). **Per-slot**: заполняется именно `item.Slot`, не «первые
   пустые по порядку». **Валидации на свежем снимке (каждая — дроп пункта + строка в
   отчёт, дропы в счётчик applied НЕ входят):**
   - **С3 — слот**: `pos` существует в recipe целевого товара и пуст; иначе — дроп
     «слот занят/удалён» (DeleteSlot сдвигает pos; слот мог быть удалён или вручную
     заполнен между фазами);
   - **С1 — link-цель**: для kind=link имя не находится (`NameIndex`) — дроп «ссылка
     исчезла (товар удалён/переименован)»; НЕ переводить в kind=new (у link-пункта
     категории нет → FK-ошибка). Для kind=new имя нашлось — ссылка на существующий
     (безопасно, категория игнорируется);
   - **С2 — категория**: для kind=new — `CategoryID` существует и kind=good в текущем
     снимке категорий; иначе — дроп «категория удалена» (DeleteCategory под lock мог
     убрать её после выбора в попапе);
   - бан/исключённое — дроп (существующее правило, перепроверка);
   - ресурс в слот без галки «заполнять ресурсом» — дроп;
   - цикл (`graph.WouldCreateCycle` на свежем снимке) — дроп;
   - UNIQUE-конфликт имени при создании (23505) — дроп, страховка (как BulkCreateGoods).
4. **Write-back** (diff строго определён — ApplyProposals только (а) добавляет новые
   товары в конец `st.Goods`, (б) заполняет конкретные слоты целевого товара):
   - для `i := origLen; i < len(st.Goods); i++`: новый товар (ID "gN" от
     `model.NextGoodID`, Name, Category — строка id выбранной категории, всегда валидна —
     выбор попапа):
     - `INSERT INTO goods (name, name_norm, category_id, kind, status, source) VALUES
       ($1,$2,$3,'good','draft','ai') RETURNING id` → карта `"gN" → realID`;
   - для целевого товара: для каждого применённого item (pos = item.Slot, слот был пуст и
     заполнен): `UPDATE goods_slots SET component_id = $1, reason = $2 WHERE good_id =
     $target AND pos = $pos`; **component_id берётся из мутированного state
     (`st.Goods[gi].Recipe[item.Slot].GoodID`) — это либо «gN» (создан в этом прогоне,
     М4: маппинг по id слота, не по имени — карта `"gN"→realID` построена на шаге INSERT
     по порядку созданных), либо числовой id существующего товара/ресурса (`parseInt64`)**.
5. COMMIT. Возврат: `applied` (число фактически записанных пунктов, дропы не входят) +
   report.

**Инварианты, сохраняемые write-back'ом:** ресурс в слот без галки — дроп (ApplyProposals),
цикл — дроп (ApplyProposals + снимок под lock), бан/исключённое — дроп, новые товары —
draft/source=ai, слоты ресурсу не создаются (ApplyProposals работает только с recipe
товара; ресурс как target невозможен — fill-хендлер: у ресурса слотов нет → 400 «нет
пустых слотов»; `checkGoodForSlots` в ApplyProposals-транзакции не нужен — ApplyProposals
сам вернёт «нет пустых слотов»).

**Промпт** — `ai.BuildFillPrompt(st *model.State, goodID string) string`: перенос
логики buildFillPrompt (state.go:763–805) на model.State: tier = `graph.Tier` (вычисленный),
filled (имена заполненных слотов), banned (banned/excluded имена), resNames (kind=resource),
catNames ("id: имя" — id теперь числовые строки), k пустых слотов, allowResSlots
(1-базовые позиции пустых слотов с галкой). Тест: перенос TestFillPromptUsesComputedTier
(handlers_test.go:783).

---

## 6. UI — кнопка fill, попап «Предложения ИИ», отчёт (web/studio.html)

Восстановление поведения старой студии (эталон: cmd/goods-studio/web/index.html) +
новый попап подтверждения (решение создателя 2026-09-20, разв. 10), адаптация под
`/studio/api/*`:

1. **Попап товара (renderPopup, ветка товара):** кнопка
   `<button class="gen" onclick="fillGood()">заполнить комплектующие</button>` —
   disabled при `state.generating || нет пустых слотов`, title как в старом
   (index.html:921). Ресурсам — НЕ показывать (ресурс — лист).
2. **Контекстное меню канваса (showCtxMenu):** пункт «заполнить комплектующие»
   для товаров (не ресурсов) — как старый index.html:742.
3. **`fillGood()`** (эталон index.html:1001–1010): если нет пустых слотов →
   `showReport(["Сначала добавьте слот («+ слот»)"])`; иначе
   `POST /studio/api/goods/{popupGoodId}/fill` → ok → `showReport(["ИИ думает… —
   предложения появятся в попапе"])`; `fetchState()`.
4. **fetchState — секция report** (эталон index.html:269–273): восстановить
   `let lastShownReport = ""`; после получения state: если `state.report && state.report.length`
   и `JSON.stringify(state.report) !== lastShownReport` → `showReport(state.report)`,
   иначе `lastShownReport = ""`. Отчёт приходит опросом state (fill асинхронный).
5. **Попап «Предложения ИИ»** — авто-открытие при получении proposals:
   - в fetchState (паттерн lastShownReport, эталон index.html:268–274):
     - если `state.proposals.length > 0`: ключ `JSON.stringify(state.proposals) +
       state.proposals_good_id`; если `ключ !== lastProposalsKey` → открыть попап,
       запомнить ключ (`let lastProposalsKey = ""`); повторный опрос тот же набор не
       переоткрывает;
     - **else-ветка (М1):** `state.proposals` пуст → `lastProposalsKey = ""` — иначе
       «Отмена» → повторный fill → идентичный набор ИИ → ключ совпал бы со старым и
       попап не открылся бы;
     - **М6:** при старте нового fill (tryStartFill сбрасывает proposals →
       `state.proposals` пуст) открытый попап **закрывается** (else-ветка + закрытие
       модалки, если открыта);
   - компоновка (модалка в стиле студии):
     - заголовок: «Предложения ИИ для «{имя товара}»» (имя из `state.goods` по
       `proposals_good_id`); подпись «применятся только принятые пункты»;
     - **строка на каждый proposal** (по слотам):
       - `kind=link`: «Заполнить слот N готовым товаром «Z»» + переключатель
         [Заполнить ✓ / Пропустить] (дефолт — Заполнить) + подпись reason;
       - `kind=new`: «Создать новый товар «X»» + переключатель [Создать ✓ / Пропустить]
         (дефолт — Создать) + **селект категорий** (товарные категории из
         `state.categories` kind=good; дефолт — `category_id`, если `category_valid`,
         иначе категория родителя — `proposals_good_id` из `state.goods`) + подпись
         reason; при `!category_valid` — пометка красным «категория „{category}" не
         найдена — выбрана категория родителя»;
     - футер: «Применить выбранное (N)» (enabled при N ≥ 1) / «Отмена».
   - **«Применить выбранное»**: собрать `accepted = [{i, category_id}]` (i — индекс в
     `state.proposals`, category_id — выбранный селект для kind=new) →
     `POST /studio/api/goods/{proposals_good_id}/fill/apply` → ok → закрыть попап,
     `fetchState()` (отчёт «Применено…/пропущено…» придёт через state);
   - **«Отмена»**: `POST /studio/api/goods/{proposals_good_id}/fill/cancel` → ok →
     закрыть попап, `fetchState()` (proposals сброшены — попап не переоткроется).
6. **Состояния fill (генератор UI):**
   - `generating` — genIndicator «⚙ генерация…», кнопка fill disabled, попап не показан;
   - `proposals` — попап «Предложения ИИ» (авто-открытие, п.5);
   - `applied` — попап закрыт, отчёт-тост «Применено: N, пропущено: M» (из state.report);
   - `error` — отчёт-тост «Ошибка ИИ: …» (opencode недоступен / мусор в ответе), попапа нет;
   - `cancel` — попап закрыт, proposals сброшены.
7. **modelInfo:** `"Модель ИИ: — (итерация C)"` → `"Модель ИИ: " + (state.model || "—")`.
   genIndicator уже читает `state.generating`/`state.model` (line 379) — работает.
8. Авторизация — переключается на общий модуль (§9).

---

## 7. Перевод читателей Go-каталога на БД — admin_resources.go

### 7.1. Граница (проверено по коду)

В рантайме `GET /admin/resources` использует Go-каталог в 3 местах: `catalog :=
resource.LayerCatalog()` (line 155) → DTO `resources` + `CoveringResources` (line 204) +
`CheckAllFed` (line 209). Всё это переводится на БД-данные. Остаётся в Go: `templates`
(LayerTemplates — модели хемотипов, не контент, iterB §5.1), `races` (config/races.json),
`chemotypes` (модели), `GetCategory(code).Icon` (стиль-маппинг, решение iterB §5.3 п.3).
`CheckMediocrity`/`CheckRanges`/`CheckBridgeLiveness` в рантайме НЕ вызываются (только в
тестах checks_test.go) — остаются на Go-каталоге как эталон сида.

### 7.2. Репозиторий — `LayerResources() ([]LayerResourceRow, error)`

```sql
SELECT g.id, g.name, c.code, g.props
FROM goods g JOIN categories c ON c.id = g.category_id
WHERE g.kind = 'resource' AND g.props ? 'closes'
ORDER BY g.id
```

Дискриминатор `props ? 'closes'` (разв. 6): `closes` есть у всех 20 layer-ресурсов
(seed.go layerProps), нет у real (realProps) и у пользовательских (props NULL).
`LayerResourceRow {ID int64, Name string, Category string /*code*/, Props []byte}`.

### 7.3. Маппинг props → `*resource.Resource` (переиспользование, разв. 5)

`layerResourcesFromDB()` (в admin_resources.go, по образцу realResourcesFromDB iterB §5.3):
- ID = `strconv.FormatInt(row.ID, 10)` (числовой → строка, как real в B);
- Name, Category = code из `categories.code`;
- 10 осей — русские ключи props → поля (маппинг `resource.Axis*`, как realResourceFromRow);
- `t_melt_k`/`t_boil_k` → TMelt/TBoil (K);
- `closes` → Closes ([]string из props), `bridge` → Bridge (bool), `supercritical` →
  Supercritical (bool);
- Sublimating — производное (метод Resource.Sublimating(), не хранится).

В `GetAdminResources`: `catalog, err := h.layerResourcesFromDB()` вместо
`resource.LayerCatalog()`; `CoveringResources(catalog, templates, race, axis)` и
`CheckAllFed(catalog, templates, allRaces)` — вызовы без изменений (чистые функции на
БД-данных). DTO-форма `adminResourceDTO` не меняется; `resources`/`races`/`gaps` —
те же поля.

### 7.4. Клиент resources.js — правок не требуется

Проверено по коду: `resourceNames[r.id]` (line 249), `resourceRaces[r.id]` (line 253),
`data-id`/`find(x => x.id === ...)` (line 314/402), coverage-имена
`resourceNames[id] || id` (line 354) — id используется как непрозрачный ключ; числовые
строки работают (как real в B). Покрытие: `CoveringResources` возвращает `res.ID` —
числовые строки, `resourceNames` построен из тех же БД-ресурсов — ключи совпадают.

**Снятие ограничения С5 (iterB §5.3):** правки layer-ресурса в студии (переименование/
категория/удаление) теперь ВИДНЫ в подвкладке «Базовый слой» и в покрытии рас (БД —
источник правды, С1). Это цель C, не баг. Удалённый в студии layer-ресурс исчезает из
слоя и покрытия; если ничего не покрывает ось — `CheckAllFed` покажет дыру (честно).

---

## 8. Общий модуль авторизации (разв. 7)

### 8.1. Ядро — `web/static/js/auth.js` (НОВЫЙ, ES-модуль, без DOM/toast)

Id-независимые примитивы (логика — один раз):

```js
const TOKEN_KEY = 'adminToken';
const ADMIN_ROLES = ['admin', 'skycomposer'];
// getToken()/setToken(t)/clearToken() — localStorage + кэш
// isAdminRole(role)
// validateToken() — GET /me с Bearer → {ok, role} (без UI)
// login(username, password) — POST /login → {ok, token, role, username, error} (без UI)
// fetchWithAuth(url, options, onUnauthorized) — Bearer; 401 → clearToken() +
//   onUnauthorized() + throw 'Unauthorized'
```

На верхнем уровне — только константы и localStorage (frontend_test.go: node-стаб
`localStorage` есть; DOM не нужен — граф админки резолвится и грузится).

### 8.2. Адаптер админки — `web/static/js/admin/auth.js` (ПРАВКА)

Тонкий адаптер с **теми же экспортами и поведением** (13 файлов админки импортируют
./auth.js — 12 по `fetchWithAuth`/`getAdminToken`, main.js по `ensureAdminAuth` и др. —
не трогаем):
`ensureAdminAuth`, `adminLogin`, `fetchWithAuth`, `getAdminToken`, `setAfterLogin`,
`getAdminRole`, `adminLogout`. Внутри: вызовы ядра + id `adminLogin*` + toast
(`notifyError`/`notifySuccess` из `../ui/toast.js` — импорт остаётся). Поведение 1:1:
оверлей, сообщения, сброс токена на 401, welcome-тост. frontend_test.go: граф
`admin/main.js → ./auth.js → ../auth.js + ../ui/toast.js` — резолвится (пути относительные,
файлы существуют).

### 8.3. Адаптер студии — `web/static/js/studio/auth.js` (НОВЫЙ)

Модуль-адаптер (заменяет инлайн-копию ~80 строк в studio.html, iterB §4.1):

- `window.ensureStudioAuth` — нет токена → оверлей; `validateToken()` fail → clearToken +
  оверлей; роль не admin/skycomposer → оверлей «Недостаточно прав: нужна роль
  администратора»; ok → скрыть оверлей.
- `window.studioLogin` — чтение `studioLoginUsername/Password/Error`, `login()`, ошибки
  как сейчас, ok → setToken + скрыть + `window.start()`.
- `window.studioFetch` — `fetchWithAuth(url, opts, () => { showStudioLogin();
  window.showReport(['Сессия истекла — войдите снова']); })`.
- `showStudioLogin`/`hideStudioLogin` — перенос из инлайн-кода (id `studioLogin*`).
- **bootstrap** (перенос строки studio.html:1653): `window.ensureStudioAuth().then(ok =>
  { if (ok) window.start(); })` — вызывается самим модулем (модуль выполняется после
  парсинга документа — `start`/`showReport` из классического скрипта уже определены).

`web/studio.html`: удалить инлайн-блок авторизации (строки ~283–362) и строку bootstrap
(1653); добавить `<script type="module" src="/static/js/studio/auth.js"></script>` в конец
body. `api()` (line 1095) продолжает вызывать `studioFetch` — теперь window-функцию из
модуля (вызов в рантайме, после загрузки модуля — безопасно).

**Поведение не меняется:** оверлей, сообщения, 401-сброс, общий `adminToken`,
переходы админка ↔ студия (кнопки из B) — работают как раньше.

---

## 9. Удаление старой студии — порядок (шаг 6, отдельным коммитом)

**Проверено:** ничто в `cmd/server/`/`internal/` не импортирует `cmd/goods-studio`
(grep по `zorion/cmd/goods-studio` — только сам бинарь; `internal/goodsstudio/seed.go`
упоминает в комментарии). Удаление не ломает сборку.

Порядок (чтобы не сломать разработку):
1. fill перенесён и работает (§5–6) — старый бинарь ещё жив (импорты ai обновлены).
2. admin_resources переведён на БД (§7).
3. auth-модуль консолидирован (§8).
4. **Удаление** (отдельный коммит зоны developer): `cmd/goods-studio/` целиком;
   `goods_data/` (state.json, top_catalog.json, trees_t8.json, export.json — вне git,
   физически); `config/goods/` (studio.json); `tools/e2e/goods-studio-check.js` (разв. 9);
   мёртвый код model: `internal/goodsstudio/model/state.go` (LoadState/SaveState) +
   `state_test.go`, `NextCategoryID` из `ids.go` (NextGoodID остаётся — используется
   ApplyProposals), `ResourceRef` из `types.go` (разв. 8).
5. DoD: `go build ./...`, `go vet ./...`, `go test -race ./...` зелёные (после удаления
   сборка обязана пройти — подтверждение отсутствия ссылок).

---

## 10. Что НЕ входит в C (границы)

- Схема substances/resources (99.2.5) — отдельный релиз; props JSONB — промежуточное.
- Маппинг категорий студии на игровую модель (99a.1 §17) — будущий.
- forage, генератор полей — будущие потребители Go-каталога, переводятся отдельно.
- Хвост «контекстное меню ПКМ в списке справочника» (99a.3) — отдельная идея.
- Экспорт JSON и импорт top-каталога/деревьев Т8 — отменены решениями создателя.
- Старые спеки 99a.1/99a.2/99a.3 — остаются как история продукта (не удаляются;
  INDEX-строки помечаются «легаси удалён»).

---

## 11. DoD и критерии приёмки

**DoD разработки (AGENTS.md §2):** `go build ./...`, `go vet ./...`,
`go test -race ./...` (93a: gcc WinLibs в PATH). TDD: тест до кода (красный → зелёный).
Минимальный набор тестов:
- ai: 23 теста переезжают (7 parse/prompt — без правок; 17 apply — адаптируются под
  per-slot-семантику `ApplyProposals`: те же кейсы — exact K/бан/ресурс-галка/цикл/
  reason/link/new — но с указанием слота) + BuildFillPrompt (перенос
  TestFillPromptUsesComputedTier) + BuildProposals (kind new/link, category_valid,
  дропы бана/ресурса/цикла в отчёте, **лишние компоненты — «больше запрошенного»**,
  М7) + ApplyProposals (подмножество: приняты пункты 0 и 2 → заполнены слоты 0 и 2;
  пропущенный — не тронут; повторный резолв на свежем снимке);
- репозиторий (sqlmock): ApplyProposals — новые товары INSERT (draft/ai, категория из
  попапа), слоты UPDATE по конкретным pos, маппинг "gN"→real id, отчёт, **applied =
  число записанных (дропы не входят, М3)**;
  LayerResources — только `props ? 'closes'`, маппинг closes/bridge/supercritical/оси;
- хендлеры: fill 404 (нет товара), 400 (нет пустых слотов), 409 (уже идёт генерация —
  TryStart), 202 + state: generating=true → после завершения false + proposals в state
  (stub-клиент с недостижимым URL — «Ошибка ИИ», proposals пусты); apply 409 (нет
  предложений / идёт генерация), 400 (пустой/битый accepted, category_id для new пуст),
  200 + applied N + отчёт, **товар удалён между фазами → 200 + отчёт «товар не найден»
  (М2)**; cancel 200 (proposals сброшены), **cancel во время генерации → 409 (М5)**;
- **валидации apply-фазы (С1–С3, юнит ApplyProposals):** link-цель удалена/переименована
  между фазами → дроп + отчёт «ссылка исчезла», НЕ создаётся новый товар (С1);
  категория удалена между фазами → дроп + отчёт «категория удалена» (С2); слот удалён
  (DeleteSlot — сдвиг pos) или вручную заполнен между фазами → дроп + отчёт
  «слот занят/удалён» (С3);
- admin_resources: layerResourcesFromDB → []*resource.Resource (поля совпадают с
  LayerCatalog по значениям — сверка на сиде);
- frontend_test.go: граф админки резолвится (admin/auth.js → ../auth.js).

**Критерии приёмки (проверка @tester):**
1. **Fill (позитив, локально с opencode):** товар с пустым слотом + галка «заполнять
   ресурсом» на слоте → POST fill → 202 → state.generating=true → опросом state →
   generating=false, **state.proposals непуст** (kind new/link, category_valid, reason),
   попап «Предложения ИИ» открылся; принять пункт (Создать/Заполнить + категория) →
   POST apply → 200 → слоты заполнены **только принятые** (пропущенный слот пуст),
   новые товары — draft/source=ai, категория — выбранная в попапе, отчёт «пропущено»
   для отклонённых.
2. **Fill (негатив, без opencode):** POST fill → 202; state → report содержит
   «Ошибка ИИ» (честная ошибка «ИИ недоступен»); кнопка в UI работает (не disabled
   глобально), ошибка видна, proposals пусты. 400 при товаре без пустых слотов; 409 при
   повторном fill во время генерации; 404 при несуществующем товаре; ресурсу — 400
   (слотов нет).
3. **Попап (вручную):** варианты по пунктам (Заполнить/Пропустить, Создать/Пропустить),
   селект категорий для new (дефолт — категория ИИ, при невалидной — категория родителя
   + пометка «категория „Y" не найдена»), «Применить выбранное (N)» disabled при N=0,
   «Отмена» → cancel → proposals сброшены (попап не переоткрывается опросом);
   повторный fill сбрасывает старые proposals; **М1: «Отмена» → повторный fill → тот же
   набор ИИ → попап открывается снова (ключ сброшен)**; **М6: старт нового fill при
   открытом попапе → попап закрывается**.
4. **Дропы apply-фазы (С1–С3, вручную/API):** между proposals и apply удалить link-цель →
   apply → 200, отчёт содержит «ссылка исчезла (товар удалён/переименован)», новый товар
   НЕ создан; удалить категорию (через API категорий) → apply → 200, отчёт «категория
   удалена»; удалить/заполнить слот (DELETE slots или PUT slots) → apply → 200, отчёт
   «слот занят/удалён»; `{"applied": N}` — только фактически записанные (дропы не входят);
   товар удалён между фазами → apply → 200 + отчёт «товар не найден» (М2). Все дропы —
   в полный отчёт попапа.
5. **Конкурентность:** apply + параллельная мутация каталога (PUT slot) — без потери
   обновлений (advisory lock); `-race` чистый; два параллельных fill — второй 409;
   apply во время генерации — 409; cancel во время генерации — 409 (М5).
6. **UI:** кнопка «заполнить комплектующие» в попапе товара (disabled при генерации/
   без пустых слотов), пункт в контекстном меню канваса, отчёт через state-опрос,
   genIndicator «⚙ генерация…», modelInfo показывает модель.
7. **admin_resources:** «Базовый слой» — 20 ресурсов из БД (id — числовые строки,
   значения осей/T совпадают с LayerCatalog — сверка по выборке); покрытие рас и gaps —
   те же, что были (на дев-БД без правок студии); переименование layer-ресурса в студии
   → видно в подвкладке (С1, снятие С5); удаление layer-ресурса → исчезает из слоя и
   покрытия, при дыре — gaps показывает (не баг).
8. **Auth:** админка и студия работают как раньше (вход, 401-сброс, общий adminToken,
   переходы админка ↔ студия); frontend_test.go зелёный (граф резолвится);
   в студии нет запросов на старый `/api/*`.
8. **Удаление:** `cmd/goods-studio/`, `goods_data/`, `config/goods/`,
   `tools/e2e/goods-studio-check.js` удалены; `go build ./...` зелёный; старый смоук
   (8799) не запускается (цель удалена); `/studio` работает.
9. **e2e:** `goods-studio-server-check.js` — старые 12/12 + новый шаг: создание товара
   с пустым слотом → POST fill → 202 → опрос state до generating=false → report непуст
   («Ошибка ИИ» — opencode в CI недоступен, детерминировано; если opencode локально
   запущен — proposals непуст, попап открывается, apply применяет принятое) → cleanup.
   Таймаут опроса ≥ OPENCODE_TIMEOUT_S + 15 c (fill может висеть до таймаута клиента).
10. **DoD:** build/vet/test -race чистые.

---

## 12. Инварианты

1. **БД — источник правды (С1):** apply пишет в БД; admin_resources читает слой из БД;
   Go-каталог — только сид и эталон тестов.
2. **Ресурс — лист:** слоты ресурсам не создаются — обычные роуты слотов закрыты
   `checkGoodForSlots` (403, iterA §9.1); fill ресурсу — 400 «нет пустых слотов» на
   пречеке хендлера (§5.1), до транзакции — `checkGoodForSlots` в ApplyProposals-транзакции
   не нужен (§5.4); ApplyProposals работает только с recipe товара.
3. **Категория соответствует kind:** новые товары от ИИ — только в товарные категории
   (категория выбирается в попапе — из реестра товарных категорий; category_id NOT NULL,
   инвариант iterA №3 не нарушается).
4. **Имена уникальны:** новые товары от ИИ — UNIQUE name_norm (страховка 23505 → отчёт);
   существующие — ссылка (ApplyProposals).
5. **Граф — DAG:** цикл из ИИ-ответа — дроп на разборе (BuildProposals) и повторно на
   apply (ApplyProposals на свежем снимке под lock).
6. **Бан/исключённое — дроп:** BuildProposals/ApplyProposals не предлагают и не применяют
   banned/excluded; ресурс в слот без галки — дроп (контракт 99a.1 §7.4).
7. **Дропы apply-фазы — в полный отчёт:** link-цель исчезла → «ссылка исчезла (товар
   удалён/переименован)», НЕ переводится в kind=new (С1); категория удалена между
   фазами → «категория удалена» (С2, проверка в tx под lock); слот удалён/занят →
   «слот занят/удалён» (С3, re-check в tx под lock). Дропы не считаются в `applied`.
8. **Новые товары от ИИ:** draft + source=ai (не approved — согласование за создателем).
9. **Одна активная генерация:** TryStart; повторный fill — 409; apply во время
   генерации — 409; cancel во время генерации — 409.
10. **Proposals — in-memory и транзиентны:** сброс отменой (cancel), применением (apply)
    и повторным fill (tryStartFill); рестарт сервера сбрасывает (dev-инструмент).
11. **Применяется только принятое:** apply заполняет только принятые пункты по своим
    слотам (per-slot); пропущенные — фиксируются в отчёте как «пропущено».
12. **Отчёт и proposals — через state:** UI не знает о goroutine, только опрашивает
    /studio/api/state.
13. **Экспорт/импорт — не существуют** (решения создателя); вопрос §8 идеи закрыт.
14. **Каталог не очищается** (ClearUniverse не трогает categories/goods/goods_slots).
15. **Авторизация:** ядро одно; поведение админки/студии не меняется (консолидация).
16. **Кандидат ГД-сущности (§14) в числа/механику не входит** (анти-двойной учёт).

---

## 13. Сверка с @writer/@scientist / @balancetester

Не требуется: чисел баланса и физических вилок нет — перенос существующей ИИ-механики
(99a.1 §7) на БД и перевод читателя на тот же источник данных (значения не меняются —
меняется источник). Fill создаёт контент каталога (dev-инструмент), не игровые числа.
@balancetester не нужен (идея §6). Вердикт по части дизайнера: **B1** — влияния на
баланс/игру нет; риски архитектурные (удаление легаси, граф импортов фронта) —
управляемые и проверяемые критериями §11.

---

## 14. Кандидат ГД-сущности (Вопрос → Ответ)

Каждое задание приносит +1 кандидата; в текущие числа **не входит** (анти-двойной учёт):

- **«Конструкторское бюро»** — ИИ-заполнение комплектующих (fill) становится игровой
  услугой: игрок заказывает «проектирование товара» (генерацию рецепта) у фракционного
  бюро за ресурсы/деньги; результат — чертёж-рецепт (не разблокировка производства —
  это кандидат «Технологические чертежи», 99a.1 §16, а видимость/услуга — как
  «Промышленный атлас», iterA §14, но активная: заказал — получил рецепт). Отличие от
  атласа: атлас — покупка знаний «что производится», бюро — заказ «спроектируй состав».
  Перенос fill на сервер делает механику естественной: движок уже в игровом сервере.
  **Вопрос:** где игрок заказывает проектирование (фракционное бюро, контракт, рынок
  услуг) и что получает (полный рецепт, только верхний уровень, чертёж с качеством)?
  **Ответ (эталон создателя):** эталон не задан (2026-09-20) — открытый вопрос.

---

## 15. Связи механик (impact map)

Дополнения к реестру при сдаче (@manager; существующие связи iterA §15/iterB §13 остаются):

| Откуда | Куда | kind | Причина |
|---|---|---|---|
| `internal/goodsstudio/ai` (новый) | `goods` | writes | apply пишет новые товары (draft/ai) и слоты через `repo.ApplyProposals` (C) |
| `internal/goodsstudio/ai` | `internal/handlers/studio_handlers.go` | api | BuildFillPrompt/BuildProposals/ApplyProposals — движок fill (C) |
| `internal/handlers/studio_handlers.go` | `goods` | writes | fill/apply/cancel-хендлеры (C); state несёт generating/model/report/proposals |
| `web/studio.html` | `internal/handlers/studio_handlers.go` | api | кнопка fill + попап «Предложения ИИ» + apply/cancel + отчёт через state-опрос (C) |
| `internal/handlers/admin_resources.go` | `goods` | reads | «Базовый слой»/покрытие/gaps из БД (props ? 'closes'), C |
| `internal/resource/layer.go` | `internal/handlers/admin_resources.go` | affects | перестаёт быть источником рантайма (C); остаётся сидом и эталоном тестов |
| `web/static/js/auth.js` (новый) | `web/static/js/admin/auth.js` | api | ядро авторизации — админка делегирует (C) |
| `web/static/js/auth.js` | `web/static/js/studio/auth.js` (новый) | api | ядро авторизации — студия делегирует (C) |
| `web/static/js/admin/auth.js` | `web/studio.html` | duplicates | дублирование снято (C): инлайн-копия удалена, общий модуль |
| `cmd/goods-studio` | — | — | **сущность удаляется** (C): бинарь, goods_data, config/goods, старый смоук |
| `goods_data/*` | — | — | **сущность удаляется** (C) |
| `tools/e2e/goods-studio-check.js` | — | — | удаляется (C); новый смоук остаётся |

---

## 16. Дельта для @dispatcher (не правится руками)

- `docs/specs/README.md`: строка манифеста `перенос-студии-товаров-iterC-доводка.md`
  — **сейчас** (дизайн готов к проверке создателем).
- `99_roadmap.md` §99.2: строка переноса студии — **C ✅** (текст — в ответе дизайнера
  менеджеру); строка «Топовые товары» (идея 2026-09-19_топовые-товары-затравка-студии.md,
  «артефакт вне git `goods_data/`») — пометка «легаси удалён (C): `goods_data/` удалено,
  данные в игру не переносились (импорт отменён, решение 2026-09-20)» — строка
  фиксируется как история, не виснет на удалённом артефакте.
- `docs/ARCHITECTURE.md` §2: строка `cmd/goods-studio` (line 145) — **удаляется**;
  строка internal/goodsstudio (line 146) — + `ai/` (client/prompt/parse/apply, fill);
  строка iterB (line 147) — + fill UI, admin_resources слой из БД, auth.js;
  + строка iterC (fill-бэкенд, читатели БД, удаление легаси).
- `docs/PITFALLS.md`: ловушка «Студия embed'ит HTML» (line ~460) — устарела (бинарь
  удалён); ловушка «opencode serve: контракт HTTP-API» (line ~442) — ссылки
  `cmd/goods-studio/ai/client.go` → `internal/goodsstudio/ai/client.go` и
  `config/goods/studio.json` (`opencode_url`) → env `OPENCODE_URL` (конфиг удалён);
  добавить «opencode на проде недоступен — fill честно падает „Ошибка ИИ"
  (dev-инструмент, конфиг env OPENCODE_*)»; «студия на 8799 больше нет».
- `docs/INDEX.md`: строки старой студии (идеи 99a, спеки 99a.1/2/3, смоук) — пометка
  «легаси удалён (C)»; + спека iterC + QA-журнал.
- `docs/impact_map.json`: сущности `cmd/goods-studio`, `goods_data/*` — **удалить**;
  `internal/goodsstudio` — + ai/; `web/studio.html` — fill, auth-модуль;
  `admin_resources.go` — слой из БД; + новые сущности `internal/goodsstudio/ai`,
  `web/static/js/auth.js`, `web/static/js/studio/auth.js` (вносит @manager, §15).
- `docs/GLOSSARY.md` (кандидаты, 53a): «студия товаров» — fill работает, старая студия
  удалена; «ИИ-заполнение комплектующих» — движок в игровом сервере (если нужно).
- `docs/COORDINATION.md`: строки `cmd/goods-studio/*` — удалить; + новые файлы C.
- `CHANGELOG.md`, `.opencode/context.md`, `STATUS.md` — на сдаче.
- **Правка старых спек (правило «живые уточнения правят доки»):** `99a.1-goods-studio.md`
  §7 (fill) — пометка «перенесено в игровой сервер (iterC), двухфазный: предложи →
  подтверди в попапе (решение создателя 2026-09-20); категория нового товара —
  выбор в попапе (дефолт — категория родителя при невалидной от ИИ)»; `99a.2`/`99a.3` —
  пометка «импорт/экспорт отменены (решения 2026-09-20), легаси удалён» (вносит
  @designer по правилу одного писателя).

---

## 17. История итераций и гейт-вопросы

**Итерация 1 (2026-09-20):** чтение идеи §17 (решения C), спек iterA/iterB, кода:
`cmd/goods-studio/` (ai/ — client/prompt/parse/apply + 23 теста; handlers/state.go
handleFill/buildFillPrompt/runFill; http.go tryStartFill/finishFill; web/index.html
fillGood/кнопка/отчёт), `studio_handlers.go` (StateView, GoodByID-диспетчер),
`goods_repository.go` (beginMutation, loadGoods/loadSlots, RealResources), `seed.go`
(layerProps/realProps — ключи props), `admin_resources.go` (LayerCatalog/CheckAllFed/
CoveringResources вызовы), `resources.js` (id-ключи, coverage), `auth.js`/`admin.html`/
`studio.html` (инлайн-паттерн), `frontend_test.go` (граф импортов), `main.go` (роуты),
`config.go` (env-паттерн), impact map. Проверено: ничто не импортирует cmd/goods-studio;
13 файлов админки импортируют ./auth.js (12 — fetchWithAuth/getAdminToken, main.js —
ensureAdminAuth и др.); клиент витрины id-непрозрачен. Разворот подходов (§2):
10 развилок. Написана спека. Гейт: жду одобрения.

**Итерация 2 (2026-09-20, решение создателя):** fill — **«предложи → подтверди в
попапе»** вместо «примени сразу»: ответ ИИ разбирается в proposals (НЕ применяется),
попап «Предложения ИИ» с вариантами по каждому пункту (Создать/Заполнить/Пропустить/
Изменить категорию), «Применить выбранное»/«Отмена»; невалидная категория — выбор в
попапе (дефолт — категория родителя, пометка «не найдена») — закрывает развилку 4.
Внесено: разв. 10 (поток fill), разв. 3 → ApplyProposals (per-slot подмножество),
разв. 4 → выбор в попапе; §5 (три эндпоинта fill/apply/cancel, BuildProposals/
ApplyProposals, proposals в state), §6 (попап, состояния), §11 (тесты/критерии),
§12 (инварианты 9–10), §15 (impact map), §16 (дельта 99a.1 §7). Гейт: жду одобрения.

**Итерация 3 (2026-09-20, вердикт критика 2):** 3 средних + 7 мелких. Средние
(решения менеджера, звоночки на гейт): **С1** link-цель исчезла между фазами → дроп +
отчёт «ссылка исчезла», НЕ перевод в kind=new (ProposalItem.Kind); **С2** проверка
категории — в транзакции ApplyProposals под lock (TOCTOU), дроп «категория удалена»;
**С3** re-check слота «pos существует и пуст» под lock, дроп «слот занят/удалён».
Мелкие: М1 else-ветка сброса lastProposalsKey; М2 apply при удалённом товаре — 200 +
отчёт «товар не найден» (единообразно в таблице §5/§5.4/§11); М3 `{"applied": N}` —
счётчик фактически записанных в ApplyProposals (дропы не входят); М4 маппинг «gN»→realID
по id слота из мутированного state (не по имени); М5 cancel во время генерации — 409;
М6 старт нового fill закрывает открытый попап; М7 лишние компоненты ИИ — дроп «больше
запрошенного» на разборе. Внесено в §5 (таблица роутов, §5.4), §6 (п.5), §11 (тесты/
критерии), §12 (инварианты 5–7). Гейт: жду одобрения.

**Итерация 4 (2026-09-20, реализация):** ✅ **РЕАЛИЗОВАНА и ПРИНЯТА создателем** —
коммиты `7fef2d1` + `4d1abb5`; тесты 9/9 (DoD build/vet/test -race зелёные),
e2e-смоук `/studio` 14/14; приёмка создателя «ок». Удаление легаси
(`cmd/goods-studio/`, `goods_data/`, `config/goods/`, старый смоук) — отдельным
коммитом в зоне developer, сборка после удаления зелёная.

### Гейт-вопросы (с предложениями по умолчанию)

1. **Конфиг opencode (разв. 1):** env OPENCODE_URL/MODEL/TIMEOUT_S/MAX_RETRIES с
   дефолтами из studio.json (необязательные — на проде fill честно падает) — ок?
   Альтернатива — сохранить config/goods/studio.json.
2. **Статус fill + proposals (разв. 2):** in-memory в хендлере (TryStart + report +
   proposals в state) — ок? Альтернатива — таблица в БД (избыточна для транзиентного
   статуса).
3. **Применение (разв. 3):** `ApplyProposals` — per-slot применение подмножества
   (резолв повторяется на свежем снимке в tx; 17 apply-тестов адаптируются под
   per-slot-сигнатуру, семантика сохранена) — ок? Альтернатива — последовательный
   ApplyFill (не умеет подмножество).
4. ~~**Категория нового товара (разв. 4)**~~ — **решено создателем 2026-09-20**: выбор
   в попапе (дефолт — категория родителя при невалидной от ИИ, пометка «не найдена»);
   закрыто, не гейт.
5. **Читатели (разв. 5/6):** переиспользование CoveringResources/CheckAllFed на
   БД-данных; дискриминатор слоя `props ? 'closes'`; снятие ограничения С5 (правки
   layer-ресурса в студии видны в подвкладке) — подтвердить границу?
6. **Auth-модуль (разв. 7):** ядро web/static/js/auth.js + тонкие адаптеры
   (admin/auth.js — те же экспорты, студия — модуль с bootstrap) — ок?
   Альтернатива — прямой импорт ядра во все 13 файлов админки (дороже).
7. **Удаление (разв. 8/9):** вместе со старой студией удаляем config/goods/,
   goods_data/, старый смоук goods-studio-check.js и мёртвый код model
   (LoadState/SaveState/NextCategoryID/ResourceRef) — ок? Старые спеки 99a.1/2/3
   остаются как история.

---

## 18. Порядок работ (для @developer)

1. Конфиг opencode в config.go (§4) + тест дефолтов.
2. Move `cmd/goods-studio/ai/` → `internal/goodsstudio/ai/` + правка импортов старой
   студии (4 файла) + `BuildFillPrompt` + тест (промпт); адаптация 17 apply-тестов под
   per-slot-семантику `ApplyProposals`.
3. `ai.BuildProposals` (разбор в предложения, дропы) + `ai.ApplyProposals` (per-slot
   подмножество) + тесты (§5.4).
4. `repo.ApplyProposals` (write-back) + `repo.LayerResources` + sqlmock-тесты (§5.4/§7.2).
5. Fill-хендлеры: ветки fill/apply/cancel в GoodByID, TryStart/finishFill/setProposals/
   clearProposals, StateView.Report/Model/Generating/Proposals, main.go (ai.Client,
   NewStudioHandlers) + тесты хендлера (§5).
6. UI: кнопка fill + пункт меню + fillGood + попап «Предложения ИИ» (варианты, селект
   категорий, Применить/Отмена) + report-секция + modelInfo (§6).
7. admin_resources: layerResourcesFromDB + маппинг + подстановка в GetAdminResources (§7).
8. Auth: ядро auth.js + адаптеры админки и студии + studio.html (§8).
9. e2e: шаг fill (негатив) + apply (§11 п.9).
10. **Удаление** (отдельный коммит): cmd/goods-studio, goods_data, config/goods,
    старый смоук, мёртвый код model (§9); DoD после удаления.
11. Критерии приёмки §11 — прогон @tester.

После каждого шага — гейт: показать результат, ждать «го» на следующий шаг.