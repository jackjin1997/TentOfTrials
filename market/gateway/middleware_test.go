package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAuthMiddlewareRejectsMissingBearerToken(t *testing.T) {
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
		t.Fatal("wrapped handler was called for missing token")
	}
	assertJSONFields(t, rec, map[string]string{
		"error":   "unauthorized",
		"message": "Missing authentication token",
	})
}

func TestAuthMiddlewareRejectsInvalidTokenWithoutCallingHandler(t *testing.T) {
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
		t.Fatal("wrapped handler was called for invalid token")
	}
	assertJSONFields(t, rec, map[string]string{
		"error": "invalid_token",
	})
}

func TestAuthMiddlewareAddsIdentityContextBeforeRateLimiting(t *testing.T) {
	var seenUserID, seenSessionID, seenAuthMethod string
	handler := AuthMiddleware(RateLimitMiddleware(1, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUserID, _ = r.Context().Value(ContextKeyUserID).(string)
		seenSessionID, _ = r.Context().Value(ContextKeySessionID).(string)
		seenAuthMethod, _ = r.Context().Value(ContextKeyAuthMethod).(string)
		w.WriteHeader(http.StatusNoContent)
	})))

	req := authenticatedRequest("valid-token-user-one", "203.0.113.10:4242")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if seenUserID != "user_valid_token_user_one" {
		t.Fatalf("user id = %q, want token-derived identity", seenUserID)
	}
	if seenSessionID != "session_valid_token_user_one" {
		t.Fatalf("session id = %q, want token-derived session", seenSessionID)
	}
	if seenAuthMethod != "bearer" {
		t.Fatalf("auth method = %q, want bearer", seenAuthMethod)
	}
}

func TestRateLimitMiddlewareUsesAuthenticatedIdentityBeforeIPAddress(t *testing.T) {
	var called int32
	handler := AuthMiddleware(RateLimitMiddleware(1, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusNoContent)
	})))

	for _, token := range []string{"valid-token-user-one", "valid-token-user-two"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, authenticatedRequest(token, "203.0.113.10:4242"))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("token %q status = %d, want %d; body=%s", token, rec.Code, http.StatusNoContent, rec.Body.String())
		}
	}
	if atomic.LoadInt32(&called) != 2 {
		t.Fatalf("wrapped handler calls = %d, want 2", called)
	}
}

func authenticatedRequest(token, remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.RemoteAddr = remoteAddr
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func assertJSONFields(t *testing.T, rec *httptest.ResponseRecorder, want map[string]string) {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not JSON: %v; body=%s", err, rec.Body.String())
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("JSON field %q = %q, want %q; body=%s", key, got[key], value, rec.Body.String())
		}
	}
}
