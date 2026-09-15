package models

import "time"

type World struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	CoordX         float64   `json:"coord_x"`
	CoordY         float64   `json:"coord_y"`
	SpectralClass  string    `json:"spectral_class"` // O, B, A, F, G, K, M, L, T, Y; у экзотики — пусто (NULL)
	Temperature    int       `json:"temperature"`    // в Кельвинах
	StarType       string    `json:"star_type"`      // star/white_dwarf/neutron/black_hole/protostar (99.2.4 §2)
	SystemType     string    `json:"system_type"`    // single/binary/multiple (99.2.4 §2)
	StellarMods    *StellarMods `json:"stellar_mods,omitempty"` // модификаторы (99.2.4 §4.3)
	StellarMass    *float64  `json:"stellar_mass,omitempty"` // масса звезды в M☉ (29a §4м); NULL у старых миров
	Population     int64     `json:"population,omitempty"` // население мира (сумма поселений), заполняется в админке «Миры»
	PopulationTrend string   `json:"population_trend,omitempty"` // тренд населения мира: decline/growth/stable (админка «Миры»)
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}