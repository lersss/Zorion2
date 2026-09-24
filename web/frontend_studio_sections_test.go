// web/frontend_studio_sections_test.go
// Контракт вкладки построек студии по разделам вида (спека 2026-09-25-студия-
// разделы-построек-по-видам §6/§11): вместо ветки «Производители» — ряд
// «Постройки» из пяти разделов; раздел — свойство каталога (producer_types.
// section), а не сравнение имён; дерево строится от корней раздела без «Строения»
// и классов. Страница — монолитный HTML с классическим <script>, поэтому контракт
// проверяется по телу функций (jsFuncBody).
package web

import (
	"strings"
	"testing"
)

// TestStudioProdSections — разделы построек: ряд в шапке, поле section как
// источник раздела, фильтр дерева, карточка/форма и сохранение раздела.
func TestStudioProdSections(t *testing.T) {
	src := studioHTML(t)

	// Ряд разделов: #segBuildings есть, #branchProducers отсутствует.
	if !strings.Contains(src, `id="segBuildings"`) {
		t.Fatalf("нет #segBuildings — ряда разделов построек")
	}
	if strings.Contains(src, "branchProducers") {
		t.Fatalf("в studio.html остался #branchProducers (кнопка «Производители» не снята)")
	}

	// PROD_SECTIONS — пять ключей разделов.
	if !strings.Contains(src, "PROD_SECTIONS") {
		t.Fatalf("нет PROD_SECTIONS — реестра разделов")
	}
	for _, key := range []string{`"colony"`, `"factory"`, `"lab"`, `"mining"`, `"energy"`} {
		if !strings.Contains(src, key) {
			t.Fatalf("PROD_SECTIONS: нет ключа %s", key)
		}
	}

	// prodSectionOf читает p.section (а не имя корня): переименование безопасно.
	sec := jsFuncBody(t, src, "prodSectionOf")
	if !strings.Contains(sec, "p.section") {
		t.Fatalf("prodSectionOf не читает p.section:\n%s", sec)
	}
	if strings.Contains(sec, "p.name") {
		t.Fatalf("prodSectionOf привязан к имени — «устойчивость сразу» не выполнена:\n%s", sec)
	}

	// Фильтр дерева по разделу; «Строения» и классы в отрисовке отсутствуют.
	tree := jsFuncBody(t, src, "buildProdTree")
	if !strings.Contains(tree, "prodSectionRoots") {
		t.Fatalf("buildProdTree не фильтрует корни по разделу:\n%s", tree)
	}
	for _, gone := range []string{"class-goods", "class-items", "class-energy", "class-other", "Строения"} {
		if strings.Contains(tree, gone) {
			t.Fatalf("buildProdTree всё ещё содержит %q (снятые «Строения»/классы):\n%s", gone, tree)
		}
	}
	if !strings.Contains(tree, "prodSubtypeNoCatRecords") {
		t.Fatalf("buildProdTree потерял prodSubtypeNoCatRecords (подтипы без товарной категории)")
	}

	// prodAdd шлёт section в POST.
	add := jsFuncBody(t, src, "prodAdd")
	if !strings.Contains(add, "section") {
		t.Fatalf("prodAdd не шлёт section:\n%s", add)
	}

	// Карточка типа-корня: блок «Раздел».
	popup := jsFuncBody(t, src, "renderProdPopup")
	if !strings.Contains(popup, "Раздел") {
		t.Fatalf("renderProdPopup без блока «Раздел»:\n%s", popup)
	}
	if !strings.Contains(popup, "prodSectionBlock") {
		t.Fatalf("renderProdPopup не зовёт prodSectionBlock:\n%s", popup)
	}

	// Сохранение раздела — PUT /studio/api/producers/{id} (паттерн saveStageBlock).
	save := jsFuncBody(t, src, "saveProdSection")
	if !strings.Contains(save, `"/studio/api/producers/"`) || !strings.Contains(save, "section") {
		t.Fatalf("saveProdSection не сохраняет раздел:\n%s", save)
	}

	// Ряд #segBuildings управляется в applyBranchUI.
	ui := jsFuncBody(t, src, "applyBranchUI")
	if !strings.Contains(ui, "segBuildings") {
		t.Fatalf("applyBranchUI не управляет рядом #segBuildings")
	}

	// Переключение раздела ставит ветку «постройки».
	set := jsFuncBody(t, src, "setProdSection")
	if !strings.Contains(set, `"producers"`) {
		t.Fatalf("setProdSection не ставит branch=producers:\n%s", set)
	}
	// «Прочее» — полноправный активный раздел (не клампится к «Колониям»).
	if !strings.Contains(set, `"other"`) || !strings.Contains(set, `key !== "other"`) {
		t.Fatalf("setProdSection клампит раздел «Прочее» к «Колониям»:\n%s", set)
	}
	// «Прочее» достижимо и подсвечивается в ряду разделов.
	if !strings.Contains(src, `id="branchProdOther"`) {
		t.Fatalf("нет кнопки #branchProdOther — раздел «Прочее» недостижим")
	}
	uiOther := jsFuncBody(t, src, "renderBuildingsTabsUI")
	if !strings.Contains(uiOther, `prodSection === "other"`) || !strings.Contains(uiOther, "branchProdOther") {
		t.Fatalf("renderBuildingsTabsUI не управляет активным «Прочее»:\n%s", uiOther)
	}
	// D1: вне ветки построек ни одна кнопка раздела не подсвечена — подсветка
	// только при branch === "producers" (спека §6.1/§12.1). На «Товарах»/
	// «Предметах» ряд виден, но активной кнопки раздела нет.
	if !strings.Contains(uiOther, `branch === "producers"`) {
		t.Fatalf("renderBuildingsTabsUI не сверяется с branch — кнопка раздела остаётся активной вне построек:\n%s", uiOther)
	}
	if strings.Count(uiOther, "inProducers &&") < 2 {
		t.Fatalf("подсветка разделов не под условием «ветка построек» (5 кнопок + «Прочее»):\n%s", uiOther)
	}
	// Ряд «Постройки» — единственная точка входа: виден на всех ветках.
	if !strings.Contains(ui, `$("segBuildings").style.display = ""`) {
		t.Fatalf("applyBranchUI не держит ряд #segBuildings видимым (точка входа):\n%s", ui)
	}
	// На старте «Прочее» выживает (клампится только неизвестное значение).
	if !strings.Contains(src, `prodSection !== "other"`) {
		t.Fatalf("стартовая нормализация обнуляет сохранённый раздел «Прочее»")
	}
}
