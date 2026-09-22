// internal/audit/planet/checks_history.go
//
// Правило консистентности истории формирования и состава (спека
// 2026-09-22-облако-этап-2-состав-и-история-формирования.md §6.3 п.2).
// Объёмные доли w_rock/w_iron/w_ice НЕ персистятся в planet.data — правило
// читает персистируемую density (и производные), а не доли. Аддитивное:
// пропускает планеты без formation_history (старые миры, гиганты, экзотика).
package planet

import (
	"fmt"
	"math"

	"zorion/internal/audit"
)

// Пороги согласованности density с историей — заглушка (калибровка
// @balancetester вместе с полосами состава §4.6; спека §6.3).
const (
	historyIcyDensityMax = 0.95 // тело, рождённое за линией (a_ice ≥ 0.5), должно быть ≤
	historyDryDensityMin = 0.30 // «сухое» тело (formed_late) должно быть ≥
)

// checkFormationHistoryMismatch — история формирования должна быть
// согласована с плотностью (состав — от x_form, а не от текущей орбиты).
func checkFormationHistoryMismatch(v *View) []audit.Issue {
	events := rawFormationEvents(v.Raw)
	if len(events) == 0 {
		return nil
	}

	// x_ice(t_form) — из formed_early; иначе базовое позднее значение 2.7.
	// formed_late — поздняя эпоха (§4.3): резервуара льда нет (a_ice = 0.05),
	// тело сухое в любой зоне (§4.6) — ледяную плотность не требуем.
	// ice_lost — потеря мантии после конденсации (§4.5): состав вторичен,
	// требовать исходную («ледяную») плотность нельзя — ободранное Fe-ядро
	// тяжёлое (§4.5.1, канал «сдуло звездой»).
	xIce := 2.7
	late, iceLost := false, false
	for _, ev := range events {
		switch ev.Type {
		case "formed_early":
			if x, ok := ev.Payload["x_ice"].(float64); ok && x > 0 {
				xIce = x
			}
		case "formed_late":
			late = true
		case "ice_lost":
			iceLost = true
		}
	}

	var issues []audit.Issue
	add := func(code, desc string, details map[string]interface{}) {
		issues = append(issues, newIssueWithDetails(v, code, audit.SeverityMedium, desc, details))
	}

	for _, ev := range events {
		switch ev.Type {
		case "ice_lost":
			frac, _ := ev.Payload["fraction"].(float64)
			residue, _ := ev.Payload["residue"].(string)
			if frac < 0.10 || frac > 0.60 || (residue != "iron" && residue != "bare_ice") {
				add("formation_history_invalid",
					fmt.Sprintf("ice_lost: fraction=%.3f, residue=%q — вне допустимого", frac, residue),
					map[string]interface{}{"fraction": frac, "residue": residue})
			}

		case "formed_late":
			// Поздняя эпоха — сухое тело (резервуара льда нет): плотность
			// не может быть «лёгкой» (обнажённый лёд / лёд).
			if v.Density > 0 && v.Density < historyDryDensityMin {
				add("formation_history_mismatch",
					fmt.Sprintf("formed_late: density=%.3f ниже полосы сухого тела (≥ %.2f)", v.Density, historyDryDensityMin),
					map[string]interface{}{"density": v.Density})
			}

		case "migrated":
			factor, _ := ev.Payload["factor"].(float64)
			dir, _ := ev.Payload["direction"].(string)
			xForm, _ := ev.Payload["x_form"].(float64)
			xNow, _ := ev.Payload["x_now"].(float64)
			valid := factor >= 1.3 && (dir == "inward" || dir == "outward") &&
				((dir == "inward" && xForm >= xNow) || (dir == "outward" && xForm <= xNow))
			if !valid {
				add("formation_history_invalid",
					fmt.Sprintf("migrated: factor=%.3f, direction=%q, x_form=%.3f, x_now=%.3f — несогласованно", factor, dir, xForm, xNow),
					map[string]interface{}{"factor": factor, "direction": dir, "x_form": xForm, "x_now": xNow})
			}
			// Состав от x_form, не от x_now: тело, родившееся за снеговой
			// линией (a_ice(x_form) ≥ 0.5), должно быть лёгким. Поздняя
			// эпоха (a_ice = 0.05) и потеря мантии меняют ожидание — см.
			// флаги выше; ледяную плотность тогда не требуем.
			if !late && !iceLost && aIceAt(xForm, xIce) >= 0.5 && v.Density > historyIcyDensityMax {
				add("formation_history_mismatch",
					fmt.Sprintf("migrated: x_form=%.3f (лёд по a_ice), но density=%.3f — состав не от x_form", xForm, v.Density),
					map[string]interface{}{"x_form": xForm, "density": v.Density})
			}
		}
	}
	return issues
}

// rawHistoryEvent — запись formation_history из сырого JSON.
type rawHistoryEvent struct {
	Type    string
	Payload map[string]interface{}
}

// rawFormationEvents — читает formation_history из v.Raw (nil, если ключа нет).
func rawFormationEvents(raw map[string]interface{}) []rawHistoryEvent {
	arr, ok := raw["formation_history"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]rawHistoryEvent, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		if typ == "" {
			continue
		}
		out = append(out, rawHistoryEvent{Type: typ, Payload: m})
	}
	return out
}

// aIceAt — вероятность конденсации a_ice = σ((x_form − x_ice)/δ), δ = 0.7
// (спека §4.6; локальная копия чистой функции генератора — audit не зависит
// от пакета generator/planet).
func aIceAt(xForm, xIce float64) float64 {
	return 1 / (1 + math.Exp(-(xForm-xIce)/0.7))
}
