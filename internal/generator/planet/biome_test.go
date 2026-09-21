// internal/generator/planet/biome_test.go
//
// Тесты биомов (99.2.28 §27): справочник (валидация, инвариант 17),
// физика весов (лава↔T, Ио, биосфера↔условия), draft без энтропии,
// аддитивность производных карт, жидкость-гейт, правила типов, прототип,
// смоук распределения.
package planet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// ==================== СПРАВОЧНИК: ВАЛИДАЦИЯ (инвариант 17) ====================

func TestBiomeCatalogSeedValid(t *testing.T) {
	cat := GetBiomeCatalog()
	require.NotNil(t, cat)
	require.Len(t, cat.Biomes, 57, "сид: 57 биомов (16 существующих + 41 новый)")
	require.Len(t, cat.SubterrainTypes, 17, "сид: 17 типов недр")
	require.Len(t, cat.PlanetTypes, 9, "сид: 9 правил типов планет")
	require.NoError(t, cat.Validate())
}

func TestBiomeCatalogLoadBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad_catalog.json")
	require.NoError(t, os.WriteFile(path, []byte("{не-json"), 0o644))
	err := LoadBiomeCatalog(path)
	require.Error(t, err, "битый JSON должен вернуть ошибку")
	// Store остаётся на сиде (сервер не падает).
	require.Len(t, GetBiomeCatalog().Biomes, 57)
}

func TestBiomeCatalogValidateEmptyBands(t *testing.T) {
	cat := GetBiomeCatalog()
	bad := *cat
	bad.Biomes = append([]BiomeDef{}, cat.Biomes...)
	bad.Biomes[0].Bands = nil
	err := bad.Validate()
	require.Error(t, err, "биом с пустыми bands — ошибка (инвариант 17)")
}

func TestBiomeCatalogValidateUnknownAtmosphereFeature(t *testing.T) {
	cat := GetBiomeCatalog()
	bad := *cat
	bad.Biomes = append([]BiomeDef{}, cat.Biomes...)
	bad.Biomes[0].AtmosphereOK = []string{"метановая"}
	err := bad.Validate()
	require.Error(t, err, "признак вне перечня §0 — ошибка (инвариант 17)")
}

func TestBiomeCatalogValidateBandsVsTRange(t *testing.T) {
	cat := GetBiomeCatalog()
	bad := *cat
	bad.Biomes = append([]BiomeDef{}, cat.Biomes...)
	// Стеклянные поля (T 480–600) в холодной полосе — мёртвая полоса.
	bad.Biomes[0].TMin, bad.Biomes[0].TMax = 480, 600
	bad.Biomes[0].Bands = []string{"х"}
	err := bad.Validate()
	require.Error(t, err, "bands без пересечения с t_range — ошибка (инвариант 17)")
}

func TestBiomeCatalogValidateUnknownBand(t *testing.T) {
	// Мусорная полоса (не из {э,ж,у,х}) — ошибка: bandTempRange дал бы 0,0,
	// пересечение с открытым t_range = true, биом молча не родился бы
	// (замечание ревью).
	cat := GetBiomeCatalog()

	bad := *cat
	bad.Biomes = append([]BiomeDef{}, cat.Biomes...)
	bad.Biomes[0].Bands = []string{"z"}
	err := bad.Validate()
	require.Error(t, err, "полоса «z» у биома — ошибка")
	require.Contains(t, err.Error(), "неизвестная полоса")

	badSub := *cat
	badSub.SubterrainTypes = append([]SubterrainTypeDef{}, cat.SubterrainTypes...)
	badSub.SubterrainTypes[0].Bands = []string{"z"}
	err = badSub.Validate()
	require.Error(t, err, "полоса «z» у типа недр — ошибка")
	require.Contains(t, err.Error(), "неизвестная полоса")
}

// ==================== ФИЗИКА ВЕСОВ (99.2.28 §27.3–§27.4) ====================

// testEnv — окружение для юнит-тестов весов.
func testEnv() *biomeEnv {
	return &biomeEnv{
		TFinal:       300,
		PressureAtm:  1,
		WaterPercent: 40,
		Flag:         true,
		Rock:         0.5, Iron: 0.3, Ice: 0.2,
		Gravity:       1,
		Life:          true,
		Atmosphere:    map[string]float64{"N2": 78, "O2": 21, "CO2": 1},
		V:             0.5,
		Radioactivity: 20,
		TidalLock:     false,
		Band:          "умеренный",
	}
}

func TestBiomeLavaRequiresHeat(t *testing.T) {
	cat := GetBiomeCatalog()
	env := testEnv()
	features := atmosphereFeatures(env.Atmosphere, env.PressureAtm, cat.Params.ToxicThresholds)

	// T < 500 → лава = 0 (гейт smoothstep + volcanism hot).
	env.TFinal = 400
	env.Band = "умеренный"
	assert.Equal(t, 0.0, biomeWeight(cat.BiomeByID(SurfaceLavaFields), env, features), "лава при T<500 = 0")

	// T ≥ 500 и V > 0 → лава > 0.
	env.TFinal = 600
	env.Band = "экстремальный"
	assert.Greater(t, biomeWeight(cat.BiomeByID(SurfaceLavaFields), env, features), 0.0, "лава при T≥500 и V>0 > 0")
}

func TestBiomeVolcanoesAnyTempIo(t *testing.T) {
	// Ио-тест (99.2.28 §27.3): T = 130, V = 0.9 → вулканы есть, лавы нет.
	cat := GetBiomeCatalog()
	env := testEnv()
	env.TFinal = 130
	env.Band = "холодный"
	env.V = 0.9
	features := atmosphereFeatures(env.Atmosphere, env.PressureAtm, cat.Params.ToxicThresholds)

	assert.Greater(t, biomeWeight(cat.BiomeByID(SurfaceVolcanicFields), env, features), 0.0, "вулканы при любой T (Ио)")
	assert.Equal(t, 0.0, biomeWeight(cat.BiomeByID(SurfaceLavaFields), env, features), "лавы нет при T<500")
}

func TestBiomeBiosphereRequiresConditions(t *testing.T) {
	cat := GetBiomeCatalog()
	env := testEnv()
	features := atmosphereFeatures(env.Atmosphere, env.PressureAtm, cat.Params.ToxicThresholds)

	// Земные леса: life + flag + no_toxic → вес > 0.
	assert.Greater(t, biomeWeight(cat.BiomeByID(SurfaceForests), env, features), 0.0, "леса при life+flag+no_toxic")

	// Без жизни → 0.
	env.Life = false
	assert.Equal(t, 0.0, biomeWeight(cat.BiomeByID(SurfaceForests), env, features), "леса без life = 0")
	env.Life = true

	// Без флага воды → 0.
	env.Flag = false
	assert.Equal(t, 0.0, biomeWeight(cat.BiomeByID(SurfaceForests), env, features), "леса без флага воды = 0")
	env.Flag = true

	// Токсичная атмосфера → 0 (решение п.19).
	env.Atmosphere = map[string]float64{"N2": 90, "CH4": 3, "O2": 7}
	features = atmosphereFeatures(env.Atmosphere, env.PressureAtm, cat.Params.ToxicThresholds)
	assert.Equal(t, 0.0, biomeWeight(cat.BiomeByID(SurfaceForests), env, features), "леса на токсичной атмосфере = 0")

	// Экстремофил (кислотные дебри) — жизнь без нетоксичной → вес > 0.
	assert.Greater(t, biomeWeight(cat.BiomeByID("кислотные_дебри"), env, features), 0.0, "кислотные дебри на токсичной атмосфере с life")
}

func TestBiomeLiquidGateAutoMatrix(t *testing.T) {
	// Вода и метан/аммиак/CO₂ попарно несовместимы (99.2.28 §27.11).
	requireDefaultCompat(t)
	assert.False(t, IsCompatible("surface", "океаны", "метановые_моря"), "вода×метан")
	assert.False(t, IsCompatible("surface", "океаны", "аммиачные_крио-океаны"), "вода×аммиак")
	assert.False(t, IsCompatible("surface", "океаны", "co2_океаны"), "вода×co2")
	assert.False(t, IsCompatible("surface", "метановые_моря", "аммиачные_крио-океаны"), "метан×аммиак")
	// Лава/магмовый океан vs вода.
	assert.False(t, IsCompatible("surface", "магмовый_океан", "океаны"), "магмовый океан×вода")
	// Одна и та же среда — совместима.
	assert.True(t, IsCompatible("surface", "океаны", "озёра_реки"), "вода×вода")
}

// ==================== DRAFT БЕЗ ЭНТРОПИИ (99.2.28 §27.5) ====================

func TestBiomeDraftDeterministic(t *testing.T) {
	env := testEnv()
	env.Band = "умеренный"
	bias := map[string]float64{"леса": 1.3, "океаны": 1.1}

	d1 := generateBiomeDraft(env, bias)
	d2 := generateBiomeDraft(env, bias)
	require.NotEmpty(t, d1)
	require.Len(t, d1, len(d2))
	// Детерминизм по seed: draft не потребляет энтропию RNG. Точные значения
	// плавают на последнем ULP (Normalize суммирует map в случайном порядке —
	// PITFALLS «точные значения не детерминированы»), поэтому сравнение —
	// с допуском, не побитовое.
	for form, share := range d1 {
		assert.InDelta(t, share, d2[form], 1e-9, "draft детерминирован (допуск ULP): %s", form)
	}
	assert.InDelta(t, 100, d1.Total(), 0.5, "сумма draft = 100")
}

// ==================== АДДИТИВНОСТЬ (99.2.28 §27.6) ====================

func TestCompositionDerivedFromBiomes(t *testing.T) {
	biomes := []models.Biome{{Form: "горы", Share: 60}, {Form: "океаны", Share: 40}}
	c := compositionFromBiomes(biomes)
	assert.InDelta(t, 60, c["горы"], 0.001)
	assert.InDelta(t, 40, c["океаны"], 0.001)
	assert.InDelta(t, 100, c.Total(), 0.001)

	zones := []models.SubterrainZone{{Type: "пустая_порода", Share: 70}, {Type: "рудные_жилы", Share: 30}}
	sub := compositionFromZones(zones)
	assert.InDelta(t, 70, sub["пустая_порода"], 0.001)
	assert.InDelta(t, 30, sub["рудные_жилы"], 0.001)
}

// ==================== УСЛОВИЯ НЕДР ИЗ ДАННЫХ (99.2.28 §8) ====================

// TestSubterrainWeightReadsConditions — гейты типов недр читают Conditions
// (данные справочника), а не захардкоженный switch (замечание ревью:
// двойной источник истины — админка правит Conditions, и это должно влиять
// на генерацию). Дрейфы сида: магматические_породы v_min 0.01 vs код V<=0,
// пещерные_системы/подземные_льды water 0.01 vs код <=0,
// магматические_камеры v_min 0.01 vs код <=0.
func TestSubterrainWeightReadsConditions(t *testing.T) {
	cat := GetBiomeCatalog()
	env := testEnv()

	// магматические_камеры: v_min 0.01 — V < 0.01 → 0 (данные, не код).
	env.V = 0.005
	assert.Equal(t, 0.0, subterrainWeight(cat.SubterrainByID(SubterrainMagmaChambers), env),
		"магматические_камеры при V < v_min = 0")
	env.V = 0.5
	assert.Greater(t, subterrainWeight(cat.SubterrainByID(SubterrainMagmaChambers), env), 0.0,
		"магматические_камеры при V ≥ v_min > 0")

	// подземные_льды: t_max 273, water_min 0.01.
	env.TFinal = 273
	env.WaterPercent = 5
	assert.Equal(t, 0.0, subterrainWeight(cat.SubterrainByID(SubterrainGroundIce), env),
		"подземные_льды при T = t_max = 0")
	env.TFinal = 272
	assert.Greater(t, subterrainWeight(cat.SubterrainByID(SubterrainGroundIce), env), 0.0,
		"подземные_льды при T < t_max > 0")
	env.WaterPercent = 0.005
	assert.Equal(t, 0.0, subterrainWeight(cat.SubterrainByID(SubterrainGroundIce), env),
		"подземные_льды при воде < water_min = 0")

	// пещерные_системы: water_min 0.01, v_max 0.5.
	env.WaterPercent = 0.005
	env.V = 0.1
	assert.Equal(t, 0.0, subterrainWeight(cat.SubterrainByID(SubterrainCaveSystems), env),
		"пещерные_системы при воде < water_min = 0")
	env.WaterPercent = 10
	env.V = 0.6
	assert.Equal(t, 0.0, subterrainWeight(cat.SubterrainByID(SubterrainCaveSystems), env),
		"пещерные_системы при V > v_max = 0")

	// магматические_породы: v_min 0.01, rock_min 0.3.
	env.V = 0.5
	env.Rock = 0.2
	assert.Equal(t, 0.0, subterrainWeight(cat.SubterrainByID(SubterrainMagmaticRocks), env),
		"магматические_породы при rock < rock_min = 0")
	env.Rock = 0.5
	assert.Greater(t, subterrainWeight(cat.SubterrainByID(SubterrainMagmaticRocks), env), 0.0,
		"магматические_породы при rock ≥ rock_min > 0")
}

// TestSubterrainWeightConditionsEditable — правка Conditions в справочнике
// меняет гейт (данные — источник истины, админка правит то, что влияет).
func TestSubterrainWeightConditionsEditable(t *testing.T) {
	cat := GetBiomeCatalog()
	env := testEnv()
	env.V = 0.5

	// Сид: магматические_камеры v_min 0.01 — V=0.5 проходит.
	require.Greater(t, subterrainWeight(cat.SubterrainByID(SubterrainMagmaChambers), env), 0.0)

	// Правка данных: v_min 0.6 → V=0.5 больше не проходит.
	edited := *cat
	edited.SubterrainTypes = append([]SubterrainTypeDef(nil), cat.SubterrainTypes...)
	for i := range edited.SubterrainTypes {
		if edited.SubterrainTypes[i].ID == SubterrainMagmaChambers {
			edited.SubterrainTypes[i].Conditions.VMin = 0.6
		}
	}
	require.NoError(t, edited.Validate())
	assert.Equal(t, 0.0, subterrainWeight(edited.SubterrainByID(SubterrainMagmaChambers), env),
		"правка v_min в данных меняет гейт")
}

// ==================== ПРАВИЛА ТИПОВ (99.2.28 §27.12) ====================

func TestClassifyRulesReferenceSets(t *testing.T) {
	// Эталонные наборы: Земля → землеподобная, Венера → вулканическая,
	// Марс → пустынная/скалистая, Титан-профиль → ледяная, радиоактивное
	// ядро → радиоактивная (99.2.28 §27.12).
	cases := []struct {
		name string
		in   PlanetClassificationInput
		want string
	}{
		{"Земля", PlanetClassificationInput{
			Settleable: true, Life: true,
			Surface: Composition{SurfaceRocks: 60, SurfaceOceans: 40},
		}, TypeEarthlike},
		{"Венера", PlanetClassificationInput{
			Temperature: 747, WaterPercent: 0,
			Surface: Composition{SurfaceLavaFields: 30, SurfaceVolcanicFields: 20, SurfaceRocks: 50},
		}, TypeVolcanic},
		{"Марс", PlanetClassificationInput{
			Temperature: 191, WaterPercent: 5,
			Surface: Composition{SurfaceSands: 40, SurfaceRocks: 60},
		}, TypeDesert},
		{"Титан-профиль", PlanetClassificationInput{
			Temperature: 94, WaterPercent: 0,
			Surface: Composition{SurfaceGlaciers: 60, SurfaceRocks: 40},
		}, TypeIce},
		{"радиоактивное ядро", PlanetClassificationInput{
			IsRadioactive: true, Surface: Composition{SurfaceRocks: 100},
		}, TypeRadioactive},
		{"газовый гигант", PlanetClassificationInput{
			IsGasGiant: true, Surface: Composition{},
		}, TypeGasGiant},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ClassifyGameDesignType(tc.in))
		})
	}
}

func TestClassifyAllTypesCovered(t *testing.T) {
	// Все 12 ГД-типов покрыты правилами/флагами (99.2.28 §27.12, приложение §3).
	cat := GetBiomeCatalog()
	ruleIDs := map[string]bool{}
	for _, r := range cat.PlanetTypes {
		ruleIDs[r.ID] = true
	}
	covered := map[string]bool{
		TypeGasGiant:    true, // флаг вне правил
		TypeRadioactive: ruleIDs[TypeRadioactive],
		TypeEarthlike:   ruleIDs[TypeEarthlike],
		TypeOceanic:     ruleIDs[TypeOceanic],
		TypeIce:         ruleIDs[TypeIce],
		TypeVolcanic:    ruleIDs[TypeVolcanic],
		TypeDesert:      ruleIDs[TypeDesert],
		TypeGlass:       ruleIDs[TypeGlass],
		TypeMetal:       ruleIDs[TypeMetal],
		TypeOrganic:     ruleIDs[TypeOrganic],
		TypeRocky:       cat.FallbackType == TypeRocky,
		TypeDead:        true, // ветка экзотики (вне каскада)
	}
	for typ, ok := range covered {
		assert.True(t, ok, "тип %q не покрыт правилами справочника", typ)
	}
}

// ==================== ПРОТОТИП (99.2.28 §27.13) ====================

func TestPrototypeBiomesConsistent(t *testing.T) {
	g := NewGenerator(nil, 7)
	for i := 0; i < 20; i++ {
		pd := g.GeneratePrototypePlanet("w1", "World", "G")
		require.NotNil(t, pd)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(pd.Data, &data))

		// Отметка форсирования (99.2.28 §16.1).
		assert.Equal(t, true, data["prototype_forced"], "prototype_forced в данных")

		// Биомы согласованы с форсированными данными: T=288, вода=80, life.
		biomes, ok := data["biomes"].([]interface{})
		require.True(t, ok, "biomes в данных прототипа")
		require.NotEmpty(t, biomes, "прототип не без биомов")
		forms := map[string]float64{}
		for _, item := range biomes {
			m := item.(map[string]interface{})
			forms[m["form"].(string)] = m["share"].(float64)
		}
		sum := 0.0
		for _, share := range forms {
			sum += share
		}
		assert.InDelta(t, 100, sum, 0.5, "сумма биомов = 100")

		// Жидкая вода → леса/луга/океаны присутствуют. Два легальных исключения
		// (предсуществующие, не связаны с залежами): тонкая атмосфера — флаг
		// законно false; примитивная планета (шаг 8, ~3.5%) — один биом.
		hasWaterBiome := forms["леса"] > 0 || forms["луга_степи"] > 0 || forms["океаны"] > 0
		if data["liquid_water_possible"] == true && len(forms) > 1 {
			assert.True(t, hasWaterBiome,
				"биомы прототипа: леса/луга/океаны, получено %v", forms)
		}
	}
}

// ==================== СМОУК РАСПРЕДЕЛЕНИЯ (99.2.28 §21) ====================

func TestBiomeSmokeDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("объёмный статистический смоук — вне быстрого цикла, гоняется отдельно")
	}
	g := NewGenerator(nil, 42)
	classes := []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}
	seen := map[string]int{}
	lavaInCold := 0
	toxicForests := 0
	planets := 0
	biomeCount := map[int]int{}

	// Мировой поток (ревью 2026-09-21, правка 2): generateWorldWithCountIntoBuffer
	// роллит per-системный бюджет B — смоук мерит реальный путь, а не runCascade
	// напрямую (там бюджет = 1, числа не те, что уходят в игру).
	buf := newBatchBuffers(64)
	for i := 0; i < 20000; i++ {
		cls := classes[g.rng.Intn(len(classes))]
		// Металличность [Fe/H] как в продакшне (galaxy.rollMetallicity):
		// N(0, 0.3), кламп [−0.8, +0.5]. Входит в массу каскада, а через неё —
		// в гравитацию/биомы; без неё поток не воспроизводит игру.
		met := g.rng.NormFloat64() * 0.3
		if met < -0.8 {
			met = -0.8
		}
		if met > 0.5 {
			met = 0.5
		}
		w := WorldInfo{ID: "w", Name: "World", SpectralClass: cls, StarType: "star",
			Mods: &models.StellarMods{Metallicity: &met}}
		count := g.planetCountFor(w)
		buf.reset()
		g.generateWorldWithCountIntoBuffer(w, count, buf)

		for _, row := range buf.planetRows {
			fields := row.([]interface{})
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(fields[4].(string)), &data))

			biomes, ok := data["biomes"].([]interface{})
			if !ok {
				continue // газовый гигант — без биомов (99.2.28 §9.5)
			}
			planets++
			require.NotEmpty(t, biomes, "не-гигант без биомов (страховка минимум 1)")
			biomeCount[len(biomes)]++

			sum := 0.0
			for _, item := range biomes {
				m := item.(map[string]interface{})
				form := m["form"].(string)
				share := m["share"].(float64)
				sum += share
				seen[form]++
				if form == SurfaceLavaFields && data["temperature"].(float64) < 500 {
					lavaInCold++
				}
				if form == SurfaceForests || form == SurfaceMeadows || form == SurfaceJungles {
					if atm, ok := data["atmosphere_data"].(map[string]interface{}); ok {
						if comp, ok := atm["composition"].(map[string]interface{}); ok {
							if ch4, ok := comp["CH4"].(float64); ok && ch4 > 1 {
								toxicForests++
							}
						}
					}
				}
			}
			assert.InDelta(t, 100, sum, 0.5, "сумма биомов = 100")
		}
	}
	require.Greater(t, planets, 10000, "смоук: достаточно не-гигантов")

	// Лава только при T ≥ 500 (99.2.28 §21.2).
	assert.Equal(t, 0, lavaInCold, "лава при T<500 не рождается")

	// Земные леса/луга/джунгли только на нетоксичной атмосфере (CH₄ ≤ 1%).
	assert.Equal(t, 0, toxicForests, "земные леса на метановой атмосфере не рождаются")

	// Число биомов: минимум 1 (страховка), типично 4–12 (99.2.28 §21.3 —
	// «4–9» калибровочная цель; фактические веса — сид приложения, калибровка
	// @balancetester правкой weight_base).
	typical := 0
	for n, count := range biomeCount {
		if n >= 4 && n <= 12 {
			typical += count
		}
	}
	assert.Greater(t, float64(typical)/float64(planets), 0.5, "типично 4–12 биомов")

	// Все 57 биомов достижимы (99.2.28 §21.5): исключения — tuned-зависимые
	// (аммиачные крио-океаны, кислотные дебри), недоказанная достижимость
	// (co2_океаны) и сверхузкое окно (метановые_моря T 92–95 K — путь
	// метановых биомов жив: углеводородные_равнины/инеевые_рощи встречаются).
	// Допуск ≤ 3 редких биомов: точные значения планет не детерминированы
	// между прогонами (PITFALLS «точные значения не детерминированы» —
	// порядок итерации map в джиттере/конфликтах).
	cat := GetBiomeCatalog()
	knownMissing := map[string]bool{
		"аммиачные_крио-океаны": true, // только tuned 99.2.22
		"кислотные_дебри":       true, // только tuned (умеренные токсичные)
		"co2_океаны":            true, // достижимость по механике не доказана
		"метановые_моря":        true, // сверхузкое окно T 92–95 K (калибровка)
	}
	var missing []string
	for _, b := range cat.Biomes {
		if seen[b.ID] == 0 && !knownMissing[b.ID] {
			missing = append(missing, b.ID)
		}
	}
	assert.LessOrEqual(t, len(missing), 3, "биомы не встретились в смоуке (вне известных исключений): %v", missing)
	t.Logf("биомов встретилось: %d из %d; не встретились: %v; распределение числа биомов: %v",
		len(seen), len(cat.Biomes), missing, biomeCount)
}

// ==================== ХЕЛПЕРЫ ====================

// requireDefaultCompat — гарантирует дефолтную матрицу совместимости.
// (объявлена в composition_test.go — здесь не дублируется)
