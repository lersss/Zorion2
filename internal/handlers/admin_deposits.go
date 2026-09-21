// internal/handlers/admin_deposits.go
//
// Админ-инструмент «добавить залежь вручную» (спека 2026-09-22-поселение-
// добыча-сырья-биома-ленивый-буфер §6) и загрузка карты ресурсов каталога
// для генерации залежей (§3.1).
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"zorion/internal/generator"
	"zorion/internal/generator/planet"
	"zorion/internal/goodsstudio/graph"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// errDepositGood — ресурса нет или он не kind='resource' (§6).
var errDepositGood = errors.New("ресурс не найден или не resource")

// insertDepositsTx — вставка залежей планеты в уже начатую транзакцию
// (прототип/гипотезы, §3.4 спеки залежей). Вызывается ПОСЛЕ INSERT планеты
// (FK deposits.planet_id → planets).
func insertDepositsTx(ctx context.Context, tx *sql.Tx, deposits []models.SurfaceDeposit, now time.Time) error {
	for i := range deposits {
		d := &deposits[i]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO deposits (id, planet_id, good_id, stratum, wealth, amount, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			d.ID, d.PlanetID, d.GoodID, d.Stratum, d.Wealth, d.Amount, now, now,
		); err != nil {
			return fmt.Errorf("insert deposit %s: %w", d.ID, err)
		}
	}
	return nil
}

// loadResourceGoodsIndex — карта «name_norm → goods.id» для kind='resource'
// (§3.1): подаётся генератору сеттером SetGoodsIndex — пакет planet в БД не
// ходит. Ошибка чтения — пустая карта + лог (залежей не будет; генерация не
// падает из-за дрейфа контента).
func loadResourceGoodsIndex(db *sql.DB) map[string]int64 {
	index := map[string]int64{}
	rows, err := db.Query(`SELECT id, name_norm FROM goods WHERE kind = 'resource'`)
	if err != nil {
		log.Printf("⚠️ залежи: не удалось прочитать карту ресурсов: %v", err)
		return index
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var nameNorm string
		if err := rows.Scan(&id, &nameNorm); err != nil {
			log.Printf("⚠️ залежи: не удалось прочитать карту ресурсов: %v", err)
			return index
		}
		index[nameNorm] = id
	}
	return index
}

// rowQuerier — общее для *sql.DB и *sql.Tx (резолв ресурса внутри tx
// админ-ручки «добавить залежь»).
type rowQuerier interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

// resolveDepositGood — резолв ресурса каталога для админ-залежи (§6): по
// good_id либо по good_name. name_norm строится Go-нормализацией
// (graph.NormalizeName, Unicode ToLower) — НЕ SQL lower() (ловушка PITFALLS:
// collation C не понижает кириллицу).
func resolveDepositGood(q rowQuerier, id *int64, name string) (int64, string, error) {
	var goodID int64
	var goodName string
	var err error
	switch {
	case id != nil:
		err = q.QueryRow(`SELECT id, name FROM goods WHERE id = $1 AND kind = 'resource'`, *id).
			Scan(&goodID, &goodName)
	case name != "":
		err = q.QueryRow(`SELECT id, name FROM goods WHERE name_norm = $1 AND kind = 'resource'`, graph.NormalizeName(name)).
			Scan(&goodID, &goodName)
	default:
		return 0, "", errDepositGood
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", errDepositGood
	}
	if err != nil {
		return 0, "", err
	}
	return goodID, goodName, nil
}

// depositPlanetID — id планеты из пути /admin/planets/{id}/deposits.
func depositPlanetID(path string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/admin/planets/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "deposits" {
		return "", false
	}
	return parts[0], true
}

// AddDeposit — POST /admin/planets/{id}/deposits (§6): добавить залежь
// вручную (песочница). Тело: {good_id | good_name, stratum?, wealth?, amount?}.
// wealth/amount не заданы — значения-заглушки дизайн-ручек (§3.2, тот же
// источник чисел, что у генерации). Ответ — блок deposits планеты (§5.2):
// {planet_id, world_id, deposits[]}. Удаление/правка залежи — вне объёма §6.
func (h *AdminHandlers) AddDeposit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	planetID, ok := depositPlanetID(r.URL.Path)
	if !ok {
		http.Error(w, "planet id required", http.StatusBadRequest)
		return
	}
	var body struct {
		GoodID   *int64   `json:"good_id"`
		GoodName string   `json:"good_name"`
		Stratum  string   `json:"stratum"`
		Wealth   *float64 `json:"wealth"`
		Amount   *float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	// Итерация 1 — только 'surface' (задел 'subsurface' живёт в DDL §3.3).
	if body.Stratum != "" && body.Stratum != planet.DepositStratumSurface {
		http.Error(w, "stratum итерации 1 — только surface", http.StatusUnprocessableEntity)
		return
	}
	// Границы песочных значений (§3.3): wealth ∈ [0, 1], amount ≥ 0. Вне
	// границ — 422, а не 500 из CHECK БД.
	if body.Wealth != nil && (*body.Wealth < 0 || *body.Wealth > 1) {
		http.Error(w, "wealth вне [0, 1]", http.StatusUnprocessableEntity)
		return
	}
	if body.Amount != nil && *body.Amount < 0 {
		http.Error(w, "amount не может быть отрицательным", http.StatusUnprocessableEntity)
		return
	}
	// Гейты мутаций вселенной (образец GenerateFactions): пакман ест миры,
	// universeMutationMu держат ClearUniverse/пакман/фракции. Проверка +
	// действие атомарны — 409 ничего не создаёт.
	if statusManager.IsRunning(generator.JobPacman) {
		http.Error(w, "Generation is running, cancel it first", http.StatusConflict)
		return
	}
	if !universeMutationMu.TryLock() {
		http.Error(w, "Universe mutation is running, wait for it", http.StatusConflict)
		return
	}
	defer universeMutationMu.Unlock()

	// Одна транзакция (§6): SELECT world_id → INSERT залежи → чтение deposits
	// атомарны; частичный сбой откатывается, ответ не отдаёт полу-состояние.
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "Failed to start transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var worldID string
	err = tx.QueryRowContext(ctx, `SELECT world_id FROM planets WHERE id = $1`, planetID).Scan(&worldID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Planet not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to load planet: "+err.Error(), http.StatusInternalServerError)
		return
	}

	goodID, goodName, err := resolveDepositGood(tx, body.GoodID, body.GoodName)
	if err != nil {
		if errors.Is(err, errDepositGood) {
			http.Error(w, "Ресурс не найден (нужен kind=resource)", http.StatusUnprocessableEntity)
			return
		}
		http.Error(w, "Failed to load resource: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	wealth := planet.StubDepositWealth(rng)
	if body.Wealth != nil {
		wealth = *body.Wealth
	}
	amount := planet.StubDepositAmount(rng)
	if body.Amount != nil {
		amount = *body.Amount
	}

	repo := repository.NewDepositRepository(h.db)
	dep := models.SurfaceDeposit{
		ID:       uuid.New().String(),
		PlanetID: planetID,
		GoodID:   goodID,
		GoodName: goodName,
		Stratum:  planet.DepositStratumSurface,
		Wealth:   wealth,
		Amount:   amount,
	}
	if err := repo.InsertDeposit(ctx, tx, dep); err != nil {
		http.Error(w, "Failed to insert deposit: "+err.Error(), http.StatusInternalServerError)
		return
	}

	byPlanet, err := repo.GetDepositsByPlanetIDsTx(ctx, tx, []string{planetID})
	if err != nil {
		http.Error(w, "Failed to load deposits: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit: "+err.Error(), http.StatusInternalServerError)
		return
	}
	deposits := byPlanet[planetID]
	if deposits == nil {
		deposits = []models.SurfaceDeposit{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"planet_id": planetID,
		"world_id":  worldID,
		"deposits":  deposits,
	})
}
