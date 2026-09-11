// internal/generator/planet/planet_data_satellites.go
package planet

import (
	"github.com/google/uuid"
	"zorion/internal/names"
)

// Satellite — спутник газового гиганта. Полноценная локация:
// имеет композицию поверхности, недр, температуру, может иметь жизнь.
type Satellite struct {
	ID                    string
	Name                  string
	OrbitIndex            int
	Size                  float64
	Mass                  float64
	Temperature           float64
	WaterPercent          float64
	Habitable             bool
	Life                  bool
	Atmosphere            string
	Biosphere             string
	SurfaceComposition    Composition
	SubterrainComposition Composition
	Description           string
}

// generateSatellites — генерирует N спутников для газового гиганта.
func (g *Generator) generateSatellites(
	count int,
	starName string,
	giantSize float64,
	giantTemp float64,
	spectralClass string,
) []*Satellite {
	satellites := make([]*Satellite, 0, count)
	usedNames := make(map[string]bool)

	for i := 0; i < count; i++ {
		sat := g.generateSatellite(
			i+1,
			starName,
			giantSize,
			giantTemp,
			spectralClass,
			usedNames,
		)
		satellites = append(satellites, sat)
	}
	return satellites
}

// generateSatellite — один спутник.
func (g *Generator) generateSatellite(
	orbitIndex int,
	starName string,
	giantSize float64,
	giantTemp float64,
	spectralClass string,
	usedNames map[string]bool,
) *Satellite {
	// 1. Размер и масса (0.1 – 2.0 земных)
	size := 0.1 + g.rng.Float64()*1.9
	mass := size * (0.4 + g.rng.Float64()*0.6)

	// 2. Температура: нагрев от гиганта + приливный + звезда
	temp := computeSatelliteTemp(giantTemp, orbitIndex)

	// 3. Тип спутника. Подавляющее большинство — сухие каменистые тела
	// (реголит, кратеры). Водные/ледяные (< 5%) — редкое исключение
	// (Европа, Титан), у которых хватает летучих.
	hydroRich := g.rng.Float64() < satelliteWaterProbability

	// 4. Вода — только у редких гидрных спутников
	waterPercent := 0.0
	if hydroRich {
		waterPercent = computeSatelliteWater(temp, g.rng)
	}

	// 5. Атмосфера
	atmosphere := pickSatelliteAtmosphere(temp, g.rng)

	// 6. Композиция поверхности и недр: у сухих спутников нет льда и воды
	var surfaceComp, subterrainComp Composition
	if hydroRich {
		surfaceComp = generateSatelliteSurface(temp, waterPercent, g.rng)
		subterrainComp = generateSatelliteSubterrain(temp, surfaceComp, g.rng)
	} else {
		surfaceComp = generateDrySatelliteSurface(temp, g.rng)
		subterrainComp = generateDrySatelliteSubterrain(temp, g.rng)
	}

	// До 20% спутников — «простые» тела: 1–2 типа поверхности и недр.
	if g.rng.Float64() < simpleSatelliteProbability {
		surfaceComp = simplifyComposition(surfaceComp, g.rng)
		subterrainComp = simplifyComposition(subterrainComp, g.rng)
	}

	// 7. Жизнь — только на редких гидрных спутниках
	life := false
	if hydroRich {
		life = determineSatelliteLife(temp, waterPercent, g.rng)
	}

	// 8. Обитаемость
	habitable := life && temp > 250 && temp < 350 && atmosphere != "ядовитая"

	// 9. Биосфера
	biosphere := "стерильная"
	if life {
		bios := []string{"микробная", "грибная", "растительная"}
		biosphere = bios[g.rng.Intn(len(bios))]
	}

	// 10. Имя
	name := names.GenerateSatelliteName(starName, g.rng, usedNames)
	if name == "" {
		name = "Спутник-" + uuidShort()
	}

	return &Satellite{
		ID:                    uuid.New().String(),
		Name:                  name,
		OrbitIndex:            orbitIndex,
		Size:                  size,
		Mass:                  mass,
		Temperature:           temp,
		WaterPercent:          waterPercent,
		Habitable:             habitable,
		Life:                  life,
		Atmosphere:            atmosphere,
		Biosphere:             biosphere,
		SurfaceComposition:    surfaceComp,
		SubterrainComposition: subterrainComp,
		Description:           satelliteDescription(temp, waterPercent, life),
	}
}