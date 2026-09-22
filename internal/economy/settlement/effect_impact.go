// Реестр воздействий эффектов (спека 2026-09-22-эффекты-снабжения-задержка-голод
// §5.4) — ОТКРЫТЫЙ набор: сейчас единственный impact 'population_rate'
// (добавляет R(load) к R_total). Резолв вклада эффекта — здесь; источник силы w
// (дефицит потребности) поставляет слой потребности (этап 2). Неизвестный
// impact или неизвестная кривая — no-op + лог (не тихий ноль, §5.4/§7.4).
package settlement

import "log"

// ImpactPopulationRate — единственный impact в объёме (голод): добавляет
// R(load) к R_total после диспетчеризации (§5.4).
const ImpactPopulationRate = "population_rate"

// EffectCurveEval — кривая силы эффекта R(load): доля/сек по нагрузке load.
type EffectCurveEval func(load float64) float64

// CurveLookup — резолв кривой компоненты «Балансировки» по ссылке
// effect_types.params.curve (§7.1): ok=false — компонента/кривая неизвестна.
type CurveLookup func(curveKey string) (EffectCurveEval, bool)

// EffectRate — вклад эффекта в R_total (доля/сек) по нагрузке load.
// Реестр impact — открытый набор (§5.4): неизвестный impact → 0 + лог.
// Кривая резолвится через curveLookup (ссылка params.curve): nil/не найдена →
// 0 + лог curve_unknown (§7.4).
func EffectRate(impact, curveKey string, load float64, curveLookup CurveLookup) float64 {
	switch impact {
	case ImpactPopulationRate:
	default:
		log.Printf("⚠️ effect: неизвестный impact %q — вклад 0 (impact_unknown)", impact)
		return 0
	}
	if curveLookup == nil {
		log.Printf("⚠️ effect: кривая %q недоступна — вклад 0 (curve_unknown)", curveKey)
		return 0
	}
	eval, ok := curveLookup(curveKey)
	if !ok || eval == nil {
		log.Printf("⚠️ effect: неизвестная кривая %q — вклад 0 (curve_unknown)", curveKey)
		return 0
	}
	return eval(load)
}
