package settlement

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldRegistryHasKnownFields(t *testing.T) {
	registry := FieldRegistry()
	require.NotEmpty(t, registry)

	// Обязательные поля, на которые ссылается DefaultModel.
	keys := map[string]bool{}
	for _, s := range registry {
		keys[s.Key] = true
	}
	for _, key := range []string{"temperature", "water_percent", "atmosphere", "is_gas_giant", "radioactive"} {
		assert.True(t, keys[key], "нет поля %q в реестре", key)
	}
}

func TestFieldSpecFor(t *testing.T) {
	s, ok := FieldSpecFor("temperature")
	require.True(t, ok)
	assert.Equal(t, FieldNumber, s.Type)
	assert.Equal(t, "°C", s.Unit)
	assert.Equal(t, -223.0, *s.Min)
	assert.Equal(t, 927.0, *s.Max)

	_, ok = FieldSpecFor("нет_такого_поля")
	assert.False(t, ok)
}

func TestFieldSpecHasValue(t *testing.T) {
	s, _ := FieldSpecFor("atmosphere")
	assert.True(t, s.HasValue("ядовитая"))
	assert.True(t, s.HasValue("азотно-кислородная"))
	assert.False(t, s.HasValue("чистый_кислород"))

	// Все значения реестра должны быть из допустимого набора (валидация
	// правил опирается на это).
	for _, key := range []string{"biosphere", "hydrosphere", "type"} {
		spec, ok := FieldSpecFor(key)
		require.True(t, ok)
		assert.NotEmpty(t, spec.Values, "%s должен иметь допустимые значения", key)
	}
}