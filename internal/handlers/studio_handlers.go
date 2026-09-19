// internal/handlers/studio_handlers.go
// HTTP-хендлеры /studio/api/* (спека переноса-студии-товаров-iterA §7):
// каталог товаров/ресурсов на БД. Доступ — auth.AdminAuth (admin/
// skycomposer, player → 403). Ответы — JSON, ошибки {"error": "..."}:
// 400 невалидный вход · 403 слот ресурсу / системная категория ·
// 404 не найдено · 409 дубликат имени / цикл / категория не пуста.
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
	"zorion/internal/goodsstudio/validate"
	"zorion/internal/repository"
)

// StudioHandlers — хендлеры студии товаров (каталог в БД).
type StudioHandlers struct {
	repo *repository.GoodsRepository
}

func NewStudioHandlers(db *sql.DB) *StudioHandlers {
	return &StudioHandlers{repo: repository.NewGoodsRepository(db)}
}

// --- представления (спека §7) ---

// CategoryView — категория в ответах: id/name/kind/is_system/code.
type CategoryView struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	IsSystem bool   `json:"is_system"`
	Code     string `json:"code,omitempty"`
}

// SlotView — слот в представлении состояния (имя/тир/статус разрешены).
type SlotView struct {
	GoodID        string `json:"good_id"`
	Name          string `json:"name"`
	Tier          int    `json:"tier"`
	Status        string `json:"status"`
	Quantity      int    `json:"quantity"`
	Reason        string `json:"reason,omitempty"`
	AllowResource bool   `json:"allow_resource"`
}

// GoodView — товар/ресурс в представлении состояния.
type GoodView struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	CategoryID   int64      `json:"category_id"`
	Status       string     `json:"status"`
	Kind         string     `json:"kind"`
	Source       string     `json:"source"`
	Tier         int        `json:"tier"`          // эффективный (override ?? вычисленный)
	TierComputed int        `json:"tier_computed"` // вычисленный (graph.Tier)
	TierOverride *int       `json:"tier_override"`
	Recipe       []SlotView `json:"recipe"`
	BannedAt     *string    `json:"banned_at"`
}

// StateView — полное состояние для UI (спека §7, GET /studio/api/state).
type StateView struct {
	Categories    []CategoryView     `json:"categories"`
	Goods         []GoodView         `json:"goods"`
	Banned        []GoodView         `json:"banned"`
	Unused        []GoodView         `json:"unused"`
	Warnings      []validate.Warning `json:"warnings"`
	Model         string             `json:"model"`
	Generating    bool               `json:"generating"`
	AutoRefreshMS int                `json:"auto_refresh_ms"`
}

// --- GET /studio/api/state ---

// State — полное состояние каталога (снимок REPEATABLE READ, §8.1).
func (h *StudioHandlers) State(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		studioErr(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	snap, err := h.repo.Snapshot()
	if err != nil {
		studioErr(w, "ошибка чтения каталога: "+err.Error(), http.StatusInternalServerError)
		return
	}
	studioJSON(w, http.StatusOK, buildStateView(snap))
}

// buildStateView — StateView из снимка (banned новые сверху, unused —
// in-degree 0, не banned, kind=good; warnings — прогон валидаторов).
func buildStateView(snap *repository.CatalogSnapshot) StateView {
	byID := graph.ByID(snap.Goods)
	view := StateView{
		Categories: make([]CategoryView, 0, len(snap.Categories)),
		Goods:      make([]GoodView, 0, len(snap.Goods)),
		Banned:     []GoodView{},
		Unused:     []GoodView{},
		Warnings:   validate.Validate(&model.State{SchemaVersion: model.SchemaVersion, Goods: snap.Goods}),
		Generating: false,
		Model:      "",
	}
	for _, c := range snap.Categories {
		cv := CategoryView{ID: c.ID, Name: c.Name, Kind: c.Kind, IsSystem: c.IsSystem}
		if c.Code.Valid {
			cv.Code = c.Code.String
		}
		view.Categories = append(view.Categories, cv)
	}

	inDegree := make(map[string]int, len(snap.Goods))
	for i := range snap.Goods {
		for _, slot := range snap.Goods[i].Recipe {
			if slot.GoodID != "" {
				inDegree[slot.GoodID]++
			}
		}
	}
	for i := range snap.Goods {
		g := &snap.Goods[i]
		catID, _ := strconv.ParseInt(g.Category, 10, 64)
		gv := GoodView{
			ID:           g.ID,
			Name:         g.Name,
			CategoryID:   catID,
			Status:       string(g.Status),
			Kind:         string(g.Kind),
			Source:       string(g.Source),
			Tier:         graph.EffectiveTier(g, byID),
			TierComputed: graph.Tier(g, byID),
			TierOverride: g.TierOverride,
			BannedAt:     g.BannedAt,
			Recipe:       []SlotView{},
		}
		for _, slot := range g.Recipe {
			q := slot.Quantity
			if q < 1 {
				q = 1
			}
			sv := SlotView{GoodID: slot.GoodID, Reason: slot.Reason, AllowResource: slot.AllowResource, Quantity: q}
			if comp := byID[slot.GoodID]; comp != nil {
				sv.Name = comp.Name
				sv.Tier = graph.EffectiveTier(comp, byID)
				sv.Status = string(comp.Status)
			}
			gv.Recipe = append(gv.Recipe, sv)
		}
		view.Goods = append(view.Goods, gv)
		if g.Status == model.StatusBanned {
			view.Banned = append(view.Banned, gv)
		}
		if g.Kind != model.KindResource && g.Status != model.StatusBanned && inDegree[g.ID] == 0 {
			view.Unused = append(view.Unused, gv)
		}
	}
	// бан — новые сверху (по banned_at, спека 99a.1 §5.2)
	sort.Slice(view.Banned, func(i, j int) bool {
		bi, bj := view.Banned[i].BannedAt, view.Banned[j].BannedAt
		if bi == nil || bj == nil {
			return false
		}
		return *bi > *bj
	})
	return view
}

// --- GET /studio/api/resources ---

// Resources — палитра ресурсов (kind=resource, не banned).
func (h *StudioHandlers) Resources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		studioErr(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	res, err := h.repo.Resources()
	if err != nil {
		studioErr(w, "ошибка чтения ресурсов: "+err.Error(), http.StatusInternalServerError)
		return
	}
	studioJSON(w, http.StatusOK, res)
}

// --- категории ---

// Categories — POST /studio/api/categories {name}: создание товарной
// категории (дубликат нормализованного имени — 409).
func (h *StudioHandlers) Categories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	c, err := h.repo.CreateCategory(body.Name)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusCreated, categoryView(c))
}

// CategoryByID — PUT/DELETE /studio/api/categories/{id}.
func (h *StudioHandlers) CategoryByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(strings.TrimPrefix(r.URL.Path, "/studio/api/categories/"))
	if err != nil {
		studioErr(w, "не найдено", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		if err := h.repo.RenameCategory(id, body.Name); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"id": id})
	case http.MethodDelete:
		if err := h.repo.DeleteCategory(id); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"deleted": id})
	default:
		studioErr(w, "только PUT/DELETE", http.StatusMethodNotAllowed)
	}
}

// --- товары ---

// Goods — POST /studio/api/goods и POST /studio/api/goods/bulk.
func (h *StudioHandlers) Goods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/bulk") {
		h.goodsBulk(w, r)
		return
	}
	var body struct {
		Name       string `json:"name"`
		CategoryID int64  `json:"category_id"`
		Kind       string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	g, err := h.repo.CreateGood(body.Name, body.CategoryID, model.Kind(body.Kind))
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	// контракт §7: категория в ответах — category_id (число), не строка
	gv := GoodView{
		ID:           g.ID,
		Name:         g.Name,
		CategoryID:   body.CategoryID,
		Status:       string(g.Status),
		Kind:         string(g.Kind),
		Source:       string(g.Source),
		Tier:         0,
		TierComputed: 0,
		Recipe:       []SlotView{},
	}
	for _, slot := range g.Recipe {
		gv.Recipe = append(gv.Recipe, SlotView{GoodID: slot.GoodID, Quantity: slot.Quantity, AllowResource: slot.AllowResource})
	}
	studioJSON(w, http.StatusCreated, gv)
}

// goodsBulk — POST /studio/api/goods/bulk {lines}: подгрузка списка
// (частичный успех, отчёт created/skipped/errors).
func (h *StudioHandlers) goodsBulk(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Lines []string `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	rep, err := h.repo.BulkCreateGoods(body.Lines)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, rep)
}

// GoodByID — PUT/DELETE /studio/api/goods/{id} и под-пути
// (status, tier, slots, slots/{n}, slots/{n}/component, slots/{n}/allow_resource).
func (h *StudioHandlers) GoodByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/studio/api/goods/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		studioErr(w, "не найдено", http.StatusNotFound)
		return
	}
	id, err := parseID(parts[0])
	if err != nil {
		studioErr(w, "не найдено", http.StatusNotFound)
		return
	}
	switch {
	case len(parts) == 1:
		h.good(w, r, id)
	case len(parts) == 2 && parts[1] == "status":
		h.goodStatus(w, r, id)
	case len(parts) == 2 && parts[1] == "tier":
		h.goodTier(w, r, id)
	case len(parts) == 2 && parts[1] == "slots":
		h.addSlot(w, r, id)
	case len(parts) == 3 && parts[1] == "slots":
		h.slot(w, r, id, parts[2])
	case len(parts) == 4 && parts[1] == "slots" && parts[3] == "component":
		h.clearSlot(w, r, id, parts[2])
	case len(parts) == 4 && parts[1] == "slots" && parts[3] == "allow_resource":
		h.slotAllowResource(w, r, id, parts[2])
	default:
		studioErr(w, "не найдено", http.StatusNotFound)
	}
}

// good — PUT/DELETE /studio/api/goods/{id} (для ресурсов разрешено, С1).
func (h *StudioHandlers) good(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name       *string `json:"name"`
			CategoryID *int64  `json:"category_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		if err := h.repo.UpdateGood(id, body.Name, body.CategoryID); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"id": id})
	case http.MethodDelete:
		cleared, err := h.repo.DeleteGood(id)
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]interface{}{"deleted": id, "cleared_links": cleared})
	default:
		studioErr(w, "только PUT/DELETE", http.StatusMethodNotAllowed)
	}
}

// goodStatus — POST /studio/api/goods/{id}/status {status}.
func (h *StudioHandlers) goodStatus(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	if err := h.repo.SetStatus(id, body.Status); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "status": body.Status})
}

// goodTier — PUT /studio/api/goods/{id}/tier {tier: int|null}.
func (h *StudioHandlers) goodTier(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPut {
		studioErr(w, "только PUT", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Tier *int `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	if err := h.repo.SetTier(id, body.Tier); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "tier": body.Tier})
}

// addSlot — POST /studio/api/goods/{id}/slots: добавить пустой слот
// (ресурсу — 403).
func (h *StudioHandlers) addSlot(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	if err := h.repo.AddSlot(id); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]int64{"id": id})
}

// slot — PUT/DELETE /studio/api/goods/{id}/slots/{n}.
func (h *StudioHandlers) slot(w http.ResponseWriter, r *http.Request, id int64, nStr string) {
	pos, err := parseID(nStr)
	if err != nil {
		studioErr(w, "невалидный номер слота", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			GoodID   *int64 `json:"good_id"`
			Quantity *int   `json:"quantity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		var compID int64
		if body.GoodID != nil {
			compID = *body.GoodID
		}
		if err := h.repo.PutSlot(id, int(pos), compID, body.Quantity); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "pos": pos})
	case http.MethodDelete:
		if err := h.repo.DeleteSlot(id, int(pos)); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "pos": pos})
	default:
		studioErr(w, "только PUT/DELETE", http.StatusMethodNotAllowed)
	}
}

// clearSlot — DELETE /studio/api/goods/{id}/slots/{n}/component.
func (h *StudioHandlers) clearSlot(w http.ResponseWriter, r *http.Request, id int64, nStr string) {
	if r.Method != http.MethodDelete {
		studioErr(w, "только DELETE", http.StatusMethodNotAllowed)
		return
	}
	pos, err := parseID(nStr)
	if err != nil {
		studioErr(w, "невалидный номер слота", http.StatusBadRequest)
		return
	}
	if err := h.repo.ClearSlot(id, int(pos)); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "pos": pos})
}

// slotAllowResource — PUT /studio/api/goods/{id}/slots/{n}/allow_resource.
func (h *StudioHandlers) slotAllowResource(w http.ResponseWriter, r *http.Request, id int64, nStr string) {
	if r.Method != http.MethodPut {
		studioErr(w, "только PUT", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AllowResource bool `json:"allow_resource"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	pos, err := parseID(nStr)
	if err != nil {
		studioErr(w, "невалидный номер слота", http.StatusBadRequest)
		return
	}
	if err := h.repo.SetSlotAllowResource(id, int(pos), body.AllowResource); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "pos": pos})
}

// --- GET /studio/api/validate ---

// Validate — прогон валидаторов (§8.3) на снимке каталога.
func (h *StudioHandlers) Validate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		studioErr(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	snap, err := h.repo.Snapshot()
	if err != nil {
		studioErr(w, "ошибка чтения каталога: "+err.Error(), http.StatusInternalServerError)
		return
	}
	warnings := validate.Validate(&model.State{SchemaVersion: model.SchemaVersion, Goods: snap.Goods})
	studioJSON(w, http.StatusOK, warnings)
}

// --- хелперы ---

// studioJSON — ответ JSON по контракту §7: Cache-Control no-store +
// Content-Type application/json; charset=utf-8.
func studioJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// studioErr — ошибка {"error": "..."} по контракту §7.
func studioErr(w http.ResponseWriter, msg string, status int) {
	studioJSON(w, status, map[string]string{"error": msg})
}

// writeCatalogErr — ошибка каталога (ErrCatalog) → HTTP-ответ; прочее — 500.
func writeCatalogErr(w http.ResponseWriter, err error) {
	if ce, ok := err.(*repository.ErrCatalog); ok {
		studioErr(w, ce.Msg, ce.Status)
		return
	}
	studioErr(w, "внутренняя ошибка: "+err.Error(), http.StatusInternalServerError)
}

// parseID — положительный int64 из пути.
func parseID(s string) (int64, error) {
	if s == "" || strings.Contains(s, "/") {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(s, 10, 64)
}

// categoryView — CategoryRow → CategoryView.
func categoryView(c repository.CategoryRow) CategoryView {
	cv := CategoryView{ID: c.ID, Name: c.Name, Kind: c.Kind, IsSystem: c.IsSystem}
	if c.Code.Valid {
		cv.Code = c.Code.String
	}
	return cv
}