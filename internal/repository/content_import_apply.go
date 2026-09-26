// internal/repository/content_import_apply.go
// Применение плана: upsert секций, перезапись дочерних наборов, удаления (§5 п.7).
package repository

import (
	"database/sql"
	"encoding/json"

	"zorion/internal/goodsstudio/contentio"
	"zorion/internal/goodsstudio/graph"
	"zorion/internal/models"
)

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
