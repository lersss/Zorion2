// web/frontend_studio_unit_scale_test.go
// Контракт студии по переключателю масштаба единицы темпа (задача «переключатель
// масштаба единицы», спека 2026-09-23-стадии-поселения §2.1/§11.1). Страница —
// монолитный HTML с классическим <script>, не модуль; исполнять её в Node без
// полного DOM нельзя, поэтому контракт проверяется по телу функций: показ и ввод
// чисел «Производит»/«Потребляет»/арифметики идут через единую точку масштаба,
// состояние — в общем localStorage-ключе, дефолт — хранимая единица («на млрд»).
package web

import (
	"strings"
	"testing"
)

// TestStudioUnitScaleContract — число уходит на сервер в хранимой единице,
// показывается в выбранном масштабе; масштаб — из общего ключа gs_unitScale.
func TestStudioUnitScaleContract(t *testing.T) {
	src := studioHTML(t)

	// Модуль единой точки масштаба подключён в студию; дефолт — «на млрд».
	if !strings.Contains(src, "/static/js/unit_scale.js") {
		t.Fatalf("studio.html не подключает web/static/js/unit_scale.js")
	}
	if !strings.Contains(src, `getItem("gs_unitScale") || "billion"`) {
		t.Fatalf("studio.html не читает масштаб из ключа gs_unitScale с дефолтом billion")
	}

	// Переключатель: сегмент масштаба в карточке постройки.
	seg := jsFuncBody(t, src, "unitScaleSegmentHtml")
	if !strings.Contains(seg, "UNIT_SCALE_LIST") || !strings.Contains(seg, "setUnitScale") {
		t.Fatalf("unitScaleSegmentHtml не рисует сегмент масштаба:\n%s", seg)
	}
	setter := jsFuncBody(t, src, "setUnitScale")
	if !strings.Contains(setter, `localStorage.setItem("gs_unitScale"`) {
		t.Fatalf("setUnitScale не сохраняет масштаб в gs_unitScale:\n%s", setter)
	}
	popup := jsFuncBody(t, src, "renderProdPopup")
	if !strings.Contains(popup, "unitScaleSegmentHtml") {
		t.Fatalf("renderProdPopup не рисует переключатель масштаба")
	}

	// Ввод: отображаемое число переводится в хранимое перед отправкой.
	save := jsFuncBody(t, src, "saveRecipeRate")
	if !strings.Contains(save, "scaleToStored") {
		t.Fatalf("saveRecipeRate не переводит ввод в хранимую единицу:\n%s", save)
	}
	if !strings.Contains(save, `v === ""`) {
		t.Fatalf("saveRecipeRate: пусто должно оставаться пусто (null), а не 0:\n%s", save)
	}
	norm := jsFuncBody(t, src, "prodEatNormChange")
	if !strings.Contains(norm, "scaleToStored") || !strings.Contains(norm, `=== ""`) {
		t.Fatalf("prodEatNormChange: норма переводится в хранимую единицу, пусто остаётся пусто:\n%s", norm)
	}

	// Показ: хранимое число переводится в выбранный масштаб.
	for _, fn := range []string{"recipesBlock", "prodEatRowHtml", "prodArithmeticBlock", "recipeTakeText"} {
		body := jsFuncBody(t, src, fn)
		if !strings.Contains(body, "scaleToDisplay") {
			t.Fatalf("%s не переводит показ в выбранный масштаб:\n%s", fn, body)
		}
	}

	// Подпись единицы рядом с числами — из масштаба (не жёсткий «/млрд»).
	for _, fn := range []string{"recipesBlock", "eatBlock", "prodEatRowHtml"} {
		if !strings.Contains(jsFuncBody(t, src, fn), "scaleUnitText") {
			t.Fatalf("%s: нет явной подписи единицы выбранного масштаба", fn)
		}
	}
}
