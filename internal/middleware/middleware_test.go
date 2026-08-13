package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddleware(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handlerToTest := CORSMiddleware(nextHandler)

	req := httptest.NewRequest("OPTIONS", "/api/sessions", nil)
	rec := httptest.NewRecorder()

	handlerToTest.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Expected status 204 for OPTIONS preflight, got %d", rec.Code)
	}

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin header to be *")
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(1.0, 2.0)
	ip := "192.168.1.1"

	if !limiter.allow(ip) {
		t.Errorf("Expected 1st request to be allowed")
	}
	if !limiter.allow(ip) {
		t.Errorf("Expected 2nd request within burst to be allowed")
	}
	if limiter.allow(ip) {
		t.Errorf("Expected 3rd request exceeding burst capacity to be blocked")
	}
}
