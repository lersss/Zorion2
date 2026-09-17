// internal/economy/settlement/race_balancer_validate.go
// Валидация записей расовых R-кривых (спека 99.2.23 §3.1): как PUT 99.2.17
// §6, но БЕЗ диапазона X компоненты (у расовых кривых свои диапазоны —
// сдвиг из карточки). Невалидные записи при загрузке пропускаются с логом
// (не роняют старт).
package settlement

import "fmt"

// validateRaceRecord — запись расы: race_id, card_hash, factory/active
// (reproduction > 0, 4 кривые валидны).
func validateRaceRecord(rec *RaceRecord) error {
	if rec == nil {
		return fmt.Errorf("пустая запись")
	}
	if rec.RaceID == "" {
		return fmt.Errorf("race_id пуст")
	}
	if rec.RaceID == "humans" {
		return fmt.Errorf("humans — спец-случай, запись не создаётся")
	}
	if rec.CardHash == "" {
		return fmt.Errorf("card_hash пуст")
	}
	if err := validateRaceCurves(&rec.Factory); err != nil {
		return fmt.Errorf("factory: %w", err)
	}
	if err := validateRaceCurves(&rec.Active); err != nil {
		return fmt.Errorf("active: %w", err)
	}
	return nil
}

// validateRaceCurves — reproduction > 0 + 4 кривые валидны.
func validateRaceCurves(rc *RaceCurves) error {
	if rc.Reproduction <= 0 {
		return fmt.Errorf("reproduction должен быть > 0, получили %v", rc.Reproduction)
	}
	for _, comp := range raceComponents {
		c := rc.Curves[comp]
		if c == nil {
			return fmt.Errorf("кривая %q отсутствует", comp)
		}
		if err := validateRaceCurve(*c); err != nil {
			return fmt.Errorf("кривая %q: %w", comp, err)
		}
	}
	return nil
}

// validateRaceCurve — как PUT 99.2.17 §6, но БЕЗ диапазона X компоненты:
// у расовых кривых свои диапазоны (сдвиг из карточки). Узлы строго
// возрастают, y ∈ [0, 0.999), |bend| ≤ 10, 3–16 узлов.
func validateRaceCurve(curve ComponentCurve) error {
	n := len(curve.Nodes)
	if n < 3 || n > 16 {
		return fmt.Errorf("число узлов должно быть в диапазоне 3..16, получили %d", n)
	}
	if len(curve.Bends) != n-1 {
		return fmt.Errorf("число изгибов должно быть равно числу узлов − 1 (%d), получили %d", n-1, len(curve.Bends))
	}
	for i, node := range curve.Nodes {
		if i > 0 && !(node.X > curve.Nodes[i-1].X) {
			return fmt.Errorf("X узлов должны строго возрастать: узел %d (x=%v) не больше предыдущего (x=%v)", i, node.X, curve.Nodes[i-1].X)
		}
		if node.Y < 0 || node.Y >= 0.999 {
			return fmt.Errorf("Y узла %d должен быть в диапазоне [0, 0.999), получили %v", i, node.Y)
		}
	}
	for i, k := range curve.Bends {
		if k < -10 || k > 10 {
			return fmt.Errorf("изгиб сегмента %d должен быть в диапазоне [-10, +10], получили %v", i, k)
		}
	}
	return nil
}