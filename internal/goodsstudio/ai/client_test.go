package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

// TestAskClosesSession — сессия помощника закрывается после ответа
// (DELETE /session/{id}), иначе они копятся (идея 2026-09-24: 1882 сессии).
func TestAskClosesSession(t *testing.T) {
	var created, deleted int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			created++
			fmt.Fprintf(w, `{"id":"s%d"}`, created)
		case strings.HasSuffix(r.URL.Path, "/message"):
			io.WriteString(w, `{"parts":[{"type":"text","text":"{\"components\":[]}"}]}`)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/session/"):
			deleted++
			io.WriteString(w, `true`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "provider/model-x", "build", time.Second, 0)
	for i := 0; i < 3; i++ {
		_, err := c.Ask("prompt")
		require.NoError(t, err)
	}
	require.Equal(t, 3, created)
	require.Equal(t, 3, deleted, "каждая сессия закрывается после ответа")
}

// TestAskAssistantErrorReason — ответ помощника с ошибкой провайдера
// (info.error, parts без text) → ошибка несёт причину человеческим текстом,
// а не обобщённое «пустой ответ» (идея 2026-09-24).
func TestAskAssistantErrorReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			io.WriteString(w, `{"id":"s1"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
			io.WriteString(w, `{"info":{"error":{"name":"ProviderAuthError",`+
				`"data":{"providerID":"gonka","message":"Model access is disabled"}}},"parts":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "provider/model-x", "build", time.Second, 2)
	_, err := c.Ask("prompt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Model access is disabled")
	require.Contains(t, err.Error(), "модель недоступна")
	require.NotContains(t, err.Error(), "пустой ответ")
}

// TestAskEmptyParts — parts пусты и ошибки нет → прежнее «пустой ответ».
func TestAskEmptyParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			io.WriteString(w, `{"id":"s1"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
			io.WriteString(w, `{"parts":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "provider/model-x", "build", time.Second, 2)
	_, err := c.Ask("prompt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "пустой ответ")
}

// TestAskTimeoutNotRetried — таймаут одной попытки не повторяется: иначе
// ожидание умножается на число попыток (идея 2026-09-24, дефолт 45 с).
// Проверка по числу обращений к помощнику (детерминировано, без тайминга).
func TestAskTimeoutNotRetried(t *testing.T) {
	done := make(chan struct{})
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			io.WriteString(w, `{"id":"s1"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
			hits.Add(1)
			<-done // «висим» до конца теста
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	defer close(done)

	c := NewClient(srv.URL, "provider/model-x", "build", 120*time.Millisecond, 3)
	start := time.Now()
	_, err := c.Ask("prompt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "таймаут")
	require.EqualValues(t, 1, hits.Load(), "таймаут не повторяем")
	require.Less(t, time.Since(start), 2*time.Second)
}
