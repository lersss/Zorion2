// internal/generator/galaxy/race_tuning.go — подкрутка весов спектральных
// классов под расу-дома кластера (99.2.22 §3.3 ручка 5, §3.4).
//
// Ручка 5 применяется там же, где profile: generateWorld (galaxy.go,
// spectralClass). Композиция с профилем: веса × profile_mult × race_mult
// (обе мягкие, все веса > 0 — инвариант «все типы возможны» сохраняется).
// Слой 1 (непрерывный): eff = 1 + (config − 1)·s.
package galaxy

import (
	"zorion/internal/races"
)

// applyRaceSpectralMult — умножает веса спектральных классов на множители
// расы-дома (99.2.22 §3.3 ручка 5): home-классы ×1.6 (звёздный слой синергии
// «звёзды+планеты», решение создателя 2026-09-17), умеренно-горячим
// тепловым O/B ×1.3; остальные классы = 1.0. Слой 1: eff = 1 + (config−1)·s.
// Каталог не загружен / раса не найдена — веса как есть (нейтрально).
func applyRaceSpectralMult(weights map[string]float64, raceID string, softness float64) map[string]float64 {
	r := races.ByID(raceID)
	if r == nil {
		return weights
	}
	t := r.Tuning()
	if t == nil {
		return weights // оф-модельная раса (вне каскада) — подкрутки нет
	}
	mult := t.SpectralMult
	if len(mult) == 0 {
		return weights
	}
	out := make(map[string]float64, len(weights))
	for k, v := range weights {
		m := 1.0
		if mv, ok := mult[k]; ok {
			m = 1 + (mv-1)*softness
		}
		out[k] = v * m
	}
	return out
}
