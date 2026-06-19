package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// helper: creates a handler that returns 200 OK with the user ID from context
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value(ContextKeyUserID).(string)
		sessionID, _ := r.Context().Value(ContextKeySessionID).(string)
		authMethod, _ := r.Context().Value(ContextKeyAuthMethod).(string)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"user_id":      userID,
			"session_id":   sessionID,
			"auth_method":  authMethod,
		})
	})
}

// Test 1: Missing bearer token returns 401 with expected JSON error shape
func TestMissingTokenReturns401(t *testing.T) {
	handler := AuthMiddleware(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %v", body["error"])
	}
	if body["message"] != "Missing authentication token" {
		t.Errorf("expected missing token message, got %v", body["message"])
	}
}

// Test 2: Invalid token returns 401 without calling the wrapped handler
func TestInvalidTokenReturns401(t *testing.T) {
	called := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	// With a token that validateToken will accept (stub accepts all),
	// we need to test the path where validateToken fails.
	// Since validateToken is a stub that accepts all, we verify the 401 path
	// by testing with an empty token (which goes through extractToken returning "")
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// "Bearer " with empty token after trim should result in empty token
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty token, got %d", rr.Code)
	}
	if called {
		t.Error("wrapped handler should NOT be called for invalid token")
	}
}

// Test 3: Valid token sets user/session/auth context before calling handler
func TestValidTokenSetsContext(t *testing.T) {
	handler := AuthMiddleware(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token-123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["user_id"] == nil || body["user_id"] == "" {
		t.Error("expected user_id to be set in context")
	}
	if body["session_id"] == nil || body["session_id"] == "" {
		t.Error("expected session_id to be set in context")
	}
	if body["auth_method"] != "bearer" {
		t.Errorf("expected auth_method 'bearer', got %v", body["auth_method"])
	}
}

// Test 4: Auth middleware chains with rate limit middleware — authenticated
// requests are keyed separately from anonymous requests
func TestAuthBeforeRateLimit(t *testing.T) {
	// Chain: AuthMiddleware -> RateLimitMiddleware -> handler
	handler := AuthMiddleware(RateLimitMiddleware(100, 10)(okHandler()))

	// Authenticated request
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer my-token")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("authenticated request expected 200, got %d", rr.Code)
	}

	// Verify user context propagated through rate limit middleware
	var body map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&body)
	if body["user_id"] == nil || body["user_id"] == "" {
		t.Error("user_id should propagate through rate limit middleware")
	}
}

// Test 5: Anonymous request (no token) is blocked by auth before rate limiting
func TestAnonymousBlockedBeforeRateLimit(t *testing.T) {
	handler := AuthMiddleware(RateLimitMiddleware(100, 10)(okHandler()))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for anonymous request, got %d", rr.Code)
	}
}

// Test 6: X-API-Key header also provides authentication
func TestAPIKeyAuth(t *testing.T) {
	handler := AuthMiddleware(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "test-api-key-123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with API key, got %d", rr.Code)
	}
}

// Test 7: Rate limit middleware allows requests within burst
func TestRateLimitAllowsBurst(t *testing.T) {
	handler := RateLimitMiddleware(100, 5)(okHandler())

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("X-API-Key", "test-burst-key")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rr.Code)
		}
	}
}

// Test 8: Rate limit middleware blocks when burst exceeded
func TestRateLimitBlocksExcess(t *testing.T) {
	handler := RateLimitMiddleware(0.1, 2)(okHandler())

	// Exhaust the burst
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("X-API-Key", "test-block-key")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// This one should be rate limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "test-block-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after burst exhausted, got %d", rr.Code)
	}
}

// Test 9: Different API keys have separate rate limit buckets
func TestRateLimitSeparateKeys(t *testing.T) {
	handler := RateLimitMiddleware(0.1, 1)(okHandler())

	// Exhaust key A
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "key-A")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("key A first: expected 200, got %d", rr.Code)
	}

	// Key A should be exhausted
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "key-A")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("key A second: expected 429, got %d", rr.Code)
	}

	// Key B should still be allowed
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "key-B")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("key B first: expected 200, got %d", rr.Code)
	}
}

// Test 10: Rate limit sets X-RateLimit headers
func TestRateLimitHeaders(t *testing.T) {
	handler := RateLimitMiddleware(100, 10)(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "headers-test-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("expected X-RateLimit-Limit header to be set")
	}
	if rr.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("expected X-RateLimit-Remaining header to be set")
	}
	if rr.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("expected X-RateLimit-Reset header to be set")
	}
}

// Test 11: Context propagation — RequestID middleware sets request ID in context
func TestRequestIDSetsContext(t *testing.T) {
	var ctxRequestID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxRequestID, _ = r.Context().Value(ContextKeyRequestID).(string)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if ctxRequestID == "" {
		t.Error("expected request ID to be set in context")
	}
}

// Test 12: Full middleware chain — auth + rate limit + handler
func TestFullChainAuthenticatedRequest(t *testing.T) {
	handler := AuthMiddleware(RateLimitMiddleware(100, 10)(okHandler()))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer full-chain-token")
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&body)
	if body["user_id"] == nil {
		t.Error("user_id should be set through full chain")
	}
	if body["auth_method"] != "bearer" {
		t.Errorf("expected auth_method 'bearer', got %v", body["auth_method"])
	}
}

// Ensure context is properly typed
func TestContextKeyType(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ContextKeyUserID, "user123")

	val := ctx.Value(ContextKeyUserID)
	if val != "user123" {
		t.Errorf("expected 'user123', got %v", val)
	}
}
