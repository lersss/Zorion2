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
	Status     string `json:"status"`                // orbit | in_flight | surface | mining
	ObjectType string `json:"object_type,omitempty"` // star|planet|satellite|belt (orbit/surface/mining)
	ObjectID   string `json:"object_id,omitempty"`   // UUID или синтетический id компаньона (orbit/surface/mining)
	Level      string `json:"level,omitempty"`       // orbit | surface | mining
	Biome      string `json:"biome,omitempty"`       // surface: form биома прогулки
	// HP и LandedAt — серверно-авторитетное здоровье прогулки (спека
	// 2026-09-21 §4.2/§8.7): hp ∈ [0,100], landed_at — UTC RFC3339. Пишет и
	// пересчитывает только сервер; клиент урон не присылает. Указатель — ноль
	// (смерть) значим и не теряется omitempty.
	HP       *float64 `json:"hp,omitempty"`
	LandedAt string   `json:"landed_at,omitempty"`
	// Заход в пояс (спека 2026-09-22-пояса-малых-тел-этап-3-добыча §3): состояние
	// мини-игры добычи — аддитивные ключи позиции (как hp/landed_at у surface).
	// Mined — буфер захода (т, серверно-авторитетный), указатель: ноль значим и
	// не теряется omitempty. StartedAt/LastCollectAt — UTC RFC3339.
	StartedAt string   `json:"started_at,omitempty"`
	Mined     *float64 `json:"mined,omitempty"`
	// MinedIce — второй буфер захода (лёд → «Вода неочищенная», спека
	// 2026-09-24 §5.7): аддитивный ключ к mined. Указатель: ноль значим и не
	// теряется omitempty. Норма чтения: отсутствует в старой позиции → 0.
	MinedIce      *float64 `json:"mined_ice,omitempty"`
	LastCollectAt string   `json:"last_collect_at,omitempty"`
	FromType      string   `json:"from_type,omitempty"` // star|planet|satellite|belt (in_flight)
	FromID        string   `json:"from_id,omitempty"`
	ToType        string   `json:"to_type,omitempty"`
	ToID          string   `json:"to_id,omitempty"`
	StartTime     int64    `json:"start_time,omitempty"` // UnixMilli (in_flight)
	ArriveAt      int64    `json:"arrive_at,omitempty"`  // UnixMilli (in_flight)
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

// MiningPosition — позиция «игрок добывает в поясе» (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §3): {status:"mining", level:"mining",
// object_type:"belt", object_id, started_at, mined:0, last_collect_at}.
// Буфер захода (mined) стартует с нуля; состояние персистится в JSONB позиции
// (новой таблицы нет). started_at — UTC RFC3339.
func MiningPosition(beltID string, startedAt time.Time) *CurrentPosition {
	t := startedAt.UTC().Format(time.RFC3339)
	mined := 0.0
	minedIce := 0.0
	return &CurrentPosition{
		Status:     "mining",
		ObjectType: "belt",
		ObjectID:   beltID,
		Level:      "mining",
		StartedAt:  t,
		Mined:      &mined,
		MinedIce:   &minedIce,
		// last_collect_at — nano-точность: секундная гранулярность давала бы
		// Δ ≈ 1 c двум сборам в одну секунду (обход клампа скорости §5.3.1).
		LastCollectAt: startedAt.UTC().Format(time.RFC3339Nano),
	}
}
