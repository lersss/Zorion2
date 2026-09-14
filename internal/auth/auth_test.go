// internal/auth/auth_test.go
// Тесты в package auth (внутренние), чтобы можно было сбрасывать
// пакетное состояние JWT-секрета между тестами.
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== ХЕЛПЕРЫ ====================

// resetJWT — сбрасывает пакетное состояние секрета.
// Без этого тесты зависели бы от порядка выполнения.
func resetJWT() {
	jwtSecretMu.Lock()
	defer jwtSecretMu.Unlock()
	jwtSecret = nil
	jwtSecretReady = false
}

// requireSecret — подготавливает инициализированный секрет.
func requireSecret(t *testing.T) {
	t.Helper()
	resetJWT()
	require.NoError(t, InitJWTSecret("test-secret-that-is-long-enough-32b!123"))
}

// ==================== INITJWTSecret ====================

func TestInitJWTSecretTooShort(t *testing.T) {
	resetJWT()
	err := InitJWTSecret("short")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "слишком короткий")

	// Секрет не должен считаться инициализированным.
	_, err = getJWTSecret()
	require.Error(t, err)
}

func TestInitJWTSecretTwice(t *testing.T) {
	resetJWT()
	require.NoError(t, InitJWTSecret("test-secret-that-is-long-enough-32b!123"))

	err := InitJWTSecret("another-test-secret-that-is-long-enough!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "уже инициализирован")
}

// ==================== ТОКЕНЫ ====================

func TestGenerateTokenWithoutSecret(t *testing.T) {
	resetJWT()
	_, err := GenerateToken("user1", "player")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не инициализирован")
}

func TestVerifyTokenWithoutSecret(t *testing.T) {
	resetJWT()
	_, _, err := VerifyToken("zzz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не инициализирован")
}

func TestGenerateTokenVerifyRoundTrip(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("user-42", string(RoleAdmin))
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	userID, role, err := VerifyToken(tok)
	require.NoError(t, err)
	assert.Equal(t, "user-42", userID)
	assert.Equal(t, string(RoleAdmin), role)
}

func TestVerifyMalformedToken(t *testing.T) {
	requireSecret(t)

	_, _, err := VerifyToken("garbage")
	require.Error(t, err)

	_, _, err = VerifyToken("only.two")
	require.Error(t, err)

	_, _, err = VerifyToken("a.b.c.d") // лишняя секция
	require.Error(t, err)
}

func TestVerifyTamperedSignature(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("user1", "player")
	require.NoError(t, err)

	// Портим байт в середине сигнатуры. (Замена последнего символа
	// base64url меняет только padding-биты и не влияет на байты подписи.)
	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3)
	sig := []byte(parts[2])
	require.Greater(t, len(sig), 20, "подпись HS256 должна быть длинной")
	orig := sig[10]
	if orig == 'A' {
		sig[10] = 'B'
	} else {
		sig[10] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(sig)

	_, _, err = VerifyToken(tampered)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

func TestVerifyExpiredToken(t *testing.T) {
	requireSecret(t)

	claims := Claims{
		UserID: "user1",
		Role:   "player",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	secret, err := getJWTSecret()
	require.NoError(t, err)

	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)

	_, _, err = VerifyToken(tok)
	require.Error(t, err)
}

func TestVerifyRejectsNoneAlgorithm(t *testing.T) {
	requireSecret(t)

	// Токен с alg=none и пустой подписью — классическая JWT-атака.
	tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"user_id": "admin",
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(tok, "eyJ")) // есть payload

	_, _, err = VerifyToken(tok)
	require.Error(t, err)
}

func TestVerifyRejectsForeignSecret(t *testing.T) {
	// Инициализируем секрет A, но подписываем секретом B.
	resetJWT()
	require.NoError(t, InitJWTSecret("secret-a-that-is-long-enough-32-bytes!"))
	otherSecret := []byte("secret-b-that-is-long-enough-but-different")

	claims := Claims{UserID: "user1", Role: "player", RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(otherSecret)
	require.NoError(t, err)

	_, _, err = VerifyToken(tok)
	require.Error(t, err)
}

func TestTokenTTL24Hours(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("user1", "player")
	require.NoError(t, err)

	// Разбираем без подписи — нам нужны только claims.
	parsed, _, err := new(jwt.Parser).ParseUnverified(tok, &Claims{})
	require.NoError(t, err)

	claims, ok := parsed.Claims.(*Claims)
	require.True(t, ok)

	require.NotNil(t, claims.ExpiresAt)
	require.NotNil(t, claims.IssuedAt)

	ttl := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	assert.GreaterOrEqual(t, ttl, 24*time.Hour-2*time.Second)
	assert.LessOrEqual(t, ttl, 24*time.Hour+2*time.Second)
}

// Старый токен (без role в claims) трактуется как player — безопасный дефолт
// наименьших прав (спека 99.2.14 §3): игру не ломает, в админку не пускает.
func TestVerifyTokenMissingRoleDefaultsToPlayer(t *testing.T) {
	requireSecret(t)

	claims := Claims{
		UserID: "user1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	secret, err := getJWTSecret()
	require.NoError(t, err)
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)

	userID, role, err := VerifyToken(tok)
	require.NoError(t, err)
	assert.Equal(t, "user1", userID)
	assert.Equal(t, string(RolePlayer), role)
}

// ==================== MIDDLEWARE ====================

func requireEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("JWT_SECRET", "test-secret-that-is-long-enough-32b!123")
}

func TestAuthMiddlewareMissingHeader(t *testing.T) {
	requireSecret(t)

	handler := AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddlewareBadFormat(t *testing.T) {
	requireSecret(t)

	handler := AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	requireSecret(t)

	handler := AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddlewareValid(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("user-77", string(RoleSkycomposer))
	require.NoError(t, err)

	var gotUserID, gotRole string
	handler := AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(UserIDKey).(string)
		gotRole, _ = r.Context().Value(RoleKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-77", gotUserID)
	assert.Equal(t, string(RoleSkycomposer), gotRole)
	assert.NotNil(t, context.WithValue(req.Context(), UserIDKey, "x")) // UserIDKey — валидный ключ
}

// ==================== WS: TOKEN В QUERY ====================
//
// Браузерный WebSocket API не умеет кастомные заголовки, поэтому для /ws
// токен принимается в query (?token=JWT) — только для этого пути (баг
// «браузерный WS-тост NPC мёртв», этап 6).

// /ws с токеном в query — 200, контекст заполнен.
func TestAuthMiddlewareWSQueryToken(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("user-ws", string(RolePlayer))
	require.NoError(t, err)

	var gotUserID string
	handler := AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(UserIDKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ws?token="+tok, nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-ws", gotUserID)
}

// /ws без токена — 401 (и заголовка нет, и query пуст).
func TestAuthMiddlewareWSQueryTokenMissing(t *testing.T) {
	requireSecret(t)

	handler := AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// Query-токен НЕ работает для остальных путей — fallback только для /ws
// (безопасность: не расширяем поверхность остальных ручек).
func TestAuthMiddlewareQueryTokenNotForOtherPaths(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("user-x", string(RolePlayer))
	require.NoError(t, err)

	handler := AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/whatever?token="+tok, nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ==================== REQUIRE ROLE ====================

// testRequireRole — хелпер: RequireRole поверх заглушки.
func testRequireRole(t *testing.T, tokenRole string, roles ...Role) (int, string) {
	requireSecret(t)

	tok, err := GenerateToken("u1", tokenRole)
	require.NoError(t, err)

	handler := RequireRole(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, roles...)

	req := httptest.NewRequest(http.MethodGet, "/admin/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	AuthMiddleware(handler)(rec, req)

	body := rec.Body.String()
	return rec.Code, body
}

func TestRequireRoleAllows(t *testing.T) {
	code, _ := testRequireRole(t, string(RoleAdmin), RoleAdmin, RoleSkycomposer)
	assert.Equal(t, http.StatusOK, code)

	code, _ = testRequireRole(t, string(RoleSkycomposer), RoleAdmin, RoleSkycomposer)
	assert.Equal(t, http.StatusOK, code)

	code, _ = testRequireRole(t, string(RoleSkycomposer), RoleSkycomposer)
	assert.Equal(t, http.StatusOK, code)
}

func TestRequireRoleDenies(t *testing.T) {
	// player не проходит ни в админку, ни в раздел пользователей.
	code, body := testRequireRole(t, string(RolePlayer), RoleAdmin, RoleSkycomposer)
	assert.Equal(t, http.StatusForbidden, code)
	assert.Contains(t, body, `"Недостаточно прав"`)

	code, _ = testRequireRole(t, string(RoleAdmin), RoleSkycomposer)
	assert.Equal(t, http.StatusForbidden, code)
}

// ==================== ADMIN AUTH (ролевая) ====================

// testAdminAuth — хелпер: AdminAuth поверх заглушки.
func testAdminAuth(t *testing.T, tokenRole string) (int, bool) {
	requireSecret(t)

	tok, err := GenerateToken("u1", tokenRole)
	require.NoError(t, err)

	called := false
	handler := AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler(rec, req)

	return rec.Code, called
}

func TestAdminAuthRequiresToken(t *testing.T) {
	requireSecret(t)

	handler := AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/whatever", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAdminAuthPlayerDenied(t *testing.T) {
	code, called := testAdminAuth(t, string(RolePlayer))
	assert.Equal(t, http.StatusForbidden, code)
	assert.False(t, called)
}

func TestAdminAuthAdminAllowed(t *testing.T) {
	code, called := testAdminAuth(t, string(RoleAdmin))
	assert.Equal(t, http.StatusOK, code)
	assert.True(t, called)
}

func TestAdminAuthSkycomposerAllowed(t *testing.T) {
	code, called := testAdminAuth(t, string(RoleSkycomposer))
	assert.Equal(t, http.StatusOK, code)
	assert.True(t, called)
}

// ==================== SKYCOMPOSER AUTH ====================

func TestSkycomposerAuthDeniesAdmin(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("u1", string(RoleAdmin))
	require.NoError(t, err)

	handler := SkycomposerAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSkycomposerAuthAllows(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("u1", string(RoleSkycomposer))
	require.NoError(t, err)

	called := false
	handler := SkycomposerAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}
