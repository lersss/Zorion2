// internal/models/stellar_mods.go
package models

// StellarMods — модификаторы звезды (колонка worlds.stellar_mods, JSONB).
// Открытый пакет: новый модификатор — добавка в JSON, не миграция (99.2.4 §2).
//
// Группы (ортогональны друг другу):
//  1. Эволюционная фаза Phase: V / III / I (только для star).
//  2. Переменность VariableType: eclipsing / mira / cepheid / uv_ceti /
//     t_tauri / nova / dwarf_nova (+ период/амплитуда).
//  3. Подтипы объекта Subtype: pulsar / magnetar (neutron), accretion
//     (black_hole), lbv / wr (star фазы I, «прочая экзотика»).
//  4. Параметры системы: BinaryType (wide/close), Companion, DiskState
//     (protoplanetary/accretion/debris), Metallicity.
//  5. Параметры компаньона (35b §2.1): CompanionMass/CompanionTemp/
//     CompanionSepAU — уточнение того же компаньона (не отдельная сущность);
//     ExtraCompanions — внешние компаньоны кратных (1 шт., статичные).
// Масса звезды — НЕ здесь: перенесена в колонку worlds.stellar_mass (29a §4м).
type StellarMods struct {
	Phase              string            `json:"phase,omitempty"`                 // V/III/I
	VariableType       string            `json:"variable_type,omitempty"`         // eclipsing/mira/cepheid/uv_ceti/t_tauri/nova/dwarf_nova
	VariablePeriodDays *float64          `json:"variable_period_days,omitempty"`  // дни (цефеиды и пр.)
	VariableAmplitude  *float64          `json:"variable_amplitude,omitempty"`    // звёздные величины (nullable)
	Subtype            string            `json:"subtype,omitempty"`               // pulsar/magnetar/accretion/lbv/wr
	BinaryType         string            `json:"binary_type,omitempty"`           // wide/close
	Companion          string            `json:"companion,omitempty"`             // спектр/тип компаньона (двойные)
	DiskState          string            `json:"disk_state,omitempty"`            // protoplanetary/accretion/debris
	Metallicity        *float64          `json:"metallicity,omitempty"`           // [Fe/H] (nullable)

	// Параметры компаньона (35b §2.1): заполняются у binary/multiple.
	CompanionMass   *float64          `json:"companion_mass,omitempty"`    // масса компаньона, M☉
	CompanionTemp   *int              `json:"companion_temp,omitempty"`    // температура компаньона, K
	CompanionSepAU  *float64          `json:"companion_sep_au,omitempty"`  // разделение пары (большая полуось), а.е.
	ExtraCompanions []ExtraCompanion  `json:"extra_companions,omitempty"`  // внешние компаньоны кратных (multiple)
}

// ExtraCompanion — внешний компаньон кратной системы (35b §2.1, §3.4):
// статичный, без собственных планет; массив для расширяемости (схема
// переживает 2+ внешних).
type ExtraCompanion struct {
	SpectralClass string   `json:"spectral_class"`
	Mass          *float64 `json:"mass,omitempty"` // M☉
	Temp          *int     `json:"temp,omitempty"` // K
	SepAU         float64  `json:"sep_au"`         // а.е., ≥ 3× разделения внутренней пары
}

// IsBinary — двойная или кратная система.
func (m *StellarMods) IsBinary() bool {
	return m != nil && (m.BinaryType == "wide" || m.BinaryType == "close")
}

// IsSupergiantExotic — «прочая экзотика»: сверхгигант горячего класса
// (фаза I + подтип lbv/wr, 99.2.4 §4.4). Для неё своё среднее числа планет.
func (m *StellarMods) IsSupergiantExotic() bool {
	return m != nil && m.Phase == "I" && (m.Subtype == "lbv" || m.Subtype == "wr")
}