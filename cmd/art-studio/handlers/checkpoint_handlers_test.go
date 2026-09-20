package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"zorion/cmd/art-studio/config"
)

// TestHandleCheckpoint — GET /checkpoint отдаёт {current, list}; POST
// /checkpoint?name= устанавливает чекпоинт на сессию (валидация имени,
// ответ {ok, current}); неизвестное имя — error без изменения current.
func TestHandleCheckpoint(t *testing.T) {
	srv, _, _ := newTestStudio(t)

	// GET: список известных + текущий (в тестовом конфиге Checkpoint пуст)
	req := httptest.NewRequest("GET", "/checkpoint", nil)
	rr := httptest.NewRecorder()
	srv.handleCheckpoint(rr, req)
	var j map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &j); err != nil {
		t.Fatalf("GET: не-JSON ответ: %v", err)
	}
	list, _ := j["list"].([]interface{})
	if len(list) != len(config.KnownCheckpoints) {
		t.Errorf("list = %d элементов, want %d", len(list), len(config.KnownCheckpoints))
	}
	if cur, _ := j["current"].(string); cur != srv.GetCheckpoint() {
		t.Errorf("current = %q, want %q", cur, srv.GetCheckpoint())
	}

	// POST: валидное имя → {ok, current}, GetCheckpoint обновился
	req = httptest.NewRequest("POST", "/checkpoint?name=dreamshaper-xl-v1.safetensors", nil)
	rr = httptest.NewRecorder()
	srv.handleCheckpoint(rr, req)
	if err := json.Unmarshal(rr.Body.Bytes(), &j); err != nil {
		t.Fatalf("POST: не-JSON ответ: %v", err)
	}
	if j["ok"] != true || j["current"] != "dreamshaper-xl-v1.safetensors" {
		t.Errorf("POST ok/current = %v/%v, want true/dreamshaper", j["ok"], j["current"])
	}
	if got := srv.GetCheckpoint(); got != "dreamshaper-xl-v1.safetensors" {
		t.Errorf("GetCheckpoint = %q, want dreamshaper-xl-v1.safetensors", got)
	}

	// GET после POST: current обновился
	req = httptest.NewRequest("GET", "/checkpoint", nil)
	rr = httptest.NewRecorder()
	srv.handleCheckpoint(rr, req)
	if err := json.Unmarshal(rr.Body.Bytes(), &j); err != nil {
		t.Fatalf("GET после POST: не-JSON ответ: %v", err)
	}
	if cur, _ := j["current"].(string); cur != "dreamshaper-xl-v1.safetensors" {
		t.Errorf("current после POST = %q, want dreamshaper-xl-v1.safetensors", cur)
	}

	// POST: неизвестное имя → error, current не меняется
	req = httptest.NewRequest("POST", "/checkpoint?name=unknown.safetensors", nil)
	rr = httptest.NewRecorder()
	srv.handleCheckpoint(rr, req)
	if err := json.Unmarshal(rr.Body.Bytes(), &j); err != nil {
		t.Fatalf("POST invalid: не-JSON ответ: %v", err)
	}
	if _, ok := j["error"].(string); !ok {
		t.Errorf("POST invalid: нет error, ответ %v", j)
	}
	if got := srv.GetCheckpoint(); got != "dreamshaper-xl-v1.safetensors" {
		t.Errorf("GetCheckpoint после invalid = %q, want без изменений", got)
	}
}