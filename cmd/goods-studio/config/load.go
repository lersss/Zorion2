package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadStudio читает и валидирует config/goods/studio.json.
// Дефолты — по спеке 99a.1 §4.1 (применяются, если поле не задано).
func LoadStudio(path string) (*StudioConfig, error) {
	cfg := &StudioConfig{}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Port == 0 {
		cfg.Port = 8799
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "goods_data"
	}
	if cfg.OpenCodeURL == "" {
		cfg.OpenCodeURL = "http://127.0.0.1:3456"
	}
	if cfg.TimeoutS == 0 {
		cfg.TimeoutS = 120
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 2
	}
	if cfg.AutoRefreshMS == 0 {
		cfg.AutoRefreshMS = 3000
	}
	return cfg, nil
}