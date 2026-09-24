// Package ai — ИИ-интеграция «заполнить комплектующие» (спека 99a.1 §7):
// HTTP к локальному opencode создателя, сборка промпта, разбор ответа
// и вставка в слоты.
package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	Agent      string
	Timeout    time.Duration
	MaxRetries int
	HTTP       *http.Client
}

// NewClient создаёт клиента. agent — явный агент opencode (в 1.18 пустое
// значение резолвится в default_agent проекта — модель отвечает прозой
// менеджера, а не JSON; поэтому агент задаётся явно, дефолт build).
func NewClient(url, model, agent string, timeout time.Duration, maxRetries int) *Client {
	return &Client{
		URL:        strings.TrimRight(url, "/"),
		Model:      model,
		Agent:      agent,
		Timeout:    timeout,
		MaxRetries: maxRetries,
		HTTP:       &http.Client{Timeout: timeout},
	}
}

// Ask отправляет промпт и возвращает сырой текст ответа. Метод generic —
// используется тремя потоками (fill, одиночный/пакетный прогон описаний).
// Повторы при сетевой ошибке/5xx (max_retries, спека 99a.1 §4.1).
func (c *Client) Ask(prompt string) (string, error) {
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
	// Сессия закрывается после ответа: иначе они копятся (живой прогон
	// 2026-09-24 — 1882 сессии, один Ask = одна новая). Уборка не влияет на
	// результат вызова — ошибку закрытия не возвращаем.
	defer c.deleteSession(sessionID)
	// opencode API ждёт model объектом {providerID, modelID}, а не строкой
	// (конфиг: "provider/model"). Проверено живым прогоном 2026-09-19.
	providerID, modelID := c.Model, ""
	if i := strings.IndexByte(c.Model, '/'); i >= 0 {
		providerID, modelID = c.Model[:i], c.Model[i+1:]
	}
	body := map[string]interface{}{
		"model": map[string]string{"providerID": providerID, "modelID": modelID},
		// Явный агент (build по умолчанию): пустая строка в opencode 1.18
		// резолвится в default_agent проекта (manager) — сессия отвечает
		// приветствием менеджера, а не JSON-составом (проверено 2026-09-21).
		"agent": c.Agent,
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
		retryable, rerr := c.requestErr(err) // таймаут не повторяем
		return "", retryable, rerr
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
		Info struct {
			Error *struct {
				Name string `json:"name"`
				Data struct {
					Message string `json:"message"`
				} `json:"data"`
			} `json:"error"`
		} `json:"info"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", false, err
	}
	// Ошибку помощника opencode несёт info.error (ответ {info, parts}), а не
	// parts: причина должна быть видна пользователю (идея 2026-09-24), а не
	// растворяться в «пустом ответе».
	if out.Info.Error != nil {
		return "", false, fmt.Errorf("opencode: %s", assistantErrorText(out.Info.Error.Name, out.Info.Error.Data.Message))
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

// assistantErrorText — человеческая причина ошибки помощника (info.error,
// ответ opencode {info, parts}); «пустой ответ» оставлен только для parts
// без текста и без ошибки (идея 2026-09-24).
func assistantErrorText(name, message string) string {
	if message == "" {
		message = name
	}
	switch name {
	case "ProviderAuthError":
		return "модель недоступна: " + message
	case "ContentFilterError":
		return "запрос отклонён фильтром содержимого: " + message
	case "ContextOverflowError":
		return "переполнение контекста модели: " + message
	case "MessageOutputLengthError":
		return "ответ модели обрезан по длине"
	case "MessageAbortedError":
		return "запрос прерван: " + message
	case "StructuredOutputError":
		return "модель вернула неструктурированный ответ: " + message
	}
	if message == "" {
		return "неизвестная ошибка"
	}
	return message
}

// requestErr классифицирует ошибку HTTP-запроса: таймаут не повторяем —
// иначе зависший вызов умножается на число попыток (идея 2026-09-24:
// короткий таймаут на попытку); прочие сетевые ошибки — retryable
// (спека 99a.1 §4.1).
func (c *Client) requestErr(err error) (bool, error) {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return false, fmt.Errorf("opencode: таймаут ответа (%s)", c.Timeout)
	}
	return true, err
}

func (c *Client) createSession() (string, bool, error) {
	req, err := http.NewRequest(http.MethodPost, c.URL+"/session", nil)
	if err != nil {
		return "", false, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		retryable, rerr := c.requestErr(err)
		return "", retryable, rerr
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

// deleteSession закрывает сессию помощника (DELETE {url}/session/{id}) после
// ответа, чтобы сессии не копились (идея 2026-09-24). Сбой уборки намеренно
// не влияет на результат вызова: ответ уже получен/ошибка уже
// классифицирована, а «висящая» сессия — меньшая беда, чем сорванный вызов.
func (c *Client) deleteSession(id string) {
	req, err := http.NewRequest(http.MethodDelete, c.URL+"/session/"+id, nil)
	if err != nil {
		return
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
