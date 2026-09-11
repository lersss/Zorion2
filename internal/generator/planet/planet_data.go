// internal/generator/planet/planet_data.go
package planet

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"zorion/internal/models"
	"zorion/internal/repository"
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
	ecoRepo   *repository.EconomyRepository
	usedNames map[string]bool
}

// NewGenerator — создаёт генератор. Если seed = 0 — берётся time.Now().
func NewGenerator(db *sql.DB, seed int64) *Generator {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &Generator{
		db:        db,
		rng:       rand.New(rand.NewSource(seed)),
		ecoRepo:   repository.NewEconomyRepository(db),
		usedNames: make(map[string]bool),
	}
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

	systemAge := determineSystemAge(spectralClass, g.rng)

	tx, err := g.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	batch := newBatchBuffers(planetCount)

	for i := 0; i < planetCount; i++ {
		orbitIndex := i + 1
		planet := g.generatePlanet(worldID, worldName, orbitIndex, spectralClass, systemAge)
		batch.addPlanet(planet)

		if err := g.collectEconomy(
			planet.ID, planet.Data, spectralClass,
			&batch.settlementRows, &batch.factoryRows, &batch.goodsRows,
		); err != nil {
			return 0, err
		}
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
	planetCount := g.determinePlanetCount(w.SpectralClass)
	if planetCount == 0 {
		return 0
	}

	systemAge := determineSystemAge(w.SpectralClass, g.rng)

	for i := 0; i < planetCount; i++ {
		orbitIndex := i + 1
		planet := g.generatePlanet(w.ID, w.Name, orbitIndex, w.SpectralClass, systemAge)
		buf.addPlanet(planet)

		if err := g.collectEconomy(
			planet.ID, planet.Data, w.SpectralClass,
			&buf.settlementRows, &buf.factoryRows, &buf.goodsRows,
		); err != nil {
			// Не валим весь батч из-за одной планеты.
			continue
		}
	}

	return planetCount
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

// ==================== БАТЧ-БУФЕР ====================

type batchBuffers struct {
	planetRows     []interface{}
	settlementRows []interface{}
	factoryRows    []interface{}
	goodsRows      []interface{}
}

func newBatchBuffers(planetCount int) *batchBuffers {
	if planetCount <= 0 {
		planetCount = 100
	}
	return &batchBuffers{
		planetRows:     make([]interface{}, 0, planetCount),
		settlementRows: make([]interface{}, 0, planetCount),
		factoryRows:    make([]interface{}, 0, planetCount*2),
		goodsRows:      make([]interface{}, 0, planetCount*4),
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
	b.settlementRows = b.settlementRows[:0]
	b.factoryRows = b.factoryRows[:0]
	b.goodsRows = b.goodsRows[:0]
}

// ==================== ФЛАШ В БД ====================

// flushBatch — вставляет всё содержимое буфера через pq.CopyIn.
// Сами CopyIn-функции — в planet_data_batch.go.
func (g *Generator) flushBatch(tx *sql.Tx, b *batchBuffers) error {
	if err := g.copyInPlanets(tx, flatten(b.planetRows)); err != nil {
		return fmt.Errorf("copy planets: %w", err)
	}
	if err := g.copyInSettlements(tx, flatten(b.settlementRows)); err != nil {
		return fmt.Errorf("copy settlements: %w", err)
	}
	if err := g.copyInFactories(tx, flatten(b.factoryRows)); err != nil {
		return fmt.Errorf("copy factories: %w", err)
	}
	if err := g.copyInGoods(tx, flatten(b.goodsRows)); err != nil {
		return fmt.Errorf("copy goods: %w", err)
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
func attachResources(
	data map[string]interface{},
	planetID string,
	dominant string,
	subterrain map[string]float64,
	spectralClass string,
	rng *rand.Rand,
) []*models.PlanetResource {
	resources := resource.GenerateResources(planetID, dominant, subterrain, spectralClass, rng)
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

func uuidShort() string {
	return uuid.New().String()[:8]
}
