// internal/repository/knowledge_repository.go
// Личный каталог знания о планетах (спека 77a §8): player_planet_knowledge.
// Чтение по PK (user_id, planet_id) — O(1); запись — UPSERT; рост таблицы
// ограничен радиусом сканера (десятки–сотни планет на игрока, §11.3).
package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

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
//
// Запись — MERGE (`data = player_planet_knowledge.data || EXCLUDED.data`, K1
// спеки 2026-09-23 §3.3): ключи, которых нет во входящем payload, сохраняются.
// За счёт этого ленивый скан (ScanSystem/ScanPlanet) физически не может стереть
// `data.snapshot`, который пишет FixatePresence.
func (r *KnowledgeRepository) UpsertKnowledge(userID, planetID string, data map[string]interface{}, source string) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("upsert knowledge: marshal: %w", err)
	}
	_, err = r.db.Exec(
		`INSERT INTO player_planet_knowledge (user_id, planet_id, data, scanned_at, source)
		 VALUES ($1, $2, $3, NOW(), $4)
		 ON CONFLICT (user_id, planet_id)
		 DO UPDATE SET data = player_planet_knowledge.data || EXCLUDED.data, scanned_at = NOW(), source = EXCLUDED.source`,
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

// ScanPlanet — авто-знание об одной планете (спека 99.2.27 §3.6, С6): прибытие
// на орбиту планеты = знание о ней (свежее, как физическое присутствие).
// Данные — как у сканера (поверхность + поселения), source — 'presence'
// (новое значение, 77a §8.1; CHECK-ограничения нет — миграция не нужна).
// Осознанное отклонение от 77a И5 (М-5): v1 даёт скан-уровень, полный уровень
// (недра/атмосфера/детали) — задел.
func (r *KnowledgeRepository) ScanPlanet(userID, planetID, source string) error {
	var surfaceDominant string
	var surfaceRaw []byte
	var settlementsCount int
	err := r.db.QueryRow(
		`SELECT COALESCE(p.data->>'surface_dominant', ''),
		        COALESCE(p.data->'surface_composition', '{}'::jsonb),
		        (SELECT COUNT(*) FROM settlements s WHERE s.planet_id = p.id)
		 FROM planets p WHERE p.id = $1`,
		planetID,
	).Scan(&surfaceDominant, &surfaceRaw, &settlementsCount)
	if err != nil {
		return fmt.Errorf("scan planet: %w", err)
	}
	var surface map[string]interface{}
	if len(surfaceRaw) > 0 && string(surfaceRaw) != "null" {
		if err := json.Unmarshal(surfaceRaw, &surface); err != nil {
			return fmt.Errorf("scan planet: %w", err)
		}
	}
	data := map[string]interface{}{
		"surface_dominant":   surfaceDominant,
		"surface_composition": surface,
		"settlements_count":  settlementsCount,
	}
	return r.UpsertKnowledge(userID, planetID, data, source)
}

// ==================== СНИМОК ПРИСУТСТВИЯ (спека 2026-09-23 §3) ====================

// presenceSnapshot — снимок знания (спека 2026-09-23 §3.1): замороженная
// картина планеты на момент фиксации присутствия, хранится полем data.snapshot.
// Хранит РЕЗУЛЬТАТ: нет population_exact/computed_at/R-компонент (иначе клиент
// достроит население по формуле и «замороженное» число поплывёт), нет
// branches[].input (вход видит только админ), нет effects/log (не
// замораживаются, решение создателя 2026-09-23 п.3).
type presenceSnapshot struct {
	At                 string                  `json:"at"`
	Source             string                  `json:"source"`
	SurfaceDominant    string                  `json:"surface_dominant"`
	SurfaceComposition map[string]float64      `json:"surface_composition"`
	Settlements        []snapshotSettlement    `json:"settlements"`
	Factions           []models.PlanetFaction  `json:"factions"`
	Buildings          []models.PlanetBuilding `json:"buildings"`
}

// snapshotSettlement — поселение в снимке: раса (список — две расы на планете
// сохраняются раздельно, М4), население-результат (целое), стабильность и
// ветки производства.
type snapshotSettlement struct {
	ID         string           `json:"id"`
	RaceID     string           `json:"race_id"`
	RaceName   string           `json:"race_name"`
	Population int              `json:"population"`
	Stability  int              `json:"stability"`
	Branches   []snapshotBranch `json:"branches"`
}

// snapshotBranch — ветка в снимке: рецепт (id/имя/сложность), выход и
// последние скаляры «за проход». Входной буфер (Input) не замораживается.
type snapshotBranch struct {
	RecipeID   int64                      `json:"recipe_id"`
	RecipeName string                     `json:"recipe_name"`
	Complexity int                        `json:"complexity"`
	Output     []models.BranchBufferEntry `json:"output"`
	Produced   float64                    `json:"produced"`
	Eaten      float64                    `json:"eaten"`
	EatenRate  float64                    `json:"eaten_rate"`
}

// FixatePresence — запись снимка присутствия (спека 2026-09-23 §3, хук для
// событий A1–A4/D1–D2): читает планету полным путём (поверхность + поселения +
// ветки + фракции + строения) и пишет data.snapshot + поверхность + счётчик
// поселений в player_planet_knowledge (merge, §3.3).
//
// Число населения берётся серверным читающим путём в момент события — это
// легальная точка продвижения чек-точки (существующий ленивый синк
// attachSettlements → SyncSettlements); в снимок идёт уже округлённое значение
// (§8.3, И-С3). Вызывающий обрабатывает ошибку best-effort (лог, полёт не
// роняется); при сбое остаётся предыдущий снимок.
func (r *KnowledgeRepository) FixatePresence(userID, planetID, source string) error {
	planet, err := NewPlanetRepository(r.db).GetPlanetByID(planetID)
	if err != nil {
		return fmt.Errorf("fixate presence: загрузка планеты %s: %w", planetID, err)
	}
	if planet == nil {
		return fmt.Errorf("fixate presence: планета %s не найдена", planetID)
	}
	data := map[string]interface{}{
		"surface_dominant":    planet.SurfaceDominant,
		"surface_composition": planet.SurfaceComposition,
		"settlements_count":   len(planet.Settlements),
		"snapshot":            buildPresenceSnapshot(planet, source, time.Now()),
	}
	return r.UpsertKnowledge(userID, planetID, data, source)
}

// buildPresenceSnapshot — чистая сборка снимка из полной модели планеты
// (спека 2026-09-23 §3.1). Списки нормализуются в пустые массивы (не null).
func buildPresenceSnapshot(p *models.Planet, source string, at time.Time) presenceSnapshot {
	snap := presenceSnapshot{
		At:                 at.UTC().Format(time.RFC3339),
		Source:             source,
		SurfaceDominant:    p.SurfaceDominant,
		SurfaceComposition: p.SurfaceComposition,
		Settlements:        make([]snapshotSettlement, 0, len(p.Settlements)),
		Factions:           p.Factions,
		Buildings:          p.Buildings,
	}
	if snap.Factions == nil {
		snap.Factions = []models.PlanetFaction{}
	}
	if snap.Buildings == nil {
		snap.Buildings = []models.PlanetBuilding{}
	}
	for i := range p.Settlements {
		s := &p.Settlements[i]
		ss := snapshotSettlement{
			ID:         s.ID,
			RaceID:     s.RaceID,
			RaceName:   s.RaceName,
			Population: s.Population,
			Stability:  s.Stability,
			Branches:   make([]snapshotBranch, 0, len(s.Branches)),
		}
		for j := range s.Branches {
			b := &s.Branches[j]
			ss.Branches = append(ss.Branches, snapshotBranch{
				RecipeID:   b.RecipeID,
				RecipeName: b.RecipeName,
				Complexity: b.Complexity,
				Output:     b.Output,
				Produced:   b.Produced,
				Eaten:      b.Eaten,
				EatenRate:  b.EatenRate,
			})
		}
		snap.Settlements = append(snap.Settlements, ss)
	}
	return snap
}
