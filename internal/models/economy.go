package models

import "time"

type Settlement struct {
	ID              string    `json:"id"`
	PlanetID        string    `json:"planet_id"`
	Population      int       `json:"population"`
	PopulationExact float64   `json:"population_exact"` // точное состояние для пересчёта, округляется в Population на выдаче (18a_population_death.md)
	Stability       int       `json:"stability"`
	ComputedAt      time.Time `json:"computed_at"` // точка отсчёта Δt для ленивого пересчёта
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Factory struct {
	ID            string    `json:"id"`
	PlanetID      string    `json:"planet_id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`           // добывающий, перерабатывающий, сборочный
	InputResource string    `json:"input_resource"` // свойство ресурса (металл, органика, энергия)
	OutputProduct string    `json:"output_product"` // название товара
	Quality       int       `json:"quality"`        // базовое качество производимого товара
	Status        string    `json:"status"`         // active, idle, building
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type GoodsBatch struct {
	ID          string     `json:"id"`
	PlanetID    string     `json:"planet_id"`
	ProductName string     `json:"product_name"`
	Quantity    int        `json:"quantity"`
	Quality     int        `json:"quality"`
	ProducerID  *string    `json:"producer_id,omitempty"`
	ProducedAt  time.Time  `json:"produced_at"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}