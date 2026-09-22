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
	OrbitCenter   string  `json:"orbit_center"`    // main / barycenter
	OrbitRadiusAU float64 `json:"orbit_radius_au"` // фактический радиус, а.е. (S: orbitRadiusByIndex; P: 3×sep)
	Circumbinary  bool    `json:"circumbinary"`    // P-планета вокруг барицентра пары

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

	// Биомы и зоны недр объектами (99.2.28 §9.3): финальная поверхность/недра.
	// Отсутствие ключа = старый мир (фолбэк nil, не ошибка); газовые гиганты
	// — пусто/отсутствует.
	Biomes     []Biome          `json:"biomes,omitempty"`
	Subterrain []SubterrainZone `json:"subterrain,omitempty"`

	// История формирования планеты (спека 2026-09-22-облако-этап-2-...
	// §6.5): типизированный список записей из planet.data["formation_history"].
	// Отсутствие ключа = старый мир (фолбэк nil, не ошибка); гиганты/экзотика
	// маркера не несут. Без знания о планете stripPlanetDetails обнуляет.
	FormationHistory []PlanetFormationEvent `json:"formation_history,omitempty"`

	// Ядро
	Core *PlanetCore `json:"core,omitempty"`

	// Спутники газовых гигантов
	IsGasGiant bool              `json:"is_gas_giant,omitempty"`
	Satellites []PlanetSatellite `json:"satellites,omitempty"`

	// Поселения планеты (источник населения)
	Settlements []Settlement `json:"settlements,omitempty"`

	// Фракции планеты (спека 2026-09-21-фабрики-релиз-2-столицы-фракций §6):
	// фракции, для которых планета родная (factions.homeworld_id). Приезжают
	// на систему вместе с планетами (attachFactionsAndBuildings); у player без
	// знания о планете сервер их не отдаёт (stripPlanetDetails).
	Factions []PlanetFaction `json:"factions,omitempty"`

	// Строения планеты (там же §6): сущность «строение», в первой итерации —
	// столицы фракций (building_type='capital'). Отдельный массив, связь
	// «владелец ↔ фракция» клиент собирает по owner_id.
	Buildings []PlanetBuilding `json:"buildings,omitempty"`

	// Залежи поверхности планеты (спека 2026-09-22-поселение-добыча-сырья-
	// биома-ленивый-буфер §5.2): «планета → конкретные залежи», отдаются
	// построчно (группирует клиент). Подтягиваются только карточкой системы
	// (GetPlanetsByWorldID → attachDeposits); light-пути залежей не несут,
	// без знания о планете stripPlanetDetails обнуляет.
	Deposits []SurfaceDeposit `json:"deposits,omitempty"`

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

// Belt — пояс малых тел — объект МИРА (спека
// 2026-09-21-пояса-малых-тел-объект-системы §3.2/§4.6/§4.8): агрегат малых
// тел (не именованное тело), хранится записью system_belts. Владелец — мир
// (world_id); типы открыты (kind). Потребитель — этап 2 (модалка/полёт);
// структура заводится сразу, публичных ручек в этапе 1 нет.
type Belt struct {
	ID         string  `json:"id"`
	WorldID    string  `json:"world_id"`
	Kind       string  `json:"kind"`         // asteroid | kuiper | oort | debris | dust_ring
	Name       string  `json:"name"`         // имя пояса (для модалки)
	OrbitIndex *int    `json:"orbit_index"`  // якорь-номер накрытой орбиты; null у Койпера/Оорта
	RadiusAU   float64 `json:"radius_au"`    // середина окна, а.е.
	WidthAU    float64 `json:"width_au"`     // протяжённость, а.е.
	Mass       float64 `json:"mass"`         // суммарная масса, M⊕
	BodySizeKm float64 `json:"body_size_km"` // типичный размер тела, км

	// Composition — доли породы/железа/льда (та же зона, что у планет,
	// compositionByZone); data — расширяемость (этап 3: data.resources).
	Composition map[string]float64     `json:"composition"`
	Visible     bool                   `json:"visible"`
	Data        map[string]interface{} `json:"data"`

	// IronRemaining — запас железа пояса, т (спека
	// 2026-09-22-пояса-малых-тел-этап-3-добыча §4/§8.4): nil = «нет данных»
	// (старый мир), 0 = «выработан», > 0 = запас. Игроку напрямую не отдаётся —
	// BeltView несёт только качественный remaining_level; admin видит значение.
	IronRemaining *float64 `json:"iron_remaining,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeltView — пояс малых тел в ответе игроку (спека
// 2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт §4.2): DTO пояса для
// модалки. Базовые поля (тип/имя/геометрия/типичное тело/масса) открыты как
// часть системы; composition — единственная «деталь», отдаётся только при
// знании (сканер в радиусе / присутствие, §4.3); data/visible/служебные —
// только admin. models.Belt (без omitempty) игроку не отдаётся.
type BeltView struct {
	ID         string  `json:"id"`
	WorldID    string  `json:"world_id"`
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	OrbitIndex *int    `json:"orbit_index"`
	RadiusAU   float64 `json:"radius_au"`
	WidthAU    float64 `json:"width_au"`
	Mass       float64 `json:"mass"`
	BodySizeKm float64 `json:"body_size_km"`
	// Composition — доли породы/железа/льда; отсутствует без знания (§4.3).
	Composition map[string]float64 `json:"composition,omitempty"`
	// BeltClass — класс пояса по composition.iron (богатый/средний/бедный,
	// «стоит ли лететь», §8.4-а); RemainingLevel — уровень остатка запаса
	// (полный/истощается/выработан, §8.4-б). Оба — только при знании пояса
	// (тот же гейт, что composition); без знания не отдаются (omitempty).
	BeltClass      string `json:"belt_class,omitempty"`
	RemainingLevel string `json:"remaining_level,omitempty"`
}

// Biome — биом поверхности планеты (99.2.28 §3.1): объект {form, share},
// form — id из справочника биомов, share — доля поверхности в %.
type Biome struct {
	Form  string  `json:"form"`
	Share float64 `json:"share"`
}

// SubterrainZone — зона недр планеты (99.2.28 §3.2): объект {type, share},
// type — id типа недр из справочника.
type SubterrainZone struct {
	Type  string  `json:"type"`
	Share float64 `json:"share"`
}

// PlanetFormationEvent — одна запись истории формирования планеты (спека
// 2026-09-22-облако-этап-2-... §6.1/§6.2): {type, payload}. Тип — один из
// formed_early / formed_late / migrated / ice_lost / stripped_embryo;
// payload открыт (поля добавляются внутри объекта). Пустые поля не пишутся.
type PlanetFormationEvent struct {
	Type string `json:"type"`
	// formed_early: t_form_myr, x_ice; formed_late: t_form_myr.
	TFormMyr float64 `json:"t_form_myr,omitempty"`
	XIce     float64 `json:"x_ice,omitempty"`
	// migrated: x_form, x_now, direction (inward/outward), factor.
	XForm     float64 `json:"x_form,omitempty"`
	XNow      float64 `json:"x_now,omitempty"`
	Direction string  `json:"direction,omitempty"`
	Factor    float64 `json:"factor,omitempty"`
	// ice_lost: fraction, residue (iron/bare_ice).
	Fraction float64 `json:"fraction,omitempty"`
	Residue  string  `json:"residue,omitempty"`
	// stripped_embryo: giant_orbit.
	GiantOrbit int `json:"giant_orbit,omitempty"`
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

// PlanetFaction — фракция планеты в ответе API (спека
// 2026-09-21-фабрики-релиз-2-столицы-фракций §6): id/name/type/color/description.
// Сила (strength) намеренно не показывается — генератор пишет заглушку 1 (§4.2).
type PlanetFaction struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// PlanetBuilding — строение планеты в ответе API (там же §6):
// id/building_type/owner_type/owner_id. Владелец полиморфный: owner_type —
// player/faction/agent, owner_id — id владельца (для столицы — factions.id).
type PlanetBuilding struct {
	ID           string `json:"id"`
	BuildingType string `json:"building_type"`
	OwnerType    string `json:"owner_type"`
	OwnerID      string `json:"owner_id"`
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
