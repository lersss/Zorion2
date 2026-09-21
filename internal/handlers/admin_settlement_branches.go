// internal/handlers/admin_settlement_branches.go
//
// Админ-ручки ветки поселения (спека 2026-09-22-поселение-ветка-буферы-
// переработка §5): создать ветку (поселение ↔ рецепт) и добавить ресурсы во
// входной буфер. Обе — JWT admin/skycomposer, гейты мутаций вселенной
// (Пакман/universeMutationMu), одна транзакция, контракт ответа — полный блок
// веток поселения в формате карточки (§6).
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"zorion/internal/generator"
	"zorion/internal/goodsstudio/graph"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// branchSettlementID — id поселения из пути /admin/settlements/{id}/branches.
func branchSettlementID(path string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/admin/settlements/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "branches" {
		return "", false
	}
	return parts[0], true
}

// branchInputID — id ветки из пути /admin/branches/{id}/input.
func branchInputID(path string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/admin/branches/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "input" {
		return "", false
	}
	return parts[0], true
}

// writeBranchesJSON — общий контракт ответа обеих ручек (§5): полный блок веток
// поселения; клиент заменяет блок целиком (образец ответа залежей).
func writeBranchesJSON(w http.ResponseWriter, settlementID, planetID string, branches []models.SettlementBranch) {
	if branches == nil {
		branches = []models.SettlementBranch{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"settlement_id": settlementID,
		"planet_id":     planetID,
		"branches":      branches,
	})
}

// AddBranch — POST /admin/settlements/{id}/branches (§5). Тело {recipe_id}.
// Поселения нет → 404; рецепта нет → 422; в рецепте нет заполненных
// компонентов → 422; ветка на этот рецепт уже есть → 409. Одна транзакция:
// INSERT ветки + посев буферов нулями (CreateBranchTx) → чтение блока веток.
func (h *AdminHandlers) AddBranch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	settlementID, ok := branchSettlementID(r.URL.Path)
	if !ok {
		http.Error(w, "settlement id required", http.StatusBadRequest)
		return
	}
	var body struct {
		RecipeID int64 `json:"recipe_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if body.RecipeID <= 0 {
		http.Error(w, "recipe_id обязателен", http.StatusUnprocessableEntity)
		return
	}
	// Гейты мутаций вселенной (образец AddDeposit): проверка + действие атомарны.
	if statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}
	if !universeMutationMu.TryLock() {
		http.Error(w, "Universe mutation is running, wait for it", http.StatusConflict)
		return
	}
	defer universeMutationMu.Unlock()

	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "Failed to start transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var planetID string
	err = tx.QueryRowContext(ctx, `SELECT planet_id FROM settlements WHERE id = $1`, settlementID).Scan(&planetID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Поселение не найдено", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to load settlement: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var outputGoodID int64
	var complexity sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT good_id, complexity FROM recipes WHERE id = $1`, body.RecipeID).Scan(&outputGoodID, &complexity)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Рецепт не найден", http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		http.Error(w, "Failed to load recipe: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var componentCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM recipe_components
		WHERE recipe_id = $1 AND component_id IS NOT NULL`, body.RecipeID,
	).Scan(&componentCount); err != nil {
		http.Error(w, "Failed to load recipe components: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if componentCount == 0 {
		http.Error(w, "Рецепт без заполненных компонентов", http.StatusUnprocessableEntity)
		return
	}

	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM settlement_branches WHERE settlement_id = $1 AND recipe_id = $2)`,
		settlementID, body.RecipeID,
	).Scan(&exists); err != nil {
		http.Error(w, "Failed to check branch: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "Ветка на этот рецепт уже есть", http.StatusConflict)
		return
	}

	repo := repository.NewBranchRepository(h.db)
	branchID := uuid.New().String()
	if err := repo.CreateBranchTx(ctx, tx, branchID, settlementID, body.RecipeID); err != nil {
		http.Error(w, "Failed to create branch: "+err.Error(), http.StatusInternalServerError)
		return
	}

	bySettlement, err := repo.GetBranchesBySettlementIDsTx(ctx, tx, []string{settlementID})
	if err != nil {
		http.Error(w, "Failed to load branches: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeBranchesJSON(w, settlementID, planetID, bySettlement[settlementID])
}

// AddBranchInput — POST /admin/branches/{id}/input (§5). Тело
// {good_id | good_name, amount}. Ветки нет → 404; amount ≤ 0/нет → 422;
// good_id не существует → 422; good — не компонент рецепта ветки → 422 (фильтр
// по component_id, не по kind, правка @critic №9). Первым действием транзакции —
// FOR UPDATE на строке ветки (единая точка сериализации с синком, §4.2), затем
// атомарный инкремент входа.
func (h *AdminHandlers) AddBranchInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	branchID, ok := branchInputID(r.URL.Path)
	if !ok {
		http.Error(w, "branch id required", http.StatusBadRequest)
		return
	}
	var body struct {
		GoodID   *int64   `json:"good_id"`
		GoodName string   `json:"good_name"`
		Amount   *float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if body.Amount == nil || *body.Amount <= 0 {
		http.Error(w, "amount должен быть больше 0", http.StatusUnprocessableEntity)
		return
	}
	if body.GoodID == nil && body.GoodName == "" {
		http.Error(w, "нужен good_id или good_name", http.StatusUnprocessableEntity)
		return
	}
	if statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}
	if !universeMutationMu.TryLock() {
		http.Error(w, "Universe mutation is running, wait for it", http.StatusConflict)
		return
	}
	defer universeMutationMu.Unlock()

	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "Failed to start transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	repo := repository.NewBranchRepository(h.db)
	settlementID, recipeID, err := repo.LockBranchTx(ctx, tx, branchID)
	if errors.Is(err, repository.ErrBranchNotFound) {
		http.Error(w, "Ветка не найдена", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to lock branch: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Резолв записи каталога: по good_id или по name_norm (Go-нормализация —
	// НЕ SQL lower(), PITFALLS). kind не фильтруется: компонент рецепта — любой
	// kind (правка @critic №9).
	var goodID int64
	var goodName string
	switch {
	case body.GoodID != nil:
		err = tx.QueryRowContext(ctx, `SELECT id, name FROM goods WHERE id = $1`, *body.GoodID).Scan(&goodID, &goodName)
	case body.GoodName != "":
		err = tx.QueryRowContext(ctx, `SELECT id, name FROM goods WHERE name_norm = $1`, graph.NormalizeName(body.GoodName)).Scan(&goodID, &goodName)
	}
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Запись каталога не найдена", http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		http.Error(w, "Failed to load good: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var isComponent bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM recipe_components WHERE recipe_id = $1 AND component_id = $2)`,
		recipeID, goodID,
	).Scan(&isComponent); err != nil {
		http.Error(w, "Failed to check component: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !isComponent {
		http.Error(w, "Запись не является компонентом рецепта ветки", http.StatusUnprocessableEntity)
		return
	}

	if err := repo.IncrementBranchInputTx(ctx, tx, branchID, goodID, *body.Amount); err != nil {
		http.Error(w, "Failed to add input: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var planetID string
	if err := tx.QueryRowContext(ctx, `SELECT planet_id FROM settlements WHERE id = $1`, settlementID).Scan(&planetID); err != nil {
		http.Error(w, "Failed to load settlement: "+err.Error(), http.StatusInternalServerError)
		return
	}
	bySettlement, err := repo.GetBranchesBySettlementIDsTx(ctx, tx, []string{settlementID})
	if err != nil {
		http.Error(w, "Failed to load branches: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeBranchesJSON(w, settlementID, planetID, bySettlement[settlementID])
}
