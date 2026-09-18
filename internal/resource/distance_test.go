// internal/resource/distance_test.go — проверка различимости каталога
// реальных веществ (идея 2026-09-18 §4б): числа оценки 92/110/40 из §4а
// становятся живым кодом. Порог: «отличаются хотя бы по одной оси на 10»
// (решение создателя §1.3); T-разрешение — |T_melt| или |T_boil| ≥ 10 K.
package resource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealCatalogSize(t *testing.T) {
	require.Len(t, RealCatalog(), 111, "каталог: 111 реальных веществ")
}

func TestRealCatalogShape(t *testing.T) {
	for _, r := range RealCatalog() {
		require.NotEmpty(t, r.ID, "id у %s", r.Name)
		require.NotEmpty(t, r.Name)
		require.NotEmpty(t, r.Family, "семейство у %s", r.Name)
		require.Contains(t, AllCategories, r.Category, "категория у %s", r.Name)
		// Оси 0–100.
		for _, v := range []float64{r.Hardness, r.Elasticity, r.Conductivity, r.Density,
			r.EnergyDensity, r.Biocompatibility, r.Radioactivity, r.Toxicity,
			r.Flammability, r.ChemicalActivity} {
			require.GreaterOrEqual(t, v, 0.0, "ось ≥ 0 у %s", r.Name)
			require.LessOrEqual(t, v, 100.0, "ось ≤ 100 у %s", r.Name)
		}
		// T в диапазоне модели [10, 6000] (кламп He/He-3 и WC).
		require.GreaterOrEqual(t, r.TMelt, 10.0, "T_melt ≥ 10 у %s", r.Name)
		require.GreaterOrEqual(t, r.TBoil, 10.0, "T_boil ≥ 10 у %s", r.Name)
		require.LessOrEqual(t, r.TBoil, 6000.0, "T_boil ≤ 6000 у %s", r.Name)
	}
}

func TestRealCatalogSublimating(t *testing.T) {
	// Механическое правило TBoil ≤ TMelt (как Resource.Sublimating, layer.go).
	// По таблице §4б «*» помечены графит/алмаз/ацетилен/CO₂, но у графита и
	// алмаза T_boil > T_melt (3800/3900, 4000/4500) — механически НЕ
	// сублимирующие (реальные графит/алмаз сублимируют при нормальном
	// давлении, но оцифровка даёт T_melt < T_boil). He/He-3 (кламп 10/10 K)
	// механически «сублимирующие» — артефакт клампа, не физика.
	subl := map[string]bool{}
	for _, r := range RealCatalog() {
		if r.Sublimating() {
			subl[r.Name] = true
		}
	}
	require.Equal(t, map[string]bool{
		"CO₂ сухой лёд": true, "Ацетилен C₂H₂": true, "Гелий He": true, "Гелий-3": true,
	}, subl)
}

func TestMaxAxisDiffAndColliding(t *testing.T) {
	he := RealByID("gelij")
	he3 := RealByID("gelij_3")
	require.NotNil(t, he)
	require.NotNil(t, he3)
	// Гелий и гелий-3 — идентичные профили (единственная неразрешимая пара §4а).
	require.Equal(t, 0.0, MaxAxisDiff(he, he3))
	require.True(t, Colliding(he, he3))
	require.False(t, ResolvedByT(he, he3), "оба кламп 10 K → T не разводит")

	// Алмаз vs графит: твёрдость 100 vs 15 → различимы по осям.
	almaz := RealByID("almaz")
	grafit := RealByID("grafit")
	require.Equal(t, 85.0, MaxAxisDiff(almaz, grafit))
	require.False(t, Colliding(almaz, grafit))

	// Вода vs аммиак: T_melt 273 vs 195 → T-разрешение есть.
	voda := RealByID("voda")
	ammiak := RealByID("ammiak")
	require.True(t, ResolvedByT(voda, ammiak))
}

func TestDistinctCountCatalog(t *testing.T) {
	byAxes, withT, collisions := DistinctCount(RealCatalog())

	// Оценка §4а: 92 по осям / 110 с T / ~40 коллизий. Фактические числа на
	// каталоге §4б (транскрипция таблицы @designer) — 86/107/43. Расхождение:
	//  1) кросс-семейные коллизии, которых не было в прогоне оценки:
	//     N₂↔благородные газы, природный газ↔метан, T₂↔этан/пропан/бутан
	//     (связывает алканы с изотопами H₂), SO₂/NO₂/NO;
	//  2) T-порог ≥ 10 K: природный газ↔метан (T 90/110 vs 91/112 — разница
	//     1–2 K) и H₂↔D₂↔T₂ (2–7 K) не разрешаются; оценка 110 предполагала
	//     «любое различие T»;
	//  3) кламп T: He/He-3 оба 10 K → не разрешаются (единственная пара,
	//     неразрешимая в принципе — в игре решают имя и география §9.1.4).
	// Вывод §4а не меняется: ~80% различимы по осям, ~96% с T, коллизии —
	// гомологические семейства.
	require.Equal(t, 86, byAxes, "различимы по осям (оценка 92)")
	require.Equal(t, 107, withT, "с учётом T (оценка 110)")
	require.Equal(t, 43, len(collisions), "коллизий по осям (оценка ~40)")

	// He↔He-3 — единственная пара, неразрешимая из-за клампа T.
	he := RealByID("gelij")
	he3 := RealByID("gelij_3")
	require.True(t, Colliding(he, he3))
	require.False(t, ResolvedByT(he, he3))
}