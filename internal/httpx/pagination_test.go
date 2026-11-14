package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParsePageAndPagination(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/items?filter=x", nil)
	p, err := ParsePage(r)
	if err != nil || p != (Page{1, 25}) {
		t.Fatalf("defaults: %#v, %v", p, err)
	}
	for _, raw := range []string{"?perPage=101", "?perPage=abc", "?page=0"} {
		if _, err := ParsePage(httptest.NewRequest("GET", "/"+raw, nil)); !kferrErrorCode(err, "invalid_parameter") {
			t.Fatalf("ParsePage(%s) error %v", raw, err)
		}
	}
	r = httptest.NewRequest("GET", "/v1/items?status=open&page=2&sortBy=id", nil)
	rr := httptest.NewRecorder()
	SetPagination(rr, r, "https://example.test", Page{2, 25}, 120)
	want := `<https://example.test/v1/items?page=1&perPage=25&sortBy=id&status=open>; rel="first", <https://example.test/v1/items?page=1&perPage=25&sortBy=id&status=open>; rel="prev", <https://example.test/v1/items?page=3&perPage=25&sortBy=id&status=open>; rel="next", <https://example.test/v1/items?page=5&perPage=25&sortBy=id&status=open>; rel="last"`
	if rr.Header().Get("Link") != want || rr.Header().Get("Total") != "120" {
		t.Fatalf("headers: %#v", rr.Header())
	}
	for _, tc := range []struct {
		page                int
		wantFirst, wantNext bool
	}{{1, false, true}, {5, true, false}} {
		w := httptest.NewRecorder()
		SetPagination(w, httptest.NewRequest("GET", "/x", nil), "https://x", Page{tc.page, 25}, 120)
		link := w.Header().Get("Link")
		if strings.Contains(link, `rel="first"`) != tc.wantFirst || strings.Contains(link, `rel="next"`) != tc.wantNext {
			t.Fatalf("page %d links %q", tc.page, link)
		}
	}
}

func TestParseSort(t *testing.T) {
	r := httptest.NewRequest("GET", "/?sortBy=createdAt-desc", nil)
	if got := ParseSort(r, "createdAt"); got != (Sort{Field: "createdAt", Desc: true, Set: true}) {
		t.Fatalf("got %#v", got)
	}
	if got := ParseSort(httptest.NewRequest("GET", "/?sortBy=name", nil), "createdAt"); got.Set {
		t.Fatalf("unsupported sort accepted: %#v", got)
	}
	if got := ParseSort(httptest.NewRequest("GET", "/?sortBy=createdAt-foo", nil), "createdAt"); got.Set {
		t.Fatalf("unsupported suffix accepted: %#v", got)
	}
}
