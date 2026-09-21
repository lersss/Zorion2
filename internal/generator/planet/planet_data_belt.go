// internal/generator/planet/planet_data_belt.go
//
// Пояса малых тел — объекты мира (спека
// 2026-09-21-пояса-малых-тел-объект-системы, этап 1б). Пояс — АГРЕГАТ
// малых тел (не именованное тело): массив тел/Плутон-класс снят (§4.3).
// Владелец — мир (`world_id`): у компонента кратной свой диск ⇒ свои пояса.
//
// Поток RNG не сдвигается (§4.7): масса пояса детерминирована
// (`κ_kind·w⁰_зон·M_диск`), состав и имя берутся с ВЫДЕЛЕННОГО генератора
// (выводится из `world_id`+`kind`), а не из `g.rng` — иначе пояса сдвинули
// бы значения планет мира и последующих миров.
package planet

import (
	"hash/fnv"
	"math"
	"math/rand"

	"github.com/google/uuid"
	"zorion/internal/names"
)

// ==================== ПАРАМЕТРЫ ПОЯСОВ (§4.1–§4.4) ====================
const (
	// beltWindowMidPrc/beltWindowWidthPrc — резонансное окно гиганта 3:1–2:1
	// (§4.1): r_3:1 = 0.4807·r_g, r_2:1 = 0.6300·r_g. radius_au — середина
	// окна, width_au — протяжённость. Якорь (orbit_index = g−1) ≠ середина.
	beltWindowMidPrc   = 0.5555 // (0.4807 + 0.6300) / 2
	beltWindowWidthPrc = 0.1493 // 0.6300 − 0.4807

	// cloudOuterX — внешняя граница диска x_out (НОРМИРОВАННАЯ величина,
	// константа этапа 1; ролл отложен — РБ6). r_out = x_out·√L.
	cloudOuterX = 35.0

	// κ_kind — удержание зональной доли профиля (§4.2): M_пояс = κ_kind·w⁰_зон·M_диск,
	// 0 < κ ≤ 1. Значения — оценка дизайнера, калибровка под реальные массы
	// поясов за @balancetester (В4): астероидный ≈ 0.0005 M⊕ (κ ≈ 10⁻⁴–10⁻³),
	// Койпера ≈ 0.02–0.1 M⊕.
	kappaAsteroid = 2.5e-4
	kappaKuiper   = 0.05

	// kuiperProfileTail — w⁰_tail: фиксированная доля профиля ЗА пределами
	// лестницы (непрерывного хвоста нет — профиль дискретен, §4.2). В S₀ не
	// входит; константа типа, значение за @balancetester.
	kuiperProfileTail = 0.1

	// kuiperWidthFactor — width_au = 0.35·radius_au (§4.3).
	kuiperWidthFactor = 0.35
	// debrisWidthFactor — протяжённость обломочного пояса WD (оценка).
	debrisWidthFactor = 0.35

	// Типичный размер тела (км): астероиды мелкие, Койпера крупнее (§4.3).
	asteroidBodySizeKm = 1.0
	kuiperBodySizeKm   = 100.0
	debrisBodySizeKm   = 10.0

	// debrisMass — масса обломочного пояса WD — КОНСТАНТА ТИПА (§4.4):
	// экзотика M_диск не потребляет, бюджет облака к ней неприменим.
	// Значение — открытый вопрос (§12 п.3).
	debrisMass = 0.02

	// Ключи состава пояса (JSONB composition): доли породы/железа/льда
	// (та же зона, что у планет — compositionByZone, cascade.go, §3.2).
	beltRockKey = "rock"
	beltIronKey = "iron"
	beltIceKey  = "ice"
)

// BeltData — пояс малых тел перед вставкой в БД (аналог PlanetData).
type BeltData struct {
	ID         string
	WorldID    string
	Kind       string       // asteroid | kuiper | oort | debris | dust_ring (открытый ключ)
	Name       string       // имя пояса (для модалки, этап 2)
	OrbitIndex *int         // якорь-номер накрытой орбиты; nil у Койпера/Оорта
	RadiusAU   float64      // физическая середина окна, а.е.
	WidthAU    float64      // протяжённость, а.е.
	Mass       float64      // суммарная масса, M⊕
	BodySizeKm float64      // типичный размер тела, км
	Composition Composition // {rock, iron, ice} — тот же источник, что у планет
	Visible    bool
	Data       []byte // JSONB расширяемости; этап 1 — "{}"
}

// ==================== ПОЯСА ====================

// asteroidBelt — пояс астероидов: резонансное окно гиганта 3:1–2:1 (§4.1).
// Занимает орбиту g−1 вместо планеты (класс A): orbit_index = g−1 — якорь,
// radius_au = 0.5555·r_g — середина окна. Вызывается только при g ≥ 2.
func (g *Generator) asteroidBelt(w WorldInfo, giantOrbit int) BeltData {
	l := luminosityBySpectral(w.SpectralClass)
	rG := orbitRadiusScaled(giantOrbit, l)
	radius := beltWindowMidPrc * rG
	width := beltWindowWidthPrc * rG
	anchor := giantOrbit - 1
	// w⁰_зон — зональная доля профиля: w_{g−1} = c_{g−1}/S₀ (§4.2).
	wZone := coreMass(orbitRadiusByIndex(anchor), 0) / cloudProfileSum
	return BeltData{
		ID:         uuid.New().String(),
		WorldID:    w.ID,
		Kind:       "asteroid",
		Name:       beltName(w.ID, "asteroid", g.usedNames),
		OrbitIndex: &anchor,
		RadiusAU:   radius,
		WidthAU:    width,
		Mass:       kappaAsteroid * wZone * g.cloudBudget,
		BodySizeKm: asteroidBodySizeKm,
		Composition: beltComposition(radius, l,
			beltRNG(w.ID, "asteroid", "comp")),
		Visible: true,
		Data:    []byte("{}"),
	}
}

// kuiperBelt — пояс Койпера: за внешней границей диска x_out (§4.1(б), §4.3).
// orbit_index = NULL; radius_au = x_out·√L; состав — за снеговой линией.
func (g *Generator) kuiperBelt(w WorldInfo) BeltData {
	l := luminosityBySpectral(w.SpectralClass)
	radius := cloudOuterX * math.Sqrt(l)
	return BeltData{
		ID:          uuid.New().String(),
		WorldID:     w.ID,
		Kind:        "kuiper",
		Name:        beltName(w.ID, "kuiper", g.usedNames),
		OrbitIndex:  nil,
		RadiusAU:    radius,
		WidthAU:     kuiperWidthFactor * radius,
		Mass:        kappaKuiper * kuiperProfileTail * g.cloudBudget,
		BodySizeKm:  kuiperBodySizeKm,
		Composition: beltComposition(radius, l, beltRNG(w.ID, "kuiper", "comp")),
		Visible:     true,
		Data:        []byte("{}"),
	}
}

// debrisBelt — обломочный пояс белого карлика: материализация
// disk_state = 'debris' (§4.4). Радиус — по выжившим орбитам WD
// (orbitRadiusByIndex, БЕЗ масштаба √L); масса — константа типа.
func (g *Generator) debrisBelt(w WorldInfo) BeltData {
	l := luminosityBySpectral(w.SpectralClass)
	orbit := 5 + beltRNG(w.ID, "debris", "orbit").Intn(4) // орбиты 5..8
	radius := orbitRadiusByIndex(orbit)
	return BeltData{
		ID:          uuid.New().String(),
		WorldID:     w.ID,
		Kind:        "debris",
		Name:        beltName(w.ID, "debris", g.usedNames),
		OrbitIndex:  &orbit,
		RadiusAU:    radius,
		WidthAU:     debrisWidthFactor * radius,
		Mass:        debrisMass,
		BodySizeKm:  debrisBodySizeKm,
		Composition: beltComposition(radius, l, beltRNG(w.ID, "debris", "comp")),
		Visible:     true,
		Data:        []byte("{}"),
	}
}

// beltComposition — состав пояса той же функцией зоны, что у планет
// (один источник — compositionByZone, §3.2): {rock, iron, ice} → Composition.
func beltComposition(radiusAU, luminosity float64, rng *rand.Rand) Composition {
	rock, iron, ice := compositionByZone(radiusAU, luminosity, rng)
	return Composition{
		beltRockKey: rock,
		beltIronKey: iron,
		beltIceKey:  ice,
	}
}

// beltRNG — ВЫДЕЛЕННЫЙ генератор пояса (world_id + kind + salt): пояса
// используют его для состава/имени/орбиты debris, чтобы НЕ потреблять g.rng
// (иначе сдвинулся бы поток планет мира, §4.7 «роллов не добавляет»).
// Детерминирован по world_id — один мир даёт одну и ту же композицию.
func beltRNG(worldID, kind, salt string) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(worldID))
	h.Write([]byte{0})
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(salt))
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

// beltName — имя пояса генератором имён проекта (финальные — @writer, §12 п.7).
func beltName(worldID, kind string, usedNames map[string]bool) string {
	if name := names.GeneratePlanetName(beltRNG(worldID, kind, "name"), usedNames); name != "" {
		return name
	}
	return "Пояс-" + uuidShort()
}
