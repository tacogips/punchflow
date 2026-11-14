package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"punchflow/internal/kftime"
)

func TestRateLimiterWindowsAndSecretMultiplier(t *testing.T) {
	clock := kftime.NewFakeClock(time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
	limiter := NewRateLimiter(2, "shared", clock)
	h := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	call := func(secret string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.0.2.4:1234"
		if secret != "" {
			r.Header.Set("X-Rate-Limit-Secret", secret)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr
	}
	for i := 0; i < 2; i++ {
		if rr := call(""); rr.Code != 204 {
			t.Fatalf("request %d: %d", i+1, rr.Code)
		}
	}
	rr := call("")
	if rr.Code != 429 || !strings.Contains(rr.Body.String(), "rate_limited") || rr.Header().Get("RateLimit-Remaining") != "0" || rr.Header().Get("RateLimit-Limit") != "2" {
		t.Fatalf("limit response: %d %v %s", rr.Code, rr.Header(), rr.Body.String())
	}
	clock.Advance(61 * time.Second)
	if rr = call(""); rr.Code != 204 {
		t.Fatalf("window did not reset: %d", rr.Code)
	}
	for i := 0; i < 20; i++ {
		rr = call("shared")
		if rr.Code != 204 {
			t.Fatalf("secret request %d: %d", i+1, rr.Code)
		}
	}
	rr = call("shared")
	if rr.Code != 429 || rr.Header().Get("RateLimit-Limit") != "20" {
		t.Fatalf("secret limit: %d %v", rr.Code, rr.Header())
	}
}

func TestRateLimiterDisabledOmitsHeaders(t *testing.T) {
	h := NewRateLimiter(0, "", nil).Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != 204 || rr.Header().Get("RateLimit-Limit") != "" {
		t.Fatalf("disabled headers: %v", rr.Header())
	}
}

func TestRateLimiterZeroTimeStartsOneWindow(t *testing.T) {
	clock := kftime.NewFakeClock(time.Time{})
	h := NewRateLimiter(1, "", clock).Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	call := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.0.2.9:1"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr
	}
	if rr := call(); rr.Code != 204 {
		t.Fatalf("first request: %d", rr.Code)
	}
	if rr := call(); rr.Code != 429 {
		t.Fatalf("second request should be limited: %d", rr.Code)
	}
}
