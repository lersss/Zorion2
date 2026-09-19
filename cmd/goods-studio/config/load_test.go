package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoadStudioDefaults — дефолты по спеке 99a.1 §4.1 применяются,
// если поле не задано.
func TestLoadStudioDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "studio.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0644))

	cfg, err := LoadStudio(path)
	require.NoError(t, err)
	require.Equal(t, 8799, cfg.Port)
	require.Equal(t, "goods_data", cfg.DataDir)
	require.Equal(t, "http://127.0.0.1:3456", cfg.OpenCodeURL)
	require.Equal(t, 120, cfg.TimeoutS)
	require.Equal(t, 2, cfg.MaxRetries)
	require.Equal(t, 3000, cfg.AutoRefreshMS)
}

// TestLoadStudioExplicit — заданные поля не перетираются дефолтами.
func TestLoadStudioExplicit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "studio.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"port": 8801,
		"data_dir": "my_data",
		"opencode_url": "http://127.0.0.1:9999",
		"model": "deepseek-v4-flash",
		"timeout_s": 30,
		"max_retries": 0,
		"auto_refresh_ms": 5000
	}`), 0644))

	cfg, err := LoadStudio(path)
	require.NoError(t, err)
	require.Equal(t, 8801, cfg.Port)
	require.Equal(t, "my_data", cfg.DataDir)
	require.Equal(t, "http://127.0.0.1:9999", cfg.OpenCodeURL)
	require.Equal(t, "deepseek-v4-flash", cfg.Model)
	require.Equal(t, 30, cfg.TimeoutS)
	require.Equal(t, 2, cfg.MaxRetries) // 0 → дефолт
	require.Equal(t, 5000, cfg.AutoRefreshMS)
}

// TestLoadStudioMissing — отсутствующий файл — ошибка.
func TestLoadStudioMissing(t *testing.T) {
	_, err := LoadStudio(filepath.Join(t.TempDir(), "nope.json"))
	require.Error(t, err)
}