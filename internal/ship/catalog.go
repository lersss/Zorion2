// internal/ship/catalog.go
// Каталог оборудования (спека 77a §3): in-memory справочник, загружается из
// БД при старте (как матрица совместимости). Источник правды — таблица
// equipment; дефолты — страховка, если БД пуста (миграция 000040 не
// применилась или справочник очищен). Каталог статичен между рестартами —
// рынки оборудования (будущее) будут менять его через БД + перезагрузку.
package ship

import (
	"database/sql"
	"encoding/json"
	"log"
	"sync"

	"zorion/internal/models"
)

// catalog — глобальный каталог оборудования (паттерн planet.LoadArchetypes).
var (
	mu       sync.RWMutex
	equipment = map[string]models.EquipmentItem{}
)

// defaultEquipment — дефолты на случай пустой БД (спека 77a §3.3/§4.2).
var defaultEquipment = []models.EquipmentItem{
	{ID: "radar_1", Type: models.EquipmentTypeRadar, Name: "Радар-1",
		Params: map[string]interface{}{"radius": float64(models.RadarRadiusDefault)}},
	{ID: "scanner_1", Type: models.EquipmentTypeScanner, Name: "Сканер-1",
		Params: map[string]interface{}{"depth": "surface", "settlements": true}},
}

// LoadCatalog — читает справочник оборудования из БД. Пустая БД — дефолты.
// Ошибка чтения — дефолты + лог (сервер живёт, как матрица совместимости).
func LoadCatalog(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, type, name, params FROM equipment`)
	if err != nil {
		log.Printf("⚠️ ship: каталог оборудования: %v (дефолты)", err)
		loadDefaults()
		return err
	}
	defer rows.Close()

	items := map[string]models.EquipmentItem{}
	for rows.Next() {
		var it models.EquipmentItem
		var typ string
		var paramsRaw []byte
		if err := rows.Scan(&it.ID, &typ, &it.Name, &paramsRaw); err != nil {
			log.Printf("⚠️ ship: каталог оборудования: %v (дефолты)", err)
			loadDefaults()
			return err
		}
		it.Type = models.EquipmentType(typ)
		if len(paramsRaw) > 0 && string(paramsRaw) != "null" {
			if err := json.Unmarshal(paramsRaw, &it.Params); err != nil {
				log.Printf("⚠️ ship: params оборудования %s: %v", it.ID, err)
			}
		}
		items[it.ID] = it
	}
	if err := rows.Err(); err != nil {
		log.Printf("⚠️ ship: каталог оборудования: %v (дефолты)", err)
		loadDefaults()
		return err
	}

	mu.Lock()
	equipment = items
	mu.Unlock()
	log.Printf("✅ ship: каталог оборудования загружен: %d предметов", len(items))
	return nil
}

// LoadDefaults — загружает дефолтный каталог (radar_1, scanner_1). Используется
// как фолбэк при ошибке чтения БД и в тестах.
func LoadDefaults() {
	items := map[string]models.EquipmentItem{}
	for _, it := range defaultEquipment {
		items[it.ID] = it
	}
	mu.Lock()
	equipment = items
	mu.Unlock()
}

func loadDefaults() {
	LoadDefaults()
}

// EquipmentByID — предмет из каталога (nil, если неизвестен).
func EquipmentByID(id string) *models.EquipmentItem {
	mu.RLock()
	defer mu.RUnlock()
	it, ok := equipment[id]
	if !ok {
		return nil
	}
	return &it
}

// HasScanner — установлен ли сканер (спека 77a §6: скан при взгляде на
// систему в радиусе радара возможен только со сканером).
func HasScanner(userEquipment map[string]interface{}) bool {
	radarID, _ := userEquipment["scanner"].(string)
	if radarID == "" {
		return false
	}
	it := EquipmentByID(radarID)
	return it != nil && it.Type == models.EquipmentTypeScanner
}