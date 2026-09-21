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
