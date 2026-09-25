// internal/handlers/surface_handlers.go
//
// Высадка на планету и мини-игра «прогулка по поверхности» (спека 2026-09-21
// §6): POST /api/surface/land — переход orbit(планета) → surface(планета, биом)
// с жребием биома по долям planets.data.biomes (§5.1) и пакетом прогулки (§7.1);
// POST /api/surface/leave — мгновенный возврат surface → orbit(планета) и тот
// же переход для смерти (инвариант 9). Здоровье — серверно-авторитетное
// (§8.7): hp/landed_at в users.current_position, клиент урон НЕ присылает.
// Новых таблиц/миграций нет.
package handlers

import (
	"encoding/json"
	"hash/crc32"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"

	"zorion/internal/auth"
	"zorion/internal/generator/planet"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/travel"
)

// SurfaceHandlers — ручки высадки/возврата.
type SurfaceHandlers struct {
	userRepo     *repository.UserRepository
	worldRepo    *repository.WorldRepository
	planetRepo   *repository.PlanetRepository
	intraManager *travel.IntrasystemManager
}

func NewSurfaceHandlers(
	userRepo *repository.UserRepository,
	worldRepo *repository.WorldRepository,
	planetRepo *repository.PlanetRepository,
	intraManager *travel.IntrasystemManager,
) *SurfaceHandlers {
	return &SurfaceHandlers{
		userRepo:     userRepo,
		worldRepo:    worldRepo,
		planetRepo:   planetRepo,
		intraManager: intraManager,
	}
}

// ==================== ПАКЕТ ПРОГУЛКИ (§7.1) ====================

// SurfaceSuit — базовый скафандр (пакет §7.1): максимум HP и вилки комфорта.
type SurfaceSuit struct {
	HPMax              float64    `json:"hp_max"`
	TempComfortK       [2]float64 `json:"temp_comfort_k"`
	PressureComfortAtm [2]float64 `json:"pressure_comfort_atm"`
}

// SurfaceStar — светило системы для параллакс-неба.
type SurfaceStar struct {
	SpectralClass string `json:"spectral_class"`
	Color         string `json:"color"`
}

// SurfaceSkyBody — тело системы в небе прогулки (планета/спутник/компаньон).
type SurfaceSkyBody struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"` // planet|satellite|companion
	SizeHint float64 `json:"size_hint"`
	Color    string  `json:"color"`
	Height   float64 `json:"height"` // 0..1: высота на параллакс-слое
	// PlanetID — id планеты для авторизованной картинки (идея 2026-09-22 §8.4).
	// Только у тел kind=planet; у спутников/компаньонов картинки нет — пусто
	// (omitempty), клиент рисует фолбэк-диск.
	PlanetID string `json:"planet_id,omitempty"`
}

// SurfaceSky — небо прогулки: светило + тела системы (В2, только из пакета).
type SurfaceSky struct {
	Star   SurfaceStar      `json:"star"`
	Bodies []SurfaceSkyBody `json:"bodies"`
}

// SurfacePackage — ответ `land` (пакет прогулки, §7.1): единственный вход
// клиентского генератора.
type SurfacePackage struct {
	PlanetID   string `json:"planet_id"`
	PlanetName string `json:"planet_name"`
	Role       string `json:"role"` // роль самого игрока (идея 2026-09-21): клиент не делает лишний GET /me
	// Корабль игрока (ЧК-ship, идея 2026-09-23 §5): спрайт у точки спавна —
	// визуальный якорь «я только что сел здесь». Иконка уже резолвлена
	// (models.ResolveShipIcon: legacy/битая → дефолт), цвет NULL = «Оригинал»;
	// pose (angle/flip) — из того же реестра, что и остальной визуал корабля.
	ShipIcon  string  `json:"ship_icon"`
	ShipColor *string `json:"ship_color"`
	ShipAngle float64 `json:"ship_angle"`
	ShipFlip  bool    `json:"ship_flip"`
	// ShipScale — размер корабля в ростах человека (решение создателя
	// 2026-09-25): клиент умножает на рост игрока. Значение — из реестра
	// кораблей (models.ShipScaleHuman, дефолт 12); поле аддитивное, старый
	// клиент его не читает.
	ShipScale        float64 `json:"ship_scale"`
	Biome            string  `json:"biome"`
	BiomeName        string  `json:"biome_name"`
	BiomeShare       float64 `json:"biome_share"`
	BiomeCategory    string  `json:"biome_category"`
	BiomeColor       string  `json:"biome_color"`
	BiomeDescription string  `json:"biome_description,omitempty"`
	// Рецепт вида биома (спека 2026-09-23 §2.6): аддитивные поля. biome_view —
	// резолвленный рецепт (пусто → клиент по фолбэку FORMATIONS/LIFE_DENSITY);
	// view_source — "catalog"|"fallback"; view_version — версия схемы рецепта.
	BiomeView   map[string]any `json:"biome_view,omitempty"`
	ViewSource  string         `json:"view_source"`
	ViewVersion int            `json:"view_version"`
	// Слой жидкости (спека 2026-09-25 ЧК6 §3.5): аддитивное резолвленное поле
	// (medium/color/level/surface/fog/ice) + источник признака (explicit|category).
	// Пусто → жидкости нет. Клиент читает отсюда (не из справочника).
	Liquid        map[string]any `json:"liquid,omitempty"`
	LiquidSource  string         `json:"liquid_source"`
	Seed          uint32         `json:"seed"`
	Life          bool           `json:"life"`
	Gravity       float64        `json:"gravity"`
	Temperature   float64        `json:"temperature"`
	PressureAtm   float64        `json:"pressure_atm"`
	Radioactive   bool           `json:"radioactive"`
	Radioactivity float64        `json:"radioactivity"`
	Toxic         bool           `json:"toxic"`
	LiquidMedium  string         `json:"liquid_medium"`
	Suit          SurfaceSuit    `json:"suit"`
	HP            float64        `json:"hp"`
	LandedAt      string         `json:"landed_at"`
	Hazard        SurfaceHazard  `json:"hazard"`
	Sky           SurfaceSky     `json:"sky"`
}

// ==================== ЖРЕБИЙ БИОМА (§5.1) ====================

// walkBiomeValid — биомы, участвующие в жребии: share > 0 и form известен
// каталогу (99.2.28 §3.1/§16). Пусто → высадка недоступна.
func walkBiomeValid(p *models.Planet) []models.Biome {
	if p == nil {
		return nil
	}
	cat := planet.GetBiomeCatalog()
	var valid []models.Biome
	for _, b := range p.Biomes {
		if b.Share <= 0 {
			continue
		}
		if cat.BiomeByID(b.Form) == nil {
			continue
		}
		valid = append(valid, b)
	}
	return valid
}

// isAdminRole — роль с админским доступом (спека 99.2.14): admin/skycomposer.
// Админский выбор биома (идея 2026-09-21 §2 п.3) — только для них.
func isAdminRole(role models.Role) bool {
	return role == models.RoleAdmin || role == models.RoleSkycomposer
}

// walkBiomeValidForm — запрошенный админом биом валиден, если он у планеты и
// проходит walkBiomeValid (share > 0 и известен каталогу, §5.1). Иначе —
// 400 «Нет такого биома на планете» (идея 2026-09-21 §3 п.1).
func walkBiomeValidForm(p *models.Planet, form string) bool {
	if form == "" {
		return false
	}
	for _, b := range walkBiomeValid(p) {
		if b.Form == form {
			return true
		}
	}
	return false
}

// pickWalkBiomeAt — взвешенный жребий по долям: r ∈ [0, total). Чистая функция
// (тест распределения). valid гарантированно непуст, total > 0.
func pickWalkBiomeAt(valid []models.Biome, r float64) string {
	total := 0.0
	for _, b := range valid {
		total += b.Share
	}
	if total <= 0 {
		return ""
	}
	if r < 0 {
		r = 0
	}
	if r >= total {
		r = math.Nextafter(total, 0)
	}
	acc := 0.0
	for _, b := range valid {
		acc += b.Share
		if r < acc {
			return b.Form
		}
	}
	return valid[len(valid)-1].Form
}

// pickWalkBiome — жребий сервером (единый источник истины): локальный rand.New
// (AGENTS §0 — общий rand не потокобезопасен). Пусто → «нет данных».
func pickWalkBiome(p *models.Planet) string {
	valid := walkBiomeValid(p)
	if len(valid) == 0 {
		return ""
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	total := 0.0
	for _, b := range valid {
		total += b.Share
	}
	return pickWalkBiomeAt(valid, rng.Float64()*total)
}

// NormalizeSurfaceBiome — ИП-4' (§4.3, B6): biome из позиции, если он валиден
// (share > 0 в биомах планеты); иначе доминирующий биом (max share). Пусто —
// у планеты нет валидных биомов (вызывающий даёт фолбэк «орбита звезды»).
// Одна функция для normalizeMyPosition и идемпотентной ветки land.
func NormalizeSurfaceBiome(p *models.Planet, biome string) string {
	if p == nil {
		return ""
	}
	best := ""
	bestShare := 0.0
	for _, b := range p.Biomes {
		if b.Share <= 0 {
			continue
		}
		if b.Form == biome {
			return biome
		}
		if b.Share > bestShare {
			bestShare = b.Share
			best = b.Form
		}
	}
	return best
}

// ==================== ФИЗИКА ПЛАНЕТЫ (для пакета/HP) ====================

// planetPressureAtm — давление атмосферы (атм) из atmosphere_data.pressure_atm;
// нет данных → 0 (hazardPressure трактует как «нет данных», без урона).
func planetPressureAtm(p *models.Planet) float64 {
	if p == nil || p.AtmosphereData == nil {
		return 0
	}
	if v, ok := p.AtmosphereData["pressure_atm"].(float64); ok {
		return v
	}
	return 0
}

// planetComposition — состав атмосферы (газ → доля) из atmosphere_data.composition.
func planetComposition(p *models.Planet) map[string]float64 {
	comp := map[string]float64{}
	if p == nil || p.AtmosphereData == nil {
		return comp
	}
	raw, ok := p.AtmosphereData["composition"].(map[string]interface{})
	if !ok {
		return comp
	}
	for gas, v := range raw {
		if f, ok := v.(float64); ok {
			comp[gas] = f
		}
	}
	return comp
}

// planetRadioactivity — радиоактивность ядра 0–100 (03_planets.md §3.5).
func planetRadioactivity(p *models.Planet) float64 {
	if p == nil || p.Core == nil {
		return 0
	}
	return p.Core.Radioactivity
}

// biomeShare — доля биома в биомах планеты (0, если нет).
func biomeShare(p *models.Planet, form string) float64 {
	if p == nil {
		return 0
	}
	for _, b := range p.Biomes {
		if b.Form == form {
			return b.Share
		}
	}
	return 0
}

// surfaceHazardFor — профиль опасности планеты для биома (общий для land/leave/чтений).
// nil-планета (битый surface) → нулевой профиль (не падать).
func surfaceHazardFor(p *models.Planet, biome string) SurfaceHazard {
	if p == nil {
		return SurfaceHazard{}
	}
	cat := planet.GetBiomeCatalog()
	category := ""
	if b := cat.BiomeByID(biome); b != nil {
		category = b.Category
	}
	toxRatio := planet.ToxRatio(planetComposition(p), cat)
	return ComputeSurfaceHazard(p.Temperature, planetPressureAtm(p), planetRadioactivity(p), toxRatio, category)
}

// surfaceSeed — детерминированный seed мира: crc32(planet_id + "|" + biome).
func surfaceSeed(planetID, biome string) uint32 {
	return crc32.ChecksumIEEE([]byte(planetID + "|" + biome))
}

// ==================== ЦВЕТ ЗВЕЗДЫ (небо §7.1) ====================

// starColorByClass — та же карта, что CONFIG.map.starColors клиента
// (web/static/js/config.js): один контракт цвета светила.
var starColorByClass = map[string]string{
	"O": "#9bb0ff", "B": "#aac7ff", "A": "#f8f7ff", "F": "#fff4e8",
	"G": "#ffd700", "K": "#ffa500", "M": "#ff6348", "L": "#8b5a2b",
	"T": "#6b4c3b", "Y": "#4d3b2b",
	"black_hole": "#2a1a4a", "neutron": "#a0d8ef", "white_dwarf": "#f0f0f0",
	"protostar": "#ff7950",
}

// starColorHex — цвет светила: экзотический star_type приоритетнее спектра.
func starColorHex(starType, spectralClass string) string {
	if starType != "" && starType != "star" {
		if c, ok := starColorByClass[starType]; ok {
			return c
		}
	}
	if len(spectralClass) > 0 {
		if c, ok := starColorByClass[spectralClass[:1]]; ok {
			return c
		}
	}
	return "#8b5cf6"
}

// ==================== СБОРКА ПАКЕТА ====================

// planetSuit — базовый скафандр (константы §8.1).
func planetSuit() SurfaceSuit {
	return SurfaceSuit{
		HPMax:              SurfaceHPMax,
		TempComfortK:       [2]float64{SurfaceTempComfortMinK, SurfaceTempComfortMaxK},
		PressureComfortAtm: [2]float64{SurfacePressureMinAtm, SurfacePressureMaxAtm},
	}
}

// buildWalkPackage собирает пакет прогулки (§7.1): биом + справочник + seed +
// физика + признак жизни + профиль опасности + небо + корабль игрока. hp
// пересчитан от landed_at на этом чтении (§8.7). world/planets переданы
// вызывающим (одна выборка на высадку; O(числа планет системы)). Не падает при
// битом каталоге (И8).
func (h *SurfaceHandlers) buildWalkPackage(p *models.Planet, biome string, pos *models.CurrentPosition, now time.Time, world *models.World, planets []models.Planet, role string, user *models.User) SurfacePackage {
	cat := planet.GetBiomeCatalog()
	def := cat.BiomeByID(biome)
	name, category, description, liquid := "", "", "", ""
	if def != nil {
		name = def.Name
		category = def.Category
		description = def.Description
		liquid = def.LiquidMedium
	}
	hazard := surfaceHazardFor(p, biome)
	landedAt, _ := time.Parse(time.RFC3339, pos.LandedAt)
	hp := surfaceHPAt(landedAt, hazard.Total, now)
	// Корабль: резолвим иконку и берём её позу из реестра (ЧК-ship).
	shipIcon := models.ResolveShipIcon(user.ShipIcon)
	shipAngle, shipFlip := models.ShipOrientByFile(shipIcon)
	shipScale := models.ShipScaleHuman(shipIcon)
	// Рецепт вида (§2.6): резолв на сервере — клиент не читает справочник сам.
	biomeView, viewSource := cat.ResolveBiomeView(biome)
	// Слой жидкости (ЧК6 §3.5): резолв на сервере, приоритет explicit > category.
	// Вид уже резолвлен выше — второго резолва нет (`biomeView` nil при фолбэке).
	liquidView, liquidSource := planet.LiquidFromResolvedView(def, biomeView)

	return SurfacePackage{
		PlanetID:         p.ID,
		PlanetName:       p.Name,
		Role:             role,
		ShipIcon:         shipIcon,
		ShipColor:        user.ShipColor,
		ShipAngle:        shipAngle,
		ShipFlip:         shipFlip,
		ShipScale:        shipScale,
		Biome:            biome,
		BiomeName:        name,
		BiomeShare:       biomeShare(p, biome),
		BiomeCategory:    category,
		BiomeColor:       planet.SurfaceBiomeColorHex(biome, p.Temperature),
		BiomeDescription: description,
		BiomeView:        biomeView,
		ViewSource:       viewSource,
		ViewVersion:      planet.ViewSchemaVersion,
		Liquid:           liquidView,
		LiquidSource:     liquidSource,
		Seed:             surfaceSeed(p.ID, biome),
		Life:             p.Life,
		Gravity:          p.Gravity,
		Temperature:      p.Temperature,
		PressureAtm:      planetPressureAtm(p),
		Radioactive:      p.Radioactive,
		Radioactivity:    planetRadioactivity(p),
		Toxic:            planet.IsToxicAtmosphere(planetComposition(p), cat),
		LiquidMedium:     liquid,
		Suit:             planetSuit(),
		HP:               hp,
		LandedAt:         pos.LandedAt,
		Hazard:           hazard,
		Sky:              buildSurfaceSky(world, planets),
	}
}

// buildSurfaceSky — параллакс-небо (§7.1, В2): светило + тела системы. Считается
// один раз на высадку (O(числа планет системы)); другого источника у клиента нет.
func buildSurfaceSky(world *models.World, planets []models.Planet) SurfaceSky {
	sky := SurfaceSky{Star: SurfaceStar{Color: "#8b5cf6"}}
	if world != nil {
		sky.Star.SpectralClass = world.SpectralClass
		sky.Star.Color = starColorHex(world.StarType, world.SpectralClass)
	}

	var bodies []SurfaceSkyBody
	for _, p := range planets {
		bodies = append(bodies, SurfaceSkyBody{
			Name:     p.Name,
			Kind:     "planet",
			SizeHint: sizeHint(p.Size),
			Color:    planet.SurfaceBiomeColorHex(dominantBiomeForm(&p), p.Temperature),
			PlanetID: p.ID,
		})
		for _, s := range p.Satellites {
			bodies = append(bodies, SurfaceSkyBody{
				Name:     s.Name,
				Kind:     "satellite",
				SizeHint: sizeHint(s.Size),
				Color:    "#9aa4b0",
			})
		}
	}
	if world != nil && world.StellarMods != nil {
		if world.StellarMods.Companion != "" {
			bodies = append(bodies, SurfaceSkyBody{
				Name:     world.StellarMods.Companion,
				Kind:     "companion",
				SizeHint: 0.5,
				Color:    starColorHex(world.StellarMods.Companion, world.StellarMods.Companion),
			})
		}
		for _, ec := range world.StellarMods.ExtraCompanions {
			bodies = append(bodies, SurfaceSkyBody{
				Name:     ec.SpectralClass,
				Kind:     "companion",
				SizeHint: 0.4,
				Color:    starColorHex(ec.SpectralClass, ec.SpectralClass),
			})
		}
	}
	// Высота на параллакс-слое: детерминированные равномерные «полосы» 0.08..0.63.
	n := len(bodies)
	for i := range bodies {
		if n > 1 {
			bodies[i].Height = 0.08 + 0.55*float64(i)/float64(n-1)
		} else {
			bodies[i].Height = 0.3
		}
	}
	sky.Bodies = bodies
	return sky
}

// sizeHint — визуальный размер тела в небе (спека 2026-09-22 §4.4): Size
// (радиусы Земли) → 0.05..1. Степенная лестница h = size^0.6 / 4.26
// (4.26 = 11.2^0.6 — нормализатор, гигант → 1.0): гиганты различимы, мелкий
// камень не пропадает. Старый clamp(size/5,…) сливал все гиганты в 1.0.
func sizeHint(size float64) float64 {
	if size <= 0 {
		return 0.05
	}
	v := math.Pow(size, 0.6) / 4.26
	if v < 0.05 {
		return 0.05
	}
	if v > 1 {
		return 1
	}
	return v
}

// dominantBiomeForm — форма биома с максимальной долей (цвет неба).
func dominantBiomeForm(p *models.Planet) string {
	form := ""
	best := 0.0
	for _, b := range p.Biomes {
		if b.Share > best {
			best = b.Share
			form = b.Form
		}
	}
	return form
}

// ==================== LAND (§6.1) ====================

// SurfaceLandRequest — тело POST /api/surface/land. Biome — необязательный
// админский выбор биома (идея 2026-09-21 §3): пусто = жребий ∝ share (§5.1).
type SurfaceLandRequest struct {
	PlanetID string `json:"planet_id"`
	Biome    string `json:"biome,omitempty"`
}

// SurfaceLeaveResponse — ответ `leave` (§6.3): позиция орбиты, HP и причина.
type SurfaceLeaveResponse struct {
	Position *models.CurrentPosition `json:"position"`
	HP       float64                 `json:"hp"`
	Cause    string                  `json:"cause,omitempty"`
}

// Land — POST /api/surface/land. Валидации §6.1 строго по порядку:
// идемпотентность (шаг 2) до проверки «на орбите»; при status=surface на другой
// планете идемпотентность не срабатывает → 400 «Вы только с орбиты планеты».
func (h *SurfaceHandlers) Land(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	var req SurfaceLandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.PlanetID == "" {
		writeJSONError(w, "planet_id обязателен", http.StatusBadRequest)
		return
	}

	user, pos, dest, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}
	now := time.Now()

	// Роль для UI-гейта — из токена (её же проверяют ручки), фолбэк — БД
	// (идея 2026-09-23: смена роли после выдачи токена не должна рассинхронить
	// пакет прогулки и авторизацию).
	role := string(user.Role)
	if ctxRole, ok := r.Context().Value(auth.RoleKey).(string); ok && ctxRole != "" {
		role = ctxRole
	}

	// Админский выбор биома (идея 2026-09-21 §2 п.3): поле biome принимается
	// только от роли admin/skycomposer — явный отказ, не молчаливое
	// игнорирование. Роль — из токена (та же, что в пакете прогулки, идея
	// 2026-09-23): иначе клиент покажет выбор биома, а сервер откажет. Гейт
	// стоит ДО идемпотентности: своя роль — не состояние прогулки. Валидация
	// самого биома — ниже (шаг 7, после идемпотентности: повторный land отдаёт
	// сохранённый биом, присланный не проверяется).
	if req.Biome != "" && !isAdminRole(models.Role(role)) {
		writeJSONError(w, "Выбор биома доступен только администратору", http.StatusBadRequest)
		return
	}

	// 2. Идемпотентность (В4): уже на поверхности этой планеты → тот же
	// нормализованный биом, landed_at как хранится, hp пересчитан (§8.7).
	// Присланный biome игнорируется (рефреш не перебрасывает биом): сменить
	// биом можно только через leave (вызов корабля) и новую высадку.
	if pos != nil && pos.Status == "surface" && pos.ObjectID == req.PlanetID && user.CurrentWorldID != nil {
		planets, err := h.planetRepo.GetPlanetsLightByWorldID(*user.CurrentWorldID)
		if err == nil {
			if p := findPlanetByID(planets, req.PlanetID); p != nil {
				if biome := NormalizeSurfaceBiome(p, pos.Biome); biome != "" {
					world, _ := h.worldRepo.GetByID(p.WorldID)
					writeJSONStatus(w, http.StatusOK, h.buildWalkPackage(p, biome, pos, now, world, planets, role, user))
					return
				}
			}
		}
	}

	// 3. Игрок в системе.
	if user.CurrentWorldID == nil {
		writeJSONError(w, "Вы не в системе", http.StatusBadRequest)
		return
	}
	world, err := h.worldRepo.GetByID(*user.CurrentWorldID)
	if err != nil || world == nil {
		writeJSONError(w, "Вы не в системе", http.StatusBadRequest)
		return
	}
	worldID := *user.CurrentWorldID

	// 4. Нет активного внутрисистемного полёта (in_flight) → иначе 400.
	if (pos != nil && pos.Status == "in_flight") ||
		(h.intraManager != nil && h.intraManager.GetIntraFlight(userID) != nil) {
		writeJSONError(w, "Вы в полёте", http.StatusBadRequest)
		return
	}

	// 5. Позиция = орбита этой планеты.
	if pos == nil || pos.Status != "orbit" || pos.ObjectType != "planet" || pos.ObjectID != req.PlanetID {
		writeJSONError(w, "Вы только с орбиты планеты", http.StatusBadRequest)
		return
	}

	// 6. Планета принадлежит этой системе (одна выборка планет системы — она же
	// для пакета и неба: O(числа планет системы), без поселений).
	planets, err := h.planetRepo.GetPlanetsLightByWorldID(worldID)
	if err != nil {
		writeJSONError(w, "Не удалось загрузить систему", http.StatusInternalServerError)
		return
	}
	p := findPlanetByID(planets, req.PlanetID)
	if p == nil {
		writeJSONError(w, "Вы только с орбиты планеты", http.StatusBadRequest)
		return
	}

	// 6б. Класс без поверхности (газовый гигант, мини-нептун — спека
	// 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны §8.3): у тела только
	// оболочка, биомов/недр нет. Отказ осмысленный (а не «нет данных»),
	// симметрично клиенту, где пункт «Высадиться» disabled для обоих классов.
	if p.IsGasGiant || p.IsMiniNeptune {
		writeJSONError(w, "У планеты нет поверхности — высадка невозможна", http.StatusBadRequest)
		return
	}

	// 7. Биомы валидны (§5.1) → иначе «Нет данных о поверхности». Админский
	// выбор биома (идея 2026-09-21): пусто — существующий жребий (поведение не
	// меняется); задан — биом обязан быть у планеты и проходить walkBiomeValid
	// (роль проверена выше — до идемпотентности).
	biome := ""
	if req.Biome != "" {
		if !walkBiomeValidForm(p, req.Biome) {
			writeJSONError(w, "Нет такого биома на планете", http.StatusBadRequest)
			return
		}
		biome = req.Biome
	} else {
		biome = pickWalkBiome(p)
	}
	if biome == "" {
		writeJSONError(w, "Нет данных о поверхности", http.StatusBadRequest)
		return
	}

	// Диагностика дрейфа намерения (§6.4, В8): бизнес-логика не меняется.
	if dest != nil {
		log.Printf("⚠️ surface: land при pending_destination (user %s)", userID)
	}

	newPos := models.SurfacePosition(p.ID, biome, SurfaceHPMax, now)
	if err := h.userRepo.UpdatePosition(userID, newPos); err != nil {
		log.Printf("⚠️ surface: land update (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось высадиться", http.StatusInternalServerError)
		return
	}

	writeJSONStatus(w, http.StatusOK, h.buildWalkPackage(p, biome, newPos, now, world, planets, role, user))
}

// ==================== LEAVE (§6.3) ====================

// Leave — POST /api/surface/leave. Не на поверхности → 200 no-op (идемпотентность
// важнее строгости). Иначе hp = max(0, 100 − total·ΔT), текущая позиция →
// orbit(планета) (битая планета → орбита звезды, ИП-4'), ответ {position, hp, cause}.
func (h *SurfaceHandlers) Leave(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	user, pos, _, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}
	now := time.Now()
	if pos == nil || pos.Status != "surface" {
		writeJSONStatus(w, http.StatusOK, SurfaceLeaveResponse{Position: pos, HP: SurfaceHPMax})
		return
	}

	planetID := pos.ObjectID
	var p *models.Planet
	if planetID != "" && user.CurrentWorldID != nil {
		if planets, err := h.planetRepo.GetPlanetsLightByWorldID(*user.CurrentWorldID); err == nil {
			p = findPlanetByID(planets, planetID)
		}
	}
	biome := NormalizeSurfaceBiome(p, pos.Biome)
	hazard := surfaceHazardFor(p, biome)
	landedAt, _ := time.Parse(time.RFC3339, pos.LandedAt)
	hp := surfaceHPAt(landedAt, hazard.Total, now)

	// Битый surface (планета удалена) → фолбэк «орбита звезды» (ИП-4').
	newPos := models.OrbitPosition("planet", planetID)
	if user.CurrentWorldID == nil || p == nil || biome == "" {
		if user.CurrentWorldID != nil {
			newPos = models.StarOrbitPosition(*user.CurrentWorldID)
		} else {
			newPos = nil
		}
	}
	if err := h.userRepo.UpdatePosition(userID, newPos); err != nil {
		log.Printf("⚠️ surface: leave update (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось вернуться на орбиту", http.StatusInternalServerError)
		return
	}

	cause := ""
	if p != nil {
		cause = dominantHazardCause(hazard, p.Temperature)
	}
	writeJSONStatus(w, http.StatusOK, SurfaceLeaveResponse{Position: newPos, HP: hp, Cause: cause})
}
