// internal/mapcache/snapshot.go
//
// Снапшот карты: все миры вселенной в памяти Go.
// Вселенная статична между генерациями, поэтому вместо SQL-запроса
// на каждый pan карты миры грузятся один раз и кластеризуются в Go.
//
// Конкурентность: Snapshot immutable, заменяется целиком через
// atomic.Pointer в Manager. Хот-пат карты (чтение) не берёт мьютексов.
package mapcache

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"sync/atomic"

	"zorion/internal/resource"
)

// World — один мир вселенной в снапшоте карты.
type World struct {
	ID           string
	Name         string
	X, Y         float64
	Spectral     string
	Temp         float64
	StarType     string                 // star/white_dwarf/neutron/black_hole/protostar (99.2.4 §2)
	SystemType   string                 // single/binary/multiple
	StellarMods  map[string]interface{} // модификаторы (для бейджей карты/модалки, §8)
	HasPlanets   bool
	HasLife      bool
	HasHabitable bool
	// Resources — бит-маска категорий ресурсов с богатством > 0.3
	// (индекс категории в resource.AllCategories).
	Resources uint8
	// PlanetTypes — присутствующие на планетах мира типы (нижний регистр).
	// nil, если планет нет.
	PlanetTypes map[string]bool
}

// Snapshot — неизменяемый срез всех миров вселенной.
type Snapshot struct {
	worlds []World
}

// NewSnapshot — снапшот из готового списка миров (тесты, пересборка).
func NewSnapshot(worlds []World) *Snapshot {
	return &Snapshot{worlds: worlds}
}

// Len — число миров в снапшоте (0 для неготового снапшота).
func (s *Snapshot) Len() int {
	if s == nil {
		return 0
	}
	return len(s.worlds)
}

// Worlds — все миры снапшота. Snapshot immutable: срез возвращается без
// копии, читатели не должны его мутировать. Используется NPCManager для
// выбора маршрута агентов (спека 20a.1 §3.2) и координат позиций (§2.2.B).
func (s *Snapshot) Worlds() []World {
	if s == nil {
		return nil
	}
	return s.worlds
}

// Manager хранит текущий снапшот карты и умеет его пересобирать из БД.
type Manager struct {
	ptr atomic.Pointer[Snapshot]
}

func NewManager() *Manager {
	return &Manager{}
}

// Snapshot — текущий снапшот. nil до первой успешной загрузки.
func (m *Manager) Snapshot() *Snapshot {
	return m.ptr.Load()
}

// Replace — атомарно заменяет снапшот целиком. Читатели, уже держащие
// указатель на старый снапшот, продолжают работать с ним без блокировок.
func (m *Manager) Replace(s *Snapshot) {
	m.ptr.Store(s)
}

// LoadAndSwap — читает миры и сводку по планетам из БД и атомарно
// заменяет снапшот. При ошибке старый снапшот остаётся в силе.
func (m *Manager) LoadAndSwap(ctx context.Context, db *sql.DB) error {
	worlds, err := loadWorlds(ctx, db)
	if err != nil {
		return err
	}
	if err := mergePlanetSummaries(ctx, db, worlds); err != nil {
		return err
	}
	m.Replace(&Snapshot{worlds: worlds})
	return nil
}

// LoadAsync — LoadAndSwap в фоне. Ошибки только логируются, при провале
// сервер продолжает работать со старым снапшотом.
func (m *Manager) LoadAsync(db *sql.DB) {
	go func() {
		if err := m.LoadAndSwap(context.Background(), db); err != nil {
			log.Printf("❌ mapcache LoadAsync: %v", err)
			return
		}
		log.Printf("🗺️ mapcache: снапшот карты обновлён: %d миров", m.Snapshot().Len())
	}()
}

// ==================== ЗАГРУЗКА ИЗ БД ====================

func loadWorlds(ctx context.Context, db *sql.DB) ([]World, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, coord_x, coord_y, COALESCE(spectral_class,''),
		       temperature, star_type, system_type, stellar_mods
		FROM worlds
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var worlds []World
	for rows.Next() {
		var w World
		var modsRaw []byte
		if err := rows.Scan(&w.ID, &w.Name, &w.X, &w.Y, &w.Spectral, &w.Temp,
			&w.StarType, &w.SystemType, &modsRaw); err != nil {
			return nil, err
		}
		if len(modsRaw) > 0 && string(modsRaw) != "null" {
			var mods map[string]interface{}
			if err := json.Unmarshal(modsRaw, &mods); err != nil {
				return nil, err
			}
			w.StellarMods = mods
		}
		worlds = append(worlds, w)
	}
	return worlds, rows.Err()
}

// mergePlanetSummaries — дополняет миры сводкой по планетам: флаги
// жизни/обитаемости, типы планет и категории ресурсов с богатством > 0.3.
// Ровно те условия, что раньше жили в EXISTS-подзапросах фильтра.
func mergePlanetSummaries(ctx context.Context, db *sql.DB, worlds []World) error {
	if len(worlds) == 0 {
		return nil
	}
	idx := make(map[string]int, len(worlds))
	for i := range worlds {
		idx[worlds[i].ID] = i
	}

	rows, err := db.QueryContext(ctx, `
		SELECT p.world_id, p.data->>'life', p.data->>'type', p.data->'resources',
		       EXISTS(SELECT 1 FROM settlements s WHERE s.planet_id = p.id)
		FROM planets p
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var worldID string
		var life, typ sql.NullString
		var resJSON []byte
		var settled sql.NullBool
		if err := rows.Scan(&worldID, &life, &typ, &resJSON, &settled); err != nil {
			return err
		}
		i, ok := idx[worldID]
		if !ok {
			continue
		}
		w := &worlds[i]
		w.HasPlanets = true
		if life.String == "true" {
			w.HasLife = true
		}
		if settled.Bool {
			w.HasHabitable = true
		}
		if typ.Valid && typ.String != "" {
			if w.PlanetTypes == nil {
				w.PlanetTypes = make(map[string]bool, 2)
			}
			w.PlanetTypes[strings.ToLower(typ.String)] = true
		}
		if len(resJSON) > 0 && string(resJSON) != "null" {
			var summary map[string]float64
			if err := json.Unmarshal(resJSON, &summary); err != nil {
				return err
			}
			for _, cat := range resource.AllCategories {
				if v, ok := summary[cat]; ok && v > 0.3 {
					w.Resources |= resourceBit(cat)
				}
			}
		}
	}
	return rows.Err()
}

// resourceBit — позиция категории в бит-маске Resources. Неизвестная
// категория даёт 0 — мир с таким фильтром не совпадает (как и в SQL).
func resourceBit(code string) uint8 {
	for i, cat := range resource.AllCategories {
		if cat == code {
			return 1 << uint(i)
		}
	}
	return 0
}