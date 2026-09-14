// internal/handlers/npc_settings.go
// Настройки NPCManager из админки (спека 20a.1 §6): GET /admin/npc/settings
// и PATCH /admin/npc/settings. Хранятся в памяти менеджера (дефолты §2.4,
// после рестарта — снова дефолты; механизма БД-настроек в проекте нет,
// решение зафиксировано на этапе 2 в internal/npc/settings.go).
package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

// HandleSettings — диспетчер по методу для /admin/npc/settings.
func (h *AdminNPCHandlers) HandleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetSettings(w, r)
	case http.MethodPatch:
		h.PatchSettings(w, r)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// settingsResponse — текущие лимиты менеджера (§2.4) + глобальный рубильник
// пушей (спека 26a.1 §7.5).
func (h *AdminNPCHandlers) settingsResponse() map[string]interface{} {
	s := h.manager.Settings()
	return map[string]interface{}{
		"speed_factor":           s.SpeedFactor(),
		"tick_interval_sec":      s.TickInterval().Seconds(),
		"batch_size":             s.BatchSize(),
		"notify_interval_sec":    s.NotifyInterval().Seconds(),
		"notification_max_batch": s.NotificationMaxBatch(),
		"notify_enabled_global":  s.NotifyGlobalEnabled(),
	}
}

func (h *AdminNPCHandlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSONStatus(w, http.StatusOK, h.settingsResponse())
}

// PatchSettings — PATCH /admin/npc/settings: {speed_factor?, tick_interval_sec?,
// batch_size?, notify_enabled_global?}. Значения — из админки, применяются
// сразу к менеджеру. notify_interval/notification_max_batch — только чтение
// (используются на этапе 5, WS-уведомления).
func (h *AdminNPCHandlers) PatchSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpeedFactor          *float64 `json:"speed_factor"`
		TickIntervalSec      *float64 `json:"tick_interval_sec"`
		BatchSize            *int     `json:"batch_size"`
		NotifyEnabledGlobal  *bool    `json:"notify_enabled_global"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if req.SpeedFactor == nil && req.TickIntervalSec == nil && req.BatchSize == nil && req.NotifyEnabledGlobal == nil {
		writeJSONError(w, "Нет полей для обновления", http.StatusBadRequest)
		return
	}

	s := h.manager.Settings()
	if req.SpeedFactor != nil {
		if *req.SpeedFactor <= 0 {
			writeJSONError(w, "speed_factor должен быть > 0", http.StatusBadRequest)
			return
		}
		s.SetSpeedFactor(*req.SpeedFactor)
	}
	if req.TickIntervalSec != nil {
		if *req.TickIntervalSec < 1 {
			writeJSONError(w, "tick_interval_sec должен быть ≥ 1", http.StatusBadRequest)
			return
		}
		s.SetTickInterval(time.Duration(*req.TickIntervalSec * float64(time.Second)))
	}
	if req.BatchSize != nil {
		if *req.BatchSize < 1 {
			writeJSONError(w, "batch_size должен быть ≥ 1", http.StatusBadRequest)
			return
		}
		s.SetBatchSize(*req.BatchSize)
	}
	if req.NotifyEnabledGlobal != nil {
		s.SetNotifyGlobalEnabled(*req.NotifyEnabledGlobal)
	}
	writeJSONStatus(w, http.StatusOK, h.settingsResponse())
}