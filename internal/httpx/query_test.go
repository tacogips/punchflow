package httpx

import (
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestQueryHelpers(t *testing.T) {
	r := httptest.NewRequest("GET", "/?status%5B%5D=a&status%5B%5D=b", nil)
	if got, err := QueryArray(r, "status", false); err != nil || !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("array = %v, %v", got, err)
	}
	r = httptest.NewRequest("GET", "/?status=a", nil)
	if got, err := QueryArray(r, "status", true); err != nil || !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("bare array = %v, %v", got, err)
	}
	r = httptest.NewRequest("GET", "/?status=a&status=b", nil)
	if _, err := QueryArray(r, "status", true); !kferrErrorCode(err, "invalid_parameter") {
		t.Fatalf("repeated bare error = %v", err)
	}
	r = httptest.NewRequest("GET", "/?status=a", nil)
	if _, err := QueryArray(r, "status", false); !kferrErrorCode(err, "invalid_parameter") {
		t.Fatalf("disallowed bare error = %v", err)
	}
	r = httptest.NewRequest("GET", "/?when=2026-10-20", nil)
	got, err := QueryTime(r, "when")
	if err != nil || got == nil || got.Hour() != 0 || got.UTC().Format("15:04") != "15:00" {
		t.Fatalf("QueryTime = %v, %v", got, err)
	}
	r = httptest.NewRequest("GET", "/?when=bad", nil)
	if _, err := QueryTime(r, "when"); !kferrErrorCode(err, "invalid_parameter") {
		t.Fatalf("bad time = %v", err)
	}
	r = httptest.NewRequest("GET", "/?q=ab", nil)
	if got, err := QueryString(r, "q", 2); err != nil || got != "ab" {
		t.Fatalf("QueryString = %q, %v", got, err)
	}
	if _, err := QueryString(httptest.NewRequest("GET", "/?q=abc", nil), "q", 2); !kferrErrorCode(err, "invalid_parameter") {
		t.Fatalf("long string = %v", err)
	}
}
