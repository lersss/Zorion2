// internal/routegame/field.go
// Генератор поля мини-игры «Прокладка маршрута» (ЧК3, подэтап S1a; спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md §6.1): нормализованный
// план [0,1]² (y вниз, как canvas), детерминированный от seed и дальности
// сегмента. Поле — чистая функция от seed + dist + Passport: не зависит от
// «живого» остатка/прогресса и не использует время (§6.1/§5.3). RNG — локальный
// (мультиплеер, §0).
package routegame

import (
	"hash/fnv"
	"math/rand"
)

// Типы объектов поля (§6.1).
const (
	beaconType      = "beacon"       // обязательный маяк
	falseSignalType = "false_signal" // ложный сигнал (обманка)
	hazardType      = "hazard"       // зона потери времени
)

// Константы генератора поля — ГИПОТЕЗЫ, калибруются после наигрыша (спека §6.1).
// Ориентиры играбельности: маяков 3–6, ложных 2–4, зон 1–3, радиус захвата
// ~0.03–0.05 поля, коэффициент зоны ~1.5–1.8.
const (
	beaconMin      = 3      // минимальное число обязательных маяков
	beaconMax      = 6      // максимальное число обязательных маяков
	beaconDistStep = 7000.0 // единиц dist на +1 обязательный маяк

	falseSignalBase = 2 // ложных сигналов на «спокойном» маршруте
	falseSignalMax  = 4 // максимум ложных сигналов

	zoneBase = 1 // зон потери времени на «спокойном» маршруте
	zoneMax  = 3 // максимум зон

	captureRadius         = 0.04 // радиус захвата маяка/ложного сигнала, доля поля
	endpointCaptureRadius = 0.06 // радиус захвата СТАРТА/ФИНИША (гипотеза)
	objectClearance       = 0.02 // минимальный зазор объекта до СТАРТА/ФИНИША

	zoneRadiusMin = 0.10 // минимальный радиус зоны потери времени
	zoneRadiusMax = 0.16 // максимальный радиус зоны потери времени
	zoneCoeffMin  = 1.5  // минимальный коэффициент «дороже» внутри зоны
	zoneCoeffMax  = 1.8  // максимальный коэффициент зоны

	fieldMargin     = 0.06 // отступ объектов от края поля
	endpointSpreadX = 0.14 // разброс СТАРТА/ФИНИША по X (разнесены к краям)
	endpointYMin    = 0.15 // нижняя граница Y для СТАРТА/ФИНИША
	endpointYMax    = 0.85 // верхняя граница Y для СТАРТА/ФИНИША
	nodeSeparation  = 0.03 // минимальный зазор между узлами

	maxPlaceAttempts = 40 // попыток размещения до детерминированного сеточного прохода
	fallbackGrid     = 16 // шаг запасной сетки размещения (RNG не тратит)
)

// Пороги «беспокойного» характера пространства (спека §5.4) — ГИПОТЕЗЫ, калибруются.
const (
	unrestExoticStar     = 1    // star_type ≠ star
	unrestMultipleSystem = 1    // system_type ≠ single
	unrestHighTemp       = 1    // высокая температура
	unrestBelt           = 1    // пояс в системе назначения
	highTempK            = 8000 // порог «высокой температуры», K
)

// Point — точка нормализованного плана [0,1]².
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// FieldNode — узел поля: обязательный маяк или ложный сигнал (§6.1).
type FieldNode struct {
	Type string  `json:"type"` // beacon | false_signal
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	R    float64 `json:"r"` // радиус захвата, доля поля
}

// FieldZone — зона потери времени: путь внутри дороже в Coefficient раз (§6.1).
type FieldZone struct {
	Type        string  `json:"type"` // hazard
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	R           float64 `json:"r"`
	Coefficient float64 `json:"coefficient"` // ≥ 1
}

// Field — поле мини-игры: нормализованный план [0,1]². Nodes/Zones — не nil.
type Field struct {
	Seed   int64       `json:"seed"`
	Start  Point       `json:"start"`
	Finish Point       `json:"finish"`
	Nodes  []FieldNode `json:"nodes"`
	Zones  []FieldZone `json:"zones"`
}

// HashSeed — стабильный seed пары миров (§5.3): hash(from_world_id,
// to_world_id). Не зависит от времени; направление A→B даёт один «характер».
func HashSeed(fromWorldID, toWorldID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fromWorldID))
	_, _ = h.Write([]byte{0}) // разделитель: ("ab","c") ≠ ("a","bc")
	_, _ = h.Write([]byte(toWorldID))
	return int64(h.Sum64())
}

// GenerateField строит поле из seed, дальности сегмента dist и паспорта (§6.1).
// Число обязательных маяков растёт с dist (§5.4); «беспокойный» паспорт даёт
// больше/дороже зон и ложных сигналов, «спокойный» — чище. СТАРТ и ФИНИШ
// разнесены к краям поля. Детерминированно: тот же вход → байт-в-байт то же поле.
func GenerateField(seed int64, dist float64, passport Passport) Field {
	rng := rand.New(rand.NewSource(seed))
	field := Field{
		Seed:  seed,
		Nodes: make([]FieldNode, 0, beaconMax+falseSignalMax),
		Zones: make([]FieldZone, 0, zoneMax),
	}
	field.Start = endpoint(rng, true)
	field.Finish = endpoint(rng, false)

	unrest := passportUnrest(passport)
	nBeacons := numBeacons(dist)
	nFalse := falseSignalBase + min(unrest, falseSignalMax-falseSignalBase)
	nZones := zoneBase + min(unrest, zoneMax-zoneBase)

	for i := 0; i < nBeacons; i++ {
		field.Nodes = append(field.Nodes, generateNode(rng, beaconType, field, captureRadius))
	}
	for i := 0; i < nFalse; i++ {
		field.Nodes = append(field.Nodes, generateNode(rng, falseSignalType, field, captureRadius))
	}
	for i := 0; i < nZones; i++ {
		field.Zones = append(field.Zones, generateZone(rng, field))
	}
	return field
}

// numBeacons — число обязательных маяков: 3..6, растёт с дальностью (§5.4).
func numBeacons(dist float64) int {
	if dist < 0 {
		dist = 0
	}
	return min(beaconMin+int(dist/beaconDistStep), beaconMax)
}

// passportUnrest — «беспокойство» пространства по паспорту (§5.4): экзотические
// звёзды, кратные системы, высокая температура, пояс в системе назначения.
func passportUnrest(p Passport) int {
	u := 0
	for _, s := range []PassportStar{p.From, p.To} {
		if s.StarType != "" && s.StarType != "star" {
			u += unrestExoticStar
		}
		if s.SystemType != "" && s.SystemType != "single" {
			u += unrestMultipleSystem
		}
		if s.Temperature >= highTempK {
			u += unrestHighTemp
		}
	}
	if len(p.DestinationBelts) > 0 {
		u += unrestBelt
	}
	return u
}

// endpoint — СТАРТ (start=true) у левого края, ФИНИШ — у правого; по Y — в
// средней полосе. Разнесены, чтобы путь был содержательным.
func endpoint(rng *rand.Rand, start bool) Point {
	x := fieldMargin + rng.Float64()*endpointSpreadX
	if !start {
		x = 1 - x
	}
	y := endpointYMin + rng.Float64()*(endpointYMax-endpointYMin)
	return Point{X: x, Y: y}
}

// generateNode размещает узел заданного типа с радиусом захвата r.
func generateNode(rng *rand.Rand, typ string, f Field, r float64) FieldNode {
	p := placePoint(rng, func(p Point) bool {
		return clearOfEndpoints(p, r, f) && clearOfNodes(p, r, f.Nodes)
	})
	return FieldNode{Type: typ, X: p.X, Y: p.Y, R: r}
}

// generateZone размещает зону потери времени: радиус и коэффициент — из RNG.
func generateZone(rng *rand.Rand, f Field) FieldZone {
	r := zoneRadiusMin + rng.Float64()*(zoneRadiusMax-zoneRadiusMin)
	coeff := zoneCoeffMin + rng.Float64()*(zoneCoeffMax-zoneCoeffMin)
	p := placePoint(rng, func(p Point) bool {
		return clearOfEndpoints(p, r, f) && clearOfZones(p, r, f.Zones)
	})
	return FieldZone{Type: hazardType, X: p.X, Y: p.Y, R: r, Coefficient: coeff}
}

// placePoint ищет точку, свободную по предикату free: сначала случайными
// попытками, затем детерминированный проход по сетке (гарантия валидности, если
// свободная точка существует; RNG на сетке не тратится).
func placePoint(rng *rand.Rand, free func(Point) bool) Point {
	for i := 0; i < maxPlaceAttempts; i++ {
		p := randomFieldPoint(rng)
		if free(p) {
			return p
		}
	}
	for gy := 0; gy < fallbackGrid; gy++ {
		for gx := 0; gx < fallbackGrid; gx++ {
			p := Point{
				X: fieldMargin + (1-2*fieldMargin)*float64(gx)/float64(fallbackGrid-1),
				Y: fieldMargin + (1-2*fieldMargin)*float64(gy)/float64(fallbackGrid-1),
			}
			if free(p) {
				return p
			}
		}
	}
	return randomFieldPoint(rng) // недостижимо при разумном числе объектов
}

// randomFieldPoint — равномерная точка внутри поля с отступом от края.
func randomFieldPoint(rng *rand.Rand) Point {
	span := 1 - 2*fieldMargin
	return Point{
		X: fieldMargin + rng.Float64()*span,
		Y: fieldMargin + rng.Float64()*span,
	}
}

// clearOfEndpoints — объект радиуса r не налезает на СТАРТ/ФИНИШ (с зазором).
func clearOfEndpoints(p Point, r float64, f Field) bool {
	keep := r + endpointCaptureRadius + objectClearance
	return dist(p, f.Start) >= keep && dist(p, f.Finish) >= keep
}

// clearOfNodes — узел не налезает на уже размещённые узлы (с зазором).
func clearOfNodes(p Point, r float64, nodes []FieldNode) bool {
	for _, n := range nodes {
		if dist(p, Point{X: n.X, Y: n.Y}) < r+n.R+nodeSeparation {
			return false
		}
	}
	return true
}

// clearOfZones — зона не налезает на уже размещённые зоны: запрещаем полное
// вложение (центр внутри радиуса соседа); частичное перекрытие допустимо.
func clearOfZones(p Point, r float64, zones []FieldZone) bool {
	for _, z := range zones {
		if dist(p, Point{X: z.X, Y: z.Y}) < max(r, z.R) {
			return false
		}
	}
	return true
}
