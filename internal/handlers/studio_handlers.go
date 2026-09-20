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
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"zorion/internal/goodsstudio/ai"
	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
	"zorion/internal/goodsstudio/validate"
	"zorion/internal/races"
	"zorion/internal/repository"
)

// StudioHandlers — хендлеры студии товаров (каталог в БД). Статус fill +
// proposals — in-memory под fillMu (спека переноса-студии-товаров-iterC §5.2:
// транзиентное состояние инструмента, не таблица; рестарт сбрасывает).
type StudioHandlers struct {
	repo    *repository.GoodsRepository
	ai      *ai.Client
	aiModel string

	fillMu               sync.Mutex
	fillGenerating       bool
	fillReport           []string
	fillProposals        []ProposalView // предложения последнего завершённого fill
	fillProposalsGoodID  string         // товар, для которого предложения
}

func NewStudioHandlers(db *sql.DB, aiClient *ai.Client, aiModel string) *StudioHandlers {
	return &StudioHandlers{
		repo:    repository.NewGoodsRepository(db),
		ai:      aiClient,
		aiModel: aiModel,
	}
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
	Volume       *float64   `json:"volume"` // данные каталога (3b.6.4); NULL у draft
	Weight       *float64   `json:"weight"`
}

// ProducerTypeView — тип производителя в представлении состояния (спека
// 2026-09-20-фабрики §4.1 + дерево построек 2026-09-21 §1.2): карточка типа
// (имя, kind, категория, семейство рас, родитель, раса, вход/выход,
// параметры). Items — привязанные предметы (kind=items).
type ProducerTypeView struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	CategoryID   *int64          `json:"category_id"`
	CategoryName string          `json:"category_name,omitempty"`
	RaceFamily   string          `json:"race_family,omitempty"`
	ParentID     *int64          `json:"parent_id,omitempty"`
	ParentName   string          `json:"parent_name,omitempty"`
	Race         string          `json:"race,omitempty"`
	Output       json.RawMessage `json:"output"`
	Input        json.RawMessage `json:"input"`
	Params       json.RawMessage `json:"params"`
	Status       string          `json:"status"`
	Items        []ItemView      `json:"items,omitempty"`
}

// ItemView — предмет в представлении состояния (спека §4.2): единый
// справочник «что бывает»; экземпляры — в инвентаре, не здесь.
type ItemView struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	SlotType string          `json:"slot_type"`
	Status   string          `json:"status"`
	Unlocks  json.RawMessage `json:"unlocks"`
	Params   json.RawMessage `json:"params"`
}

// StateView — полное состояние для UI (спека §7, GET /studio/api/state).
// Поля fill (report/proposals/proposals_good_id) — аддитивны к iterA §7
// (спека iterC §5.3): существующие не меняются. ProducerTypes/Items —
// аддитивны (спека 2026-09-20-фабрики §4).
type StateView struct {
	Categories       []CategoryView     `json:"categories"`
	Goods            []GoodView         `json:"goods"`
	Banned           []GoodView         `json:"banned"`
	Unused           []GoodView         `json:"unused"`
	Warnings         []validate.Warning `json:"warnings"`
	Model            string             `json:"model"`
	Generating       bool               `json:"generating"`
	AutoRefreshMS    int                `json:"auto_refresh_ms"`
	Report           []string           `json:"report"`
	Proposals        []ProposalView     `json:"proposals"`
	ProposalsGoodID  string             `json:"proposals_good_id,omitempty"`
	ProducerTypes    []ProducerTypeView `json:"producer_types"`
	Items            []ItemView         `json:"items"`
}

// ProposalView — предложение ИИ для попапа (спека iterC §5.3): kind new/link,
// category_valid — производное; link_id/link_name — только для kind=link.
type ProposalView struct {
	Slot          int    `json:"slot"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	CategoryID    int64  `json:"category_id"`
	CategoryValid bool   `json:"category_valid"`
	Reason        string `json:"reason"`
	Kind          string `json:"kind"`
	LinkID        string `json:"link_id,omitempty"`
	LinkName      string `json:"link_name,omitempty"`
}

// --- GET /studio/api/state ---

// State — полное состояние каталога (снимок REPEATABLE READ, §8.1) +
// статус fill (generating/model/report/proposals, спека iterC §5.3).
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
	studioJSON(w, http.StatusOK, h.buildStateView(snap))
}

// buildStateView — StateView из снимка (banned новые сверху, unused —
// in-degree 0, не banned, kind=good; warnings — прогон валидаторов).
// Fill-поля — копии под fillMu (слайсы не мутировать извне).
func (h *StudioHandlers) buildStateView(snap *repository.CatalogSnapshot) StateView {
	byID := graph.ByID(snap.Goods)
	view := StateView{
		Categories:    make([]CategoryView, 0, len(snap.Categories)),
		Goods:         make([]GoodView, 0, len(snap.Goods)),
		Banned:        []GoodView{},
		Unused:        []GoodView{},
		Warnings:      validate.Validate(&model.State{SchemaVersion: model.SchemaVersion, Goods: snap.Goods}),
		ProducerTypes: make([]ProducerTypeView, 0, len(snap.ProducerTypes)),
		Items:         make([]ItemView, 0, len(snap.Items)),
	}
	h.fillMu.Lock()
	view.Model = h.aiModel
	view.Generating = h.fillGenerating
	view.Report = append([]string{}, h.fillReport...)
	view.Proposals = append([]ProposalView{}, h.fillProposals...)
	view.ProposalsGoodID = h.fillProposalsGoodID
	h.fillMu.Unlock()
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
			Volume:       g.Volume,
			Weight:       g.Weight,
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

	// Типы производителей + предметы (спека 2026-09-20-фабрики §4):
	// карточка типа — имя/kind/категория/семейство/вход/выход/параметры;
	// для kind=items — привязанные предметы (producer_items).
	catByID := make(map[int64]string, len(snap.Categories))
	for _, c := range snap.Categories {
		catByID[c.ID] = c.Name
	}
	itemByID := make(map[int64]ItemView, len(snap.Items))
	for _, it := range snap.Items {
		iv := ItemView{ID: it.ID, Name: it.Name, SlotType: it.SlotType, Status: it.Status}
		if it.Unlocks != nil {
			iv.Unlocks = json.RawMessage(it.Unlocks)
		}
		if it.Params != nil {
			iv.Params = json.RawMessage(it.Params)
		}
		itemByID[it.ID] = iv
		view.Items = append(view.Items, iv)
	}
	producerItems := make(map[int64][]ItemView, len(snap.ProducerItems))
	for _, pi := range snap.ProducerItems {
		if iv, ok := itemByID[pi.ItemID]; ok {
			producerItems[pi.ProducerTypeID] = append(producerItems[pi.ProducerTypeID], iv)
		}
	}
	// Имена родителей (дерево построек §1.2): parent_id → имя типа-родителя.
	prodNameByID := make(map[int64]string, len(snap.ProducerTypes))
	for _, p := range snap.ProducerTypes {
		prodNameByID[p.ID] = p.Name
	}
	for _, p := range snap.ProducerTypes {
		pv := ProducerTypeView{
			ID:     p.ID,
			Name:   p.Name,
			Kind:   p.Kind,
			Status: p.Status,
		}
		if p.CategoryID.Valid {
			id := p.CategoryID.Int64
			pv.CategoryID = &id
			pv.CategoryName = catByID[id]
		}
		if p.RaceFamily.Valid {
			pv.RaceFamily = p.RaceFamily.String
		}
		if p.ParentID.Valid {
			id := p.ParentID.Int64
			pv.ParentID = &id
			pv.ParentName = prodNameByID[id]
		}
		if p.Race.Valid {
			pv.Race = p.Race.String
		}
		if p.Output != nil {
			pv.Output = json.RawMessage(p.Output)
		}
		if p.Input != nil {
			pv.Input = json.RawMessage(p.Input)
		}
		if p.Params != nil {
			pv.Params = json.RawMessage(p.Params)
		}
		if items := producerItems[p.ID]; len(items) > 0 {
			pv.Items = items
		}
		view.ProducerTypes = append(view.ProducerTypes, pv)
	}
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
	case len(parts) == 2 && parts[1] == "fill":
		h.goodFill(w, r, id)
	case len(parts) == 3 && parts[1] == "slots":
		h.slot(w, r, id, parts[2])
	case len(parts) == 3 && parts[1] == "fill" && parts[2] == "apply":
		h.goodFillApply(w, r, id)
	case len(parts) == 3 && parts[1] == "fill" && parts[2] == "cancel":
		h.goodFillCancel(w, r, id)
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
			Name       *string  `json:"name"`
			CategoryID *int64   `json:"category_id"`
			Volume     *float64 `json:"volume"`
			Weight     *float64 `json:"weight"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		if err := h.repo.UpdateGood(id, body.Name, body.CategoryID, body.Volume, body.Weight); err != nil {
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

// --- типы производителей (спека 2026-09-20-фабрики §4.1) ---

// Producers — POST /studio/api/producers {name, kind, category_id, parent_id,
// race_family, race}: создание типа производителя (дерево построек §1.2:
// категория — только у подтипов kind=goods; parent_id — базовый тип;
// race → race_family по каталогу рас).
func (h *StudioHandlers) Producers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name       string  `json:"name"`
		Kind       string  `json:"kind"`
		CategoryID *int64  `json:"category_id"`
		ParentID   *int64  `json:"parent_id"`
		RaceFamily *string `json:"race_family"`
		Race       *string `json:"race"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	if err := validateRaceFamily(body.Race, body.RaceFamily); err != nil {
		writeCatalogErr(w, err)
		return
	}
	p, err := h.repo.CreateProducerType(body.Name, body.Kind, body.CategoryID, body.ParentID, body.RaceFamily, body.Race)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusCreated, producerTypeView(p, "", nil))
}

// ProducerByID — PUT/DELETE /studio/api/producers/{id} и под-пути
// (status, items, items/{itemId}).
func (h *StudioHandlers) ProducerByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/studio/api/producers/")
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
		h.producer(w, r, id)
	case len(parts) == 2 && parts[1] == "status":
		h.producerStatus(w, r, id)
	case len(parts) == 2 && parts[1] == "items":
		h.producerLinkItem(w, r, id)
	case len(parts) == 3 && parts[1] == "items":
		itemID, err := parseID(parts[2])
		if err != nil {
			studioErr(w, "не найдено", http.StatusNotFound)
			return
		}
		h.producerUnlinkItem(w, r, id, itemID)
	default:
		studioErr(w, "не найдено", http.StatusNotFound)
	}
}

// producer — PUT/DELETE /studio/api/producers/{id}.
func (h *StudioHandlers) producer(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name       *string `json:"name"`
			CategoryID *int64  `json:"category_id"`
			ParentID   **int64 `json:"parent_id"`
			RaceFamily *string `json:"race_family"`
			Race       *string `json:"race"`
			Output     *string `json:"output"`
			Input      *string `json:"input"`
			Params     *string `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		if err := validateRaceFamily(body.Race, body.RaceFamily); err != nil {
			writeCatalogErr(w, err)
			return
		}
		if err := h.repo.UpdateProducerType(id, body.Name, body.CategoryID, body.ParentID, body.RaceFamily, body.Race, body.Output, body.Input, body.Params); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"id": id})
	case http.MethodDelete:
		if err := h.repo.DeleteProducerType(id); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"deleted": id})
	default:
		studioErr(w, "только PUT/DELETE", http.StatusMethodNotAllowed)
	}
}

// producerStatus — POST /studio/api/producers/{id}/status {status}.
func (h *StudioHandlers) producerStatus(w http.ResponseWriter, r *http.Request, id int64) {
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
	if err := h.repo.SetProducerTypeStatus(id, body.Status); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "status": body.Status})
}

// producerLinkItem — POST /studio/api/producers/{id}/items {item_id}:
// привязать предмет к производителю (kind=items).
func (h *StudioHandlers) producerLinkItem(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ItemID       int64   `json:"item_id"`
		Requirements *string `json:"requirements"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	if body.ItemID <= 0 {
		studioErr(w, "item_id обязателен", http.StatusBadRequest)
		return
	}
	if err := h.repo.LinkProducerItem(id, body.ItemID, body.Requirements); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"producer_type_id": id, "item_id": body.ItemID})
}

// producerUnlinkItem — DELETE /studio/api/producers/{id}/items/{itemId}.
func (h *StudioHandlers) producerUnlinkItem(w http.ResponseWriter, r *http.Request, id, itemID int64) {
	if r.Method != http.MethodDelete {
		studioErr(w, "только DELETE", http.StatusMethodNotAllowed)
		return
	}
	if err := h.repo.UnlinkProducerItem(id, itemID); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"producer_type_id": id, "item_id": itemID})
}

// --- уровни расовости (дерево построек, спека 2026-09-21 §3/§5) ---

// RaceFamilyView — семейство рас для переключателя (спека §3): F1–F9 +
// robotic (в UI — «F10 Роботы»).
type RaceFamilyView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RaceView — раса для переключателя/попапа (спека §3): id/name из
// config/races.json, family из config/race_lore.json (единый источник —
// internal/races, студия файлы не знает).
type RaceView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Family string `json:"family"`
}

// RacesView — GET /studio/api/races: уровни расовости для переключателя.
type RacesView struct {
	Families []RaceFamilyView `json:"families"`
	Races    []RaceView       `json:"races"`
}

// raceFamilyNames — имена семейств (22_races.md §2.2/§4): F1–F9 + robotic.
var raceFamilyNames = map[string]string{
	"F1": "Водные", "F2": "Крио-аммиачные", "F3": "Метановые",
	"F4": "Серные", "F5": "Терморедокс", "F6": "Кремниевые",
	"F7": "Водородные/небесные", "F8": "Углекислые", "F9": "Экзотика",
	"robotic": "Роботы",
}

// Races — GET /studio/api/races: семейства (F1–F9 + robotic) и расы
// (id, name, family) из races.Catalog() + races.LoreCatalog() (Go-конфиги,
// единый источник; дублирования в БД нет — «один факт — одно место»).
func (h *StudioHandlers) Races(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		studioErr(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	famByID := make(map[string]string, len(races.LoreCatalog()))
	for _, l := range races.LoreCatalog() {
		famByID[l.ID] = l.Family
	}
	view := RacesView{Families: []RaceFamilyView{}, Races: []RaceView{}}
	for _, id := range []string{"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "robotic"} {
		view.Families = append(view.Families, RaceFamilyView{ID: id, Name: raceFamilyNames[id]})
	}
	for _, rc := range races.Catalog() {
		view.Races = append(view.Races, RaceView{ID: rc.ID, Name: rc.Name, Family: famByID[rc.ID]})
	}
	studioJSON(w, http.StatusOK, view)
}

// validateRaceFamily — инвариант §1.2 п.6: race задана → race_family задана
// и соответствует семейству расы по каталогу (config/race_lore.json,
// internal/races.LoreByID). Проверка в хендлере: каталог рас загружен при
// старте (cmd/server/main.go), репозиторий каталог не знает.
func validateRaceFamily(race, raceFamily *string) error {
	if race == nil || *race == "" {
		return nil
	}
	if raceFamily == nil || *raceFamily == "" {
		return &repository.ErrCatalog{Status: 400, Msg: "раса задана — семейство рас обязательно"}
	}
	lore := races.LoreByID(*race)
	if lore == nil {
		return &repository.ErrCatalog{Status: 400, Msg: "раса не найдена в каталоге"}
	}
	if lore.Family != *raceFamily {
		return &repository.ErrCatalog{Status: 400, Msg: "семейство рас не соответствует расе (ожидается " + lore.Family + ")"}
	}
	return nil
}

// --- предметы (спека 2026-09-20-фабрики §4.2) ---

// Items — POST /studio/api/items {name, slot_type}: создание предмета.
func (h *StudioHandlers) Items(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name     string `json:"name"`
		SlotType string `json:"slot_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	it, err := h.repo.CreateItem(body.Name, body.SlotType)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusCreated, itemView(it))
}

// ItemByID — PUT/DELETE /studio/api/items/{id} и под-пути (status).
func (h *StudioHandlers) ItemByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/studio/api/items/")
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
		h.item(w, r, id)
	case len(parts) == 2 && parts[1] == "status":
		h.itemStatus(w, r, id)
	default:
		studioErr(w, "не найдено", http.StatusNotFound)
	}
}

// item — PUT/DELETE /studio/api/items/{id}.
func (h *StudioHandlers) item(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name     *string `json:"name"`
			SlotType *string `json:"slot_type"`
			Unlocks  *string `json:"unlocks"`
			Params   *string `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		if err := h.repo.UpdateItem(id, body.Name, body.SlotType, body.Unlocks, body.Params); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"id": id})
	case http.MethodDelete:
		if err := h.repo.DeleteItem(id); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"deleted": id})
	default:
		studioErr(w, "только PUT/DELETE", http.StatusMethodNotAllowed)
	}
}

// itemStatus — POST /studio/api/items/{id}/status {status}.
func (h *StudioHandlers) itemStatus(w http.ResponseWriter, r *http.Request, id int64) {
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
	if err := h.repo.SetItemStatus(id, body.Status); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "status": body.Status})
}

// producerTypeView — ProducerTypeRow → ProducerTypeView (catName — имя
// категории, items — привязанные предметы; nil — не заполнять).
func producerTypeView(p repository.ProducerTypeRow, catName string, items []ItemView) ProducerTypeView {
	pv := ProducerTypeView{ID: p.ID, Name: p.Name, Kind: p.Kind, Status: p.Status, CategoryName: catName}
	if p.CategoryID.Valid {
		id := p.CategoryID.Int64
		pv.CategoryID = &id
	}
	if p.RaceFamily.Valid {
		pv.RaceFamily = p.RaceFamily.String
	}
	if p.ParentID.Valid {
		id := p.ParentID.Int64
		pv.ParentID = &id
	}
	if p.Race.Valid {
		pv.Race = p.Race.String
	}
	if p.Output != nil {
		pv.Output = json.RawMessage(p.Output)
	}
	if p.Input != nil {
		pv.Input = json.RawMessage(p.Input)
	}
	if p.Params != nil {
		pv.Params = json.RawMessage(p.Params)
	}
	if len(items) > 0 {
		pv.Items = items
	}
	return pv
}

// itemView — ItemRow → ItemView.
func itemView(it repository.ItemRow) ItemView {
	iv := ItemView{ID: it.ID, Name: it.Name, SlotType: it.SlotType, Status: it.Status}
	if it.Unlocks != nil {
		iv.Unlocks = json.RawMessage(it.Unlocks)
	}
	if it.Params != nil {
		iv.Params = json.RawMessage(it.Params)
	}
	return iv
}

// --- fill: «заполнить комплектующие» (спека iterC §5) ---

// goodFill — POST /studio/api/goods/{id}/fill: запуск ИИ (асинхронно).
// Ответ ИИ разбирается в предложения (proposals), НЕ применяется (двухфазный
// fill «предложи → подтверди в попапе», решение создателя 2026-09-20).
// 202 {"started":"true"} · 400 нет пустых слотов · 404 нет товара ·
// 409 уже идёт генерация (TryStart).
func (h *StudioHandlers) goodFill(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	snap, err := h.repo.Snapshot()
	if err != nil {
		studioErr(w, "ошибка чтения каталога: "+err.Error(), http.StatusInternalServerError)
		return
	}
	goodID := strconv.FormatInt(id, 10)
	gi := indexOfGood(snap.Goods, goodID)
	if gi < 0 {
		studioErr(w, "товар не найден", http.StatusNotFound)
		return
	}
	if len(emptySlots(snap.Goods[gi].Recipe)) == 0 {
		studioErr(w, "нет пустых слотов", http.StatusBadRequest)
		return
	}
	prompt := ai.BuildFillPrompt(snapshotState(snap), goodID)
	if !h.tryStartFill() {
		studioErr(w, "уже идёт генерация", http.StatusConflict)
		return
	}
	go h.runFill(goodID, prompt)
	studioJSON(w, http.StatusAccepted, map[string]string{"started": "true"})
}

// runFill — фоновая генерация (эталон state.go:743–759, изменён: разбор →
// proposals, НЕ применение): запрос к ИИ → разбор → BuildProposals на свежем
// снимке → setProposals + finishFill (дропы бана/ресурса/цикла — в отчёте
// сразу; proposals — в state для попапа).
func (h *StudioHandlers) runFill(goodID, prompt string) {
	raw, err := h.ai.FillComponents(prompt)
	if err != nil {
		h.finishFill([]string{"Ошибка ИИ: " + err.Error()})
		return
	}
	comps, err := ai.ParseFillResponse(raw)
	if err != nil {
		h.finishFill([]string{"Мусор в ответе ИИ: " + err.Error()})
		return
	}
	snap, err := h.repo.Snapshot()
	if err != nil {
		h.finishFill([]string{"Ошибка чтения каталога: " + err.Error()})
		return
	}
	proposals, dropReport := ai.BuildProposals(snapshotState(snap), goodID, comps)
	h.setProposals(proposalViews(proposals), goodID)
	h.finishFill(dropReport)
}

// proposalViews — ai.Proposal → ProposalView (state-представление, спека
// iterC §5.3: те же поля, JSON-теги).
func proposalViews(ps []ai.Proposal) []ProposalView {
	out := make([]ProposalView, 0, len(ps))
	for _, p := range ps {
		out = append(out, ProposalView{
			Slot: p.Slot, Name: p.Name, Category: p.Category,
			CategoryID: p.CategoryID, CategoryValid: p.CategoryValid,
			Reason: p.Reason, Kind: p.Kind,
			LinkID: p.LinkID, LinkName: p.LinkName,
		})
	}
	return out
}

// goodFillApply — POST /studio/api/goods/{id}/fill/apply {accepted: [{i,
// category_id}]}: применение принятых предложений (одна транзакция с
// advisory lock, repo.ApplyProposals). 200 {"applied": N} · 400 пустой/битый
// accepted · 409 нет предложений / идёт генерация · товар удалён между
// фазами → 200 + отчёт «товар не найден» (М2).
func (h *StudioHandlers) goodFillApply(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Accepted []struct {
			I          int    `json:"i"`
			CategoryID *int64 `json:"category_id"`
		} `json:"accepted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	goodID := strconv.FormatInt(id, 10)
	h.fillMu.Lock()
	if h.fillGenerating {
		h.fillMu.Unlock()
		studioErr(w, "идёт генерация", http.StatusConflict)
		return
	}
	if len(h.fillProposals) == 0 || h.fillProposalsGoodID != goodID {
		h.fillMu.Unlock()
		studioErr(w, "нет предложений", http.StatusConflict)
		return
	}
	proposals := append([]ProposalView{}, h.fillProposals...)
	h.fillMu.Unlock()

	if len(body.Accepted) == 0 {
		studioErr(w, "accepted пуст", http.StatusBadRequest)
		return
	}
	items := make([]ai.ProposalItem, 0, len(body.Accepted))
	for _, a := range body.Accepted {
		if a.I < 0 || a.I >= len(proposals) {
			studioErr(w, "индекс вне диапазона", http.StatusBadRequest)
			return
		}
		p := proposals[a.I]
		item := ai.ProposalItem{Slot: p.Slot, Name: p.Name, Reason: p.Reason, Kind: p.Kind}
		if p.Kind == "new" {
			// category_id обязателен для kind=new (выбор попапа); для link
			// игнорируется. Существование категории НЕ проверяется здесь —
			// только в транзакции под lock (С2, TOCTOU).
			if a.CategoryID == nil || *a.CategoryID <= 0 {
				studioErr(w, "category_id обязателен для нового товара", http.StatusBadRequest)
				return
			}
			item.CategoryID = strconv.FormatInt(*a.CategoryID, 10)
		}
		items = append(items, item)
	}

	applied, rep, err := h.repo.ApplyProposals(id, items)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	// отчёт: «пропущено: X (не принято)» для отклонённых пользователем +
	// результат repo.ApplyProposals + «Применено: N» (N — из ответа, М3)
	report := make([]string, 0, len(rep)+2)
	if rejected := len(proposals) - len(body.Accepted); rejected > 0 {
		report = append(report, fmt.Sprintf("пропущено: %d (не принято)", rejected))
	}
	report = append(report, rep...)
	report = append(report, fmt.Sprintf("Применено: %d", applied))
	h.finishFill(report)
	h.clearProposals()
	studioJSON(w, http.StatusOK, map[string]int{"applied": applied})
}

// goodFillCancel — POST /studio/api/goods/{id}/fill/cancel: сброс предложений
// (Отмена в попапе). 200 · 404 нет товара · 409 идёт генерация — отмена
// после завершения (М5).
func (h *StudioHandlers) goodFillCancel(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	h.fillMu.Lock()
	generating := h.fillGenerating
	h.fillMu.Unlock()
	if generating {
		studioErr(w, "идёт генерация — отмена после завершения", http.StatusConflict)
		return
	}
	snap, err := h.repo.Snapshot()
	if err != nil {
		studioErr(w, "ошибка чтения каталога: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if indexOfGood(snap.Goods, strconv.FormatInt(id, 10)) < 0 {
		studioErr(w, "товар не найден", http.StatusNotFound)
		return
	}
	h.clearProposals()
	studioJSON(w, http.StatusOK, map[string]string{"cancelled": "true"})
}

// tryStartFill — атомарный старт генерации (TryStart, спека iterC §5.2);
// сбрасывает предыдущие proposals (повторный fill — новые предложения).
func (h *StudioHandlers) tryStartFill() bool {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	if h.fillGenerating {
		return false
	}
	h.fillGenerating = true
	h.fillReport = nil
	h.fillProposals = nil
	h.fillProposalsGoodID = ""
	return true
}

// finishFill — завершение генерации (успех или ошибка).
func (h *StudioHandlers) finishFill(report []string) {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	h.fillGenerating = false
	h.fillReport = report
}

// setProposals — запись предложений под fillMu (из runFill).
func (h *StudioHandlers) setProposals(proposals []ProposalView, goodID string) {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	h.fillProposals = proposals
	h.fillProposalsGoodID = goodID
}

// clearProposals — сброс предложений (apply/cancel; новый fill — tryStartFill).
func (h *StudioHandlers) clearProposals() {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	h.fillProposals = nil
	h.fillProposalsGoodID = ""
}

// snapshotState — model.State из снимка каталога (для BuildFillPrompt/
// BuildProposals): категории с kind (С2-проверка в ApplyProposals).
func snapshotState(snap *repository.CatalogSnapshot) *model.State {
	st := &model.State{
		SchemaVersion: model.SchemaVersion,
		Categories:    make([]model.Category, 0, len(snap.Categories)),
		Goods:         snap.Goods,
	}
	for _, c := range snap.Categories {
		st.Categories = append(st.Categories, model.Category{
			ID: strconv.FormatInt(c.ID, 10), Name: c.Name, Kind: model.Kind(c.Kind),
		})
	}
	return st
}

// emptySlots — индексы пустых слотов рецепта (0-based).
func emptySlots(recipe []model.Slot) []int {
	var out []int
	for i := range recipe {
		if recipe[i].GoodID == "" {
			out = append(out, i)
		}
	}
	return out
}

// indexOfGood — индекс товара по id (для fill-хендлеров).
func indexOfGood(goods []model.Good, id string) int {
	for i := range goods {
		if goods[i].ID == id {
			return i
		}
	}
	return -1
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