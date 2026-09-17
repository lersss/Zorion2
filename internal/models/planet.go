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

	// Орбитальный контекст (35b §2.2): вокруг чего обращается планета.
	// Заполняется из planets.data; старые миры без ключа — "main" (фолбэк §2.4).
	OrbitCenter    string  `json:"orbit_center"`     // main / barycenter
	OrbitRadiusAU  float64 `json:"orbit_radius_au"`  // фактический радиус, а.е. (S: orbitRadiusByIndex; P: 3×sep)
	Circumbinary   bool    `json:"circumbinary"`     // P-планета вокруг барицентра пары

	// Физика
	Size         float64 `json:"size"`          // радиус, в земных
	Mass         float64 `json:"mass"`          // масса, в земных
	Density      float64 `json:"density"`       // в единицах Земли
	Temperature  float64 `json:"temperature"`   // K
	Gravity      float64 `json:"gravity"`       // в земных (g)
	WaterPercent float64 `json:"water_percent"` // 0–100

	// Атмосфера и биосфера
	Atmosphere  string `json:"atmosphere"`
	Hydrosphere string `json:"hydrosphere"`
	Biosphere   string `json:"biosphere"`
	Archetype   string `json:"archetype"`

	// Атмосфера-объект и флаг жидкой воды (99.2.20 §4.1): новые поля каскада.
	// Старые планеты (до 45a) — без ключей: нули/false (фолбэки §7).
	AtmosphereData      map[string]interface{} `json:"atmosphere_data,omitempty"`
	LiquidWaterPossible bool                   `json:"liquid_water_possible,omitempty"`
	OrbitalPeriod       float64                `json:"orbital_period,omitempty"`
	Eccentricity        float64                `json:"eccentricity,omitempty"`
	EscapeVelocity      float64                `json:"escape_velocity,omitempty"`
	TidalLock           bool                   `json:"tidal_lock,omitempty"`

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

	// Knowledge — видимость знания о планете для модалки (спека 77a §6.2):
	// заполняется сервером для role=player (поверхность + наличие поселений
	// с датой актуальности); null для admin/skycomposer и без знания —
	// клиент показывает «нет данных — купить отчёт».
	Knowledge *PlanetKnowledgeView `json:"knowledge"`

	// Прочее
	Description string    `json:"description,omitempty"`
	SystemAge   float64   `json:"system_age,omitempty"`
	Moons       int       `json:"moons,omitempty"`
	Radioactive bool      `json:"radioactive,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PlanetKnowledgeView — знание игрока о планете в ответе модалки (спека 77a
// §6.2/§8.2): поверхность и наличие поселений с датой актуальности (И8).
// Недра/атмосфера/детали поселений сканер не вскрывает — их нет в ответе.
type PlanetKnowledgeView struct {
	ScannedAt          time.Time          `json:"scanned_at"` // дата актуальности (момент скана)
	Fresh              bool               `json:"fresh"`      // актуально (≤ 7 дней, §8.2)
	SurfaceDominant    string             `json:"surface_dominant,omitempty"`
	SurfaceComposition map[string]float64 `json:"surface_composition,omitempty"`
	SettlementsCount   int                `json:"settlements_count,omitempty"` // есть/нет + число (§6.2)
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