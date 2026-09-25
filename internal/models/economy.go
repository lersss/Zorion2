package models

import (
	"encoding/json"
	"time"
)

// RComponent — ряд админской витрины состава R_total (идея 2026-09-25):
// Code — код компонента среды (natural/birth/heat/cold/gravity/radiation) или
// name_norm типа эффекта; Name — человекочитаемое имя (для рядов эффектов).
// Value — вклад за секунду в родном знаке R движка (+ убыль, − рост).
type RComponent struct {
	Code  string  `json:"code"`
	Name  string  `json:"name,omitempty"`
	Value float64 `json:"value"`
}

type Settlement struct {
	ID              string                `json:"id"`
	PlanetID        string                `json:"planet_id"`
	Population      int                   `json:"population"`
	PopulationExact float64               `json:"population_exact"` // точное состояние для пересчёта, округляется в Population на выдаче (18a_population_death.md)
	Stability       int                   `json:"stability"`
	ComputedAt      time.Time             `json:"computed_at"`            // точка отсчёта Δt для ленивого пересчёта
	LambdaPerHour   float64               `json:"lambda_per_hour,omitempty"` // λ-компоненты изменения населения за час (холод/гравитация/радиоактивность), для косметической экстраполяции на клиенте (18a)
	RPerSec         float64               `json:"r_per_sec,omitempty"`       // рекурсивная компонента изменения за 1 секунду (жара, HeatChangeRate); при росте отрицательная (99.2.12)
	// RBreakdown — админская витрина состава R_total (идея 2026-09-25): по ряду
	// на компонент. Среда — коды natural/birth/heat/cold/gravity/radiation, ряды
	// эффектов — человекочитаемый Name (effect_types.name). Знак — родной знак R
	// движка: + убыль, − рост. Игроку не отдаётся (planet_visibility обнуляет).
	RBreakdown []RComponent `json:"r_breakdown,omitempty"`
	NDead           float64               `json:"n_dead,omitempty"`          // порог обнуления, тот же NDead — для той же экстраполяции
	Log             []SettlementLogEntry  `json:"log,omitempty"`          // последние записи лога (первый тип — «Вымерло»), 18b §«Лог поселения»
// RaceID — раса поселения (спека 99.2.21 §2.3); пусто = легаси/люди.
	// Две расы на планете = два ряда settlements с разными race_id (§7.4).
	RaceID string `json:"race_id,omitempty"`
	// RaceName — человекочитаемое имя расы поселения (JSON-вывод, не колонка
	// БД): вычисляется в attachSettlements из каталога рас; пусто = легаси/люди.
	RaceName string `json:"race_name,omitempty"`
	// SettlementTypeID — тип поселения (FK producer_types, подтип «Поселения»,
	// спека итерации 4 §3.3); 0 = тип не задан (легаси/ручные фикстуры) → норма
	// еды по DefaultEatK.
	SettlementTypeID int64 `json:"type_id,omitempty"`
	// TypeName — имя типа поселения (JSON-вывод, не колонка БД), как RaceName.
	TypeName string `json:"type_name,omitempty"`
	// OwnerType/OwnerID — владелец поселения (спека 2026-09-24-постройка-
	// структур §3.1): player/faction/agent; пусто = владельца нет (легаси/Г1).
	OwnerType string `json:"owner_type,omitempty"`
	OwnerID   string `json:"owner_id,omitempty"`
	// OwnerName — человекочитаемое имя владельца (JSON-вывод, не колонка БД),
	// резолвится пакетно в attachSettlements (§10.3). Пусто → витрина
	// «NPC (без владельца)» (§10.2).
	OwnerName string `json:"owner_name,omitempty"`
	// Stage — витрина ступени (спека 2026-09-23 §11.3): пороги текущей ступени
	// и вход следующей; nil — тип вне ладдеры (карточка рисует только имя).
	// Заполняется owner-проходом из уже загруженной ладдеры. Пороги — конфиг,
	// не тайна: ни presence-, ни snapshot-путь ступень не чистит.
	Stage *SettlementStageView `json:"stage,omitempty"`
	// EatByPosition — структура норм еды типа поселения (producer_types.params.eat),
	// внутреннее поле для слоя потребности (ключ — name_norm ПОЗИЦИИ корзины,
	// спека 2026-09-22-эффекты-снабжения §4.2); в JSON не выводится (нормы живут
	// в студии, §6).
	EatByPosition map[string]float64 `json:"-"`
	// EffectsByPosition — привязка «позиция корзины → name_norm типа эффекта»
	// (producer_types.params.effects, §4.2): множество позиций объекта. В JSON
	// не выводится (привязки — в студии).
	EffectsByPosition map[string]string `json:"-"`
	// Effects — витрина действующих эффектов поселения, ключ `effects`.
	// interface{} — один JSON-ключ на две роли (спека 2026-09-23-орбита-планеты-
	// присутствие-и-снимок §5.4): админ получает []ActiveEffect (нагрузка/порог/
	// сила/владелец — как раньше), игрок в режиме presence — []EffectView
	// (только name/impact/state, без внутренних полей). Смена контейнера не
	// меняет ни load/load_at/кривые, ни админский JSON.
	Effects interface{} `json:"effects,omitempty"`
	// Branches — ветки поселения (спека 2026-09-22-поселение-ветка-буферы-
	// переработка §6): связь поселение ↔ рецепт каталога с входным/выходным
	// буфером и своей чек-точкой. Подтягиваются в attachSettlements после
	// ленивого пересчёта населения (§4.2); входной буфер виден только админу
	// (stripPlanetDetails обнуляет Input для player).
	Branches  []SettlementBranch `json:"branches,omitempty"`
	// Arithmetic — витрина арифметики на текущем населении по позициям
	// (спека 2026-09-23-стадии-поселения §8.1/§8.2): производим/потребляем/сверх.
	// Админ-вид полный; игроку (presence) — под настройкой видимости, снимок —
	// блок не несёт (strip всегда чистит, §8.3).
	Arithmetic []SettlementPositionArithmetic `json:"arithmetic,omitempty"`
	CreatedAt  time.Time                      `json:"created_at"`
	UpdatedAt  time.Time                      `json:"updated_at"`
}

// SettlementPositionArithmetic — арифметика одной позиции корзины на текущем
// населении, ед/сутки (спека 2026-09-23-стадии-поселения §8.2). NetPerDay < 0 —
// дефицит позиции.
//
// Строка нужды (§10.2 спеки 2026-09-24-потребление-по-товарам) живёт на этой же
// позиции: Need/Effect (ключ/тип нужды), CoveredShare/DeficitShare (покрытие/
// дефицит) и Norm (норма позиции). Поля нужды — внутри блока арифметики
// намеренно: видимость игроку/снимку чистит блок целиком (strip), без правки
// занятого planet_visibility.go (§10.3).
type SettlementPositionArithmetic struct {
	Position string `json:"position"`
	// NormPerDayPerBillion — норма позиции, «ед/сутки/млрд» (§10.2). Позиция без
	// эффекта нормы не несёт (0, ключ не выводится).
	NormPerDayPerBillion float64 `json:"norm,omitempty"`
	ProducedPerDay       float64 `json:"produced_per_day"`
	ConsumedPerDay       float64 `json:"consumed_per_day"`
	NetPerDay            float64 `json:"net_per_day"`
	// Need — ключ нужды (= effect_types.name_norm); Effect — тип эффекта. Пусто —
	// позиция без эффекта (нет нужды). CoveredShare = 1 − DeficitShare (§10.2).
	Need         string  `json:"need,omitempty"`
	Effect       string  `json:"effect,omitempty"`
	CoveredShare float64 `json:"covered_share"`
	DeficitShare float64 `json:"deficit_share"`
}

// SettlementStageView — витрина ступени поселения (спека 2026-09-23-стадии-
// поселения §11.3): пороги в людях. Exit = 0 — порог выхода не читается (пол);
// NextEnter = 0 — выше текущей ступени нет.
type SettlementStageView struct {
	Enter     float64 `json:"enter"`
	Exit      float64 `json:"exit,omitempty"`
	NextEnter float64 `json:"next_enter,omitempty"`
}

// SettlementBranchTake — «забираем» по компоненту рецепта ветки, ед/сутки
// (выход × quantity_i, §8.2). Игроку идёт как расчётная производная рецепта,
// не как содержимое входного буфера (склад закрыт stripBranchInputs).
type SettlementBranchTake struct {
	GoodID   int64   `json:"good_id"`
	GoodName string  `json:"good_name,omitempty"`
	PerDay   float64 `json:"per_day"`
}

// SettlementBranch — ветка поселения в ответе карточки (спека 2026-09-22-
// поселение-ветка-буферы-переработка §6): рецепт каталога (имя выхода и
// сложность), своя чек-точка processed_at. Буферы ветки сняты (спека
// 2026-09-25-внутреннее-хранилище §4.4/§4.5, ЧК2а): физический запас живёт в
// ячейках внутреннего хранилища поселения, а не в ветке.
type SettlementBranch struct {
	ID          string    `json:"id"`
	RecipeID    int64     `json:"recipe_id"`
	RecipeName  string    `json:"recipe_name,omitempty"`
	Complexity  int       `json:"complexity,omitempty"`
	ProcessedAt time.Time `json:"processed_at"`
	// Produced — произведено товара-выхода за последний проход переработки
	// (транзитное, не хранится; спека итерации 4 §6).
	Produced float64 `json:"produced,omitempty"`
	// Eaten — физически списано из выходного буфера ветки слоем потребности за
	// последний проход (транзитное; спека 2026-09-22-эффекты-снабжения §4.2:
	// единственная точка записи буфера — слой потребности).
	Eaten float64 `json:"eaten,omitempty"`
	// EatenRate — скорость списания из буфера, единиц/сек (витрина, §6).
	EatenRate float64 `json:"eaten_rate,omitempty"`
	// RatePerDayPerBillion — число скорости пары «тип поселения × рецепт»
	// (producer_recipes.rate, ед/сутки/млрд, спека 2026-09-23-стадии-поселения
	// §8.1/§11.3): nil = «не объявлено» (ключа нет вовсе или rate NULL), 0 —
	// объявленный ноль. Витринное поле: игроку — под настройкой, снимок не несёт.
	RatePerDayPerBillion *float64 `json:"rate,omitempty"`
	// NotInStageSet — рецепт ветки отсутствует в наборе рецептов текущей стадии
	// (producer_recipes типа поселения) → ветка не производит (§3.5). Признак
	// админ-вида; игроку — под настройкой видимости (strip §8.3 п.4).
	NotInStageSet bool `json:"not_in_stage_set,omitempty"`
	// Take — «забираем» по ветке (расход входа, §8.2): выход × quantity по
	// компонентам рецепта, ед/сутки. Player-безопасная производная рецепта.
	Take []SettlementBranchTake `json:"take,omitempty"`
	// DepositShare — доля «забираем», добранная из залежей за последний проход
	// (0..1, §8.2): доля ветки, не атрибуция по позиции.
	DepositShare float64 `json:"deposit_share,omitempty"`
}

// StorageCell — запись ячейки внутреннего хранилища (таблица
// settlement_storage_cells, спека 2026-09-25-внутреннее-хранилище-и-рождение-
// заказов §4.1, ЧК2а): одна на (владелец, товар). Владелец полиморфный —
// OwnerType `settlement`/`building` (CHECK схемы), OwnerID — id владельца без
// FK. CapShare — вес/доля ячейки (порог = размер × доля/Σ долей, §1.3); вид
// потребности и складской дефицит — производные, в записи не хранятся (§4.2).
type StorageCell struct {
	ID        int64   `json:"id"`
	OwnerType string  `json:"owner_type"`
	OwnerID   string  `json:"owner_id"`
	GoodID    int64   `json:"good_id"`
	Amount    float64 `json:"amount"`
	CapShare  float64 `json:"cap_share"`
}

// SettlementLogEntry — запись лога поселения (18b_settlement_log.md, §«Лог
// поселения»). Первый тип — 'extinct' (Вымерло); список открыт. occurred_at
// NOT NULL — запись без даты не существует (бэкфилл отменён, решение создателя
// 2026-09-14); cause — код причины ('heat'/'cold'/'gravity_high'/'gravity_low'/
// 'radiation'), NULL = неприменимо для будущих типов.
type SettlementLogEntry struct {
	ID           string     `json:"id"`
	SettlementID string     `json:"settlement_id"`
	Type         string     `json:"type"`
	OccurredAt   time.Time  `json:"occurred_at"`
	Cause        *string    `json:"cause,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// MarshalJSON — пустые служебные чек-точки не сериализуются (спека
// 2026-09-23-орбита-планеты-присутствие-и-снимок §3.1/§15): у player-витрины
// присутствия (playerSettlements) обнулены population_exact/computed_at/
// R-компоненты, у снимковых поселений чек-точки не заполнены вовсе — иначе
// клиент достроит население по формуле (extrapolate.js) и «замороженное» число
// поплывёт. created_at/updated_at витрина не обнуляет (спекой §15 они не
// запрещены) — omitempty лишь скрывает незаполненные значения; у админских
// поселений они заполнены, вывод не меняется.
func (s Settlement) MarshalJSON() ([]byte, error) {
	type alias Settlement
	out := struct {
		alias
		PopulationExact *float64   `json:"population_exact,omitempty"`
		ComputedAt      *time.Time `json:"computed_at,omitempty"`
		CreatedAt       *time.Time `json:"created_at,omitempty"`
		UpdatedAt       *time.Time `json:"updated_at,omitempty"`
	}{alias: alias(s)}
	if s.PopulationExact != 0 {
		v := s.PopulationExact
		out.PopulationExact = &v
	}
	if !s.ComputedAt.IsZero() {
		v := s.ComputedAt
		out.ComputedAt = &v
	}
	if !s.CreatedAt.IsZero() {
		v := s.CreatedAt
		out.CreatedAt = &v
	}
	if !s.UpdatedAt.IsZero() {
		v := s.UpdatedAt
		out.UpdatedAt = &v
	}
	return json.Marshal(out)
}

// MarshalJSON — пустой processed_at ветки не сериализуется (та же витрина
// снимка, §3.1): у админских/живых веток чек-точка заполнена, у снимка — нет.
func (b SettlementBranch) MarshalJSON() ([]byte, error) {
	type alias SettlementBranch
	out := struct {
		alias
		ProcessedAt *time.Time `json:"processed_at,omitempty"`
	}{alias: alias(b)}
	if !b.ProcessedAt.IsZero() {
		v := b.ProcessedAt
		out.ProcessedAt = &v
	}
	return json.Marshal(out)
}
