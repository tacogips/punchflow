package testutil

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

// Response contains the status, headers, and response bytes from an Env request.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// JSON decodes the response body into v or fails the test.
func (r *Response) JSON(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("decode response JSON %q: %v", r.Body, err)
	}
}

// AssertError checks the status and kickflow error code.
func AssertError(t *testing.T, r *Response, status int, code string) {
	t.Helper()
	if r.Status != status {
		t.Fatalf("response status = %d, want %d (body %q)", r.Status, status, r.Body)
	}
	var body struct {
		Code string `json:"code"`
	}
	r.JSON(t, &body)
	if body.Code != code {
		t.Fatalf("error code = %q, want %q (body %q)", body.Code, code, r.Body)
	}
}

// AssertPagination checks the pagination headers for one response page.
func AssertPagination(t *testing.T, r *Response, page, perPage, total int) {
	t.Helper()
	for header, want := range map[string]int{"Page": page, "Per-Page": perPage, "Total": total} {
		got, err := strconv.Atoi(r.Header.Get(header))
		if err != nil || got != want {
			t.Errorf("%s header = %q (parse error %v), want %d", header, r.Header.Get(header), err, want)
		}
	}
	if r.Header.Get("Link") == "" {
		t.Errorf("Link header is empty")
	}
}

// AssertKeys checks that obj has exactly the provided keys.
func AssertKeys(t *testing.T, obj map[string]any, keys ...string) {
	t.Helper()
	want := append([]string(nil), keys...)
	got := make([]string, 0, len(obj))
	for key := range obj {
		got = append(got, key)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("object keys = %v, want %v", got, want)
	}
}
