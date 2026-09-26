// internal/repository/content_import_mode.go
// Режим сопоставления (§3.5/§5 п.2) и валидация ссылок снимка.
package repository

import (
	"database/sql"

	"zorion/internal/goodsstudio/contentio"
	"zorion/internal/goodsstudio/graph"
)

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
