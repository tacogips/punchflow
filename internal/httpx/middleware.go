package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"

	"punchflow/internal/kferr"
)

type pf04RequestIDKey struct{}

// Chain composes middleware with the first middleware as the outermost layer.
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// RequestID adds a generated request identifier to each response and context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := pf04UUID()
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), pf04RequestIDKey{}, id)))
	})
}

// RequestIDFrom returns the request id attached by RequestID, or an empty string.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(pf04RequestIDKey{}).(string)
	return id
}

func pf04UUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" + hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:16])
}

// Recover logs panics with their request id and emits the standard internal error.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				if value == http.ErrAbortHandler {
					panic(value)
				}
				slog.Error("http handler panic", "request_id", RequestIDFrom(r.Context()), "panic", value)
				kferr.Write(w, kferr.Internal())
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS adds the public API CORS headers and handles preflight requests.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Expose-Headers", "Link, Page, Per-Page, Total, RateLimit-Limit, RateLimit-Remaining, RateLimit-Reset, X-Request-Id")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Caller-Id, X-Rate-Limit-Secret")
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// LengthRequired enforces Content-Length for HTTP/1.x POST and PUT requests.
func LengthRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 1 && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			_, hasLength := r.Header[http.CanonicalHeaderKey("Content-Length")]
			if !hasLength && len(r.TransferEncoding) == 0 {
				w.WriteHeader(http.StatusLengthRequired)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ClientIP returns the host portion of RemoteAddr without trusting proxy headers.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
