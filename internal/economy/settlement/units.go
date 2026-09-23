// internal/economy/settlement/units.go
//
// Единая точка конверсии единицы модели «ед/сутки/млрд» → «батч/сек» (спека
// 2026-09-23-стадии-поселения-и-скорость-производства §2.2). Единица одна для
// обеих ручек: числа скорости пары (producer_recipes.rate) и нормы потребления
// (params.eat[позиция]). Вызывают ровно две точки: производство ветки
// (ProcessBranch) и спрос слоя потребности (needs.go). Никакая другая точка не
// пересчитывает единицы (правило теста §15.2 T-Е1).
package settlement

// secondsPerDay — сутки в секундах (единица времени модели, §2.2). Вместе с 1e9
// живёт только здесь: конверсия единицы темпа одна на пакет (§15.2 T-Е1).
const secondsPerDay = 86400

// PerSecond — «ед/сутки/млрд» → «ед/сек»: units × population / 1e9 / 86400.
func PerSecond(unitsPerDayPerBillion, population float64) float64 {
	return unitsPerDayPerBillion * population / 1e9 / 86400
}

// PerDay — «ед/сутки/млрд» → «ед/сутки»: units × population / 1e9. Обратный
// разворот PerSecond для витрины арифметики (спека 2026-09-23 §8.1/§8.2):
// расчётные объёмы показываются в сутках, как и заданное число. Единица
// конверсии одна (этот файл, §2.2).
func PerDay(unitsPerDayPerBillion, population float64) float64 {
	return PerSecond(unitsPerDayPerBillion, population) * secondsPerDay
}
