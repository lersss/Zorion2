// internal/handlers/websocket_hub.go
package handlers

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// wsClient — обёртка над соединением с per-connection мьютексом на запись.
//
// gorilla/websocket НЕ потокобезопасна для параллельных записей,
// поэтому каждое соединение защищаем отдельным мьютексом.
// Глобальный мьютекс хаба защищает только map.
type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex // защищает запись в conn
}

func (c *wsClient) writeMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(messageType, data)
}

func (c *wsClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.Close()
}

// WebSocketHub — реестр активных соединений по userID.
type WebSocketHub struct {
	mu      sync.RWMutex
	clients map[string]*wsClient
}

func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients: make(map[string]*wsClient),
	}
}

// Register — регистрирует соединение. Если у пользователя уже было — старое закрываем.
func (h *WebSocketHub) Register(userID string, conn *websocket.Conn) {
	client := &wsClient{conn: conn}

	h.mu.Lock()
	old, hadOld := h.clients[userID]
	h.clients[userID] = client
	h.mu.Unlock()

	// Закрываем старое соединение ВНЕ глобального лока,
	// чтобы не блокировать других клиентов.
	if hadOld && old != nil {
		old.close()
	}

	log.Printf("WebSocket registered: user %s", userID)
}

// Unregister — удаляет соединение из реестра.
func (h *WebSocketHub) Unregister(userID string) {
	h.mu.Lock()
	delete(h.clients, userID)
	h.mu.Unlock()
}

// SendToUser — отправляет сообщение конкретному пользователю.
// Возвращает nil, если пользователь не в сети (это не ошибка).
func (h *WebSocketHub) SendToUser(userID string, message []byte) error {
	h.mu.RLock()
	client, ok := h.clients[userID]
	h.mu.RUnlock()
	if !ok {
		return nil
	}
	return client.writeMessage(websocket.TextMessage, message)
}

// Broadcast — отправляет сообщение ВСЕМ подключённым клиентам (спека 20a.1
// §5: broadcast всем, ролей пока нет; по ролям — после 99.2.14). Список
// клиентов копируется под RLock, запись — вне глобального лока, каждая —
// под per-connection мьютексом (AGENTS.md §0: одна горутина на запись).
// Ошибки записи игнорируются: отвалившийся клиент будет удалён при чтении.
func (h *WebSocketHub) Broadcast(message []byte) {
	h.mu.RLock()
	clients := make([]*wsClient, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		_ = c.writeMessage(websocket.TextMessage, message)
	}
}