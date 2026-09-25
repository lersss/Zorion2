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
	"sort"
	"sync"

	"zorion/internal/models"
)

// catalog — глобальный каталог оборудования (паттерн planet.LoadArchetypes).
var (
	mu       sync.RWMutex
	equipment = map[string]models.EquipmentItem{}
)

// defaultEquipment — дефолты на случай пустой БД (спека 77a §3.3/§4.2,
// 91a §7.1: двигатель engine_1 — настоящий модуль; трюм §8.3: грузовой
// модуль cargo_1 — params.capacity в тоннах).
var defaultEquipment = []models.EquipmentItem{
	{ID: "radar_1", Type: models.EquipmentTypeRadar, Name: "Радар-1",
		Params: map[string]interface{}{"radius": float64(models.RadarRadiusDefault)}},
	{ID: "scanner_1", Type: models.EquipmentTypeScanner, Name: "Сканер-1",
		Params: map[string]interface{}{"depth": "surface", "settlements": true}},
	{ID: "engine_1", Type: models.EquipmentTypeEngine, Name: "Двигатель-1",
		Params: map[string]interface{}{"speed_factor": models.EngineSpeedDefault}},
	{ID: models.StarterCargoModuleID, Type: models.EquipmentTypeCargo, Name: "Грузовой модуль-1",
		Params: map[string]interface{}{"capacity": float64(30)}},
	{ID: models.StarterAcceleratorID, Type: models.EquipmentTypeAccelerator, Name: "Ускоритель-1",
		Params: map[string]interface{}{
			"game":                  AcceleratorGameRoute,
			"cooldown_min":          float64(25),
			"bonus_max":             float64(0.50),
			"min_remaining_offer_s": float64(180),
			"min_remaining_boost_s": float64(90),
		}},
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
// систему в радиусе радара возможен только со сканером). Спека магазина §9
// (И-М5): валидный сканер ищется по ЛЮБОМУ слоту equipment.
func HasScanner(userEquipment map[string]interface{}) bool {
	return hasType(userEquipment, models.EquipmentTypeScanner)
}

// HasEngine — установлен ли валидный двигатель (спека 91a §6.1): предмет
// существует в каталоге и имеет тип engine. Без валидного двигателя полёт для
// role=player запрещён (валидация /travel); админ/skycomposer — исключение.
// Спека магазина §9 (И-М5): двигатель ищется по ЛЮБОМУ слоту equipment (право
// полёта); скорость (EngineSpeed) — по-прежнему из легаси-слота engine.
func HasEngine(userEquipment map[string]interface{}) bool {
	return hasType(userEquipment, models.EquipmentTypeEngine)
}

// hasType — есть ли в equipment (любой слот) валидный предмет заданного типа.
// Спека магазина §9 (И-М5): модуль работает по типу, независимо от слота
// (легаси-ключи radar/scanner/engine и универсальные universal* равнозначны).
func hasType(userEquipment map[string]interface{}, typ models.EquipmentType) bool {
	for _, v := range userEquipment {
		id, ok := v.(string)
		if !ok || id == "" {
			continue
		}
		if it := EquipmentByID(id); it != nil && it.Type == typ {
			return true
		}
	}
	return false
}

// AllEquipment — весь каталог оборудования (для /me.ship_catalog, спека 91a
// §7.3): id/type/name/params, включая engine_1. Клиент рисует только
// установленное (И1) — каталог используется как справочник имён/параметров.
// Сортировка по id — детерминированный порядок (radar_1, scanner_1, engine_1).
func AllEquipment() []models.EquipmentItem {
	mu.RLock()
	defer mu.RUnlock()
	items := make([]models.EquipmentItem, 0, len(equipment))
	for _, it := range equipment {
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

// CargoCapacityMass — ёмкость трюма по массе (тонны, спека трюма §4/§8.3):
// врождённая ёмкость модели корабля + сумма params.capacity установленных
// грузовых модулей (equipment.type='cargo'). modelID — users.ship_model_id,
// userEquipment — users.equipment (JSONB).
// Безопасное чтение (режим отказа §11): неизвестная модель, неизвестный/
// битый модуль, чужой тип или нечисловой params — вклад не учитывается,
// ёмкость не становится бесконечной (в худшем случае — только врождённая).
func CargoCapacityMass(modelID string, userEquipment map[string]interface{}) float64 {
	total := 0.0
	if m := ShipModelByID(modelID); m != nil {
		total = m.BaseCapacity
	}
	for _, v := range userEquipment {
		id, ok := v.(string)
		if !ok || id == "" {
			continue
		}
		it := EquipmentByID(id)
		if it == nil || it.Type != models.EquipmentTypeCargo {
			continue
		}
		if c, ok := it.Params["capacity"].(float64); ok && c > 0 {
			total += c
		}
	}
	return total
}
