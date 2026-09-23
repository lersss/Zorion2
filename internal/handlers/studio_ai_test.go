// internal/handlers/studio_ai_test.go
// T7/T10: контракт ручек /studio/api/ai/* (спека 2026-09-24 §3): 405, форма
// StatusView, 403 на уровне роута, запрет остановки во время ИИ-прогона.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/auth"
	"zorion/internal/goodsstudio/aiserve"
)

// fakeAI — заглушка aiController: фиксирует вызовы Stop/Start.
type fakeAI struct {
	status    aiserve.Status
	startCall int
	stopCall  int
	stopRes   aiserve.StopResult
}

func (f *fakeAI) Status() aiserve.Status { return f.status }
func (f *fakeAI) Start() aiserve.StartResult {
	f.startCall++
	return aiserve.StartResult{Code: 202, Started: true, Status: f.status}
}
func (f *fakeAI) Stop() aiserve.StopResult { f.stopCall++; return f.stopRes }

// T7: метод не разрешён → 405 на всех трёх ручках.
func TestStudioAIMethodNotAllowed(t *testing.T) {
	h := NewStudioHandlers(nil, nil, "test-model")
	f := &fakeAI{}
	h.aiServe = f

	cases := []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{"status POST", http.MethodPost, h.AIStatus},
		{"start GET", http.MethodGet, h.AIStart},
		{"stop GET", http.MethodGet, h.AIStop},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, "/studio/api/ai/x", nil)
		rec := httptest.NewRecorder()
		c.handler(rec, req)
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code, c.name)
	}
}

// T7: форма StatusView (поля спеки §3).
func TestStudioAIStatusShape(t *testing.T) {
	h := NewStudioHandlers(nil, nil, "test-model")
	h.aiServe = &fakeAI{status: aiserve.Status{
		State: "running", Managed: true, Detail: "ИИ: работает (v1.1.20)",
		URL: "http://127.0.0.1:3456", Port: 3456, Version: "1.1.20",
		StartingSince: "", LastError: "", Log: "logs/opencode_serve.log",
	}}

	req := httptest.NewRequest(http.MethodGet, "/studio/api/ai/status", nil)
	rec := httptest.NewRecorder()
	h.AIStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	for _, key := range []string{"state", "managed", "reason", "detail", "url", "port", "version", "starting_since", "last_error", "log"} {
		require.Contains(t, got, key)
	}
	require.Equal(t, "running", got["state"])
	require.Equal(t, float64(3456), got["port"])
}

// T7: player → 403 на уровне роута (auth.AdminAuth).
func TestStudioAIRouteDeniesPlayer(t *testing.T) {
	requireHandlerJWT(t)
	tok, err := auth.GenerateToken("player1", string(auth.RolePlayer))
	require.NoError(t, err)

	h := NewStudioHandlers(nil, nil, "test-model")
	handler := auth.AdminAuth(h.AIStatus)
	req := httptest.NewRequest(http.MethodGet, "/studio/api/ai/status", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

// T10: stop при активном ИИ-прогоне студии → 409, Stop не вызван.
func TestStudioAIStopBlockedDuringRun(t *testing.T) {
	h := NewStudioHandlers(nil, nil, "test-model")
	f := &fakeAI{
		status:  aiserve.Status{State: "running", Managed: true},
		stopRes: aiserve.StopResult{Code: 200, Stopped: true},
	}
	h.aiServe = f

	h.fillMu.Lock()
	h.fillGenerating = true
	h.fillMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/studio/api/ai/stop", nil)
	rec := httptest.NewRecorder()
	h.AIStop(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "идёт ИИ-прогон студии")
	require.Equal(t, 0, f.stopCall, "kill не вызван — Stop не дошёл до менеджера")

	// прогон закончился — stop проходит.
	h.fillMu.Lock()
	h.fillGenerating = false
	h.fillMu.Unlock()
	rec = httptest.NewRecorder()
	h.AIStop(rec, httptest.NewRequest(http.MethodPost, "/studio/api/ai/stop", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, f.stopCall)
}

// T7: без менеджера aiStatus — «недоступен», поле ai в state аддитивно.
func TestStudioStateHasAIField(t *testing.T) {
	h := NewStudioHandlers(nil, nil, "test-model")
	require.False(t, h.aiStatus().Managed)
	require.Equal(t, "stopped", h.aiStatus().State)
}

// T10b: порядок §3.3 — managed=false важнее активного прогона: «управление
// недоступно», а не «идёт ИИ-прогон»; Stop не вызван.
func TestStudioAIStopManagedFirst(t *testing.T) {
	h := NewStudioHandlers(nil, nil, "test-model")
	f := &fakeAI{status: aiserve.Status{State: "stopped", Managed: false, Reason: "npx не найден в PATH"}}
	h.aiServe = f

	h.fillMu.Lock()
	h.fillGenerating = true
	h.fillMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/studio/api/ai/stop", nil)
	rec := httptest.NewRecorder()
	h.AIStop(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "управление недоступно")
	require.NotContains(t, rec.Body.String(), "идёт ИИ-прогон")
	require.Equal(t, 0, f.stopCall)
}
