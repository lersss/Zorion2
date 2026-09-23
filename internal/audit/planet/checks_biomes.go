// internal/audit/planet/checks_biomes.go
//
// Правила биомов (99.2.28 §14): биосфера без условий, пустые биомы/недры,
// биом вне справочника. Аддитивные: пропускают старые миры без ключей.
package planet

import (
	"fmt"

	"zorion/internal/audit"
	genplanet "zorion/internal/generator/planet"
)

// checkBiosphereWithoutConditions — земная биосферная форма (тег
// «биосферный» + признак no_toxic) при отсутствии life/жидкой воды/
// нетоксичной атмосферы → High (99.2.28 §14, решение п.19, закрывает
// @critic №2). Экстремофилы (кислотные дебри/хемосады/радиационные ковры) —
// жизнь без нетоксичной — не проверяются.
func checkBiosphereWithoutConditions(v *View) []audit.Issue {
	// Старый мир без ключа biomes — поверхность из старых данных (слой 8),
	// правило биомов к ней не применяется (99.2.28 §14; замечание ревью:
	// иначе правило шумит на старых мирах с лесами без life).
	if _, ok := v.Raw["biomes"]; !ok {
		return nil
	}
	cat := genplanet.GetBiomeCatalog()
	var issues []audit.Issue
	for form, share := range v.Surface {
		if share < 1 {
			continue
		}
		b := cat.BiomeByID(form)
		if b == nil || !hasTag(b.TypeTags, "биосферный") || !hasFeature(b.AtmosphereOK, "no_toxic") {
			continue
		}
		flag, _ := v.Raw["liquid_water_possible"].(bool)
		toxic := atmosphereToxic(v)
		if v.Life && flag && !toxic {
			continue
		}
		issues = append(issues, newIssueWithDetails(v, "biosphere_without_conditions", audit.SeverityHigh,
			fmt.Sprintf("Земная биосферная форма %s %.1f%% при life=%v, флаге воды=%v, токсичной атмосфере=%v",
				form, share, v.Life, flag, toxic),
			map[string]interface{}{"form": form, "share": share, "life": v.Life, "flag": flag, "toxic": toxic}))
	}
	return issues
}

// checkEmptyBiomes — ключ biomes присутствует и пуст у не-гиганта и
// не-мини-нептуна → High (99.2.28 §14): новый мир без биомов — ошибка.
// Отсутствие ключа = старый мир — НЕ проверяется (находка @critic «не
// различает старые миры»); то же для subterrain. Мини-нептуны исключены
// (спека 2026-09-23 §11.2/§14.3): пустые биомы/недры — норма класса
// (поверхность под оболочкой в модели не описывается, как у гиганта);
// без исключения правило флагует High на каждом мини-нептуне.
func checkEmptyBiomes(v *View) []audit.Issue {
	var issues []audit.Issue
	if !v.IsGasGiant && !v.IsMiniNeptune {
		if raw, ok := v.Raw["biomes"].([]interface{}); ok && len(raw) == 0 {
			issues = append(issues, newIssue(v, "empty_biomes", audit.SeverityHigh,
				"Ключ biomes присутствует и пуст у не-гиганта (новый мир без биомов)"))
		}
		if raw, ok := v.Raw["subterrain"].([]interface{}); ok && len(raw) == 0 {
			issues = append(issues, newIssue(v, "empty_biomes", audit.SeverityHigh,
				"Ключ subterrain присутствует и пуст у не-гиганта (новый мир без зон недр)"))
		}
	}
	return issues
}

// checkBiomeNotInCatalog — биом в surface_composition, которого нет в
// справочнике → Medium (99.2.28 §14): старый мир с удалённым/переименованным
// биомом читается как есть; валидация «форма из справочника» — только для
// новых данных.
func checkBiomeNotInCatalog(v *View) []audit.Issue {
	cat := genplanet.GetBiomeCatalog()
	var issues []audit.Issue
	for form, share := range v.Surface {
		if share < 1 {
			continue
		}
		if cat.BiomeByID(form) == nil {
			issues = append(issues, newIssueWithDetails(v, "biome_not_in_catalog", audit.SeverityMedium,
				fmt.Sprintf("Биом %q (%.1f%%) отсутствует в справочнике", form, share),
				map[string]interface{}{"form": form, "share": share}))
		}
	}
	return issues
}

// ==================== ХЕЛПЕРЫ ====================

// atmosphereToxic — токсична ли атмосфера по составу. Граница аудита — строгая
// (доля > порога), исходная семантика не меняется: новая метка спеки высадки
// (tox_ratio ≥ 1, 2026-09-21 §8.2) на результаты аудита старых миров не влияет.
func atmosphereToxic(v *View) bool {
	raw, ok := v.Raw["atmosphere_data"].(map[string]interface{})
	if !ok {
		return false
	}
	compRaw, ok := raw["composition"].(map[string]interface{})
	if !ok {
		return false
	}
	comp := make(map[string]float64, len(compRaw))
	for gas, val := range compRaw {
		if f, ok := val.(float64); ok {
			comp[gas] = f
		}
	}
	return genplanet.ToxRatio(comp, genplanet.GetBiomeCatalog()) > 1
}

// hasTag — есть ли тег в списке тегов биома.
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// hasFeature — есть ли признак в списке атмосферных признаков биома.
func hasFeature(features []string, f string) bool {
	for _, x := range features {
		if x == f {
			return true
		}
	}
	return false
}