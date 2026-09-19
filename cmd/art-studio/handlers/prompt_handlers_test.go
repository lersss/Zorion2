package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// getPrompt вызывает handlePrompt и возвращает prompt или error из JSON.
func getPrompt(srv *Server, url string) string {
	req := httptest.NewRequest("GET", url, nil)
	rr := httptest.NewRecorder()
	srv.handlePrompt(rr, req)
	var j map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &j); err != nil {
		return "не-JSON ответ: " + err.Error()
	}
	if e, ok := j["error"].(string); ok {
		return e
	}
	p, _ := j["prompt"].(string)
	return p
}

// TestHandlePrompt — 98b-дополнение: /prompt строит полный промпт для выбора
// БЕЗ запуска генерации. Валидация fam/race, морф/без морфа, tags в промпте,
// seed-воспроизводимость (одинаковый seed → одинаковый промпт).
func TestHandlePrompt(t *testing.T) {
	srv, _, _ := newTestStudio(t)

	// валидация: нет семейства
	if got := getPrompt(srv, "/prompt?fam=XX&race=5"); !strings.Contains(got, "нет семейства") {
		t.Errorf("нет семейства: %q", got)
	}
	// валидация: нет расы в семействе
	if got := getPrompt(srv, "/prompt?fam=F2&race=999"); !strings.Contains(got, "нет расы") {
		t.Errorf("нет расы: %q", got)
	}

	// без морфа — вариация (BuildPrompt): субъект «an abstract structure»
	got := getPrompt(srv, "/prompt?fam=F2&race=5&seed=42")
	if !strings.Contains(got, "an abstract structure") {
		t.Errorf("без морфа: %q", got)
	}
	// «-» как морф — тоже вариация
	gotDash := getPrompt(srv, "/prompt?fam=F2&race=5&morph=-&seed=42")
	if !strings.Contains(gotDash, "an abstract structure") {
		t.Errorf("morph=-: %q", gotDash)
	}

	// с морфом — кандидат эталона (BuildPromptWide): «humanoid race»
	gotMorph := getPrompt(srv, "/prompt?fam=F2&race=5&morph=anthro&seed=42")
	if !strings.Contains(gotMorph, "humanoid race") {
		t.Errorf("морф anthro: %q", gotMorph)
	}

	// tags в промпте
	gotTags := getPrompt(srv, "/prompt?fam=F2&race=5&tags=my+tag&seed=42")
	if !strings.Contains(gotTags, "my tag") {
		t.Errorf("tags: %q", gotTags)
	}

	// seed-воспроизводимость: одинаковый seed → одинаковый промпт
	p1 := getPrompt(srv, "/prompt?fam=F2&race=5&seed=12345")
	p2 := getPrompt(srv, "/prompt?fam=F2&race=5&seed=12345")
	if p1 != p2 {
		t.Errorf("seed-воспроизводимость: %q vs %q", p1, p2)
	}
	// разброс по разным seed (стиль TestBuildPromptWideFormFromRace)
	seen := map[string]bool{}
	for i := 1; i <= 10; i++ {
		seen[getPrompt(srv, fmt.Sprintf("/prompt?fam=F2&race=5&seed=%d", i))] = true
	}
	if len(seen) < 2 {
		t.Errorf("разброс по seed: %d уникальных промптов, want >= 2", len(seen))
	}
}