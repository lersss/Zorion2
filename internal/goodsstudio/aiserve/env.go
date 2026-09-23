// internal/goodsstudio/aiserve/env.go
// Окружение дочернего процесса помощника (спека 2026-09-24 §0): пароль
// basic-auth считается незаданным. Чистая функция — тестируется независимо от
// ОС (спавн-специфика — в proc_windows.go).
package aiserve

import "strings"

// helperEnv — копия родительского окружения без OPENCODE_SERVER_PASSWORD и
// OPENCODE_SERVER_USERNAME (фильтр по имени, регистронезависимо; остальные
// переменные не трогаются). Иначе помощник, поднятый студией, наследует пароль
// из окружения игрового сервера и требует basic-auth, которого клиент студии
// (`internal/goodsstudio/ai`) не умеет → любой ИИ-запрос падает 401.
func helperEnv(parent []string) []string {
	out := make([]string, 0, len(parent))
	for _, kv := range parent {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		switch strings.ToUpper(name) {
		case "OPENCODE_SERVER_PASSWORD", "OPENCODE_SERVER_USERNAME":
			continue
		}
		out = append(out, kv)
	}
	return out
}
