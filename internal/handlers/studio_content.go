// internal/handlers/studio_content.go
// Экспорт снимка контента каталога (спека 2026-09-24-каталог-экспорт-импорт-
// контента-на-прод §4/§7, итерация И2): GET /studio/api/content/export
// собирает снимок по НАТУРАЛЬНЫМ ключам + меткам (code), без id, пишет
// content/catalog.json и отдаёт тот же JSON на скачивание. Секции упорядочены
// топологически по FK (родители раньше детей). Доступ — auth.AdminAuth на
// роуте (cmd/server/main.go). Импорт (И3) здесь НЕ реализуется.
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"zorion/internal/goodsstudio/contentio"
	"zorion/internal/repository"
)

// Типы формата снимка вынесены в пакет contentio (единый источник для экспорта
// И2 и импорта И3, §4/§5); здесь — алиасы, чтобы существующий код/тесты
// продолжали ссылаться на handlers.ContentXxx.
type (
	ContentSnapshot         = contentio.Snapshot
	ContentCategory         = contentio.Category
	ContentGood             = contentio.Good
	ContentRecipe           = contentio.Recipe
	ContentRecipeComponent  = contentio.RecipeComponent
	ContentProducerType     = contentio.ProducerType
	ContentUnlockCategory   = contentio.UnlockCategory
	ContentUnlock           = contentio.Unlock
	ContentItem             = contentio.Item
	ContentProducerSlot     = contentio.ProducerSlot
	ContentProducerRecipe   = contentio.ProducerRecipe
	ContentProducerItem     = contentio.ProducerItem
	ContentEffectType       = contentio.EffectType
	ContentGenerationConfig = contentio.GenerationConfig
)

// contentSource — маркер источника снимка (§4).
const contentSource = "dev"

// SetContentExportPath — путь файла-снимка (env CONTENT_CATALOG_PATH, дефолт
// content/catalog.json) — cmd/server/main.go.
func (h *StudioHandlers) SetContentExportPath(path string) {
	if path != "" {
		h.contentPath = path
	}
}

// ContentExport — GET /studio/api/content/export (§4/§7, И2).
func (h *StudioHandlers) ContentExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		studioErr(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	rows, err := h.repo.ContentExport()
	if err != nil {
		studioErr(w, "ошибка чтения каталога: "+err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	snap, err := buildContentSnapshot(rows, now)
	if err != nil {
		studioErr(w, "снимок не собран: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		studioErr(w, "сериализация снимка: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Запись файла — best-effort (§4): на проде ФС эфемерна/read-only,
	// скачивание от неё не зависит.
	if err := writeContentFile(h.contentPath, data); err != nil {
		log.Printf("WARN: экспорт контента: запись %s: %v", h.contentPath, err)
	}
	name := "catalog-" + now.Format("20060102") + ".json"
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// writeContentFile — записать снимок, создав каталог при необходимости.
func writeContentFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// buildContentSnapshot — natural-key сборка файла из сырых строк (§4):
// ссылки — по метке (code) и натуральным ключам, без id; producer_types
// упорядочены родители-раньше-детей (self-FK parent_id); отсутствие метки у
// контентной записи — ошибка (снимок обязан быть полным, §3.1/§4).
func buildContentSnapshot(rows *repository.ContentExportRows, now time.Time) (*ContentSnapshot, error) {
	catName := make(map[int64]string, len(rows.Categories))
	catRef := make(map[int64]ContentUnlockCategory, len(rows.Categories))
	for _, c := range rows.Categories {
		catName[c.ID] = c.Name
		catRef[c.ID] = ContentUnlockCategory{Kind: c.Kind, Name: c.Name}
	}
	goodCode := make(map[int64]string, len(rows.Goods))
	for i := range rows.Goods {
		g := &rows.Goods[i]
		if g.Code.String == "" {
			return nil, fmt.Errorf("goods %q: пустая метка code — снимок невозможен (§3.1)", g.Name)
		}
		goodCode[g.ID] = g.Code.String
	}
	prodCode := make(map[int64]string, len(rows.ProducerTypes))
	for i := range rows.ProducerTypes {
		p := &rows.ProducerTypes[i]
		if p.Code.String == "" {
			return nil, fmt.Errorf("producer_types %q: пустая метка code — снимок невозможен (§3.1)", p.Name)
		}
		prodCode[p.ID] = p.Code.String
	}
	itemCode := make(map[int64]string, len(rows.Items))
	for i := range rows.Items {
		it := &rows.Items[i]
		if it.Code.String == "" {
			return nil, fmt.Errorf("items %q: пустая метка code — снимок невозможен (§3.1)", it.Name)
		}
		itemCode[it.ID] = it.Code.String
	}
	for i := range rows.EffectTypes {
		e := &rows.EffectTypes[i]
		if e.Code.String == "" {
			return nil, fmt.Errorf("effect_types %q: пустая метка code — снимок невозможен (§3.1)", e.Name)
		}
	}

	snap := &ContentSnapshot{
		SchemaVersion: 1,
		GeneratedAt:   now.Format(time.RFC3339),
		Source:        contentSource,
		Categories:    make([]ContentCategory, 0, len(rows.Categories)),
		Goods:         make([]ContentGood, 0, len(rows.Goods)),
		Recipes:       make([]ContentRecipe, 0, len(rows.Recipes)),
		ProducerTypes: make([]ContentProducerType, 0, len(rows.ProducerTypes)),
		Items:         make([]ContentItem, 0, len(rows.Items)),
	}

	for _, c := range rows.Categories {
		cc := ContentCategory{Name: c.Name, Kind: c.Kind, IsSystem: c.IsSystem}
		if c.Code.Valid {
			cc.Code = c.Code.String
		}
		snap.Categories = append(snap.Categories, cc)
	}
	for i := range rows.Goods {
		g := &rows.Goods[i]
		cg := ContentGood{
			Name:     g.Name,
			Kind:     g.Kind,
			Category: catName[g.CategoryID],
			Source:   g.Source,
			Code:     g.Code.String,
		}
		if g.Description.Valid {
			cg.Description = g.Description.String
		}
		if g.Volume.Valid {
			v := g.Volume.Float64
			cg.Volume = &v
		}
		if g.Weight.Valid {
			w := g.Weight.Float64
			cg.Weight = &w
		}
		if len(g.Props) > 0 {
			cg.Props = json.RawMessage(g.Props)
		}
		snap.Goods = append(snap.Goods, cg)
	}
	for _, rec := range rows.Recipes {
		cr := ContentRecipe{Good: goodCode[rec.GoodID]}
		if rec.Complexity.Valid {
			c := int(rec.Complexity.Int64)
			cr.Complexity = &c
		}
		snap.Recipes = append(snap.Recipes, cr)
	}
	snap.RecipeComponents = make([]ContentRecipeComponent, 0, len(rows.Components))
	for _, comp := range rows.Components {
		cc := ContentRecipeComponent{
			Good:          goodCode[comp.GoodID],
			Pos:           comp.Pos,
			Quantity:      comp.Quantity,
			AllowResource: comp.AllowResource,
		}
		if comp.ComponentID.Valid {
			cc.Component = goodCode[comp.ComponentID.Int64]
		}
		if comp.Reason.Valid {
			cc.Reason = comp.Reason.String
		}
		snap.RecipeComponents = append(snap.RecipeComponents, cc)
	}
	for _, p := range orderProducerTypes(rows.ProducerTypes) {
		cpt := ContentProducerType{Name: p.Name, Kind: p.Kind, Hidden: p.Hidden, Code: p.Code.String}
		if p.CategoryID.Valid {
			cpt.Category = catName[p.CategoryID.Int64]
			cpt.CategoryKind = catRef[p.CategoryID.Int64].Kind
		}
		if p.ParentID.Valid {
			cpt.Parent = prodCode[p.ParentID.Int64]
		}
		if p.RaceFamily.Valid {
			cpt.RaceFamily = p.RaceFamily.String
		}
		if p.Race.Valid {
			cpt.Race = p.Race.String
		}
		cpt.Output = rawOrNil(p.Output)
		cpt.Input = rawOrNil(p.Input)
		cpt.Params = rawOrNil(p.Params)
		snap.ProducerTypes = append(snap.ProducerTypes, cpt)
	}
	for i := range rows.Items {
		it := &rows.Items[i]
		unlocks, err := translateUnlocks(it.Name, it.Unlocks, prodCode, catRef)
		if err != nil {
			return nil, err
		}
		snap.Items = append(snap.Items, ContentItem{
			Name:     it.Name,
			SlotType: it.SlotType,
			Unlocks:  unlocks,
			Params:   rawOrNil(it.Params),
			Code:     it.Code.String,
		})
	}
	snap.ProducerSlots = make([]ContentProducerSlot, 0, len(rows.ProducerSlots))
	for _, s := range rows.ProducerSlots {
		cs := ContentProducerSlot{
			Parent:       prodCode[s.ParentID],
			Category:     catName[s.CategoryID],
			CategoryKind: catRef[s.CategoryID].Kind,
			Hidden:       s.Hidden,
		}
		if s.RaceFamily.Valid {
			cs.RaceFamily = s.RaceFamily.String
		}
		if s.Race.Valid {
			cs.Race = s.Race.String
		}
		snap.ProducerSlots = append(snap.ProducerSlots, cs)
	}
	snap.ProducerRecipes = make([]ContentProducerRecipe, 0, len(rows.ProducerRecipes))
	for _, b := range rows.ProducerRecipes {
		cr := ContentProducerRecipe{Producer: prodCode[b.ProducerTypeID], Good: goodCode[b.GoodID]}
		if b.Rate.Valid {
			v := b.Rate.Float64
			cr.Rate = &v
		}
		snap.ProducerRecipes = append(snap.ProducerRecipes, cr)
	}
	snap.ProducerItems = make([]ContentProducerItem, 0, len(rows.ProducerItems))
	for _, pi := range rows.ProducerItems {
		snap.ProducerItems = append(snap.ProducerItems, ContentProducerItem{
			Producer:     prodCode[pi.ProducerTypeID],
			Item:         itemCode[pi.ItemID],
			Requirements: rawOrNil(pi.Requirements),
		})
	}
	for i := range rows.EffectTypes {
		e := &rows.EffectTypes[i]
		snap.EffectTypes = append(snap.EffectTypes, ContentEffectType{
			Name:   e.Name,
			Impact: e.Impact,
			Params: rawOrNil(e.Params),
			Code:   e.Code.String,
		})
	}
	if rows.DefaultTypeID != 0 {
		code, ok := prodCode[rows.DefaultTypeID]
		if !ok {
			return nil, fmt.Errorf("generation_config.default_settlement_type_id: запись %d не найдена среди producer_types", rows.DefaultTypeID)
		}
		snap.GenerationConfig.DefaultSettlementTypeID = code
	}

	snap.Counts = map[string]int{
		"categories":        len(snap.Categories),
		"goods":             len(snap.Goods),
		"recipes":           len(snap.Recipes),
		"recipe_components": len(snap.RecipeComponents),
		"producer_types":    len(snap.ProducerTypes),
		"items":             len(snap.Items),
		"producer_slots":    len(snap.ProducerSlots),
		"producer_recipes":  len(snap.ProducerRecipes),
		"producer_items":    len(snap.ProducerItems),
		"effect_types":      len(snap.EffectTypes),
	}
	return snap, nil
}

// rawOrNil — непустой JSONB → RawMessage, пустой/NULL → nil (omitempty).
func rawOrNil(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

// contentUnlockRaw — id-форма unlocks в БД (`[{producer_type_id, category_id}]`,
// `000048`); переводится в метку/натуральный ключ при экспорте (§4).
type contentUnlockRaw struct {
	ProducerTypeID int64 `json:"producer_type_id"`
	CategoryID     int64 `json:"category_id"`
}

// translateUnlocks — id-форма unlocks БД → переносимая форма снимка (§4):
// producer — метка producer_type, category — (kind, name). Нерезолвимая ссылка
// (id нет в снимке) — ОШИБКА, не молчаливый пропуск (T15). Пусто/NULL/`[]` →
// nil (omitempty).
func translateUnlocks(itemName string, raw []byte, prodCode map[int64]string, catRef map[int64]ContentUnlockCategory) ([]ContentUnlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var elems []contentUnlockRaw
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, fmt.Errorf("items %q: unlocks не разобран: %w", itemName, err)
	}
	if len(elems) == 0 {
		return nil, nil
	}
	out := make([]ContentUnlock, 0, len(elems))
	for _, e := range elems {
		code, ok := prodCode[e.ProducerTypeID]
		if !ok {
			return nil, fmt.Errorf("items %q: unlocks: producer_type_id %d не найден в снимке (§4/T15)", itemName, e.ProducerTypeID)
		}
		cat, ok := catRef[e.CategoryID]
		if !ok {
			return nil, fmt.Errorf("items %q: unlocks: category_id %d не найден в снимке (§4/T15)", itemName, e.CategoryID)
		}
		out = append(out, ContentUnlock{Producer: code, Category: cat})
	}
	return out, nil
}

// orderProducerTypes — порядок «родители раньше детей» для self-FK parent_id
// (§4): стабильная сортировка по глубине (0 — корень, 1 — подтип, …) с
// сохранением исходного порядка внутри уровня — повторный экспорт того же
// состояния даёт тот же порядок (детерминированно, T2). Глубина считается
// итеративно от известных родителей (глубина 1 — инвариант каталога, но
// алгоритм не завязан на это); цикл/родитель вне среза не двигают узел.
func orderProducerTypes(in []repository.ProducerTypeRow) []repository.ProducerTypeRow {
	if len(in) <= 1 {
		return in
	}
	idx := make(map[int64]int, len(in))
	for i := range in {
		idx[in[i].ID] = i
	}
	depth := make(map[int64]int, len(in))
	for iter := 0; iter <= len(in); iter++ {
		changed := false
		for i := range in {
			p := in[i].ParentID
			if !p.Valid {
				continue
			}
			pi, ok := idx[p.Int64]
			if !ok {
				continue
			}
			if want := depth[in[pi].ID] + 1; depth[in[i].ID] < want {
				depth[in[i].ID] = want
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	out := make([]repository.ProducerTypeRow, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool { return depth[out[i].ID] < depth[out[j].ID] })
	return out
}

// contentSections — обязательные секции снимка (§5 п.1).
var contentSections = []string{
	"schema_version", "categories", "goods", "recipes", "recipe_components",
	"producer_types", "items", "producer_slots", "producer_recipes",
	"producer_items", "effect_types", "generation_config",
}

// ContentStatus — GET /studio/api/content/status (§7): есть ли файл-снимок в
// репозитории (кнопка импорта активна только при нём), размер/дата/counts.
func (h *StudioHandlers) ContentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		studioErr(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	info, err := os.Stat(h.contentPath)
	if err != nil {
		studioJSON(w, http.StatusOK, map[string]interface{}{"file": false, "path": h.contentPath})
		return
	}
	resp := map[string]interface{}{
		"file": true, "path": h.contentPath, "size": info.Size(),
		"modified_at": info.ModTime().UTC().Format(time.RFC3339),
	}
	if data, err := os.ReadFile(h.contentPath); err == nil {
		var snap ContentSnapshot
		if json.Unmarshal(data, &snap) == nil {
			resp["generated_at"] = snap.GeneratedAt
			resp["counts"] = snap.Counts
			resp["schema_version"] = snap.SchemaVersion
		}
	}
	studioJSON(w, http.StatusOK, resp)
}

// ContentImport — POST /studio/api/content/import (§5/§7, И3). Тело —
// `{}` (взять content/catalog.json с диска) или `{ "snapshot": {...} }`.
// `?dry_run=true` — сухой прогон (§5.2): дифф без записи.
func (h *StudioHandlers) ContentImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	dryRun := r.URL.Query().Get("dry_run") == "true" || r.URL.Query().Get("dry_run") == "1"

	var body struct {
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		studioErr(w, "тело запроса не разобрано: "+err.Error(), http.StatusBadRequest)
		return
	}
	raw := body.Snapshot
	if len(raw) == 0 {
		data, err := os.ReadFile(h.contentPath)
		if err != nil {
			studioErr(w, "файл снимка не найден ("+h.contentPath+"): загрузите файл или положите его в репозиторий", http.StatusBadRequest)
			return
		}
		raw = data
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		studioErr(w, "снимок не разобран: "+err.Error(), http.StatusBadRequest)
		return
	}
	for _, sec := range contentSections {
		if _, ok := probe[sec]; !ok {
			studioErr(w, "в снимке нет секции "+sec, http.StatusBadRequest)
			return
		}
	}
	var snap ContentSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		studioErr(w, "снимок не разобран: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := h.repo.ImportContent(&snap, dryRun)
	if err != nil {
		status := http.StatusInternalServerError
		var ce *repository.ErrCatalog
		if errors.As(err, &ce) {
			status = ce.Status
		}
		payload := contentImportPayload(res, dryRun)
		payload["error"] = err.Error()
		studioJSON(w, status, payload)
		return
	}
	studioJSON(w, http.StatusOK, contentImportPayload(res, dryRun))
}

// contentImportPayload — тело ответа ручки: для dry_run — дифф (§5.2), для
// применения — отчёт (§5.5); blocked/unmatched отдаются всегда (UI блокирует
// «Применить»).
func contentImportPayload(res *repository.ImportResult, dryRun bool) map[string]interface{} {
	out := map[string]interface{}{"dry_run": dryRun}
	if res == nil {
		return out
	}
	out["mode"] = res.Mode
	if res.Diff != nil {
		out["create"] = res.Diff.Create
		out["update"] = res.Diff.Update
		out["delete"] = res.Diff.Delete
		out["blocked"] = res.Diff.Blocked
		out["unmatched"] = res.Diff.Unmatched
		out["remap"] = res.Diff.Remap
	}
	if !dryRun && res.Report != nil {
		out["created"] = res.Report.Created
		out["updated"] = res.Report.Updated
		out["deleted"] = res.Report.Deleted
		out["warnings"] = res.Report.Warnings
	}
	return out
}
