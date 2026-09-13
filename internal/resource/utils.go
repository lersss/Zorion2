// internal/resource/utils.go
package resource

import "math/rand"

// determineTotal — сколько всего ресурсов генерировать на планете.
//
// Учитывает:
//   - доминирующую форму поверхности (богатство);
//   - количество типов недр в композиции (чем больше типов, тем больше ресурсов);
//   - спектральный класс звезды (горячие звёзды → больше редких).
func determineTotal(
	surfaceDominant string,
	subterrain map[string]float64,
	spectralClass string,
	rng *rand.Rand,
) int {
	base := baseCountForSurface(surfaceDominant)

	// Бонус за разнообразие недр: каждый тип с долей > 5% даёт +1 ресурс.
	subterrainBonus := 0
	for _, share := range subterrain {
		if share >= minSubterrainShare {
			subterrainBonus++
		}
	}
	// Ограничим бонус, чтобы не раздувать планеты
	if subterrainBonus > 5 {
		subterrainBonus = 5
	}

	// Бонус за спектральный класс
	spectralBonus := 0
	switch spectralClass {
	case "O", "B", "A":
		spectralBonus = 2
	case "F", "G":
		spectralBonus = 1
	}

	// Случайный разброс ±2
	jitter := rng.Intn(5) - 2

	total := base + subterrainBonus + spectralBonus + jitter
	if total < 1 {
		total = 1
	}
	if total > 20 {
		total = 20
	}
	return total
}

// baseCountForSurface — базовое количество ресурсов по доминирующей форме.
func baseCountForSurface(surfaceDominant string) int {
	switch surfaceDominant {
	case "океаны", "озёра_реки":
		return 6
	case "леса", "джунгли", "болота", "коралловые_рифы":
		return 6
	case "горы", "пески_пустыни", "кратеры":
		return 5
	case "лавовые_поля", "вулканические_поля":
		return 6
	case "ледники", "мёрзлые_газы":
		return 4
	case "стеклянные_поля", "металлические_поля":
		return 5
	default:
		return 5
	}
}