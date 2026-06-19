package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareMissingBearerToken(t *testing.T) {
	called := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("wrapped handler was called without an auth token")
	}
	assertJSONError(t, rec, http.StatusUnauthorized, "unauthorized", "Missing authentication token")
}

func TestAuthMiddlewareInvalidTokenStopsBeforeHandler(t *testing.T) {
	restore := stubTokenValidator(func(token string) (string, string, error) {
		if token != "bad-token" {
			t.Fatalf("validator saw token %q, want bad-token", token)
		}
		return "", "", errors.New("token revoked")
	})
	defer restore()

	called := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("wrapped handler was called for an invalid token")
	}
	assertJSONError(t, rec, http.StatusUnauthorized, "invalid_token", "token revoked")
}

func TestAuthMiddlewareSetsContextBeforeRateLimit(t *testing.T) {
	restore := stubTokenValidator(func(token string) (string, string, error) {
		return "user-123", "session-456", nil
	})
	defer restore()

	var gotUserID, gotSessionID, gotAuthMethod string
	handler := AuthMiddleware(RateLimitMiddleware(1, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(ContextKeyUserID).(string)
		gotSessionID, _ = r.Context().Value(ContextKeySessionID).(string)
		gotAuthMethod, _ = r.Context().Value(ContextKeyAuthMethod).(string)
		w.WriteHeader(http.StatusNoContent)
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.RemoteAddr = "192.0.2.10:5000"
	req.Header.Set("Authorization", "Bearer good-token")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if gotUserID != "user-123" || gotSessionID != "session-456" || gotAuthMethod != "bearer" {
		t.Fatalf("context = user %q session %q auth %q", gotUserID, gotSessionID, gotAuthMethod)
	}
}

func TestRateLimitMiddlewareSeparatesAuthenticatedUsersFromAnonymousIPBucket(t *testing.T) {
	restore := stubTokenValidator(func(token string) (string, string, error) {
		return "user-123", "session-456", nil
	})
	defer restore()

	limiter := RateLimitMiddleware(1, 1)
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	anonymous := limiter(okHandler)
	authenticated := AuthMiddleware(limiter(okHandler))

	var exhausted bool
	for i := 0; i < 5; i++ {
		anonReq := httptest.NewRequest(http.MethodGet, "/orders", nil)
		anonReq.RemoteAddr = "192.0.2.10:5000"
		anonRec := httptest.NewRecorder()
		anonymous.ServeHTTP(anonRec, anonReq)
		if anonRec.Code == http.StatusTooManyRequests {
			exhausted = true
			break
		}
	}
	if !exhausted {
		t.Fatal("anonymous IP bucket was not exhausted before authenticated request")
	}

	authReq := httptest.NewRequest(http.MethodGet, "/orders", nil)
	authReq.RemoteAddr = "192.0.2.10:5000"
	authReq.Header.Set("Authorization", "Bearer good-token")
	authRec := httptest.NewRecorder()
	authenticated.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d, want %d; body: %s", authRec.Code, http.StatusNoContent, authRec.Body.String())
	}
}

func TestRateLimitMiddlewareIgnoresUnauthenticatedAPIKeyHeader(t *testing.T) {
	limiter := RateLimitMiddleware(1, 1)
	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	firstReq := httptest.NewRequest(http.MethodGet, "/orders", nil)
	firstReq.RemoteAddr = "192.0.2.10:5000"
	firstReq.Header.Set("X-API-Key", "rotated-key-1")
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want %d", firstRec.Code, http.StatusNoContent)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/orders", nil)
	secondReq.RemoteAddr = "192.0.2.10:5000"
	secondReq.Header.Set("X-API-Key", "rotated-key-2")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", secondRec.Code, http.StatusTooManyRequests)
	}
}

func stubTokenValidator(fn func(string) (string, string, error)) func() {
	original := validateAuthToken
	validateAuthToken = fn
	return func() {
		validateAuthToken = original
	}
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string, message string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, status, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v; body: %s", err, rec.Body.String())
	}
	if body["error"] != code {
		t.Fatalf("error = %q, want %q", body["error"], code)
	}
	if body["message"] != message {
		t.Fatalf("message = %q, want %q", body["message"], message)
	}
}
