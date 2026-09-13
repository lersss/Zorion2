// internal/models/planet.go
package models

import "time"

// Planet — планета в API-ответе. Поля заполняются из JSON-колонки data
// в planet_repo.go.
type Planet struct {
	// Идентификация
	ID         string `json:"id"`
	WorldID    string `json:"world_id"`
	Name       string `json:"name"`
	OrbitIndex int    `json:"orbit_index"`

	// Геймдизайнерский тип
	Type            string `json:"type"`             // для обратной совместимости
	SurfaceDominant string `json:"surface_dominant"` // доминирующая форма

	// Физика
	Size         float64 `json:"size"`          // радиус, в земных
	Mass         float64 `json:"mass"`          // масса, в земных
	Density      float64 `json:"density"`       // в единицах Земли
	Temperature  float64 `json:"temperature"`   // K
	WaterPercent float64 `json:"water_percent"` // 0–100

	// Атмосфера и биосфера
	Atmosphere  string `json:"atmosphere"`
	Hydrosphere string `json:"hydrosphere"`
	Biosphere   string `json:"biosphere"`
	Climate     string `json:"climate"`

	// Жизнь
	Habitable bool `json:"habitable"`
	Life      bool `json:"life"`

	// Население — вычисляется из поселений (SUM settlements.population).
	Population int64 `json:"population"`

	// Композиции (форма → процент)
	SurfaceComposition    map[string]float64 `json:"surface_composition"`
	SubterrainComposition map[string]float64 `json:"subterrain_composition"`

	// Ядро
	Core *PlanetCore `json:"core,omitempty"`

	// Спутники газовых гигантов
	IsGasGiant bool              `json:"is_gas_giant,omitempty"`
	Satellites []PlanetSatellite `json:"satellites,omitempty"`

	// Поселения планеты (источник населения)
	Settlements []Settlement `json:"settlements,omitempty"`

	// Прочее
	Description string    `json:"description,omitempty"`
	SystemAge   float64   `json:"system_age,omitempty"`
	Moons       int       `json:"moons,omitempty"`
	Radioactive bool      `json:"radioactive,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PlanetCore — ядро планеты.
type PlanetCore struct {
	Type          string  `json:"type"`
	MassPercent   float64 `json:"mass_percent"`
	Activity      float64 `json:"activity"`
	Radioactivity float64 `json:"radioactivity"`
	Age           float64 `json:"age"`
	IsActive      bool    `json:"is_active"`
	IsMetallic    bool    `json:"is_metallic"`
}

// PlanetSatellite — спутник газового гиганта.
// Используется как полноценная локация (композиция, температура, жизнь).
type PlanetSatellite struct {
	ID                    string             `json:"id"`
	Name                  string             `json:"name"`
	OrbitIndex            int                `json:"orbit_index"`
	Size                  float64            `json:"size"`
	Mass                  float64            `json:"mass"`
	Temperature           float64            `json:"temperature"`
	WaterPercent          float64            `json:"water_percent"`
	Habitable             bool               `json:"habitable"`
	Life                  bool               `json:"life"`
	Atmosphere            string             `json:"atmosphere"`
	Biosphere             string             `json:"biosphere"`
	SurfaceDominant       string             `json:"surface_dominant"`
	SurfaceComposition    map[string]float64 `json:"surface_composition"`
	SubterrainComposition map[string]float64 `json:"subterrain_composition"`
	Description           string             `json:"description,omitempty"`
}