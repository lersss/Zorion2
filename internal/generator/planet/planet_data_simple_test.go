package planet

import (
	"encoding/json"
	"testing"
)

// «Примитивные» планеты (1–2 типа поверхность/недра) — доля ~3–4%, не больше.
// Покрывает все пути: стандартный архетип, океанические, радиоактивные.
func TestPlanetsSimpleRare(t *testing.T) {
	g := NewGenerator(nil, 5)
	classes := []string{"O", "B", "A", "F", "G", "K", "M"}

	total := 0
	simple := 0
	sanity := 0

	for i := 0; i < 20000; i++ {
		spec := classes[g.rng.Intn(len(classes))]
		orbit := g.rng.Intn(9)
		age := determineSystemAge(spec, g.rng)
		pd := g.generatePlanet("w", "World", orbit, spec, age)

		raw := map[string]interface{}{}
		if err := json.Unmarshal(pd.Data, &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		surf, ok := raw["surface_composition"].(map[string]interface{})
		if !ok {
			continue // газовый гигант — без композиций
		}
		sub, ok := raw["subterrain_composition"].(map[string]interface{})
		if !ok {
			t.Fatalf("планета %s без композиции недр", pd.Name)
		}

		total++
		if len(surf) == 0 || len(sub) == 0 {
			t.Fatalf("планета %s получила пустую композицию: surface=%d subterrain=%d", pd.Name, len(surf), len(sub))
		}
		sumSurf, sumSub := 0.0, 0.0
		for _, v := range surf {
			sumSurf += v.(float64)
		}
		for _, v := range sub {
			sumSub += v.(float64)
		}
		if sumSurf < 99.5 || sumSurf > 100.5 || sumSub < 99.5 || sumSub > 100.5 {
			t.Fatalf("планета %s: суммы композиций surface=%.1f subterrain=%.1f != 100", pd.Name, sumSurf, sumSub)
		}
		sanity++
		if len(surf) <= 2 && len(sub) <= 2 {
			simple++
		}
	}

	if total == 0 {
		t.Fatal("no planets generated")
	}

	frac := float64(simple) / float64(total)
	t.Logf("total=%d simple(<=2 форм)=%.2f%% sanity=%d", total, frac*100, sanity)
	if frac > primitivePlanetProbability+0.02 {
		t.Errorf("примитивных планет %.2f%% — больше допустимых 3-4%% (порог %v)", frac*100, primitivePlanetProbability)
	}
}