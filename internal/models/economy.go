package models

import (
	"encoding/json"
	"time"
)

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
