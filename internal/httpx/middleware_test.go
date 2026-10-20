package httpx

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCORSPrefightAndRequestIDRecovery(t *testing.T) {
	called := false
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), CORS)
	r := httptest.NewRequest("OPTIONS", "/v1/x", nil)
	r.Header.Set("Access-Control-Request-Method", "GET")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if called || rr.Code != 200 || rr.Header().Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(rr.Header().Get("Access-Control-Expose-Headers"), "X-Request-Id") {
		t.Fatalf("preflight: called=%v code=%d headers=%v", called, rr.Code, rr.Header())
	}
	h = Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("test panic") }), RequestID, Recover)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != 500 || rr.Header().Get("X-Request-Id") == "" || !strings.Contains(rr.Body.String(), "internal_server_error") {
		t.Fatalf("recovery: %d %v %s", rr.Code, rr.Header(), rr.Body.String())
	}
}

func TestLengthRequiredRawHTTP(t *testing.T) {
	s := httptest.NewServer(LengthRequired(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })))
	defer s.Close()
	addr := strings.TrimPrefix(s.URL, "http://")
	for _, tc := range []struct {
		line string
		want int
	}{{"POST /x HTTP/1.1\r\nHost: test\r\nConnection: close\r\n\r\n", 411}, {"POST /x HTTP/1.1\r\nHost: test\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", 204}} {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fmt.Fprint(conn, tc.line); err != nil {
			t.Fatal(err)
		}
		status, err := bufio.NewReader(conn).ReadString('\n')
		_ = conn.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(status, fmt.Sprintf(" %d ", tc.want)) {
			t.Fatalf("status %q want %d", status, tc.want)
		}
	}
}

func TestLengthRequiredPassesChunkedAndNonHTTP1(t *testing.T) {
	called := false
	h := LengthRequired(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(204) }))
	r := httptest.NewRequest("POST", "/", strings.NewReader("x"))
	r.TransferEncoding = []string{"chunked"}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if !called || rr.Code != 204 {
		t.Fatalf("chunked request rejected: %d", rr.Code)
	}
	called = false
	r = httptest.NewRequest("POST", "/", nil)
	r.ProtoMajor = 2
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if !called || rr.Code != 204 {
		t.Fatalf("non HTTP/1 request rejected: %d", rr.Code)
	}
}
