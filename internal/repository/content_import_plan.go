// internal/repository/content_import_plan.go
// План импорта (сравнение снимка с целью → числа create/update + remap),
// перечень удаляемого с предсчётом мировых ссылок (§5.3/§5.4).
package repository

import (
	"database/sql"
	"fmt"
	"sort"

	"zorion/internal/goodsstudio/contentio"
)

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
