// internal/repository/content_import_load.go
// Загрузка целевых строк импорта в состояние (десять чтений; см. loadImportState).
package repository

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
