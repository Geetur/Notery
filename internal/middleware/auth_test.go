package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() { gin.SetMode(gin.TestMode) }

func makeToken(secret string, claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}

func callMiddleware(mw gin.HandlerFunc, authHeader string) (*httptest.ResponseRecorder, *gin.Context) {
	return callMiddlewareWithPath(mw, authHeader, "/test")
}

func callMiddlewareWithPath(mw gin.HandlerFunc, authHeader, path string) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.Use(mw)
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	req := httptest.NewRequest("GET", path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	r.ServeHTTP(w, req)
	return w, c
}

// ===== RequireAuth Tests =====

func TestRequireAuth_ValidToken(t *testing.T) {
	secret := "test-secret"
	token := makeToken(secret, jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iss":     "notery-api",
		"aud":     "notery-web",
	})

	w, _ := callMiddleware(RequireAuth(secret), "Bearer "+token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	w, _ := callMiddleware(RequireAuth("secret"), "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_InvalidFormat(t *testing.T) {
	w, _ := callMiddleware(RequireAuth("secret"), "NotBearer token")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	secret := "test-secret"
	token := makeToken(secret, jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(-time.Hour).Unix(), // expired
		"iss":     "notery-api",
		"aud":     "notery-web",
	})

	w, _ := callMiddleware(RequireAuth(secret), "Bearer "+token)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", w.Code)
	}
}

func TestRequireAuth_WrongSecret(t *testing.T) {
	token := makeToken("correct-secret", jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iss":     "notery-api",
		"aud":     "notery-web",
	})

	w, _ := callMiddleware(RequireAuth("wrong-secret"), "Bearer "+token)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong secret, got %d", w.Code)
	}
}

func TestRequireAuth_MissingUserIDClaim(t *testing.T) {
	secret := "test-secret"
	token := makeToken(secret, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
		"iss": "notery-api",
		"aud": "notery-web",
		// no user_id
	})

	w, _ := callMiddleware(RequireAuth(secret), "Bearer "+token)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing user_id, got %d", w.Code)
	}
}

func TestRequireAuth_NonNumericUserID(t *testing.T) {
	secret := "test-secret"
	token := makeToken(secret, jwt.MapClaims{
		"user_id": "not-a-number",
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iss":     "notery-api",
		"aud":     "notery-web",
	})

	w, _ := callMiddleware(RequireAuth(secret), "Bearer "+token)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-numeric user_id, got %d", w.Code)
	}
}

func TestRequireAuth_AlgorithmNone(t *testing.T) {
	// Attempt alg:none attack
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iss":     "notery-api",
		"aud":     "notery-web",
	})
	s, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	w, _ := callMiddleware(RequireAuth("secret"), "Bearer "+s)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for alg:none attack, got %d", w.Code)
	}
}

// ===== OptionalAuth Tests =====

func TestOptionalAuth_NoHeader(t *testing.T) {
	w, _ := callMiddleware(OptionalAuth("secret"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for anonymous, got %d", w.Code)
	}
}

func TestOptionalAuth_ValidToken(t *testing.T) {
	secret := "test-secret"
	token := makeToken(secret, jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iss":     "notery-api",
		"aud":     "notery-web",
	})

	w, _ := callMiddleware(OptionalAuth(secret), "Bearer "+token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestOptionalAuth_InvalidToken(t *testing.T) {
	w, _ := callMiddleware(OptionalAuth("secret"), "Bearer invalid-token")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for invalid token (proceeds as anonymous), got %d", w.Code)
	}
}

func TestOptionalAuth_ExpiredToken(t *testing.T) {
	secret := "test-secret"
	token := makeToken(secret, jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(-time.Hour).Unix(),
		"iss":     "notery-api",
		"aud":     "notery-web",
	})

	w, _ := callMiddleware(OptionalAuth(secret), "Bearer "+token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for expired token (proceeds as anonymous), got %d", w.Code)
	}
}

func TestOptionalAuth_InvalidFormat(t *testing.T) {
	w, _ := callMiddleware(OptionalAuth("secret"), "Token xyz")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for bad format (proceeds as anonymous), got %d", w.Code)
	}
}

func TestOptionalAuth_QueryTokenValid(t *testing.T) {
	secret := "test-secret"
	token := makeToken(secret, jwt.MapClaims{
		"user_id": "42",
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iss":     "notery-api",
		"aud":     "notery-web",
	})

	w, _ := callMiddlewareWithPath(OptionalAuth(secret), "", "/test?token="+token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid query token, got %d", w.Code)
	}
}

func TestOptionalAuth_QueryTokenInvalid(t *testing.T) {
	w, _ := callMiddlewareWithPath(OptionalAuth("secret"), "", "/test?token=invalid-token")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for invalid query token (proceeds as anonymous), got %d", w.Code)
	}
}
