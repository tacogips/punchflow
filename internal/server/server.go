// Package server assembles the punchflow HTTP API and its supporting services.
package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	punchflow "punchflow"
	"punchflow/internal/admin"
	"punchflow/internal/api/ticketui"
	v1 "punchflow/internal/api/v1"
	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/files"
	"punchflow/internal/fixtures"
	"punchflow/internal/httpx"
	"punchflow/internal/kftime"
	"punchflow/internal/notify"
	"punchflow/internal/store"
	"punchflow/internal/uiapi"
	"punchflow/internal/webhook"
	"punchflow/internal/webui"
)

// Config controls the local server and its backing services.
type Config struct {
	Listen, PublicBaseURL, DataDir, DBPath   string
	FixturesFS                               fs.FS
	Seed, EnableLocalLogin, EnableAdminReset bool
	OIDC                                     auth.OIDCConfig
	RateLimit                                int
	RateLimitSecret                          string
	WebhookRetryBase                         time.Duration
	MaxUploadBytes                           int64
}

// Server owns the database, webhooks and assembled HTTP handler.
type Server struct {
	DB       *store.DB
	Webhooks *webhook.Service
	Faults   *httpx.FaultSet

	handler http.Handler
}

// New opens the database, seeds an empty database when requested and assembles
// the API services. A nil clock selects the system clock.
func New(ctx context.Context, cfg Config, clock kftime.Clock) (*Server, error) {
	if clock == nil {
		clock = kftime.SystemClock{}
	}
	if cfg.DataDir == "" {
		cfg.DataDir = ".punchflow-data"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = cfg.DataDir + "/punchflow.sqlite"
	}
	if cfg.FixturesFS == nil {
		var err error
		cfg.FixturesFS, err = fixtures.DefaultFS(punchflow.DefaultFixtures)
		if err != nil {
			return nil, fmt.Errorf("server: locate default fixtures: %w", err)
		}
	}

	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("server: open database: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = db.Close()
		}
	}()

	if cfg.Seed {
		empty, err := db.IsEmpty(ctx)
		if err != nil {
			return nil, fmt.Errorf("server: check database: %w", err)
		}
		if empty {
			if _, err := fixtures.Load(ctx, db, cfg.FixturesFS, clock); err != nil {
				return nil, fmt.Errorf("server: seed database: %w", err)
			}
		}
	}

	if cfg.EnableLocalLogin {
		slog.Warn("development local login is enabled")
	}
	if cfg.EnableAdminReset {
		slog.Warn("development admin fixture reset is enabled")
	}

	filesDir := cfg.DataDir + "/files"
	filesKey, err := store.ServerSecret(ctx, db.Q(), "files")
	if err != nil {
		return nil, fmt.Errorf("server: load file signing key: %w", err)
	}
	fileStore := files.New(filesDir, filesKey, cfg.PublicBaseURL, clock, cfg.MaxUploadBytes)
	faults := httpx.NewFaultSet()
	authn := auth.NewAuthenticator(db, clock)
	oidcConfig := cfg.OIDC
	if oidcConfig.RedirectURL == "" && cfg.PublicBaseURL != "" {
		oidcConfig.RedirectURL = strings.TrimRight(cfg.PublicBaseURL, "/") + "/auth/oidc/callback"
	}
	secureCookie := false
	if parsed, parseErr := url.Parse(cfg.PublicBaseURL); parseErr == nil {
		secureCookie = strings.EqualFold(parsed.Scheme, "https")
	}
	oidc, err := auth.NewOIDC(ctx, oidcConfig, authn, db, clock, secureCookie)
	if err != nil {
		return nil, fmt.Errorf("server: configure OIDC: %w", err)
	}

	engineService := engine.New(clock)
	webhooks := webhook.New(db, webhook.NewPayloads(), clock, nil, webhook.Config{RetryBase: cfg.WebhookRetryBase})
	apiMux := http.NewServeMux()
	// Engine events are fanned out to the local notification inbox inside the
	// same transaction that enqueues the webhook deliveries.
	events := notify.NewSink(webhooks, notify.New(engineService, clock))
	v1.New(v1.Deps{
		DB: db, Engine: engineService, Events: events, Files: fileStore,
		Clock: clock, BaseURL: cfg.PublicBaseURL,
	}).Register(apiMux, nil)
	limiter := httpx.NewRateLimiter(cfg.RateLimit, cfg.RateLimitSecret, clock)
	v1Handler := httpx.Chain(apiMux,
		httpx.RequestID,
		httpx.Recover,
		httpx.CORS,
		faults.Middleware,
		httpx.LengthRequired,
		limiter.Middleware,
		authn.Middleware,
	)

	uiMux := http.NewServeMux()
	uiapi.New(uiapi.Deps{DB: db, Engine: engineService, V1: apiMux, Clock: clock, BaseURL: cfg.PublicBaseURL}).Register(uiMux)
	ticketui.New(ticketui.Deps{DB: db, Engine: engineService, Events: events, Clock: clock}).RegisterRoutes(uiMux)
	uiHandler := httpx.Chain(uiMux,
		httpx.RequestID,
		httpx.Recover,
		authn.Middleware,
	)

	adminMux := http.NewServeMux()
	admin.New(admin.Deps{
		DB: db, Auth: authn, Webhooks: webhooks, Faults: faults,
		Clock: clock, FixturesFS: cfg.FixturesFS, FilesDir: filesDir,
		EnableReset: cfg.EnableAdminReset, BaseURL: cfg.PublicBaseURL,
	}).Register(adminMux)
	adminHandler := httpx.Chain(adminMux,
		httpx.RequestID,
		httpx.Recover,
		httpx.LengthRequired,
		authn.Middleware,
	)

	authMux := http.NewServeMux()
	authn.Register(authMux, auth.RoutesOptions{EnableLocalLogin: cfg.EnableLocalLogin, SecureCookie: secureCookie, OIDC: oidc})
	authHandler := httpx.Chain(authMux, httpx.RequestID, httpx.Recover)

	root := http.NewServeMux()
	root.Handle("/v1/", v1Handler)
	root.Handle("/admin/api/", adminHandler)
	root.Handle("/ui/api/", uiHandler)
	root.Handle("/auth/", authHandler)
	root.Handle("/files/download/", fileStore.DownloadHandler(db))
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	root.Handle("/", webui.Handler(punchflow.WebDist))

	closeOnError = false
	return &Server{DB: db, Webhooks: webhooks, Faults: faults, handler: root}, nil
}

// Handler returns the top-level router, including the reserved API prefixes.
func (s *Server) Handler() http.Handler { return s.handler }

// Serve runs the HTTP server and webhook dispatcher until cancellation. It
// gracefully shuts down HTTP and joins both owned goroutines before returning.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	if ln == nil {
		return fmt.Errorf("server: listener is nil")
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		_ = ln.Close()
		return nil
	}

	httpServer := &http.Server{Handler: s.Handler()}
	httpDone := make(chan error, 1)
	go func() { httpDone <- httpServer.Serve(ln) }()
	webhookDone := make(chan error, 1)
	go func() { webhookDone <- s.Webhooks.Run(runCtx) }()

	shutdown := func() error {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	}
	serveErr := func(err error) error {
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}

	select {
	case err := <-httpDone:
		cancel()
		<-webhookDone
		return serveErr(err)
	case err := <-webhookDone:
		if runCtx.Err() != nil {
			shutdownErr := shutdown()
			serveResult := <-httpDone
			if shutdownErr != nil {
				return fmt.Errorf("server: shut down HTTP server: %w", shutdownErr)
			}
			if err := serveErr(serveResult); err != nil {
				return fmt.Errorf("server: HTTP server: %w", err)
			}
			return err
		}
		cancel()
		shutdownErr := shutdown()
		serveResult := <-httpDone
		if err != nil {
			return fmt.Errorf("server: webhook dispatcher: %w", err)
		}
		if shutdownErr != nil {
			return fmt.Errorf("server: shut down HTTP server: %w", shutdownErr)
		}
		return fmt.Errorf("server: webhook dispatcher stopped unexpectedly (HTTP: %v)", serveErr(serveResult))
	case <-ctx.Done():
		shutdownErr := shutdown()
		cancel()
		serveResult := <-httpDone
		webhookResult := <-webhookDone
		if shutdownErr != nil {
			return fmt.Errorf("server: shut down HTTP server: %w", shutdownErr)
		}
		if err := serveErr(serveResult); err != nil {
			return fmt.Errorf("server: HTTP server: %w", err)
		}
		if webhookResult != nil {
			return fmt.Errorf("server: webhook dispatcher: %w", webhookResult)
		}
		return nil
	}
}

// Close releases the server's database connection.
func (s *Server) Close() error { return s.DB.Close() }
