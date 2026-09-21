// internal/models/current_position.go
// Внутрисистемная позиция игрока (спека 99.2.27 §2.2): users.current_position
// JSONB, NULL = вне системы (в межзвёздном полёте) или нет данных.
// Позиция ЯВНО отражает внутрисистемный полёт (решение создателя 2026-09-20):
// покой — {status: orbit, object_type, object_id, level}; полёт —
// {status: in_flight, from, to, start_time, arrive_at}.
package models

import "time"

// CurrentPosition — внутрисистемная позиция игрока.
type CurrentPosition struct {
	Status     string `json:"status"`                // orbit | in_flight | surface
	ObjectType string `json:"object_type,omitempty"` // star|planet|satellite (orbit/surface)
	ObjectID   string `json:"object_id,omitempty"`   // UUID или синтетический id компаньона (orbit/surface)
	Level      string `json:"level,omitempty"`       // orbit | surface
	Biome      string `json:"biome,omitempty"`       // surface: form биома прогулки
	// HP и LandedAt — серверно-авторитетное здоровье прогулки (спека
	// 2026-09-21 §4.2/§8.7): hp ∈ [0,100], landed_at — UTC RFC3339. Пишет и
	// пересчитывает только сервер; клиент урон не присылает. Указатель — ноль
	// (смерть) значим и не теряется omitempty.
	HP        *float64 `json:"hp,omitempty"`
	LandedAt  string   `json:"landed_at,omitempty"`
	FromType  string   `json:"from_type,omitempty"` // star|planet|satellite (in_flight)
	FromID    string   `json:"from_id,omitempty"`
	ToType    string   `json:"to_type,omitempty"`
	ToID      string   `json:"to_id,omitempty"`
	StartTime int64    `json:"start_time,omitempty"` // UnixMilli (in_flight)
	ArriveAt  int64    `json:"arrive_at,omitempty"`  // UnixMilli (in_flight)
}

// OrbitPosition — позиция «на орбите объекта» (покой).
func OrbitPosition(objectType, objectID string) *CurrentPosition {
	return &CurrentPosition{
		Status:     "orbit",
		ObjectType: objectType,
		ObjectID:   objectID,
		Level:      "orbit",
	}
}

// StarOrbitPosition — дефолт «орбита звезды» (критик, мелкое 1; ИП-4 фолбэк).
func StarOrbitPosition(worldID string) *CurrentPosition {
	return OrbitPosition("star", worldID)
}

// InFlightPosition — позиция «внутрисистемный полёт идёт» (решение создателя:
// никакого «телепорта» — модалка при открытии сразу видит маршрут).
func InFlightPosition(fromType, fromID, toType, toID string, startTime, arriveAt time.Time) *CurrentPosition {
	return &CurrentPosition{
		Status:    "in_flight",
		FromType:  fromType,
		FromID:    fromID,
		ToType:    toType,
		ToID:      toID,
		StartTime: startTime.UnixMilli(),
		ArriveAt:  arriveAt.UnixMilli(),
	}
}

// SurfacePosition — позиция «игрок на поверхности планеты» (спека 2026-09-21
// §4.2): {status:"surface", level:"surface", object_type:"planet", object_id,
// biome, hp, landed_at}. hp/landed_at — серверно-авторитетное здоровье прогулки
// (§8.7); x/y не хранятся (решение создателя: «планета + биом»).
// landed_at — RFC3339 без долей секунды, округлённый ВВЕРХ до секунды: иначе
// усечение вниз сразу отнимало бы у игрока часть HP (hp считается от landed_at,
// §8.7) — на момент высадки hp ровно 100.
func SurfacePosition(planetID, biome string, hp float64, landedAt time.Time) *CurrentPosition {
	t := landedAt.UTC()
	if t.Nanosecond() != 0 {
		t = t.Truncate(time.Second).Add(time.Second)
	}
	return &CurrentPosition{
		Status:     "surface",
		ObjectType: "planet",
		ObjectID:   planetID,
		Level:      "surface",
		Biome:      biome,
		HP:         &hp,
		LandedAt:   t.Format(time.RFC3339),
	}
}