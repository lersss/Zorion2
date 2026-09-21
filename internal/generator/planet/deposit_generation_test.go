// internal/generator/planet/deposit_generation_test.go
//
// Тесты генерации залежей поверхности (спека 2026-09-22-поселение-добыча-
// сырья-биома-ленивый-буфер §3.1/§3.2): случайное число из трёх пилотов,
// предикат поверхности, отсутствие карты goods, перезапись без дублей.
// Числа не фиксированы — тесты на поведение и границы дизайн-ручек.
package planet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// testGoodsIndex — карта пилотов «name_norm → goods.id» (§3.1: Растения 358,
// Мясо 359, вода-ресурс 1).
func testGoodsIndex() map[string]int64 {
	return map[string]int64{"растения": 358, "мясо": 359, "вода-ресурс": 1}
}

// genPlanetWithDeposits — планета общего пути + ролл залежей с картой пилотов.
func genPlanetWithDeposits(g *Generator) *PlanetData {
	pd := g.generatePlanet("w1", "Мир", 1, stellarParamsFromClass("G", 0, g.rng))
	g.generateDeposits(pd)
	return pd
}

// T1/T4: число залежей в границах ручек; ресурс — из трёх пилотов;
// wealth ∈ [wealthMin, wealthMax], stratum = 'surface', amount > 0.
func TestGenerateDepositsFromPilots(t *testing.T) {
	g := NewGenerator(nil, 42)
	g.SetGoodsIndex(testGoodsIndex())

	pilotIDs := map[int64]bool{358: true, 359: true, 1: true}
	sawDeposit := false
	for i := 0; i < 300; i++ {
		pd := genPlanetWithDeposits(g)
		require.GreaterOrEqual(t, len(pd.Deposits), depositsMin, "число залежей ≥ depositsMin")
		require.LessOrEqual(t, len(pd.Deposits), depositsMax, "число залежей ≤ depositsMax")
		for _, d := range pd.Deposits {
			sawDeposit = true
			require.True(t, pilotIDs[d.GoodID], "good_id — из трёх пилотов")
			require.Equal(t, "surface", d.Stratum)
			require.GreaterOrEqual(t, d.Wealth, depositWealthMin)
			require.LessOrEqual(t, d.Wealth, depositWealthMax)
			require.Greater(t, d.Amount, 0.0, "запас генерации строго > 0")
			require.GreaterOrEqual(t, d.Amount, depositAmountBase*(1-depositJitter))
			require.LessOrEqual(t, d.Amount, depositAmountBase*(1+depositJitter))
			require.NotEmpty(t, d.ID)
			require.Equal(t, pd.ID, d.PlanetID)
		}
	}
	require.True(t, sawDeposit, "за 300 планет должна родиться хотя бы одна залежь")
}

// T2: предикат поверхности по данным — газовый гигант без залежей; планета с
// surface_composition (общий путь / экзотический остаток) — с залежами;
// пустая surface_composition — поверхности нет.
func TestDepositsSurfacePredicate(t *testing.T) {
	g := NewGenerator(nil, 1)
	g.SetGoodsIndex(testGoodsIndex())

	gas := &PlanetData{ID: "gas", Data: []byte(`{"is_gas_giant":true}`)}
	for i := 0; i < 20; i++ {
		gas.Deposits = nil
		g.generateDeposits(gas)
		require.Empty(t, gas.Deposits, "газовый гигант залежей не получает")
	}

	// Экзотический остаток: surface_composition есть (камни/кратеры).
	exotic := &PlanetData{ID: "exotic", Data: []byte(`{"surface_composition":{"камни":60,"кратеры":40},"archetype":"мёртвый"}`)}
	saw := false
	for i := 0; i < 100 && !saw; i++ {
		exotic.Deposits = nil
		g.generateDeposits(exotic)
		saw = len(exotic.Deposits) > 0
	}
	require.True(t, saw, "тело с поверхностью получает залежи (в т.ч. экзотика)")

	empty := &PlanetData{ID: "nov", Data: []byte(`{"surface_composition":{}}`)}
	for i := 0; i < 20; i++ {
		empty.Deposits = nil
		g.generateDeposits(empty)
		require.Empty(t, empty.Deposits, "пустая surface_composition — поверхности нет")
	}
}

// T12: без инъекции карты goods залежей нет (неизвестные пилоты
// пропускаются), с картой — рождаются.
func TestDepositsWithoutGoodsIndex(t *testing.T) {
	g := NewGenerator(nil, 3)
	g.SetGoodsIndex(nil)
	for i := 0; i < 80; i++ {
		pd := genPlanetWithDeposits(g)
		require.Empty(t, pd.Deposits, "без карты goods залежей нет")
	}

	g2 := NewGenerator(nil, 4)
	g2.SetGoodsIndex(testGoodsIndex())
	saw := false
	for i := 0; i < 300 && !saw; i++ {
		saw = len(genPlanetWithDeposits(g2).Deposits) > 0
	}
	require.True(t, saw, "с картой goods залежи рождаются")
}

// T12: повторный проход перезаписывает залежи, а не удваивает (не плодит
// дубликаты).
func TestGenerateDepositsOverwrites(t *testing.T) {
	g := NewGenerator(nil, 5)
	g.SetGoodsIndex(testGoodsIndex())
	pd := &PlanetData{ID: "p", Data: []byte(`{"surface_composition":{"камни":100}}`)}
	g.generateDeposits(pd)
	g.generateDeposits(pd)
	require.LessOrEqual(t, len(pd.Deposits), depositsMax,
		"двойной вызов не удваивает число залежей")
}

// T5: несколько залежей одного ресурса на планете допустимы (§3.2, нет UNIQUE).
func TestMultipleDepositsSameResourceAllowed(t *testing.T) {
	g := NewGenerator(nil, 7)
	g.SetGoodsIndex(testGoodsIndex())
	found := false
	for i := 0; i < 400 && !found; i++ {
		counts := map[int64]int{}
		for _, d := range genPlanetWithDeposits(g).Deposits {
			counts[d.GoodID]++
		}
		for _, c := range counts {
			if c > 1 {
				found = true
			}
		}
	}
	require.True(t, found, "несколько пятен одного ресурса на планете допустимы")
}
