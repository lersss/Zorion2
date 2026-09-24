// web/frontend_studio_eat_goods_test.go
// Контракт студии по селекту «Потребляет» (эпик «Снабжение форпоста водой»,
// итерация И2; спека 2026-09-24-потребление-по-товарам §9.1/§9.2): позиция
// потребления — ТОВАР (goods.name_norm), категория как позиция снята. Страница —
// монолитный HTML с классическим <script>, не модуль; исполнять её в Node без
// полного DOM нельзя, поэтому контракт проверяется по телу функций: селект
// перечисляет state.goods, резолв/подписи/предупреждения — по товарам,
// неизвестная позиция не уходит на сервер (сервер строг — 422, §9.2).
package web

import (
	"strings"
	"testing"
)

// TestStudioEatPositionGoods — селект позиции и резолв идут по товарам
// (goods.name_norm); категории в селекте не участвуют.
func TestStudioEatPositionGoods(t *testing.T) {
	src := studioHTML(t)

	// Селект позиции перечисляет state.goods, value = name_norm.
	opts := jsFuncBody(t, src, "prodPosOptions")
	if !strings.Contains(opts, "state.goods") {
		t.Fatalf("prodPosOptions не перечисляет state.goods:\n%s", opts)
	}
	if strings.Contains(opts, "state.categories") {
		t.Fatalf("prodPosOptions всё ещё перечисляет state.categories:\n%s", opts)
	}
	if !strings.Contains(opts, "prodNorm(g.name)") {
		t.Fatalf("prodPosOptions: value позиции — не goods.name_norm:\n%s", opts)
	}

	// Резолв позиции — по товару.
	byNorm := jsFuncBody(t, src, "goodByNorm")
	if !strings.Contains(byNorm, "state.goods") || !strings.Contains(byNorm, "prodNorm(g.name)") {
		t.Fatalf("goodByNorm не резолвит товар по goods.name_norm:\n%s", byNorm)
	}
	nameByNorm := jsFuncBody(t, src, "goodNameByNorm")
	if !strings.Contains(nameByNorm, "goodByNorm") {
		t.Fatalf("goodNameByNorm не использует goodByNorm:\n%s", nameByNorm)
	}

	// Предупреждения «Потребляет» — по товарам.
	eat := jsFuncBody(t, src, "eatBlock")
	if !strings.Contains(eat, "goodByNorm") || !strings.Contains(eat, "goodNameByNorm") {
		t.Fatalf("eatBlock не резолвит позицию по товару:\n%s", eat)
	}
	if !strings.Contains(eat, "справочнике товаров") {
		t.Fatalf("eatBlock: нет предупреждения «нет в справочнике товаров»:\n%s", eat)
	}

	// Сохранение: позиция вне справочника товаров на сервер не шлётся (422, §9.2).
	save := jsFuncBody(t, src, "saveEatBlock")
	if !strings.Contains(save, "goodByNorm") {
		t.Fatalf("saveEatBlock не отсекает позицию вне справочника товаров:\n%s", save)
	}

	// Арифметика студии ключует позицию товаром-выходом, не категорией.
	arith := jsFuncBody(t, src, "prodArithmeticBlock")
	if !strings.Contains(arith, "prodNorm(g.name)") {
		t.Fatalf("prodArithmeticBlock: позиция — не goods.name_norm:\n%s", arith)
	}

	// Осиротевший категорийный резолв удалён из студии.
	if strings.Contains(src, "catByNorm") || strings.Contains(src, "catNameByNorm") {
		t.Fatalf("в studio.html остался категорийный резолв позиции (catByNorm/catNameByNorm)")
	}
}
