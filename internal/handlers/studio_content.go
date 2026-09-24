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
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"zorion/internal/repository"
)

// ContentSnapshot — файл-снимок контента (§4), schema_version отдельный от
// model.State (§4): порядок полей struct = порядок секций в JSON.
type ContentSnapshot struct {
	SchemaVersion    int                      `json:"schema_version"`
	GeneratedAt      string                   `json:"generated_at"`
	Source           string                   `json:"source"`
	Counts           map[string]int           `json:"counts"`
	Categories       []ContentCategory        `json:"categories"`
	Goods            []ContentGood            `json:"goods"`
	Recipes          []ContentRecipe          `json:"recipes"`
	RecipeComponents []ContentRecipeComponent `json:"recipe_components"`
	ProducerTypes    []ContentProducerType    `json:"producer_types"`
	Items            []ContentItem            `json:"items"`
	ProducerSlots    []ContentProducerSlot    `json:"producer_slots"`
	ProducerRecipes  []ContentProducerRecipe  `json:"producer_recipes"`
	ProducerItems    []ContentProducerItem    `json:"producer_items"`
	EffectTypes      []ContentEffectType      `json:"effect_types"`
	GenerationConfig ContentGenerationConfig  `json:"generation_config"`
}

// ContentCategory — категория: идентичность (kind, name_norm); code —
// семантический (water/gas/…), не метка переноса (§3.4).
type ContentCategory struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Code     string `json:"code,omitempty"`
	IsSystem bool   `json:"is_system"`
}

// ContentGood — товар/ресурс: category — «голое» имя (резолв по goods.kind,
// §4); props переносятся дословно; code — обязательная метка (§3).
type ContentGood struct {
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	Category    string          `json:"category"`
	Description string          `json:"description,omitempty"`
	Volume      *float64        `json:"volume"`
	Weight      *float64        `json:"weight"`
	Props       json.RawMessage `json:"props,omitempty"`
	Source      string          `json:"source"`
	Code        string          `json:"code"`
}

// ContentRecipe — рецепт товара: good — метка товара-выхода.
type ContentRecipe struct {
	Good       string `json:"good"`
	Complexity *int   `json:"complexity"`
}

// ContentRecipeComponent — позиция состава: good — метка товара рецепта,
// component — метка составляющей (пусто = пустой слот).
type ContentRecipeComponent struct {
	Good          string `json:"good"`
	Pos           int    `json:"pos"`
	Component     string `json:"component,omitempty"`
	Quantity      int    `json:"quantity"`
	Reason        string `json:"reason,omitempty"`
	AllowResource bool   `json:"allow_resource"`
}

// ContentProducerType — тип производителя: parent — метка родителя,
// category — имя категории (kind=goods).
type ContentProducerType struct {
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	Category   string          `json:"category,omitempty"`
	Parent     string          `json:"parent,omitempty"`
	RaceFamily string          `json:"race_family,omitempty"`
	Race       string          `json:"race,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Params     json.RawMessage `json:"params,omitempty"`
	Hidden     bool            `json:"hidden"`
	Code       string          `json:"code"`
}

// ContentUnlockCategory — натуральный ключ категории в ссылке предмета (§4):
// (kind, name) — как у прочих ссылок на categories (§3.4).
type ContentUnlockCategory struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ContentUnlock — предмет-рецепт в снимке (§4): producer — метка типа
// производителя (`producer_types.code`), category — натуральный ключ.
type ContentUnlock struct {
	Producer string                `json:"producer"`
	Category ContentUnlockCategory `json:"category"`
}

// ContentItem — тип предмета (§4): unlocks несётся по метке/натуральному ключу
// (НЕ по id: в БД `items.unlocks` хранит внутренние id, они не переносимы,
// §4/§5 п.6/T15); params — дословно; code — обязательна.
type ContentItem struct {
	Name     string          `json:"name"`
	SlotType string          `json:"slot_type"`
	Unlocks  []ContentUnlock `json:"unlocks,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Code     string          `json:"code"`
}

// ContentProducerSlot — слот родителя: parent — метка, category — имя
// категории (kind='good', §4).
type ContentProducerSlot struct {
	Parent     string `json:"parent"`
	Category   string `json:"category"`
	RaceFamily string `json:"race_family,omitempty"`
	Race       string `json:"race,omitempty"`
	Hidden     bool   `json:"hidden"`
}

// ContentProducerRecipe — привязка «постройка × рецепт»: producer/good — метки.
type ContentProducerRecipe struct {
	Producer string   `json:"producer"`
	Good     string   `json:"good"`
	Rate     *float64 `json:"rate"`
}

// ContentProducerItem — связь «производитель предметов ↔ предмет»: метки.
type ContentProducerItem struct {
	Producer     string          `json:"producer"`
	Item         string          `json:"item"`
	Requirements json.RawMessage `json:"requirements,omitempty"`
}

// ContentEffectType — тип эффекта: params дословно; code — обязательна.
type ContentEffectType struct {
	Name   string          `json:"name"`
	Impact string          `json:"impact"`
	Params json.RawMessage `json:"params,omitempty"`
	Code   string          `json:"code"`
}

// ContentGenerationConfig — ссылка-якорь (§1.3): метка записи базового типа
// поселения (не сырой id); пусто — ключ не задан.
type ContentGenerationConfig struct {
	DefaultSettlementTypeID string `json:"default_settlement_type_id"`
}

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
			Parent:   prodCode[s.ParentID],
			Category: catName[s.CategoryID],
			Hidden:   s.Hidden,
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
