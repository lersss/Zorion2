// internal/generator/planet/planet_image_test.go
package planet

import (
	"math/rand"
	"testing"
)

func testPlanetGenerator() *PlanetGenerator {
	return &PlanetGenerator{
		climates: []Climate{{
			ID:                  "temperate",
			Name:                "Умеренный",
			TemperatureMin:      200,
			TemperatureMax:      300,
			AllowedSurfaces:     []string{"каменистая"},
			AllowedHydrospheres: []string{"океаны"},
			AllowedAtmospheres:  []string{"плотная"},
			AllowedBiospheres:   []string{"сложная"},
		}},
		canvasSize:   64,
		enableCache:  true,
		cache:        make(map[string]*CachedPlanet),
		cacheOrder:   []string{},
		maxCacheSize: 2000,
		rand:         rand.New(rand.NewSource(1)),
	}
}

func TestGeneratePlanetCacheHit(t *testing.T) {
	pg := testPlanetGenerator()

	a, err := pg.GeneratePlanet(20, WithSeed(42), WithClimateID("temperate"))
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	b, err := pg.GeneratePlanet(20, WithSeed(42), WithClimateID("temperate"))
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if a != b {
		t.Fatalf("ожидался хит кэша — одинаковый seed должен вернуть тот же *CachedPlanet")
	}
	if len(pg.cache) != 1 {
		t.Fatalf("в кэше должно быть 1 планета, got %d", len(pg.cache))
	}
	if a.Meta.Seed != 42 || a.Meta.Radius != 20 {
		t.Fatalf("meta испорчена: seed=%d radius=%d", a.Meta.Seed, a.Meta.Radius)
	}
}

func TestGeneratePlanetCacheMissOnDifferentSeed(t *testing.T) {
	pg := testPlanetGenerator()

	a, err := pg.GeneratePlanet(20, WithSeed(1), WithClimateID("temperate"))
	if err != nil {
		t.Fatalf("seed=1: %v", err)
	}
	b, err := pg.GeneratePlanet(20, WithSeed(2), WithClimateID("temperate"))
	if err != nil {
		t.Fatalf("seed=2: %v", err)
	}
	if a == b {
		t.Fatalf("разные seed должны давать разные планеты")
	}
	if len(pg.cache) != 2 {
		t.Fatalf("в кэше должно быть 2 планеты, got %d", len(pg.cache))
	}
}

func TestGenerateKeyConsistency(t *testing.T) {
	opts := &GenerateOptions{Radius: 20, StarType: "G", ClimateID: "temperate"}

	k1 := generateKey(7, opts)
	k2 := generateKey(7, opts)
	if k1 != k2 {
		t.Fatalf("одинаковые входы — одинаковый ключ: %q vs %q", k1, k2)
	}

	withRings := &GenerateOptions{Radius: 20, HasRings: boolPtr(true)}
	noRings := &GenerateOptions{Radius: 20, HasRings: boolPtr(false)}
	unspecified := &GenerateOptions{Radius: 20}
	if generateKey(7, withRings) == generateKey(7, noRings) {
		t.Fatalf("явные rings=true и rings=false не должны совпадать")
	}
	if generateKey(7, unspecified) == generateKey(7, withRings) {
		t.Fatalf("незаданные rings должны отличаться от явных")
	}
}

func boolPtr(b bool) *bool {
	return &b
}