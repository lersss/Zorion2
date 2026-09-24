// web/frontend_studio_graph_recipe_id_test.go
// Контракт студии: на карточке товара в графе раздела «Товары» рядом с
// категорией показан номер рецепта (g.recipe_id); у ресурса рецепта нет —
// ничего не рисуется. Страница — монолитный HTML с классическим <script>, не
// модуль; исполнять её в Node без полного DOM нельзя, поэтому контракт
// проверяется по телу функции renderGraph (как frontend_studio_unit_scale_test.go).
package web

import (
	"strings"
	"testing"
)

// TestStudioGraphShowsRecipeID — карточка графа товаров несёт номер рецепта.
func TestStudioGraphShowsRecipeID(t *testing.T) {
	src := studioHTML(t)
	graph := jsFuncBody(t, src, "renderGraph")
	if !strings.Contains(graph, "g.recipe_id") {
		t.Fatalf("renderGraph не показывает номер рецепта на карточке:\n%s", graph)
	}
	if !strings.Contains(graph, "рецепт ") {
		t.Fatalf("renderGraph: нет подписи «рецепт N» на карточке:\n%s", graph)
	}
}
