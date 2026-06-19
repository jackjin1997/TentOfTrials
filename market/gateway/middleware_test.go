package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setTokenValidatorForTest(t *testing.T, validator func(string) (string, string, error)) {
	t.Helper()
	previous := tokenValidator
	tokenValidator = validator
	t.Cleanup(func() {
		tokenValidator = previous
	})
}

func newMiddlewareRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.RemoteAddr = "203.0.113.10:4567"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func decodeJSONBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]string {
	t.Helper()

	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not JSON: %v\nbody: %s", err, recorder.Body.String())
	}
	return body
}

func TestAuthMiddlewareMissingBearerTokenReturnsUnauthorizedJSON(t *testing.T) {
	called := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, newMiddlewareRequest(""))

	if called {
		t.Fatal("wrapped handler was called without a bearer token")
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	body := decodeJSONBody(t, recorder)
	if body["error"] != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", body["error"])
	}
	if body["message"] != "Missing authentication token" {
		t.Fatalf("message = %q, want missing-token message", body["message"])
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
}

func TestAuthMiddlewareInvalidTokenReturnsUnauthorizedWithoutCallingHandler(t *testing.T) {
	setTokenValidatorForTest(t, func(token string) (string, string, error) {
		if token != "bad-token" {
			t.Fatalf("validator saw token %q, want bad-token", token)
		}
		return "", "", errors.New("token rejected")
	})

	called := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, newMiddlewareRequest("bad-token"))

	if called {
		t.Fatal("wrapped handler was called for an invalid token")
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	body := decodeJSONBody(t, recorder)
	if body["error"] != "invalid_token" {
		t.Fatalf("error = %q, want invalid_token", body["error"])
	}
	if body["message"] != "token rejected" {
		t.Fatalf("message = %q, want token rejected", body["message"])
	}
}

func TestAuthMiddlewareSetsContextBeforeRateLimitAndHandler(t *testing.T) {
	setTokenValidatorForTest(t, func(token string) (string, string, error) {
		if token != "good-token" {
			t.Fatalf("validator saw token %q, want good-token", token)
		}
		return "user-123", "session-456", nil
	})

	handler := AuthMiddleware(RateLimitMiddleware(10, 2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Context().Value(ContextKeyUserID); got != "user-123" {
			t.Fatalf("ContextKeyUserID = %v, want user-123", got)
		}
		if got := r.Context().Value(ContextKeySessionID); got != "session-456" {
			t.Fatalf("ContextKeySessionID = %v, want session-456", got)
		}
		if got := r.Context().Value(ContextKeyAuthMethod); got != "bearer" {
			t.Fatalf("ContextKeyAuthMethod = %v, want bearer", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, newMiddlewareRequest("good-token"))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("X-RateLimit-Limit"); got != "2" {
		t.Fatalf("X-RateLimit-Limit = %q, want 2", got)
	}
}

func TestAuthenticatedRequestsAreRateLimitedSeparatelyFromAnonymousIP(t *testing.T) {
	setTokenValidatorForTest(t, func(token string) (string, string, error) {
		if token != "good-token" {
			t.Fatalf("validator saw token %q, want good-token", token)
		}
		return "user-123", "session-456", nil
	})

	limiter := RateLimitMiddleware(0.001, 1)
	anonymousHandler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	authenticatedHandler := AuthMiddleware(limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	anonymous := httptest.NewRecorder()
	anonymousHandler.ServeHTTP(anonymous, newMiddlewareRequest(""))
	if anonymous.Code != http.StatusNoContent {
		t.Fatalf("anonymous status = %d, want %d", anonymous.Code, http.StatusNoContent)
	}

	authenticated := httptest.NewRecorder()
	authenticatedHandler.ServeHTTP(authenticated, newMiddlewareRequest("good-token"))
	if authenticated.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d, want %d; body: %s", authenticated.Code, http.StatusNoContent, authenticated.Body.String())
	}

	repeatedAuthenticated := httptest.NewRecorder()
	authenticatedHandler.ServeHTTP(repeatedAuthenticated, newMiddlewareRequest("good-token"))
	if repeatedAuthenticated.Code != http.StatusTooManyRequests {
		t.Fatalf("second authenticated status = %d, want %d", repeatedAuthenticated.Code, http.StatusTooManyRequests)
	}
}
