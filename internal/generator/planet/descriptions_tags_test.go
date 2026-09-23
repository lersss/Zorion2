// internal/generator/planet/descriptions_tags_test.go
//
// Регресс тегов описаний (спека 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны
// §12.3/§14.4): мини-нептуны (8–16 M⊕, не гиганты) — всегда massive (порог 5);
// гиганты нижнего хвоста [16, 100] — без massive (порог massThresholdGas = 100,
// названное следствие); тяжёлые гиганты > 100 — massive.
package planet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMassTagsMiniNeptuneAndGiantTail(t *testing.T) {
	// Мини-нептун 12 M⊕ — не гигант: порог massThreshold = 5 → massive.
	mini := DescriptionContext{Type: TypeMiniNeptune, Mass: 12, IsGasGiant: false, Life: false}
	miniTags := computeTags(mini)
	assert.True(t, miniTags["massive"], "мини-нептун (8–16) всегда massive (порог 5)")
	assert.True(t, miniTags["no_biosphere"], "life=false → no_biosphere")

	// Гигант нижнего хвоста [16, 100] — без massive (порог 100).
	assert.False(t, computeTags(DescriptionContext{
		Type: TypeGasGiant, Mass: 50, IsGasGiant: true,
	})["massive"], "гигант [16, 100] без massive (§12.3)")

	// Тяжёлый гигант > 100 — massive.
	assert.True(t, computeTags(DescriptionContext{
		Type: TypeGasGiant, Mass: 200, IsGasGiant: true,
	})["massive"], "гигант > 100 → massive")

	// Каменистая 1 M⊕ — без massive.
	assert.False(t, computeTags(DescriptionContext{Mass: 1})["massive"])
}
