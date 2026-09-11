// internal/names/region.go
package names

import (
	"math/rand"
	"strings"
)

// GenerateRegionName генерирует красивое название региона галактики.
//
// Имя = корень + окончание. Корни заканчиваются на согласную, окончания
// начинаются с гласной — правило созвучия исключает неблагозвучные стечения
// (нет "врвр", "нн" на стыке и т.п.). Оба списка курируются вручную,
// поэтому комбинации звучат по-человечески.
//
// Пространство: ~80 корней × 7 окончаний = 560 базовых имён, ещё больше —
// с префиксами-типами («Сектор …», «Туманность …»). Этого хватает на сотни
// регионов в одной галактике. Уникальность — через usedNames.
func GenerateRegionName(rng *rand.Rand, usedNames map[string]bool) string {
	for attempt := 0; attempt < 100; attempt++ {
		name := regionRoots[rng.Intn(len(regionRoots))] +
			regionEndings[rng.Intn(len(regionEndings))]

		// ~30% регионов получают префикс-тип.
		if rng.Float64() < 0.3 {
			name = regionPrefixes[rng.Intn(len(regionPrefixes))] + " " + name
		}

		if usedNames[name] {
			continue
		}
		usedNames[name] = true
		return name
	}
	return "Сектор-" + randomSuffix(rng)
}

// regionPrefixes — типы регионов.
var regionPrefixes = []string{
	"Сектор", "Туманность", "Пояс", "Рубеж", "Область",
}

// regionEndings — окончания, начинающиеся с гласной.
// Соединяются только с корнями, заканчивающимися на согласную.
var regionEndings = []string{
	"ия", "ея", "ис", "ин", "ан", "ион", "он",
}

// regionRoots — курируемые корни, заканчивающиеся на согласную.
// Каждый корень проверен вручную в связке с каждым окончанием.
var regionRoots = []string{
	"Аст", "Вер", "Кал", "Мер", "Неб", "Драэл", "Лиан", "Аурел", "Сильв", "Сол",
	"Зефир", "Тал", "Кир", "Сар", "Вир", "Мел", "Дан", "Гор", "Вел", "Тар",
	"Эрид", "Плеяд", "Аквил", "Лор", "Аэр", "Кас", "Дельт", "Сим", "Эль", "Ксир",
	"Гард", "Корв", "Бор", "Валь", "Вест", "Вин", "Вор", "Галь", "Ган", "Гис",
	"Гран", "Грель", "Жар", "Зан", "Зар", "Зор", "Исар", "Канар", "Кельм", "Лавр",
	"Леон", "Мар", "Мир", "Нар", "Орд", "Рен", "Рин", "Сандр", "Свен", "Сейр",
	"Тор", "Фар", "Фен", "Флор", "Шан", "Элан", "Крин", "Луан",
	"Мин", "Нир", "Пел", "Ран", "Син", "Тейр", "Фел", "Хель", "Валд",
}

// IsEuphoniousRegionName — эвристика благозвучия для теста:
// имя не пустое, 5–20 символов, без некрасивых стечений согласных.
func IsEuphoniousRegionName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	// "Сектор X" и т.п. — проверяем только второе слово.
	words := strings.Fields(name)
	base := words[len(words)-1]
	if len([]rune(base)) < 5 || len([]rune(base)) > 20 {
		return false
	}
	return !containsAny(base, []string{"рр", "нн", "лл", "мм", "зз", "сс", "ии", "вв"})
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}