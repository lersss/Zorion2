// internal/models/generation_config.go
package models

import (
	"encoding/json"
	"time"
)

// GenerationConfig — строка реестра конфигов генерации (таблица generation_config,
// спека 99.2.3 §3). Паттерн «JSON-дефолты в коде + override в БД» — как матрица
// совместимости. Payload — произвольный JSON по ключу.
type GenerationConfig struct {
	Key       string          `json:"key"`
	Payload   json.RawMessage `json:"payload"`
	UpdatedAt time.Time       `json:"updated_at"`
}