package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== 35b §2.1: параметры компаньона ====================

// TestStellarModsCompanionFieldsRoundTrip — сериализация новых полей
// компаньона (companion_mass/temp/sep_au, extra_companions[]) переживает
// JSON round-trip.
func TestStellarModsCompanionFieldsRoundTrip(t *testing.T) {
	mass := 0.85
	temp := 5400
	sep := 120.0
	emass := 0.3
	etemp := 3000

	m := StellarMods{
		BinaryType:     "wide",
		Companion:      "K",
		CompanionMass:  &mass,
		CompanionTemp:  &temp,
		CompanionSepAU: &sep,
		ExtraCompanions: []ExtraCompanion{{
			SpectralClass: "M",
			Mass:          &emass,
			Temp:          &etemp,
			SepAU:         5000,
		}},
	}

	b, err := json.Marshal(m)
	require.NoError(t, err)

	var out StellarMods
	require.NoError(t, json.Unmarshal(b, &out))

	assert.Equal(t, "wide", out.BinaryType)
	assert.Equal(t, "K", out.Companion)
	require.NotNil(t, out.CompanionMass)
	assert.InDelta(t, 0.85, *out.CompanionMass, 1e-9)
	require.NotNil(t, out.CompanionTemp)
	assert.Equal(t, 5400, *out.CompanionTemp)
	require.NotNil(t, out.CompanionSepAU)
	assert.InDelta(t, 120.0, *out.CompanionSepAU, 1e-9)

	require.Len(t, out.ExtraCompanions, 1)
	ec := out.ExtraCompanions[0]
	assert.Equal(t, "M", ec.SpectralClass)
	require.NotNil(t, ec.Mass)
	assert.InDelta(t, 0.3, *ec.Mass, 1e-9)
	require.NotNil(t, ec.Temp)
	assert.Equal(t, 3000, *ec.Temp)
	assert.InDelta(t, 5000, ec.SepAU, 1e-9)
}

// TestStellarModsOldJSONNoCompanionFields — старый мир (35a): только
// binary_type + companion, новых полей нет — разбор не падает, новые поля nil
// (фолбэки §2.4).
func TestStellarModsOldJSONNoCompanionFields(t *testing.T) {
	src := `{"binary_type":"close","companion":"M"}`

	var m StellarMods
	require.NoError(t, json.Unmarshal([]byte(src), &m))

	assert.Equal(t, "close", m.BinaryType)
	assert.Equal(t, "M", m.Companion)
	assert.Nil(t, m.CompanionMass, "старый мир без companion_mass → nil")
	assert.Nil(t, m.CompanionTemp, "старый мир без companion_temp → nil")
	assert.Nil(t, m.CompanionSepAU, "старый мир без companion_sep_au → nil")
	assert.Empty(t, m.ExtraCompanions, "старый мир без extra_companions → пусто")
}

// TestStellarModsMarshalOmitsEmptyCompanionFields — пустые новые поля не
// пишутся в JSON (открытый пакет, старый формат не разрастается).
func TestStellarModsMarshalOmitsEmptyCompanionFields(t *testing.T) {
	m := StellarMods{BinaryType: "wide", Companion: "M"}
	b, err := json.Marshal(m)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &raw))
	assert.NotContains(t, raw, "companion_mass")
	assert.NotContains(t, raw, "companion_temp")
	assert.NotContains(t, raw, "companion_sep_au")
	assert.NotContains(t, raw, "extra_companions")
}