package settlement

import "time"

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
	// Effects — кусочно-постоянная сила эффектов населения за интервал
	// [AsOf, now) (спека 2026-09-22-эффекты-снабжения-задержка-голод §5.1):
	// вклад эффекта передаётся ТОЛЬКО здесь (§5.3 — двойной учёт).
	Effects []EffectForcePoint
	// AsOf — точка, на которую точечные потребители (ChangeComponents,
	// DeathCause, DeathTime, Projection) берут силу эффекта: сегмент,
	// содержащий AsOf. У потребителей без прохода веток — load_at
	// (хранимый базис, §5.2/§5.5); у owner-прохода — computed_at.
	AsOf time.Time
}

// EffectForcePoint — кусочно-постоянная сила эффекта R(load(t)) на
// [Since, Until) (спека 2026-09-22-эффекты-снабжения-задержка-голод §5.1):
// владелец (слой потребности) строит траекторию по сегментам нагрузки.
type EffectForcePoint struct {
	Rate  float64
	Since time.Time
	Until time.Time
}

// RateAt — сила сегмента, содержащего t. Сегменты полуоткрытые [Since, Until):
// на общем разрыве двух соседних сегментов значение берётся ровно у одного
// (нет двойного учёта при сумме ChangeComponents). У одноточечного сегмента
// (Since == Until) сила применяется ровно в этой точке — так точечные
// потребители получают текущую силу (§5.2/§5.5).
func (p EffectForcePoint) RateAt(t time.Time) float64 {
	if p.Since.Equal(p.Until) {
		if t.Equal(p.Since) {
			return p.Rate
		}
		return 0
	}
	if t.Before(p.Since) || !t.Before(p.Until) {
		return 0
	}
	return p.Rate
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
