package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestFaultRuleMatchCountdownAndDelete(t *testing.T) {
	f := NewFaultSet()
	added := f.Add(FaultRule{Method: "GET", PathPrefix: "/v1/tickets", Status: 504, Code: "gateway_timeout", Remaining: 1})
	if added.ID == "" || added.CreatedAt.IsZero() {
		t.Fatalf("rule fields not assigned: %#v", added)
	}
	h := f.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "/v1/tickets/1", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != 504 || !strings.Contains(rr.Body.String(), "gateway_timeout") || len(f.List()) != 0 {
		t.Fatalf("fault response: %d %s rules=%v", rr.Code, rr.Body.String(), f.List())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != 204 {
		t.Fatalf("one-shot fault repeated: %d", rr.Code)
	}
	other := f.Add(FaultRule{PathPrefix: "/x", Status: 503, Code: "feature_disabled"})
	if !f.Delete(other.ID) || f.Delete(other.ID) {
		t.Fatal("Delete result incorrect")
	}
	f.Add(FaultRule{PathPrefix: "/never", Status: 500, Code: "x"})
	f.Clear()
	if len(f.List()) != 0 {
		t.Fatalf("Clear left rules: %v", f.List())
	}
}

func TestFaultSetConcurrentAccess(t *testing.T) {
	f := NewFaultSet()
	h := f.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rule := f.Add(FaultRule{PathPrefix: "/no-match", Status: 503, Code: "x"})
			_ = f.List()
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest("GET", "/ok", nil))
			f.Delete(rule.ID)
		}()
	}
	wg.Wait()
	f.Clear()
}
