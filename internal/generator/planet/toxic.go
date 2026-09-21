// internal/generator/planet/toxic.go
//
// Токсичность атмосферы (спека 2026-09-21 §7.1/§8.2): непрерывная мера
// tox_ratio = max по газам (share / threshold) из params.toxic_thresholds
// справочника биомов и метка токсичности (tox_ratio ≥ 1). Логика вынесена из
// audit/planet/checks_biomes.go в общий хелпер пакета planet.
package planet

// ToxRatio — непрерывная мера токсичности атмосферы (спека 2026-09-21 §8.2):
// максимум отношения доли газа к порогу справочника. 0 — токсичных газов нет;
// газы без порога не учитываются. nil-каталог → 0 (не падать).
func ToxRatio(comp map[string]float64, cat *BiomeCatalog) float64 {
	if cat == nil {
		return 0
	}
	max := 0.0
	for gas, th := range cat.Params.ToxicThresholds {
		if th <= 0 {
			continue
		}
		if r := comp[gas] / th; r > max {
			max = r
		}
	}
	return max
}

// IsToxicAtmosphere — метка токсичности (спека 2026-09-21 §8.2): tox_ratio ≥ 1.
func IsToxicAtmosphere(comp map[string]float64, cat *BiomeCatalog) bool {
	return ToxRatio(comp, cat) >= 1
}
