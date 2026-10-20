package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"punchflow"
	"punchflow/internal/fixtures"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/server"
	"punchflow/internal/store"
)

// Env is an isolated, seeded HTTP server for API handler tests.
type Env struct {
	URL       string
	Server    *server.Server
	DB        *store.DB
	Clock     *kftime.FakeClock
	Receiver  *Receiver
	CallerFor string
}

// Option configures a test server.
type Option func(*options)

type options struct {
	enableLocalLogin bool
	enableAdminReset bool
	rateLimit        int
	rateLimitSecret  string
}

// WithLocalLogin enables the explicit development-only local login route.
func WithLocalLogin() Option { return func(o *options) { o.enableLocalLogin = true } }

// WithAdminReset enables the explicit development-only admin fixture reset.
func WithAdminReset() Option { return func(o *options) { o.enableAdminReset = true } }

// WithRateLimit enables the server's request limiter with the supplied limit and secret.
func WithRateLimit(n int, secret string) Option {
	return func(o *options) { o.rateLimit, o.rateLimitSecret = n, secret }
}

// New starts a temporary seeded server. The webhook dispatcher is deliberately
// not started; tests drive deliveries with DeliverWebhooks.
func New(t *testing.T, opts ...Option) *Env {
	t.Helper()
	config := options{}
	for _, option := range opts {
		if option != nil {
			option(&config)
		}
	}
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "files"), 0700); err != nil {
		t.Fatalf("create test files directory: %v", err)
	}
	fixtureFS, err := fixtures.DefaultFS(punchflow.DefaultFixtures)
	if err != nil {
		t.Fatalf("open embedded default fixtures: %v", err)
	}
	clock := kftime.NewFakeClock(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	srv, err := server.New(context.Background(), server.Config{
		DataDir: dataDir, DBPath: filepath.Join(dataDir, "punchflow.sqlite"),
		FixturesFS: fixtureFS, Seed: true,
		EnableLocalLogin: config.enableLocalLogin, EnableAdminReset: config.enableAdminReset,
		RateLimit: config.rateLimit, RateLimitSecret: config.rateLimitSecret,
	}, clock)
	if err != nil {
		t.Fatalf("create test server: %v", err)
	}
	httpServer := httptest.NewServer(srv.Handler())
	receiver := newReceiver()
	env := &Env{URL: httpServer.URL, Server: srv, DB: srv.DB, Clock: clock, Receiver: receiver, CallerFor: "admin"}
	t.Cleanup(func() {
		httpServer.Close()
		receiver.server.Close()
		if err := srv.Close(); err != nil {
			t.Errorf("close test server: %v", err)
		}
	})
	return env
}

// Do sends a request to the test server. A nonempty persona adds the pinned
// fixture token; the Integration SAT uses CallerFor for X-Caller-Id.
func (e *Env) Do(t *testing.T, persona Persona, method, requestPath string, body any) *Response {
	t.Helper()
	var requestBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request JSON: %v", err)
		}
		requestBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, e.URL+requestPath, requestBody)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if persona != "" {
		token := persona.token()
		if token == "" {
			t.Fatalf("unknown persona %q", persona)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if persona == Integration {
			callerID := e.UserID(t, e.CallerFor)
			req.Header.Set("X-Caller-Id", callerID)
		}
	}
	return e.DoRaw(t, req)
}

// DoRaw sends req as-is, resolving relative request URLs against the test server.
func (e *Env) DoRaw(t *testing.T, req *http.Request) *Response {
	t.Helper()
	request := req.Clone(req.Context())
	request.Header = req.Header.Clone()
	request.RequestURI = ""
	if !request.URL.IsAbs() {
		base, err := url.Parse(e.URL)
		if err != nil {
			t.Fatalf("parse test server URL: %v", err)
		}
		request.URL = base.ResolveReference(request.URL)
	}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send test request: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read test response body: %v", err)
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header.Clone(), Body: data}
}

// DeliverWebhooks runs one dispatcher cycle synchronously and returns the
// number of deliveries attempted.
func (e *Env) DeliverWebhooks(t *testing.T) int {
	t.Helper()
	attempts, err := e.Server.Webhooks.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("deliver webhooks: %v", err)
	}
	return attempts
}

// RegisterReceiverWebhook creates an enabled webhook registration pointing at
// the local receiver and returns its ID.
func (e *Env) RegisterReceiverWebhook(t *testing.T, secret string, eventTypes ...string) string {
	t.Helper()
	if len(eventTypes) == 0 {
		eventTypes = append([]string(nil), model.WebhookEventTypes...)
	}
	now := e.Clock.Now()
	id := store.NewID()
	if err := store.InsertWebhook(t.Context(), e.DB.Q(), model.Webhook{
		ID: id, Name: "test receiver", URL: e.Receiver.server.URL, Secret: &secret,
		Enabled: true, EventTypes: eventTypes, WorkflowIDs: []string{}, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("register receiver webhook: %v", err)
	}
	return id
}

// UserID resolves a fixture user code to its stable ID.
func (e *Env) UserID(t *testing.T, code string) string {
	t.Helper()
	if code == "" {
		code = "admin"
	}
	id, err := store.UserIDByCode(t.Context(), e.DB.Q(), code)
	if err != nil {
		t.Fatalf("resolve user code %q: %v", code, err)
	}
	if id == "" {
		t.Fatalf("user code %q resolved to empty ID", code)
	}
	return id
}
