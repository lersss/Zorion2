// internal/repository/property_repository.go
// Read-модель собственности игрока (спека
// 2026-09-26-собственность-игрока-в-дашборде §4/§5): поселения и строения с
// владельцем-игроком (owner_type='player', owner_id из JWT) + адрес
// (планета/система) + режим знания строки. Новых сущностей нет — только чтение;
// пакетно (И-2): два запроса по владельцу, один по именам планет/систем и один
// по знанию — число запросов не растёт с числом планет/галактики.
//
// Семантика режимов повторяет резолв видимости (§4.3,
// internal/handlers/planet_visibility.go): presence — физическое присутствие
// (приоритет, at=null); snapshot — data.snapshot, fresh = не старше
// KnowledgeTTL; scan — запись знания без снимка; none — записи нет, mode
// выставляется ЯВНО. Дублирование осознанное (источник истины — резолв
// видимости; applyPlanetVisibility не вызывается ради пакетности).
package repository

import (
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"zorion/internal/models"
)

// Режимы знания строки собственности (§4.3).
const (
	PropertyKnowledgePresence = "presence"
	PropertyKnowledgeSnapshot = "snapshot"
	PropertyKnowledgeScan     = "scan"
	PropertyKnowledgeNone     = "none"
)

// Типы единиц собственности (§4.1, открытый список — задел расширения).
const (
	PropertyKindSettlement = "settlement"
	PropertyKindBuilding   = "building"
)

// PropertyKnowledge — режим знания строки (§5.1): at — UTC; nil для
// presence/none; fresh — актуальность показанной даты (не старше KnowledgeTTL).
type PropertyKnowledge struct {
	Mode  string     `json:"mode"`
	At    *time.Time `json:"at"`
	Fresh bool       `json:"fresh"`
}

// PropertyItem — строка собственности игрока (§5.1): только поля контракта,
// живых чисел планеты в списке нет (решение В2).
type PropertyItem struct {
	Kind       string            `json:"kind"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Subtitle   string            `json:"subtitle"`
	PlanetID   string            `json:"planet_id"`
	PlanetName string            `json:"planet_name"`
	WorldID    string            `json:"world_id"`
	WorldName  string            `json:"world_name"`
	Knowledge  PropertyKnowledge `json:"knowledge"`
}

// PropertyRepository — доступ к собственности игрока (пакетное чтение).
type PropertyRepository struct {
	db *sql.DB
}

func NewPropertyRepository(db *sql.DB) *PropertyRepository {
	return &PropertyRepository{db: db}
}

// propertyRow — единица владения до резолва адреса и знания.
type propertyRow struct {
	kind     string
	id       string
	name     string // поселение: имя расы; строение: имя типа-производителя
	subtitle string
	planetID string
}

// planetRef — адрес планеты: имя планеты и система.
type planetRef struct {
	planetName string
	worldID    string
	worldName  string
}

// propertyKnowledgeEntry — запись знания игрока о планете (пакетно).
type propertyKnowledgeEntry struct {
	scannedAt   time.Time
	hasSnapshot bool
	snapshotAt  time.Time
}

// GetPlayerProperty — список собственности игрока (§5). presencePlanetID —
// планета физического присутствия (пусто — присутствия нет); при совпадении
// строка получает режим presence (§4.3). Пустой список → пустой срез (И-4).
func (r *PropertyRepository) GetPlayerProperty(userID, presencePlanetID string) ([]PropertyItem, error) {
	settlements, err := r.playerSettlements(userID)
	if err != nil {
		return nil, err
	}
	buildings, err := r.playerBuildings(userID)
	if err != nil {
		return nil, err
	}
	rows := make([]propertyRow, 0, len(settlements)+len(buildings))
	rows = append(rows, settlements...)
	rows = append(rows, buildings...)
	if len(rows) == 0 {
		return []PropertyItem{}, nil
	}

	planetIDs := make([]string, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.planetID != "" && !seen[row.planetID] {
			seen[row.planetID] = true
			planetIDs = append(planetIDs, row.planetID)
		}
	}

	refs, err := r.planetAndWorldNames(planetIDs)
	if err != nil {
		return nil, err
	}
	knowledge, err := r.knowledgeBatch(userID, planetIDs)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	items := make([]PropertyItem, 0, len(rows))
	for _, row := range rows {
		ref := refs[row.planetID]
		entry, has := knowledge[row.planetID]
		items = append(items, PropertyItem{
			Kind:       row.kind,
			ID:         row.id,
			Name:       row.name,
			Subtitle:   row.subtitle,
			PlanetID:   row.planetID,
			PlanetName: ref.planetName,
			WorldID:    ref.worldID,
			WorldName:  ref.worldName,
			Knowledge:  resolvePropertyKnowledge(row.planetID, presencePlanetID, entry, has, now),
		})
	}
	sortPropertyItems(items)
	return items, nil
}

// playerSettlements — поселения игрока (§4.2): имя единицы — имя расы
// (каталог рас, как в моделях), подпись — тип поселения (producer_types.name).
func (r *PropertyRepository) playerSettlements(userID string) ([]propertyRow, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.planet_id, COALESCE(s.race_id, ''), COALESCE(pt.name, '')
		FROM settlements s
		LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id
		WHERE s.owner_type = 'player' AND s.owner_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]propertyRow, 0)
	for rows.Next() {
		var row propertyRow
		var raceID, typeName string
		if err := rows.Scan(&row.id, &row.planetID, &raceID, &typeName); err != nil {
			return nil, err
		}
		row.kind = PropertyKindSettlement
		row.name = raceName(raceID)
		row.subtitle = settlementTypeSubtitle(typeName)
		out = append(out, row)
	}
	return out, rows.Err()
}

// playerBuildings — строения игрока (§4.2): имя единицы — имя типа-производителя
// (у строений-игрока producer_type_id обязателен; fallback защитный).
func (r *PropertyRepository) playerBuildings(userID string) ([]propertyRow, error) {
	rows, err := r.db.Query(`
		SELECT b.id, b.planet_id, COALESCE(pt.name, '')
		FROM buildings b
		LEFT JOIN producer_types pt ON pt.id = b.producer_type_id
		WHERE b.owner_type = 'player' AND b.owner_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]propertyRow, 0)
	for rows.Next() {
		var row propertyRow
		var typeName string
		if err := rows.Scan(&row.id, &row.planetID, &typeName); err != nil {
			return nil, err
		}
		row.kind = PropertyKindBuilding
		row.name = buildingTypeName(typeName)
		row.subtitle = buildingSubtitle
		out = append(out, row)
	}
	return out, rows.Err()
}

// planetAndWorldNames — один пакетный запрос имён планет и их систем (И-2).
func (r *PropertyRepository) planetAndWorldNames(planetIDs []string) (map[string]planetRef, error) {
	refs := make(map[string]planetRef, len(planetIDs))
	if len(planetIDs) == 0 {
		return refs, nil
	}
	rows, err := r.db.Query(`
		SELECT p.id, p.name, p.world_id, COALESCE(w.name, '')
		FROM planets p
		LEFT JOIN worlds w ON w.id = p.world_id
		WHERE p.id = ANY($1)`, pqStringArray(planetIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var planetID, planetName, worldID, worldName string
		if err := rows.Scan(&planetID, &planetName, &worldID, &worldName); err != nil {
			return nil, err
		}
		refs[planetID] = planetRef{planetName: planetName, worldID: worldID, worldName: worldName}
	}
	return refs, rows.Err()
}

// knowledgeBatch — один пакетный запрос знания игрока по планетам (И-2).
func (r *PropertyRepository) knowledgeBatch(userID string, planetIDs []string) (map[string]propertyKnowledgeEntry, error) {
	out := make(map[string]propertyKnowledgeEntry, len(planetIDs))
	if len(planetIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(`
		SELECT planet_id, data, scanned_at
		FROM player_planet_knowledge
		WHERE user_id = $1 AND planet_id = ANY($2)`, userID, pqStringArray(planetIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var planetID string
		var dataRaw []byte
		var scannedAt time.Time
		if err := rows.Scan(&planetID, &dataRaw, &scannedAt); err != nil {
			return nil, err
		}
		entry := propertyKnowledgeEntry{scannedAt: scannedAt}
		if len(dataRaw) > 0 && string(dataRaw) != "null" {
			var data map[string]interface{}
			if err := json.Unmarshal(dataRaw, &data); err == nil {
				if at, ok := snapshotAtFromData(data); ok {
					entry.hasSnapshot = true
					entry.snapshotAt = at
				}
			}
		}
		out[planetID] = entry
	}
	return out, rows.Err()
}

// snapshotAtFromData — дата снимка data.snapshot (§3.1/§4.3), ok=false если
// снимка нет или дата нечитаема (тогда планета — скан-уровня, как в резолве).
func snapshotAtFromData(data map[string]interface{}) (time.Time, bool) {
	raw, ok := data["snapshot"]
	if !ok || raw == nil {
		return time.Time{}, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return time.Time{}, false
	}
	var s struct {
		At string `json:"at"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, s.At)
	if err != nil {
		return time.Time{}, false
	}
	return at, true
}

// resolvePropertyKnowledge — режим знания строки (§4.3). Приоритет:
// presence → snapshot → scan → none. has=false — записи знания нет.
func resolvePropertyKnowledge(planetID, presencePlanetID string, entry propertyKnowledgeEntry, has bool, now time.Time) PropertyKnowledge {
	if presencePlanetID != "" && planetID == presencePlanetID {
		return PropertyKnowledge{Mode: PropertyKnowledgePresence}
	}
	if !has {
		return PropertyKnowledge{Mode: PropertyKnowledgeNone}
	}
	if entry.hasSnapshot {
		at := entry.snapshotAt.UTC()
		return PropertyKnowledge{
			Mode:  PropertyKnowledgeSnapshot,
			At:    &at,
			Fresh: now.Sub(entry.snapshotAt) <= models.KnowledgeTTL,
		}
	}
	at := entry.scannedAt.UTC()
	return PropertyKnowledge{
		Mode:  PropertyKnowledgeScan,
		At:    &at,
		Fresh: now.Sub(entry.scannedAt) <= models.KnowledgeTTL,
	}
}

// sortPropertyItems — детерминированный порядок (§5.3): поселения, затем
// строения; внутри — world_name, planet_name, name, id (id — финальный
// тай-брейк: выборки идут без ORDER BY, а пары с полностью равными именами
// иначе всплыли бы в неопределённом порядке — ревью ЧК1а).
func sortPropertyItems(items []PropertyItem) {
	rank := func(kind string) int {
		if kind == PropertyKindSettlement {
			return 0
		}
		return 1
	}
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := rank(items[i].Kind), rank(items[j].Kind)
		if ri != rj {
			return ri < rj
		}
		if items[i].WorldName != items[j].WorldName {
			return items[i].WorldName < items[j].WorldName
		}
		if items[i].PlanetName != items[j].PlanetName {
			return items[i].PlanetName < items[j].PlanetName
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].ID < items[j].ID
	})
}

// Подписи единиц (§4.2): поселение — тип поселения (fallback «Поселение»),
// строение — «Строение».
const (
	settlementSubtitleFallback = "Поселение"
	buildingSubtitle           = "Строение"
)

func settlementTypeSubtitle(typeName string) string {
	if typeName == "" {
		return settlementSubtitleFallback
	}
	return typeName
}

func buildingTypeName(typeName string) string {
	if typeName == "" {
		return buildingSubtitle
	}
	return typeName
}
