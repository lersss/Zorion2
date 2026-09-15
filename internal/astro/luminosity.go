// Package astro — общие звёздные константы и формулы.
//
// Появился в 35b («Двойные системы»): сортировка пары «главная = ярче»
// (galaxy.go) сравнивает светимости, но planet не может импортировать galaxy
// (цикл через planet/twin.go → galaxy), а galaxy — planet. Единый источник
// таблицы светимостей живёт здесь; planet/physics.go переиспользует его
// через обёртку luminosityBySpectral (fallback 1.0 закреплён тестами).
package astro

// LuminosityBySpectral — светимость звезды по спектральному классу (L☉).
// Таблица — канон 35b §3 и 99.2.4 §2 (источник): O 1000, B 100, A 10, F 2,
// G 1, K 0.1, M 0.01, L 0.001, T 0.0001, Y 0.00001.
// Неизвестный/пустой класс — fallback 1.0 («как Солнце», историческое
// поведение luminosityBySpectral из planet/physics.go, закреплено тестами).
func LuminosityBySpectral(spectralClass string) float64 {
	table := map[string]float64{
		"O": 1000, "B": 100, "A": 10, "F": 2, "G": 1,
		"K": 0.1, "M": 0.01, "L": 0.001, "T": 0.0001, "Y": 0.00001,
	}
	if l, ok := table[spectralClass]; ok && l > 0 {
		return l
	}
	return 1.0
}