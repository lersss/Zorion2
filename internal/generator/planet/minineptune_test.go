// internal/generator/planet/minineptune_test.go
//
// Тесты класса «мини-нептун» (спека
// 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны §8, §12.1: O11–O13):
// кривая M→R на реальных анкерах, стык с кривой гигантов, классификация,
// тело без биомов/недр.
package planet

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== O11: КРИВАЯ M→R МИНИ-НЕПТУНА (§8.2) ====================

// O11 — R_minineptune(8.6) ≈ 2.61 (K2-18 b), R(17.1) ≈ 3.88 (Нептун — точка
// стыка), R(16) ≈ 3.73; α ≈ 0.577; монотонна; стык с кривой гигантов
// R_giant(16) = 3.70 (Δ ≈ 0.03 R⊕).
func TestMiniNeptuneRadiusCurve(t *testing.T) {
	assert.InDelta(t, 2.61, MiniNeptuneRadius(8.6), 0.01, "K2-18 b — анкер кривой")
	assert.InDelta(t, 3.88, MiniNeptuneRadius(17.1), 0.02, "Нептун — точка стыка")
	assert.InDelta(t, 3.73, MiniNeptuneRadius(16), 0.02, "R(16) — верх зоны класса")
	assert.InDelta(t, 2.50, MiniNeptuneRadius(8), 0.03, "R(8) — низ зоны (канон M_crit = 8)")
	assert.InDelta(t, 0.577, minineptuneAlpha, 0.001, "показатель α")

	// Монотонность на всей зоне класса.
	prev := 0.0
	for m := massMax; m <= GasGiantMassMin; m += 0.05 {
		r := MiniNeptuneRadius(m)
		require.Greater(t, r, prev, "R строго растёт (M = %.2f)", m)
		prev = r
	}

	// Стык кривых непрерывен: разрыв 3.73 → 5.91 прежнего анкера устранён.
	junction := MiniNeptuneRadius(GasGiantMassMin) - GasGiantRadius(GasGiantMassMin)
	assert.InDelta(t, 0.03, junction, 0.02, "R_minineptune(16) ≈ R_giant(16) (Δ ≈ 0.03)")
}

// ==================== O12: КЛАССИФИКАЦИЯ (§11.2) ====================

// O12 — IsMiniNeptune → «мини-нептун»; IsGasGiant приоритетнее; правила
// справочника мини-нептун не трогают (флаг класса вне правил).
func TestMiniNeptuneClassification(t *testing.T) {
	assert.Equal(t, TypeMiniNeptune, ClassifyGameDesignType(PlanetClassificationInput{
		IsMiniNeptune: true,
	}), "мини-нептун — явный флаг класса")
	assert.Equal(t, TypeGasGiant, ClassifyGameDesignType(PlanetClassificationInput{
		IsGasGiant: true, IsMiniNeptune: true,
	}), "газовый гигант приоритетнее")
	// Без флага тип определяется правилами справочника (не мини-нептун).
	assert.NotEqual(t, TypeMiniNeptune, ClassifyGameDesignType(PlanetClassificationInput{
		Surface:     Composition{SurfaceRocks: 1},
		Temperature: 300,
	}), "правила справочника не выдают мини-нептун")
	assert.Contains(t, AllGameDesignTypes, TypeMiniNeptune, "тип в списке всех типов")

	// Маппинг описаний знает тип (иначе генерация сыпала бы логом «неизвестный
	// тип» на каждой планете класса; контент — подэтап 2/3, @writer).
	assert.Equal(t, "minineptune", typeToFolder(TypeMiniNeptune),
		"тип зарегистрирован в маппинге описаний (нейтральная заготовка)")
}

// ==================== O13: ТЕЛО БЕЗ БИОМОВ/НЕДР (§8.3) ====================

// O13 — biomes/subterrain пустые, life = false, settleable = false,
// is_mini_neptune = true, is_gas_giant = false; R на кривой §8.2.
func TestMiniNeptuneNoBiomes(t *testing.T) {
	g := NewGenerator(nil, 20260935)

	var found int
	for i := 0; i < 2000 && found < 20; i++ {
		buf := newBatchBuffers(16)
		runOverflowWorld(g, buf, "G")
		for _, row := range buf.planetRows {
			f := row.([]interface{})
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(f[4].(string)), &data))
			if data["is_mini_neptune"] != true {
				continue
			}
			found++
			mass := data["mass"].(float64)
			size := data["size"].(float64)
			require.Greater(t, mass, massMax, "мини-нептун выше порога убегания")
			require.LessOrEqual(t, mass, GasGiantMassMin, "мини-нептун в (M_crit, 16]")
			assert.InDelta(t, MiniNeptuneRadius(mass), size, 1e-9, "R — по кривой мини-нептуна")
			assert.InDelta(t, mass/(size*size*size), data["density"].(float64), 1e-9, "ρ = M/R³")
			assert.InDelta(t, mass/(size*size), data["gravity"].(float64), 1e-9, "g = M/R²")
			assert.Equal(t, false, data["life"], "life = false")
			assert.NotEqual(t, true, data["is_gas_giant"], "is_gas_giant = false")
			assert.Equal(t, TypeMiniNeptune, data["type"], "type = мини-нептун")
			assert.Equal(t, "мини-нептун", data["surface_dominant"])
			assert.Empty(t, data["biomes"], "биомов нет (норма класса)")
			assert.Empty(t, data["subterrain"], "недр нет (норма класса)")
			moons, ok := data["moons"].(float64)
			require.True(t, ok, "moons обязателен")
			assert.LessOrEqual(t, moons, 4.0, "спутники 0–4 (гипотеза §8.3)")
		}
	}
	require.Greater(t, found, 0, "мини-нептуны в выборке есть (тест не пустой)")
}
