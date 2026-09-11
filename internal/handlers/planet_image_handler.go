// internal/handlers/planet_image_handler.go
package handlers

import (
	"image/png"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"zorion/internal/generator/planet"
)

func PlanetImageHandler(w http.ResponseWriter, r *http.Request) {
	seedStr := r.URL.Query().Get("seed")
	starType := r.URL.Query().Get("starType")
	climateID := r.URL.Query().Get("climateId")
	radiusStr := r.URL.Query().Get("radius")
	if radiusStr == "" {
		radiusStr = "20"
	}
	radius, err := strconv.Atoi(radiusStr)
	if err != nil || radius < 1 || radius > 80 {
		radius = 20
	}
	var seed int64 = 0
	if seedStr != "" {
		seed, err = strconv.ParseInt(seedStr, 10, 64)
		if err != nil {
			seed = 0
		}
	}
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	gen, err := getPlanetGenerator()
	if err != nil {
		log.Printf("❌ Failed to get planet generator: %v", err)
		http.Error(w, "Generator not available", http.StatusInternalServerError)
		return
	}

	opts := []func(*planet.GenerateOptions){
		planet.WithRadius(radius),
		planet.WithSeed(seed),
	}
	if starType != "" {
		opts = append(opts, planet.WithStarType(starType))
	}
	if climateID != "" {
		opts = append(opts, planet.WithClimateID(climateID))
	}

	cachedPlanet, err := gen.GeneratePlanet(radius, opts...)
	if err != nil {
		log.Printf("❌ Failed to generate planet: %v", err)
		http.Error(w, "Failed to generate planet", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, cachedPlanet.Image); err != nil {
		log.Printf("❌ Failed to encode image: %v", err)
		http.Error(w, "Failed to encode image", http.StatusInternalServerError)
		return
	}
}

var (
	planetGenOnce sync.Once
	planetGen     *planet.PlanetGenerator
	planetGenErr  error
)

func getPlanetGenerator() (*planet.PlanetGenerator, error) {
	planetGenOnce.Do(func() {
		const climateFile = "config/planet_archetypes.json"
		pg, err := planet.NewPlanetGenerator(climateFile,
			planet.WithCanvasSize(256), // <-- увеличена базовая текстура (детализация при зуме)
			planet.WithCacheEnabled(true),
			planet.WithMaxCacheSize(1000),
		)
		if err != nil {
			log.Printf("⚠️ Failed to load planet generator: %v", err)
			planetGenErr = err
			return
		}
		planetGen = pg
	})
	return planetGen, planetGenErr
}