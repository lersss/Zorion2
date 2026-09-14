// internal/repository/ship_catalog.go
// Каталог деталей кораблей в памяти (спека 99.2.15 §3.4): загрузка при
// старте + инвалидация/перезагрузка после генерации в админке. Паттерн
// mapcache: immutable snapshot + atomic.Pointer — чтение O(1) без мьютексов,
// БД не в хот-пате рендера (И7). Читатели, держащие старый snapshot,
// продолжают работать с ним после Replace.
package repository

import (
	"context"
	"database/sql"
	"log"
	"sort"
	"sync/atomic"

	"zorion/internal/generator/ship"
	"zorion/internal/models"
)

// CatalogSnapshot — неизменяемый срез каталога: детали + индексы + палитра
// и порядок слоёв (из config/ship_visual.json).
type CatalogSnapshot struct {
	parts      []models.ShipPart
	byID       map[string]models.ShipPart
	byCategory map[string][]models.ShipPart // отсортировано по id
	palette    []string
	layerOrder []string
}

// Parts — все детали каталога. Snapshot immutable: срез возвращается без
// копии, читатели не должны его мутировать.
func (s *CatalogSnapshot) Parts() []models.ShipPart {
	if s == nil {
		return nil
	}
	return s.parts
}

// Part — деталь по id; ok=false, если такой нет (фолбэк И4).
func (s *CatalogSnapshot) Part(id string) (models.ShipPart, bool) {
	if s == nil {
		return models.ShipPart{}, false
	}
	p, ok := s.byID[id]
	return p, ok
}

// PartsByCategory — детали категории (отсортированы по id).
func (s *CatalogSnapshot) PartsByCategory(cat string) []models.ShipPart {
	if s == nil {
		return nil
	}
	return s.byCategory[cat]
}

// DefaultPart — дефолтная деталь категории (первая в каталоге) для фолбэка
// И4: деталь из схемы отсутствует в каталоге → дефолтная деталь категории.
func (s *CatalogSnapshot) DefaultPart(cat string) (models.ShipPart, bool) {
	parts := s.PartsByCategory(cat)
	if len(parts) == 0 {
		return models.ShipPart{}, false
	}
	return parts[0], true
}

// Palette — палитра кораблей (12 цветов, read-only из конфига).
func (s *CatalogSnapshot) Palette() []string {
	if s == nil {
		return nil
	}
	return s.palette
}

// LayerOrder — порядок слоёв композиции (корпус → крылья → нос → двигатели → хвост).
func (s *CatalogSnapshot) LayerOrder() []string {
	if s == nil {
		return nil
	}
	return s.layerOrder
}

// AssemblyFromSeed — детерминированная сборка схемы от seed (спека §7.1,
// инвариант И3): SHA-256 seed → цвет из палитры + индекс детали по каждой
// категории. Чистая функция: ноль хранилища, дешево при 100k агентов (И7).
// Пустая категория в каталоге → пустой id детали (клиент рисует фолбэк И4).
func (s *CatalogSnapshot) AssemblyFromSeed(seed []byte) models.ShipVisual {
	if s == nil {
		return models.ShipVisual{}
	}
	byCat := make(map[string][]models.ShipPart, len(s.byCategory))
	for cat, parts := range s.byCategory {
		byCat[cat] = parts
	}
	return ship.AssemblyFromSeed(seed, s.palette, s.layerOrder, byCat)
}

// Empty — true, если каталог не загружен или пуст (фолбэк-примитив на клиенте).
func (s *CatalogSnapshot) Empty() bool {
	return s == nil || len(s.parts) == 0
}

// ==================== МЕНЕДЖЕР ====================

// ShipCatalog хранит текущий snapshot каталога и умеет пересобирать его из БД.
type ShipCatalog struct {
	ptr atomic.Pointer[CatalogSnapshot]
}

func NewShipCatalog(palette, layerOrder []string) *ShipCatalog {
	c := &ShipCatalog{}
	c.Replace(nil, palette, layerOrder)
	return c
}

// Snapshot — текущий каталог. Не пустой snapshot создаётся первой успешной
// загрузкой; до неё — пустой (фолбэк И4 на клиенте).
func (c *ShipCatalog) Snapshot() *CatalogSnapshot {
	return c.ptr.Load()
}

// Replace — атомарно заменяет каталог целиком (immutable snapshot).
// parts == nil — пустой каталог (не загружен).
func (c *ShipCatalog) Replace(parts []models.ShipPart, palette, layerOrder []string) {
	byID := make(map[string]models.ShipPart, len(parts))
	byCat := make(map[string][]models.ShipPart)
	for _, p := range parts {
		byID[p.ID] = p
		byCat[p.Category] = append(byCat[p.Category], p)
	}
	for cat := range byCat {
		sort.Slice(byCat[cat], func(i, j int) bool { return byCat[cat][i].ID < byCat[cat][j].ID })
	}
	c.ptr.Store(&CatalogSnapshot{
		parts:      parts,
		byID:       byID,
		byCategory: byCat,
		palette:    palette,
		layerOrder: layerOrder,
	})
}

// Load — загрузка всех деталей из БД и атомарная замена snapshot.
// При ошибке старый snapshot остаётся в силе.
func (c *ShipCatalog) Load(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT `+shipPartColumns+` FROM ship_parts ORDER BY category, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	parts, err := scanShipParts(rows)
	if err != nil {
		return err
	}
	cur := c.Snapshot()
	c.Replace(parts, cur.palette, cur.layerOrder)
	log.Printf("🚀 Каталог деталей кораблей загружен: %d деталей", len(parts))
	return nil
}