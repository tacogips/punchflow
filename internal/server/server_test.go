package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServerRoutesAndReservedPrefixes(t *testing.T) {
	s, err := New(context.Background(), Config{DBPath: t.TempDir() + "/punchflow.sqlite", Seed: true, EnableAdminReset: true}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	request := func(method, path, token, body string, headers http.Header) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		for name, values := range headers {
			for _, value := range values {
				req.Header.Add(name, value)
			}
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		return rr
	}

	if got := request(http.MethodGet, "/healthz", "", "", nil); got.Code != http.StatusOK || got.Body.String() != `{"status":"ok"}` {
		t.Fatalf("GET /healthz = %d %q, want 200 status JSON", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/v1/user", "", "", nil); got.Code != http.StatusUnauthorized || !strings.Contains(got.Body.String(), `"code":"invalid_access_token"`) {
		t.Fatalf("unauthenticated GET /v1/user = %d %q, want 401 invalid_access_token", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/v1/user", "pf_pat_dev_admin", "", nil); got.Code == http.StatusUnauthorized || got.Code == http.StatusNotFound {
		t.Fatalf("authenticated GET /v1/user = %d, want routed response", got.Code)
	}
	if got := request(http.MethodGet, "/v1/nope", "pf_pat_dev_admin", "", nil); got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), `"code":"endpoint_not_found"`) {
		t.Fatalf("unknown GET /v1/nope = %d %q, want JSON endpoint_not_found", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/admin/api/nope", "pf_pat_dev_admin", "", nil); got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), `"code":"endpoint_not_found"`) {
		t.Fatalf("unknown GET /admin/api/nope = %d %q, want JSON endpoint_not_found", got.Code, got.Body.String())
	}
	preflight := make(http.Header)
	preflight.Set("Origin", "http://localhost:8080")
	preflight.Set("Access-Control-Request-Method", "GET")
	if got := request(http.MethodOptions, "/v1/users", "", "", preflight); got.Code != http.StatusOK {
		t.Fatalf("OPTIONS /v1/users = %d, want 200", got.Code)
	}
	spa := request(http.MethodGet, "/tickets/123", "", "", nil)
	if spa.Code != http.StatusOK || strings.HasPrefix(spa.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("GET /tickets/123 = %d %q, want SPA response", spa.Code, spa.Header().Get("Content-Type"))
	}
	reset := request(http.MethodPost, "/admin/api/fixtures/reset", "pf_pat_dev_admin", "{}", nil)
	if reset.Code == http.StatusUnauthorized || reset.Code == http.StatusNotFound {
		t.Fatalf("enabled admin reset route = %d, want authenticated registered route", reset.Code)
	}
}

func TestServeStopsOwnedLoopsOnCancellation(t *testing.T) {
	s, err := New(context.Background(), Config{DBPath: t.TempDir() + "/punchflow.sqlite", Seed: true}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, requestErr := client.Get("http://" + ln.Addr().String() + "/healthz")
		if requestErr == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("Serve() never accepted a health request: %v", requestErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not stop after cancellation")
	}
}

func TestServeClosesListenerWhenContextAlreadyCanceled(t *testing.T) {
	s, err := New(context.Background(), Config{DBPath: t.TempDir() + "/punchflow.sqlite", Seed: true}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Serve(ctx, ln); err != nil {
		t.Fatalf("Serve() error = %v, want nil", err)
	}
	if _, err := ln.Accept(); err == nil {
		t.Fatal("Serve() left the listener open after an already-canceled context")
	}
}
