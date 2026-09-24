// web/frontend_studio_import_test.go
// Контракт студии по импорту снимка контента (спека 2026-09-24-каталог-
// экспорт-импорт-контента-на-прод §5/§7, итерация И3): кнопка, двухшаговый
// пропуск (Проверить/dry_run → попап-дифф → Применить), блокировка «Применить»
// при blocked/unmatched, статус файла. Страница — монолитный HTML с
// классическим <script>, поэтому контракт проверяется по телу функций и разметке.
package web

import (
	"strings"
	"testing"
)

// TestStudioContentImportContract — кнопка импорта и её обработчики.
func TestStudioContentImportContract(t *testing.T) {
	src := studioHTML(t)

	if !strings.Contains(src, `<button id="btnImportContent"`) {
		t.Fatal("в студии нет кнопки #btnImportContent (импорт контента)")
	}
	if !strings.Contains(src, `id="importPopup"`) || !strings.Contains(src, `id="importBody"`) {
		t.Fatal("в студии нет попапа импорта (#importPopup/#importBody)")
	}

	// шаг 1 — сухой прогон + guard после ветки ошибки (api → null)
	step1 := jsFuncBody(t, src, "importContent")
	for _, want := range []string{
		"/studio/api/content/import?dry_run=true",
		"if (!r) ",
		"openImportPopup",
	} {
		if !strings.Contains(step1, want) {
			t.Fatalf("importContent: нет %q в теле:\n%s", want, step1)
		}
	}

	// попап-дифф: режим, удаления, blocked, блокировка «Применить»
	popup := jsFuncBody(t, src, "openImportPopup")
	for _, want := range []string{"diff.mode", "diff.delete", "diff.blocked", "diff.remap", "canApply", "importApplyBtn"} {
		if !strings.Contains(popup, want) {
			t.Fatalf("openImportPopup: нет %q в теле:\n%s", want, popup)
		}
	}

	// шаг 2 — второе подтверждение и ручка применения
	step2 := jsFuncBody(t, src, "applyImportContent")
	for _, want := range []string{"window.confirm", `"/studio/api/content/import"`, "if (!r) "} {
		if !strings.Contains(step2, want) {
			t.Fatalf("applyImportContent: нет %q в теле:\n%s", want, step2)
		}
	}

	// статус файла: запрос есть, кнопка гасится при отсутствии файла
	status := jsFuncBody(t, src, "refreshImportStatus")
	if !strings.Contains(status, "/studio/api/content/status") || !strings.Contains(status, "btn.disabled") {
		t.Fatalf("refreshImportStatus не проверяет наличие файла:\n%s", status)
	}

	// D1: после успешного экспорта файл появился — кнопка импорта должна ожить
	exp := jsFuncBody(t, src, "exportContent")
	if !strings.Contains(exp, "refreshImportStatus") {
		t.Fatalf("exportContent не обновляет статус кнопки импорта (баг D1):\n%s", exp)
	}

	if !strings.Contains(src, `$("btnImportContent").addEventListener("click", importContent)`) {
		t.Fatal("кнопка #btnImportContent не привязана к importContent")
	}
}
