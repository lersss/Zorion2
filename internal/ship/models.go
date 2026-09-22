// internal/ship/models.go
// Модели кораблей (спека 77a §2): in-memory справочник, загружается из БД
// при старте (по образцу каталога оборудования). Источник правды — таблица
// ship_models; дефолт — стартовая модель 'starter' (страховка, если БД пуста
// или миграция 000040 не применилась). Нужен /me для имени модели (спека 91a §7.3).
package ship

import (
	"database/sql"
	"encoding/json"
	"log"
	"sync"

	"zorion/internal/models"
)

var (
	modelsMu   sync.RWMutex
	shipModels = map[string]models.ShipModel{}
)

// defaultShipModels — дефолты на случай пустой БД (спека 77a §2.2).
// Slots — float64, как после json.Unmarshal из БД (консистентный вывод /me).
// base_capacity 20 т + универсальный слот — трюм (спека трюма §5/§8.3).
var defaultShipModels = []models.ShipModel{
	{ID: models.StarterShipModelID, Name: "Стартовый разведчик",
		Slots:        map[string]interface{}{"radar": float64(1), "scanner": float64(1), "engine": float64(1), "universal": float64(1)},
		BaseCapacity: 20},
}

// LoadModels — читает справочник моделей кораблей из БД. Пустая БД — дефолты.
// Ошибка чтения — дефолты + лог (сервер живёт, как каталог оборудования).
func LoadModels(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, name, slots, base_capacity FROM ship_models`)
	if err != nil {
		log.Printf("⚠️ ship: модели кораблей: %v (дефолты)", err)
		loadModelDefaults()
		return err
	}
	defer rows.Close()

	items := map[string]models.ShipModel{}
	for rows.Next() {
		var m models.ShipModel
		var slotsRaw []byte
		if err := rows.Scan(&m.ID, &m.Name, &slotsRaw, &m.BaseCapacity); err != nil {
			log.Printf("⚠️ ship: модели кораблей: %v (дефолты)", err)
			loadModelDefaults()
			return err
		}
		if len(slotsRaw) > 0 && string(slotsRaw) != "null" {
			if err := json.Unmarshal(slotsRaw, &m.Slots); err != nil {
				log.Printf("⚠️ ship: slots модели %s: %v", m.ID, err)
			}
		}
		items[m.ID] = m
	}
	if err := rows.Err(); err != nil {
		log.Printf("⚠️ ship: модели кораблей: %v (дефолты)", err)
		loadModelDefaults()
		return err
	}

	modelsMu.Lock()
	shipModels = items
	modelsMu.Unlock()
	log.Printf("✅ ship: модели кораблей загружены: %d моделей", len(items))
	return nil
}

// LoadModelDefaults — загружает дефолтную модель (starter). Используется как
// фолбэк при ошибке чтения БД и в тестах.
func LoadModelDefaults() {
	items := map[string]models.ShipModel{}
	for _, m := range defaultShipModels {
		items[m.ID] = m
	}
	modelsMu.Lock()
	shipModels = items
	modelsMu.Unlock()
}

func loadModelDefaults() {
	LoadModelDefaults()
}

// ShipModelByID — модель корабля из справочника (nil, если неизвестна).
func ShipModelByID(id string) *models.ShipModel {
	modelsMu.RLock()
	defer modelsMu.RUnlock()
	m, ok := shipModels[id]
	if !ok {
		return nil
	}
	return &m
}