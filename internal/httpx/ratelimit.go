package httpx

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
)

type pf04RateBucket struct {
	start time.Time
	count int
	set   bool
}

// RateLimiter implements the optional fixed 60-second per-IP limit.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	secret  string
	clock   kftime.Clock
	buckets map[string]pf04RateBucket
}

// NewRateLimiter creates a fixed-window rate limiter; zero disables it.
func NewRateLimiter(limit int, secret string, clock kftime.Clock) *RateLimiter {
	if clock == nil {
		clock = kftime.SystemClock{}
	}
	return &RateLimiter{limit: limit, secret: secret, clock: clock, buckets: make(map[string]pf04RateBucket)}
}

// Middleware sets rate headers and rejects requests beyond the configured limit.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l == nil || l.limit <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		limit := l.limit
		secretBucket := l.secret != "" && r.Header.Get("X-Rate-Limit-Secret") == l.secret
		if secretBucket && limit <= int(^uint(0)>>1)/10 {
			limit *= 10
		}
		key := ClientIP(r)
		if secretBucket {
			key += "\x00secret"
		}
		now := l.clock.Now()
		l.mu.Lock()
		bucket := l.buckets[key]
		if !bucket.set || now.Sub(bucket.start) >= time.Minute || now.Before(bucket.start) {
			bucket = pf04RateBucket{start: now, set: true}
		}
		bucket.count++
		l.buckets[key] = bucket
		remaining := limit - bucket.count
		if remaining < 0 {
			remaining = 0
		}
		reset := bucket.start.Add(time.Minute).Unix()
		l.mu.Unlock()
		w.Header().Set("RateLimit-Limit", strconv.Itoa(limit))
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("RateLimit-Reset", strconv.FormatInt(reset, 10))
		if remaining == 0 && bucket.count > limit {
			kferr.Write(w, kferr.RateLimited())
			return
		}
		next.ServeHTTP(w, r)
	})
}
