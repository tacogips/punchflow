// Package webui serves the embedded browser application.
package webui

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"punchflow/internal/kferr"
)

const placeholder = "<!doctype html><html><body>punchflow UI is not built. Run `mise run web:build`.</body></html>\n"

// Handler serves the built SPA when it is present in dist. If the frontend has
// not been built, it serves a small development hint instead.
func Handler(dist fs.FS) http.Handler {
	app, err := fs.Sub(dist, "web/dist/app")
	if err != nil {
		return placeholderHandler()
	}
	index, err := fs.ReadFile(app, "index.html")
	if err != nil {
		return placeholderHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			kferr.Write(w, kferr.EndpointNotFound())
			return
		}

		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		name = path.Clean(name)
		if !fs.ValidPath(name) {
			name = "index.html"
		}
		if data, readErr := fs.ReadFile(app, name); readErr == nil {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		kferr.Write(w, kferr.EndpointNotFound())
	})
}

func placeholderHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			kferr.Write(w, kferr.EndpointNotFound())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(placeholder))
	})
}
