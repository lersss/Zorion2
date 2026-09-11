package models

import "time"

// Region — регион галактики (сектор вокруг кластерного центра).
// Используется для отображения карты на малом зуме.
type Region struct {
	ID         string
	Name       string
	CenterX    float64
	CenterY    float64
	Radius     float64
	Color      string
	WorldCount int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}