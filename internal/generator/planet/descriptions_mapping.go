// internal/generator/planet/descriptions_mapping.go
package planet

// ==================== МАППИНГ ТИП → ПАПКА ====================
//
// В classify.go геймдизайнерские типы — это русские строки:
// "ледяная", "радиоактивная", "газовый гигант".
//
// Папки в config/descriptions/ называются по-английски:
// icy, radioactive, gas_giant.
//
// Функция переводит одно в другое. Если тип неизвестен — "".

// typeToFolder — геймдизайнерский тип → имя папки с описаниями.
func typeToFolder(gdType string) string {
	switch gdType {
	case TypeGasGiant:
		return "gas_giant"
	case TypeRadioactive:
		return "radioactive"
	case TypeEarthlike:
		return "earthlike"
	case TypeOceanic:
		return "oceanic"
	case TypeIce:
		return "icy"
	case TypeVolcanic:
		return "volcanic"
	case TypeDesert:
		return "desert"
	case TypeGlass:
		return "glass"
	case TypeMetal:
		return "metal"
	case TypeOrganic:
		return "organic"
	case TypeRocky:
		return "rocky"
	case TypeMiniNeptune:
		// Класс (M_crit, 16] M⊕ (спека 2026-09-23 §11.2). Контент описаний —
		// подэтап 2/3 (@writer): тип зарегистрирован нейтральной заготовкой,
		// чтобы генерация не сыпала логом «неизвестный тип» на каждой планете.
		return "minineptune"
	}
	return ""
}