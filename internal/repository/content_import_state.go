// internal/repository/content_import_state.go
// Состояние импорта: загруженные строки, индексы (метка/ключ/id) и сопоставление.
package repository

import (
	"zorion/internal/goodsstudio/contentio"
	"zorion/internal/goodsstudio/graph"
)

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
