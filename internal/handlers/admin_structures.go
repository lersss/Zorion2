// internal/handlers/admin_structures.go
//
// Админ-инструмент «Построить структуру на планете» (спека
// 2026-09-24-постройка-структур-на-планете §5–§11, ред. 3): поставить любую
// структуру из дерева производителей прямо из карточки планеты.
//   - GET  /admin/planets/{id}/build-options — данные формы (типы/фракции/
//     дефолты; сборка — admin_structures_options.go);
//   - GET  /admin/owner-candidates?type=&q= — поиск владельца (player/agent);
//   - POST /admin/planets/{id}/structures — создание структуры
//     (admin_structures_create.go).
//
// Здесь — роутинг, резолв класса типа по якорю и поиск владельца. Права —
// auth.AdminAuth (роут). Гейты мутаций вселенной — как у AddDeposit: пакман/
// универсальный мьютекс → 409 (проверка и действие атомарны).
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"zorion/internal/models"
)

// populationFallback — песочный дефолт населения поселения, когда у ступени
// нет порога входа (stage.enter < 1). Не калибровка (спека §8, M2).
const populationFallback = 1000

// ownerCandidatesLimit — верхняя граница выдачи поиска владельца (спека §11).
const ownerCandidatesLimit = 50

// HandlePlanetRoute — диспетчер /admin/planets/{id}/{action} (спека §5):
// …/deposits — AddDeposit (залежь), …/build-options — BuildOptions,
// …/structures — CreateStructure.
func (h *AdminHandlers) HandlePlanetRoute(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/deposits"):
		h.AddDeposit(w, r)
	case strings.HasSuffix(r.URL.Path, "/build-options"):
		h.BuildOptions(w, r)
	case strings.HasSuffix(r.URL.Path, "/structures"):
		h.CreateStructure(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// planetRouteID — id планеты из пути /admin/planets/{id}/{action}.
func planetRouteID(path, action string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/admin/planets/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != action {
		return "", false
	}
	return parts[0], true
}

// ==================== КЛАСС ТИПА (резолв по якорю) ====================

// resolveSettlementRootID — id класс-корня «Поселение»: якорь
// generation_config.default_settlement_type_id → рекурсивный обход parent_id
// вверх до корня (спека §4, rename-safe, НЕ по name_norm). Якорь может стоять
// на любой глубине дерева, поэтому обход рекурсивный (как typeRootID).
// 0 = не определён.
func resolveSettlementRootID(q rowQuerier) (int64, error) {
	var raw []byte
	err := q.QueryRow(`SELECT payload FROM generation_config WHERE key = $1`,
		models.DefaultSettlementTypeIDKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var typeID int64
	if err := json.Unmarshal(raw, &typeID); err != nil || typeID == 0 {
		return 0, nil
	}
	return typeRootID(q, typeID)
}

// typeRootID — корень дерева типа (рекурсивный обход parent_id вверх до
// parent_id IS NULL). 0 = тип не найден / корень не определён.
func typeRootID(q rowQuerier, typeID int64) (int64, error) {
	var rootID sql.NullInt64
	err := q.QueryRow(`
		WITH RECURSIVE up AS (
			SELECT id, parent_id FROM producer_types WHERE id = $1
			UNION ALL
			SELECT p.id, p.parent_id FROM producer_types p JOIN up ON p.id = up.parent_id
		)
		SELECT id FROM up WHERE parent_id IS NULL`, typeID).Scan(&rootID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !rootID.Valid {
		return 0, nil
	}
	return rootID.Int64, nil
}

// ==================== OWNER-CANDIDATES ====================

// ownerCandidate — кандидат владельца (игрок/агент) для поиска.
type ownerCandidate struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Subtitle string `json:"subtitle"`
}

// OwnerCandidates — GET /admin/owner-candidates?type=player|agent&q=&limit=
// (спека §11): поиск владельца, ≤50. Минимум q = 2: короче (в т.ч. пусто) —
// запрос в БД не идёт (пустой список, подсказка — на клиенте).
func (h *AdminHandlers) OwnerCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	ownerType := r.URL.Query().Get("type")
	limit := ownerCandidatesLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n < limit {
			limit = n
		}
	}

	items := []ownerCandidate{}
	if utf8.RuneCountInString(q) < 2 {
		writeOwnerCandidates(w, items)
		return
	}

	pattern := "%" + escapeLike(q) + "%"
	var rows *sql.Rows
	var err error
	switch ownerType {
	case "player":
		rows, err = h.db.Query(
			`SELECT id, username FROM users WHERE username ILIKE $1 ESCAPE '\' ORDER BY username LIMIT $2`,
			pattern, limit)
	case "agent":
		rows, err = h.db.Query(
			`SELECT id, name FROM npc_agents WHERE name ILIKE $1 ESCAPE '\' ORDER BY name LIMIT $2`,
			pattern, limit)
	default:
		http.Error(w, "type должен быть player|agent", http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		http.Error(w, "Failed to search owners: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	subtitle := "игрок"
	if ownerType == "agent" {
		subtitle = "агент"
	}
	for rows.Next() {
		var c ownerCandidate
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			http.Error(w, "Failed to scan owner: "+err.Error(), http.StatusInternalServerError)
			return
		}
		c.Subtitle = subtitle
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to search owners: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeOwnerCandidates(w, items)
}

// writeOwnerCandidates — контракт ответа поиска (спека §11): {items:[...]}.
func writeOwnerCandidates(w http.ResponseWriter, items []ownerCandidate) {
	if items == nil {
		items = []ownerCandidate{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
}

// escapeLike — экранирование спецсимволов LIKE (%, _, \) в пользовательском q
// (запрос идёт с ESCAPE '\').
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
