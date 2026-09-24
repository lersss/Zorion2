// Тесты разбивки средового R по компонентам (идея 2026-09-25 «R суммарный с
// составом в карточке поселения», §1/§4): EnvBreakdown — единый источник,
// сумма ряда ТОЧНО равна EnvComponents; набор кодов человеческой и расовой
// моделей; нулевые вклады присутствуют; нет записи расы → фолбэк на людей.
package settlement

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// sumContributions — сумма вкладов разбивки.
func sumContributions(list []ComponentContribution) float64 {
	var sum float64
	for _, c := range list {
		sum += c.Value
	}
	return sum
}

// codesOf — карта «код → вклад» разбивки.
func codesOf(list []ComponentContribution) map[string]float64 {
	out := map[string]float64{}
	for _, c := range list {
		out[c.Code] = c.Value
	}
	return out
}

// Человеческая модель: сумма == EnvComponents; шесть кодов; нулевые вклады
// (холод/радиация) присутствуют рядом с ненулевыми.
func TestEnvBreakdownHumansSumAndCodes(t *testing.T) {
	input := PlanetInput{TemperatureK: 400, GravityG: 1.0, CoreRadioactivity: 5}
	got := EnvBreakdown(input)

	require.InDelta(t, EnvComponents(input), sumContributions(got), 1e-20,
		"сумма рядов == EnvComponents (единый источник)")
	require.Len(t, got, 6, "человеческая модель несёт шесть рядов")

	codes := codesOf(got)
	for _, code := range []string{"natural", "birth", "heat", "cold", "gravity", "radiation"} {
		_, ok := codes[code]
		require.True(t, ok, "человеческая модель несёт ряд %q", code)
	}
	require.Positive(t, codes["heat"], "400 K — жара положительна")
	require.Positive(t, codes["natural"], "естественная смертность положительна")
	require.Negative(t, codes["birth"], "рождаемость отрицательна (рост)")
	require.Zero(t, codes["cold"], "нулевой вклад присутствует")
	require.Zero(t, codes["radiation"], "нулевой вклад присутствует")
}

// Расовая модель: сумма == EnvComponents; пять кодов (без birth — нетто расы
// в natural); нулевые вклады присутствуют.
func TestEnvBreakdownRaceSumAndCodes(t *testing.T) {
	loadRaceStoreForTests(t)

	input := PlanetInput{TemperatureK: 255, GravityG: 1, CoreRadioactivity: 10, RaceID: "ammonia"}
	got := EnvBreakdown(input)

	require.InDelta(t, EnvComponents(input), sumContributions(got), 1e-20,
		"сумма рядов расы == EnvComponents")
	require.Len(t, got, 5, "расовая модель без ряда birth (нетто — в natural)")

	codes := codesOf(got)
	for _, code := range []string{"natural", "heat", "cold", "gravity", "radiation"} {
		_, ok := codes[code]
		require.True(t, ok, "расовая модель несёт ряд %q", code)
	}
	_, hasBirth := codes["birth"]
	require.False(t, hasBirth, "у расы нет отдельного ряда birth")
	require.Zero(t, codes["cold"], "нулевой вклад присутствует (аммиачник 255 K — жара, не холод)")
}

// Нет записи расы в store → человеческая модель (гвард 99.2.23 §3.2).
func TestEnvBreakdownRaceFallbackHuman(t *testing.T) {
	loadRaceStoreForTests(t)

	fallback := EnvBreakdown(PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5, RaceID: "no_such_race"})
	human := EnvBreakdown(PlanetInput{TemperatureK: 303.15, GravityG: 1.0, CoreRadioactivity: 5})
	require.Equal(t, human, fallback, "раса без записи → человеческая разбивка")
}
