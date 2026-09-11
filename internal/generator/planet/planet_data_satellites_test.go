package planet

import "testing"

// Спутники газовых гигантов в подавляющем большинстве — сухие каменистые тела.
// Вода, ледяная кора, мёрзлые газы, гейзеры — редкое явление (≤5%).
// Фиксированный seed → тест детерминированный.
func TestSatellitesHydroRare(t *testing.T) {
	g := NewGenerator(nil, 42)

	const iterations = 20000
	total := 0
	water := 0
	iceCrust := 0

	for i := 0; i < iterations; i++ {
		giantTemp := 50 + g.rng.Float64()*100
		count := 3 + g.rng.Intn(8)
		for _, s := range g.generateSatellites(count, "Star", 10, giantTemp, "M") {
			total++
			if s.WaterPercent > 5 {
				water++
			}
			if s.SurfaceComposition.Has(SurfaceIceCrust) {
				iceCrust++
			}
		}
	}

	if total == 0 {
		t.Fatal("no satellites generated")
	}

	waterFrac := float64(water) / float64(total)
	iceFrac := float64(iceCrust) / float64(total)
	t.Logf("total=%d water>5=%.2f%% iceCrust=%.2f%%", total, waterFrac*100, iceFrac*100)

	if waterFrac >= 0.05 {
		t.Errorf("вода у %.2f%% спутников — больше 5%%", waterFrac*100)
	}
	if iceFrac >= 0.05 {
		t.Errorf("ледяная кора у %.2f%% спутников — больше 5%%", iceFrac*100)
	}
}

// Сухой спутник (без воды) не получает льда, мёрзлых газов и подземных льдов.
func TestSatelliteDryHasNoIce(t *testing.T) {
	g := NewGenerator(nil, 7)

	for i := 0; i < 5000; i++ {
		giantTemp := 50 + g.rng.Float64()*100
		count := 3 + g.rng.Intn(8)
		for _, s := range g.generateSatellites(count, "Star", 10, giantTemp, "M") {
			if s.WaterPercent > 5 {
				continue // редкий гидрный спутник — вне проверки
			}
			if s.SurfaceComposition.Has(SurfaceIceCrust) {
				t.Fatalf("сухой спутник %s получил ледяную кору", s.Name)
			}
			if s.SurfaceComposition.Has(SurfaceFrozenGases) {
				t.Fatalf("сухой спутник %s получил мёрзлые газы", s.Name)
			}
			if s.SubterrainComposition.Has(SubterrainGroundIce) {
				t.Fatalf("сухой спутник %s получил подземные льды", s.Name)
			}
		}
	}
}

// «Простые» спутники (1–2 типа) — не чаще 20% и всегда ≤2 форм на поверхность и недра.
func TestSatellitesSimpleRare(t *testing.T) {
	g := NewGenerator(nil, 11)

	const iterations = 10000
	total := 0
	simpleSurface := 0

	for i := 0; i < iterations; i++ {
		giantTemp := 50 + g.rng.Float64()*100
		count := 3 + g.rng.Intn(8)
		for _, s := range g.generateSatellites(count, "Star", 10, giantTemp, "M") {
			total++
			if len(s.SurfaceComposition) <= 2 && len(s.SubterrainComposition) <= 2 {
				simpleSurface++
			}
		}
	}

	if total == 0 {
		t.Fatal("no satellites generated")
	}

	frac := float64(simpleSurface) / float64(total)
	t.Logf("total=%d simple(<=2 форм)=%.2f%%", total, frac*100)

	// «До 20%» — доля простых тел не должна превышать порог даже с учётом
	// тех, кто и так имеет ≤2 форм без упрощения.
	if frac > simpleSatelliteProbability+0.02 {
		t.Errorf("простых спутников %.2f%% — больше допустимых 20%%", frac*100)
	}

	// Каждый «простой» спутник обязан иметь 1–2 формы на поверхность и недра.
	n := 0
	for i := 0; i < 5000; i++ {
		giantTemp := 50 + g.rng.Float64()*100
		count := 3 + g.rng.Intn(8)
		for _, s := range g.generateSatellites(count, "Star", 10, giantTemp, "M") {
			if len(s.SurfaceComposition) <= 2 && len(s.SubterrainComposition) <= 2 {
				if len(s.SurfaceComposition) < 1 || len(s.SubterrainComposition) < 1 {
					t.Fatalf("спутник %s без композиции", s.Name)
				}
				if sum := s.SurfaceComposition.Total(); sum < 99.5 || sum > 100.5 {
					t.Fatalf("спутник %s: сумма поверхности %.2f != 100", s.Name, sum)
				}
				if sum := s.SubterrainComposition.Total(); sum < 99.5 || sum > 100.5 {
					t.Fatalf("спутник %s: сумма недр %.2f != 100", s.Name, sum)
				}
				n++
			}
		}
	}
	t.Logf("validated simple satellites=%d", n)
}