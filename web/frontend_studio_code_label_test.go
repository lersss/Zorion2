// web/frontend_studio_code_label_test.go
// Контракт студии по показу метки переноса (code, спека
// 2026-09-24-каталог-экспорт-импорт-контента-на-прод §3.3/§7): метка справочная,
// только чтение; в карточке записи (товар/постройка/предмет/эффект) видна мелким
// текстом «метка переноса». Страница — монолитный HTML с классическим <script>,
// поэтому контракт проверяется по телу функций.
package web

import (
	"strings"
	"testing"
)

// TestStudioCodeLabelContract — метка переноса показывается read-only в карточках.
func TestStudioCodeLabelContract(t *testing.T) {
	src := studioHTML(t)

	note := jsFuncBody(t, src, "codeNote")
	for _, want := range []string{"метка переноса", "code-note"} {
		if !strings.Contains(note, want) {
			t.Fatalf("codeNote: нет %q в теле:\n%s", want, note)
		}
	}

	for _, fn := range []string{"renderPopup", "renderProdPopup", "renderItemPopup"} {
		if !strings.Contains(jsFuncBody(t, src, fn), "codeNote") {
			t.Fatalf("%s не вызывает codeNote — метка переноса не показана", fn)
		}
	}

	// Эффекты — строки попапа каталога: метка отдельным read-only span.
	eff := jsFuncBody(t, src, "renderEffectPopup")
	if !strings.Contains(eff, "e.code") || !strings.Contains(eff, "метка переноса") {
		t.Fatalf("renderEffectPopup не показывает метку переноса:\n%s", eff)
	}
}
