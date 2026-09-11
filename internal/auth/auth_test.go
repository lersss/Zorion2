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
	_, err := GenerateToken("user1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не инициализирован")
}

func TestVerifyTokenWithoutSecret(t *testing.T) {
	resetJWT()
	_, err := VerifyToken("zzz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не инициализирован")
}

func TestGenerateTokenVerifyRoundTrip(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("user-42")
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	userID, err := VerifyToken(tok)
	require.NoError(t, err)
	assert.Equal(t, "user-42", userID)
}

func TestVerifyMalformedToken(t *testing.T) {
	requireSecret(t)

	_, err := VerifyToken("garbage")
	require.Error(t, err)

	_, err = VerifyToken("only.two")
	require.Error(t, err)

	_, err = VerifyToken("a.b.c.d") // лишняя секция
	require.Error(t, err)
}

func TestVerifyTamperedSignature(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("user1")
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

	_, err = VerifyToken(tampered)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

func TestVerifyExpiredToken(t *testing.T) {
	requireSecret(t)

	claims := Claims{
		UserID: "user1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	secret, err := getJWTSecret()
	require.NoError(t, err)

	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)

	_, err = VerifyToken(tok)
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

	_, err = VerifyToken(tok)
	require.Error(t, err)
}

func TestVerifyRejectsForeignSecret(t *testing.T) {
	// Инициализируем секрет A, но подписываем секретом B.
	resetJWT()
	require.NoError(t, InitJWTSecret("secret-a-that-is-long-enough-32-bytes!"))
	otherSecret := []byte("secret-b-that-is-long-enough-but-different")

	claims := Claims{UserID: "user1", RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(otherSecret)
	require.NoError(t, err)

	_, err = VerifyToken(tok)
	require.Error(t, err)
}

func TestTokenTTL24Hours(t *testing.T) {
	requireSecret(t)

	tok, err := GenerateToken("user1")
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

// ==================== MIDDLEWARE ====================

func requireEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("JWT_SECRET", "test-secret-that-is-long-enough-32b!123")
	t.Setenv("ADMIN_PASSWORD", "admin123")
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

	tok, err := GenerateToken("user-77")
	require.NoError(t, err)

	var gotUserID string
	handler := AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(UserIDKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-77", gotUserID)
	assert.NotNil(t, context.WithValue(req.Context(), UserIDKey, "x")) // UserIDKey — валидный ключ
}

// ==================== ADMIN AUTH ====================

func TestAdminAuthNoPassword(t *testing.T) {
	requireEnv(t)

	handler := AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/whatever", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAdminAuthWrongPassword(t *testing.T) {
	requireEnv(t)

	handler := AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/whatever", nil)
	req.Header.Set("X-Admin-Password", "wrong")
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAdminAuthCorrectPassword(t *testing.T) {
	requireEnv(t)

	called := false
	handler := AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/whatever", nil)
	req.Header.Set("X-Admin-Password", "admin123")
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}