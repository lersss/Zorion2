// internal/economy/settlement/effect_impact_test.go
// Критерии T16/T17 спеки 2026-09-22-эффекты-снабжения-задержка-голод (§12):
// реестр impact — открытый набор (неизвестный impact → 0 + лог); неизвестная
// кривая (params.curve) → 0 + лог curve_unknown; известный impact + найденная
// кривая → сила R(load).
package settlement

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// withCapturedLog — перехватить вывод лога на время f (для проверки логов-
// предупреждений impact_unknown/curve_unknown).
func withCapturedLog(t *testing.T, f func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)
	f()
	return buf.String()
}

func TestEffectRateKnown(t *testing.T) {
	lookup := func(key string) (EffectCurveEval, bool) {
		require.Equal(t, "hunger", key)
		return func(load float64) float64 { return load * 2 }, true
	}
	require.InDelta(t, 48.0, EffectRate(ImpactPopulationRate, "hunger", 24, lookup), 1e-12)
}

func TestEffectRateUnknownImpact(t *testing.T) {
	var got float64
	out := withCapturedLog(t, func() {
		got = EffectRate("stability", "hunger", 100, func(string) (EffectCurveEval, bool) {
			return func(float64) float64 { return 1 }, true
		})
	})
	require.Zero(t, got, "неизвестный impact → вклад 0 (открытый набор, §5.4)")
	require.Contains(t, out, "impact_unknown")
}

func TestEffectRateUnknownCurve(t *testing.T) {
	var got float64
	out := withCapturedLog(t, func() {
		got = EffectRate(ImpactPopulationRate, "nope", 100, func(string) (EffectCurveEval, bool) {
			return nil, false
		})
	})
	require.Zero(t, got, "неизвестная кривая → вклад 0 (§7.4)")
	require.Contains(t, out, "curve_unknown")

	// nil-lookup — тоже no-op + лог (не паника).
	out2 := withCapturedLog(t, func() {
		got = EffectRate(ImpactPopulationRate, "hunger", 100, nil)
	})
	require.Zero(t, got)
	require.Contains(t, out2, "curve_unknown")
	require.False(t, strings.Contains(out2, "panic"))
}
