// Package config — загрузка и валидация конфигов студии товаров
// (config/goods/studio.json, спека 99a.1 §4).
package config

// StudioConfig — параметры инструмента (config/goods/studio.json, спека 99a.1 §4.1).
type StudioConfig struct {
	Port          int    `json:"port"`           // порт студии (арт-студия заняла 8798)
	DataDir       string `json:"data_dir"`       // каталог состояния (вне git, как ai_drafts)
	OpenCodeURL   string `json:"opencode_url"`   // базовый URL локального opencode
	Model         string `json:"model"`          // модель для генерации (задаёт создатель; видна в UI, §7.5)
	TimeoutS      int    `json:"timeout_s"`      // таймаут ответа ИИ
	MaxRetries    int    `json:"max_retries"`    // повторы при сетевой ошибке/5xx
	AutoRefreshMS int    `json:"auto_refresh_ms"` // авто-обновление UI
}