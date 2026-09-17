// internal/repository/knowledge_repository.go
// Личный каталог знания о планетах (спека 77a §8): player_planet_knowledge.
// Чтение по PK (user_id, planet_id) — O(1); запись — UPSERT; рост таблицы
// ограничен радиусом сканера (десятки–сотни планет на игрока, §11.3).
package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"zorion/internal/models"
)

// KnowledgeRepository — доступ к player_planet_knowledge.
type KnowledgeRepository struct {
	db *sql.DB
}

func NewKnowledgeRepository(db *sql.DB) *KnowledgeRepository {
	return &KnowledgeRepository{db: db}
}

// GetKnowledge — знание игрока о планете (nil, если записи нет).
func (r *KnowledgeRepository) GetKnowledge(userID, planetID string) (*models.PlanetKnowledge, error) {
	var k models.PlanetKnowledge
	var dataRaw []byte
	err := r.db.QueryRow(
		`SELECT user_id, planet_id, data, scanned_at, source
		 FROM player_planet_knowledge WHERE user_id = $1 AND planet_id = $2`,
		userID, planetID,
	).Scan(&k.UserID, &k.PlanetID, &dataRaw, &k.ScannedAt, &k.Source)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(dataRaw) > 0 && string(dataRaw) != "null" {
		if err := json.Unmarshal(dataRaw, &k.Data); err != nil {
			return nil, err
		}
	}
	return &k, nil
}

// UpsertKnowledge — пишет/обновляет знание (одна запись на (игрок, планета),
// спека 77a §8.1: обновление при новом скане, дата = now).
func (r *KnowledgeRepository) UpsertKnowledge(userID, planetID string, data map[string]interface{}, source string) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("upsert knowledge: marshal: %w", err)
	}
	_, err = r.db.Exec(
		`INSERT INTO player_planet_knowledge (user_id, planet_id, data, scanned_at, source)
		 VALUES ($1, $2, $3, NOW(), $4)
		 ON CONFLICT (user_id, planet_id)
		 DO UPDATE SET data = EXCLUDED.data, scanned_at = NOW(), source = EXCLUDED.source`,
		userID, planetID, dataJSON, source,
	)
	if err != nil {
		return fmt.Errorf("upsert knowledge: %w", err)
	}
	return nil
}

// KnownWorldIDs — множество world_id систем, о которых у игрока есть знание
// (спека 77a §5.1/§7.1: знание координат «зажигает» звезду — полёт и полные
// данные разрешены). Один запрос на запрос карты, дальше — проверка в map.
func (r *KnowledgeRepository) KnownWorldIDs(userID string) (map[string]bool, error) {
	rows, err := r.db.Query(
		`SELECT DISTINCT p.world_id
		 FROM player_planet_knowledge k
		 JOIN planets p ON p.id = k.planet_id
		 WHERE k.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	known := map[string]bool{}
	for rows.Next() {
		var worldID string
		if err := rows.Scan(&worldID); err != nil {
			return nil, err
		}
		known[worldID] = true
	}
	return known, rows.Err()
}

// ScanSystem — ленивый прогон сканера (спека 77a §6.1, режим A): при взгляде
// на систему в радиусе радара сервер обновляет знание игрока о планетах
// системы (дата = now). Сканер вскрывает поверхность (surface_dominant,
// surface_composition) и наличие поселений (есть/нет + число) — спека §6.2;
// недра/атмосфера/детали поселений не вскрываются.
func (r *KnowledgeRepository) ScanSystem(userID, worldID string) error {
	rows, err := r.db.Query(
		`SELECT p.id,
		        COALESCE(p.data->>'surface_dominant', ''),
		        COALESCE(p.data->'surface_composition', '{}'::jsonb),
		        (SELECT COUNT(*) FROM settlements s WHERE s.planet_id = p.id)
		 FROM planets p WHERE p.world_id = $1`,
		worldID,
	)
	if err != nil {
		return fmt.Errorf("scan system: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var planetID string
		var surfaceDominant string
		var surfaceRaw []byte
		var settlementsCount int
		if err := rows.Scan(&planetID, &surfaceDominant, &surfaceRaw, &settlementsCount); err != nil {
			return fmt.Errorf("scan system: %w", err)
		}
		var surface map[string]interface{}
		if len(surfaceRaw) > 0 && string(surfaceRaw) != "null" {
			if err := json.Unmarshal(surfaceRaw, &surface); err != nil {
				return fmt.Errorf("scan system: %w", err)
			}
		}
		data := map[string]interface{}{
			"surface_dominant":   surfaceDominant,
			"surface_composition": surface,
			"settlements_count":  settlementsCount,
		}
		if err := r.UpsertKnowledge(userID, planetID, data, "scanner"); err != nil {
			return err
		}
	}
	return rows.Err()
}