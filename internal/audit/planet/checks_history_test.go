// internal/audit/planet/checks_history_test.go
//
// H19 — правило консистентности истории формирования и состава (спека
// 2026-09-22-облако-этап-2 §6.3 п.2): несогласованный сэмпл (migrated без
// состава от x_form) ловится; согласованный — чисто. Легальные сочетания
// migrated+ice_lost (§4.5.1) и formed_late+migrated (§4.6) не флагуются.
package planet

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"zorion/internal/audit"
)

func TestAuditConsistencyRule(t *testing.T) {
	// Несогласованный: migrated inward, тело родилось за линией (x_form=4.0,
	// a_ice ≈ 0.87), но плотность «сухого/Fe» тела (1.3) — состав не от x_form.
	d := baseData()
	d["density"] = 1.3
	d["formation_history"] = []interface{}{
		map[string]interface{}{
			"type": "migrated", "x_form": 4.0, "x_now": 1.5,
			"direction": "inward", "factor": 2.67,
		},
	}
	assertSingle(t, runCheck(checkFormationHistoryMismatch, d),
		"formation_history_mismatch", audit.SeverityMedium)

	// Согласованный: лёгкое тело (0.35) — состав от x_form → чисто.
	d["density"] = 0.35
	assert.Empty(t, runCheck(checkFormationHistoryMismatch, d))

	// Валидность payload: factor < 1.3 → formation_history_invalid.
	d["formation_history"] = []interface{}{
		map[string]interface{}{
			"type": "migrated", "x_form": 4.0, "x_now": 1.5,
			"direction": "inward", "factor": 1.1,
		},
	}
	assertSingle(t, runCheck(checkFormationHistoryMismatch, d),
		"formation_history_invalid", audit.SeverityMedium)

	// Легально: migrated + ice_lost «сдуло звездой» (§4.5.1) — ободранное
	// Fe-ядро тяжёлое (ρ≈1.40); требовать ледяную плотность нельзя.
	d["density"] = 1.40
	d["formation_history"] = []interface{}{
		map[string]interface{}{
			"type": "migrated", "x_form": 3.41, "x_now": 1.16,
			"direction": "inward", "factor": 2.94,
		},
		map[string]interface{}{
			"type": "ice_lost", "fraction": 0.34, "residue": "iron",
		},
	}
	assert.Empty(t, runCheck(checkFormationHistoryMismatch, d))

	// Легально: formed_late — поздняя эпоха (a_ice = 0.05, §4.6), тело сухое
	// в любой зоне; тяжёлая плотность законна, миграция льда не добавляет.
	d["density"] = 1.12
	d["formation_history"] = []interface{}{
		map[string]interface{}{
			"type": "formed_late", "t_form_myr": 40.0,
		},
		map[string]interface{}{
			"type": "migrated", "x_form": 3.41, "x_now": 1.16,
			"direction": "inward", "factor": 2.94,
		},
	}
	assert.Empty(t, runCheck(checkFormationHistoryMismatch, d))

	// formed_late, но плотность «лёгкая» (обнажённый лёд) — истинное
	// расхождение: сухое тело не может быть таким лёгким.
	d["density"] = 0.20
	assertSingle(t, runCheck(checkFormationHistoryMismatch, d),
		"formation_history_mismatch", audit.SeverityMedium)

	// Без formation_history → пропуск (старые миры/гиганты/экзотика).
	delete(d, "formation_history")
	assert.Empty(t, runCheck(checkFormationHistoryMismatch, d))
}
