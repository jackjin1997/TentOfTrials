package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAuthMiddlewareMissingBearerToken(t *testing.T) {
	var called int32
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatal("handler invoked without token")
	}
	expectJSONFields(t, rec, map[string]string{
		"error":   "unauthorized",
		"message": "Missing authentication token",
	})
}

func TestAuthMiddlewareInvalidTokenSkipsHandler(t *testing.T) {
	var called int32
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatal("handler invoked for invalid token")
	}
	expectJSONFields(t, rec, map[string]string{"error": "invalid_token"})
}

func TestAuthMiddlewareValidTokenSetsContext(t *testing.T) {
	var userID, sessionID, authMethod string
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ = r.Context().Value(ContextKeyUserID).(string)
		sessionID, _ = r.Context().Value(ContextKeySessionID).(string)
		authMethod, _ = r.Context().Value(ContextKeyAuthMethod).(string)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := bearerRequest("alpha-token-1", "198.51.100.4:5150")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if userID != "user_alpha_token_1" {
		t.Fatalf("user id = %q", userID)
	}
	if sessionID != "session_alpha_token_1" {
		t.Fatalf("session id = %q", sessionID)
	}
	if authMethod != "bearer" {
		t.Fatalf("auth method = %q", authMethod)
	}
}

func TestAuthBeforeRateLimitSharesPerUserBuckets(t *testing.T) {
	var called int32
	handler := AuthMiddleware(RateLimitMiddleware(1, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusNoContent)
	})))

	for _, token := range []string{"alpha-token-1", "beta-token-2"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, bearerRequest(token, "198.51.100.4:5150"))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("token %q status = %d body=%s", token, rec.Code, rec.Body.String())
		}
	}
	if atomic.LoadInt32(&called) != 2 {
		t.Fatalf("handler calls = %d, want 2 distinct authenticated users", called)
	}
}

func TestAnonymousRequestsRateLimitedByIP(t *testing.T) {
	handler := RateLimitMiddleware(1, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "203.0.113.44:8080"
	handler.ServeHTTP(first, req)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d", first.Code)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", second.Code)
	}
	expectJSONFields(t, second, map[string]string{"error": "rate_limit_exceeded"})
}

func bearerRequest(token, remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.RemoteAddr = remoteAddr
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func expectJSONFields(t *testing.T, rec *httptest.ResponseRecorder, want map[string]string) {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not JSON: %v; body=%s", err, rec.Body.String())
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("field %q = %q, want %q; body=%s", key, got[key], value, rec.Body.String())
		}
	}
}
