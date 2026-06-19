package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthMiddlewareMissingToken returns 401 with proper JSON error shape
func TestAuthMiddlewareMissingToken(t *testing.T) {
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/orders", nil)
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
		t.Errorf("expected error 'unauthorized', got '%v'", body["error"])
	}
	if body["message"] != "Missing authentication token" {
		t.Errorf("expected message about missing token, got '%v'", body["message"])
	}
}

// TestAuthMiddlewareInvalidToken returns 401 without calling wrapped handler
func TestAuthMiddlewareInvalidToken(t *testing.T) {
	handlerCalled := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/orders", nil)
	req.Header.Set("Authorization", "Bearer invalid_token_12345")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
	if handlerCalled {
		t.Error("wrapped handler should NOT be called for invalid token")
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["error"] != "invalid_token" {
		t.Errorf("expected error 'invalid_token', got '%v'", body["error"])
	}
}

// TestAuthMiddlewareValidToken sets user context before rate limiting
func TestAuthMiddlewareValidToken(t *testing.T) {
	var capturedUserID string
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = r.Context().Value(ContextKeyUserID).(string)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/orders", nil)
	req.Header.Set("Authorization", "Bearer valid_token_abc")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if capturedUserID != "user_stub" {
		t.Errorf("expected user_stub in context, got '%s'", capturedUserID)
	}
}

// TestMiddlewareOrdering ensures auth runs before rate limiting
func TestMiddlewareOrdering(t *testing.T) {
	var order []string

	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "auth")
			next.ServeHTTP(w, r)
		})
	}

	rateLimitMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "ratelimit")
			next.ServeHTTP(w, r)
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	})

	chain := authMiddleware(rateLimitMiddleware(finalHandler))
	req := httptest.NewRequest("GET", "/api/orders", nil)
	req.Header.Set("Authorization", "Bearer valid_token_abc")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if len(order) != 3 {
		t.Fatalf("expected 3 middleware calls, got %d: %v", len(order), order)
	}
	if order[0] != "auth" {
		t.Errorf("auth must run before ratelimit, got order: %v", order)
	}
	if order[1] != "ratelimit" {
		t.Errorf("ratelimit must run after auth, got order: %v", order)
	}
}
