// internal/generator/planet/debris_belt_test.go
//
// Баг-фикс 2026-09-23: обломочный пояс WD (kind='debris') выбирал орбиту 5..8
// независимо от планет и мог сесть на занятую — планета оказывалась «внутри»
// пояса на схеме системы. Пояс обязан занимать СВОБОДНУЮ орбиту (образец —
// пояс астероидов), причём orbit_index у debris всегда непустой.
// TDD: красное до правки → зелёное после.
package planet

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// TestDebrisBeltFreeOrbit — на многих мирах WD с disk_state='debris' (каждый
// с планетой) утверждаем: нет пары «планета и пояс с одинаковым orbit_index»,
// debris-пояс получает непустой orbit_index, и при свободных выживших орбитах
// остаётся в диапазоне 5..8.
func TestDebrisBeltFreeOrbit(t *testing.T) {
	g := NewGenerator(nil, 20260957)
	buf := newBatchBuffers(4)
	debrisSeen, withPlanet := 0, 0
	for i := 0; i < 2000; i++ {
		buf.reset()
		w := WorldInfo{
			ID:       fmt.Sprintf("wd-%d", i),
			Name:     "WD",
			StarType: "white_dwarf",
			Mods:     &models.StellarMods{DiskState: "debris"},
		}
		// count = 1: планета есть у каждого мира (WD-планеты — орбиты 5..8).
		g.generateWorldWithCountIntoBuffer(w, 1, buf)

		planetOrbits := map[int]bool{}
		for _, row := range buf.planetRows {
			f := row.([]interface{})
			planetOrbits[f[3].(int)] = true
		}
		if len(planetOrbits) > 0 {
			withPlanet++
		}

		require.NotEmpty(t, buf.beltRows, "мир %s: ожидался debris-пояс", w.ID)
		for _, b := range beltsOf(t, buf) {
			if b.kind != "debris" {
				continue
			}
			debrisSeen++
			require.NotNil(t, b.orbit, "мир %s: orbit_index debris-пояса непустой", w.ID)
			assert.Falsef(t, planetOrbits[*b.orbit],
				"мир %s: debris-пояс на орбите %d совпал с планетой", w.ID, *b.orbit)
			// Одна планета занимает одну из 5..8 — три свободны, пояс
			// остаётся в выживших орбитах WD.
			assert.Truef(t, *b.orbit >= 5 && *b.orbit <= 8,
				"мир %s: орбита debris %d вне 5..8 при свободных выживших", w.ID, *b.orbit)
		}
	}
	require.Greater(t, debrisSeen, 0, "выборка содержит debris-пояса")
	require.Greater(t, withPlanet, 0, "выборка содержит миры с планетой")
	t.Logf("debris-поясов: %d, миров с планетой: %d", debrisSeen, withPlanet)
}
