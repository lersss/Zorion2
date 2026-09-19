package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/model"
)

// TestImportResources — импорт LayerCatalog() (20) + RealCatalog() (111):
// 131 ресурсов, id уникальны, resource_ref корректен (спека 99a.1 §5.3).
func TestImportResources(t *testing.T) {
	goods := ImportResources()
	require.Len(t, goods, 131)
	seen := make(map[string]bool, len(goods))
	for _, g := range goods {
		require.Equal(t, model.KindResource, g.Kind)
		require.Equal(t, model.StatusResource, g.Status)
		require.Empty(t, g.Recipe)
		require.NotNil(t, g.ResourceRef)
		require.False(t, seen[g.ID], "дубликат id %s", g.ID)
		seen[g.ID] = true
		require.Equal(t, "res:"+g.ResourceRef.CatalogID, g.ID)
	}
	// пример из спеки: res:zhelezo — real/zhelezo
	require.Contains(t, seen, "res:zhelezo")
	// пример слоя: res:water — layer/water
	require.Contains(t, seen, "res:water")
}

// TestMergeResources — новые добавляются, существующие обновляют имя/
// категорию, статус/бан сохраняются (память статусов переживает перезапуск).
func TestMergeResources(t *testing.T) {
	st := model.NewState()
	banTime := "2026-09-18T12:00:00Z"
	st.Goods = append(st.Goods, model.Good{
		ID: "res:zhelezo", Name: "Старое имя", Category: "mineral",
		Status: model.StatusBanned, BannedAt: &banTime, Kind: model.KindResource,
		ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "zhelezo"},
	})
	fresh := ImportResources()
	MergeResources(st, fresh)
	require.Len(t, st.Goods, 131)
	// существующий обновил имя, статус/бан сохранились
	var zhelezo *model.Good
	for i := range st.Goods {
		if st.Goods[i].ID == "res:zhelezo" {
			zhelezo = &st.Goods[i]
		}
	}
	require.NotNil(t, zhelezo)
	require.Equal(t, "Железо Fe", zhelezo.Name)
	require.Equal(t, model.StatusBanned, zhelezo.Status)
	require.NotNil(t, zhelezo.BannedAt)
}