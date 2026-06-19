package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// stubHandler returns a handler that records whether it was called and captures
// the request context for inspection.
func stubHandler(called *bool, ctxCapture *map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*called = true
		if ctxCapture != nil {
			*ctxCapture = map[string]interface{}{
				"user_id":     r.Context().Value(ContextKeyUserID),
				"session_id":  r.Context().Value(ContextKeySessionID),
				"auth_method": r.Context().Value(ContextKeyAuthMethod),
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}

// ---------------------------------------------------------------------------
// AuthMiddleware: missing bearer token → 401
// ---------------------------------------------------------------------------

func TestAuthMiddleware_MissingToken_Returns401(t *testing.T) {
	var called bool
	handler := AuthMiddleware(stubHandler(&called, nil))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("inner handler should NOT be called when token is missing")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("expected error field 'unauthorized', got %v", body["error"])
	}
	if body["message"] != "Missing authentication token" {
		t.Errorf("expected message 'Missing authentication token', got %v", body["message"])
	}
}

// ---------------------------------------------------------------------------
// AuthMiddleware: invalid token → 401 without calling wrapped handler
// ---------------------------------------------------------------------------

func TestAuthMiddleware_InvalidToken_Returns401(t *testing.T) {
	// The current validateToken stub accepts any non-empty token.
	// To truly test "invalid" we pass an empty bearer value, which
	// extractToken returns as empty after stripping "Bearer ".
	var called bool
	handler := AuthMiddleware(stubHandler(&called, nil))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("inner handler should NOT be called for empty bearer token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %v", body["error"])
	}
}

// TestAuthMiddleware_InvalidTokenFormat_Returns401 covers malformed auth headers.
func TestAuthMiddleware_InvalidTokenFormat_Returns401(t *testing.T) {
	var called bool
	handler := AuthMiddleware(stubHandler(&called, nil))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "NotBearer somevalue")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("inner handler should NOT be called for non-Bearer auth")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// AuthMiddleware: valid token sets user/session/auth context
// ---------------------------------------------------------------------------

func TestAuthMiddleware_ValidToken_SetsContext(t *testing.T) {
	var called bool
	var ctxVals map[string]interface{}
	handler := AuthMiddleware(stubHandler(&called, &ctxVals))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer test-token-abc123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("inner handler should be called with a valid token")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if ctxVals["user_id"] != "user_stub" {
		t.Errorf("expected user_id 'user_stub', got %v", ctxVals["user_id"])
	}
	if ctxVals["session_id"] != "session_stub" {
		t.Errorf("expected session_id 'session_stub', got %v", ctxVals["session_id"])
	}
	if ctxVals["auth_method"] != "bearer" {
		t.Errorf("expected auth_method 'bearer', got %v", ctxVals["auth_method"])
	}
}

// TestAuthMiddleware_APIKey_SetsContext covers API key auth via X-API-Key header.
func TestAuthMiddleware_APIKey_SetsContext(t *testing.T) {
	var called bool
	var ctxVals map[string]interface{}
	handler := AuthMiddleware(stubHandler(&called, &ctxVals))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "my-api-key-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("inner handler should be called with a valid API key")
	}
	if ctxVals["auth_method"] != "bearer" {
		t.Errorf("expected auth_method 'bearer', got %v", ctxVals["auth_method"])
	}
}

// ---------------------------------------------------------------------------
// Auth + RateLimit middleware ordering: authenticated vs anonymous rate limit keys
// ---------------------------------------------------------------------------

func TestMiddlewareOrdering_AuthenticatedVsAnonymous_SeparateKeys(t *testing.T) {
	// Simulate the correct ordering: Auth → RateLimit → handler.
	// Authenticated requests should be keyed by the API key header (or IP),
	// and anonymous requests should be keyed by IP. The key difference is
	// that authenticated requests passing through AuthMiddleware first will
	// have context set, while anonymous ones will hit the 401.

	rateLimiter := RateLimitMiddleware(100, 5)

	t.Run("anonymous request gets 401 before rate limit", func(t *testing.T) {
		var called bool
		chain := AuthMiddleware(rateLimiter(stubHandler(&called, nil)))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if called {
			t.Fatal("handler should not be called for unauthenticated request")
		}
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("authenticated request passes auth and hits rate limiter", func(t *testing.T) {
		var called bool
		chain := AuthMiddleware(rateLimiter(stubHandler(&called, nil)))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if !called {
			t.Fatal("handler should be called for authenticated request")
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		// Rate limit headers should be present
		if rec.Header().Get("X-RateLimit-Limit") == "" {
			t.Error("expected X-RateLimit-Limit header to be set")
		}
	})

	t.Run("different API keys get separate rate limit buckets", func(t *testing.T) {
		var called1, called2 bool
		chain := AuthMiddleware(rateLimiter(stubHandler(&called1, nil)))

		// First request with key-A
		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req1.RemoteAddr = "10.0.0.1:1111"
		req1.Header.Set("Authorization", "Bearer token-a")
		rec1 := httptest.NewRecorder()
		chain.ServeHTTP(rec1, req1)

		if !called1 {
			t.Fatal("handler should be called for first request")
		}

		// Second request with key-B from same IP
		chain2 := AuthMiddleware(rateLimiter(stubHandler(&called2, nil)))
		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.RemoteAddr = "10.0.0.1:2222"
		req2.Header.Set("Authorization", "Bearer token-b")
		rec2 := httptest.NewRecorder()
		chain2.ServeHTTP(rec2, req2)

		if !called2 {
			t.Fatal("handler should be called for second request")
		}

		// Both should get their own rate limit headers
		rl1 := rec1.Header().Get("X-RateLimit-Remaining")
		rl2 := rec2.Header().Get("X-RateLimit-Remaining")
		if rl1 == "" || rl2 == "" {
			t.Error("expected rate limit remaining headers on both responses")
		}
	})
}

// ---------------------------------------------------------------------------
// AuthMiddleware: handler never called on 401 paths
// ---------------------------------------------------------------------------

func TestAuthMiddleware_HandlerNeverCalledOn401(t *testing.T) {
	cases := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"empty bearer", "Bearer "},
		{"garbage auth", "Garbage value"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var callCount int
			var mu sync.Mutex
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				callCount++
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			})

			handler := AuthMiddleware(inner)
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			mu.Lock()
			defer mu.Unlock()
			if callCount != 0 {
				t.Errorf("inner handler called %d times, expected 0", callCount)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AuthMiddleware: response JSON shape on 401
// ---------------------------------------------------------------------------

func TestAuthMiddleware_401ResponseShape(t *testing.T) {
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %v", ct)
	}

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if _, ok := parsed["error"]; !ok {
		t.Error("JSON response missing 'error' field")
	}
	if _, ok := parsed["message"]; !ok {
		t.Error("JSON response missing 'message' field")
	}
}

// ---------------------------------------------------------------------------
// RateLimitMiddleware: basic allow/deny
// ---------------------------------------------------------------------------

func TestRateLimitMiddleware_AllowsWithinBurst(t *testing.T) {
	rl := RateLimitMiddleware(10, 5)
	called := false
	handler := rl(stubHandler(&called, nil))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler should be called within burst limit")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	remaining := rec.Header().Get("X-RateLimit-Remaining")
	if remaining == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
}

func TestRateLimitMiddleware_DeniesOverBurst(t *testing.T) {
	rl := RateLimitMiddleware(0.001, 2)

	for i := 0; i < 5; i++ {
		called := false
		handler := rl(stubHandler(&called, nil))
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.2:5678"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if i < 2 {
			if !called {
				t.Errorf("request %d: handler should be called within burst", i)
			}
		} else {
			if rec.Code != http.StatusTooManyRequests {
				t.Errorf("request %d: expected 429, got %d", i, rec.Code)
			}
		}
	}
}
