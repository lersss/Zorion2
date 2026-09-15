// internal/audit/planet/checks_exotic.go
package planet

import (
	"zorion/internal/audit"
)

// ==================== ЭКЗОТИЧЕСКИЕ СИСТЕМЫ (99.2.4 §6.2) ====================
//
// Планеты экзотических объектов (ЧД/НЗ/WD/протозвезда) генерируются веткой
// §5.3: вода 0, жизнь 0, T 25–150 K, settleable=false. Правила — машинная
// гарантия инварианта: если в данных планеты проявилась жизнь или пресет
// пригодности при exotic_system=true — ветка была нарушена.
//
// Правила читают Raw["exotic_system"] / Raw["star_type"] — эти поля кладутся
// в JSON планеты веткой генерации (99.2.4 §5.3). На обычных звёздах (O–Y)
// флаг отсутствует — ложных срабатываний нет.

// checkLifeInExoticSystem — жизнь в экзотической системе: невозможно по ветке.
func checkLifeInExoticSystem(v *View) []audit.Issue {
	if !v.Life || !aBool(v.Raw, "exotic_system") {
		return nil
	}
	return []audit.Issue{newIssue(v, "life_in_exotic_system", audit.SeverityHigh,
		"Жизнь в экзотической системе — невозможно по ветке генерации (99.2.4 §5.3)")}
}

// checkSettleableUnderExotic — пресет пригодности поселений (T 200–350 K,
// вода ≥ 10) в экзотической системе: невозможно по ветке §5.3.
func checkSettleableUnderExotic(v *View) []audit.Issue {
	if !aBool(v.Raw, "exotic_system") {
		return nil
	}
	if v.Temperature >= 200 && v.Temperature <= 350 && v.WaterPercent >= 10 {
		return []audit.Issue{newIssue(v, "settleable_under_exotic", audit.SeverityHigh,
			"Планета экзотической системы с пресетом пригодности — невозможно по ветке генерации (99.2.4 §5.3)")}
	}
	return nil
}