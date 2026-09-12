package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"zorion/internal/models"
)

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

	worlds, total, err := h.worldRepo.GetAllPaginated(page, limit, search)
	if err != nil {
		http.Error(w, "Failed to fetch worlds: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := struct {
		Data  []*models.World `json:"data"`
		Page  int             `json:"page"`
		Limit int             `json:"limit"`
		Total int             `json:"total"`
	}{
		Data:  worlds,
		Page:  page,
		Limit: limit,
		Total: total,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *AdminHandlers) DeleteWorld(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID string `json:"id"` }
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
	query := `DELETE FROM worlds WHERE id = $1`
	_, err = h.db.Exec(query, req.ID)
	if err != nil {
		http.Error(w, "Failed to delete world: "+err.Error(), http.StatusInternalServerError)
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