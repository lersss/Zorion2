package models

import "time"

type Settlement struct {
	ID              string                `json:"id"`
	PlanetID        string                `json:"planet_id"`
	Population      int                   `json:"population"`
	PopulationExact float64               `json:"population_exact"` // точное состояние для пересчёта, округляется в Population на выдаче (18a_population_death.md)
	Stability       int                   `json:"stability"`
	ComputedAt      time.Time             `json:"computed_at"`            // точка отсчёта Δt для ленивого пересчёта
	LambdaPerHour   float64               `json:"lambda_per_hour,omitempty"` // λ-компоненты изменения населения за час (холод/гравитация/радиоактивность), для косметической экстраполяции на клиенте (18a)
	RPerSec         float64               `json:"r_per_sec,omitempty"`       // рекурсивная компонента изменения за 1 секунду (жара, HeatChangeRate); при росте отрицательная (99.2.12)
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
	// EatByPosition — структура норм еды типа поселения (producer_types.params.eat),
	// внутреннее поле для слоя потребности (ключ — name_norm ПОЗИЦИИ корзины,
	// спека 2026-09-22-эффекты-снабжения §4.2); в JSON не выводится (нормы живут
	// в студии, §6).
	EatByPosition map[string]float64 `json:"-"`
	// EffectsByPosition — привязка «позиция корзины → name_norm типа эффекта»
	// (producer_types.params.effects, §4.2): множество позиций объекта. В JSON
	// не выводится (привязки — в студии).
	EffectsByPosition map[string]string `json:"-"`
	// Effects — действующие эффекты поселения (нагрузка/порог/состояние/сила),
	// витрина админа (§6); заполняется owner-проходом.
	Effects []ActiveEffect `json:"effects,omitempty"`
	// Branches — ветки поселения (спека 2026-09-22-поселение-ветка-буферы-
	// переработка §6): связь поселение ↔ рецепт каталога с входным/выходным
	// буфером и своей чек-точкой. Подтягиваются в attachSettlements после
	// ленивого пересчёта населения (§4.2); входной буфер виден только админу
	// (stripPlanetDetails обнуляет Input для player).
	Branches  []SettlementBranch `json:"branches,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// SettlementBranch — ветка поселения в ответе карточки (спека 2026-09-22-
// поселение-ветка-буферы-переработка §6): рецепт каталога (имя выхода и
// сложность), своя чек-точка processed_at, входной и выходной буферы. Входной
// буфер (Input) отдаётся только админу (§6 — выход вместе с деталями
// поселения); выход — «пол» по качеству (поле качества не заводим, §1).
type SettlementBranch struct {
	ID          string              `json:"id"`
	RecipeID    int64               `json:"recipe_id"`
	RecipeName  string              `json:"recipe_name,omitempty"`
	Complexity  int                 `json:"complexity,omitempty"`
	ProcessedAt time.Time           `json:"processed_at"`
	Output      []BranchBufferEntry `json:"output,omitempty"`
	Input       []BranchBufferEntry `json:"input,omitempty"`
	// Produced — произведено товара-выхода за последний проход переработки
	// (транзитное, не хранится; спека итерации 4 §6).
	Produced float64 `json:"produced,omitempty"`
	// Eaten — физически списано из выходного буфера ветки слоем потребности за
	// последний проход (транзитное; спека 2026-09-22-эффекты-снабжения §4.2:
	// единственная точка записи буфера — слой потребности).
	Eaten float64 `json:"eaten,omitempty"`
	// EatenRate — скорость списания из буфера, единиц/сек (витрина, §6).
	EatenRate float64 `json:"eaten_rate,omitempty"`
}

// BranchBufferEntry — запись буфера ветки «ресурс → количество» (таблица
// settlement_branch_buffers, §3.1): direction не сериализуется — вход и выход
// лежат отдельными массивами (Input/Output).
type BranchBufferEntry struct {
	GoodID   int64   `json:"good_id"`
	GoodName string  `json:"good_name"`
	Amount   float64 `json:"amount"`
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
