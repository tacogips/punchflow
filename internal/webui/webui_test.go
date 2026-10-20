package webui

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"punchflow"
)

func TestHandlerServesAssetAndSPA(t *testing.T) {
	dist := testMapFS(map[string]string{
		"web/dist/app/index.html":      `<html><script src="/assets/a.js"></script></html>`,
		"web/dist/app/assets/a.js":     `console.log("ok")`,
		"web/dist/app/images/logo.svg": `<svg></svg>`,
	})
	handler := Handler(dist)

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/a.js", nil))
	if asset.Code != http.StatusOK || asset.Body.String() != `console.log("ok")` {
		t.Fatalf("asset response = (%d, %q), want 200 and JS body", asset.Code, asset.Body.String())
	}
	if got, want := asset.Header().Get("Cache-Control"), "public, max-age=31536000, immutable"; got != want {
		t.Fatalf("asset Cache-Control = %q, want %q", got, want)
	}

	deepLink := httptest.NewRecorder()
	handler.ServeHTTP(deepLink, httptest.NewRequest(http.MethodGet, "/tickets/123", nil))
	if deepLink.Code != http.StatusOK || !strings.Contains(deepLink.Body.String(), `<script src="/assets/a.js">`) {
		t.Fatalf("deep link response = (%d, %q), want index.html", deepLink.Code, deepLink.Body.String())
	}
	if got, want := deepLink.Header().Get("Cache-Control"), "no-cache"; got != want {
		t.Fatalf("deep link Cache-Control = %q, want %q", got, want)
	}

	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/images/logo.svg", nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD response = (%d, %q), want 200 with no body", head.Code, head.Body.String())
	}
}

func TestHandlerPlaceholderAndMethodNotAllowed(t *testing.T) {
	placeholderResponse := httptest.NewRecorder()
	Handler(testMapFS(nil)).ServeHTTP(placeholderResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if placeholderResponse.Code != http.StatusOK || !strings.Contains(placeholderResponse.Body.String(), "punchflow UI is not built") {
		t.Fatalf("placeholder response = (%d, %q)", placeholderResponse.Code, placeholderResponse.Body.String())
	}

	for _, handler := range []http.Handler{Handler(testMapFS(nil)), Handler(testMapFS(map[string]string{"web/dist/app/index.html": "index"}))} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body")))
		if response.Code != http.StatusNotFound {
			t.Fatalf("POST status = %d, want 404", response.Code)
		}
		var body struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != "endpoint_not_found" {
			t.Fatalf("POST error body = %q (decode error %v), want endpoint_not_found", response.Body.String(), err)
		}
	}
}

func TestEmbeddedBuild(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	if _, err := fs.Stat(os.DirFS(moduleRoot), "web/dist/app/index.html"); err != nil {
		t.Skip("web/dist/app/index.html is created by the final web build")
	}
	response := httptest.NewRecorder()
	Handler(punchflow.WebDist).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<script") {
		t.Fatalf("embedded UI response = (%d, %q), want built index with script tag", response.Code, response.Body.String())
	}
}

func testMapFS(files map[string]string) fs.FS {
	mapFS := make(fstest.MapFS, len(files))
	for name, content := range files {
		mapFS[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return mapFS
}
