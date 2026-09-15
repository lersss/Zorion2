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
	Log             []SettlementLogEntry  `json:"log,omitempty"`          // последние записи лога (первый тип — «Вымерло»), 18a §«Лог поселения»
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

// SettlementLogEntry — запись лога поселения (18a_population_death.md, §«Лог
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
