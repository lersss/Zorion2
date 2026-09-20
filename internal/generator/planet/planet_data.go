// internal/generator/planet/planet_data.go
package planet

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"zorion/internal/models"
	"zorion/internal/regionprofile"
	"zorion/internal/resource"
)

// PlanetData — одна планета перед вставкой в БД
type PlanetData struct {
	ID         string
	WorldID    string
	Name       string
	OrbitIndex int
	Data       []byte
	// Resources — сгенерированные ресурсы планеты.
	// Хранятся только в памяти (сводка — в JSON data["resources"]);
	// таблица planet_resources удалена миграцией 000016.
	Resources []*models.PlanetResource
}

// Generator — генератор планет для мира
type Generator struct {
	db        *sql.DB
	rng       *rand.Rand
	usedNames map[string]bool
	means     PlanetMeans // среднее число планет по типу звезды (99.2.4 §5.2)

	// profile/profileIntensity — профиль региона текущего мира (59a, спека
	// §8): задаётся в generateWorldWithCountIntoBuffer, читается хелперами
	// (число планет, шанс гиганта, веса полосы, ресурсы). Генерация
	// однопоточная (один генератор — одна горутина) — поле безопасно.
	profile          *regionprofile.Profile
	profileIntensity regionprofile.Intensity

	// raceID — доминантная раса региона текущего мира (99.2.22 §2.1):
	// задаётся там же, где profile (из regions.race_id через
	// NearestRegionIndex), читается подкруткой (race_tuning.go). Пусто —
	// фоновый регион/легаси-вселенная без рас (подкрутки нет).
	raceID string
	// raceSoftness — мягкость подкрутки s ∈ [0, 1] (99.2.22 §4): слой 1
	// непрерывный (число планет), слой 2 вероятностный (физические ручки).
	// Дефолт 0.5 (админка, generation_config).
	raceSoftness float64
	// racePlanetCountMult — множитель числа планет в кластерах рас (99.2.22
	// §3.3 ручка 6): mean × mult перед потолком 8. Дефолт 1.1, диапазон
	// 0.7–1.3 (админка, generation_config).
	racePlanetCountMult float64
}

// NewGenerator — создаёт генератор. Если seed = 0 — берётся time.Now().
func NewGenerator(db *sql.DB, seed int64) *Generator {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &Generator{
		db:        db,
		rng:       rand.New(rand.NewSource(seed)),
		usedNames: make(map[string]bool),
		means:     DefaultPlanetMeans(),
	}
}

// SetMeans — задаёт средние числа планет (конфиг админки, 99.2.3 §4.3).
func (g *Generator) SetMeans(m PlanetMeans) {
	g.means = m
}

// SetRaceTuning — параметры подкрутки под расу-дома (99.2.22 §4.3, админка,
// generation_config): мягкость s ∈ [0, 1] и множитель числа планет в
// кластерах рас (0.7–1.3). Читаются и звёздным, и планетным джобами.
func (g *Generator) SetRaceTuning(softness, planetCountMult float64) {
	g.raceSoftness = softness
	g.racePlanetCountMult = planetCountMult
}

// ==================== СТАРАЯ ФУНКЦИЯ ====================

// GeneratePlanetsForWorld — генерирует все планеты одного мира и сохраняет их в БД.
// Одна транзакция на мир. Используется, если нужно сгенерировать планеты для
// одного конкретного мира.
//
// Для массовой генерации (100k миров) — использовать GeneratePlanetsForWorlds,
// там батчи по многим мирам в одной транзакции.
func (g *Generator) GeneratePlanetsForWorld(worldID, worldName, spectralClass string, temperature int) (int, error) {
	planetCount := g.determinePlanetCount(spectralClass)
	if planetCount == 0 {
		return 0, nil
	}

	sp := stellarParamsFromClass(spectralClass, temperature, g.rng)

	tx, err := g.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	batch := newBatchBuffers(planetCount)

	for i := 0; i < planetCount; i++ {
		orbitIndex := i + 1
		planet := g.generatePlanet(worldID, worldName, orbitIndex, sp)
		batch.addPlanet(planet)
	}

	if err := g.flushBatch(tx, batch); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return planetCount, nil
}

// ==================== НОВАЯ ФУНКЦИЯ (батч по многим мирам) ====================

// WorldInfo — минимальные данные мира, нужные для генерации планет.
type WorldInfo struct {
	ID            string
	Name          string
	SpectralClass string
	Temperature   int
	// Экзотические типы (99.2.4 §2): StarType — тип объекта (star/white_dwarf/
	// neutron/black_hole/protostar), SystemType — тип системы (single/binary/
	// multiple), Mods — модификаторы (фаза, переменность, подтипы, двойные).
	StarType   string
	SystemType string
	Mods       *models.StellarMods
	// Age — возраст системы в млрд лет (41a §4.2): планеты экзотики наследуют
	// его в data["system_age"]; старые миры (nil) — фолбэк-ролл генератора.
	Age *float64
	// StellarMass — масса звезды в M☉ (29a §4м): вход каскада (Кеплер III,
	// приливный захват, 99.2.20 §3.1); старые миры (nil) — фолбэк серединой
	// диапазона класса.
	StellarMass *float64
	// Profile — профиль региона мира (59a §10): привязка по ближайшему
	// центру региона (NearestRegionIndex); nil — фоновый регион.
	Profile *regionprofile.Profile
	// ProfileIntensity — интенсивность профиля 0/1/2: слабая/средняя/сильная.
	ProfileIntensity regionprofile.Intensity
	// RaceID — доминантная раса региона мира (99.2.22 §2.1): из
	// regions.race_id через ту же NearestRegionIndex, что и Profile
	// (консистентно с генератором поселений рас); пусто — фоновый
	// регион/легаси-вселенная без рас (подкрутки нет).
	RaceID string
}

// GeneratePlanetsForWorlds — генерирует планеты для списка миров.
// Одна транзакция на batchSize миров, вставка через pq.CopyIn.
//
// Раньше хендлер вызывал GeneratePlanetsForWorld в цикле — по одной транзакции
// на мир (100 000 BEGIN/COMMIT на 100k миров). Теперь — одна транзакция
// на 500 миров + COPY FROM STDIN. Ожидаемое ускорение: 10–50x.
//
// progressFn вызывается после каждого обработанного мира (для статус-бара).
// Может быть nil.
func (g *Generator) GeneratePlanetsForWorlds(
	worlds []WorldInfo,
	batchSize int,
	progressFn func(processed int),
) (int, error) {
	if batchSize <= 0 {
		batchSize = 500
	}
	if len(worlds) == 0 {
		return 0, nil
	}

	totalPlanets := 0
	processed := 0

	// Буфер накапливается между мирами и флашится раз в batchSize миров.
	buf := newBatchBuffers(batchSize * 8)

	for i, w := range worlds {
		planetCount := g.generateWorldIntoBuffer(w, buf)
		totalPlanets += planetCount

		processed++
		if progressFn != nil {
			progressFn(processed)
		}

		// Флаш раз в batchSize миров.
		if (i+1)%batchSize == 0 {
			if err := g.flushAndCommit(buf); err != nil {
				return totalPlanets, err
			}
		}
	}

	// Финальный флаш — остаток.
	if err := g.flushAndCommit(buf); err != nil {
		return totalPlanets, err
	}

	return totalPlanets, nil
}

// generateWorldIntoBuffer — генерирует планеты одного мира и складывает в буфер.
// Возвращает число сгенерированных планет.
func (g *Generator) generateWorldIntoBuffer(w WorldInfo, buf *batchBuffers) int {
	// Профиль региона мира (59a §10) выставляется ДО planetCountFor:
	// иначе счёт планет берёт профиль ПРЕДЫДУЩЕГО мира (stale — баг 59a,
	// ревью гейта 2). Остальные хуки (гиганты, веса полосы, ресурсы)
	// выставляются в generateWorldWithCountIntoBuffer.
	g.profile = w.Profile
	g.profileIntensity = w.ProfileIntensity
	g.raceID = w.RaceID
	return g.generateWorldWithCountIntoBuffer(w, g.planetCountFor(w), buf)
}

// generateWorldWithCountIntoBuffer — планеты мира с явным счётом (для
// пересчёта планет, 99.2.3 §5). Экзотика — ветка generateExoticPlanet,
// тесные двойные (close, с разделением) — P-ветка generateCircumbinaryPlanet
// (35b §4.1, отклонение от 99.2.4 §5.3), остальные обычные звёзды — общий
// путь generatePlanet. Возвращает число фактически созданных планет.
func (g *Generator) generateWorldWithCountIntoBuffer(w WorldInfo, count int, buf *batchBuffers) int {
	if count <= 0 {
		return 0
	}

	// Профиль региона мира (59a §10): применяется ко всем планетам мира.
	g.profile = w.Profile
	g.profileIntensity = w.ProfileIntensity
	// Раса-дома региона мира (99.2.22 §2.1): применяется ко всем планетам
	// мира (подкрутка входов каскада).
	g.raceID = w.RaceID

	generated := 0

	// P-ветка: тесная двойная с разделением (новые миры). Старые close-миры
	// без companion_sep_au идут общим путём (фолбэк §2.4).
	isCircumbinary := !isExoticObject(w.StarType) && w.Mods != nil &&
		w.Mods.BinaryType == "close" && w.Mods.CompanionSepAU != nil

	for i := 0; i < count; i++ {
		orbitIndex := i + 1
		if isExoticObject(w.StarType) {
			// Орбиты остатков: ЧД — далёкие 10–12, WD — выжившие 5–8 (§5.3).
			switch w.StarType {
			case "black_hole":
				orbitIndex = 10 + g.rng.Intn(3)
			case "white_dwarf":
				orbitIndex = 5 + g.rng.Intn(4)
			}
			planet := g.generateExoticPlanet(w, orbitIndex)
			if planet != nil {
				buf.addPlanet(planet)
				generated++
			}
			continue
		}
		if isCircumbinary {
			planet := g.generateCircumbinaryPlanet(w)
			if planet != nil {
				buf.addPlanet(planet)
				generated++
			}
			continue
		}
		planet := g.generatePlanet(w.ID, w.Name, orbitIndex, stellarParamsFromWorld(w, g.rng))
		buf.addPlanet(planet)
		generated++
	}

	return generated
}

// isExoticObject — star_type ≠ star (остатки и протозвезда).
func isExoticObject(starType string) bool {
	switch starType {
	case "star", "":
		return false
	}
	return true
}

// flushAndCommit — флашит буфер в БД одной транзакцией.
func (g *Generator) flushAndCommit(buf *batchBuffers) error {
	if buf.isEmpty() {
		return nil
	}
	tx, err := g.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := g.flushBatch(tx, buf); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	buf.reset()
	return nil
}

// RegeneratePlanetsForWorlds — ручной пересчёт планет (99.2.3 §5): для каждого
// мира число планет — равномерный счёт из [minCount, maxCount]. Средние из
// конфига НЕ применяются (инструмент-«лекарство», не второй генератор).
// Обычные звёзды — общий путь generatePlanet, экзотика — generateExoticPlanet
// (99.2.4 §5.3). Удаление старых планет — обязанность вызывающего.
func (g *Generator) RegeneratePlanetsForWorlds(
	worlds []WorldInfo,
	minCount, maxCount int,
	batchSize int,
	progressFn func(processed int),
) (int, error) {
	if batchSize <= 0 {
		batchSize = 500
	}
	if len(worlds) == 0 {
		return 0, nil
	}

	totalPlanets := 0

	// Буфер накапливается между мирами и флашится раз в batchSize миров.
	buf := newBatchBuffers(batchSize * 8)

	for i, w := range worlds {
		count := minCount
		if maxCount > minCount {
			count = minCount + g.rng.Intn(maxCount-minCount+1)
		}
		totalPlanets += g.generateWorldWithCountIntoBuffer(w, count, buf)

		if progressFn != nil {
			progressFn(i + 1)
		}
		if (i+1)%batchSize == 0 {
			if err := g.flushAndCommit(buf); err != nil {
				return totalPlanets, err
			}
		}
	}

	if err := g.flushAndCommit(buf); err != nil {
		return totalPlanets, err
	}
	return totalPlanets, nil
}

// ==================== БАТЧ-БУФЕР ====================

type batchBuffers struct {
	planetRows []interface{}
}

func newBatchBuffers(planetCount int) *batchBuffers {
	if planetCount <= 0 {
		planetCount = 100
	}
	return &batchBuffers{
		planetRows: make([]interface{}, 0, planetCount),
	}
}

// addPlanet — складывает планету в буфер.
//
// ВАЖНО: p.Data — это []byte (JSON). При INSERT ... VALUES lib/pq передавал
// его как текст, и Postgres парсил в json. При COPY драйвер не знает тип
// колонки и отправляет []byte как bytea — Postgres ругается
// "invalid input syntax for type json". Поэтому явно приводим к string.
func (b *batchBuffers) addPlanet(p *PlanetData) {
	now := time.Now()
	b.planetRows = append(b.planetRows, []interface{}{
		p.ID,
		p.WorldID,
		p.Name,
		p.OrbitIndex,
		string(p.Data), // []byte → string, чтобы pq.CopyIn отправил как text
		now,
		now,
	})
}

func (b *batchBuffers) isEmpty() bool {
	return len(b.planetRows) == 0
}

// reset — очищает буферы, сохраняя выделенную память.
func (b *batchBuffers) reset() {
	b.planetRows = b.planetRows[:0]
}

// ==================== ФЛАШ В БД ====================

// flushBatch — вставляет всё содержимое буфера через pq.CopyIn.
// Сами CopyIn-функции — в planet_data_batch.go.
func (g *Generator) flushBatch(tx *sql.Tx, b *batchBuffers) error {
	if err := g.copyInPlanets(tx, flatten(b.planetRows)); err != nil {
		return fmt.Errorf("copy planets: %w", err)
	}
	return nil
}

// flatten — превращает [][]interface{} в плоский []interface{}.
// Нужно для передачи в copyInRows, где данные идут одним потоком.
func flatten(rows []interface{}) []interface{} {
	total := 0
	for _, r := range rows {
		if slice, ok := r.([]interface{}); ok {
			total += len(slice)
		}
	}
	out := make([]interface{}, 0, total)
	for _, r := range rows {
		if slice, ok := r.([]interface{}); ok {
			out = append(out, slice...)
		}
	}
	return out
}

// ==================== РЕСУРСЫ ====================

// attachResources — генерирует ресурсы планеты и кладёт summary в data.
//
// На вход — уже собранный map планеты. Добавляет в него ключ "resources"
// (категория → богатство 0..1, английские коды) и возвращает ресурсы.
// resourceBias — веса категорий профиля региона (59a §8 P2); nil — равномерно.
func attachResources(
	data map[string]interface{},
	planetID string,
	dominant string,
	subterrain map[string]float64,
	spectralClass string,
	rng *rand.Rand,
	resourceBias map[string]float64,
) []*models.PlanetResource {
	resources := resource.GenerateResources(planetID, dominant, subterrain, spectralClass, rng, resourceBias)
	data["resources"] = resource.Summary(resources)
	return resources
}

// ==================== ХЕЛПЕРЫ ДЛЯ JSON ====================

func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

func getFloat(data map[string]interface{}, key string) float64 {
	if val, ok := data[key].(float64); ok {
		return val
	}
	return 0
}

func getBool(data map[string]interface{}, key string) bool {
	if val, ok := data[key].(bool); ok {
		return val
	}
	return false
}

func composeToJSON(c Composition) map[string]float64 {
	if c == nil {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}

// biomesToJSON — объекты биомов → JSON-массив {form, share} (99.2.28 §9.1).
func biomesToJSON(biomes []models.Biome) []map[string]interface{} {
	if len(biomes) == 0 {
		return []map[string]interface{}{}
	}
	out := make([]map[string]interface{}, 0, len(biomes))
	for _, b := range biomes {
		out = append(out, map[string]interface{}{"form": b.Form, "share": b.Share})
	}
	return out
}

// zonesToJSON — зоны недр → JSON-массив {type, share} (99.2.28 §9.1).
func zonesToJSON(zones []models.SubterrainZone) []map[string]interface{} {
	if len(zones) == 0 {
		return []map[string]interface{}{}
	}
	out := make([]map[string]interface{}, 0, len(zones))
	for _, z := range zones {
		out = append(out, map[string]interface{}{"type": z.Type, "share": z.Share})
	}
	return out
}

func uuidShort() string {
	return uuid.New().String()[:8]
}
