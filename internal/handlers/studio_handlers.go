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
	"strconv"
	"strings"
	"sync"

	"zorion/internal/goodsstudio/ai"
	"zorion/internal/goodsstudio/aiserve"
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
	repo     *repository.GoodsRepository
	deposits *repository.DepositRepository
	branches *repository.BranchRepository
	effects  *repository.EffectRepository
	ai       *ai.Client
	aiModel  string
	// aiServe — управление локальным ИИ-помощником (спека 2026-09-24 §3);
	// nil — состояние «недоступен» (тесты/выключено).
	aiServe aiController
	// contentPath — путь файла-снимка контента (спека 2026-09-24 §4/§5):
	// дефолт content/catalog.json; переопределяется SetContentExportPath
	// (env CONTENT_CATALOG_PATH).
	contentPath string

	fillMu              sync.Mutex
	fillGenerating      bool // любой ИИ-джоб студии (И6): fill ИЛИ описания
	fillReport          []string
	fillProposals       []ProposalView // предложения последнего завершённого fill
	fillProposalsGoodID string         // товар, для которого предложения

	// канал описаний (спека 2026-09-21-каталог-описание §7.2): отдельные
	// предложения/прогресс; descGenerating отличает джоб описаний от fill.
	descGenerating bool
	descReport     []string
	descProposals  []DescProposalView
	descTotal      int
	descDone       int
	descCancel     bool
	// descExplicit — режим джоба описаний (И4): false — пакетный
	// (scope:"missing", onlyIfEmpty=true), true — явный (good_ids, переописание
	// разрешено). Хранится рядом с предложениями, сбрасывается вместе с ними.
	descExplicit bool
}

func NewStudioHandlers(db *sql.DB, aiClient *ai.Client, aiModel string) *StudioHandlers {
	return &StudioHandlers{
		repo:        repository.NewGoodsRepository(db),
		deposits:    repository.NewDepositRepository(db),
		branches:    repository.NewBranchRepository(db),
		effects:     repository.NewEffectRepository(db),
		ai:          aiClient,
		aiModel:     aiModel,
		contentPath: "content/catalog.json",
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

// SlotView — слот в представлении состояния (имя/тир разрешены).
type SlotView struct {
	GoodID        string `json:"good_id"`
	Name          string `json:"name"`
	Tier          int    `json:"tier"`
	Quantity      int    `json:"quantity"`
	Reason        string `json:"reason,omitempty"`
	AllowResource bool   `json:"allow_resource"`
}

// GoodView — товар/ресурс в представлении состояния. Статуса и скрытия у
// товара/ресурса нет (спека 2026-09-21 §4.2): «убрать» — только удаление.
// Рецепт как сущность (спека 2026-09-21-рецепт-сущность §5): recipe_id/
// complexity/bound_factories; tier_override удалён (сложность — у рецепта).
type GoodView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CategoryID   int64  `json:"category_id"`
	Kind         string `json:"kind"`
	Source       string `json:"source"`
	Tier         int    `json:"tier"`          // эффективный (complexity ?? вычисленный)
	TierComputed int    `json:"tier_computed"` // вычисленный (graph.Tier)
	// RecipeID — id рецепта (recipes.id; 0 у ресурса — рецепта нет).
	RecipeID int64 `json:"recipe_id"`
	// Complexity — сложность рецепта (null = вычисляется по графу).
	Complexity *int `json:"complexity"`
	// BoundFactories — фабрики, держащие рецепт (producer_recipes, обратная
	// привязка): маркер «в наборе фабрики»/«не привязан».
	BoundFactories []int64    `json:"bound_factories"`
	Recipe         []SlotView `json:"recipe"`
	Volume         *float64   `json:"volume"` // данные каталога (3b.6.4); значение есть всегда (Р2)
	Weight         *float64   `json:"weight"`
	// Description — описание каталога; в ответе всегда есть, NULL в БД → "" (И3).
	Description string `json:"description"`
	// Code — метка переноса (goods.code, спека 2026-09-24 §3.1/§3.3): справочная,
	// только чтение; NULL в БД → поле отсутствует (omitempty).
	Code string `json:"code,omitempty"`
}

// RecipeBindingView — привязка рецепта к фабрике (producer_recipes) в
// представлении состояния (спека 2026-09-21-рецепт-сущность §5,
// верхнеуровневый producer_recipes).
type RecipeBindingView struct {
	ProducerTypeID int64 `json:"producer_type_id"`
	RecipeID       int64 `json:"recipe_id"`
	GoodID         int64 `json:"good_id"`
	// Rate — число скорости пары, ед/сутки/млрд (спека 2026-09-23 §10);
	// null = «не объявлено» → постройка не производит.
	Rate *float64 `json:"rate"`
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
	Hidden       bool            `json:"hidden"`
	Items        []ItemView      `json:"items,omitempty"`
	// Code — метка переноса (producer_types.code, спека 2026-09-24 §3.1/§3.3):
	// справочная, только чтение; NULL в БД → поле отсутствует (omitempty).
	Code string `json:"code,omitempty"`
}

// ProducerSlotView — слот родителя в представлении состояния (спека скрытых
// §4): id/parent/category/расовость/hidden; имена родителя и категории — из
// снимка (как parent_name/category_name у записей). «Унаследован/переопределён»
// считает клиент по уровню расовости.
type ProducerSlotView struct {
	ID           int64  `json:"id"`
	ParentID     int64  `json:"parent_id"`
	ParentName   string `json:"parent_name,omitempty"`
	CategoryID   int64  `json:"category_id"`
	CategoryName string `json:"category_name,omitempty"`
	RaceFamily   string `json:"race_family,omitempty"`
	Race         string `json:"race,omitempty"`
	Hidden       bool   `json:"hidden"`
}

// ItemView — предмет в представлении состояния (спека §4.2): единый
// справочник «что бывает»; экземпляры — в инвентаре, не здесь.
type ItemView struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	SlotType string          `json:"slot_type"`
	Unlocks  json.RawMessage `json:"unlocks"`
	Params   json.RawMessage `json:"params"`
	// Code — метка переноса (items.code, спека 2026-09-24 §3.1/§3.3):
	// справочная, только чтение; NULL в БД → поле отсутствует (omitempty).
	Code string `json:"code,omitempty"`
}

// StateView — полное состояние для UI (спека §7, GET /studio/api/state).
// Поля fill (report/proposals/proposals_good_id) — аддитивны к iterA §7
// (спека iterC §5.3): существующие не меняются. ProducerTypes/Items —
// аддитивны (спека 2026-09-20-фабрики §4).
type StateView struct {
	Categories      []CategoryView      `json:"categories"`
	Goods           []GoodView          `json:"goods"`
	Unused          []GoodView          `json:"unused"`
	Warnings        []validate.Warning  `json:"warnings"`
	Model           string              `json:"model"`
	Generating      bool                `json:"generating"`
	AutoRefreshMS   int                 `json:"auto_refresh_ms"`
	Report          []string            `json:"report"`
	Proposals       []ProposalView      `json:"proposals"`
	ProposalsGoodID string              `json:"proposals_good_id,omitempty"`
	DescGenerating  bool                `json:"desc_generating"`
	DescReport      []string            `json:"desc_report"`
	DescProposals   []DescProposalView  `json:"desc_proposals"`
	DescTotal       int                 `json:"desc_total"`
	DescDone        int                 `json:"desc_done"`
	ProducerTypes   []ProducerTypeView  `json:"producer_types"`
	Items           []ItemView          `json:"items"`
	ProducerSlots   []ProducerSlotView  `json:"producer_slots"`
	ProducerRecipes []RecipeBindingView `json:"producer_recipes"`
	// AI — состояние локального ИИ-помощника (спека 2026-09-24 §3/§4):
	// аддитивно, обновляется существующим циклом fetchState (~3 с).
	AI aiserve.Status `json:"ai"`
}

// DescProposalView — предложение описания для попапа «Описания ИИ» (спека
// 2026-09-21-каталог-описание §7.2/§9.4): одна строка на запись каталога.
type DescProposalView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Text string `json:"text"`
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
	// Description — игровое описание нового товара (спека
	// 2026-09-21-каталог-описание §8.1); только kind=new.
	Description string `json:"description,omitempty"`
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

// buildStateView — StateView из снимка (unused — in-degree 0, kind=good;
// warnings — прогон валидаторов). Fill-поля — копии под fillMu (слайсы не
// мутировать извне).
func (h *StudioHandlers) buildStateView(snap *repository.CatalogSnapshot) StateView {
	byID := graph.ByID(snap.Goods)
	bindings := toModelBindings(snap.Bindings)
	view := StateView{
		Categories:    make([]CategoryView, 0, len(snap.Categories)),
		Goods:         make([]GoodView, 0, len(snap.Goods)),
		Unused:        []GoodView{},
		Warnings:      validate.Validate(&model.State{SchemaVersion: model.SchemaVersion, Goods: snap.Goods, Bindings: bindings}),
		ProducerTypes: make([]ProducerTypeView, 0, len(snap.ProducerTypes)),
		Items:         make([]ItemView, 0, len(snap.Items)),
	}
	h.fillMu.Lock()
	view.Model = h.aiModel
	view.Generating = h.fillGenerating
	view.Report = append([]string{}, h.fillReport...)
	view.Proposals = append([]ProposalView{}, h.fillProposals...)
	view.ProposalsGoodID = h.fillProposalsGoodID
	view.DescGenerating = h.descGenerating
	view.DescReport = append([]string{}, h.descReport...)
	view.DescProposals = append([]DescProposalView{}, h.descProposals...)
	view.DescTotal = h.descTotal
	view.DescDone = h.descDone
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
	// обратная привязка рецептов: good_id → [producer_type_id] (спека
	// 2026-09-21-рецепт-сущность §5) — маркер «в наборе фабрики»/«не привязан».
	boundByGood := make(map[int64][]int64, len(snap.Bindings))
	for _, b := range snap.Bindings {
		boundByGood[b.GoodID] = append(boundByGood[b.GoodID], b.ProducerTypeID)
	}
	for i := range snap.Goods {
		g := &snap.Goods[i]
		catID, _ := strconv.ParseInt(g.Category, 10, 64)
		gID, _ := strconv.ParseInt(g.ID, 10, 64)
		bound := boundByGood[gID]
		if bound == nil {
			bound = []int64{}
		}
		gv := GoodView{
			ID:             g.ID,
			Name:           g.Name,
			CategoryID:     catID,
			Kind:           string(g.Kind),
			Source:         string(g.Source),
			Tier:           graph.EffectiveTier(g, byID),
			TierComputed:   graph.Tier(g, byID),
			RecipeID:       g.RecipeID,
			Complexity:     g.Complexity,
			BoundFactories: bound,
			Volume:         g.Volume,
			Weight:         g.Weight,
			Description:    g.Description,
			Code:           g.Code,
			Recipe:         []SlotView{},
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
			}
			gv.Recipe = append(gv.Recipe, sv)
		}
		view.Goods = append(view.Goods, gv)
		if g.Kind != model.KindResource && inDegree[g.ID] == 0 {
			view.Unused = append(view.Unused, gv)
		}
	}

	// Типы производителей + предметы (спека 2026-09-20-фабрики §4):
	// карточка типа — имя/kind/категория/семейство/вход/выход/параметры;
	// для kind=items — привязанные предметы (producer_items).
	catByID := make(map[int64]string, len(snap.Categories))
	for _, c := range snap.Categories {
		catByID[c.ID] = c.Name
	}
	itemByID := make(map[int64]ItemView, len(snap.Items))
	for _, it := range snap.Items {
		iv := ItemView{ID: it.ID, Name: it.Name, SlotType: it.SlotType, Code: it.Code.String}
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
			Hidden: p.Hidden,
			Code:   p.Code.String,
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
	// Слоты родителя (спека скрытых §4): все слоты каталога с именами
	// родителя/категории из снимка.
	view.ProducerSlots = make([]ProducerSlotView, 0, len(snap.ProducerSlots))
	for _, s := range snap.ProducerSlots {
		sv := ProducerSlotView{
			ID:         s.ID,
			ParentID:   s.ParentID,
			ParentName: prodNameByID[s.ParentID],
			CategoryID: s.CategoryID,
			Hidden:     s.Hidden,
		}
		if name, ok := catByID[s.CategoryID]; ok {
			sv.CategoryName = name
		}
		if s.RaceFamily.Valid {
			sv.RaceFamily = s.RaceFamily.String
		}
		if s.Race.Valid {
			sv.Race = s.Race.String
		}
		view.ProducerSlots = append(view.ProducerSlots, sv)
	}
	// Привязки рецептов к фабрикам (спека 2026-09-21-рецепт-сущность §5):
	// верхнеуровневый producer_recipes для селектора/фильтра семейства/маркеров.
	view.ProducerRecipes = make([]RecipeBindingView, 0, len(snap.Bindings))
	for _, b := range snap.Bindings {
		view.ProducerRecipes = append(view.ProducerRecipes, RecipeBindingView{
			ProducerTypeID: b.ProducerTypeID, RecipeID: b.RecipeID, GoodID: b.GoodID, Rate: b.Rate,
		})
	}
	// Состояние ИИ-помощника (спека 2026-09-24 §3/§4) — по факту пробы.
	view.AI = h.aiStatus()
	return view
}

// toModelBindings — RecipeBindingRow → model.RecipeBinding (для validator).
func toModelBindings(rows []repository.RecipeBindingRow) []model.RecipeBinding {
	out := make([]model.RecipeBinding, 0, len(rows))
	for _, b := range rows {
		out = append(out, model.RecipeBinding{
			RecipeID: b.RecipeID, ProducerTypeID: b.ProducerTypeID, GoodID: b.GoodID,
		})
	}
	return out
}

// --- GET /studio/api/resources ---

// Resources — палитра ресурсов (kind=resource).
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
		ID:             g.ID,
		Name:           g.Name,
		CategoryID:     body.CategoryID,
		Kind:           string(g.Kind),
		Source:         string(g.Source),
		Tier:           0,
		TierComputed:   0,
		RecipeID:       g.RecipeID,
		Complexity:     g.Complexity,
		BoundFactories: []int64{},
		Description:    g.Description,
		Recipe:         []SlotView{},
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

// GoodByID — PUT/DELETE /studio/api/goods/{id} и под-пути fill/fill-apply/
// fill-cancel. Статусного роута у goods нет (спека §4.2). Слоты рецепта и
// тир переехали на /studio/api/recipes* (спека 2026-09-21-рецепт-сущность §5:
// одна модель — один адрес), у goods этих под-путей больше нет.
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
	case len(parts) == 2 && parts[1] == "deposits-count":
		h.goodDepositsCount(w, r, id)
	case len(parts) == 2 && parts[1] == "branches-count":
		h.goodBranchesCount(w, r, id)
	case len(parts) == 2 && parts[1] == "fill":
		h.goodFill(w, r, id)
	case len(parts) == 3 && parts[1] == "fill" && parts[2] == "apply":
		h.goodFillApply(w, r, id)
	case len(parts) == 3 && parts[1] == "fill" && parts[2] == "cancel":
		h.goodFillCancel(w, r, id)
	default:
		studioErr(w, "не найдено", http.StatusNotFound)
	}
}

// goodDepositsCount — GET /studio/api/goods/{id}/deposits-count (§3.3/T14):
// предпроверка числа залежей ресурса во всех мирах перед удалением — студия
// показывает счётчик до подтверждения (UI — фронтенд-задача). Ответ {"count": N}.
// Один источник числа — DepositRepository.CountDepositsByGood (тем же SQL
// считает DeleteGood). JWT admin/skycomposer — на роуте (main.go).
func (h *StudioHandlers) goodDepositsCount(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		studioErr(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	n, err := h.deposits.CountDepositsByGood(id)
	if err != nil {
		studioErr(w, "ошибка чтения залежей: "+err.Error(), http.StatusInternalServerError)
		return
	}
	studioJSON(w, http.StatusOK, map[string]int{"count": n})
}

// goodBranchesCount — GET /studio/api/goods/{id}/branches-count (§8/О3
// итерации 2): предпроверка числа веток поселений, у которых товар — выход
// рецепта, перед удалением (каскад recipes → settlement_branches). Ответ
// {"count": N}. Один источник числа — BranchRepository.CountBranchesByGood
// (тем же SQL считает DeleteGood). JWT admin/skycomposer — на роуте (main.go).
func (h *StudioHandlers) goodBranchesCount(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		studioErr(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	n, err := h.branches.CountBranchesByGood(id)
	if err != nil {
		studioErr(w, "ошибка чтения веток: "+err.Error(), http.StatusInternalServerError)
		return
	}
	studioJSON(w, http.StatusOK, map[string]int{"count": n})
}

// good — PUT/DELETE /studio/api/goods/{id} (для ресурсов разрешено, С1).
func (h *StudioHandlers) good(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name        *string  `json:"name"`
			CategoryID  *int64   `json:"category_id"`
			Volume      *float64 `json:"volume"`
			Weight      *float64 `json:"weight"`
			Description *string  `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		if err := h.repo.UpdateGood(id, body.Name, body.CategoryID, body.Volume, body.Weight, body.Description); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"id": id})
	case http.MethodDelete:
		res, err := h.repo.DeleteGood(id)
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]interface{}{
			"deleted":       id,
			"cleared_links": res.ClearedLinks,
			"deposits":      res.Deposits,
			"branches":      res.Branches,
		})
	default:
		studioErr(w, "только PUT/DELETE", http.StatusMethodNotAllowed)
	}
}

// --- типы производителей (спека 2026-09-20-фабрики §4.1) ---

// Producers — POST /studio/api/producers {name, kind, category_id, parent_id,
// race_family, race}: создание типа производителя (дерево построек §1.2:
// категория — только у подтипов kind=goods; parent_id — базовый тип;
// race → race_family по каталогу рас; слот-инвариант С4 — подтип kind=goods
// требует применяемого слота родителя, §1.4 п.1 спеки скрытых).
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
// (hidden, items, items/{itemId}, recipes, recipes/{recipe_id} —
// DELETE отвязка / PUT число скорости, recipes/copy-universal).
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
	case len(parts) == 2 && parts[1] == "hidden":
		h.producerHidden(w, r, id)
	case len(parts) == 2 && parts[1] == "items":
		h.producerLinkItem(w, r, id)
	case len(parts) == 2 && parts[1] == "recipes":
		h.producerBindRecipe(w, r, id)
	case len(parts) == 3 && parts[1] == "items":
		itemID, err := parseID(parts[2])
		if err != nil {
			studioErr(w, "не найдено", http.StatusNotFound)
			return
		}
		h.producerUnlinkItem(w, r, id, itemID)
	case len(parts) == 3 && parts[1] == "recipes" && parts[2] == "copy-universal":
		h.producerCopyUniversal(w, r, id)
	case len(parts) == 3 && parts[1] == "recipes":
		recipeID, err := parseID(parts[2])
		if err != nil {
			studioErr(w, "не найдено", http.StatusNotFound)
			return
		}
		h.producerRecipeByID(w, r, id, recipeID)
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
			// Типизированные поля редактора стадии (спека 2026-09-23 §10):
			// eat — позиция → норма (ед/сутки/млрд), effects — позиция → имя
			// типа эффекта, stage — пороги {enter, exit}. Всё ложится в params.
			Eat     *map[string]float64 `json:"eat"`
			Effects *map[string]string  `json:"effects"`
			Stage   *stageParamsInput   `json:"stage"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		if err := validateRaceFamily(body.Race, body.RaceFamily); err != nil {
			writeCatalogErr(w, err)
			return
		}
		// Приоритет полей (M6, §10): при типизированных полях побеждают они —
		// строка params участвует лишь как источник остальных ключей.
		paramsArg := body.Params
		var warnings []string
		if body.Eat != nil || body.Effects != nil || body.Stage != nil {
			merged, warns, err := h.buildTypedProducerParams(id, body.Params, body.Eat, body.Effects, body.Stage)
			if err != nil {
				writeCatalogErr(w, err)
				return
			}
			paramsArg = &merged
			warnings = warns
		}
		if err := h.repo.UpdateProducerType(id, body.Name, body.CategoryID, body.ParentID, body.RaceFamily, body.Race, body.Output, body.Input, paramsArg); err != nil {
			writeCatalogErr(w, err)
			return
		}
		resp := map[string]interface{}{"id": id}
		if len(warnings) > 0 {
			resp["warnings"] = warnings
		}
		studioJSON(w, http.StatusOK, resp)
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

// producerHidden — POST /studio/api/producers/{id}/hidden {hidden}: обратимое
// скрытие записи-фабрики (единственный носитель скрытия, спека 2026-09-21 §4.2).
func (h *StudioHandlers) producerHidden(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Hidden *bool `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	if body.Hidden == nil {
		studioErr(w, "hidden обязателен", http.StatusBadRequest)
		return
	}
	if err := h.repo.SetProducerHidden(id, *body.Hidden); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "hidden": *body.Hidden})
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

// --- слоты родителя (спека 2026-09-21-скрытые §4) ---

// Slots — POST /studio/api/slots {parent_id, category_id, race_family, race,
// hidden?}: создать слот родителя (переопределение уровня / база на
// универсальном). Валидации §1.4 п.3: родитель — тип kind=goods (400);
// категория существует (400); race→family (400); дубликат кортежа NULL-safe
// (409).
func (h *StudioHandlers) Slots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ParentID   int64   `json:"parent_id"`
		CategoryID int64   `json:"category_id"`
		RaceFamily *string `json:"race_family"`
		Race       *string `json:"race"`
		Hidden     *bool   `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	if body.ParentID <= 0 || body.CategoryID <= 0 {
		studioErr(w, "parent_id и category_id обязательны", http.StatusBadRequest)
		return
	}
	if err := validateRaceFamily(body.Race, body.RaceFamily); err != nil {
		writeCatalogErr(w, err)
		return
	}
	s, err := h.repo.CreateProducerSlot(body.ParentID, body.CategoryID, body.RaceFamily, body.Race, body.Hidden != nil && *body.Hidden)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusCreated, producerSlotView(s, "", ""))
}

// SlotByID — PUT/DELETE /studio/api/slots/{id}.
func (h *StudioHandlers) SlotByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(strings.TrimPrefix(r.URL.Path, "/studio/api/slots/"))
	if err != nil {
		studioErr(w, "не найдено", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Hidden *bool `json:"hidden"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		if body.Hidden == nil {
			studioErr(w, "hidden обязателен", http.StatusBadRequest)
			return
		}
		if err := h.repo.UpdateProducerSlotHidden(id, *body.Hidden); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "hidden": *body.Hidden})
	case http.MethodDelete:
		if err := h.repo.DeleteProducerSlot(id); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]int64{"deleted": id})
	default:
		studioErr(w, "только PUT/DELETE", http.StatusMethodNotAllowed)
	}
}

// producerSlotView — ProducerSlotRow → ProducerSlotView (catName/parentName —
// имена из снимка; пустые — не заполнять).
func producerSlotView(s repository.ProducerSlotRow, catName, parentName string) ProducerSlotView {
	sv := ProducerSlotView{
		ID:           s.ID,
		ParentID:     s.ParentID,
		ParentName:   parentName,
		CategoryID:   s.CategoryID,
		CategoryName: catName,
		Hidden:       s.Hidden,
	}
	if s.RaceFamily.Valid {
		sv.RaceFamily = s.RaceFamily.String
	}
	if s.Race.Valid {
		sv.Race = s.Race.String
	}
	return sv
}

// --- уровни расовости (дерево построек, спека 2026-09-21 §3/§5) ---

// RaceFamilyView — семейство рас для переключателя (спека §3): F0–F9 +
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

// raceFamilyNames — имена семейств (22_races.md §2.2/§4): F0–F9 + robotic.
var raceFamilyNames = map[string]string{
	"F0": "Люди", "F1": "Водные", "F2": "Крио-аммиачные", "F3": "Метановые",
	"F4": "Серные", "F5": "Терморедокс", "F6": "Кремниевые",
	"F7": "Водородные/небесные", "F8": "Углекислые", "F9": "Экзотика",
	"robotic": "Роботы",
}

// Races — GET /studio/api/races: семейства (F0–F9 + robotic) и расы
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
	for _, id := range []string{"F0", "F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "robotic"} {
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

// ItemByID — PUT/DELETE /studio/api/items/{id}. Статусного роута у items нет
// (спека 2026-09-21 §4.2).
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

// producerTypeView — ProducerTypeRow → ProducerTypeView (catName — имя
// категории, items — привязанные предметы; nil — не заполнять).
func producerTypeView(p repository.ProducerTypeRow, catName string, items []ItemView) ProducerTypeView {
	pv := ProducerTypeView{ID: p.ID, Name: p.Name, Kind: p.Kind, Hidden: p.Hidden, CategoryName: catName}
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
	iv := ItemView{ID: it.ID, Name: it.Name, SlotType: it.SlotType}
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
	raw, err := h.ai.Ask(prompt)
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
			Description: p.Description,
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
		item := ai.ProposalItem{Slot: p.Slot, Name: p.Name, Reason: p.Reason, Kind: p.Kind, Description: p.Description}
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

// --- описания каталога: «Описания ИИ» (спека 2026-09-21-каталог-описание §7.3) ---

// descBatchSize — порция записей на один запрос ИИ (спека §8.4).
const descBatchSize = 10

// Descriptions — POST /studio/api/descriptions/{fill|apply|cancel}.
func (h *StudioHandlers) Descriptions(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/studio/api/descriptions/")
	switch rest {
	case "fill":
		h.descriptionsFill(w, r)
	case "apply":
		h.descriptionsApply(w, r)
	case "cancel":
		h.descriptionsCancel(w, r)
	default:
		studioErr(w, "не найдено", http.StatusNotFound)
	}
}

// descriptionsFill — POST /studio/api/descriptions/fill {scope:"missing"} |
// {good_ids:[...]}: старт джоба описаний (асинхронно). 202 {"started","total"} ·
// 400 (пустая цель / несуществующий id / оба поля / неизвестный scope) ·
// 409 уже идёт генерация (общий флаг И6).
func (h *StudioHandlers) descriptionsFill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Scope   string   `json:"scope"`
		GoodIDs []string `json:"good_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	hasScope := body.Scope != ""
	hasIDs := body.GoodIDs != nil
	if hasScope == hasIDs {
		studioErr(w, "укажите scope или good_ids", http.StatusBadRequest)
		return
	}
	snap, err := h.repo.Snapshot()
	if err != nil {
		studioErr(w, "ошибка чтения каталога: "+err.Error(), http.StatusInternalServerError)
		return
	}
	catName := make(map[int64]string, len(snap.Categories))
	for _, c := range snap.Categories {
		catName[c.ID] = c.Name
	}
	var targets []ai.DescTarget
	var targetIDs []int64
	if hasScope {
		if body.Scope != "missing" {
			studioErr(w, "неизвестный scope", http.StatusBadRequest)
			return
		}
		for i := range snap.Goods {
			g := &snap.Goods[i]
			if strings.TrimSpace(g.Description) != "" {
				continue // И4: записи с непустым описанием — не цель
			}
			id, _ := strconv.ParseInt(g.ID, 10, 64)
			targetIDs = append(targetIDs, id)
			targets = append(targets, descTarget(g, catName))
		}
		if len(targets) == 0 {
			studioErr(w, "нет записей без описания", http.StatusBadRequest)
			return
		}
	} else {
		for _, sid := range body.GoodIDs {
			id, err := parseID(sid)
			if err != nil {
				studioErr(w, "запись не найдена: "+sid, http.StatusBadRequest)
				return
			}
			gi := indexOfGood(snap.Goods, strconv.FormatInt(id, 10))
			if gi < 0 {
				studioErr(w, "запись не найдена: "+sid, http.StatusBadRequest)
				return
			}
			g := &snap.Goods[gi]
			targetIDs = append(targetIDs, id)
			targets = append(targets, descTarget(g, catName))
		}
		if len(targets) == 0 {
			studioErr(w, "good_ids пуст", http.StatusBadRequest)
			return
		}
	}
	if !h.tryStartDesc(len(targets), hasIDs) {
		studioErr(w, "уже идёт генерация", http.StatusConflict)
		return
	}
	go h.runDescriptions(targets, targetIDs)
	studioJSON(w, http.StatusAccepted, map[string]interface{}{"started": "true", "total": len(targets)})
}

// descTarget — DescTarget из записи каталога (категория — имя из снимка).
func descTarget(g *model.Good, catName map[int64]string) ai.DescTarget {
	catID, _ := strconv.ParseInt(g.Category, 10, 64)
	return ai.DescTarget{Name: g.Name, Category: catName[catID], Kind: string(g.Kind)}
}

// runDescriptions — фоновая генерация описаний порциями по descBatchSize
// (спека §8.4): один запрос за раз (И6); ошибка/мусор порции — строка в отчёт
// и идём дальше; прогресс desc_done += len(порции); предложения добавляются по
// мере готовности; между порциями проверяется флаг остановки (И7: предложения
// сохраняются).
func (h *StudioHandlers) runDescriptions(targets []ai.DescTarget, ids []int64) {
	var report []string
	for start := 0; start < len(targets); start += descBatchSize {
		if h.descCancelled() {
			done, total := h.descProgress()
			report = append(report, fmt.Sprintf("остановлено: %d из %d", done, total))
			h.finishDesc(report)
			return
		}
		end := start + descBatchSize
		if end > len(targets) {
			end = len(targets)
		}
		batch := targets[start:end]
		batchIDs := ids[start:end]
		part := start/descBatchSize + 1
		raw, err := h.ai.Ask(ai.BuildDescriptionPrompt(batch))
		if err != nil {
			report = append(report, fmt.Sprintf("порция %d: ошибка ИИ: %s", part, err.Error()))
			h.addDescProgress(nil, len(batch))
			continue
		}
		items, err := ai.ParseDescriptionResponse(raw)
		if err != nil {
			report = append(report, fmt.Sprintf("порция %d: мусор в ответе ИИ", part))
			h.addDescProgress(nil, len(batch))
			continue
		}
		byName := make(map[string]string, len(items))
		for _, it := range items {
			byName[graph.NormalizeName(it.Name)] = it.Description
		}
		var props []DescProposalView
		for i, t := range batch {
			text := ai.NormalizeDescription(byName[graph.NormalizeName(t.Name)])
			if text == "" {
				report = append(report, fmt.Sprintf("нет описания: %s — пропущено", t.Name))
				continue
			}
			props = append(props, DescProposalView{ID: batchIDs[i], Name: t.Name, Kind: t.Kind, Text: text})
		}
		h.addDescProgress(props, len(batch))
	}
	h.finishDesc(report)
}

// descriptionsApply — POST /studio/api/descriptions/apply {accepted:[{i,text}]}:
// применение принятых пунктов (одна транзакция repo.UpdateDescriptions).
// 400 невалидный JSON / accepted пуст / индекс вне диапазона · 409 идёт ИИ-джоб
// / нет предложений · 200 {"applied":N}. Пункт с пустым/пробельным text не
// применяется («пустой текст — пропущено»). Предложения после применения
// очищаются.
func (h *StudioHandlers) descriptionsApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Accepted []struct {
			I    int    `json:"i"`
			Text string `json:"text"`
		} `json:"accepted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	h.fillMu.Lock()
	if h.fillGenerating {
		h.fillMu.Unlock()
		studioErr(w, "идёт генерация", http.StatusConflict)
		return
	}
	if len(h.descProposals) == 0 {
		h.fillMu.Unlock()
		studioErr(w, "нет предложений", http.StatusConflict)
		return
	}
	proposals := append([]DescProposalView{}, h.descProposals...)
	onlyIfEmpty := !h.descExplicit // И4: пакетный — не писать поверх, явный — перезапись
	h.fillMu.Unlock()

	if len(body.Accepted) == 0 {
		studioErr(w, "accepted пуст", http.StatusBadRequest)
		return
	}
	items := make([]repository.DescItem, 0, len(body.Accepted))
	var skipped []string
	for _, a := range body.Accepted {
		if a.I < 0 || a.I >= len(proposals) {
			studioErr(w, "индекс вне диапазона", http.StatusBadRequest)
			return
		}
		text := strings.TrimSpace(a.Text)
		if text == "" {
			skipped = append(skipped, "пустой текст — пропущено")
			continue
		}
		items = append(items, repository.DescItem{ID: proposals[a.I].ID, Text: ai.NormalizeDescription(text)})
	}

	applied, rep, err := h.repo.UpdateDescriptions(items, onlyIfEmpty)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	report := make([]string, 0, len(skipped)+len(rep)+2)
	if rejected := len(proposals) - len(body.Accepted); rejected > 0 {
		report = append(report, fmt.Sprintf("пропущено: %d (не принято)", rejected))
	}
	report = append(report, skipped...)
	report = append(report, rep...)
	report = append(report, fmt.Sprintf("Применено: %d", applied))
	h.finishDesc(report)
	h.clearDescProposals()
	studioJSON(w, http.StatusOK, map[string]int{"applied": applied})
}

// descriptionsCancel — POST /studio/api/descriptions/cancel: остановка идущего
// прогона (флаг; горутина завершает текущую порцию и останавливается, И7 —
// предложения сохраняются). 200 {"cancelled":"true","discarded":"false",
// "done":M,"total":N}; если джоб не идёт — отказ от набора: предложения
// выбрасываются (набор пуст, попап не всплывает после перезагрузки) —
// 200 {"cancelled":"false","discarded":"true"} (в отличие от fill-cancel 409 нет).
// Поле cancelled сохранено для совместимости (старый смоук/QA).
func (h *StudioHandlers) descriptionsCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	h.fillMu.Lock()
	if h.descGenerating {
		h.descCancel = true
		done, total := h.descDone, h.descTotal
		h.fillMu.Unlock()
		studioJSON(w, http.StatusOK, map[string]interface{}{"cancelled": "true", "discarded": "false", "done": done, "total": total})
		return
	}
	// прогон не идёт — отказаться от набора: те же поля, что сбрасывают
	// применение (clearDescProposals) и старт нового прогона (tryStartDesc).
	h.clearDescProposalsLocked()
	h.fillMu.Unlock()
	studioJSON(w, http.StatusOK, map[string]interface{}{"cancelled": "false", "discarded": "true"})
}

// tryStartDesc — атомарный старт джоба описаний (И6: общий флаг с fill).
// Сбрасывает канал описаний; total — число записей в прогоне, explicit — режим
// (И4: false — пакетный scope:"missing", true — явный good_ids).
func (h *StudioHandlers) tryStartDesc(total int, explicit bool) bool {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	if h.fillGenerating {
		return false
	}
	h.fillGenerating = true
	h.descGenerating = true
	h.descReport = nil
	h.descProposals = nil
	h.descTotal = total
	h.descDone = 0
	h.descCancel = false
	h.descExplicit = explicit
	return true
}

// finishDesc — завершение джоба/применения описаний (успех или остановка).
func (h *StudioHandlers) finishDesc(report []string) {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	h.fillGenerating = false
	h.descGenerating = false
	h.descReport = report
}

// addDescProgress — добавление предложений порции + прогресс (из runDescriptions).
func (h *StudioHandlers) addDescProgress(props []DescProposalView, doneDelta int) {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	h.descProposals = append(h.descProposals, props...)
	h.descDone += doneDelta
}

// descProgress — текущий прогресс (done, total) под fillMu.
func (h *StudioHandlers) descProgress() (int, int) {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	return h.descDone, h.descTotal
}

// descCancelled — запрошена ли остановка прогона описаний.
func (h *StudioHandlers) descCancelled() bool {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	return h.descCancel
}

// clearDescProposals — сброс предложений описаний (после применения либо отказа
// от набора при простое) вместе с режимом джоба (descExplicit, И4). Cancel
// идущего прогона предложения сохраняет (И7) — режим тоже сохраняется: apply
// после отмены обязан применить верный onlyIfEmpty.
func (h *StudioHandlers) clearDescProposals() {
	h.fillMu.Lock()
	defer h.fillMu.Unlock()
	h.clearDescProposalsLocked()
}

// clearDescProposalsLocked — тело clearDescProposals под уже взятым fillMu
// (нужно descriptionsCancel: сброс и проверка descGenerating — под одним локом).
func (h *StudioHandlers) clearDescProposalsLocked() {
	h.descProposals = nil
	h.descTotal = 0
	h.descDone = 0
	h.descExplicit = false
}

// snapshotState — model.State из снимка каталога (для BuildFillPrompt/
// BuildProposals): категории с kind (С2-проверка в ApplyProposals) +
// привязки рецептов (спека 2026-09-21-рецепт-сущность §5, риск 11).
func snapshotState(snap *repository.CatalogSnapshot) *model.State {
	st := &model.State{
		SchemaVersion: model.SchemaVersion,
		Categories:    make([]model.Category, 0, len(snap.Categories)),
		Goods:         snap.Goods,
		Bindings:      toModelBindings(snap.Bindings),
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
	warnings := validate.Validate(&model.State{SchemaVersion: model.SchemaVersion, Goods: snap.Goods, Bindings: toModelBindings(snap.Bindings)})
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
