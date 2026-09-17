// internal/races/tuning_test.go — вывод ручек подкрутки из карточки
// (спека 99.2.22 §3.3, §14.1–§14.3): режим, обогащение, тепло, веса звёзд,
// поверхность. Детерминизм и оф-модельные расы.
package races

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadTestCatalog — каталог рас для тестов (TestMain в пакете нет — грузим
// здесь; LoadCatalog идемпотентен).
func loadTestCatalog(t *testing.T) {
	t.Helper()
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.Len(t, Catalog(), 60)
}

// Аммиачники (5): холодный режим (T₁ = 188 < 273), обогащение NH₃ 5%
// (need NH₃ 1 → max(1,5)%), тепла нет, тёмная поверхность, home K/M.
func TestTuningAmmonia(t *testing.T) {
	loadTestCatalog(t)
	tun := ByID("ammonia").Tuning()
	require.NotNil(t, tun)
	assert.Equal(t, RegimeCold, tun.Regime)
	assert.InDelta(t, 217.5, tun.TempCenter, 0.01)
	assert.Equal(t, map[string]float64{"NH3": 0.05}, tun.Enrich, "NH₃ 1% → цель 5%")
	assert.Zero(t, tun.FIntTarget, "тепла нет (heat_flux не задан)")
	assert.True(t, tun.SurfaceDark, "холодные — тёмная поверхность (ручка 7-холод)")
	assert.Equal(t, map[string]float64{"K": 1.6, "M": 1.6}, tun.SpectralMult)
}

// Метан-грибницы (11): холодный, обогащение CH₄ 5% (need CH₄ 5 → max(5,5)%).
func TestTuningMethaneFungi(t *testing.T) {
	loadTestCatalog(t)
	tun := ByID("methane_fungi").Tuning()
	require.NotNil(t, tun)
	assert.Equal(t, RegimeCold, tun.Regime)
	assert.Equal(t, map[string]float64{"CH4": 0.05}, tun.Enrich, "CH₄ 5% → цель 5%")
	assert.Equal(t, map[string]float64{"M": 1.6, "L": 1.6, "T": 1.6}, tun.SpectralMult)
}

// Серные гнёзда (15): горячий, обогащение H₂S 5%, тепло F_int = 3 (Lo 1 × 3),
// O/B усилены (373 < opt_hi 480 ≤ 600 — умеренно-горячие).
func TestTuningSulfurNests(t *testing.T) {
	loadTestCatalog(t)
	tun := ByID("sulfur_nests").Tuning()
	require.NotNil(t, tun)
	assert.Equal(t, RegimeHot, tun.Regime)
	assert.Equal(t, map[string]float64{"H2S": 0.05}, tun.Enrich, "H₂S 1% → цель 5%")
	assert.InDelta(t, 3.0, tun.FIntTarget, 0.001, "F_int_target = Lo×3 = 3")
	assert.True(t, tun.SurfaceDark, "горячие — тёмная поверхность (ручка 7)")
	assert.Equal(t, map[string]float64{"K": 1.6, "M": 1.6, "O": 1.3, "B": 1.3}, tun.SpectralMult,
		"умеренно-горячие (opt_hi 480 ≤ 600) — O/B ×1.3")
}

// Курильщики (16): горячий, обогащения нет (все need = 0 — «не требуется»,
// фикс итерации 3), тепло F_int = 6 (Lo 2 × 3).
func TestTuningSmokers(t *testing.T) {
	loadTestCatalog(t)
	tun := ByID("smokers").Tuning()
	require.NotNil(t, tun)
	assert.Equal(t, RegimeHot, tun.Regime)
	assert.Empty(t, tun.Enrich, "need {H2S: 0, SO2: 0, CO2: 0} — обогащать нечего")
	assert.InDelta(t, 6.0, tun.FIntTarget, 0.001, "F_int_target = Lo×3 = 6")
}

// Сверхкритические (25): горячий, окно через 600 (opt_lo 550 < 600 < opt_hi 750)
// — O/B нейтральны (дефолт итерации 3).
func TestTuningSupercritical(t *testing.T) {
	loadTestCatalog(t)
	tun := ByID("supercritical").Tuning()
	require.NotNil(t, tun)
	assert.Equal(t, RegimeHot, tun.Regime)
	assert.NotContains(t, tun.SpectralMult, "O", "окно через 600 — O/B нейтральны")
	assert.NotContains(t, tun.SpectralMult, "B", "окно через 600 — O/B нейтральны")
	assert.Equal(t, map[string]float64{"F": 1.6, "G": 1.6, "K": 1.6}, tun.SpectralMult)
}

// Лавовые (24): очень горячие (opt_lo 700 ≥ 600) — O/B нейтральны.
func TestTuningLava(t *testing.T) {
	loadTestCatalog(t)
	tun := ByID("lava").Tuning()
	require.NotNil(t, tun)
	assert.Equal(t, RegimeHot, tun.Regime)
	assert.InDelta(t, 30.0, tun.FIntTarget, 0.001, "F_int_target = Lo×3 = 30")
	assert.NotContains(t, tun.SpectralMult, "O", "opt_lo ≥ 600 — O/B нейтральны")
}

// Глубинники (3): холодный режим (T₁ = 197.6 < 273 — режим не флипает),
// тепло F_int = 3, холодные тепловые (opt_hi 300 ≤ 373) — O/B нейтральны.
func TestTuningDeepDwellers(t *testing.T) {
	loadTestCatalog(t)
	tun := ByID("deep_dwellers").Tuning()
	require.NotNil(t, tun)
	assert.Equal(t, RegimeCold, tun.Regime, "T₁ = 197.6 < 273 — холодный режим")
	assert.InDelta(t, 3.0, tun.FIntTarget, 0.001, "F_int_target = Lo×3 = 3")
	assert.True(t, tun.SurfaceDark)
	assert.NotContains(t, tun.SpectralMult, "O", "opt_hi ≤ 373 — O/B нейтральны")
}

// Люди (1): умеренный режим, обогащения нет (O₂ не в режим-гейте; O₂ даёт
// жизненный проход), тепла нет, поверхности-оверрайда нет, home G/K.
func TestTuningHumans(t *testing.T) {
	loadTestCatalog(t)
	tun := ByID("humans").Tuning()
	require.NotNil(t, tun)
	assert.Equal(t, RegimeTemperate, tun.Regime)
	assert.Empty(t, tun.Enrich, "умеренный — режим-гейт {} (O₂ не обогащается)")
	assert.Zero(t, tun.FIntTarget)
	assert.False(t, tun.SurfaceDark, "умеренные — без оверрайда поверхности")
	assert.Equal(t, map[string]float64{"G": 1.6, "K": 1.6}, tun.SpectralMult)
}

// Крио-лесные (6): need {NH₃: 0, CH₄: 0} — обогащения нет (min = 0 =
// «не требуется», фикс итерации 3).
func TestTuningEnrichOnlyNeedPositive(t *testing.T) {
	loadTestCatalog(t)
	tun := ByID("cryo_forest").Tuning()
	require.NotNil(t, tun)
	assert.Equal(t, RegimeCold, tun.Regime)
	assert.Empty(t, tun.Enrich, "need {NH₃: 0, CH₄: 0} — обогащать нечего")
}

// Оф-модельная раса (40 Водородные, P «1000+»): подкрутки нет (nil) —
// планет в каскаде для неё нет, дыра не маскируется (99.2.21 §10).
func TestTuningOffCascade(t *testing.T) {
	loadTestCatalog(t)
	assert.Nil(t, ByID("hydrogen").Tuning(), "оф-модельная раса — без подкрутки")
}

// Детерминизм: два вызова Tuning() — одинаковый результат.
func TestTuningDeterminism(t *testing.T) {
	loadTestCatalog(t)
	a := ByID("sulfur_nests").Tuning()
	b := ByID("sulfur_nests").Tuning()
	require.NotNil(t, a)
	require.NotNil(t, b)
	assert.Equal(t, a.Regime, b.Regime)
	assert.Equal(t, a.Enrich, b.Enrich)
	assert.Equal(t, a.SpectralMult, b.SpectralMult)
	assert.InDelta(t, a.FIntTarget, b.FIntTarget, 0.001)
}

// Все 60 рас каталога дают валидный вывод (или nil для оф-модельных):
// режим из допустимого набора, обогащение в режим-гейте, веса > 0.
func TestTuningAllRacesValid(t *testing.T) {
	loadTestCatalog(t)
	for _, r := range Catalog() {
		tun := r.Tuning()
		if r.OffCascade {
			assert.Nil(t, tun, "оф-модельная %s — без подкрутки", r.ID)
			continue
		}
		require.NotNil(t, tun, "раса %s — вывод есть", r.ID)
		assert.Contains(t, []string{RegimeCold, RegimeTemperate, RegimeHot}, tun.Regime, r.ID)
		for gas, v := range tun.Enrich {
			assert.Greater(t, v, 0.0, "%s: доля %s > 0", r.ID, gas)
			assert.LessOrEqual(t, v, 0.2, "%s: доля %s ≤ 20%%", r.ID, gas)
		}
		for cls, v := range tun.SpectralMult {
			assert.Greater(t, v, 0.0, "%s: вес %s > 0 (все типы возможны)", r.ID, cls)
		}
	}
}
