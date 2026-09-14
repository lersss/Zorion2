package auth

import (
	"context"
	"log"
	"net/http"
	"strings"

	"zorion/internal/models"
)

type contextKey string

const UserIDKey contextKey = "userID"
const RoleKey contextKey = "role"

// Role — алиас модели роли пользователя (один факт — одно место: models).
type Role = models.Role

const (
	RolePlayer      = models.RolePlayer
	RoleAdmin       = models.RoleAdmin
	RoleSkycomposer = models.RoleSkycomposer
)

// AuthMiddleware проверяет JWT и добавляет userID и role в контекст запроса.
// Токен — в заголовке Authorization: Bearer <JWT>. Для WebSocket (/ws)
// дополнительно принимается токен в query (?token=JWT): браузерный
// WebSocket API не позволяет задавать кастомные заголовки (спека 20a.1 §5,
// этап 6). Оценка безопасности: WS-URL не попадает в адресную строку,
// историю браузера и referer, серверные логи URL не пишут, JWT
// короткоживущий (TTL 24ч, jwt.go); fallback ограничен ТОЛЬКО путём /ws —
// остальные ручки расширенной поверхности не получают.
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenString := ""
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				log.Printf("Auth: invalid header format: %s", authHeader)
				http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
				return
			}
			tokenString = parts[1]
		} else if r.URL.Path == "/ws" {
			// Браузерный WebSocket не умеет заголовки — токен в query.
			tokenString = r.URL.Query().Get("token")
			if tokenString == "" {
				log.Println("Auth: no Authorization header")
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
		} else {
			log.Println("Auth: no Authorization header")
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		userID, role, err := VerifyToken(tokenString)
		if err != nil {
			log.Printf("Auth: token verification failed: %v", err)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		log.Printf("Auth: user %s (role %s) authenticated", userID, role)
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, RoleKey, role)
		next(w, r.WithContext(ctx))
	}
}

// RequireRole ограничивает доступ по роли из контекста (спека 99.2.14 §3).
// При несовпадении — 403 {"error":"Недостаточно прав"} (JSON, как writeJSONError).
func RequireRole(next http.HandlerFunc, roles ...Role) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role, ok := r.Context().Value(RoleKey).(string)
		if !ok || role == "" {
			writeRoleForbidden(w)
			return
		}
		for _, allowed := range roles {
			if role == string(allowed) {
				next(w, r)
				return
			}
		}
		writeRoleForbidden(w)
	}
}

// writeRoleForbidden — 403 в формате {"error":"Недостаточно прав"}.
func writeRoleForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"error":"Недостаточно прав"}`))
}
