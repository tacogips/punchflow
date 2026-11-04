package cli

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	punchflow "punchflow"
	"punchflow/internal/auth"
	"punchflow/internal/fixtures"
	"punchflow/internal/kftime"
	"punchflow/internal/server"
)

func runServe(ctx context.Context, args []string, stderr interface{ Write([]byte) (int, error) }) error {
	fs := cliFlagSet("serve", stderr)
	common := commonFlags(fs)
	listen := fs.String("listen", envString("PUNCHFLOW_LISTEN", "127.0.0.1:8080"), "listen address")
	baseURL := fs.String("public-base-url", envString("PUNCHFLOW_PUBLIC_BASE_URL", ""), "public base URL")
	seed, err := envBool("PUNCHFLOW_SEED", "seed", args, true)
	if err != nil {
		return err
	}
	seedFlag := fs.Bool("seed", seed, "seed an empty database")
	local, err := envBool("PUNCHFLOW_ENABLE_LOCAL_LOGIN", "enable-local-login", args, false)
	if err != nil {
		return err
	}
	localFlag := fs.Bool("enable-local-login", local, "enable development local login")
	reset, err := envBool("PUNCHFLOW_ENABLE_ADMIN_RESET", "enable-admin-reset", args, false)
	if err != nil {
		return err
	}
	resetFlag := fs.Bool("enable-admin-reset", reset, "enable development fixture reset API")
	oidcIssuer := fs.String("oidc-issuer", envString("PUNCHFLOW_OIDC_ISSUER", ""), "OIDC issuer URL")
	oidcClient := fs.String("oidc-client-id", envString("PUNCHFLOW_OIDC_CLIENT_ID", "punchflow-ui"), "OIDC client ID")
	oidcSecret := fs.String("oidc-client-secret", envString("PUNCHFLOW_OIDC_CLIENT_SECRET", ""), "OIDC client secret")
	oidcRedirect := fs.String("oidc-redirect-url", envString("PUNCHFLOW_OIDC_REDIRECT_URL", ""), "OIDC callback URL")
	rateLimit, err := strconv.Atoi(envString("PUNCHFLOW_RATE_LIMIT", "0"))
	if err != nil {
		return usageError{err: fmt.Errorf("invalid PUNCHFLOW_RATE_LIMIT: %w", err)}
	}
	rate := fs.Int("rate-limit", rateLimit, "requests per minute per IP; zero disables")
	rateSecret := fs.String("rate-limit-secret", envString("PUNCHFLOW_RATE_LIMIT_SECRET", ""), "rate limit multiplier secret")
	retry := fs.Duration("webhook-retry-base", envDuration("PUNCHFLOW_WEBHOOK_RETRY_BASE", 10*time.Second), "webhook retry base delay")
	maxUpload, err := strconv.ParseInt(envString("PUNCHFLOW_MAX_UPLOAD_BYTES", "52428800"), 10, 64)
	if err != nil {
		return usageError{err: fmt.Errorf("invalid PUNCHFLOW_MAX_UPLOAD_BYTES: %w", err)}
	}
	maxUploadFlag := fs.Int64("max-upload-bytes", maxUpload, "maximum uploaded file size")
	logLevel := fs.String("log-level", envString("PUNCHFLOW_LOG_LEVEL", "info"), "debug, info, warn, or error")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if !validLogLevel(*logLevel) || *rate < 0 || *maxUploadFlag < 1 || *retry <= 0 {
		return usageError{err: fmt.Errorf("invalid serve flag value")}
	}
	common.resolve()
	if *baseURL == "" {
		*baseURL = defaultBaseURL(*listen)
	}
	if *oidcRedirect == "" {
		*oidcRedirect = strings.TrimRight(*baseURL, "/") + "/auth/oidc/callback"
	}
	if !validBaseURL(*baseURL) || !validBaseURL(*oidcRedirect) || (*oidcIssuer != "" && !validBaseURL(*oidcIssuer)) {
		return usageError{err: fmt.Errorf("public base URL, OIDC issuer, or OIDC redirect URL is invalid")}
	}
	fixtureFS, err := cliFixturesFS(*common.fixtures)
	if err != nil {
		return err
	}
	cfg := server.Config{Listen: *listen, PublicBaseURL: *baseURL, DataDir: *common.dataDir, DBPath: *common.dbPath,
		FixturesFS: fixtureFS, Seed: *seedFlag, EnableLocalLogin: *localFlag, EnableAdminReset: *resetFlag,
		OIDC:      auth.OIDCConfig{Issuer: *oidcIssuer, ClientID: *oidcClient, ClientSecret: *oidcSecret, RedirectURL: *oidcRedirect},
		RateLimit: *rate, RateLimitSecret: *rateSecret, WebhookRetryBase: *retry, MaxUploadBytes: *maxUploadFlag}
	s, err := server.New(ctx, cfg, kftime.SystemClock{})
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	defer func() { _ = s.Close() }()
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if _, err := fmt.Fprintf(stderr, "listening on http://%s\n", ln.Addr().String()); err != nil {
		_ = ln.Close()
		return err
	}
	return s.Serve(ctx, ln)
}

func envDuration(name string, fallback time.Duration) time.Duration {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return v
}

func validLogLevel(v string) bool { return v == "debug" || v == "info" || v == "warn" || v == "error" }

func defaultBaseURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://" + listen
	}
	if strings.EqualFold(host, "localhost") {
		host = "localhost"
	} else if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func cliFixturesFS(path string) (fs.FS, error) {
	if path != "" {
		return os.DirFS(path), nil
	}
	return fixtures.DefaultFS(punchflow.DefaultFixtures)
}

func validBaseURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && u.Scheme != "" && u.Host != ""
}
