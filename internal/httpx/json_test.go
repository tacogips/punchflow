package httpx

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"punchflow/internal/kferr"
)

func TestJSONWireAndDecode(t *testing.T) {
	t.Run("compact HTML escaped output", func(t *testing.T) {
		rr := httptest.NewRecorder()
		WriteJSON(rr, 201, map[string]string{"x": "<a&b>"})
		if rr.Code != 201 || rr.Header().Get("Content-Type") != "application/json; charset=utf-8" || rr.Body.String() != `{"x":"\u003ca\u0026b\u003e"}` {
			t.Fatalf("unexpected response: %d %q %q", rr.Code, rr.Header().Get("Content-Type"), rr.Body.String())
		}
	})
	t.Run("empty response", func(t *testing.T) {
		rr := httptest.NewRecorder()
		WriteEmpty(rr)
		if rr.Code != 200 || rr.Body.Len() != 0 || rr.Header().Get("Content-Type") != "" {
			t.Fatalf("unexpected empty response: %#v", rr)
		}
	})
	cases := []struct {
		name, contentType, body string
		want                    string
	}{
		{"wrong type", "text/plain", `{}`, "invalid_content_type"},
		{"bad JSON", "application/json", `{"a":`, "invalid_json"},
		{"type mismatch", "application/json", `{"to":"x"}`, "invalid_parameter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			var dst struct {
				To int `json:"to"`
			}
			err := DecodeJSON(r, &dst)
			apiErr, ok := kferr.As(err)
			if !ok || apiErr.Code != tc.want {
				t.Fatalf("DecodeJSON error = %v", err)
			}
			if tc.name == "type mismatch" && !strings.Contains(apiErr.Message, "expected") {
				t.Fatalf("type mismatch message: %q", apiErr.Message)
			}
		})
	}
	t.Run("body too large", func(t *testing.T) {
		body := bytes.NewReader(bytes.Repeat([]byte(" "), MaxJSONBody+1))
		r := httptest.NewRequest("POST", "/", body)
		r.Header.Set("Content-Type", "application/json")
		if err := DecodeJSON(r, &struct{}{}); !kferrErrorCode(err, "payload_too_large") {
			t.Fatalf("DecodeJSON error = %v", err)
		}
	})
	t.Run("empty body", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", nil)
		if err := DecodeJSON(r, &struct{}{}); err != nil {
			t.Fatal(err)
		}
	})
}

func kferrErrorCode(err error, want string) bool {
	apiErr, ok := kferr.As(err)
	return ok && apiErr.Code == want
}
