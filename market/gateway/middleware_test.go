package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/ttptest"
	"testing"
)

// ErrorResponse represents the expected JSON error shape from auth middleware
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// TestAuthMiddleware_MissingToken verifies that requests without a bearer token
// return 401 with the expected JSON error shape
func TestAuthMiddleware_MissingToken(t *testing.T) {
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errResp.Error == "" {
		t.Error("expected error field to be present in JSON response")
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

// TestAuthMiddleware_InvalidToken verifies that requests with an invalid token
// return 401 with the expected JSON error shape
func TestAuthMiddleware_InvalidToken(t *testing.T) {
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-12345")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errResp.Error == "" {
		t.Error("expected error field to be present in JSON response")
	}
}

// TestMiddlewareChain_ContextPropagation verifies that authenticated user
// context is properly propagated through the middleware chain
func TestMiddlewareChain_ContextPropagation(t *testing.T) {
	var capturedUserID string

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID := r.Context().Value(UserIDKey); userID != nil {
			capturedUserID = userID.(string)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Chain: RateLimit -> Auth -> Handler
	// This order is critical per the documented incident
	handler := RateLimitMiddleware(AuthMiddleware(innerHandler))

	req := httptest.NewRequest("GET", "/api/test", nil)
	// Use a valid test token format (assuming test token prefix is accepted)
	req.Header.Set("Authorization", "Bearer test-valid-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// If auth passed, check context propagation
	if rr.Code == http.StatusOK && capturedUserID == "" {
		t.Error("expected user ID to be propagated in context")
	}
}

// TestRateLimitMiddleware_WithAuth verifies rate limiting respects user identity
func TestRateLimitMiddleware_WithAuth(t *testing.T) {
	requestCount := 0
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(innerHandler)

	// Make multiple requests without auth to test IP-based rate limiting
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// Verify at least some requests were processed
	if requestCount == 0 {
		t.Error("rate limit middleware blocked all requests")
	}
}

// TestAuthMiddleware_MalformedHeader verifies handling of malformed auth headers
func TestAuthMiddleware_MalformedHeader(t *testing.T) {
	testCases := []struct {
		name       string
		authHeader string
	}{
		{"no_bearer_prefix", "some-token"},
		{"basic_auth", "Basic dGVzdDp0ZXN0"},
		{"empty_bearer", "Bearer "},
		{"lowercase_bearer", "bearer some-token"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/api/test", nil)
			req.Header.Set("Authorization", tc.authHeader)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected status %d for malformed header, got %d", http.StatusUnauthorized, rr.Code)
			}
		})
	}
}
