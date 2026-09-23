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

// UnitScale — масштаб отображения/ввода единицы темпа «ед/сутки/млрд» (задача
// «переключатель масштаба единицы»). Модель и ХРАНИМАЯ единица не меняются:
// масштаб влияет только на показ и ввод, значение на сервер всегда уходит в
// хранимой единице. Ключи совпадают с JS-дублем (web/static/js/unit_scale.js).
type UnitScale string

const (
	ScalePerBillion  UnitScale = "billion" // ×1 — вид по умолчанию, как до переключателя
	ScalePerMillion  UnitScale = "mega"    // ×1e-3
	ScalePerThousand UnitScale = "kilo"    // ×1e-6
	ScalePerPerson   UnitScale = "person"  // ×1e-9
)

// UnitScaleMultiplier — множитель «хранимое → отображаемое». Неизвестный
// масштаб (в т.ч. пустой) — хранимая единица (×1), без паники.
func UnitScaleMultiplier(s UnitScale) float64 {
	switch s {
	case ScalePerPerson:
		return 1e-9
	case ScalePerThousand:
		return 1e-6
	case ScalePerMillion:
		return 1e-3
	default:
		return 1
	}
}

// StoredToDisplay — «ед/сутки/млрд» → значение в масштабе s (показ).
func StoredToDisplay(storedPerDayPerBillion float64, s UnitScale) float64 {
	return storedPerDayPerBillion * UnitScaleMultiplier(s)
}

// DisplayToStored — значение в масштабе s → «ед/сутки/млрд» (ввод).
func DisplayToStored(display float64, s UnitScale) float64 {
	return display / UnitScaleMultiplier(s)
}
