package httpx

import (
	"net/http"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
)

// QueryArray reads repeated key[] values and optionally accepts one bare key=value.
func QueryArray(r *http.Request, key string, allowBare bool) ([]string, error) {
	q := r.URL.Query()
	values := append([]string(nil), q[key+"[]"]...)
	bare, hasBare := q[key]
	if hasBare && (!allowBare || len(bare) != 1) {
		return nil, kferr.InvalidParameter("Invalid request: value at root is not an array")
	}
	if hasBare {
		values = append(values, bare[0])
	}
	return values, nil
}

// QueryStrings reads a list-valued parameter using key[]=value serialization.
// A single bare key=value is also accepted for compatibility, but repeated
// bare values are ambiguous and are rejected.
func QueryStrings(r *http.Request, key string) ([]string, error) {
	return QueryArray(r, key, true)
}

// QueryTime parses an optional timestamp or date query parameter.
func QueryTime(r *http.Request, key string) (*time.Time, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return nil, nil
	}
	t, err := kftime.ParseInput(value)
	if err != nil {
		return nil, kferr.InvalidParameter("Invalid " + key)
	}
	return &t, nil
}

// QueryString reads an optional string query parameter with a maximum rune count.
func QueryString(r *http.Request, key string, maxLen int) (string, error) {
	value := r.URL.Query().Get(key)
	if maxLen >= 0 && len([]rune(value)) > maxLen {
		return "", kferr.InvalidParameter("Invalid " + key)
	}
	return value, nil
}
