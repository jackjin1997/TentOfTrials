// middleware_test.go

package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestAuthMiddleware(t *testing.T) {
	// Test case for missing bearer token
	t.Run("MissingBearerToken_Returns401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/some-endpoint", nil)
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		AuthMiddleware(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"Unauthorized"}`, rr.Body.String())
	})

	// Test case for invalid token
	t.Run("InvalidToken_Returns401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/some-endpoint", nil)
		req.Header.Set("Authorization", "Bearer invalid_token")
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		AuthMiddleware(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"Unauthorized"}`, rr.Body.String())
	})

	// Test case for valid token
	t.Run("ValidToken_SetsUserContext", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/some-endpoint", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := r.Context().Value("user")
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			}
			w.WriteHeader(http.StatusOK)
		})
		AuthMiddleware(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	// Test case for middleware ordering
	t.Run("MiddlewareOrdering_AuthenticatedVsAnonymous", func(t *testing.T) {
		// Simulate an authenticated request
		reqAuth := httptest.NewRequest(http.MethodGet, "/some-endpoint", nil)
		reqAuth.Header.Set("Authorization", "Bearer valid_token")
		rrAuth := httptest.NewRecorder()
		handlerAuth := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		AuthMiddleware(handlerAuth).ServeHTTP(rrAuth, reqAuth)
		assert.Equal(t, http.StatusOK, rrAuth.Code)

		// Simulate an anonymous request
		reqAnon := httptest.NewRequest(http.MethodGet, "/some-endpoint", nil)
		rrAnon := httptest.NewRecorder()
		handlerAnon := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		AuthMiddleware(handlerAnon).ServeHTTP(rrAnon, reqAnon)
		assert.Equal(t, http.StatusUnauthorized, rrAnon.Code)
	})
}
