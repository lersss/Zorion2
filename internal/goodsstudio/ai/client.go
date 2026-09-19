// Package ai — ИИ-интеграция «заполнить комплектующие» (спека 99a.1 §7):
// HTTP к локальному opencode создателя, сборка промпта, разбор ответа,
// локальный фильтр бана/исключённого и вставка в слоты.
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client — HTTP-клиент к локальному opencode (спека 99a.1 §7.5).
// Транспорт (техническое белое пятно спеки — зона @developer):
// POST {url}/session → id сессии; POST {url}/session/{id}/message
// с телом {model, parts:[{type:"text", text: prompt}]} → текст ответа
// из parts. Модель передаётся явно (решение создателя: «модель должна
// быть явно указана»).
type Client struct {
	URL        string
	Model      string
	Timeout    time.Duration
	MaxRetries int
	HTTP       *http.Client
}

// NewClient создаёт клиента.
func NewClient(url, model string, timeout time.Duration, maxRetries int) *Client {
	return &Client{
		URL:        strings.TrimRight(url, "/"),
		Model:      model,
		Timeout:    timeout,
		MaxRetries: maxRetries,
		HTTP:       &http.Client{Timeout: timeout},
	}
}

// FillComponents отправляет промпт и возвращает сырой текст ответа.
// Повторы при сетевой ошибке/5xx (max_retries, спека 99a.1 §4.1).
func (c *Client) FillComponents(prompt string) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		text, retryable, err := c.fillOnce(prompt)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !retryable {
			break
		}
	}
	return "", lastErr
}

func (c *Client) fillOnce(prompt string) (string, bool, error) {
	sessionID, retryable, err := c.createSession()
	if err != nil {
		return "", retryable, err
	}
	// opencode API ждёт model объектом {providerID, modelID}, а не строкой
	// (конфиг: "provider/model"). Проверено живым прогоном 2026-09-19.
	providerID, modelID := c.Model, ""
	if i := strings.IndexByte(c.Model, '/'); i >= 0 {
		providerID, modelID = c.Model[:i], c.Model[i+1:]
	}
	body := map[string]interface{}{
		"model": map[string]string{"providerID": providerID, "modelID": modelID},
		// пустая строка — сырая модель, а не default_agent (manager) из
		// opencode.json: иначе сессия отвечает приветствием менеджера,
		// а не JSON-составом (проверено живым прогоном 2026-09-19).
		"agent": "",
		"parts": []map[string]string{{"type": "text", "text": prompt}},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", false, err
	}
	req, err := http.NewRequest(http.MethodPost, c.URL+"/session/"+sessionID+"/message", bytes.NewReader(data))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", true, err // сетевая ошибка — retryable
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		io.Copy(io.Discard, resp.Body)
		return "", true, fmt.Errorf("opencode: %s", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return "", false, fmt.Errorf("opencode: %s", resp.Status)
	}
	var out struct {
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", false, err
	}
	var sb strings.Builder
	for _, p := range out.Parts {
		if p.Type == "text" && p.Text != "" {
			sb.WriteString(p.Text)
		}
	}
	if sb.Len() == 0 {
		return "", false, fmt.Errorf("opencode: пустой ответ")
	}
	return sb.String(), false, nil
}

func (c *Client) createSession() (string, bool, error) {
	req, err := http.NewRequest(http.MethodPost, c.URL+"/session", nil)
	if err != nil {
		return "", false, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		io.Copy(io.Discard, resp.Body)
		return "", true, fmt.Errorf("opencode: %s", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return "", false, fmt.Errorf("opencode: %s", resp.Status)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", false, err
	}
	if out.ID == "" {
		return "", false, fmt.Errorf("opencode: пустой id сессии")
	}
	return out.ID, false, nil
}