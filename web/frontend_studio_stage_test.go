// web/frontend_studio_stage_test.go
// Контракт студии по редактору стадии (спека 2026-09-23-стадии-поселения-и-
// скорость-производства §11.1 п.3 / §11.2): в карточке постройки — блок
// «Стадия» с полями [порог входа] → [порог выхода] в людях, связка со следующей
// ступенью по данным (min enter соседей, больший своего), exit ≥ enter —
// красная ошибка и блокировка сохранения; с карточек дерева диапазон снят.
// Страница — монолитный HTML с классическим <script>, не модуль; исполнять её
// в Node без полного DOM нельзя, поэтому контракт проверяется по телу функций.
package web

import (
	"strings"
	"testing"
)

// TestStudioStageBlock — блок «Стадия» карточки постройки и снятие диапазона
// с карточек дерева.
func TestStudioStageBlock(t *testing.T) {
	src := studioHTML(t)

	// Блок: заголовок, поля порогов, предикат класса, ошибка exit ≥ enter.
	block := jsFuncBody(t, src, "stageBlock")
	for _, want := range []string{"Стадия", `type="number"`, "prodSlots", "prodNextStage", "prodStageEnter", "порог выхода должен быть меньше порога входа"} {
		if !strings.Contains(block, want) {
			t.Fatalf("stageBlock: нет %q в теле:\n%s", want, block)
		}
	}

	// Связка со следующей ступенью — по данным (запись, не только число).
	if !strings.Contains(src, "function prodNextStage(") {
		t.Fatalf("нет prodNextStage — связка со следующей ступенью не по данным")
	}

	// renderProdPopup зовёт блок.
	popup := jsFuncBody(t, src, "renderProdPopup")
	if !strings.Contains(popup, "stageBlock(p)") {
		t.Fatalf("renderProdPopup не зовёт stageBlock")
	}

	// Сохранение: PUT в /producers/{id} телом stage, проверка exit ≥ enter и
	// перерисовка при ошибке/пустых полях.
	save := jsFuncBody(t, src, "saveStageBlock")
	for _, want := range []string{`"/studio/api/producers/"`, "stage", ">=", "renderProdPopup"} {
		if !strings.Contains(save, want) {
			t.Fatalf("saveStageBlock: нет %q в теле:\n%s", want, save)
		}
	}

	// Диапазон снят с карточек дерева: prodStageText удалён, дерево его не зовёт.
	if strings.Contains(src, "prodStageText") {
		t.Fatalf("prodStageText остался — диапазон ступени не снят с карточек дерева")
	}
	tree := jsFuncBody(t, src, "renderProdTree")
	if strings.Contains(tree, "stageLines") {
		t.Fatalf("renderProdTree всё ещё сдвигает строку «производит» под стадию:\n%s", tree)
	}
}
