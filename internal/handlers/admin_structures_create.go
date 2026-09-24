// internal/handlers/admin_structures_create.go
//
// Создание структуры из игрового интерфейса (спека
// 2026-09-24-постройка-структур-на-планете §5.1/§6/§8, ред. 3):
// POST /admin/planets/{id}/structures. Роутинг и резолв класса — в
// admin_structures.go.
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"zorion/internal/generator"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// structureCreated — блок created ответа POST (спека §5.1): kind settlement|
// building, id, запрошенное type_name, имя владельца.
type structureCreated struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	TypeName  string `json:"type_name"`
	OwnerName string `json:"owner_name"`
}

// CreateStructure — POST /admin/planets/{id}/structures (спека §5.1/§6/§8).
// Тело {producer_type_id, owner_type, owner_id, population?}. Поселение →
// settlements (+ владелец + раса региона + население), прочее → buildings
// (building_type='producer' + producer_type_id + владелец). Ответ — свежие
// блоки карточки {created, buildings, settlements}.
func (h *AdminHandlers) CreateStructure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	planetID, ok := planetRouteID(r.URL.Path, "structures")
	if !ok {
		http.Error(w, "planet id required", http.StatusBadRequest)
		return
	}
	var body struct {
		ProducerTypeID int64  `json:"producer_type_id"`
		OwnerType      string `json:"owner_type"`
		OwnerID        string `json:"owner_id"`
		Population     *int   `json:"population"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Валидации входа (спека §5.1, порядок): тип → класс → владелец → население.
	if body.ProducerTypeID <= 0 {
		http.Error(w, "producer_type_id обязателен", http.StatusUnprocessableEntity)
		return
	}
	if body.OwnerType != "player" && body.OwnerType != "faction" && body.OwnerType != "agent" {
		http.Error(w, "owner_type вне {player,faction,agent}", http.StatusUnprocessableEntity)
		return
	}
	if body.OwnerID == "" {
		http.Error(w, "owner_id обязателен", http.StatusUnprocessableEntity)
		return
	}
	if _, err := uuid.Parse(body.OwnerID); err != nil {
		http.Error(w, "owner_id не UUID", http.StatusUnprocessableEntity)
		return
	}
	if body.Population != nil && *body.Population < 1 {
		http.Error(w, "население должно быть больше нуля", http.StatusUnprocessableEntity)
		return
	}

	// Гейты мутаций вселенной (образец AddDeposit): проверка + действие
	// атомарны — 409 ничего не создаёт.
	if statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}
	if !universeMutationMu.TryLock() {
		http.Error(w, "Universe mutation is running, wait for it", http.StatusConflict)
		return
	}
	defer universeMutationMu.Unlock()

	// Планета и раса её региона (Р9) — до транзакции (раса требует Go-резолва
	// ближайшего региона).
	worldID, x, y, err := loadPlanetPlace(h.db, planetID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Planet not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to load planet: "+err.Error(), http.StatusInternalServerError)
		return
	}
	raceID, err := h.regionRace(x, y)
	if err != nil {
		http.Error(w, "Failed to load region: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "Failed to start transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Тип: существует и не скрыт.
	var typeName string
	var hidden bool
	err = tx.QueryRowContext(ctx, `SELECT name, hidden FROM producer_types WHERE id = $1`, body.ProducerTypeID).
		Scan(&typeName, &hidden)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Тип структуры не найден", http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		http.Error(w, "Failed to load type: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if hidden {
		http.Error(w, "Тип скрыт", http.StatusUnprocessableEntity)
		return
	}

	// Класс — по корню дерева от якоря (§4): якорь не найден → 422.
	settlementRootID, err := resolveSettlementRootID(tx)
	if err != nil {
		http.Error(w, "Failed to resolve class: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if settlementRootID == 0 {
		http.Error(w, "Класс типа не определён (якорь default_settlement_type_id)", http.StatusUnprocessableEntity)
		return
	}
	rootID, err := typeRootID(tx, body.ProducerTypeID)
	if err != nil {
		http.Error(w, "Failed to resolve class: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if rootID == 0 {
		http.Error(w, "Класс типа не определён", http.StatusUnprocessableEntity)
		return
	}
	isSettlement := rootID == settlementRootID

	// Владелец — реальная сущность своего класса (висячий владелец сломал бы
	// плательщика ЧК2).
	ownerName, found, err := ownerExists(tx, body.OwnerType, body.OwnerID)
	if err != nil {
		http.Error(w, "Failed to load owner: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "Владелец не найден", http.StatusUnprocessableEntity)
		return
	}

	createdID := uuid.New().String()
	kind := "building"
	if isSettlement {
		kind = "settlement"
		population, err := settlementPopulation(tx, body.ProducerTypeID, body.Population)
		if err != nil {
			http.Error(w, "Failed to resolve population: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var raceArg interface{}
		if raceID != "" {
			raceArg = raceID
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settlements (id, planet_id, population, population_exact, stability,
			                         computed_at, race_id, settlement_type_id, owner_type, owner_id,
			                         created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), $6, $7, $8, $9, NOW(), NOW())`,
			createdID, planetID, population, float64(population), 100,
			raceArg, body.ProducerTypeID, body.OwnerType, body.OwnerID,
		); err != nil {
			http.Error(w, "Failed to insert settlement: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO buildings (id, planet_id, building_type, owner_type, owner_id, producer_type_id,
			                       created_at, updated_at)
			VALUES ($1, $2, 'producer', $3, $4, $5, NOW(), NOW())`,
			createdID, planetID, body.OwnerType, body.OwnerID, body.ProducerTypeID,
		); err != nil {
			http.Error(w, "Failed to insert building: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Свежие блоки карточки (спека §5.1): чтение полным путём — owner-проход
	// может перевести поселение на другую ступень (M6), фактическую ступень
	// клиент берёт из settlements[] по created.id.
	planet, err := repository.NewPlanetRepository(h.db).GetPlanetByID(planetID)
	if err != nil {
		http.Error(w, "Failed to load planet: "+err.Error(), http.StatusInternalServerError)
		return
	}
	buildings := []models.PlanetBuilding{}
	settlements := []models.Settlement{}
	if planet != nil {
		if planet.Buildings != nil {
			buildings = planet.Buildings
		}
		if planet.Settlements != nil {
			settlements = planet.Settlements
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"planet_id": planetID,
		"world_id":  worldID,
		"created": structureCreated{
			Kind:      kind,
			ID:        createdID,
			TypeName:  typeName, // ЗАПРОШЕННАЯ ступень (фактическая — в settlements[])
			OwnerName: ownerName,
		},
		"buildings":   buildings,
		"settlements": settlements,
	})
}

// settlementPopulation — население создаваемого поселения (M2): поле передано
// и ≥ 1 → как есть (проверено выше); не передано/пусто → stage.enter ступени,
// если enter ≥ 1, иначе песочный дефолт 1000.
func settlementPopulation(q rowQuerier, typeID int64, given *int) (int, error) {
	if given != nil {
		return *given, nil
	}
	var raw sql.NullString
	err := q.QueryRow(`SELECT params->'stage'->>'enter' FROM producer_types WHERE id = $1`, typeID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return populationFallback, nil
	}
	if err != nil {
		return 0, err
	}
	if raw.Valid {
		if f, err := strconv.ParseFloat(raw.String, 64); err == nil && f >= 1 {
			return int(f), nil
		}
	}
	return populationFallback, nil
}

// ownerExists — владелец существует в своей таблице (users/factions/
// npc_agents). Возвращает имя владельца. UUID уже проверен вызывающим.
func ownerExists(q rowQuerier, ownerType, ownerID string) (string, bool, error) {
	var name string
	var err error
	switch ownerType {
	case "player":
		err = q.QueryRow(`SELECT username FROM users WHERE id = $1`, ownerID).Scan(&name)
	case "faction":
		err = q.QueryRow(`SELECT name FROM factions WHERE id = $1`, ownerID).Scan(&name)
	case "agent":
		err = q.QueryRow(`SELECT name FROM npc_agents WHERE id = $1`, ownerID).Scan(&name)
	default:
		return "", false, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return name, true, nil
}
