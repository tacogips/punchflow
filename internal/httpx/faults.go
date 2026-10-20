package httpx

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"punchflow/internal/kferr"
)

// FaultRule describes one temporary response override.
type FaultRule struct {
	ID, Method, PathPrefix, Code, Message string
	Status, Remaining                     int
	CreatedAt                             time.Time
}

// FaultSet stores in-memory fault rules safely across concurrent requests.
type FaultSet struct {
	mu    sync.Mutex
	rules []FaultRule
}

// NewFaultSet creates an empty fault set.
func NewFaultSet() *FaultSet { return &FaultSet{} }

// Add inserts a rule, assigning its id and creation time when absent.
func (f *FaultSet) Add(rule FaultRule) FaultRule {
	f.mu.Lock()
	defer f.mu.Unlock()
	if rule.ID == "" {
		rule.ID = pf04UUID()
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	f.rules = append(f.rules, rule)
	return rule
}

// List returns a snapshot of the rules in insertion order.
func (f *FaultSet) List() []FaultRule {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FaultRule(nil), f.rules...)
}

// Delete removes a rule by id and reports whether it existed.
func (f *FaultSet) Delete(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, rule := range f.rules {
		if rule.ID == id {
			f.rules = append(f.rules[:i], f.rules[i+1:]...)
			return true
		}
	}
	return false
}

// Clear removes all fault rules.
func (f *FaultSet) Clear() { f.mu.Lock(); f.rules = nil; f.mu.Unlock() }

// Middleware applies the first matching fault rule before passing through.
func (f *FaultSet) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f == nil {
			next.ServeHTTP(w, r)
			return
		}
		f.mu.Lock()
		for i := 0; i < len(f.rules); i++ {
			rule := &f.rules[i]
			if rule.Method != "" && !strings.EqualFold(rule.Method, r.Method) {
				continue
			}
			if !strings.HasPrefix(r.URL.Path, rule.PathPrefix) {
				continue
			}
			copy := *rule
			if rule.Remaining > 0 {
				rule.Remaining--
				if rule.Remaining == 0 {
					f.rules = append(f.rules[:i], f.rules[i+1:]...)
				}
			}
			f.mu.Unlock()
			message := copy.Message
			if message == "" {
				message = "fault injected"
			}
			kferr.Write(w, kferr.New(copy.Status, copy.Code, message))
			return
		}
		f.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}
