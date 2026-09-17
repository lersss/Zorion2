package settlement

// PlanetInput — физические значения планеты, нужные для изменения населения
// от среды (docs/gamedesign/18a_population_death.md).
type PlanetInput struct {
	TemperatureK      float64
	GravityG          float64
	CoreRadioactivity float64
	// RaceID — раса поселения (спека 99.2.23 §2.2): пусто/"humans" —
	// человеческая модель (глобальный store 99.2.17); иначе — active-кривые
	// расы из расового store. Проставляется из поселения в точке пересчёта.
	RaceID string
}

// Checkpoint — точка времени для предпросмотра кривой изменения населения.
type Checkpoint struct {
	Label string
	Hours float64
}

// StandardCheckpoints — стандартные точки предпросмотра. Кривая содержательна
// в диапазоне часы-недели-месяцы (18a, «Профиль устойчивости»): более мелкий
// шаг никто не увидит, поселение не наблюдают непрерывно.
var StandardCheckpoints = []Checkpoint{
	{Label: "1 час", Hours: 1},
	{Label: "1 сутки", Hours: 24},
	{Label: "1 неделя", Hours: 24 * 7},
	{Label: "1 месяц", Hours: 24 * 30},
	{Label: "1 год", Hours: 24 * 365},
}

// Projection считает население на стандартных точках времени при постоянной
// рекурсивной компоненте r (ChangeComponents) — предпросмотр «что будет», без
// ожидания реального времени и без изменения состояния поселения.
func Projection(p0 float64, r float64) map[string]float64 {
	result := make(map[string]float64, len(StandardCheckpoints))
	for _, cp := range StandardCheckpoints {
		result[cp.Label] = Population(p0, r, cp.Hours*3600)
	}
	return result
}
