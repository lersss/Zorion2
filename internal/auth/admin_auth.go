package auth

import "net/http"

// AdminAuth — доступ к админке по роли из JWT (спека 99.2.14 §3):
// admin и skycomposer. Сигнатура сохранена от старого X-Admin-Password,
// чтобы не менять регистрацию роутов в main.go.
func AdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return AuthMiddleware(RequireRole(next, RoleAdmin, RoleSkycomposer))
}

// SkycomposerAuth — управление пользователями (§6): только skycomposer.
func SkycomposerAuth(next http.HandlerFunc) http.HandlerFunc {
	return AuthMiddleware(RequireRole(next, RoleSkycomposer))
}
