// internal/handlers/studio_ai.go
// Управление локальным ИИ-помощником (opencode serve) из студии товаров
// (спека 2026-09-24-студия-управление-локальным-ии §3): status/start/stop.
// Доступ — auth.AdminAuth на роутах (cmd/server/main.go).
package handlers

import (
	"net/http"

	"zorion/internal/goodsstudio/aiserve"
)

// aiController — то, что нужно хендлерам от менеджера помощника (тесты
// подставляют заглушку); реализуется *aiserve.Manager.
type aiController interface {
	Status() aiserve.Status
	Start() aiserve.StartResult
	Stop() aiserve.StopResult
}

// SetAIServe — подключение менеджера помощника (cmd/server/main.go).
func (h *StudioHandlers) SetAIServe(m *aiserve.Manager) { h.aiServe = m }

// aiStatus — текущее состояние помощника; без менеджера — «недоступен».
func (h *StudioHandlers) aiStatus() aiserve.Status {
	if h.aiServe == nil {
		return aiserve.Unavailable()
	}
	return h.aiServe.Status()
}

// AIStatus — GET /studio/api/ai/status (спека §3.1): 200 StatusView.
func (h *StudioHandlers) AIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		studioErr(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	studioJSON(w, http.StatusOK, h.aiStatus())
}

// AIStart — POST /studio/api/ai/start (спека §3.2): 200/202/409/500 с телом
// {started, status} либо {error, status}.
func (h *StudioHandlers) AIStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	if h.aiServe == nil {
		studioJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "управление недоступно", "status": aiserve.Unavailable(),
		})
		return
	}
	res := h.aiServe.Start()
	if res.Error != "" {
		studioJSON(w, res.Code, map[string]interface{}{"error": res.Error, "status": res.Status})
		return
	}
	studioJSON(w, res.Code, map[string]interface{}{"started": res.Started, "status": res.Status})
}

// AIStop — POST /studio/api/ai/stop (спека §3.3). Порядок: сначала гейт
// managed (управление недоступно), затем блокировка ИИ-прогона студии
// (решение создателя 2026-09-24, вариант 1).
func (h *StudioHandlers) AIStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	if h.aiServe == nil {
		studioJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "управление недоступно", "status": aiserve.Unavailable(),
		})
		return
	}
	if st := h.aiServe.Status(); !st.Managed {
		studioJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "управление недоступно: " + st.Reason, "status": st,
		})
		return
	}
	h.fillMu.Lock()
	generating := h.fillGenerating
	h.fillMu.Unlock()
	if generating {
		studioJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "идёт ИИ-прогон студии — сначала остановите прогон", "status": h.aiStatus(),
		})
		return
	}
	res := h.aiServe.Stop()
	if res.Error != "" {
		studioJSON(w, res.Code, map[string]interface{}{"error": res.Error, "status": res.Status})
		return
	}
	studioJSON(w, res.Code, map[string]interface{}{"stopped": res.Stopped, "status": res.Status})
}
