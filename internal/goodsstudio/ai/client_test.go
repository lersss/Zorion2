package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAskSendsExplicitAgent — регрессия бага 2026-09-21: пустой "agent" в
// opencode 1.18 резолвился в default_agent проекта (manager) → модель отвечала
// прозой менеджера вместо JSON. Клиент обязан слать явный агент.
func TestAskSendsExplicitAgent(t *testing.T) {
	var gotAgent string
	var gotModel map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			io.WriteString(w, `{"id":"s1"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
			var body struct {
				Agent string            `json:"agent"`
				Model map[string]string `json:"model"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			gotAgent = body.Agent
			gotModel = body.Model
			io.WriteString(w, `{"parts":[{"type":"text","text":"{\"components\":[]}"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "provider/model-x", "build", time.Second, 0)
	text, err := c.Ask("prompt")
	require.NoError(t, err)
	require.Equal(t, `{"components":[]}`, text)
	require.Equal(t, "build", gotAgent, "явный агент обязателен (не пустой)")
	require.Equal(t, "provider", gotModel["providerID"])
	require.Equal(t, "model-x", gotModel["modelID"])
}
