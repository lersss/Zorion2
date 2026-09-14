// internal/handlers/ws_notifier_test.go
// Тесты уведомлений NPC через WebSocket (спека 20a.1 §5): WSHub.Broadcast
// доставляет сообщение всем подключённым; WSNotifier шлёт
// npc_arrivals_batch по прибытии агента.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/npc"
)

// newWSClient поднимает WS-сервер (upgrade + регистрация в хабе + чтение)
// и возвращает клиентское соединение.
func newWSClient(t *testing.T, hub *WebSocketHub) *websocket.Conn {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		hub.Register("u1", conn)
		defer func() {
			hub.Unregister("u1")
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return client
}

func readWSMessage(t *testing.T, client *websocket.Conn) []byte {
	t.Helper()
	require.NoError(t, client.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, msg, err := client.ReadMessage()
	require.NoError(t, err)
	return msg
}

// WSHub.Broadcast — сообщение доходит до подключённого клиента.
func TestWSHubBroadcast(t *testing.T) {
	hub := NewWebSocketHub()
	client := newWSClient(t, hub)

	hub.Broadcast([]byte("hello"))
	require.Equal(t, "hello", string(readWSMessage(t, client)))
}

// WSNotifier: прибытие → npc_arrivals_batch с agent/world/observed_at.
func TestWSNotifierSendsBatch(t *testing.T) {
	hub := NewWebSocketHub()
	client := newWSClient(t, hub)

	manager := npc.NewManager(npcFakeStore{}, npcFakeWorlds{}, npc.DefaultSettings())
	notifier := NewWSNotifier(hub, manager, 200*time.Millisecond, 100)
	notifier.Start()
	t.Cleanup(notifier.Stop)

	observedAt := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	target := "22222222-2222-2222-2222-222222222222"
	notifier.NotifyArrival(models.NPCAgent{
		ID:            "a1",
		Name:          "Наблюдатель-1",
		TargetWorldID: &target,
	}, observedAt)

	msg := readWSMessage(t, client)
	var resp struct {
		Type     string `json:"type"`
		Since    string `json:"since"`
		Arrivals []struct {
			Agent struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"agent"`
			World struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"world"`
			ObservedAt string `json:"observed_at"`
		} `json:"arrivals"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(msg, &resp))
	require.Equal(t, "npc_arrivals_batch", resp.Type)
	require.Len(t, resp.Arrivals, 1)
	require.Equal(t, "a1", resp.Arrivals[0].Agent.ID)
	require.Equal(t, "Наблюдатель-1", resp.Arrivals[0].Agent.Name)
	require.Equal(t, target, resp.Arrivals[0].World.ID)
	require.Equal(t, "2026-09-14T12:00:00Z", resp.Arrivals[0].ObservedAt)
	require.Equal(t, 0, resp.Total)
}

// Несколько прибытий в окне — одной пачкой (batch, спека §2.2.C).
func TestWSNotifierBatchesArrivals(t *testing.T) {
	hub := NewWebSocketHub()
	client := newWSClient(t, hub)

	manager := npc.NewManager(npcFakeStore{}, npcFakeWorlds{}, npc.DefaultSettings())
	notifier := NewWSNotifier(hub, manager, 200*time.Millisecond, 100)
	notifier.Start()
	t.Cleanup(notifier.Stop)

	target := "22222222-2222-2222-2222-222222222222"
	notifier.NotifyArrival(models.NPCAgent{ID: "a1", Name: "A1", TargetWorldID: &target}, time.Now())
	notifier.NotifyArrival(models.NPCAgent{ID: "a2", Name: "A2", TargetWorldID: &target}, time.Now())

	msg := readWSMessage(t, client)
	var resp struct {
		Arrivals []struct {
			Agent struct {
				ID string `json:"id"`
			} `json:"agent"`
		} `json:"arrivals"`
	}
	require.NoError(t, json.Unmarshal(msg, &resp))
	require.Len(t, resp.Arrivals, 2, "два прибытия — одна пачка")
}

// Дроссель (не чаще 1 сообщения за интервал, избыток — total) покрыт
// юнит-тестами NotificationBatch (internal/npc/notification_batch_test.go);
// WS-слой логики дросселя не содержит — только FlushDue по таймеру,
// поэтому end-to-end тайминги здесь не проверяются (чтение WS с истёкшим
// дедлайном необратимо ломает gorilla-соединение).