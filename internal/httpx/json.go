package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"punchflow/internal/kferr"
)

// MaxJSONBody is the maximum accepted JSON request body size (10 MiB).
const MaxJSONBody = 10 << 20

// WriteJSON writes compact, HTML-escaped JSON without a trailing newline.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// WriteEmpty writes a successful response with no body or content type.
func WriteEmpty(w http.ResponseWriter) { w.WriteHeader(http.StatusOK) }

// DecodeJSON decodes an optional JSON body and converts wire failures to API errors.
func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	if r.ContentLength == 0 && len(r.TransferEncoding) == 0 {
		return nil
	}
	mediaType := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])
	if !strings.EqualFold(mediaType, "application/json") {
		return kferr.InvalidContentType()
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, MaxJSONBody))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return kferr.PayloadTooLarge()
		}
		return kferr.InvalidJSON()
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, dst); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return kferr.InvalidParameter(fmt.Sprintf("expected %s, but received %s", pf04JSONType(typeErr.Type), pf04JSONValueType(typeErr.Value)))
		}
		return kferr.InvalidJSON()
	}
	return nil
}

func pf04JSONType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "String"
	case reflect.Bool:
		return "Boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "Integer"
	case reflect.Float32, reflect.Float64:
		return "Number"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	default:
		return t.String()
	}
}

func pf04JSONValueType(v string) string {
	switch v {
	case "string":
		return "String"
	case "number":
		return "Number"
	case "bool":
		return "Boolean"
	case "array":
		return "array"
	case "object":
		return "object"
	case "null":
		return "null"
	default:
		return v
	}
}
