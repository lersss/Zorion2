// web/frontend_studio_subtype_test.go
// Тест контракта studio.html по подтипу без товарной категории (итерация 4
// §5.3 п.4/T14/T15). Страница — монолитный HTML с классическим <script>, не
// модуль; исполнять его в Node без полного DOM нельзя, поэтому контракт
// проверяется по телу конкретных функций: дерево рисует подтипы без товарной
// категории у типа без слотов, есть путь создания «+ подтип» без category_id,
// а «фабричные» пути требуют непустую товарную категорию.
package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// studioHTML читает web/studio.html.
func studioHTML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "studio.html"))
	if err != nil {
		t.Fatalf("чтение studio.html: %v", err)
	}
	return string(b)
}

// jsFuncBody — текст функции name верхнего уровня (от «function name(» до
// следующей функции верхнего уровня). Достаточно для контрактной проверки:
// в studio.html функции объявлены в колонке 0.
func jsFuncBody(t *testing.T, src, name string) string {
	t.Helper()
	i := strings.Index(src, "function "+name+"(")
	if i < 0 {
		t.Fatalf("в studio.html нет функции %s", name)
	}
	rest := src[i:]
	end := len(rest)
	for _, m := range []string{"\nfunction ", "\nasync function "} {
		if j := strings.Index(rest, m); j > 0 && j < end {
			end = j
		}
	}
	return rest[:end]
}

// TestStudioSubtypeWithoutGoodsCategory — T14/T15: подтип без товарной
// категории («Обычное поселение») виден в дереве, заводится кнопкой «+ подтип»
// (POST без category_id) и не трактуется «фабричными» путями.
func TestStudioSubtypeWithoutGoodsCategory(t *testing.T) {
	src := studioHTML(t)

	// T14: рендер подтипов без товарной категории у типа без слотов.
	rec := jsFuncBody(t, src, "prodSubtypeNoCatRecords")
	for _, want := range []string{"s.parent_id === t.id", "s.category_id == null", "prodVisible"} {
		if !strings.Contains(rec, want) {
			t.Fatalf("prodSubtypeNoCatRecords: нет %q в теле:\n%s", want, rec)
		}
	}
	tree := jsFuncBody(t, src, "buildProdTree")
	if !strings.Contains(tree, "prodSubtypeNoCatRecords") {
		t.Fatalf("buildProdTree не рисует подтипы без товарной категории")
	}

	// T14: путь создания «+ подтип» из попапа типа без слотов.
	popup := jsFuncBody(t, src, "renderProdPopup")
	if !strings.Contains(popup, "openProdSubtypeCreatePopup") {
		t.Fatalf("renderProdPopup: нет кнопки «+ подтип» для типа без слотов")
	}
	createPopup := jsFuncBody(t, src, "openProdSubtypeCreatePopup")
	if !strings.Contains(createPopup, "openModal") {
		t.Fatalf("openProdSubtypeCreatePopup: нет открытия попапа")
	}
	doCreate := jsFuncBody(t, src, "doProdSubtypeCreate")
	if !strings.Contains(doCreate, `"/studio/api/producers"`) || !strings.Contains(doCreate, "parent_id") {
		t.Fatalf("doProdSubtypeCreate: нет POST с parent_id:\n%s", doCreate)
	}
	if strings.Contains(doCreate, "category_id") {
		t.Fatalf("doProdSubtypeCreate шлёт category_id — у такого подтипа товарной категории нет:\n%s", doCreate)
	}

	// Подтип виден и в списке (ось класса строения, не слотов).
	vis := jsFuncBody(t, src, "prodNodeVisible")
	for _, want := range []string{"prodSlots(t).length === 0", "p.category_id == null"} {
		if !strings.Contains(vis, want) {
			t.Fatalf("prodNodeVisible: нет %q (подтип без товарной категории не виден)", want)
		}
	}

	// T15: «фабричные» пути требуют непустую товарную категорию — подтип без
	// неё в них не попадает по построению.
	facs := jsFuncBody(t, src, "concreteGoodsFactories")
	if !strings.Contains(facs, "p.category_id != null") {
		t.Fatalf("concreteGoodsFactories без проверки непустой товарной категории:\n%s", facs)
	}
	fam := jsFuncBody(t, src, "familyAppliedFactories")
	if !strings.Contains(fam, "p.category_id === catId") {
		t.Fatalf("familyAppliedFactories без привязки к товарной категории:\n%s", fam)
	}
}
