// internal/goodsstudio/aiserve/env_test.go
// Фильтр окружения помощника (спека 2026-09-24 §0): пароль/юзер basic-auth
// снимаются, остальное сохраняется. Без build-тега — тест собирается и на
// не-Windows (спавн-специфика не тестируется).
package aiserve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHelperEnvStripsOpenCodeAuth(t *testing.T) {
	parent := []string{
		"PATH=C:\\Windows",
		"OPENCODE_SERVER_PASSWORD=secret",
		"opencode_server_username=admin",
		"OPENCODE_MODEL=opencode/deepseek-v4-flash",
		"HOME=C:\\Users\\admin",
	}
	got := helperEnv(parent)
	require.Equal(t, []string{
		"PATH=C:\\Windows",
		"OPENCODE_MODEL=opencode/deepseek-v4-flash",
		"HOME=C:\\Users\\admin",
	}, got)
	// родительский слайс не мутируется
	require.Len(t, parent, 5)
	require.Contains(t, parent, "OPENCODE_SERVER_PASSWORD=secret")

	// прочие OPENCODE_* остаются нетронутыми
	require.Equal(t, []string{"OPENCODE_URL=http://127.0.0.1:3456"},
		helperEnv([]string{"OPENCODE_URL=http://127.0.0.1:3456"}))

	// значение содержит '=' — имя берётся до первого '='
	require.Empty(t, helperEnv([]string{"OPENCODE_SERVER_PASSWORD=a=b=c"}))

	// пустой вход
	require.Empty(t, helperEnv(nil))
}
