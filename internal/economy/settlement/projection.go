package settlement

// PlanetInput — физические значения планеты, нужные для смерти населения от
// среды (docs/gamedesign/18a_population_death.md).
type PlanetInput struct {
	TemperatureK      float64
	GravityG          float64
	CoreRadioactivity float64
}

// TotalLambda считает суммарную скорость убыли населения по всем трём
// факторам среды для профиля человека. Факторы складываются, а не берётся
// худший — несколько угроз убивают быстрее одной (18a, «Механизм»).
func TotalLambda(input PlanetInput, scale Scale) float64 {
	temperature := Lambda(TwoSidedSeverity(HumanTemperatureProfile, input.TemperatureK), scale)
	gravity := Lambda(TwoSidedSeverity(HumanGravityProfile, input.GravityG), scale)
	radioactivity := Lambda(OneSidedSeverity(HumanRadioactivityProfile, input.CoreRadioactivity), scale)
	return temperature + gravity + radioactivity
}

// Checkpoint — точка времени для предпросмотра кривой убыли.
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
// суммарной скорости убыли lambda — предпросмотр «что будет», без ожидания
// реального времени и без изменения состояния поселения.
func Projection(p0 float64, lambda float64) map[string]float64 {
	result := make(map[string]float64, len(StandardCheckpoints))
	for _, cp := range StandardCheckpoints {
		result[cp.Label] = Population(p0, lambda, cp.Hours)
	}
	return result
}
