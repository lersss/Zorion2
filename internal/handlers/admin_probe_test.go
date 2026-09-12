package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zorion/internal/probe"
)

func probeHandler() *AdminHandlers {
	return &AdminHandlers{probeRunner: probe.NewRunner(nil, tTempDir())}
}

func tTempDir() string {
	dir, _ := os.MkdirTemp("", "probe_handler")
	return dir
}

func TestProbeResultsEmpty(t *testing.T) {
	h := probeHandler()
	rr := httptest.NewRecorder()
	h.ProbeResults(rr, httptest.NewRequest(http.MethodGet, "/admin/probe/results", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ожидал 200, получил %d", rr.Code)
	}
	if strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Fatalf("пустой список: %s", rr.Body.String())
	}
}

func TestProbeResultNotFound(t *testing.T) {
	h := probeHandler()
	rr := httptest.NewRecorder()
	h.ProbeResult(rr, httptest.NewRequest(http.MethodGet, "/admin/probe/results/xyz", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("ожидал 404, получил %d", rr.Code)
	}
}

func TestProbeRunInvalidJSON(t *testing.T) {
	h := probeHandler()
	rr := httptest.NewRecorder()
	h.ProbeRun(rr, httptest.NewRequest(http.MethodPost, "/admin/probe/run", strings.NewReader("{")))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("ожидал 400, получил %d", rr.Code)
	}
}

func TestProbePresets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.json")
	if err := os.WriteFile(path, []byte(`[{"id":"p1","name":"П1","params":{}}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := probe.LoadPresets(path); err != nil {
		t.Fatal(err)
	}

	h := probeHandler()
	rr := httptest.NewRecorder()
	h.ProbePresets(rr, httptest.NewRequest(http.MethodGet, "/admin/probe/presets", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ожидал 200, получил %d", rr.Code)
	}
	var list []probe.Preset
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "p1" {
		t.Fatalf("пресеты: %+v", list)
	}
}