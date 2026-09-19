// internal/config/config_test.go
// Тесты конфига opencode (спека переноса-студии-товаров-iterC §4): дефолты
// из старого config/goods/studio.json (url 127.0.0.1:3456, model
// opencode/deepseek-v4-flash, timeout 120, retries 2) + env-переопределение.
package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// loadWithEnv — Load() с обязательными env (иначе log.Fatal) и очисткой
// opencode-переменных (дефолты).
func loadWithEnv(t *testing.T, set map[string]string) *Config {
	t.Helper()
	required := map[string]string{
		"DATABASE_URL": "postgres://t:t@127.0.0.1:5432/t",
		"REDIS_URL":    "redis://localhost:6379",
		"JWT_SECRET":   "test-secret-test-secret-test-secret-test-secret",
	}
	for k, v := range required {
		os.Setenv(k, v)
	}
	for k, v := range set {
		os.Setenv(k, v)
	}
	// очистка opencode-переменных — дефолты
	for _, k := range []string{"OPENCODE_URL", "OPENCODE_MODEL", "OPENCODE_TIMEOUT_S", "OPENCODE_MAX_RETRIES"} {
		if _, ok := set[k]; !ok {
			os.Unsetenv(k)
		}
	}
	t.Cleanup(func() {
		for k := range required {
			os.Unsetenv(k)
		}
		for k := range set {
			os.Unsetenv(k)
		}
	})
	return Load()
}

// TestOpenCodeDefaults — env не заданы: дефолты из studio.json.
func TestOpenCodeDefaults(t *testing.T) {
	cfg := loadWithEnv(t, nil)
	require.Equal(t, "http://127.0.0.1:3456", cfg.OpenCodeURL)
	require.Equal(t, "opencode/deepseek-v4-flash", cfg.OpenCodeModel)
	require.Equal(t, 120*time.Second, cfg.OpenCodeTimeout)
	require.Equal(t, 2, cfg.OpenCodeMaxRetries)
}

// TestOpenCodeEnvOverride — env переопределяют дефолты.
func TestOpenCodeEnvOverride(t *testing.T) {
	cfg := loadWithEnv(t, map[string]string{
		"OPENCODE_URL":        "http://127.0.0.1:9999",
		"OPENCODE_MODEL":      "opencode/other-model",
		"OPENCODE_TIMEOUT_S":  "30",
		"OPENCODE_MAX_RETRIES": "5",
	})
	require.Equal(t, "http://127.0.0.1:9999", cfg.OpenCodeURL)
	require.Equal(t, "opencode/other-model", cfg.OpenCodeModel)
	require.Equal(t, 30*time.Second, cfg.OpenCodeTimeout)
	require.Equal(t, 5, cfg.OpenCodeMaxRetries)
}