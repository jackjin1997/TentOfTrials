package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthMiddlewareMissingBearerTokenReturnsJSON401(t *testing.T) {
	called := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := performGatewayRequest(handler, "")

	if called {
		t.Fatal("wrapped handler was called without a bearer token")
	}
	assertStatus(t, rec, http.StatusUnauthorized)
	assertJSONContentType(t, rec)
	body := decodeGatewayJSON(t, rec)
	assertJSONField(t, body, "error", "unauthorized")
	assertJSONField(t, body, "message", "Missing authentication token")
}

func TestAuthMiddlewareInvalidTokenReturnsJSON401WithoutCallingHandler(t *testing.T) {
	called := false
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := performGatewayRequest(handler, "invalid-token")

	if called {
		t.Fatal("wrapped handler was called with an invalid token")
	}
	assertStatus(t, rec, http.StatusUnauthorized)
	assertJSONContentType(t, rec)
	body := decodeGatewayJSON(t, rec)
	assertJSONField(t, body, "error", "invalid_token")
	assertJSONField(t, body, "message", "invalid authentication token")
}

func TestAuthBeforeRateLimitPopulatesIdentityContext(t *testing.T) {
	handler := AuthMiddleware(RateLimitMiddleware(1000, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertContextString(t, r, ContextKeyUserID, "user_alpha")
		assertContextString(t, r, ContextKeySessionID, "session_alpha")
		assertContextString(t, r, ContextKeyAuthMethod, "bearer")
		w.WriteHeader(http.StatusNoContent)
	})))

	rec := performGatewayRequest(handler, "alpha")

	assertStatus(t, rec, http.StatusNoContent)
}

func TestRateLimitUsesAuthenticatedIdentityBeforeIP(t *testing.T) {
	limiter := RateLimitMiddleware(0.0001, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	anonymous := limiter(handler)
	authenticated := AuthMiddleware(limiter(handler))

	firstAnonymous := performGatewayRequest(anonymous, "")
	assertStatus(t, firstAnonymous, http.StatusNoContent)

	firstAuthenticated := performGatewayRequest(authenticated, "alpha")
	assertStatus(t, firstAuthenticated, http.StatusNoContent)

	secondAuthenticated := performGatewayRequest(authenticated, "alpha")
	assertStatus(t, secondAuthenticated, http.StatusTooManyRequests)
}

func performGatewayRequest(handler http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/orders", nil)
	req.RemoteAddr = "198.51.100.9:4321"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeGatewayJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	return body
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body.String())
	}
}

func assertJSONContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func assertJSONField(t *testing.T, body map[string]interface{}, key, want string) {
	t.Helper()
	if got, ok := body[key].(string); !ok || got != want {
		t.Fatalf("body[%q] = %v, want %q", key, body[key], want)
	}
}

func assertContextString(t *testing.T, r *http.Request, key contextKey, want string) {
	t.Helper()
	if got, ok := r.Context().Value(key).(string); !ok || got != want {
		t.Fatalf("context[%q] = %v, want %q", key, r.Context().Value(key), want)
	}
}
