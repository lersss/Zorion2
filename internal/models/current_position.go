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
	Status     string `json:"status"` // orbit | in_flight
	ObjectType string `json:"object_type,omitempty"` // star|planet|satellite (orbit)
	ObjectID   string `json:"object_id,omitempty"`   // UUID или синтетический id компаньона (orbit)
	Level      string `json:"level,omitempty"`       // orbit (surface + biome — задел под высадку, решение А)
	Biome      string `json:"biome,omitempty"`
	FromType   string `json:"from_type,omitempty"` // star|planet|satellite (in_flight)
	FromID     string `json:"from_id,omitempty"`
	ToType     string `json:"to_type,omitempty"`
	ToID       string `json:"to_id,omitempty"`
	StartTime  int64  `json:"start_time,omitempty"` // UnixMilli (in_flight)
	ArriveAt   int64  `json:"arrive_at,omitempty"`  // UnixMilli (in_flight)
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