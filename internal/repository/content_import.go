// internal/repository/content_import.go
// Импорт снимка контента каталога dev → прод (спека 2026-09-24-каталог-
// экспорт-импорт-контента-на-прод §5, итерация И3): одна транзакция с
// pg_advisory_xact_lock (beginMutation), режим сопоставления по метке (штатный)
// или по (kind, name_norm) (первичная накатка, §3.5), upsert без смены `id`
// существующих записей, полная замена отсутствующего в файле с предсчётом
// мировых ссылок (§5.4), сухой прогон (ROLLBACK) и отчёт (§5.5).
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
	"zorion/internal/models"
)

// Секции отчёта/диффа (совпадают с ключами `counts` снимка, §4).
const (
	secCategories       = "categories"
	secGoods            = "goods"
	secRecipes          = "recipes"
	secRecipeComponents = "recipe_components"
	secProducerTypes    = "producer_types"
	secItems            = "items"
	secProducerSlots    = "producer_slots"
	secProducerRecipes  = "producer_recipes"
	secProducerItems    = "producer_items"
	secEffectTypes      = "effect_types"
)

// ImportDiffEntry — запись в перечне диффа/расхождений (§5.2): секция, метка
// (у categories метки нет — пусто), имя.
type ImportDiffEntry struct {
	Section string `json:"section"`
	Code    string `json:"code,omitempty"`
	Name    string `json:"name"`
}

// ImportBlocked — удаление, упёршееся в мировую ссылку (§5.4): таблица-ссылка
// и число ссылок.
type ImportBlocked struct {
	Section  string `json:"section"`
	Code     string `json:"code,omitempty"`
	Name     string `json:"name"`
	RefTable string `json:"ref_table"`
	RefCount int    `json:"ref_count"`
}

// ImportRemapEntry — метка цели, которая будет переназначена по файлу при
// первичной накатке (§3.5/§5.2): та же запись по `(kind, name_norm)`, но другой
// (или пустой) `code`.
type ImportRemapEntry struct {
	Section string `json:"section"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	OldCode string `json:"old_code,omitempty"`
}

// ImportDiff — сухой прогон (§5.2): режим, числа создать/обновить, перечень
// удаляемых записей, заблокированные мировыми ссылками, расхождения тождества,
// переназначаемые метки (накатка).
type ImportDiff struct {
	Mode      string             `json:"mode"`
	Create    map[string]int     `json:"create"`
	Update    map[string]int     `json:"update"`
	Delete    []ImportDiffEntry  `json:"delete"`
	Blocked   []ImportBlocked    `json:"blocked"`
	Unmatched []ImportDiffEntry  `json:"unmatched"`
	Remap     []ImportRemapEntry `json:"remap"`
}

// ImportReport — отчёт успешного применения (§5.5): числа по секциям.
type ImportReport struct {
	Created  map[string]int `json:"created"`
	Updated  map[string]int `json:"updated"`
	Deleted  map[string]int `json:"deleted"`
	Warnings []string       `json:"warnings"`
}

// ImportResult — итог импорта: дифф (всегда) и отчёт (при применении).
type ImportResult struct {
	Mode   string        `json:"mode"`
	DryRun bool          `json:"dry_run"`
	Diff   *ImportDiff   `json:"diff,omitempty"`
	Report *ImportReport `json:"report,omitempty"`
}

// --- строки целевой БД (с name_norm — для сопоставления §3.5) ---

type impCategory struct {
	ID       int64
	Name     string
	NameNorm string
	Kind     string
	Code     sql.NullString
	IsSystem bool
}

type impGood struct {
	ID          int64
	Name        string
	NameNorm    string
	CategoryID  int64
	Kind        string
	Source      string
	Description sql.NullString
	Volume      float64
	Weight      float64
	Props       []byte
	Code        sql.NullString
}

type impRecipe struct {
	ID         int64
	GoodID     int64
	Complexity sql.NullInt64
}

type impComponent struct {
	RecipeID      int64
	Pos           int
	ComponentID   sql.NullInt64
	Quantity      int
	Reason        sql.NullString
	AllowResource bool
}

type impProducer struct {
	ID         int64
	Name       string
	NameNorm   string
	Kind       string
	CategoryID sql.NullInt64
	ParentID   sql.NullInt64
	RaceFamily sql.NullString
	Race       sql.NullString
	Output     []byte
	Input      []byte
	Params     []byte
	Hidden     bool
	Section    sql.NullString
	Code       sql.NullString
}

type impItem struct {
	ID       int64
	Name     string
	NameNorm string
	SlotType string
	Unlocks  []byte
	Params   []byte
	Code     sql.NullString
}

type impSlot struct {
	ParentID   int64
	CategoryID int64
	RaceFamily sql.NullString
	Race       sql.NullString
	Hidden     bool
}

type impBinding struct {
	ProducerTypeID int64
	RecipeID       int64
	GoodID         int64
	Rate           sql.NullFloat64
}

type impProducerItem struct {
	ProducerTypeID int64
	ItemID         int64
	Requirements   []byte
}

type impEffect struct {
	ID       int64
	Name     string
	NameNorm string
	Impact   string
	Params   []byte
	Code     sql.NullString
}

// catKey — натуральный ключ категории (§3.4): (kind, name_norm).
type catKey struct {
	Kind string
	Name string
}

func catKeyOf(kind, name string) catKey { return catKey{Kind: kind, Name: graph.NormalizeName(name)} }
func catKeyStr(k catKey) string         { return k.Kind + "\x00" + k.Name }

// catKindOrDefault — kind категории из снимка; пусто (старый файл) → «good»
// (обратная совместимость, §4).
func catKindOrDefault(kind string) string {
	if kind == "" {
		return "good"
	}
	return kind
}

// catExists — есть ли категория (kind, name_norm) в множестве снимка. Пустой
// kind (старый файл без `category_kind`) трактуется ТАК ЖЕ, как в apply
// (`catKindOrDefault` → «good») — иначе валидация пропускала бы resource-имя,
// а применение падало FK-ошибкой (рассинхрон фолбэка).
func catExists(set map[catKey]bool, kind, name string) bool {
	return set[catKeyOf(catKindOrDefault(kind), name)]
}

// goodKey/prodKey — ключи первичной накатки (§3.5).
type goodKey struct {
	Kind string
	Name string
}

func goodKeyOf(kind, name string) goodKey {
	return goodKey{Kind: kind, Name: graph.NormalizeName(name)}
}

type prodKey struct {
	Kind string
	Name string
}

func prodKeyOf(kind, name string) prodKey {
	return prodKey{Kind: kind, Name: graph.NormalizeName(name)}
}

// --- загрузка целевых строк ---

func loadImportCategories(q queryer) ([]impCategory, error) {
	rows, err := q.Query(`SELECT id, name, name_norm, kind, code, is_system FROM categories ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []impCategory
	for rows.Next() {
		var c impCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.NameNorm, &c.Kind, &c.Code, &c.IsSystem); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadImportGoods(q queryer) ([]impGood, error) {
	rows, err := q.Query(
		`SELECT id, name, name_norm, category_id, kind, source, description, volume, weight, props, code
		 FROM goods ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []impGood
	for rows.Next() {
		var g impGood
		if err := rows.Scan(&g.ID, &g.Name, &g.NameNorm, &g.CategoryID, &g.Kind, &g.Source,
			&g.Description, &g.Volume, &g.Weight, &g.Props, &g.Code); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func loadImportRecipes(q queryer) ([]impRecipe, map[int64][]impComponent, error) {
	rs, err := q.Query(`SELECT id, good_id, complexity FROM recipes ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rs.Close()
	var recipes []impRecipe
	for rs.Next() {
		var r impRecipe
		if err := rs.Scan(&r.ID, &r.GoodID, &r.Complexity); err != nil {
			return nil, nil, err
		}
		recipes = append(recipes, r)
	}
	if err := rs.Err(); err != nil {
		return nil, nil, err
	}

	cs, err := q.Query(
		`SELECT recipe_id, pos, component_id, quantity, reason, allow_resource
		 FROM recipe_components ORDER BY recipe_id, pos`)
	if err != nil {
		return nil, nil, err
	}
	defer cs.Close()
	comps := make(map[int64][]impComponent)
	for cs.Next() {
		var c impComponent
		if err := cs.Scan(&c.RecipeID, &c.Pos, &c.ComponentID, &c.Quantity, &c.Reason, &c.AllowResource); err != nil {
			return nil, nil, err
		}
		comps[c.RecipeID] = append(comps[c.RecipeID], c)
	}
	return recipes, comps, cs.Err()
}

func loadImportProducers(q queryer) ([]impProducer, error) {
	rows, err := q.Query(
		`SELECT id, name, name_norm, kind, category_id, parent_id, race_family, race, output, input, params, hidden, section, code
		 FROM producer_types ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []impProducer
	for rows.Next() {
		var p impProducer
		if err := rows.Scan(&p.ID, &p.Name, &p.NameNorm, &p.Kind, &p.CategoryID, &p.ParentID,
			&p.RaceFamily, &p.Race, &p.Output, &p.Input, &p.Params, &p.Hidden, &p.Section, &p.Code); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func loadImportItems(q queryer) ([]impItem, error) {
	rows, err := q.Query(`SELECT id, name, name_norm, slot_type, unlocks, params, code FROM items ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []impItem
	for rows.Next() {
		var it impItem
		if err := rows.Scan(&it.ID, &it.Name, &it.NameNorm, &it.SlotType, &it.Unlocks, &it.Params, &it.Code); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func loadImportSlots(q queryer) (map[int64][]impSlot, error) {
	rows, err := q.Query(`SELECT parent_id, category_id, race_family, race, hidden FROM producer_slots ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]impSlot)
	for rows.Next() {
		var s impSlot
		if err := rows.Scan(&s.ParentID, &s.CategoryID, &s.RaceFamily, &s.Race, &s.Hidden); err != nil {
			return nil, err
		}
		out[s.ParentID] = append(out[s.ParentID], s)
	}
	return out, rows.Err()
}

func loadImportBindings(q queryer) (map[int64][]impBinding, error) {
	rows, err := q.Query(
		`SELECT pr.producer_type_id, pr.recipe_id, r.good_id, pr.rate
		 FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, r.good_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]impBinding)
	for rows.Next() {
		var b impBinding
		if err := rows.Scan(&b.ProducerTypeID, &b.RecipeID, &b.GoodID, &b.Rate); err != nil {
			return nil, err
		}
		out[b.ProducerTypeID] = append(out[b.ProducerTypeID], b)
	}
	return out, rows.Err()
}

func loadImportProducerItems(q queryer) (map[int64][]impProducerItem, error) {
	rows, err := q.Query(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]impProducerItem)
	for rows.Next() {
		var pi impProducerItem
		if err := rows.Scan(&pi.ProducerTypeID, &pi.ItemID, &pi.Requirements); err != nil {
			return nil, err
		}
		out[pi.ProducerTypeID] = append(out[pi.ProducerTypeID], pi)
	}
	return out, rows.Err()
}

func loadImportEffects(q queryer) ([]impEffect, error) {
	rows, err := q.Query(`SELECT id, name, name_norm, impact, params, code FROM effect_types ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []impEffect
	for rows.Next() {
		var e impEffect
		if err := rows.Scan(&e.ID, &e.Name, &e.NameNorm, &e.Impact, &e.Params, &e.Code); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- состояние импорта ---

type importState struct {
	mode string

	cats      []impCategory
	goods     []impGood
	recipes   []impRecipe
	producers []impProducer
	items     []impItem
	effects   []impEffect
	comps     map[int64][]impComponent
	slots     map[int64][]impSlot
	binds     map[int64][]impBinding
	pitems    map[int64][]impProducerItem

	catByKey     map[catKey]impCategory
	catByID      map[int64]impCategory
	catKeyByID   map[int64]catKey
	goodByCode   map[string]impGood
	goodByKey    map[goodKey]impGood
	goodByID     map[int64]impGood
	goodCodeByID map[int64]string
	prodByCode   map[string]impProducer
	prodByKey    map[prodKey]impProducer
	prodByID     map[int64]impProducer
	prodCodeByID map[int64]string
	itemByCode   map[string]impItem
	itemByName   map[string]impItem
	itemByID     map[int64]impItem
	itemCodeByID map[int64]string
	effectByCode map[string]impEffect
	effectByName map[string]impEffect
	effectByID   map[int64]impEffect
	recipeByGood map[int64]impRecipe
}

func (st *importState) buildIndexes() {
	st.catByKey = make(map[catKey]impCategory, len(st.cats))
	st.catByID = make(map[int64]impCategory, len(st.cats))
	st.catKeyByID = make(map[int64]catKey, len(st.cats))
	for _, c := range st.cats {
		k := catKey{Kind: c.Kind, Name: c.NameNorm}
		st.catByKey[k] = c
		st.catByID[c.ID] = c
		st.catKeyByID[c.ID] = k
	}
	st.goodByCode = make(map[string]impGood, len(st.goods))
	st.goodByKey = make(map[goodKey]impGood, len(st.goods))
	st.goodByID = make(map[int64]impGood, len(st.goods))
	st.goodCodeByID = make(map[int64]string, len(st.goods))
	for _, g := range st.goods {
		if g.Code.Valid {
			st.goodByCode[g.Code.String] = g
			st.goodCodeByID[g.ID] = g.Code.String
		}
		st.goodByKey[goodKey{Kind: g.Kind, Name: g.NameNorm}] = g
		st.goodByID[g.ID] = g
	}
	st.prodByCode = make(map[string]impProducer, len(st.producers))
	st.prodByKey = make(map[prodKey]impProducer, len(st.producers))
	st.prodByID = make(map[int64]impProducer, len(st.producers))
	st.prodCodeByID = make(map[int64]string, len(st.producers))
	for _, p := range st.producers {
		if p.Code.Valid {
			st.prodByCode[p.Code.String] = p
			st.prodCodeByID[p.ID] = p.Code.String
		}
		st.prodByKey[prodKey{Kind: p.Kind, Name: p.NameNorm}] = p
		st.prodByID[p.ID] = p
	}
	st.itemByCode = make(map[string]impItem, len(st.items))
	st.itemByName = make(map[string]impItem, len(st.items))
	st.itemByID = make(map[int64]impItem, len(st.items))
	st.itemCodeByID = make(map[int64]string, len(st.items))
	for _, it := range st.items {
		if it.Code.Valid {
			st.itemByCode[it.Code.String] = it
			st.itemCodeByID[it.ID] = it.Code.String
		}
		st.itemByName[it.NameNorm] = it
		st.itemByID[it.ID] = it
	}
	st.effectByCode = make(map[string]impEffect, len(st.effects))
	st.effectByName = make(map[string]impEffect, len(st.effects))
	st.effectByID = make(map[int64]impEffect, len(st.effects))
	for _, e := range st.effects {
		if e.Code.Valid {
			st.effectByCode[e.Code.String] = e
		}
		st.effectByName[e.NameNorm] = e
		st.effectByID[e.ID] = e
	}
	st.recipeByGood = make(map[int64]impRecipe, len(st.recipes))
	for _, r := range st.recipes {
		st.recipeByGood[r.GoodID] = r
	}
}

func (st *importState) matchGood(g contentio.Good) (impGood, bool) {
	if st.mode == "bootstrap" {
		row, ok := st.goodByKey[goodKeyOf(g.Kind, g.Name)]
		return row, ok
	}
	row, ok := st.goodByCode[g.Code]
	return row, ok
}

func (st *importState) matchProducer(p contentio.ProducerType) (impProducer, bool) {
	if st.mode == "bootstrap" {
		row, ok := st.prodByKey[prodKeyOf(p.Kind, p.Name)]
		return row, ok
	}
	row, ok := st.prodByCode[p.Code]
	return row, ok
}

func (st *importState) matchItem(it contentio.Item) (impItem, bool) {
	if st.mode == "bootstrap" {
		row, ok := st.itemByName[graph.NormalizeName(it.Name)]
		return row, ok
	}
	row, ok := st.itemByCode[it.Code]
	return row, ok
}

func (st *importState) matchEffect(e contentio.EffectType) (impEffect, bool) {
	if st.mode == "bootstrap" {
		row, ok := st.effectByName[graph.NormalizeName(e.Name)]
		return row, ok
	}
	row, ok := st.effectByCode[e.Code]
	return row, ok
}

// --- импорт ---

// ImportContent — импорт снимка (§5). Одна транзакция с advisory-локом
// каталога; dryRun — тот же прогон с ROLLBACK. При dry_run дифф возвращается
// без ошибки даже при blocked/unmatched; при применении blocked/unmatched дают
// 409 и полный откат (никакого частичного применения, §5.4/§5.5).
func (r *GoodsRepository) ImportContent(snap *contentio.Snapshot, dryRun bool) (*ImportResult, error) {
	if snap == nil {
		return nil, errCatalog(400, "пустой снимок")
	}
	if snap.SchemaVersion > contentio.SupportedSchemaVersion {
		return nil, errCatalog(400, fmt.Sprintf("schema_version %d не поддерживается (максимум %d)",
			snap.SchemaVersion, contentio.SupportedSchemaVersion))
	}

	tx, err := r.beginMutation()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res := &ImportResult{DryRun: dryRun, Diff: &ImportDiff{
		Create: map[string]int{}, Update: map[string]int{},
		Delete: []ImportDiffEntry{}, Blocked: []ImportBlocked{}, Unmatched: []ImportDiffEntry{},
		Remap: []ImportRemapEntry{},
	}}
	diff := res.Diff

	st, err := loadImportState(tx)
	if err != nil {
		return nil, err
	}
	mode, unmatched := resolveImportMode(st, snap)
	st.mode = mode
	diff.Mode, res.Mode = mode, mode
	diff.Unmatched = unmatched

	snapProducers := orderSnapshotProducers(snap.ProducerTypes)
	if err := validateSnapshotRefs(snap, snapProducers); err != nil {
		return nil, err
	}
	st.buildIndexes()

	plan, err := buildImportPlan(st, snap, snapProducers, diff)
	if err != nil {
		return nil, err
	}
	if err := collectImportDeletions(st, plan, diff, tx); err != nil {
		return nil, err
	}

	if !dryRun && (len(diff.Blocked) > 0 || len(diff.Unmatched) > 0) {
		return res, errCatalog(409, fmt.Sprintf("импорт отклонён: заблокировано %d, расхождений %d (§5.4)",
			len(diff.Blocked), len(diff.Unmatched)))
	}
	if dryRun {
		return res, nil
	}

	report := &ImportReport{Created: map[string]int{}, Updated: map[string]int{}, Deleted: map[string]int{}}
	res.Report = report
	if err := applyImport(tx, st, snap, snapProducers, plan, report); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

func loadImportState(q queryer) (*importState, error) {
	st := &importState{}
	var err error
	if st.cats, err = loadImportCategories(q); err != nil {
		return nil, err
	}
	if st.goods, err = loadImportGoods(q); err != nil {
		return nil, err
	}
	if st.recipes, st.comps, err = loadImportRecipes(q); err != nil {
		return nil, err
	}
	if st.producers, err = loadImportProducers(q); err != nil {
		return nil, err
	}
	if st.items, err = loadImportItems(q); err != nil {
		return nil, err
	}
	if st.slots, err = loadImportSlots(q); err != nil {
		return nil, err
	}
	if st.binds, err = loadImportBindings(q); err != nil {
		return nil, err
	}
	if st.pitems, err = loadImportProducerItems(q); err != nil {
		return nil, err
	}
	if st.effects, err = loadImportEffects(q); err != nil {
		return nil, err
	}
	return st, nil
}

// resolveImportMode — режим сопоставления (§3.5/§5 п.2) по состоянию целевых
// контентных таблиц:
//   - смесь (часть с меткой, часть без) → unmatched + отказ;
//   - меток нет вовсе (чистая цель) → bootstrap (накатка по имени);
//   - метки есть у всех и тождество согласовано с файлом → managed (по метке);
//   - метки есть, но тождество расходится → bootstrap (накатка по имени,
//     метки цели переназначаются по файлу, локальные id сохраняются).
//
// «Расхождение тождества»: хотя бы одна размеченная запись цели такая, что
// её `code` есть в файле, но натуральный ключ `(kind, name_norm)` не совпадает
// (метка указывает не туда), ИЛИ её `code` в файле отсутствует, а натуральный
// ключ есть (та же запись в файле под другой меткой). Это ровно случай F2:
// бэкфилл раздал метки по `id`, а `id` dev и свежего прода разные.
func resolveImportMode(st *importState, snap *contentio.Snapshot) (string, []ImportDiffEntry) {
	labeled, unlabeled := 0, 0
	var unmatched []ImportDiffEntry
	mark := func(code sql.NullString, section, name string) {
		if code.Valid && code.String != "" {
			labeled++
			return
		}
		unlabeled++
		unmatched = append(unmatched, ImportDiffEntry{Section: section, Name: name})
	}
	for _, g := range st.goods {
		mark(g.Code, secGoods, g.Name)
	}
	for _, p := range st.producers {
		mark(p.Code, secProducerTypes, p.Name)
	}
	for _, it := range st.items {
		mark(it.Code, secItems, it.Name)
	}
	for _, e := range st.effects {
		mark(e.Code, secEffectTypes, e.Name)
	}
	switch {
	case labeled > 0 && unlabeled > 0:
		return "unmatched", unmatched
	case labeled == 0 && unlabeled > 0:
		return "bootstrap", nil
	case labeled == 0:
		return "managed", nil // пустая цель: все записи — новые, режим не важен
	}
	if identityConsistent(st, snap) {
		return "managed", nil
	}
	return "bootstrap", nil
}

// identityConsistent — согласовано ли тождество «метка ↔ запись» между целью и
// файлом по четырём таблицам (§3.5): не существует натурального ключа
// `(kind, name_norm)`, который есть и в файле, и в цели, но связан там и там с
// **разными** метками. (Переименование при той же метке расхождением НЕ
// считается: метка нашлась в цели — это штатный `managed`, §3.5 п.1.)
func identityConsistent(st *importState, snap *contentio.Snapshot) bool {
	fileGood := make(map[goodKey]string, len(snap.Goods))
	for _, g := range snap.Goods {
		fileGood[goodKeyOf(g.Kind, g.Name)] = g.Code
	}
	for _, g := range st.goods {
		if !g.Code.Valid {
			continue
		}
		if fc, ok := fileGood[goodKey{Kind: g.Kind, Name: g.NameNorm}]; ok && fc != g.Code.String {
			return false
		}
	}

	fileProd := make(map[prodKey]string, len(snap.ProducerTypes))
	for _, p := range snap.ProducerTypes {
		fileProd[prodKeyOf(p.Kind, p.Name)] = p.Code
	}
	for _, p := range st.producers {
		if !p.Code.Valid {
			continue
		}
		if fc, ok := fileProd[prodKey{Kind: p.Kind, Name: p.NameNorm}]; ok && fc != p.Code.String {
			return false
		}
	}

	fileItem := make(map[string]string, len(snap.Items))
	for _, it := range snap.Items {
		fileItem[graph.NormalizeName(it.Name)] = it.Code
	}
	for _, it := range st.items {
		if !it.Code.Valid {
			continue
		}
		if fc, ok := fileItem[it.NameNorm]; ok && fc != it.Code.String {
			return false
		}
	}

	fileEffect := make(map[string]string, len(snap.EffectTypes))
	for _, e := range snap.EffectTypes {
		fileEffect[graph.NormalizeName(e.Name)] = e.Code
	}
	for _, e := range st.effects {
		if !e.Code.Valid {
			continue
		}
		if fc, ok := fileEffect[e.NameNorm]; ok && fc != e.Code.String {
			return false
		}
	}
	return true
}

// --- валидация ссылок снимка ---

func validateSnapshotRefs(snap *contentio.Snapshot, producers []contentio.ProducerType) error {
	catSet := make(map[catKey]bool, len(snap.Categories))
	for _, c := range snap.Categories {
		k := catKeyOf(c.Kind, c.Name)
		if catSet[k] {
			return errCatalog(400, "секция categories: дубль категории %q (%s)", c.Name, c.Kind)
		}
		catSet[k] = true
	}
	goodSet := make(map[string]contentio.Good, len(snap.Goods))
	for _, g := range snap.Goods {
		if g.Code == "" {
			return errCatalog(400, "секция goods: пустая метка у %q", g.Name)
		}
		if _, dup := goodSet[g.Code]; dup {
			return errCatalog(400, "секция goods: дубль метки %s", g.Code)
		}
		goodSet[g.Code] = g
		if !catSet[catKeyOf(g.Kind, g.Category)] {
			return errCatalog(400, "секция goods, метка %s: категория %q (%s) не найдена", g.Code, g.Category, g.Kind)
		}
	}
	prodSet := make(map[string]bool, len(producers))
	for _, p := range producers {
		if p.Code == "" {
			return errCatalog(400, "секция producer_types: пустая метка у %q", p.Name)
		}
		if prodSet[p.Code] {
			return errCatalog(400, "секция producer_types: дубль метки %s", p.Code)
		}
		prodSet[p.Code] = true
	}
	for _, p := range producers {
		if p.Parent != "" && !prodSet[p.Parent] {
			return errCatalog(400, "секция producer_types, метка %s: родитель %s не найден", p.Code, p.Parent)
		}
		if p.Category != "" && !catExists(catSet, p.CategoryKind, p.Category) {
			return errCatalog(400, "секция producer_types, метка %s: категория %q (%s) не найдена", p.Code, p.Category, catKindOrDefault(p.CategoryKind))
		}
	}
	itemSet := make(map[string]bool, len(snap.Items))
	for _, it := range snap.Items {
		if it.Code == "" {
			return errCatalog(400, "секция items: пустая метка у %q", it.Name)
		}
		if itemSet[it.Code] {
			return errCatalog(400, "секция items: дубль метки %s", it.Code)
		}
		itemSet[it.Code] = true
		for _, u := range it.Unlocks {
			if u.ProducerTypeID != nil || u.CategoryID != nil {
				return errCatalog(400, "секция items, метка %s: unlocks в id-форме (producer_type_id/category_id) — отказ (§4/T15)", it.Code)
			}
			if u.Producer == "" || !prodSet[u.Producer] {
				return errCatalog(400, "секция items, метка %s: unlocks.producer %q не найден", it.Code, u.Producer)
			}
			if !catSet[catKeyOf(u.Category.Kind, u.Category.Name)] {
				return errCatalog(400, "секция items, метка %s: unlocks.category %q (%s) не найдена", it.Code, u.Category.Name, u.Category.Kind)
			}
		}
	}
	for _, rec := range snap.Recipes {
		if _, ok := goodSet[rec.Good]; !ok {
			return errCatalog(400, "секция recipes: товар-выход %s не найден", rec.Good)
		}
	}
	for _, c := range snap.RecipeComponents {
		if _, ok := goodSet[c.Good]; !ok {
			return errCatalog(400, "секция recipe_components: товар рецепта %s не найден", c.Good)
		}
		if c.Component != "" {
			if _, ok := goodSet[c.Component]; !ok {
				return errCatalog(400, "секция recipe_components: компонент %s не найден", c.Component)
			}
		}
	}
	recipeGoods := make(map[string]bool, len(snap.Recipes))
	for _, rec := range snap.Recipes {
		recipeGoods[rec.Good] = true
	}
	for _, s := range snap.ProducerSlots {
		if !prodSet[s.Parent] {
			return errCatalog(400, "секция producer_slots: родитель %s не найден", s.Parent)
		}
		if !catExists(catSet, s.CategoryKind, s.Category) {
			return errCatalog(400, "секция producer_slots: категория %q (%s) не найдена", s.Category, catKindOrDefault(s.CategoryKind))
		}
	}
	for _, b := range snap.ProducerRecipes {
		if !prodSet[b.Producer] {
			return errCatalog(400, "секция producer_recipes: постройка %s не найдена", b.Producer)
		}
		if _, ok := goodSet[b.Good]; !ok {
			return errCatalog(400, "секция producer_recipes: товар %s не найден", b.Good)
		}
		if !recipeGoods[b.Good] {
			return errCatalog(400, "секция producer_recipes: у товара %s нет рецепта", b.Good)
		}
	}
	for _, pi := range snap.ProducerItems {
		if !prodSet[pi.Producer] {
			return errCatalog(400, "секция producer_items: постройка %s не найдена", pi.Producer)
		}
		if !itemSet[pi.Item] {
			return errCatalog(400, "секция producer_items: предмет %s не найден", pi.Item)
		}
	}
	for _, e := range snap.EffectTypes {
		if e.Code == "" {
			return errCatalog(400, "секция effect_types: пустая метка у %q", e.Name)
		}
	}
	if anchor := snap.GenerationConfig.DefaultSettlementTypeID; anchor != "" && !prodSet[anchor] {
		return errCatalog(400, "generation_config.default_settlement_type_id: метка %s не найдена среди producer_types", anchor)
	}
	return nil
}

// --- план ---

type importPlan struct {
	catID       map[catKey]int64
	catFound    map[catKey]bool
	goodID      map[string]int64 // существующие/новые id (новые — 0 на фазе плана)
	goodNeeds   map[string]bool
	prodID      map[string]int64
	prodNeeds   map[string]bool
	itemID      map[string]int64
	itemNeeds   map[string]bool
	effectID    map[string]int64
	effectNeeds map[string]bool
	recipeID    map[string]int64 // существующие рецепты, оставленные файлом

	claimedGoods   map[int64]bool
	claimedProds   map[int64]bool
	claimedItems   map[int64]bool
	claimedEffects map[int64]bool
}

func buildImportPlan(st *importState, snap *contentio.Snapshot, producers []contentio.ProducerType, diff *ImportDiff) (*importPlan, error) {
	plan := &importPlan{
		catID: map[catKey]int64{}, catFound: map[catKey]bool{},
		goodID: map[string]int64{}, goodNeeds: map[string]bool{},
		prodID: map[string]int64{}, prodNeeds: map[string]bool{},
		itemID: map[string]int64{}, itemNeeds: map[string]bool{},
		effectID: map[string]int64{}, effectNeeds: map[string]bool{},
		recipeID:       map[string]int64{},
		claimedGoods:   map[int64]bool{},
		claimedProds:   map[int64]bool{},
		claimedItems:   map[int64]bool{},
		claimedEffects: map[int64]bool{},
	}

	for _, c := range snap.Categories {
		k := catKeyOf(c.Kind, c.Name)
		if cur, ok := st.catByKey[k]; ok {
			plan.catID[k] = cur.ID
			plan.catFound[k] = true
			if categoryChanged(cur, c) {
				diff.Update[secCategories]++
			}
		} else {
			diff.Create[secCategories]++
		}
	}

	for _, g := range snap.Goods {
		if cur, ok := st.matchGood(g); ok {
			plan.goodID[g.Code] = cur.ID
			plan.claimedGoods[cur.ID] = true
			if st.mode == "bootstrap" {
				// сравнение листьев (компоненты/привязки) — по БУДУЩЕЙ метке
				// (файловой), иначе расхождение метки самого товара даёт ложный
				// «update» у его листьев
				st.goodCodeByID[cur.ID] = g.Code
				if codeOrEmpty(cur.Code) != g.Code {
					diff.Remap = append(diff.Remap, ImportRemapEntry{Section: secGoods, Code: g.Code, Name: g.Name, OldCode: codeOrEmpty(cur.Code)})
				}
			}
			if goodChanged(st, cur, g) {
				diff.Update[secGoods]++
				plan.goodNeeds[g.Code] = true
			}
		} else {
			diff.Create[secGoods]++
		}
	}

	for _, rec := range snap.Recipes {
		gid := plan.goodID[rec.Good]
		if gid == 0 {
			diff.Create[secRecipes]++ // товар новый → рецепт новый
			continue
		}
		if cur, ok := st.recipeByGood[gid]; ok {
			plan.recipeID[rec.Good] = cur.ID
			if !intPtrEqual(cur.Complexity, rec.Complexity) {
				diff.Update[secRecipes]++
			}
		} else {
			diff.Create[secRecipes]++
		}
	}

	for _, rec := range snap.Recipes {
		rid := plan.recipeID[rec.Good]
		desired := desiredComponents(snap, rec.Good)
		if plan.goodID[rec.Good] == 0 {
			diff.Create[secRecipeComponents] += len(desired)
			continue
		}
		if rid == 0 {
			diff.Create[secRecipeComponents] += len(desired)
			continue
		}
		add, chg, rem := diffComponents(st.comps[rid], desired, st.goodCodeByID)
		diff.Create[secRecipeComponents] += add
		diff.Update[secRecipeComponents] += chg
		for i := 0; i < rem; i++ {
			diff.Delete = append(diff.Delete, ImportDiffEntry{Section: secRecipeComponents, Code: rec.Good, Name: rec.Good})
		}
	}

	for _, p := range producers {
		if cur, ok := st.matchProducer(p); ok {
			plan.prodID[p.Code] = cur.ID
			plan.claimedProds[cur.ID] = true
			if st.mode == "bootstrap" {
				// будущая метка — чтобы дочерние/родительские сравнения и
				// unlocks не видели старое (переназначаемое) значение
				st.prodCodeByID[cur.ID] = p.Code
				if codeOrEmpty(cur.Code) != p.Code {
					diff.Remap = append(diff.Remap, ImportRemapEntry{Section: secProducerTypes, Code: p.Code, Name: p.Name, OldCode: codeOrEmpty(cur.Code)})
				}
			}
			if producerChanged(st, cur, p) {
				diff.Update[secProducerTypes]++
				plan.prodNeeds[p.Code] = true
			}
		} else {
			diff.Create[secProducerTypes]++
		}
	}

	for _, it := range snap.Items {
		if cur, ok := st.matchItem(it); ok {
			plan.itemID[it.Code] = cur.ID
			plan.claimedItems[cur.ID] = true
			if st.mode == "bootstrap" && codeOrEmpty(cur.Code) != it.Code {
				diff.Remap = append(diff.Remap, ImportRemapEntry{Section: secItems, Code: it.Code, Name: it.Name, OldCode: codeOrEmpty(cur.Code)})
			}
			if itemChanged(st, cur, it) {
				diff.Update[secItems]++
				plan.itemNeeds[it.Code] = true
			}
		} else {
			diff.Create[secItems]++
		}
	}

	for _, e := range snap.EffectTypes {
		if cur, ok := st.matchEffect(e); ok {
			plan.effectID[e.Code] = cur.ID
			plan.claimedEffects[cur.ID] = true
			if st.mode == "bootstrap" && codeOrEmpty(cur.Code) != e.Code {
				diff.Remap = append(diff.Remap, ImportRemapEntry{Section: secEffectTypes, Code: e.Code, Name: e.Name, OldCode: codeOrEmpty(cur.Code)})
			}
			if effectChanged(cur, e) {
				diff.Update[secEffectTypes]++
				plan.effectNeeds[e.Code] = true
			}
		} else {
			diff.Create[secEffectTypes]++
		}
	}

	// листья: сравнение текущего набора с желаемым
	for _, p := range producers {
		pid := plan.prodID[p.Code]
		desiredSl := desiredSlots(snap, p.Code)
		desiredBi := desiredBindings(snap, p.Code)
		desiredPi := desiredProducerItems(snap, p.Code)
		if pid == 0 {
			diff.Create[secProducerSlots] += len(desiredSl)
			diff.Create[secProducerRecipes] += len(desiredBi)
			diff.Create[secProducerItems] += len(desiredPi)
			continue
		}
		as, cs, rs := diffSlots(st.slots[pid], desiredSl, st.catKeyByID)
		diff.Create[secProducerSlots] += as
		diff.Update[secProducerSlots] += cs
		for i := 0; i < rs; i++ {
			diff.Delete = append(diff.Delete, ImportDiffEntry{Section: secProducerSlots, Code: p.Code, Name: p.Name})
		}
		ab, cb, rb := diffBindings(st.binds[pid], desiredBi, st.goodCodeByID)
		diff.Create[secProducerRecipes] += ab
		diff.Update[secProducerRecipes] += cb
		for i := 0; i < rb; i++ {
			diff.Delete = append(diff.Delete, ImportDiffEntry{Section: secProducerRecipes, Code: p.Code, Name: p.Name})
		}
		ap, cp, rp := diffProducerItems(st.pitems[pid], desiredPi, st.itemCodeByID)
		diff.Create[secProducerItems] += ap
		diff.Update[secProducerItems] += cp
		for i := 0; i < rp; i++ {
			diff.Delete = append(diff.Delete, ImportDiffEntry{Section: secProducerItems, Code: p.Code, Name: p.Name})
		}
	}
	return plan, nil
}

// collectImportDeletions — перечень удаляемого (полная замена, §5.3) и предсчёт
// мировых ссылок (§5.4).
//
// Предсчёт охватывает только ссылки из НЕконтентных таблиц («живой мир»): см.
// guardGood/guardRecipeRefs/guardProducer/guardEffect. Контентные FK (recipes→
// goods, recipe_components, producer_slots/recipes/items, producer_types.parent)
// в blocked намеренно не входят: применяются до удалений, дочерние наборы
// переписываются целиком, producer_types удаляются детьми-вперёд, а `id`
// существующих записей не меняются — поэтому контентные ссылки либо уже сняты,
// либо ведут на удаляемые вместе с родителем записи. Каскад как путь удаления
// контента не используется (решение §5.4).
func collectImportDeletions(st *importState, plan *importPlan, diff *ImportDiff, tx *sql.Tx) error {
	for _, g := range st.goods {
		if plan.claimedGoods[g.ID] {
			continue
		}
		diff.Delete = append(diff.Delete, ImportDiffEntry{Section: secGoods, Code: codeOrEmpty(g.Code), Name: g.Name})
		blocked, err := guardGood(tx, codeOrEmpty(g.Code), g.Name, g.ID)
		if err != nil {
			return err
		}
		diff.Blocked = append(diff.Blocked, blocked...)
	}
	for _, p := range st.producers {
		if plan.claimedProds[p.ID] {
			continue
		}
		diff.Delete = append(diff.Delete, ImportDiffEntry{Section: secProducerTypes, Code: codeOrEmpty(p.Code), Name: p.Name})
		blocked, err := guardProducer(tx, codeOrEmpty(p.Code), p.Name, p.ID)
		if err != nil {
			return err
		}
		diff.Blocked = append(diff.Blocked, blocked...)
	}
	for _, it := range st.items {
		if plan.claimedItems[it.ID] {
			continue
		}
		diff.Delete = append(diff.Delete, ImportDiffEntry{Section: secItems, Code: codeOrEmpty(it.Code), Name: it.Name})
	}
	for _, e := range st.effects {
		if plan.claimedEffects[e.ID] {
			continue
		}
		diff.Delete = append(diff.Delete, ImportDiffEntry{Section: secEffectTypes, Code: codeOrEmpty(e.Code), Name: e.Name})
		blocked, err := guardEffect(tx, codeOrEmpty(e.Code), e.Name, e.ID)
		if err != nil {
			return err
		}
		diff.Blocked = append(diff.Blocked, blocked...)
	}
	keptRecipes := make(map[int64]bool, len(plan.recipeID))
	for _, id := range plan.recipeID {
		keptRecipes[id] = true
	}
	for _, r := range st.recipes {
		if keptRecipes[r.ID] {
			continue
		}
		g, ok := st.goodByID[r.GoodID]
		if !ok {
			continue
		}
		diff.Delete = append(diff.Delete, ImportDiffEntry{Section: secRecipes, Code: codeOrEmpty(g.Code), Name: g.Name})
		blocked, err := guardRecipeRefs(tx, codeOrEmpty(g.Code), g.Name, r.ID)
		if err != nil {
			return err
		}
		diff.Blocked = append(diff.Blocked, blocked...)
	}
	return nil
}

func codeOrEmpty(c sql.NullString) string {
	if c.Valid {
		return c.String
	}
	return ""
}

type importRefTable struct {
	Table  string
	Column string
}

func guardRefs(tx *sql.Tx, section, code, name string, id int64, refs []importRefTable) ([]ImportBlocked, error) {
	var out []ImportBlocked
	for _, ref := range refs {
		var n int
		if err := tx.QueryRow(
			fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", ref.Table, ref.Column), id,
		).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			out = append(out, ImportBlocked{Section: section, Code: code, Name: name, RefTable: ref.Table, RefCount: n})
		}
	}
	return out, nil
}

func guardGood(tx *sql.Tx, code, name string, id int64) ([]ImportBlocked, error) {
	return guardRefs(tx, secGoods, code, name, id, []importRefTable{
		{"deposits", "good_id"},
		{"player_cargo", "good_id"},
		{"settlement_storage_cells", "good_id"},
	})
}

func guardRecipeRefs(tx *sql.Tx, code, name string, id int64) ([]ImportBlocked, error) {
	return guardRefs(tx, secRecipes, code, name, id, []importRefTable{
		{"settlement_branches", "recipe_id"},
	})
}

func guardProducer(tx *sql.Tx, code, name string, id int64) ([]ImportBlocked, error) {
	return guardRefs(tx, secProducerTypes, code, name, id, []importRefTable{
		{"settlements", "settlement_type_id"},
		{"buildings", "producer_type_id"},
	})
}

func guardEffect(tx *sql.Tx, code, name string, id int64) ([]ImportBlocked, error) {
	return guardRefs(tx, secEffectTypes, code, name, id, []importRefTable{
		{"active_effects", "effect_type_id"},
	})
}

// --- применение ---

func applyImport(tx *sql.Tx, st *importState, snap *contentio.Snapshot, producers []contentio.ProducerType, plan *importPlan, report *ImportReport) error {
	// Первичная накатка: метки цели переназначаются по файлу. Снимаем их
	// заранее (в этой же транзакции), иначе при «обмене» метками между
	// записями (g_0001 у одной записи → другой) частичный UNIQUE(code) может
	// сработать в середине — до того, как запись-владелец метки обновлена.
	// Откат вернёт метки при любой ошибке (§5.4/§5.5).
	if st.mode == "bootstrap" {
		for _, q := range []string{
			`UPDATE goods SET code = NULL`,
			`UPDATE producer_types SET code = NULL`,
			`UPDATE items SET code = NULL`,
			`UPDATE effect_types SET code = NULL`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return wrapImportErr(err)
			}
		}
	}

	catID := map[catKey]int64{}
	for _, c := range snap.Categories {
		k := catKeyOf(c.Kind, c.Name)
		if plan.catFound[k] {
			id := plan.catID[k]
			if categoryChanged(st.catByKey[k], c) {
				if _, err := tx.Exec(
					`UPDATE categories SET name=$2, name_norm=$3, code=$4, is_system=$5 WHERE id=$1`,
					id, c.Name, graph.NormalizeName(c.Name), nullText(c.Code), c.IsSystem,
				); err != nil {
					return wrapImportErr(err)
				}
				report.Updated[secCategories]++
			}
			catID[k] = id
			continue
		}
		var id int64
		if err := tx.QueryRow(
			`INSERT INTO categories (name, name_norm, kind, code, is_system) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			c.Name, graph.NormalizeName(c.Name), c.Kind, nullText(c.Code), c.IsSystem,
		).Scan(&id); err != nil {
			return wrapImportErr(err)
		}
		catID[k] = id
		report.Created[secCategories]++
	}

	goodID := map[string]int64{}
	for _, g := range snap.Goods {
		cat := catID[catKeyOf(g.Kind, g.Category)]
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
		if cur, ok := st.matchGood(g); ok {
			goodID[g.Code] = cur.ID
			if st.mode == "bootstrap" || plan.goodNeeds[g.Code] {
				if _, err := tx.Exec(
					`UPDATE goods SET name=$2, name_norm=$3, category_id=$4, kind=$5, source=$6, description=$7, volume=$8, weight=$9, props=$10, code=$11 WHERE id=$1`,
					cur.ID, g.Name, graph.NormalizeName(g.Name), cat, g.Kind, src,
					nullText(g.Description), vol, weight, jsonParam(g.Props), g.Code,
				); err != nil {
					return wrapImportErr(err)
				}
				if plan.goodNeeds[g.Code] {
					report.Updated[secGoods]++
				}
			}
			continue
		}
		var id int64
		if err := tx.QueryRow(
			`INSERT INTO goods (name, name_norm, category_id, kind, source, description, volume, weight, props, code)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
			g.Name, graph.NormalizeName(g.Name), cat, g.Kind, src,
			nullText(g.Description), vol, weight, jsonParam(g.Props), g.Code,
		).Scan(&id); err != nil {
			return wrapImportErr(err)
		}
		goodID[g.Code] = id
		report.Created[secGoods]++
	}

	recipeID := map[string]int64{}
	for _, r := range snap.Recipes {
		gid := goodID[r.Good]
		if cur, ok := plan.recipeID[r.Good]; ok && cur != 0 {
			recipeID[r.Good] = cur
			if !intPtrEqual(st.recipeByGood[gid].Complexity, r.Complexity) {
				if _, err := tx.Exec(`UPDATE recipes SET complexity=$2 WHERE id=$1`, cur, intPtrParam(r.Complexity)); err != nil {
					return wrapImportErr(err)
				}
				report.Updated[secRecipes]++
			}
			continue
		}
		var id int64
		if err := tx.QueryRow(`INSERT INTO recipes (good_id, complexity) VALUES ($1,$2) RETURNING id`,
			gid, intPtrParam(r.Complexity)).Scan(&id); err != nil {
			return wrapImportErr(err)
		}
		recipeID[r.Good] = id
		report.Created[secRecipes]++
	}
	for _, r := range snap.Recipes {
		rid := recipeID[r.Good]
		if rid == 0 {
			continue
		}
		desired := desiredComponents(snap, r.Good)
		add, chg, rem := diffComponents(st.comps[rid], desired, st.goodCodeByID)
		if add == 0 && chg == 0 && rem == 0 {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM recipe_components WHERE recipe_id=$1`, rid); err != nil {
			return wrapImportErr(err)
		}
		for _, c := range desired {
			var comp interface{}
			if c.Component != "" {
				comp = goodID[c.Component]
			}
			if _, err := tx.Exec(
				`INSERT INTO recipe_components (recipe_id, pos, component_id, quantity, reason, allow_resource)
				 VALUES ($1,$2,$3,$4,$5,$6)`,
				rid, c.Pos, comp, c.Quantity, nullText(c.Reason), c.AllowResource,
			); err != nil {
				return wrapImportErr(err)
			}
		}
		report.Created[secRecipeComponents] += add
		report.Updated[secRecipeComponents] += chg
		report.Deleted[secRecipeComponents] += rem
	}

	prodID := map[string]int64{}
	for _, p := range producers {
		var cat, parent interface{}
		if p.Category != "" {
			cat = catID[catKeyOf(catKindOrDefault(p.CategoryKind), p.Category)]
		}
		if p.Parent != "" {
			parent = prodID[p.Parent]
		}
		if cur, ok := st.matchProducer(p); ok {
			prodID[p.Code] = cur.ID
			if st.mode == "bootstrap" || plan.prodNeeds[p.Code] {
				if _, err := tx.Exec(
					`UPDATE producer_types SET name=$2, name_norm=$3, kind=$4, category_id=$5, race_family=$6, parent_id=$7,
					   race=$8, output=$9, input=$10, params=$11, hidden=$12, section=$13, code=$14 WHERE id=$1`,
					cur.ID, p.Name, graph.NormalizeName(p.Name), p.Kind, cat,
					nullText(p.RaceFamily), parent, nullText(p.Race),
					jsonParam(p.Output), jsonParam(p.Input), jsonParam(p.Params), p.Hidden, nullText(p.Section), p.Code,
				); err != nil {
					return wrapImportErr(err)
				}
				if plan.prodNeeds[p.Code] {
					report.Updated[secProducerTypes]++
				}
			}
			continue
		}
		var id int64
		if err := tx.QueryRow(
			`INSERT INTO producer_types (name, name_norm, kind, category_id, race_family, parent_id, race, output, input, params, hidden, section, code)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
			p.Name, graph.NormalizeName(p.Name), p.Kind, cat, nullText(p.RaceFamily), parent, nullText(p.Race),
			jsonParam(p.Output), jsonParam(p.Input), jsonParam(p.Params), p.Hidden, nullText(p.Section), p.Code,
		).Scan(&id); err != nil {
			return wrapImportErr(err)
		}
		prodID[p.Code] = id
		report.Created[secProducerTypes]++
	}

	itemID := map[string]int64{}
	for _, it := range snap.Items {
		unlocks, err := buildUnlocks(it, prodID, catID)
		if err != nil {
			return err
		}
		if cur, ok := st.matchItem(it); ok {
			itemID[it.Code] = cur.ID
			if st.mode == "bootstrap" || plan.itemNeeds[it.Code] {
				if _, err := tx.Exec(
					`UPDATE items SET name=$2, name_norm=$3, slot_type=$4, unlocks=$5, params=$6, code=$7 WHERE id=$1`,
					cur.ID, it.Name, graph.NormalizeName(it.Name), it.SlotType, unlocks, jsonParam(it.Params), it.Code,
				); err != nil {
					return wrapImportErr(err)
				}
				if plan.itemNeeds[it.Code] {
					report.Updated[secItems]++
				}
			}
			continue
		}
		var id int64
		if err := tx.QueryRow(
			`INSERT INTO items (name, name_norm, slot_type, unlocks, params, code) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			it.Name, graph.NormalizeName(it.Name), it.SlotType, unlocks, jsonParam(it.Params), it.Code,
		).Scan(&id); err != nil {
			return wrapImportErr(err)
		}
		itemID[it.Code] = id
		report.Created[secItems]++
	}

	for _, p := range producers {
		pid := prodID[p.Code]
		desired := desiredSlots(snap, p.Code)
		add, chg, rem := diffSlots(st.slots[pid], desired, st.catKeyByID)
		if add == 0 && chg == 0 && rem == 0 {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM producer_slots WHERE parent_id=$1`, pid); err != nil {
			return wrapImportErr(err)
		}
		for _, s := range desired {
			if _, err := tx.Exec(
				`INSERT INTO producer_slots (parent_id, category_id, race_family, race, hidden) VALUES ($1,$2,$3,$4,$5)`,
				pid, catID[s.Cat], nullText(s.RaceFamily), nullText(s.Race), s.Hidden,
			); err != nil {
				return wrapImportErr(err)
			}
		}
		report.Created[secProducerSlots] += add
		report.Updated[secProducerSlots] += chg
		report.Deleted[secProducerSlots] += rem
	}

	for _, p := range producers {
		pid := prodID[p.Code]
		desired := desiredBindings(snap, p.Code)
		add, chg, rem := diffBindings(st.binds[pid], desired, st.goodCodeByID)
		if add == 0 && chg == 0 && rem == 0 {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM producer_recipes WHERE producer_type_id=$1`, pid); err != nil {
			return wrapImportErr(err)
		}
		for _, b := range desired {
			if _, err := tx.Exec(
				`INSERT INTO producer_recipes (producer_type_id, recipe_id, rate) VALUES ($1,$2,$3)`,
				pid, recipeID[b.Good], nullFloat(b.Rate),
			); err != nil {
				return wrapImportErr(err)
			}
		}
		report.Created[secProducerRecipes] += add
		report.Updated[secProducerRecipes] += chg
		report.Deleted[secProducerRecipes] += rem
	}

	for _, p := range producers {
		pid := prodID[p.Code]
		desired := desiredProducerItems(snap, p.Code)
		add, chg, rem := diffProducerItems(st.pitems[pid], desired, st.itemCodeByID)
		if add == 0 && chg == 0 && rem == 0 {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM producer_items WHERE producer_type_id=$1`, pid); err != nil {
			return wrapImportErr(err)
		}
		for _, pi := range desired {
			if _, err := tx.Exec(
				`INSERT INTO producer_items (producer_type_id, item_id, requirements) VALUES ($1,$2,$3)`,
				pid, itemID[pi.Item], jsonParam(pi.Requirements),
			); err != nil {
				return wrapImportErr(err)
			}
		}
		report.Created[secProducerItems] += add
		report.Updated[secProducerItems] += chg
		report.Deleted[secProducerItems] += rem
	}

	for _, e := range snap.EffectTypes {
		params := e.Params
		if len(params) == 0 {
			params = json.RawMessage(`{}`)
		}
		if cur, ok := st.matchEffect(e); ok {
			if st.mode == "bootstrap" || plan.effectNeeds[e.Code] {
				if _, err := tx.Exec(
					`UPDATE effect_types SET name=$2, name_norm=$3, impact=$4, params=$5, code=$6, updated_at=NOW() WHERE id=$1`,
					cur.ID, e.Name, graph.NormalizeName(e.Name), e.Impact, string(params), e.Code,
				); err != nil {
					return wrapImportErr(err)
				}
				if plan.effectNeeds[e.Code] {
					report.Updated[secEffectTypes]++
				}
			}
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO effect_types (name, name_norm, impact, params, code) VALUES ($1,$2,$3,$4,$5)`,
			e.Name, graph.NormalizeName(e.Name), e.Impact, string(params), e.Code,
		); err != nil {
			return wrapImportErr(err)
		}
		report.Created[secEffectTypes]++
	}

	if anchor := snap.GenerationConfig.DefaultSettlementTypeID; anchor != "" {
		params, _ := json.Marshal(prodID[anchor])
		if _, err := tx.Exec(
			`INSERT INTO generation_config (key, payload) VALUES ($1, $2::jsonb)
			 ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`,
			models.DefaultSettlementTypeIDKey, string(params),
		); err != nil {
			return wrapImportErr(err)
		}
	}

	return applyDeletions(tx, st, plan, report)
}

// applyDeletions — полная замена: удаление отсутствующего в файле (§5 п.7/§5.3).
// Порядок — дети раньше родителей.
func applyDeletions(tx *sql.Tx, st *importState, plan *importPlan, report *ImportReport) error {
	keptRecipes := make(map[int64]bool, len(plan.recipeID))
	for _, id := range plan.recipeID {
		keptRecipes[id] = true
	}
	for _, r := range st.recipes {
		if keptRecipes[r.ID] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM recipes WHERE id=$1`, r.ID); err != nil {
			return wrapImportErr(err)
		}
		report.Deleted[secRecipes]++
	}
	for _, p := range orderProducersDeepFirst(st.producers) {
		if plan.claimedProds[p.ID] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM producer_types WHERE id=$1`, p.ID); err != nil {
			return wrapImportErr(err)
		}
		report.Deleted[secProducerTypes]++
	}
	for _, it := range st.items {
		if plan.claimedItems[it.ID] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM items WHERE id=$1`, it.ID); err != nil {
			return wrapImportErr(err)
		}
		report.Deleted[secItems]++
	}
	for _, g := range st.goods {
		if plan.claimedGoods[g.ID] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM goods WHERE id=$1`, g.ID); err != nil {
			return wrapImportErr(err)
		}
		report.Deleted[secGoods]++
	}
	for _, c := range st.cats {
		k := catKey{Kind: c.Kind, Name: c.NameNorm}
		if plan.catFound[k] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM categories WHERE id=$1`, c.ID); err != nil {
			return wrapImportErr(err)
		}
		report.Deleted[secCategories]++
	}
	for _, e := range st.effects {
		if plan.claimedEffects[e.ID] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM effect_types WHERE id=$1`, e.ID); err != nil {
			return wrapImportErr(err)
		}
		report.Deleted[secEffectTypes]++
	}
	return nil
}

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

// orderSnapshotProducers — порядок «родители раньше детей» (§5 п.3).
func orderSnapshotProducers(in []contentio.ProducerType) []contentio.ProducerType {
	if len(in) <= 1 {
		return in
	}
	depth := make(map[string]int, len(in))
	for iter := 0; iter <= len(in); iter++ {
		changed := false
		for i := range in {
			if in[i].Parent == "" {
				continue
			}
			want := depth[in[i].Parent] + 1
			if depth[in[i].Code] < want {
				depth[in[i].Code] = want
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	out := make([]contentio.ProducerType, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool { return depth[out[i].Code] < depth[out[j].Code] })
	return out
}

// orderProducersDeepFirst — типы по глубине убыв. (дети раньше родителей).
func orderProducersDeepFirst(in []impProducer) []impProducer {
	depth := make(map[int64]int, len(in))
	for iter := 0; iter <= len(in); iter++ {
		changed := false
		for _, p := range in {
			if !p.ParentID.Valid {
				continue
			}
			want := depth[p.ParentID.Int64] + 1
			if depth[p.ID] < want {
				depth[p.ID] = want
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	out := make([]impProducer, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool { return depth[out[i].ID] > depth[out[j].ID] })
	return out
}
