package settlement

import "time"

// NDead — порог (абсолютное число людей), ниже которого население считается
// вымершим и обнуляется целиком, а не тает дальше дробно. Стыковка с уже
// принятым разовым обвалом населения (docs/gamedesign/18a_population_death.md,
// «Механизм»; 18_needs.md §18.3.1). Черновое значение, не финальное.
var NDead = 100.0

// MinPersistInterval — прошедшее время между чек-точками, после которого
// «простой визит» игрока становится «событием»: только тогда ленивый
// пересчёт продвигает сохранённую чек-точку (population_exact/computed_at) в
// БД. Частые просмотры считают население только в памяти — пересчёт это
// чистая функция от чек-точки, запись на хот-пате чтения не нужна (модель
// «правда на сервере, синк по событию», docs/gamedesign/18a_population_death.md).
// Противоположный предел — запись при каждом просмотре; его цена в том, что
// сохранённое население устаревает для читателей без пересчёта (admin-stats).
var MinPersistInterval = 30 * time.Minute

// Recompute считает новое точное население поселения на момент now по
// физике планеты и точному населению на момент since. Ниже NDead —
// население обнуляется, а не продолжает таять дробно.
func Recompute(input PlanetInput, scale Scale, populationExact float64, since time.Time, now time.Time) float64 {
	deltaHours := now.Sub(since).Hours()
	if deltaHours <= 0 {
		return populationExact
	}

	lambda := TotalLambda(input, scale)
	next := Population(populationExact, lambda, deltaHours)
	if next < NDead {
		return 0
	}
	return next
}
