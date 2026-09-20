// internal/handlers/planet_image_handler.go
//
// Авторизованный эндпоинт картинки планеты (спека 2026-09-20 §3.2):
// GET /api/planet-image?planet_id=<UUID>&size=small|big (или radius).
// JWT обязателен (401 без токена); режим картинки (честная/заглушка) решает
// сервер (§3.3): admin/skycomposer → честная; своя система → честная; знание
// о планете (любая запись, включая протухшую) → честная; иначе заглушка.
// Кэш: in-memory (планета, режим, размер) + диск-кэш PNG для большой (§5.2).
package handlers

import (
	"bytes"
	"image/png"
	"log"
	"net/http"
	"strconv"
	"sync"

	"zorion/internal/auth"
	"zorion/internal/generator/planet"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// PlanetImageHandler — хендлер картинки планеты.
type PlanetImageHandler struct {
	planetRepo *repository.PlanetRepository
	userRepo   *repository.UserRepository
	knowledge  *repository.KnowledgeRepository
	gen        *planet.PlanetGenerator
	cacheDir   string
}

// NewPlanetImageHandler — конструктор. cacheDir — каталог диск-кэша большой
// картинки (env PLANET_IMAGE_CACHE_DIR, дефолт data/planet_images); пусто —
// диск-кэш отключён (тесты).
func NewPlanetImageHandler(
	planetRepo *repository.PlanetRepository,
	userRepo *repository.UserRepository,
	knowledge *repository.KnowledgeRepository,
	cacheDir string,
) *PlanetImageHandler {
	return &PlanetImageHandler{
		planetRepo: planetRepo,
		userRepo:   userRepo,
		knowledge:  knowledge,
		gen:        getPlanetImageGenerator(),
		cacheDir:   cacheDir,
	}
}

// ServeHTTP — GET /api/planet-image.
func (h *PlanetImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	planetID := r.URL.Query().Get("planet_id")
	if planetID == "" {
		writeJSONError(w, "planet_id обязателен", http.StatusBadRequest)
		return
	}
	size := parseImageSize(r)

	p, err := h.planetRepo.GetPlanetByID(planetID)
	if err != nil {
		log.Printf("❌ planet-image: планета %s: %v", planetID, err)
		writeJSONError(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if p == nil {
		writeJSONError(w, "Планета не найдена", http.StatusNotFound)
		return
	}

	mode := h.resolveMode(r, p)

	// Диск-кэш большой (спека §5.2): PNG 512 в {dir}/{sha256(planet_id|mode)}.png.
	if size == planet.ImageSizeBig && h.cacheDir != "" {
		if data, ok := diskCacheGet(h.cacheDir, planetID, mode); ok {
			writePNG(w, data)
			return
		}
	}

	in := planet.PlanetImageInput{
		PlanetID:     p.ID,
		Mode:         mode,
		Size:         size,
		Biomes:       p.Biomes,
		Type:         p.Type,
		IsGasGiant:   p.IsGasGiant,
		Temperature:  p.Temperature,
		WaterPercent: p.WaterPercent,
		Habitable:    p.Habitable,
		Life:         p.Life,
	}
	// Атмосферные поля — только режим full (спека §3.1, контракт C):
	// состав/давление/τ_IR/масштабная высота из atmosphere_data.
	if mode == planet.ImageModeFull {
		in.Composition, in.PressureAtm, in.TauIR, in.ScaleHeightKm = atmosphereInputs(p.AtmosphereData)
	}
	img, err := h.gen.GeneratePlanetImage(in)
	if err != nil {
		log.Printf("❌ planet-image: генерация %s: %v", planetID, err)
		writeJSONError(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Printf("❌ planet-image: encode %s: %v", planetID, err)
		writeJSONError(w, "Внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if size == planet.ImageSizeBig && h.cacheDir != "" {
		diskCachePut(h.cacheDir, planetID, mode, buf.Bytes())
	}
	writePNG(w, buf.Bytes())
}

// parseImageSize — типоразмер из запроса (спека §3.2): size=small|big —
// явный выбор (radius игнорируется); иначе radius: ≤ 40 → малая, ≥ 128 →
// большая, промежуточные — малая (канвас 256).
func parseImageSize(r *http.Request) planet.ImageSize {
	switch r.URL.Query().Get("size") {
	case "big":
		return planet.ImageSizeBig
	case "small":
		return planet.ImageSizeSmall
	}
	if radiusStr := r.URL.Query().Get("radius"); radiusStr != "" {
		if radius, err := strconv.Atoi(radiusStr); err == nil && radius >= 128 {
			return planet.ImageSizeBig
		}
	}
	return planet.ImageSizeSmall
}

// resolveMode — режим картинки (спека §3.3, порядок): роль admin/skycomposer →
// full (И7); своя система (current_world_id == world_id) → full (вся своя
// система, решение гейта §12.5-A); знание о планете (любая запись, включая
// протухшую, С3) → honest (знание атмосферу НЕ содержит — производная, K1);
// иначе заглушка.
func (h *PlanetImageHandler) resolveMode(r *http.Request, p *models.Planet) planet.ImageMode {
	role, _ := r.Context().Value(auth.RoleKey).(string)
	if role == string(auth.RoleAdmin) || role == string(auth.RoleSkycomposer) {
		return planet.ImageModeFull
	}
	userID, _ := r.Context().Value(auth.UserIDKey).(string)
	user, err := h.userRepo.GetByID(userID)
	if err == nil && user != nil && user.CurrentWorldID != nil && *user.CurrentWorldID == p.WorldID {
		return planet.ImageModeFull
	}
	k, err := h.knowledge.GetKnowledge(userID, p.ID)
	if err == nil && k != nil {
		return planet.ImageModeHonest
	}
	return planet.ImageModeStub
}

// atmosphereInputs — атмосферные поля из atmosphere_data (спека §3.1, контракт
// C): composition (газ → %, сумма 100), pressure_atm, tau_ir, scale_height_km.
// Ключи JSON — из atmosphereDataToJSON (planet_data_generate.go). Отсутствие
// ключей → нули (фолбэки §7 в генераторе).
func atmosphereInputs(ad map[string]interface{}) (map[string]float64, float64, float64, float64) {
	if ad == nil {
		return nil, 0, 0, 0
	}
	comp := map[string]float64{}
	if cm, ok := ad["composition"].(map[string]interface{}); ok {
		for gas, v := range cm {
			if f, ok := v.(float64); ok {
				comp[gas] = f
			}
		}
	}
	P, _ := ad["pressure_atm"].(float64)
	tau, _ := ad["tau_ir"].(float64)
	sh, _ := ad["scale_height_km"].(float64)
	return comp, P, tau, sh
}

// writePNG — ответ PNG.
func writePNG(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Write(data)
}

var (
	planetImageGenOnce sync.Once
	planetImageGen     *planet.PlanetGenerator
)

// getPlanetImageGenerator — синглтон генератора картинок (in-memory кэш
// 1000 FIFO, спека §5.2). Без климат-файла: GeneratePlanetImage не использует
// climates (входы — видимые параметры, §3.1).
func getPlanetImageGenerator() *planet.PlanetGenerator {
	planetImageGenOnce.Do(func() {
		planetImageGen = planet.NewImageGenerator(
			planet.WithCanvasSize(256),
			planet.WithCacheEnabled(true),
			planet.WithMaxCacheSize(1000),
		)
	})
	return planetImageGen
}