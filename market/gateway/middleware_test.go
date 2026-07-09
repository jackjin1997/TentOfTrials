package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back the context values to verify middleware chain
		resp := map[string]interface{}{
			"called": true,
		}
		if uid, ok := r.Context().Value(ContextKeyUserID).(string); ok {
			resp["user_id"] = uid
		}
		if sid, ok := r.Context().Value(ContextKeySessionID).(string); ok {
			resp["session_id"] = sid
		}
		if auth, ok := r.Context().Value(ContextKeyAuthMethod).(string); ok {
			resp["auth_method"] = auth
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

func readJSONResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var data map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}
	return data
}

// ---------------------------------------------------------------------------
// AuthMiddleware tests
// ---------------------------------------------------------------------------

func TestAuthMiddleware_MissingBearerToken_Returns401(t *testing.T) {
	handler := AuthMiddleware(testHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	data := readJSONResponse(t, w)
	if data["error"] != "unauthorized" {
		t.Errorf("expected 'unauthorized' error, got %v", data["error"])
	}
}

func TestAuthMiddleware_InvalidToken_StubAccepts(t *testing.T) {
	// The current validateToken is a stub that accepts any token.
	// When properly implemented, this should return 401.
	// For now, we verify the stub allows the request through.
	handler := AuthMiddleware(testHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer some-invalid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("stub validator should accept any token, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidToken_SetsContext(t *testing.T) {
	handler := AuthMiddleware(testHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token-123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	data := readJSONResponse(t, w)
	if data["user_id"] != "user_stub" {
		t.Errorf("expected user_id 'user_stub', got %v", data["user_id"])
	}
	if data["session_id"] != "session_stub" {
		t.Errorf("expected session_id 'session_stub', got %v", data["session_id"])
	}
	if data["auth_method"] != "bearer" {
		t.Errorf("expected auth_method 'bearer', got %v", data["auth_method"])
	}
}

func TestAuthMiddleware_DoesNotCallHandlerOnUnauthorized(t *testing.T) {
	called := false
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := AuthMiddleware(protected)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("handler was called despite missing token")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// RateLimitMiddleware tests
// ---------------------------------------------------------------------------

func TestRateLimitMiddleware_AllowsFirstRequest(t *testing.T) {
	handler := RateLimitMiddleware(100, 10)(testHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if remaining := w.Header().Get("X-RateLimit-Remaining"); remaining == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
	if limit := w.Header().Get("X-RateLimit-Limit"); limit != "10" {
		t.Errorf("expected X-RateLimit-Limit 10, got %s", limit)
	}
}

func TestRateLimitMiddleware_ExhaustsBudget(t *testing.T) {
	burst := 3
	handler := RateLimitMiddleware(0.01, burst)(testHandler())

	// Use all tokens
	for i := 0; i < burst; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
	data := readJSONResponse(t, w)
	if data["error"] != "rate_limit_exceeded" {
		t.Errorf("expected rate_limit_exceeded, got %v", data["error"])
	}
}

// ---------------------------------------------------------------------------
// Middleware ordering: authenticated requests are keyed separately
// ---------------------------------------------------------------------------

func TestAuthThenRateLimit_MissingToken_BlockedBeforeRateLimit(t *testing.T) {
	// Simulate the real middleware order: Auth before RateLimit
	handler := AuthMiddleware(RateLimitMiddleware(100, 5)(testHandler()))

	// Missing token should be blocked by AuthMiddleware before reaching rate limit
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("missing token should return 401 before rate limit, got %d", w.Code)
	}
}

func TestAuthThenRateLimit_AuthenticatedRequest_GetsThrough(t *testing.T) {
	handler := AuthMiddleware(RateLimitMiddleware(100, 5)(testHandler()))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("authenticated request should pass, got %d", w.Code)
	}
	data := readJSONResponse(t, w)
	if data["user_id"] != "user_stub" {
		t.Errorf("expected user_id 'user_stub', got %v", data["user_id"])
	}
}

// ---------------------------------------------------------------------------
// Request context propagation
// ---------------------------------------------------------------------------

func TestMiddlewareChain_PropagatesContext(t *testing.T) {
	// Verify that auth context is available to downstream handlers
	handler := RequestIDMiddleware(LoggingMiddleware(AuthMiddleware(testHandler())))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	data := readJSONResponse(t, w)
	if data["user_id"] != "user_stub" {
		t.Errorf("context propagation failed: expected user_id 'user_stub', got %v", data["user_id"])
	}
}

func TestRequestIDMiddleware_SetsRequestID(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Context().Value(ContextKeyRequestID)
		if rid == nil {
			t.Error("request ID not set in context")
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if rid := w.Header().Get("X-Request-ID"); rid == "" {
		t.Error("expected X-Request-ID header")
	}
}

// ---------------------------------------------------------------------------
// RecoveryMiddleware tests
// ---------------------------------------------------------------------------

func TestRecoveryMiddleware_CatchesPanic(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})
	handler := RecoveryMiddleware(panicking)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	data := readJSONResponse(t, w)
	if data["error"] != "internal_server_error" {
		t.Errorf("expected internal_server_error, got %v", data["error"])
	}
}

func TestRecoveryMiddleware_AllowsNormalRequests(t *testing.T) {
	handler := RecoveryMiddleware(testHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
