// internal/handlers/admin_structures_options.go
//
// Сборка данных формы «Построить структуру» (спека
// 2026-09-24-постройка-структур-на-планете §5.2/§11): запросы, типы, хелперы
// для GET /admin/planets/{id}/build-options. Хендлеры и создание структуры —
// admin_structures.go / admin_structures_create.go.
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"zorion/internal/races"
	"zorion/internal/regionprofile"
)

// ==================== BUILD-OPTIONS ====================

// buildOptionType — тип структуры для формы (спека §11).
type buildOptionType struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	ClassID   int64             `json:"class_id"`
	ClassName string            `json:"class_name"`
	Target    string            `json:"target"`
	Stage     *buildOptionStage `json:"stage"`
	Live      bool              `json:"live"`
}

// buildOptionStage — ступень ладдеры (порог входа в людях).
type buildOptionStage struct {
	Enter float64 `json:"enter"`
}

// buildOptionFaction — фракция для выбора владельца (color/type — UI красит).
type buildOptionFaction struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Color string `json:"color"`
}

// buildOwnerOption — дефолтный владелец (фракция расы региона; ключ всегда).
type buildOwnerOption struct {
	OwnerType string `json:"owner_type"`
	OwnerID   string `json:"owner_id"`
	Name      string `json:"name"`
}

// buildRaceOption — раса региона (что проставится поселению; ключ всегда).
type buildRaceOption struct {
	RaceID   string `json:"race_id"`
	RaceName string `json:"race_name"`
}

// buildOptionsTypesSQL — дерево типов с классом-корнем и признаками (спека
// §5.2/§11): рекурсивный CTE поднимает каждый тип до корня; корень «Поселение»
// (id = $1) исключён; порядок — группы дерева (по корню) и id, а сортировка
// внутри «Поселения» по stage.enter — в Go (loadBuildOptionTypes): каст
// `enter` в numeric здесь падал бы на нечисловом значении (500), а Go уже
// парсит enter безопасно.
const buildOptionsTypesSQL = `
	WITH RECURSIVE up AS (
		SELECT id, parent_id, id AS root_id FROM producer_types WHERE parent_id IS NULL
		UNION ALL
		SELECT t.id, t.parent_id, u.root_id FROM producer_types t JOIN up u ON t.parent_id = u.id
	)
	SELECT t.id, t.name, u.root_id, root.name, t.params->'stage'->>'enter',
	       COALESCE((t.params ? 'eat') OR (t.params ? 'effects'), false)
	FROM producer_types t
	JOIN up u ON u.id = t.id
	JOIN producer_types root ON root.id = u.root_id
	WHERE t.hidden = false AND t.id <> $1
	ORDER BY u.root_id ASC, t.id ASC`

// BuildOptions — GET /admin/planets/{id}/build-options (спека §5.2/§11): типы
// дерева, фракции, дефолтный владелец/раса, песочный дефолт населения.
func (h *AdminHandlers) BuildOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	planetID, ok := planetRouteID(r.URL.Path, "build-options")
	if !ok {
		http.Error(w, "planet id required", http.StatusBadRequest)
		return
	}
	worldID, x, y, err := loadPlanetPlace(h.db, planetID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Planet not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to load planet: "+err.Error(), http.StatusInternalServerError)
		return
	}

	settlementRootID, err := resolveSettlementRootID(h.db)
	if err != nil {
		http.Error(w, "Failed to resolve class: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if settlementRootID == 0 {
		http.Error(w, "Класс типа не определён (якорь default_settlement_type_id)", http.StatusUnprocessableEntity)
		return
	}

	types, err := loadBuildOptionTypes(h.db, settlementRootID)
	if err != nil {
		http.Error(w, "Failed to load types: "+err.Error(), http.StatusInternalServerError)
		return
	}
	factions, err := loadBuildOptionFactions(h.db)
	if err != nil {
		http.Error(w, "Failed to load factions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	raceID, err := h.regionRace(x, y)
	if err != nil {
		http.Error(w, "Failed to load region: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defaultOwner, err := h.defaultOwner(planetID, raceID)
	if err != nil {
		http.Error(w, "Failed to resolve default owner: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defaultRace := buildRaceOptionOrNil(raceID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"planet_id":           planetID,
		"world_id":            worldID,
		"types":               types,
		"factions":            factions,
		"default_owner":       defaultOwner,
		"default_race":        defaultRace,
		"population_fallback": populationFallback,
	})
}

// loadPlanetPlace — world_id и координаты мира планеты (для расы региона).
func loadPlanetPlace(q rowQuerier, planetID string) (string, float64, float64, error) {
	var worldID string
	var x, y float64
	err := q.QueryRow(`
		SELECT p.world_id, w.coord_x, w.coord_y
		FROM planets p JOIN worlds w ON w.id = p.world_id
		WHERE p.id = $1`, planetID).Scan(&worldID, &x, &y)
	return worldID, x, y, err
}

// loadBuildOptionTypes — типы дерева для формы (см. buildOptionsTypesSQL).
// Порядок — группы дерева (по корню), внутри «Поселения» — по возрастанию
// stage.enter (нет входа / нечисловой enter — в конце, как NULLS LAST); сортировка
// в Go уже распарсенными значениями, чтобы нечисловой enter не ронял каст в SQL.
func loadBuildOptionTypes(db *sql.DB, settlementRootID int64) ([]buildOptionType, error) {
	rows, err := db.Query(buildOptionsTypesSQL, settlementRootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]buildOptionType, 0, 32)
	for rows.Next() {
		var t buildOptionType
		var classID int64
		var className string
		var enterRaw sql.NullString
		var live bool
		if err := rows.Scan(&t.ID, &t.Name, &classID, &className, &enterRaw, &live); err != nil {
			return nil, err
		}
		t.ClassID = classID
		t.ClassName = className
		t.Live = live
		if classID == settlementRootID {
			t.Target = "settlement"
		} else {
			t.Target = "building"
		}
		if enterRaw.Valid {
			if f, err := strconv.ParseFloat(enterRaw.String, 64); err == nil {
				t.Stage = &buildOptionStage{Enter: f}
			}
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ClassID != out[j].ClassID {
			return out[i].ClassID < out[j].ClassID
		}
		ei, ej := out[i].Stage, out[j].Stage
		if (ei == nil) != (ej == nil) {
			return ei != nil // у кого есть enter — раньше (NULLS LAST)
		}
		if ei != nil && ei.Enter != ej.Enter {
			return ei.Enter < ej.Enter
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// loadBuildOptionFactions — полный список фракций (color/type для UI).
func loadBuildOptionFactions(db *sql.DB) ([]buildOptionFaction, error) {
	rows, err := db.Query(`SELECT id, name, type, color FROM factions ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]buildOptionFaction, 0, 64)
	for rows.Next() {
		var f buildOptionFaction
		var ftype, color sql.NullString
		if err := rows.Scan(&f.ID, &f.Name, &ftype, &color); err != nil {
			return nil, err
		}
		f.Type = ftype.String
		f.Color = color.String
		out = append(out, f)
	}
	return out, rows.Err()
}

// regionRace — раса ближайшего региона к точке мира (Р9, как генерация).
func (h *AdminHandlers) regionRace(x, y float64) (string, error) {
	regions, err := h.loadRegionsWithProfiles()
	if err != nil {
		return "", err
	}
	if idx := regionprofile.NearestRegionIndex(x, y, regions); idx >= 0 {
		return regions[idx].RaceID, nil
	}
	return "", nil
}

// defaultOwner — дефолтный владелец формы: фракция расы региона планеты
// (Р8/Р9); fallback — фракция с homeworld_id = planet_id; иначе nil (ключ
// отдаётся всегда — значение null, спека §11).
func (h *AdminHandlers) defaultOwner(planetID, raceID string) (*buildOwnerOption, error) {
	var id, name string
	var err error
	if raceID != "" {
		err = h.db.QueryRow(`SELECT id, name FROM factions WHERE race_id = $1 LIMIT 1`, raceID).
			Scan(&id, &name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if id == "" {
		err = h.db.QueryRow(`SELECT id, name FROM factions WHERE homeworld_id = $1 LIMIT 1`, planetID).
			Scan(&id, &name)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
	}
	return &buildOwnerOption{OwnerType: "faction", OwnerID: id, Name: name}, nil
}

// buildRaceOptionOrNil — раса региона (nil, если регион без расы).
func buildRaceOptionOrNil(raceID string) *buildRaceOption {
	if raceID == "" {
		return nil
	}
	name := raceID
	if r := races.ByID(raceID); r != nil && r.Name != "" {
		name = r.Name
	}
	return &buildRaceOption{RaceID: raceID, RaceName: name}
}
