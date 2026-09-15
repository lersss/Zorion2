package astro

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLuminosityBySpectral — таблица светимостей по классу (35b §3, единый
// источник). Fallback для неизвестного класса — 1.0 («как Солнце»), закреплён
// тестами planet/physics_test.go (TestLuminosityBySpectral).
func TestLuminosityBySpectral(t *testing.T) {
	cases := map[string]float64{
		"O": 1000, "B": 100, "A": 10, "F": 2, "G": 1,
		"K": 0.1, "M": 0.01, "L": 0.001, "T": 0.0001, "Y": 0.00001,
	}
	for cls, want := range cases {
		assert.InDelta(t, want, LuminosityBySpectral(cls), 1e-12, "класс %s", cls)
	}
	assert.Equal(t, 1.0, LuminosityBySpectral("X"), "неизвестный класс — fallback 1.0")
	assert.Equal(t, 1.0, LuminosityBySpectral(""), "пустой класс — fallback 1.0")
}