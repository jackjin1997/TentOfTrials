package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware_MissingToken(t *testing.T) {
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["error"] != "unauthorized" || resp["message"] != "Missing authentication token" {
		t.Errorf("unexpected error format: %v", resp)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	handlerCalled := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer invalid_token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	if handlerCalled {
		t.Errorf("expected inner handler not to be called for invalid token")
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["error"] != "invalid_token" {
		t.Errorf("unexpected error format: %v", resp)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	handlerCalled := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		userID := r.Context().Value(ContextKeyUserID).(string)
		sessionID := r.Context().Value(ContextKeySessionID).(string)
		authMethod := r.Context().Value(ContextKeyAuthMethod).(string)

		if userID != "user_stub" {
			t.Errorf("expected user_stub, got %s", userID)
		}
		if sessionID != "session_stub" {
			t.Errorf("expected session_stub, got %s", sessionID)
		}
		if authMethod != "bearer" {
			t.Errorf("expected bearer, got %s", authMethod)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer valid_token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if !handlerCalled {
		t.Errorf("expected inner handler to be called")
	}
}

func TestMiddleware_Ordering_And_Keying(t *testing.T) {
	// Chain AuthMiddleware then RateLimitMiddleware (with 1 req/sec limit, burst 1)
	rateLimiter := RateLimitMiddleware(1.0, 1)
	
	// Inner handler counts how many times it was called
	handlerCallCount := 0
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCallCount++
		w.WriteHeader(http.StatusOK)
	})

	chainedHandler := AuthMiddleware(rateLimiter(innerHandler))

	// 1. Send first request with User A -> should pass
	reqA1 := httptest.NewRequest("GET", "/api/v1/test", nil)
	reqA1.Header.Set("Authorization", "Bearer user_a")
	reqA1.RemoteAddr = "1.2.3.4:1234"
	rrA1 := httptest.NewRecorder()
	chainedHandler.ServeHTTP(rrA1, reqA1)
	if rrA1.Code != http.StatusOK {
		t.Fatalf("first request for User A failed: %d", rrA1.Code)
	}

	// 2. Send second request with User A from same IP -> should be rate limited (429)
	reqA2 := httptest.NewRequest("GET", "/api/v1/test", nil)
	reqA2.Header.Set("Authorization", "Bearer user_a")
	reqA2.RemoteAddr = "1.2.3.4:1234"
	rrA2 := httptest.NewRecorder()
	chainedHandler.ServeHTTP(rrA2, reqA2)
	if rrA2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for second User A request, got %d", rrA2.Code)
	}

	// 3. Send request with User B from same IP -> should pass, because keyed separately by userID!
	reqB1 := httptest.NewRequest("GET", "/api/v1/test", nil)
	reqB1.Header.Set("Authorization", "Bearer user_b")
	reqB1.RemoteAddr = "1.2.3.4:1234"
	rrB1 := httptest.NewRecorder()
	chainedHandler.ServeHTTP(rrB1, reqB1)
	if rrB1.Code != http.StatusOK {
		t.Fatalf("request for User B failed: %d", rrB1.Code)
	}
}
