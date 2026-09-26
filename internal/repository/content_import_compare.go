// internal/repository/content_import_compare.go
// Сравнение полей и диффы дочерних наборов (план dry_run и применение).
package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/lib/pq"

	"zorion/internal/goodsstudio/contentio"
	"zorion/internal/goodsstudio/graph"
)

// --- сравнение полей ---

func categoryChanged(cur impCategory, c contentio.Category) bool {
	return cur.Name != c.Name || cur.IsSystem != c.IsSystem || cur.Code.String != c.Code
}

func goodChanged(st *importState, cur impGood, g contentio.Good) bool {
	vol, weight := 1.0, 1.0
	if g.Volume != nil {
		vol = *g.Volume
	}
	if g.Weight != nil {
		weight = *g.Weight
	}
	src := g.Source
	if src == "" {
		src = "manual"
	}
	if cur.Name != g.Name || cur.Kind != g.Kind || cur.Source != src ||
		cur.Description.String != g.Description || cur.Volume != vol || cur.Weight != weight ||
		!jsonEqual(cur.Props, g.Props) || cur.Code.String != g.Code {
		return true
	}
	return st.catKeyByID[cur.CategoryID] != catKeyOf(g.Kind, g.Category)
}

func producerChanged(st *importState, cur impProducer, p contentio.ProducerType) bool {
	if cur.Name != p.Name || cur.Kind != p.Kind || cur.Hidden != p.Hidden ||
		cur.RaceFamily.String != p.RaceFamily || cur.Race.String != p.Race ||
		cur.Section.String != p.Section || cur.Code.String != p.Code {
		return true
	}
	if !jsonEqual(cur.Output, p.Output) || !jsonEqual(cur.Input, p.Input) || !jsonEqual(cur.Params, p.Params) {
		return true
	}
	parentCode := ""
	if cur.ParentID.Valid {
		parentCode = st.prodCodeByID[cur.ParentID.Int64]
	}
	if parentCode != p.Parent {
		return true
	}
	catKey := catKey{}
	if cur.CategoryID.Valid {
		catKey = st.catKeyByID[cur.CategoryID.Int64]
	}
	if p.Category == "" {
		return cur.CategoryID.Valid
	}
	return catKey != catKeyOf(catKindOrDefault(p.CategoryKind), p.Category)
}

func itemChanged(st *importState, cur impItem, it contentio.Item) bool {
	if cur.Name != it.Name || cur.SlotType != it.SlotType || cur.Code.String != it.Code ||
		!jsonEqual(cur.Params, it.Params) {
		return true
	}
	curUnlocks, err := unlockCodes(cur.Unlocks, st)
	if err != nil {
		return true
	}
	// Сравниваем по НОРМАЛИЗОВАННОМУ натуральному ключу категории: в БД ключ
	// хранится как name_norm, а снимок несёт сырое имя (спека §4) — прямое
	// сравнение ломало бы идемпотентность для категорий с `name != name_norm`
	// (ресурсные «Минералы»/«Топливо»). См. unlockCodes.
	return !reflect.DeepEqual(curUnlocks, normalizeUnlockNames(it.Unlocks))
}

// normalizeUnlockNames — приводит имена категорий unlocks к name_norm, чтобы
// сравнение с текущей (БД) формой было устойчивым.
func normalizeUnlockNames(in []contentio.Unlock) []contentio.Unlock {
	if len(in) == 0 {
		return nil
	}
	out := make([]contentio.Unlock, len(in))
	copy(out, in)
	for i := range out {
		out[i].Category.Name = graph.NormalizeName(out[i].Category.Name)
	}
	return out
}

func effectChanged(cur impEffect, e contentio.EffectType) bool {
	params := e.Params
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	return cur.Name != e.Name || cur.Impact != e.Impact ||
		!jsonEqual(cur.Params, params) || cur.Code.String != e.Code
}

func intPtrEqual(cur sql.NullInt64, want *int) bool {
	if want == nil {
		return !cur.Valid
	}
	return cur.Valid && int(cur.Int64) == *want
}

// unlockCodes — id-форма items.unlocks (БД, §4) → переносимая форма (метка/ключ)
// для сравнения с снимком.
func unlockCodes(raw []byte, st *importState) ([]contentio.Unlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var elems []struct {
		ProducerTypeID int64 `json:"producer_type_id"`
		CategoryID     int64 `json:"category_id"`
	}
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, err
	}
	if len(elems) == 0 {
		return nil, nil
	}
	out := make([]contentio.Unlock, 0, len(elems))
	for _, e := range elems {
		code := st.prodCodeByID[e.ProducerTypeID]
		ck, ok := st.catKeyByID[e.CategoryID]
		if code == "" || !ok {
			return nil, fmt.Errorf("unlocks: ссылка (producer_type_id=%d, category_id=%d) не резолвится", e.ProducerTypeID, e.CategoryID)
		}
		out = append(out, contentio.Unlock{Producer: code, Category: contentio.UnlockCategory{Kind: ck.Kind, Name: ck.Name}})
	}
	return out, nil
}

// buildUnlocks — переносимая форма unlocks снимка → id-форма БД (§4/T15).
func buildUnlocks(it contentio.Item, prodID map[string]int64, catID map[catKey]int64) (interface{}, error) {
	if len(it.Unlocks) == 0 {
		return nil, nil
	}
	type rawUnlock struct {
		ProducerTypeID int64 `json:"producer_type_id"`
		CategoryID     int64 `json:"category_id"`
	}
	out := make([]rawUnlock, 0, len(it.Unlocks))
	for _, u := range it.Unlocks {
		if u.ProducerTypeID != nil || u.CategoryID != nil {
			return nil, errCatalog(400, "секция items, метка %s: unlocks в id-форме — отказ (§4/T15)", it.Code)
		}
		pid, ok := prodID[u.Producer]
		if !ok || pid == 0 {
			return nil, errCatalog(400, "секция items, метка %s: unlocks.producer %q не резолвится", it.Code, u.Producer)
		}
		cid, ok := catID[catKeyOf(u.Category.Kind, u.Category.Name)]
		if !ok || cid == 0 {
			return nil, errCatalog(400, "секция items, метка %s: unlocks.category %q не резолвится", it.Code, u.Category.Name)
		}
		out = append(out, rawUnlock{ProducerTypeID: pid, CategoryID: cid})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// --- желаемые листья ---

type compDesired struct {
	Pos           int
	Component     string
	Quantity      int
	Reason        string
	AllowResource bool
}

type slotDesired struct {
	Cat        catKey
	RaceFamily string
	Race       string
	Hidden     bool
}

type bindDesired struct {
	Good string
	Rate *float64
}

type pitemDesired struct {
	Item         string
	Requirements json.RawMessage
}

func desiredComponents(snap *contentio.Snapshot, good string) []compDesired {
	var out []compDesired
	for _, c := range snap.RecipeComponents {
		if c.Good != good {
			continue
		}
		out = append(out, compDesired{Pos: c.Pos, Component: c.Component, Quantity: c.Quantity, Reason: c.Reason, AllowResource: c.AllowResource})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pos < out[j].Pos })
	return out
}

func desiredSlots(snap *contentio.Snapshot, parent string) []slotDesired {
	var out []slotDesired
	for _, s := range snap.ProducerSlots {
		if s.Parent != parent {
			continue
		}
		out = append(out, slotDesired{Cat: catKeyOf(catKindOrDefault(s.CategoryKind), s.Category), RaceFamily: s.RaceFamily, Race: s.Race, Hidden: s.Hidden})
	}
	return out
}

func desiredBindings(snap *contentio.Snapshot, producer string) []bindDesired {
	var out []bindDesired
	for _, b := range snap.ProducerRecipes {
		if b.Producer != producer {
			continue
		}
		out = append(out, bindDesired{Good: b.Good, Rate: b.Rate})
	}
	return out
}

func desiredProducerItems(snap *contentio.Snapshot, producer string) []pitemDesired {
	var out []pitemDesired
	for _, pi := range snap.ProducerItems {
		if pi.Producer != producer {
			continue
		}
		out = append(out, pitemDesired{Item: pi.Item, Requirements: pi.Requirements})
	}
	return out
}

// --- диффы листьев ---

func diffComponents(current []impComponent, desired []compDesired, codeByID map[int64]string) (add, chg, rem int) {
	cur := make(map[int]impComponent, len(current))
	for _, c := range current {
		cur[c.Pos] = c
	}
	want := make(map[int]bool, len(desired))
	for _, d := range desired {
		want[d.Pos] = true
		c, ok := cur[d.Pos]
		if !ok {
			add++
			continue
		}
		code := ""
		if c.ComponentID.Valid {
			code = codeByID[c.ComponentID.Int64]
		}
		if code != d.Component || c.Quantity != d.Quantity || c.Reason.String != d.Reason || c.AllowResource != d.AllowResource {
			chg++
		}
	}
	for _, c := range current {
		if !want[c.Pos] {
			rem++
		}
	}
	return
}

func diffSlots(current []impSlot, desired []slotDesired, catKeyByID map[int64]catKey) (add, chg, rem int) {
	cur := make(map[string]impSlot, len(current))
	for _, s := range current {
		cur[slotKey(catKeyByID[s.CategoryID], s.RaceFamily.String, s.Race.String)] = s
	}
	want := make(map[string]bool, len(desired))
	for _, d := range desired {
		k := slotKey(d.Cat, d.RaceFamily, d.Race)
		want[k] = true
		c, ok := cur[k]
		if !ok {
			add++
			continue
		}
		if c.Hidden != d.Hidden {
			chg++
		}
	}
	for _, s := range current {
		if !want[slotKey(catKeyByID[s.CategoryID], s.RaceFamily.String, s.Race.String)] {
			rem++
		}
	}
	return
}

func slotKey(cat catKey, family, race string) string {
	return catKeyStr(cat) + "\x00" + family + "\x00" + race
}

func diffBindings(current []impBinding, desired []bindDesired, goodCodeByID map[int64]string) (add, chg, rem int) {
	cur := make(map[string]impBinding, len(current))
	for _, b := range current {
		cur[goodCodeByID[b.GoodID]] = b
	}
	want := make(map[string]bool, len(desired))
	for _, d := range desired {
		want[d.Good] = true
		c, ok := cur[d.Good]
		if !ok {
			add++
			continue
		}
		if !floatPtrEqual(c.Rate, d.Rate) {
			chg++
		}
	}
	for _, b := range current {
		if !want[goodCodeByID[b.GoodID]] {
			rem++
		}
	}
	return
}

func diffProducerItems(current []impProducerItem, desired []pitemDesired, itemCodeByID map[int64]string) (add, chg, rem int) {
	cur := make(map[string]impProducerItem, len(current))
	for _, pi := range current {
		cur[itemCodeByID[pi.ItemID]] = pi
	}
	want := make(map[string]bool, len(desired))
	for _, d := range desired {
		want[d.Item] = true
		c, ok := cur[d.Item]
		if !ok {
			add++
			continue
		}
		if !jsonEqual(c.Requirements, d.Requirements) {
			chg++
		}
	}
	for _, pi := range current {
		if !want[itemCodeByID[pi.ItemID]] {
			rem++
		}
	}
	return
}

func floatPtrEqual(cur sql.NullFloat64, want *float64) bool {
	if want == nil {
		return !cur.Valid
	}
	return cur.Valid && cur.Float64 == *want
}

// --- помощники ---

func nullText(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func jsonParam(v json.RawMessage) interface{} {
	if len(v) == 0 {
		return nil
	}
	return string(v)
}

func nullFloat(v *float64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func intPtrParam(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func jsonEqual(a, b []byte) bool {
	return reflect.DeepEqual(canonJSON(a), canonJSON(b))
}

func canonJSON(raw []byte) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

// wrapImportErr — ошибки целостности/данных PostgreSQL → понятный 4xx
// (страховка к §5.4/§5.5): UNIQUE/FK → 409, CHECK/NOT NULL/иная целостность и
// ошибки данных (классы 23/22) → 400; прочее остаётся 500.
func wrapImportErr(err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch {
		case pqErr.Code == "23505":
			return errCatalog(409, "нарушение уникальности при импорте: "+err.Error())
		case pqErr.Code == "23503":
			return errCatalog(409, "нарушение ссылочной целостности при импорте: "+err.Error())
		case strings.HasPrefix(string(pqErr.Code), "23"):
			return errCatalog(400, "нарушение ограничения целостности при импорте: "+err.Error())
		case strings.HasPrefix(string(pqErr.Code), "22"):
			return errCatalog(400, "некорректные данные при импорте: "+err.Error())
		}
	}
	return err
}

// Порядок типов — в content_import_plan.go (orderSnapshotProducers,
// orderProducersDeepFirst).
