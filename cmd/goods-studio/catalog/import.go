// Package catalog — импорт каталогов ресурсов сервера (спека 99a.1 §2,
// развилка 2): LayerCatalog() (20) и RealCatalog() (111) — read-only
// ресурсы. Пакет internal/resource БД не тянет (проверено: импортирует
// только internal/races, internal/models, internal/names — без БД).
package catalog

import (
	"fmt"

	"zorion/cmd/goods-studio/model"
	"zorion/internal/resource"
)

// ImportResources строит товары-ресурсы из каталогов internal/resource.
// id — res:<catalog_id> (спека 99a.1 §5, пример res:zhelezo); коллизий
// между layer и real нет (проверено по каталогам).
func ImportResources() []model.Good {
	var goods []model.Good
	for _, r := range resource.LayerCatalog() {
		goods = append(goods, resourceGood("layer", r.ID, r.Name, r.Category))
	}
	for _, r := range resource.RealCatalog() {
		goods = append(goods, resourceGood("real", r.ID, r.Name, r.Category))
	}
	return goods
}

func resourceGood(catalogID, id, name, category string) model.Good {
	return model.Good{
		ID:        fmt.Sprintf("res:%s", id),
		Name:      name,
		Category:  category,
		Status:    model.StatusResource,
		Kind:      model.KindResource,
		Source:    model.SourcePalette,
		Recipe:    []model.Slot{},
		CreatedAt: model.NowISO(),
		ResourceRef: &model.ResourceRef{
			Catalog:   catalogID,
			CatalogID: id,
		},
	}
}

// MergeResources обновляет ресурсы состояния из каталогов (спека 99a.1 §5.3):
// новые добавляются, существующие (по resource_ref) обновляют имя/категорию
// (статус/бан сохраняются — память статусов переживает перезапуск);
// удалённые из каталога остаются (могут быть в рецептах — чистит создатель).
func MergeResources(st *model.State, fresh []model.Good) {
	byRef := make(map[string]int) // "catalog:catalog_id" → индекс в st.Goods
	for i := range st.Goods {
		g := &st.Goods[i]
		if g.Kind == model.KindResource && g.ResourceRef != nil {
			byRef[g.ResourceRef.Catalog+":"+g.ResourceRef.CatalogID] = i
		}
	}
	for _, f := range fresh {
		key := f.ResourceRef.Catalog + ":" + f.ResourceRef.CatalogID
		if idx, ok := byRef[key]; ok {
			st.Goods[idx].Name = f.Name
			st.Goods[idx].Category = f.Category
		} else {
			st.Goods = append(st.Goods, f)
		}
	}
}