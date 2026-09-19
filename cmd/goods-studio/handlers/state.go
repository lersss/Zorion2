package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"zorion/cmd/goods-studio/ai"
	"zorion/cmd/goods-studio/catalog"
	"zorion/cmd/goods-studio/export"
	"zorion/internal/goodsstudio/graph"
	"zorion/internal/goodsstudio/model"
	"zorion/internal/goodsstudio/validate"
)

// SlotView — слот в представлении состояния (имя/тир/статус разрешены).
type SlotView struct {
	GoodID        string `json:"good_id"`
	Name          string `json:"name"`
	Tier          int    `json:"tier"`
	Status        string `json:"status"`
	Quantity      int    `json:"quantity"` // единиц составляющей, всегда ≥ 1 (99a.2 §6.2)
	Reason        string `json:"reason,omitempty"`
	AllowResource bool   `json:"allow_resource"` // галка «заполнять ресурсом» (99a Пакет 4, п.8)
}

// GoodView — товар в представлении состояния.
type GoodView struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Category     string     `json:"category"`
	Status       string     `json:"status"`
	Kind         string     `json:"kind"`
	Source       string     `json:"source"`
	Tier         int        `json:"tier"`          // эффективный (override ?? вычисленный, 99a.3 §4.2)
	TierComputed int        `json:"tier_computed"` // вычисленный (graph.Tier)
	TierOverride *int       `json:"tier_override"` // ручной оверрайд, null = вычисляется
	Recipe       []SlotView `json:"recipe"`
	BannedAt     *string    `json:"banned_at"`
}

// StateView — полное состояние для UI (спека 99a.1 §9, GET /api/state).
type StateView struct {
	Categories    []model.Category   `json:"categories"`
	Goods         []GoodView         `json:"goods"`
	Banned        []GoodView         `json:"banned"`
	Unused        []GoodView         `json:"unused"`
	Warnings      []validate.Warning `json:"warnings"`
	Model         string             `json:"model"`
	Generating    bool               `json:"generating"`
	Report        []string           `json:"report"`
	AutoRefreshMS int                `json:"auto_refresh_ms"`
}

// ResourceView — ресурс палитры (GET /api/resources).
type ResourceView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Catalog  string `json:"catalog"`
}

// handleState — GET /api/state: полное состояние + banned + unused + warnings.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "только GET")
		return
	}
	s.mu.RLock()
	view := s.buildView()
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, view)
}

// handleResources — GET /api/resources: импортированные каталоги для палитры
// (забаненные скрыты — бан скрывает из обычных видов, спека 99a.1 §5.2).
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "только GET")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ResourceView, 0, len(s.state.Goods))
	for i := range s.state.Goods {
		g := &s.state.Goods[i]
		if g.Kind != model.KindResource || g.Status == model.StatusBanned {
			continue
		}
		out = append(out, ResourceView{
			ID:       g.ID,
			Name:     g.Name,
			Category: g.Category,
			Catalog:  g.ResourceRef.Catalog,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// --- категории ---

// handleCategories — POST /api/categories {name}.
func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "только POST")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный JSON")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "имя пустое")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.state.Categories {
		if graph.NormalizeName(c.Name) == graph.NormalizeName(name) {
			writeErr(w, http.StatusConflict, "категория с таким именем уже есть")
			return
		}
	}
	s.state.Categories = append(s.state.Categories, model.Category{ID: model.NextCategoryID(s.state.Categories), Name: name})
	s.persist()
	writeJSON(w, http.StatusCreated, s.state.Categories)
}

// handleCategoryByID — PUT/DELETE /api/categories/{id}.
func (s *Server) handleCategoryByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/categories/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, http.StatusNotFound, "не найдено")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "невалидный JSON")
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			writeErr(w, http.StatusBadRequest, "имя пустое")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		idx := categoryIndex(s.state.Categories, id)
		if idx < 0 {
			writeErr(w, http.StatusNotFound, "категория не найдена")
			return
		}
		for i, c := range s.state.Categories {
			if i != idx && graph.NormalizeName(c.Name) == graph.NormalizeName(name) {
				writeErr(w, http.StatusConflict, "категория с таким именем уже есть")
				return
			}
		}
		s.state.Categories[idx].Name = name
		s.persist()
		writeJSON(w, http.StatusOK, s.state.Categories)
	case http.MethodDelete:
		s.mu.Lock()
		defer s.mu.Unlock()
		idx := categoryIndex(s.state.Categories, id)
		if idx < 0 {
			writeErr(w, http.StatusNotFound, "категория не найдена")
			return
		}
		for i := range s.state.Goods {
			if s.state.Goods[i].Category == id {
				writeErr(w, http.StatusConflict, "категория не пуста — сначала перекатегоризуйте товары")
				return
			}
		}
		s.state.Categories = append(s.state.Categories[:idx], s.state.Categories[idx+1:]...)
		s.persist()
		writeJSON(w, http.StatusOK, s.state.Categories)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "только PUT/DELETE")
	}
}

// --- товары ---

// handleGoods — POST /api/goods {name, category_id}: товар-черновик
// с одним пустым слотом (по умолчанию слот один, спека 99a.1 §5.1).
func (s *Server) handleGoods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "только POST")
		return
	}
	var body struct {
		Name       string `json:"name"`
		CategoryID string `json:"category_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный JSON")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "имя пустое")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if categoryIndex(s.state.Categories, body.CategoryID) < 0 {
		writeErr(w, http.StatusBadRequest, "категория не найдена")
		return
	}
	if graph.FindByName(name, s.state.Goods) != nil {
		writeErr(w, http.StatusConflict, "товар с таким именем уже есть")
		return
	}
	g := model.Good{
		ID:        model.NextGoodID(s.state.Goods),
		Name:      name,
		Category:  body.CategoryID,
		Status:    model.StatusDraft,
		Kind:      model.KindGood,
		Source:    model.SourceManual,
		Recipe:    []model.Slot{{Quantity: 1}}, // по умолчанию слот один, quantity=1 (99a.2 §6.1)
		CreatedAt: nowISO(),
	}
	s.state.Goods = append(s.state.Goods, g)
	s.persist()
	writeJSON(w, http.StatusCreated, g)
}

// handleGoodByID — PUT/DELETE /api/goods/{id} и под-пути
// (status, slots, fill).
func (s *Server) handleGoodByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/goods/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusNotFound, "не найдено")
		return
	}
	id := parts[0]
	switch {
	case len(parts) == 1 && parts[0] == "bulk":
		s.handleGoodsBulk(w, r)
	case len(parts) == 1:
		s.handleGood(w, r, id)
	case len(parts) == 2 && parts[1] == "status":
		s.handleGoodStatus(w, r, id)
	case len(parts) == 2 && parts[1] == "tier":
		s.handleGoodTier(w, r, id)
	case len(parts) == 2 && parts[1] == "slots":
		s.handleAddSlot(w, r, id)
	case len(parts) == 2 && parts[1] == "fill":
		s.handleFill(w, r, id)
	case len(parts) == 3 && parts[1] == "slots":
		s.handleSlot(w, r, id, parts[2])
	case len(parts) == 4 && parts[1] == "slots" && parts[3] == "component":
		s.handleClearSlot(w, r, id, parts[2])
	case len(parts) == 4 && parts[1] == "slots" && parts[3] == "allow_resource":
		s.handleSlotAllowResource(w, r, id, parts[2])
	default:
		writeErr(w, http.StatusNotFound, "не найдено")
	}
}

// handleGood — PUT/DELETE /api/goods/{id} (для ресурсов — 403, §5.3).
func (s *Server) handleGood(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name       *string `json:"name"`
			CategoryID *string `json:"category_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "невалидный JSON")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		idx := goodIndex(s.state.Goods, id)
		if idx < 0 {
			writeErr(w, http.StatusNotFound, "товар не найден")
			return
		}
		if s.state.Goods[idx].Kind == model.KindResource {
			writeErr(w, http.StatusForbidden, "ресурс read-only (правка каталога — в layer.go/real.go сервера)")
			return
		}
		if body.Name != nil {
			name := strings.TrimSpace(*body.Name)
			if name == "" {
				writeErr(w, http.StatusBadRequest, "имя пустое")
				return
			}
			for i := range s.state.Goods {
				if i != idx && graph.NormalizeName(s.state.Goods[i].Name) == graph.NormalizeName(name) {
					writeErr(w, http.StatusConflict, "товар с таким именем уже есть")
					return
				}
			}
			s.state.Goods[idx].Name = name
		}
		if body.CategoryID != nil {
			if categoryIndex(s.state.Categories, *body.CategoryID) < 0 {
				writeErr(w, http.StatusBadRequest, "категория не найдена")
				return
			}
			s.state.Goods[idx].Category = *body.CategoryID
		}
		s.persist()
		writeJSON(w, http.StatusOK, s.state.Goods[idx])
	case http.MethodDelete:
		s.mu.Lock()
		defer s.mu.Unlock()
		idx := goodIndex(s.state.Goods, id)
		if idx < 0 {
			writeErr(w, http.StatusNotFound, "товар не найден")
			return
		}
		if s.state.Goods[idx].Kind == model.KindResource {
			writeErr(w, http.StatusForbidden, "ресурс read-only (правка каталога — в layer.go/real.go сервера)")
			return
		}
		// удаление совсем; слоты, ссылающиеся на него, очищаются (§9)
		s.state.Goods = append(s.state.Goods[:idx], s.state.Goods[idx+1:]...)
		for i := range s.state.Goods {
			for j := range s.state.Goods[i].Recipe {
				if s.state.Goods[i].Recipe[j].GoodID == id {
					s.state.Goods[i].Recipe[j].GoodID = ""
					s.state.Goods[i].Recipe[j].Reason = ""
				}
			}
		}
		s.persist()
		writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "только PUT/DELETE")
	}
}

// handleGoodStatus — POST /api/goods/{id}/status {status}.
// draft/approved/excluded/banned/unban; для ресурсов — только banned/unban.
func (s *Server) handleGoodStatus(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "только POST")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный JSON")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := goodIndex(s.state.Goods, id)
	if idx < 0 {
		writeErr(w, http.StatusNotFound, "товар не найден")
		return
	}
	g := &s.state.Goods[idx]
	switch body.Status {
	case "banned":
		now := nowISO()
		g.Status = model.StatusBanned
		g.BannedAt = &now
	case "unban":
		g.Status = model.StatusDraft
		g.BannedAt = nil
	case "draft", "approved", "excluded":
		if g.Kind == model.KindResource {
			writeErr(w, http.StatusForbidden, "ресурс read-only: только banned/unban")
			return
		}
		g.Status = model.Status(body.Status)
		g.BannedAt = nil
	default:
		writeErr(w, http.StatusBadRequest, "неизвестный статус")
		return
	}
	s.persist()
	writeJSON(w, http.StatusOK, g)
}

// handleGoodTier — PUT /api/goods/{id}/tier {tier: int|null} (99a.3 §9.1):
// установить ручной оверрайд тира; null — очистить (вернуть к вычисленному).
// Полная свобода значений ≥ 0 (в т.ч. ниже вычисленного); отрицательное — 400;
// ресурс read-only — 403; неизвестный id — 404.
func (s *Server) handleGoodTier(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPut {
		writeErr(w, http.StatusMethodNotAllowed, "только PUT")
		return
	}
	var body struct {
		Tier *int `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный JSON")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := goodIndex(s.state.Goods, id)
	if idx < 0 {
		writeErr(w, http.StatusNotFound, "товар не найден")
		return
	}
	if s.state.Goods[idx].Kind == model.KindResource {
		writeErr(w, http.StatusForbidden, "ресурс read-only (тир ресурса всегда 0)")
		return
	}
	if body.Tier != nil && *body.Tier < 0 {
		writeErr(w, http.StatusBadRequest, "тир не может быть отрицательным")
		return
	}
	s.state.Goods[idx].TierOverride = body.Tier
	s.persist()
	writeJSON(w, http.StatusOK, s.state.Goods[idx])
}

// handleGoodsBulk — POST /api/goods/bulk {lines: [...]}: подгрузка списка
// названий (99a.3 §9.2). Частичный успех по строкам: валидные создаются
// (draft, один пустой слот, quantity 1, source=manual), проблемные — в
// отчёте; состояние не откатывается. Персист — одна запись на пачку.
func (s *Server) handleGoodsBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "только POST")
		return
	}
	var body struct {
		Lines []string `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный JSON")
		return
	}
	if len(body.Lines) == 0 {
		writeErr(w, http.StatusBadRequest, "lines пуст")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rep := s.applyBulk(body.Lines)
	s.persist()
	writeJSON(w, http.StatusOK, rep)
}

// bulkCreated/bulkSkipped/bulkError/bulkReport — отчёт подгрузки списка
// (99a.3 §9.2): created/skipped/errors с физическими номерами строк (1-based).
type bulkCreated struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type bulkSkipped struct {
	Line   int    `json:"line"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type bulkError struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

type bulkReport struct {
	Created []bulkCreated `json:"created"`
	Skipped []bulkSkipped `json:"skipped"`
	Errors  []bulkError   `json:"errors"`
}

// applyBulk разбирает строки подгрузки (под s.mu). Правила 99a.3 §9.2:
// разделитель — первый «|»; пустая строка пропускается молча и не нумеруется;
// номер строки = физический индекс в textarea (1-based); дубликат имени
// (существующий или в пачке) — пропуск с предупреждением.
func (s *Server) applyBulk(lines []string) bulkReport {
	rep := bulkReport{Created: []bulkCreated{}, Skipped: []bulkSkipped{}, Errors: []bulkError{}}
	// категории по нормализованному имени (паттерн 99a.2 §4.3)
	catByName := make(map[string]string, len(s.state.Categories))
	for _, c := range s.state.Categories {
		catByName[graph.NormalizeName(c.Name)] = c.ID
	}
	// существующие имена (нормализованные) — для пропуска дубликатов
	existing := make(map[string]bool, len(s.state.Goods))
	for i := range s.state.Goods {
		existing[graph.NormalizeName(s.state.Goods[i].Name)] = true
	}
	for i, raw := range lines {
		line := i + 1 // физический номер строки (1-based); пустые не нумеруются
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue // пустая строка — молча
		}
		// разделитель — первый «|»; остальные «|» остаются частью имени
		sep := strings.Index(trimmed, "|")
		if sep < 0 {
			rep.Errors = append(rep.Errors, bulkError{Line: line, Reason: "нет разделителя „|"})
			continue
		}
		name := strings.TrimSpace(trimmed[:sep])
		catName := strings.TrimSpace(trimmed[sep+1:])
		if name == "" {
			rep.Errors = append(rep.Errors, bulkError{Line: line, Reason: "пустое имя"})
			continue
		}
		if catName == "" {
			rep.Errors = append(rep.Errors, bulkError{Line: line, Reason: "пустая категория"})
			continue
		}
		catID, ok := catByName[graph.NormalizeName(catName)]
		if !ok {
			rep.Errors = append(rep.Errors, bulkError{Line: line, Reason: "категория не найдена: " + catName})
			continue
		}
		norm := graph.NormalizeName(name)
		if existing[norm] {
			rep.Skipped = append(rep.Skipped, bulkSkipped{Line: line, Name: name, Reason: "товар с таким именем уже есть"})
			continue
		}
		g := model.Good{
			ID:        model.NextGoodID(s.state.Goods),
			Name:      name,
			Category:  catID,
			Status:    model.StatusDraft,
			Kind:      model.KindGood,
			Source:    model.SourceManual,
			Recipe:    []model.Slot{{Quantity: 1}}, // один пустой слот, quantity=1 (99a.2 §6.1)
			CreatedAt: nowISO(),
		}
		s.state.Goods = append(s.state.Goods, g)
		existing[norm] = true // дубликат в пачке — тоже пропуск
		rep.Created = append(rep.Created, bulkCreated{ID: g.ID, Name: g.Name})
	}
	return rep
}

// handleAddSlot — POST /api/goods/{id}/slots: добавить пустой слот.
func (s *Server) handleAddSlot(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "только POST")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := goodIndex(s.state.Goods, id)
	if idx < 0 {
		writeErr(w, http.StatusNotFound, "товар не найден")
		return
	}
	s.state.Goods[idx].Recipe = append(s.state.Goods[idx].Recipe, model.Slot{Quantity: 1}) // новый слот с quantity=1 (99a.2 §6.1)
	s.persist()
	writeJSON(w, http.StatusOK, s.state.Goods[idx])
}

// handleSlot — PUT/DELETE /api/goods/{id}/slots/{n}.
// PUT {good_id} — положить составляющую (цикл §6.2 — 409).
// DELETE — удалить слот.
func (s *Server) handleSlot(w http.ResponseWriter, r *http.Request, id, nStr string) {
	n, err := parseSlotIndex(nStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный номер слота")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			GoodID   string `json:"good_id"`
			Quantity *int   `json:"quantity"` // опционально (99a.2 §6.2)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "невалидный JSON")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		idx := goodIndex(s.state.Goods, id)
		if idx < 0 {
			writeErr(w, http.StatusNotFound, "товар не найден")
			return
		}
		if n < 0 || n >= len(s.state.Goods[idx].Recipe) {
			writeErr(w, http.StatusNotFound, "слот не найден")
			return
		}
		compIdx := goodIndex(s.state.Goods, body.GoodID)
		if body.GoodID != "" && compIdx < 0 {
			writeErr(w, http.StatusNotFound, "составляющая не найдена")
			return
		}
		// good_id опционален (99a.2 §6.5): PUT {quantity} — только количество,
		// составляющая не меняется; PUT {good_id} — замена составляющей
		if body.GoodID != "" {
			byID := graph.ByID(s.state.Goods)
			if graph.WouldCreateCycle(body.GoodID, id, byID) {
				writeErr(w, http.StatusConflict, "цикл: товар не может быть составляющей самого себя (рёбра только вверх)")
				return
			}
			s.state.Goods[idx].Recipe[n].GoodID = body.GoodID
			s.state.Goods[idx].Recipe[n].Reason = ""
		}
		// quantity: передан и ≥ 1 — обновить; не передан — оставить текущее
		// (замена составляющей не сбрасывает количество); < 1 — трактуется как 1
		if body.Quantity != nil {
			q := *body.Quantity
			if q < 1 {
				q = 1
			}
			s.state.Goods[idx].Recipe[n].Quantity = q
		}
		s.persist()
		writeJSON(w, http.StatusOK, s.state.Goods[idx])
	case http.MethodDelete:
		s.mu.Lock()
		defer s.mu.Unlock()
		idx := goodIndex(s.state.Goods, id)
		if idx < 0 {
			writeErr(w, http.StatusNotFound, "товар не найден")
			return
		}
		if n < 0 || n >= len(s.state.Goods[idx].Recipe) {
			writeErr(w, http.StatusNotFound, "слот не найден")
			return
		}
		s.state.Goods[idx].Recipe = append(s.state.Goods[idx].Recipe[:n], s.state.Goods[idx].Recipe[n+1:]...)
		s.persist()
		writeJSON(w, http.StatusOK, s.state.Goods[idx])
	default:
		writeErr(w, http.StatusMethodNotAllowed, "только PUT/DELETE")
	}
}

// handleClearSlot — DELETE /api/goods/{id}/slots/{n}/component:
// очистить слот («исключить из рецепта», товар остаётся в реестре).
func (s *Server) handleClearSlot(w http.ResponseWriter, r *http.Request, id, nStr string) {
	if r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, "только DELETE")
		return
	}
	n, err := parseSlotIndex(nStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный номер слота")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := goodIndex(s.state.Goods, id)
	if idx < 0 {
		writeErr(w, http.StatusNotFound, "товар не найден")
		return
	}
	if n < 0 || n >= len(s.state.Goods[idx].Recipe) {
		writeErr(w, http.StatusNotFound, "слот не найден")
		return
	}
	s.state.Goods[idx].Recipe[n].GoodID = ""
	s.state.Goods[idx].Recipe[n].Reason = ""
	s.persist()
	writeJSON(w, http.StatusOK, s.state.Goods[idx])
}

// handleSlotAllowResource — PUT /api/goods/{id}/slots/{n}/allow_resource
// {allow_resource: bool}: галка «заполнять ресурсом» на слоте (99a Пакет 4,
// п.8, решение создателя 2026-09-19). Влияет только на пустой слот: с галкой
// ИИ может предложить для него ресурс, без галки — только товары. На
// заполненном слоте галка хранится, но не действует (ресурс уже вставлен).
func (s *Server) handleSlotAllowResource(w http.ResponseWriter, r *http.Request, id, nStr string) {
	if r.Method != http.MethodPut {
		writeErr(w, http.StatusMethodNotAllowed, "только PUT")
		return
	}
	var body struct {
		AllowResource bool `json:"allow_resource"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный JSON")
		return
	}
	n, err := parseSlotIndex(nStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный номер слота")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := goodIndex(s.state.Goods, id)
	if idx < 0 {
		writeErr(w, http.StatusNotFound, "товар не найден")
		return
	}
	if n < 0 || n >= len(s.state.Goods[idx].Recipe) {
		writeErr(w, http.StatusNotFound, "слот не найден")
		return
	}
	s.state.Goods[idx].Recipe[n].AllowResource = body.AllowResource
	s.persist()
	writeJSON(w, http.StatusOK, s.state.Goods[idx])
}

// handleFill — POST /api/goods/{id}/fill: «заполнить комплектующие» (§7).
// Асинхронно: одна активная генерация (TryStart), UI опрашивает /api/state.
func (s *Server) handleFill(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "только POST")
		return
	}
	s.mu.RLock()
	idx := goodIndex(s.state.Goods, id)
	if idx < 0 {
		s.mu.RUnlock()
		writeErr(w, http.StatusNotFound, "товар не найден")
		return
	}
	if len(emptySlots(s.state.Goods[idx].Recipe)) == 0 {
		s.mu.RUnlock()
		writeErr(w, http.StatusBadRequest, "нет пустых слотов")
		return
	}
	prompt := s.buildFillPrompt(idx)
	s.mu.RUnlock()

	if !s.tryStartFill() {
		writeErr(w, http.StatusConflict, "уже идёт генерация")
		return
	}
	go s.runFill(id, prompt)
	writeJSON(w, http.StatusAccepted, map[string]string{"started": "true"})
}

// runFill — фоновая генерация: запрос к ИИ → разбор → вставка → персист.
func (s *Server) runFill(goodID, prompt string) {
	raw, err := s.ai.FillComponents(prompt)
	if err != nil {
		s.finishFill([]string{"Ошибка ИИ: " + err.Error()})
		return
	}
	comps, err := ai.ParseFillResponse(raw)
	if err != nil {
		s.finishFill([]string{"Мусор в ответе ИИ: " + err.Error()})
		return
	}
	s.mu.Lock()
	report := ai.ApplyFill(s.state, goodID, comps)
	s.persist()
	s.mu.Unlock()
	s.finishFill(report)
}

// buildFillPrompt — промпт «заполнить комплектующие» (§7.2). Вызывается
// под RLock.
func (s *Server) buildFillPrompt(idx int) string {
	g := &s.state.Goods[idx]
	byID := graph.ByID(s.state.Goods)
	tier := graph.Tier(g, byID)
	catName := ""
	for _, c := range s.state.Categories {
		if c.ID == g.Category {
			catName = c.Name
			break
		}
	}
	var filled, banned, resNames, catNames []string
	for _, slot := range g.Recipe {
		if slot.GoodID == "" {
			continue
		}
		if comp := byID[slot.GoodID]; comp != nil {
			filled = append(filled, comp.Name)
		}
	}
	for i := range s.state.Goods {
		gg := &s.state.Goods[i]
		if gg.Status == model.StatusBanned || gg.Status == model.StatusExcluded {
			banned = append(banned, gg.Name)
		}
		if gg.Kind == model.KindResource {
			resNames = append(resNames, gg.Name)
		}
	}
	for _, c := range s.state.Categories {
		catNames = append(catNames, c.ID+": "+c.Name)
	}
	k := len(emptySlots(g.Recipe))
	// позиции пустых слотов с галкой «заполнять ресурсом» (1-базовые, по
	// порядку пустых слотов) — для промпта (99a Пакет 4, п.8)
	var allowResSlots []int
	for pos, slotIdx := range emptySlots(g.Recipe) {
		if g.Recipe[slotIdx].AllowResource {
			allowResSlots = append(allowResSlots, pos+1)
		}
	}
	return ai.BuildPrompt(g, catName, tier, filled, banned, resNames, catNames, k, allowResSlots)
}

// --- экспорт и валидация ---

// handleExport — POST /api/export {path?}: запись export.json
// (дефолт goods_data/export.json) + warnings (спека 99a.1 §11).
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "только POST")
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.RLock()
	ex := export.Build(s.state)
	s.mu.RUnlock()

	path := body.Path
	if path == "" {
		path = filepath.Join(s.cfg.DataDir, "export.json")
	}
	if err := writeJSONFileAtomic(path, ex); err != nil {
		writeErr(w, http.StatusInternalServerError, "ошибка записи: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":     path,
		"warnings": ex.Warnings,
	})
}

// handleImport — POST /api/import {path?}: импорт каталога (99a.2 §7 +
// деревья Т8 schema 2). Формат определяется по schema_version файла:
// 1 — top-каталог (ParseTopCatalog), 2 — деревья Т8 (ParseTreesCatalog).
// Полная замена товаров студии (ресурсы не тронуты); бэкап state.json.bak
// перед заменой; ответ — новое состояние + отчёт. Защита: файл не найден →
// 404; невалидный каталог → 400 (состояние не меняется); активная
// ИИ-генерация → 409. Путь — как передан (filepath.Clean), дефолт
// goods_data/trees_t8.json.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "только POST")
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	path := body.Path
	if path == "" {
		path = filepath.Join(s.cfg.DataDir, "trees_t8.json")
	}
	path = filepath.Clean(path)

	// защита: активная генерация → 409 (импорт сносит товары, на которые
	// может писать генерация; 99a.2 §7)
	s.mu.RLock()
	generating := s.generating
	s.mu.RUnlock()
	if generating {
		writeErr(w, http.StatusConflict, "идёт генерация — импорт после завершения")
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		writeErr(w, http.StatusNotFound, "файл не найден: "+path)
		return
	}

	// определение формата по schema_version (1 — top-каталог, 2 — деревья)
	var head struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		writeErr(w, http.StatusBadRequest, "невалидный JSON: "+err.Error())
		return
	}
	var apply func(st *model.State) *catalog.ImportReport
	switch head.SchemaVersion {
	case 1:
		tc, err := catalog.ParseTopCatalog(data)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		apply = func(st *model.State) *catalog.ImportReport { return catalog.ApplyTopCatalog(st, tc) }
	case 2:
		tc, err := catalog.ParseTreesCatalog(data)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		apply = func(st *model.State) *catalog.ImportReport { return catalog.ApplyTreesCatalog(st, tc) }
	default:
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("неподдерживаемая версия каталога: %d (ожидается 1 или 2)", head.SchemaVersion))
		return
	}

	s.mu.Lock()
	// бэкап перед заменой (99a.2 §4.1): атомарная копия state.json → .bak;
	// если state.json нет (первый запуск) — бэкап не создаётся, не ошибка
	backupPath := ""
	if raw, err := os.ReadFile(s.statePath); err == nil {
		if err := os.WriteFile(s.statePath+".bak", raw, 0644); err == nil {
			backupPath = s.statePath + ".bak"
		}
	}
	report := apply(s.state)
	report.Backup = backupPath
	s.persist()
	view := s.buildView()
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"report": report,
		"state":  view,
	})
}

// handleValidate — GET /api/validate: прогон валидаторов (§8).
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "только GET")
		return
	}
	s.mu.RLock()
	warnings := validate.Validate(s.state)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, warnings)
}

// --- представление состояния ---

// buildView собирает StateView. Вызывается под RLock.
func (s *Server) buildView() StateView {
	byID := graph.ByID(s.state.Goods)
	view := StateView{
		Categories:    s.state.Categories,
		Model:         s.cfg.Model,
		Generating:    s.generating,
		Report:        s.lastReport,
		Warnings:      validate.Validate(s.state),
		AutoRefreshMS: s.cfg.AutoRefreshMS,
	}
	inDegree := make(map[string]int, len(s.state.Goods))
	for i := range s.state.Goods {
		for _, slot := range s.state.Goods[i].Recipe {
			if slot.GoodID != "" {
				inDegree[slot.GoodID]++
			}
		}
	}
	for i := range s.state.Goods {
		g := &s.state.Goods[i]
		gv := GoodView{
			ID:           g.ID,
			Name:         g.Name,
			Category:     g.Category,
			Status:       string(g.Status),
			Kind:         string(g.Kind),
			Source:       string(g.Source),
			Tier:         graph.EffectiveTier(g, byID),
			TierComputed: graph.Tier(g, byID),
			TierOverride: g.TierOverride,
			BannedAt:     g.BannedAt,
		}
		for _, slot := range g.Recipe {
			q := slot.Quantity
			if q < 1 {
				q = 1 // страховка: в памяти всегда ≥ 1 после нормализации (99a.2 §6.1)
			}
			sv := SlotView{GoodID: slot.GoodID, Reason: slot.Reason, AllowResource: slot.AllowResource, Quantity: q}
			if comp := byID[slot.GoodID]; comp != nil {
				sv.Name = comp.Name
				sv.Tier = graph.EffectiveTier(comp, byID) // эффективный тир составляющей (99a.3 §9.3)
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

// --- хелперы ---

func categoryIndex(cats []model.Category, id string) int {
	for i := range cats {
		if cats[i].ID == id {
			return i
		}
	}
	return -1
}

func goodIndex(goods []model.Good, id string) int {
	for i := range goods {
		if goods[i].ID == id {
			return i
		}
	}
	return -1
}

func emptySlots(recipe []model.Slot) []int {
	var out []int
	for i := range recipe {
		if recipe[i].GoodID == "" {
			out = append(out, i)
		}
	}
	return out
}

func parseSlotIndex(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}

// writeJSONFileAtomic — атомарная запись JSON-файла (tmp + rename с ретраем,
// паттерн SaveState, docs/PITFALLS.md «Go и конкурентность»).
func writeJSONFileAtomic(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	var lastErr error
	for i := 0; i < 5; i++ {
		if err := os.Rename(tmp, path); err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(5 * time.Millisecond)
	}
	return lastErr
}