// web/frontend_studio_export_test.go
// Контракт студии по экспорту снимка контента (спека 2026-09-24-каталог-
// экспорт-импорт-контента-на-прод §7, итерация И2): кнопка есть, зовёт
// GET /studio/api/content/export, скачивает файл (Content-Disposition) и
// показывает отчёт. Страница — монолитный HTML с классическим <script>,
// поэтому контракт проверяется по телу функции и разметке.
package web

import (
	"strings"
	"testing"
)

// TestStudioContentExportContract — кнопка «экспорт контента» и её обработчик.
func TestStudioContentExportContract(t *testing.T) {
	src := studioHTML(t)

	if !strings.Contains(src, `<button id="btnExportContent"`) {
		t.Fatal("в студии нет кнопки #btnExportContent (экспорт контента)")
	}

	body := jsFuncBody(t, src, "exportContent")
	for _, want := range []string{
		"/studio/api/content/export", // зовёт серверную ручку (§7)
		"if (!r) ",                   // guard после ветки ошибки (api → null)
		"Content-Disposition",        // имя файла из заголовка
		"a.download",                 // скачивание файла
		"showReport",                 // отчёт «сохранено: N записей, файл …»
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("exportContent: нет %q в теле:\n%s", want, body)
		}
	}

	if !strings.Contains(src, `$("btnExportContent").addEventListener("click", exportContent)`) {
		t.Fatal("кнопка #btnExportContent не привязана к exportContent")
	}
}
