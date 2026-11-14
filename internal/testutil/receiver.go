package testutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

// Received is one HTTP request captured by a Receiver.
type Received struct {
	Header http.Header
	Body   []byte
}

// Receiver is a thread-safe local endpoint for webhook delivery tests.
type Receiver struct {
	mu       sync.Mutex
	received []Received
	statuses []int
	server   *httptest.Server
}

func newReceiver() *Receiver {
	r := &Receiver{}
	r.server = httptest.NewServer(http.HandlerFunc(r.serveHTTP))
	return r
}

func (r *Receiver) serveHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		http.Error(w, "could not read request body", http.StatusBadRequest)
		return
	}
	r.mu.Lock()
	r.received = append(r.received, Received{Header: req.Header.Clone(), Body: append([]byte(nil), body...)})
	status := http.StatusOK
	if len(r.statuses) > 0 {
		status = r.statuses[0]
		if len(r.statuses) > 1 {
			r.statuses = r.statuses[1:]
		}
	}
	r.mu.Unlock()
	w.WriteHeader(status)
}

// Requests returns a snapshot of all requests received so far.
func (r *Receiver) Requests() []Received {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]Received, len(r.received))
	for i, request := range r.received {
		requests[i] = Received{Header: request.Header.Clone(), Body: append([]byte(nil), request.Body...)}
	}
	return requests
}

// RespondWith sets the response sequence for subsequent requests. When the
// sequence is exhausted, the last status is reused.
func (r *Receiver) RespondWith(statuses ...int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses[:0], statuses...)
}
