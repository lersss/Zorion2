package planet

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ==================== 35b §4.1: P-ветка generateCircumbinaryPlanet ====================

// closeWorld — тестовая тесная двойная (binary_type=close) с заданным
// разделением; после сортировки «главная = ярче» главная — G (L=1).
func closeWorld(sepAU float64) WorldInfo {
	return WorldInfo{
		ID:            "w1",
		Name:          "World",
		SpectralClass: "G",
		Temperature:   5700,
		StarType:      "star",
		SystemType:    "binary",
		Mods: &models.StellarMods{
			BinaryType:     "close",
			Companion:      "G",
			CompanionMass:  f64(1.0),
			CompanionTemp:  i(5700),
			CompanionSepAU: f64(sepAU),
		},
	}
}

func f64(v float64) *float64 { return &v }
func i(v int) *int           { return &v }

func planetJSON(t *testing.T, p *PlanetData) map[string]interface{} {
	t.Helper()
	require.NotNil(t, p)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(p.Data, &data))
	return data
}

// rockyCircumbinary — каменистая P-планета (гиганты P-типа разрешены и
// имеют свою температуру/воду — тесты формулы/полос фильтруют их).
func rockyCircumbinary(g *Generator, w WorldInfo) map[string]interface{} {
	for i := 0; i < 200; i++ {
		p := g.generateCircumbinaryPlanet(w)
		var data map[string]interface{}
		if err := json.Unmarshal(p.Data, &data); err != nil {
			panic(err)
		}
		if data["is_gas_giant"] != true {
			return data
		}
	}
	panic("rockyCircumbinary: 200 попыток — все гиганты")
}

// TestCircumbinaryPlanetFlags — close-система с планетой: orbit_center=
// barycenter, orbit_radius_au=3×sep, circumbinary=true.
func TestCircumbinaryPlanetFlags(t *testing.T) {
	g := NewGenerator(nil, 42)
	for _, a := range []float64{0.05, 0.39, 0.66, 0.9} {
		p := g.generateCircumbinaryPlanet(closeWorld(a))
		data := planetJSON(t, p)
		assert.Equal(t, "barycenter", data["orbit_center"], "a=%.2f", a)
		assert.InDelta(t, 3*a, data["orbit_radius_au"].(float64), 1e-9, "a=%.2f: r_P = 3a", a)
		assert.Equal(t, true, data["circumbinary"], "a=%.2f", a)
		assert.Equal(t, 1, p.OrbitIndex, "единственная P-орбита")
	}
}

// TestCircumbinaryTempCascade — температура P-планеты от каскада (99.2.20
// §6.6): справочные температуры 99.2.18 §5.1 исторические, новые значения
// даёт каскад. G+G в пригодной полосе (a ∈ [0.39, 0.66]) — умеренно-тёплые
// миры (T_final в широкой полосе от тонкой до плотной CO₂-атмосферы),
// без архетип-клампов; абсолютные границы [20, 2500] держатся всегда.
func TestCircumbinaryTempCascade(t *testing.T) {
	g := NewGenerator(nil, 7)
	for _, a := range []float64{0.39, 0.5, 0.65} {
		data := rockyCircumbinary(g, closeWorld(a))
		got := data["temperature"].(float64)
		assert.GreaterOrEqual(t, got, 200.0, "a=%.2f: пригодная полоса ≥ 200 K", a)
		assert.LessOrEqual(t, got, 500.0, "a=%.2f: пригодная полоса ≤ 500 K", a)
		assert.GreaterOrEqual(t, got, 20.0, "a=%.2f: в [20, 2500]", a)
		assert.LessOrEqual(t, got, 2500.0, "a=%.2f: в [20, 2500]", a)
	}
}

// TestCircumbinaryHotPairClampedTo2500 — worst O+O при a=0.05: честный расчёт
// каскада даёт T ≫ 2500 → кламп к аудит-гейту 2500 (§4.1, §5.1).
func TestCircumbinaryHotPairClampedTo2500(t *testing.T) {
	w := WorldInfo{
		ID:            "w1",
		Name:          "World",
		SpectralClass: "O", // L = 5·10⁴
		Temperature:   35000,
		StarType:      "star",
		SystemType:    "binary",
		Mods: &models.StellarMods{
			BinaryType:     "close",
			Companion:      "O",
			CompanionMass:  f64(30),
			CompanionTemp:  i(35000),
			CompanionSepAU: f64(0.05),
		},
	}
	g := NewGenerator(nil, 3)
	for i := 0; i < 50; i++ {
		p := g.generateCircumbinaryPlanet(w)
		data := planetJSON(t, p)
		assert.Equal(t, 2500.0, data["temperature"].(float64), "O+O a=0.05 → кламп 2500")
	}
}

// TestCircumbinaryNoArchetypeClamp — F+G при a=0.05: каскад даёт T ≫ 900
// (горячий CO₂-мир, кламп к абсолютному максимуму 2500) — архетип-клампов
// нет (99.2.11 отменены, 99.2.20 §3.5).
func TestCircumbinaryNoArchetypeClamp(t *testing.T) {
	w := WorldInfo{
		ID:            "w1",
		Name:          "World",
		SpectralClass: "F", // L = 3
		Temperature:   6500,
		StarType:      "star",
		SystemType:    "binary",
		Mods: &models.StellarMods{
			BinaryType:     "close",
			Companion:      "G",
			CompanionMass:  f64(1.0),
			CompanionTemp:  i(5700),
			CompanionSepAU: f64(0.05),
		},
	}
	g := NewGenerator(nil, 11)
	data := rockyCircumbinary(g, w)
	got := data["temperature"].(float64)
	assert.Greater(t, got, 900.0, "F+G a=0.05: T > 900 K — выше потолка экстремального архетипа, клампов нет")
	assert.LessOrEqual(t, got, 2500.0, "в [20, 2500]")
}

// TestCircumbinaryGiantAllowed — гиганты P-типа разрешены (шанс
// gasGiantChance(спектр главной)); на выборке тесных двойных гиганты есть.
func TestCircumbinaryGiantAllowed(t *testing.T) {
	g := NewGenerator(nil, 13)
	giants := 0
	const n = 400
	for i := 0; i < n; i++ {
		w := closeWorld(0.05 + g.rng.Float64()*0.85)
		p := g.generateCircumbinaryPlanet(w)
		data := planetJSON(t, p)
		if data["is_gas_giant"] == true {
			giants++
		}
		assert.Equal(t, "barycenter", data["orbit_center"])
	}
	assert.Greater(t, giants, 10, "гиганты P-типа должны встречаться (шанс 0.3–0.8)")
}

// TestCircumbinaryWaterLifeByBands — вода/жизнь по каскаду: G+G при a=0.5
// (умеренно-тёплая полоса) — вода и жизнь возможны (гидросфера гейтится
// флагом, жизнь требует жидкой воды/подлёдного океана).
func TestCircumbinaryWaterLifeByBands(t *testing.T) {
	g := NewGenerator(nil, 17)
	withWater := 0
	withLife := 0
	const n = 200
	for i := 0; i < n; i++ {
		data := rockyCircumbinary(g, closeWorld(0.5))
		water := data["water_percent"].(float64)
		if water > 10 {
			withWater++
		}
		if data["life"] == true {
			withLife++
		}
		assert.GreaterOrEqual(t, data["temperature"].(float64), 20.0, "в [20, 2500]")
		assert.LessOrEqual(t, data["temperature"].(float64), 2500.0)
	}
	assert.Greater(t, withWater, 0, "в пригодной полосе вода возможна")
	assert.Greater(t, withLife, 0, "жизнь возможна в полосе 200<T<400 (шанс > 0)")
}

// TestCircumbinaryHotNoLife — выжженная P-планета (T=2500): жизнь невозможна
// (вне полосы 200–400).
func TestCircumbinaryHotNoLife(t *testing.T) {
	w := WorldInfo{
		ID: "w1", Name: "World", SpectralClass: "O", Temperature: 35000,
		StarType: "star", SystemType: "binary",
		Mods: &models.StellarMods{
			BinaryType: "close", Companion: "O",
			CompanionMass: f64(30), CompanionTemp: i(35000), CompanionSepAU: f64(0.05),
		},
	}
	g := NewGenerator(nil, 5)
	for i := 0; i < 50; i++ {
		p := g.generateCircumbinaryPlanet(w)
		data := planetJSON(t, p)
		assert.Equal(t, false, data["life"], "при T=2500 жизнь невозможна")
	}
}

// TestCircumbinaryOldWorldFallsBackToMain — старый close-мир без
// companion_sep_au: P-ветка не вызывается, планеты идут общим путём как
// S-тип (фолбэк §2.4 — orbit_center=main).
func TestCircumbinaryOldWorldFallsBackToMain(t *testing.T) {
	w := WorldInfo{
		ID: "w1", Name: "World", SpectralClass: "G", Temperature: 5700,
		StarType: "star", SystemType: "binary",
		Mods: &models.StellarMods{BinaryType: "close", Companion: "M"},
	}
	g := NewGenerator(nil, 9)
	buf := newBatchBuffers(8)
	n := g.generateWorldWithCountIntoBuffer(w, 1, buf)
	require.Equal(t, 1, n)
	require.NotEmpty(t, buf.planetRows)
	row := buf.planetRows[0].([]interface{})
	data := row[4].(string)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(data), &parsed))
	assert.Equal(t, "main", parsed["orbit_center"], "старый мир без sep → S-путь (фолбэк §2.4)")
}

// TestSCircumbinaryHookInBatch — close-мир с sep: планета из батча уходит на
// P-ветку (orbit_center=barycenter). Wide/S-мир — orbit_center=main +
// orbit_radius_au=orbitRadiusByIndex.
func TestSCircumbinaryHookInBatch(t *testing.T) {
	g := NewGenerator(nil, 23)

	// Тесная: P-ветка.
	buf := newBatchBuffers(8)
	n := g.generateWorldWithCountIntoBuffer(closeWorld(0.3), 1, buf)
	require.Equal(t, 1, n)
	row := buf.planetRows[0].([]interface{})
	var pData map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(row[4].(string)), &pData))
	assert.Equal(t, "barycenter", pData["orbit_center"])
	assert.InDelta(t, 0.9, pData["orbit_radius_au"].(float64), 1e-9, "r_P = 3×0.3")

	// Широкая: S-путь, orbit_center=main, фактический радиус лэддера.
	wide := WorldInfo{
		ID: "w2", Name: "World2", SpectralClass: "G", Temperature: 5700,
		StarType: "star", SystemType: "binary",
		Mods: &models.StellarMods{
			BinaryType: "wide", Companion: "K",
			CompanionMass: f64(0.7), CompanionTemp: i(4500), CompanionSepAU: f64(200),
		},
	}
	buf2 := newBatchBuffers(8)
	n2 := g.generateWorldWithCountIntoBuffer(wide, 1, buf2)
	require.Equal(t, 1, n2)
	row2 := buf2.planetRows[0].([]interface{})
	var sData map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(row2[4].(string)), &sData))
	assert.Equal(t, "main", sData["orbit_center"], "S-планета вокруг главной")
	assert.InDelta(t, orbitRadiusByIndex(1), sData["orbit_radius_au"].(float64), 1e-9,
		"S: фактический радиус = orbitRadiusByIndex(1)")

	// Одиночная: orbit_center=main (не-регрессия).
	single := WorldInfo{ID: "w3", Name: "World3", SpectralClass: "G", Temperature: 5700}
	buf3 := newBatchBuffers(8)
	n3 := g.generateWorldWithCountIntoBuffer(single, 1, buf3)
	require.Equal(t, 1, n3)
	row3 := buf3.planetRows[0].([]interface{})
	var singleData map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(row3[4].(string)), &singleData))
	assert.Equal(t, "main", singleData["orbit_center"], "одиночная: планеты вокруг главной")
}

// TestCircumbinarySettlablePossible — пригодные P-планеты возможны
// (вариант A): G+G в пригодной полосе — settlement.Suitable проходит на
// части выборки (признак — political_system ≠ «нет»).
func TestCircumbinarySettlablePossible(t *testing.T) {
	g := NewGenerator(nil, 29)
	settleable := 0
	const n = 400
	for i := 0; i < n; i++ {
		a := 0.39 + g.rng.Float64()*0.26 // [0.39, 0.65] — пригодная полоса
		data := rockyCircumbinary(g, closeWorld(a))
		if data["political_system"] != "нет" {
			settleable++
		}
	}
	assert.Greater(t, settleable, 0, "пригодные P-планеты возможны на G-подобных парах")
}

// TestCircumbinaryNewFields — новые поля каскада (99.2.20 §4.1) пишутся
// P-планетам: atmosphere_data, liquid_water_possible, orbital_period,
// eccentricity, escape_velocity, tidal_lock.
func TestCircumbinaryNewFields(t *testing.T) {
	g := NewGenerator(nil, 31)
	data := rockyCircumbinary(g, closeWorld(0.5))
	_, hasAtm := data["atmosphere_data"].(map[string]interface{})
	assert.True(t, hasAtm, "atmosphere_data обязателен")
	_, hasFlag := data["liquid_water_possible"].(bool)
	assert.True(t, hasFlag, "liquid_water_possible обязателен")
	period, ok := data["orbital_period"].(float64)
	require.True(t, ok, "orbital_period обязателен")
	assert.Greater(t, period, 0.0)
	_, ok = data["eccentricity"].(float64)
	assert.True(t, ok, "eccentricity обязателен")
	_, ok = data["escape_velocity"].(float64)
	assert.True(t, ok, "escape_velocity обязателен")
	_, ok = data["tidal_lock"].(bool)
	assert.True(t, ok, "tidal_lock обязателен")
}