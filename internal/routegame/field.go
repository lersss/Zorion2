// internal/routegame/field.go
// Общие хелперы сегмента перелёта, оставшиеся после снятия старой (v1) модели:
// HashSeed — стабильный seed пары миров (§5.3); passportUnrest — «беспокойство»
// пространства по паспорту (§5.4), его читает поле v9 (grid_model.go). Генерация
// и оценка полилинии v1 удалены — прод-потребителей у них нет.
package routegame

import "hash/fnv"

// Пороги «беспокойного» характера пространства (спека §5.4) — ГИПОТЕЗЫ, калибруются.
const (
	unrestExoticStar     = 1    // star_type ≠ star
	unrestMultipleSystem = 1    // system_type ≠ single
	unrestHighTemp       = 1    // высокая температура
	unrestBelt           = 1    // пояс в системе назначения
	highTempK            = 8000 // порог «высокой температуры», K
)

// HashSeed — стабильный seed пары миров (§5.3): hash(from_world_id,
// to_world_id). Не зависит от времени; направление A→B даёт один «характер».
func HashSeed(fromWorldID, toWorldID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fromWorldID))
	_, _ = h.Write([]byte{0}) // разделитель: ("ab","c") ≠ ("a","bc")
	_, _ = h.Write([]byte(toWorldID))
	return int64(h.Sum64())
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
