package models

import "time"

type World struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	CoordX         float64   `json:"coord_x"`
	CoordY         float64   `json:"coord_y"`
	SpectralClass  string    `json:"spectral_class"` // O, B, A, F, G, K, M, L, T, Y
	Temperature    int       `json:"temperature"`    // в Кельвинах
	Population     int64     `json:"population,omitempty"` // население мира (сумма поселений), заполняется в админке «Миры»
	PopulationTrend string   `json:"population_trend,omitempty"` // тренд населения мира: decline/growth/stable (админка «Миры»)
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}