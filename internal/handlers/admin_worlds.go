package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// GetAllWorlds — список миров для вкладки «Миры»: к пагинации по имени
// добавляется живое население мира (сумма поселений, пересчитанных на
// сейчас), итог по галактике с трендом и сортировка по населению в обе
// стороны (sort=population&order=asc|desc — миры грузятся целиком и
// сортируются по показанным значениям, а не по синхронному срезу).
func (h *AdminHandlers) GetAllWorlds(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 50
	}
	search := r.URL.Query().Get("search")
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	if order != "desc" {
		order = "asc"
	}

	report, err := repository.NewEconomyRepository(h.db).GalaxyPopulation(time.Now())
	if err != nil {
		http.Error(w, "Failed to fetch galaxy population: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var worlds []*models.World
	var total int
	if sortBy == "population" {
		all, err := h.worldRepo.GetAll()
		if err != nil {
			http.Error(w, "Failed to fetch worlds: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if search != "" {
			needle := strings.ToLower(search)
			filtered := make([]*models.World, 0, len(all))
			for _, world := range all {
				if strings.Contains(strings.ToLower(world.Name), needle) {
					filtered = append(filtered, world)
				}
			}
			all = filtered
		}
		for _, world := range all {
			world.Population = report.ByWorld[world.ID]
			world.PopulationTrend = report.WorldTrend(world.ID)
		}

		sort.SliceStable(all, func(i, j int) bool {
			if all[i].Population == all[j].Population {
				return all[i].Name < all[j].Name
			}
			if order == "desc" {
				return all[i].Population > all[j].Population
			}
			return all[i].Population < all[j].Population
		})

		total = len(all)
		start := (page - 1) * limit
		if start >= total {
			worlds = []*models.World{}
		} else {
			end := start + limit
			if end > total {
				end = total
			}
			worlds = all[start:end]
		}
	} else {
		worlds, total, err = h.worldRepo.GetAllPaginated(page, limit, search)
		if err != nil {
			http.Error(w, "Failed to fetch worlds: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, world := range worlds {
			world.Population = report.ByWorld[world.ID]
			world.PopulationTrend = report.WorldTrend(world.ID)
		}
	}

	response := struct {
		Data             []*models.World `json:"data"`
		Page             int             `json:"page"`
		Limit            int             `json:"limit"`
		Total            int             `json:"total"`
		GalaxyPopulation int64           `json:"galaxy_population"`
		GalaxyTrend      string          `json:"galaxy_trend"`
	}{
		Data:             worlds,
		Page:             page,
		Limit:            limit,
		Total:            total,
		GalaxyPopulation: report.Total,
		GalaxyTrend:      report.Trend,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *AdminHandlers) DeleteWorld(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	_, err := h.worldRepo.GetByID(req.ID)
	if err != nil {
		http.Error(w, "World not found", http.StatusNotFound)
		return
	}
	// Возврат залога живых контрактов мира ДО удаления (§6.5) — одна транзакция
	// с DELETE: каскад сносит contracts через planets, залог не должен пропасть.
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	if _, err := repository.ReturnEscrowForContractsTx(tx,
		repository.ContractScope{WorldIDs: []string{req.ID}}, models.EscrowReasonWorldDeleted); err != nil {
		http.Error(w, "Failed to return escrow: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Очистка ссылок на удаляемый мир (шаги 1, 2, 3, 4, 4.5 пакман-очистки,
	// общий хелпер §3.2): игроки теряют мир, NPC-агенты/знание/внутрисистемные
	// полёты/намерения маршрута сносятся — иначе DELETE worlds падает на FK
	// без каскада (npc_agents.current/from/target_world_id — NO ACTION, B25).
	if _, err := h.clearWorldReferencesForWorlds(tx, []string{req.ID}); err != nil {
		http.Error(w, "Failed to clear world references: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(`DELETE FROM worlds WHERE id = $1`, req.ID); err != nil {
		http.Error(w, "Failed to delete world: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.invalidatePlanetStats()
	h.mapCache.LoadAsync(h.db)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"deleted"}`))
}

func (h *AdminHandlers) CreateWorld(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string  `json:"name"`
		CoordX float64 `json:"coord_x"`
		CoordY float64 `json:"coord_y"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	id := uuid.New().String()
	query := `INSERT INTO worlds (id, name, coord_x, coord_y) VALUES ($1, $2, $3, $4)`
	_, err := h.db.Exec(query, id, req.Name, req.CoordX, req.CoordY)
	if err != nil {
		http.Error(w, "Failed to create world: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.invalidatePlanetStats()
	h.mapCache.LoadAsync(h.db)
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"created","id":"` + id + `"}`))
}
