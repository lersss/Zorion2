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
// физике планеты, точному населению на момент since и возрасту поселения
// (created_at; 99.2.12): изменение = сумма компонент (рекурсивная жара + λ
// прочих факторов, ChangeComponents) с возрастным потолком — при
// age = now − created_at ≥ MaxLifespanSeconds (120 лет) население = 0
// (компонента-ограничение). Инвариант: два последовательных пересчёта дают
// тот же результат, что один (потолок и экспонента — функции абсолютного
// возраста/времени, а не числа пересчётов). Ниже NDead — население
// обнуляется, а не продолжает таять дробно (механизм 18_needs, к температуре
// не привязан; порог температуры — p < 1 внутри Population).
func Recompute(input PlanetInput, scale Scale, populationExact float64, since time.Time, now time.Time, createdAt time.Time) float64 {
	deltaSeconds := now.Sub(since).Seconds()
	if deltaSeconds <= 0 {
		return populationExact
	}

	if now.Sub(createdAt).Seconds() >= MaxLifespanSeconds {
		return 0
	}

	r, lambdaPerHour := ChangeComponents(input, scale)
	next := Population(populationExact, r, lambdaPerHour, deltaSeconds)
	if next < NDead {
		return 0
	}
	return next
}
