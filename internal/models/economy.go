package models

import "time"

type Settlement struct {
	ID         string    `json:"id"`
	PlanetID   string    `json:"planet_id"`
	Population int       `json:"population"`
	Stability  int       `json:"stability"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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