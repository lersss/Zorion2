// Package comfy — интеграция с ComfyUI API (спека 67a.1 §5.1):
// POST /prompt (txt2img/img2img), polling /history/{pid}, скачивание /view.
package comfy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Client — HTTP-клиент ComfyUI API.
type Client struct {
	BaseURL      string
	HTTP         *http.Client
	PollInterval time.Duration
	Timeout      time.Duration
}

// NewClient создаёт клиент с интервалом опроса и таймаутом ожидания генерации.
func NewClient(baseURL string, pollIntervalS, timeoutS int) *Client {
	return &Client{
		BaseURL:      baseURL,
		HTTP:         &http.Client{Timeout: 30 * time.Second},
		PollInterval: time.Duration(pollIntervalS) * time.Second,
		Timeout:      time.Duration(timeoutS) * time.Second,
	}
}

// Submit отправляет workflow и возвращает prompt_id.
func (c *Client) Submit(wf map[string]interface{}) (string, error) {
	body, err := json.Marshal(map[string]interface{}{"prompt": wf})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/prompt", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("comfy /prompt: %s: %s", resp.Status, string(b))
	}
	var out struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.PromptID == "" {
		return "", fmt.Errorf("comfy /prompt: пустой prompt_id")
	}
	return out.PromptID, nil
}

// WaitAndDownload опрашивает /history/{pid} до успеха/ошибки/таймаута
// и скачивает результат в outPath. ok=false — ошибка генерации (не сети).
func (c *Client) WaitAndDownload(pid, outPath string) (bool, error) {
	deadline := time.Now().Add(c.Timeout)
	for time.Now().Before(deadline) {
		time.Sleep(c.PollInterval)
		h, err := c.history(pid)
		if err != nil {
			continue // сетевые сбои опроса пропускаем (как прототип)
		}
		e, ok := h[pid]
		if !ok {
			continue
		}
		switch e.Status.StatusStr {
		case "success":
			imgs := e.Outputs["7"].Images
			if len(imgs) == 0 {
				return false, fmt.Errorf("comfy: нет изображений в выводе")
			}
			return true, c.download(imgs[0], outPath)
		case "error":
			return false, nil
		}
	}
	return false, fmt.Errorf("comfy: таймаут ожидания генерации")
}

type historyEntry struct {
	Status struct {
		StatusStr string `json:"status_str"`
	} `json:"status"`
	Outputs map[string]struct {
		Images []struct {
			Filename  string `json:"filename"`
			Subfolder string `json:"subfolder"`
			Type      string `json:"type"`
		} `json:"images"`
	} `json:"outputs"`
}

func (c *Client) history(pid string) (map[string]historyEntry, error) {
	resp, err := c.HTTP.Get(c.BaseURL + "/history/" + pid)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var h map[string]historyEntry
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, err
	}
	return h, nil
}

func (c *Client) download(img struct {
	Filename  string `json:"filename"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}, outPath string) error {
	q := url.Values{}
	q.Set("filename", img.Filename)
	q.Set("subfolder", img.Subfolder)
	q.Set("type", img.Type)
	resp, err := c.HTTP.Get(c.BaseURL + "/view?" + q.Encode())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("comfy /view: %s", resp.Status)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}