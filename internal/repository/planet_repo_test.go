package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ==================== МИГРАЦИЯ 000069 (спека поясов этап 3 §4) ====================

// M17: колонка iron_remaining — nullable (NULL «нет данных»), CHECK >= 0,
// без DEFAULT (0 = «выработан» — самостоятельное состояние, а не дефолт).
func TestMigrationIronRemaining(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000069_belt_mining.sql"))
	require.NoError(t, err, "миграция 000069_belt_mining.sql должна существовать")
	s := string(src)
	require.Contains(t, s, "ADD COLUMN IF NOT EXISTS iron_remaining DOUBLE PRECISION")
	require.Contains(t, s, "CHECK (iron_remaining >= 0)", "инвариант «запас не уходит в минус»")
	require.NotContains(t, s, "iron_remaining DOUBLE PRECISION NOT NULL",
		"NULL = «нет данных о запасе» — колонка nullable")
	require.NotContains(t, s, "DEFAULT 0",
		"без DEFAULT: старый мир не должен молча выглядеть выработанным")
}

// ==================== МИГРАЦИЯ 000078 (спека 2026-09-24 §4) ====================

// M22: колонка ice_remaining — nullable (NULL двусмыслен: «нет данных о льде»
// ИЛИ «льда в поясе нет»), CHECK >= 0, без DEFAULT (0 = «выработан» —
// самостоятельное состояние, а не дефолт).
func TestMigrationIceRemaining(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000078_belt_ice_remaining.sql"))
	require.NoError(t, err, "миграция 000078_belt_ice_remaining.sql должна существовать")
	s := string(src)
	require.Contains(t, s, "ADD COLUMN IF NOT EXISTS ice_remaining DOUBLE PRECISION")
	require.Contains(t, s, "CHECK (ice_remaining >= 0)", "инвариант «запас не уходит в минус»")
	require.NotContains(t, s, "ice_remaining DOUBLE PRECISION NOT NULL",
		"NULL = «нет данных/льда нет» — колонка nullable")
	require.NotContains(t, s, "DEFAULT 0",
		"без DEFAULT: старый мир не должен молча выглядеть выработанным")
}

// ==================== getStr ====================

func TestGetStr(t *testing.T) {
	data := map[string]interface{}{
		"ok":    "значение",
		"wrong": 42,
		"nil":   nil,
	}

	assert.Equal(t, "значение", getStr(data, "ok"))
	assert.Equal(t, "", getStr(data, "missing"), "отсутствующий ключ")
	assert.Equal(t, "", getStr(data, "wrong"), "не-строка")
	assert.Equal(t, "", getStr(data, "nil"))
	assert.Equal(t, "", getStr(nil, "ok"), "nil map")
}

// ==================== getFloat ====================

func TestGetFloat(t *testing.T) {
	data := map[string]interface{}{
		"ok":    288.5,
		"wrong": "288.5",
		"int":   7,
	}

	assert.InDelta(t, 288.5, getFloat(data, "ok"), 0.0001)
	assert.InDelta(t, 0, getFloat(data, "missing"), 0.0001)
	assert.InDelta(t, 0, getFloat(data, "wrong"), 0.0001)
	assert.InDelta(t, 0, getFloat(data, "int"), 0.0001)
}

// ==================== getBool ====================

func TestGetBool(t *testing.T) {
	data := map[string]interface{}{
		"yes":    true,
		"no":     false,
		"string": "true",
	}

	assert.True(t, getBool(data, "yes"))
	assert.False(t, getBool(data, "no"))
	assert.False(t, getBool(data, "missing"))
	assert.False(t, getBool(data, "string"))
}

// ==================== getFloatMap ====================

func TestGetFloatMap(t *testing.T) {
	data := map[string]interface{}{
		"compositions": map[string]interface{}{
			"горы":     60.0,
			"океаны":    30.5,
			"не-число":  "ошибка",
			"пропущено": float64(9),
		},
	}

	m := getFloatMap(data, "compositions")
	assert.InDelta(t, 60.0, m["горы"], 0.0001)
	assert.InDelta(t, 30.5, m["океаны"], 0.0001)
	_, ok := m["не-число"]
	assert.False(t, ok, "значения-не числа отбрасываются")

	empty := getFloatMap(data, "missing")
	assert.NotNil(t, empty, "пустой результат не должен быть nil")
	assert.Empty(t, empty)
}

// ==================== parseCore ====================

func TestParseCoreValid(t *testing.T) {
	data := map[string]interface{}{
		"core": map[string]interface{}{
			"type":          "железное",
			"mass_percent":  30.0,
			"activity":      55.0,
			"radioactivity": 8.0,
			"age":           4.2,
			"is_active":     true,
			"is_metallic":   true,
		},
	}

	core := parseCore(data)
	assert.NotNil(t, core)
	assert.Equal(t, "железное", core.Type)
	assert.Equal(t, 30.0, core.MassPercent)
	assert.Equal(t, 55.0, core.Activity)
	assert.Equal(t, 8.0, core.Radioactivity)
	assert.Equal(t, 4.2, core.Age)
	assert.True(t, core.IsActive)
	assert.True(t, core.IsMetallic)
}

func TestParseCoreNilCases(t *testing.T) {
	assert.Nil(t, parseCore(map[string]interface{}{}), "нет ключа core")
	assert.Nil(t, parseCore(map[string]interface{}{"core": "не-объект"}), "core не объект")

	empty := parseCore(map[string]interface{}{"core": map[string]interface{}{}})
	assert.NotNil(t, empty, "пустой core — тоже объект ядра")
	assert.Equal(t, "", empty.Type)
	assert.Equal(t, 0.0, empty.MassPercent)
}

// ==================== parseSatellites ====================

func TestParseSatellitesValid(t *testing.T) {
	data := map[string]interface{}{
		"satellites": []interface{}{
			map[string]interface{}{
				"id": "s1", "name": "Луна-1", "orbit_index": float64(3),
				"size": 0.27, "mass": 0.012, "temperature": 210.0,
				"water_percent": 60.0, "habitable": false, "life": true,
				"atmosphere": "азотная", "biosphere": "примитивная",
				"surface_dominant": "лёд",
				"surface_composition": map[string]interface{}{"лёд": 90.0},
				"description":        "Описание спутника",
			},
		},
	}

	sats := parseSatellites(data)
	assert.Len(t, sats, 1)
	s := sats[0]
	assert.Equal(t, "s1", s.ID)
	assert.Equal(t, "Луна-1", s.Name)
	assert.Equal(t, 3, s.OrbitIndex)
	assert.Equal(t, 0.27, s.Size)
	assert.Equal(t, 0.012, s.Mass)
	assert.Equal(t, 210.0, s.Temperature)
	assert.Equal(t, 60.0, s.WaterPercent)
	assert.False(t, s.Habitable)
	assert.True(t, s.Life)
	assert.Equal(t, "азотная", s.Atmosphere)
	assert.Equal(t, "примитивная", s.Biosphere)
	assert.Equal(t, "лёд", s.SurfaceDominant)
	assert.InDelta(t, 90.0, s.SurfaceComposition["лёд"], 0.0001)
	assert.InDelta(t, 0.0, s.SubterrainComposition["лёд"], 0.0001)
	assert.Equal(t, "Описание спутника", s.Description)
}

func TestParseSatellitesEdgeCases(t *testing.T) {
	// Неверный элемент пропускается, ключ отсутствует — nil.
	data := map[string]interface{}{
		"satellites": []interface{}{
			"не-объект",
			map[string]interface{}{"id": "s2", "name": "Луна-2"},
		},
	}
	sats := parseSatellites(data)
	assert.Len(t, sats, 1)
	assert.Equal(t, "s2", sats[0].ID)

	assert.Nil(t, parseSatellites(map[string]interface{}{}), "нет ключа satellites")
	assert.Empty(t, parseSatellites(map[string]interface{}{"satellites": []interface{}{}}))
	assert.Nil(t, parseSatellites(map[string]interface{}{"satellites": "не-массив"}))
}

// ==================== орбитальный контекст (35b §2.2) ====================

// TestPopulatePlanetFromJSONOrbitContext — P-планета (orbit_center=barycenter,
// orbit_radius_au=3a, circumbinary=true) и S-планета (orbit_center=main)
// мапятся из data (whitelist).
func TestPopulatePlanetFromJSONOrbitContext(t *testing.T) {
	// P-планета: циркумбинарная, вокруг барицентра пары.
	pData := map[string]interface{}{
		"orbit_center":    "barycenter",
		"orbit_radius_au": 0.9,
		"circumbinary":    true,
	}
	p := models.Planet{}
	populatePlanetFromJSON(&p, pData)
	assert.Equal(t, "barycenter", p.OrbitCenter)
	assert.InDelta(t, 0.9, p.OrbitRadiusAU, 1e-9)
	assert.True(t, p.Circumbinary)

	// S-планета: вокруг главной.
	sData := map[string]interface{}{
		"orbit_center":    "main",
		"orbit_radius_au": 0.68,
	}
	s := models.Planet{}
	populatePlanetFromJSON(&s, sData)
	assert.Equal(t, "main", s.OrbitCenter)
	assert.InDelta(t, 0.68, s.OrbitRadiusAU, 1e-9)
	assert.False(t, s.Circumbinary, "S-планета не циркумбинарная")
}

// TestPopulatePlanetFromJSONOrbitFallbacks — старые миры без ключей
// (35b §2.4): orbit_center → "main", circumbinary → false; радиус не трогаем
// (фронт считает из orbit_index).
func TestPopulatePlanetFromJSONOrbitFallbacks(t *testing.T) {
	p := models.Planet{}
	populatePlanetFromJSON(&p, map[string]interface{}{})

	assert.Equal(t, "main", p.OrbitCenter, "нет ключа orbit_center → фолбэк main")
	assert.False(t, p.Circumbinary, "нет ключа circumbinary → false")
	assert.InDelta(t, 0.0, p.OrbitRadiusAU, 1e-9, "радиус не вычисляется — 0 без ключа")
}

// ==================== populatePlanetFromJSON ====================

func TestPopulatePlanetFromJSONFull(t *testing.T) {
	data := map[string]interface{}{
		"type": "углеродная", "surface_dominant": "высокие горы", "archetype": "умеренный",
		"size": 1.2, "mass": 0.9, "density": 1.1, "temperature": 288.5,
		"water_percent": 40.0,
		"atmosphere":    "азотно-кислородная", "hydrosphere": "моря", "biosphere": "развитая",
		"habitable": true, "life": true,
		"surface_composition": map[string]interface{}{"высокие горы": 45.0, "моря": 40.0, "льды": 15.0},
		"subterrain_composition": map[string]interface{}{"гранит": 50.0},
		"core": map[string]interface{}{
			"type": "каменное", "mass_percent": 27.0, "activity": 45.0,
			"radioactivity": 3.0, "age": 4.0, "is_active": true,
		},
		"is_gas_giant": true,
		"satellites": []interface{}{
			map[string]interface{}{"id": "sat-1", "name": "Ганимед-подобный", "orbit_index": 1},
		},
		"description": "Влажный мир", "system_age": 4.6, "moons": float64(2), "radioactive": false,
	}

	p := models.Planet{}
	populatePlanetFromJSON(&p, data)

	assert.Equal(t, "углеродная", p.Type)
	assert.Equal(t, "высокие горы", p.SurfaceDominant)
	assert.Equal(t, "умеренный", p.Archetype)
	assert.Equal(t, 1.2, p.Size)
	assert.Equal(t, 0.9, p.Mass)
	assert.Equal(t, 1.1, p.Density)
	assert.Equal(t, 288.5, p.Temperature)
	assert.Equal(t, 40.0, p.WaterPercent)
	assert.Equal(t, "азотно-кислородная", p.Atmosphere)
	assert.Equal(t, "моря", p.Hydrosphere)
	assert.Equal(t, "развитая", p.Biosphere)
	assert.False(t, p.Habitable, "обитаемость не берётся из JSON — вычисляется из поселений")
	assert.True(t, p.Life)
	assert.Equal(t, int64(0), p.Population, "население не берётся из JSON — вычисляется из поселений")

	assert.InDelta(t, 45.0, p.SurfaceComposition["высокие горы"], 0.0001)
	assert.InDelta(t, 40.0, p.SurfaceComposition["моря"], 0.0001)
	assert.InDelta(t, 50.0, p.SubterrainComposition["гранит"], 0.0001)

	assert.NotNil(t, p.Core)
	assert.Equal(t, "каменное", p.Core.Type)
	assert.Equal(t, 27.0, p.Core.MassPercent)
	assert.True(t, p.Core.IsActive)

	assert.True(t, p.IsGasGiant)
	assert.Len(t, p.Satellites, 1)
	assert.Equal(t, "sat-1", p.Satellites[0].ID)

	assert.Equal(t, "Влажный мир", p.Description)
	assert.Equal(t, 4.6, p.SystemAge)
	assert.Equal(t, 2, p.Moons)
	assert.False(t, p.Radioactive)
}

func TestPopulatePlanetFromJSONEmpty(t *testing.T) {
	p := models.Planet{}
	populatePlanetFromJSON(&p, map[string]interface{}{})

	// Все поля — нулевые значения, композиции остаются пустыми (не nil).
	assert.Equal(t, "", p.Type)
	assert.Equal(t, 0.0, p.Temperature)
	assert.False(t, p.Habitable)
	assert.False(t, p.IsGasGiant)
	assert.Nil(t, p.Core)
	assert.Empty(t, p.Satellites)
	assert.NotNil(t, p.SurfaceComposition)
	assert.NotNil(t, p.SubterrainComposition)
	assert.InDelta(t, 0.0, p.SurfaceComposition["все"], 0.0001)
	// Биомы/недры-объекты: отсутствие ключа → nil (старый мир), не ошибка.
	assert.Nil(t, p.Biomes)
	assert.Nil(t, p.Subterrain)
}

func TestPopulatePlanetFromJSONBiomes(t *testing.T) {
	// Whitelist API (99.2.28 §9.3): biomes/subterrain отдаются; отсутствие
	// ключа → nil (PITFALLS «новый ключ молча пропадает»).
	data := map[string]interface{}{
		"biomes": []interface{}{
			map[string]interface{}{"form": "горы", "share": 60.0},
			map[string]interface{}{"form": "океаны", "share": 40.0},
		},
		"subterrain": []interface{}{
			map[string]interface{}{"type": "пустая_порода", "share": 70.0},
			map[string]interface{}{"type": "рудные_жилы", "share": 30.0},
		},
	}
	p := models.Planet{}
	populatePlanetFromJSON(&p, data)

	require.Len(t, p.Biomes, 2)
	assert.Equal(t, "горы", p.Biomes[0].Form)
	assert.InDelta(t, 60.0, p.Biomes[0].Share, 0.0001)
	assert.Equal(t, "океаны", p.Biomes[1].Form)

	require.Len(t, p.Subterrain, 2)
	assert.Equal(t, "пустая_порода", p.Subterrain[0].Type)
	assert.InDelta(t, 30.0, p.Subterrain[1].Share, 0.0001)
}

func TestPopulatePlanetFromJSONFormationHistory(t *testing.T) {
	// Whitelist API (спека 2026-09-22-облако-этап-2 §6.5): formation_history
	// доезжает до модели; отсутствие ключа → nil (PITFALLS «новый ключ молча
	// пропадает»).
	data := map[string]interface{}{
		"formation_history": []interface{}{
			map[string]interface{}{"type": "formed_early", "t_form_myr": 0.35, "x_ice": 4.09},
			map[string]interface{}{"type": "migrated", "x_form": 3.41, "x_now": 1.16, "direction": "inward", "factor": 2.94},
			map[string]interface{}{"type": "ice_lost", "fraction": 0.34, "residue": "iron"},
		},
	}
	p := models.Planet{}
	populatePlanetFromJSON(&p, data)

	require.Len(t, p.FormationHistory, 3)
	assert.Equal(t, "formed_early", p.FormationHistory[0].Type)
	assert.InDelta(t, 0.35, p.FormationHistory[0].TFormMyr, 1e-9)
	assert.InDelta(t, 4.09, p.FormationHistory[0].XIce, 1e-9)
	assert.Equal(t, "migrated", p.FormationHistory[1].Type)
	assert.Equal(t, "inward", p.FormationHistory[1].Direction)
	assert.InDelta(t, 2.94, p.FormationHistory[1].Factor, 1e-9)
	assert.Equal(t, "ice_lost", p.FormationHistory[2].Type)
	assert.Equal(t, "iron", p.FormationHistory[2].Residue)

	// Ключа нет → nil (старый мир), не ошибка.
	p2 := models.Planet{}
	populatePlanetFromJSON(&p2, map[string]interface{}{})
	assert.Nil(t, p2.FormationHistory)
}

func TestPopulatePlanetFromJSONRealSerialized(t *testing.T) {
	// Путь, как в БД: data JSON → map → populate.
	raw := `{
		"type":"землеподобная","surface_dominant":"океаны","archetype":"мягкий",
		"size":1.0,"mass":1.0,"density":1.0,"temperature":288.0,"water_percent":70.0,
		"atmosphere":"азотно-кислородная","hydrosphere":"океаны","biosphere":"развитая",
		"habitable":true,"life":true,
		"surface_composition":{"океаны":70.0,"горы":20.0,"льды":10.0},
		"core":{"type":"железное","mass_percent":32.5,"activity":55.0,"radioactivity":5.0,"age":4.5,"is_active":true,"is_metallic":true},
		"moons":1,"system_age":4.6
	}`

	var data map[string]interface{}
	assert.NoError(t, json.Unmarshal([]byte(raw), &data))

	p := models.Planet{}
	populatePlanetFromJSON(&p, data)

	assert.Equal(t, "землеподобная", p.Type)
	assert.Equal(t, "океаны", p.SurfaceDominant)
	assert.Equal(t, 288.0, p.Temperature)
	assert.Equal(t, 70.0, p.WaterPercent)
	assert.False(t, p.Habitable, "обитаемость не берётся из JSON — вычисляется из поселений")
	assert.True(t, p.Life)
	assert.Equal(t, int64(0), p.Population, "население не берётся из JSON — вычисляется из поселений")
	assert.NotNil(t, p.Core)
	assert.True(t, p.Core.IsMetallic)
	assert.Equal(t, 32.5, p.Core.MassPercent)
	assert.InDelta(t, 70.0, p.SurfaceComposition["океаны"], 0.0001)
	assert.Equal(t, 1, p.Moons)
	assert.Equal(t, 4.6, p.SystemAge)
	assert.False(t, p.Radioactive)
	assert.False(t, p.IsGasGiant)
}